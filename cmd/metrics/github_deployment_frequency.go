package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"devctl-em/internal/output"
)

var deploymentFrequencyCmd = &cobra.Command{
	Use:   "deployment-frequency",
	Short: "Measure deployment frequency (DORA metric)",
	Long: `Count successful runs of configured deploy workflows and compute DORA rating.

DORA deployment frequency ratings:
  Elite  — More than once per day
  High   — Once per day to once per week
  Medium — Once per week to once per month
  Low    — Less than once per month

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
	Repo         string
	Workflow     string
	Deployments  int
	DeploysWeek  float64
	DORA         string
	Error        string
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
			}
		}

		deploysPerWeek := float64(successCount) / weeks
		dora := classifyDORA(deploysPerWeek)

		results = append(results, repoDeploymentResult{
			Repo:        repo,
			Workflow:    wfFilename,
			Deployments: successCount,
			DeploysWeek: deploysPerWeek,
			DORA:        dora,
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
	fmt.Printf("| %-*s | %-*s | %11s | %12s | %-6s |\n",
		repoW, "Repo", wfW, "Workflow", "Deployments", "Deploys/Week", "DORA")
	fmt.Printf("|%s|%s|%s|%s|%s|\n",
		strings.Repeat("-", repoW+2),
		strings.Repeat("-", wfW+2),
		strings.Repeat("-", 13),
		strings.Repeat("-", 14),
		strings.Repeat("-", 8))

	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("| %-*s | %-*s | %11s | %12s | %-6s |\n",
				repoW, r.Repo, wfW, r.Workflow, r.Error, "", "")
		} else {
			fmt.Printf("| %-*s | %-*s | %11d | %12.1f | %-6s |\n",
				repoW, r.Repo, wfW, r.Workflow, r.Deployments, r.DeploysWeek, r.DORA)
		}
	}

	// CSV export
	if getGithubOutputFormat("") == "csv" {
		outputPath := getGithubOutputPath("deployment-frequency", "csv")
		if err := exportDeploymentFrequencyCSV(results, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("\nExported to %s\n", outputPath)
	}

	return nil
}

// classifyDORA returns the DORA deployment frequency classification.
func classifyDORA(deploysPerWeek float64) string {
	deploysPerDay := deploysPerWeek / 7.0
	deploysPerMonth := deploysPerWeek * (30.0 / 7.0)

	if deploysPerDay > 1 {
		return "Elite"
	}
	if deploysPerWeek >= 1 {
		return "High"
	}
	if deploysPerMonth >= 1 {
		return "Medium"
	}
	return "Low"
}

func exportDeploymentFrequencyCSV(results []repoDeploymentResult, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Repo", "Workflow", "Deployments", "Deploys/Week", "DORA"}
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
			r.DORA,
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
