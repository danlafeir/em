package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/internal/charts"
	gh "devctl-em/internal/github"
	"devctl-em/internal/output"
)

var deploymentFrequencyCmd = &cobra.Command{
	Use:   "deployment-frequency",
	Short: "Measure deployment frequency",
	Long: `Count successful runs of configured deploy workflows.

Run "devctl-em metrics github setup" first to configure deploy workflows.

Examples:
  devctl-em metrics github deployment-frequency
  devctl-em metrics github deployment-frequency --from 2025-01-01 --to 2025-06-30
  devctl-em metrics github deployment-frequency -f csv -o deployments.csv`,
	RunE: runDeploymentFrequency,
}

func init() {
	GithubCmd.AddCommand(deploymentFrequencyCmd)
}

type repoDeploymentResult struct {
	Repo        string
	Workflow    string
	Deployments int
	DeploysWeek float64
	Error       string
}

func runDeploymentFrequency(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getGithubClient()
	if err != nil {
		return err
	}

	fmt.Println("Testing GitHub connection...")
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to GitHub: %w", err)
	}

	org := getGithubOrg()
	if org == "" {
		return fmt.Errorf("GitHub org not configured. Run: devctl-em config set github.org <org>")
	}

	workflows, err := getConfiguredWorkflows()
	if err != nil {
		return err
	}

	from, to, err := getGithubDateRange()
	if err != nil {
		return err
	}

	weeks := to.Sub(from).Hours() / (24 * 7)
	if weeks < 1 {
		weeks = 1
	}

	fmt.Printf("Date range: %s to %s (%.1f weeks)\n", from.Format("2006-01-02"), to.Format("2006-01-02"), weeks)
	fmt.Printf("Checking %d configured repositories...\n\n", len(workflows))

	var results []repoDeploymentResult
	var allRuns []gh.WorkflowRun

	// Sort repos for deterministic output
	repos := make([]string, 0, len(workflows))
	for repo := range workflows {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	for _, repo := range repos {
		wfFilename := workflows[repo]
		fmt.Printf("  %s (%s)...", repo, wfFilename)

		// Find workflow ID by matching filename
		allWfs, err := client.ListWorkflows(ctx, org, repo)
		if err != nil {
			results = append(results, repoDeploymentResult{
				Repo:     repo,
				Workflow: wfFilename,
				Error:    fmt.Sprintf("list workflows: %v", err),
			})
			fmt.Println(" error")
			continue
		}

		var workflowID int64
		for _, wf := range allWfs {
			if filepath.Base(wf.Path) == wfFilename {
				workflowID = wf.ID
				break
			}
		}
		if workflowID == 0 {
			results = append(results, repoDeploymentResult{
				Repo:     repo,
				Workflow: wfFilename,
				Error:    "workflow not found",
			})
			fmt.Println(" not found")
			continue
		}

		runs, err := client.ListWorkflowRuns(ctx, org, repo, workflowID, "", from, to)
		if err != nil {
			results = append(results, repoDeploymentResult{
				Repo:     repo,
				Workflow: wfFilename,
				Error:    fmt.Sprintf("list runs: %v", err),
			})
			fmt.Println(" error")
			continue
		}

		// Count only successful runs
		successCount := 0
		for _, run := range runs {
			if run.Conclusion == "success" {
				successCount++
				allRuns = append(allRuns, run)
			}
		}

		deploysPerWeek := float64(successCount) / weeks

		results = append(results, repoDeploymentResult{
			Repo:        repo,
			Workflow:    wfFilename,
			Deployments: successCount,
			DeploysWeek: deploysPerWeek,
		})
		fmt.Printf(" %d deployments\n", successCount)
	}

	// Print table
	fmt.Printf("\nDeployment Frequency\n")
	fmt.Printf("====================\n\n")

	// Calculate column widths
	repoW := 4 // "Repo"
	wfW := 8   // "Workflow"
	for _, r := range results {
		if len(r.Repo) > repoW {
			repoW = len(r.Repo)
		}
		if len(r.Workflow) > wfW {
			wfW = len(r.Workflow)
		}
	}

	// Header
	fmt.Printf("| %-*s | %-*s | %11s | %12s |\n",
		repoW, "Repo", wfW, "Workflow", "Deployments", "Deploys/Week")
	fmt.Printf("|%s|%s|%s|%s|\n",
		strings.Repeat("-", repoW+2),
		strings.Repeat("-", wfW+2),
		strings.Repeat("-", 13),
		strings.Repeat("-", 14))

	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("| %-*s | %-*s | %11s | %12s |\n",
				repoW, r.Repo, wfW, r.Workflow, r.Error, "")
		} else {
			fmt.Printf("| %-*s | %-*s | %11d | %12.1f |\n",
				repoW, r.Repo, wfW, r.Workflow, r.Deployments, r.DeploysWeek)
		}
	}

	// CSV export
	if getGithubOutputFormat("png") == "csv" {
		outputPath := getGithubOutputPath("deployment-frequency", "csv")
		if err := exportDeploymentFrequencyCSV(results, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("\nExported to %s\n", outputPath)
		return nil
	}

	// Generate PNG chart with aggregate weekly deployments
	weeklyData := aggregateWeeklyDeployments(allRuns, from, to)
	if len(weeklyData) > 0 {
		cfg := charts.DefaultConfig()
		p, err := charts.DeploymentFrequencyLine(weeklyData, cfg)
		if err != nil {
			return fmt.Errorf("failed to create chart: %w", err)
		}
		outputPath := getGithubOutputPath("deployment-frequency", "png")
		if err := charts.SaveChart(p, outputPath, cfg); err != nil {
			return fmt.Errorf("failed to save chart: %w", err)
		}
		fmt.Printf("\nChart saved to %s\n", outputPath)
	}

	return nil
}

// aggregateWeeklyDeployments buckets all runs into ISO weeks and returns sorted weekly counts.
func aggregateWeeklyDeployments(runs []gh.WorkflowRun, from, to time.Time) []charts.DeploymentWeek {
	counts := make(map[time.Time]int)

	for _, run := range runs {
		year, week := run.CreatedAt.ISOWeek()
		// Monday of ISO week
		weekStart := isoWeekStart(year, week)
		counts[weekStart]++
	}

	// Fill in zero-count weeks within the range
	startYear, startWeek := from.ISOWeek()
	cur := isoWeekStart(startYear, startWeek)
	for !cur.After(to) {
		if _, ok := counts[cur]; !ok {
			counts[cur] = 0
		}
		cur = cur.AddDate(0, 0, 7)
	}

	// Sort by week
	weeks := make([]charts.DeploymentWeek, 0, len(counts))
	for ws, c := range counts {
		weeks = append(weeks, charts.DeploymentWeek{WeekStart: ws, Count: c})
	}
	sort.Slice(weeks, func(i, j int) bool {
		return weeks[i].WeekStart.Before(weeks[j].WeekStart)
	})

	return weeks
}

// isoWeekStart returns the Monday of the given ISO year/week.
func isoWeekStart(year, week int) time.Time {
	// Jan 4 is always in ISO week 1
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	// Weekday offset to Monday
	offset := int(time.Monday - jan4.Weekday())
	if offset > 0 {
		offset -= 7
	}
	w1Monday := jan4.AddDate(0, 0, offset)
	return w1Monday.AddDate(0, 0, (week-1)*7)
}

func exportDeploymentFrequencyCSV(results []repoDeploymentResult, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Repo", "Workflow", "Deployments", "Deploys/Week"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, r := range results {
		deploysWeek := ""
		if r.Error == "" {
			deploysWeek = fmt.Sprintf("%.1f", math.Round(r.DeploysWeek*10)/10)
		}
		row := []string{
			r.Repo,
			r.Workflow,
			fmt.Sprintf("%d", r.Deployments),
			deploysWeek,
		}
		if r.Error != "" {
			row[2] = r.Error
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
