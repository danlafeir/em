package metrics

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danlafeir/em/internal/charts"
	gh "github.com/danlafeir/em/internal/github"
	"github.com/danlafeir/em/pkg/metrics"
)

var deploymentFrequencyCmd = &cobra.Command{
	Use:   "deployment-frequency",
	Short: "Measure deployment frequency",
	Long: `Count successful deploy workflow runs per repository.

Required:
  em metrics github config`,
	RunE: runDeploymentFrequency,
}

func init() {
	GithubCmd.AddCommand(deploymentFrequencyCmd)
}

type repoDeploymentResult struct {
	Team        string
	Repo        string
	Workflow    string
	Deployments int
	DeploysWeek float64
	Error       string
}

func runDeploymentFrequency(cmd *cobra.Command, args []string) error {
	fmt.Println("GitHub")
	fmt.Println(sectionDivider)
	fmt.Println()

	ctx := context.Background()

	from, to, err := getGithubDateRange()
	if err != nil {
		return err
	}

	if useSavedDataFlag {
		weeklyData, loadErr := loadDeploymentData("")
		if loadErr != nil {
			return fmt.Errorf("use-saved-data: %w", loadErr)
		}
		cfg := charts.Config{}
		outputPath := getGithubOutputPath("deployment-frequency", "html")
		if err := charts.DeploymentFrequencyLine(weeklyData, metrics.ThroughputResult{}, cfg, outputPath); err != nil {
			return fmt.Errorf("failed to create chart: %w", err)
		}
		fmt.Printf("\nChart saved to %s\n", outputPath)
		openBrowser(outputPath)
		return nil
	}

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
		return fmt.Errorf("GitHub org not configured. Run: em config set github.org <org>")
	}

	allTeamWorkflows, err := getAllConfiguredWorkflows()
	if err != nil {
		return err
	}

	weeks := to.Sub(from).Hours() / (24 * 7)
	if weeks < 1 {
		weeks = 1
	}

	// Count total repos across all teams
	totalRepos := 0
	for _, tw := range allTeamWorkflows {
		totalRepos += len(tw.Workflows)
	}

	multiTeam := len(allTeamWorkflows) > 1

	fmt.Printf("Date range: %s to %s (%.1f weeks)\n", from.Format("2006-01-02"), to.Format("2006-01-02"), weeks)
	fmt.Printf("Checking %d configured repositories across %d team(s)...\n\n", totalRepos, len(allTeamWorkflows))

	var results []repoDeploymentResult
	var allRuns []gh.WorkflowRun
	var failedRuns []gh.WorkflowRun

	for _, tw := range allTeamWorkflows {
		if multiTeam {
			fmt.Printf("[%s]\n", tw.Team)
		}

		// Sort repos for deterministic output
		repos := make([]string, 0, len(tw.Workflows))
		for repo := range tw.Workflows {
			repos = append(repos, repo)
		}
		sort.Strings(repos)

		for _, repo := range repos {
			wfFilenames := tw.Workflows[repo]
			label := strings.Join(wfFilenames, ", ")
			fmt.Printf("  %s (%s)...", repo, label)

			allWfs, err := client.ListWorkflows(ctx, org, repo)
			if err != nil {
				results = append(results, repoDeploymentResult{
					Team:     tw.Team,
					Repo:     repo,
					Workflow: label,
					Error:    fmt.Sprintf("list workflows: %v", err),
				})
				fmt.Println(" error")
				continue
			}

			// Build filename → ID index
			wfIndex := make(map[string]int64, len(allWfs))
			for _, wf := range allWfs {
				wfIndex[filepath.Base(wf.Path)] = wf.ID
			}

			successCount := 0
			var firstErr string
			for _, wfFilename := range wfFilenames {
				workflowID := wfIndex[wfFilename]
				if workflowID == 0 {
					firstErr = fmt.Sprintf("%s not found", wfFilename)
					continue
				}

				runs, err := client.ListWorkflowRuns(ctx, org, repo, workflowID, "", from, to)
				if err != nil {
					firstErr = fmt.Sprintf("list runs for %s: %v", wfFilename, err)
					continue
				}

				for _, run := range runs {
					switch run.Conclusion {
					case "success":
						successCount++
						allRuns = append(allRuns, run)
					case "failure", "timed_out":
						failedRuns = append(failedRuns, run)
					}
				}
			}

			if successCount == 0 && firstErr != "" {
				results = append(results, repoDeploymentResult{
					Team:     tw.Team,
					Repo:     repo,
					Workflow: label,
					Error:    firstErr,
				})
				fmt.Println(" error")
				continue
			}

			results = append(results, repoDeploymentResult{
				Team:        tw.Team,
				Repo:        repo,
				Workflow:    label,
				Deployments: successCount,
				DeploysWeek: float64(successCount) / weeks,
			})
			fmt.Printf(" %d deployments\n", successCount)
		}
	}

	// Print table
	fmt.Printf("\nDeployment Frequency\n")
	fmt.Printf("====================\n\n")

	// Calculate column widths
	teamW := 4 // "Team"
	repoW := 4 // "Repo"
	wfW := 8   // "Workflow"
	for _, r := range results {
		if len(r.Team) > teamW {
			teamW = len(r.Team)
		}
		if len(r.Repo) > repoW {
			repoW = len(r.Repo)
		}
		if len(r.Workflow) > wfW {
			wfW = len(r.Workflow)
		}
	}

	if multiTeam {
		// Header with Team column
		fmt.Printf("| %-*s | %-*s | %-*s | %11s | %12s |\n",
			teamW, "Team", repoW, "Repo", wfW, "Workflow", "Deployments", "Deploys/Week")
		fmt.Printf("|%s|%s|%s|%s|%s|\n",
			strings.Repeat("-", teamW+2),
			strings.Repeat("-", repoW+2),
			strings.Repeat("-", wfW+2),
			strings.Repeat("-", 13),
			strings.Repeat("-", 14))

		for _, r := range results {
			if r.Error != "" {
				fmt.Printf("| %-*s | %-*s | %-*s | %11s | %12s |\n",
					teamW, r.Team, repoW, r.Repo, wfW, r.Workflow, r.Error, "")
			} else {
				fmt.Printf("| %-*s | %-*s | %-*s | %11d | %12.1f |\n",
					teamW, r.Team, repoW, r.Repo, wfW, r.Workflow, r.Deployments, r.DeploysWeek)
			}
		}
	} else {
		// Header without Team column
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
	}

	// Generate HTML chart with aggregate weekly deployments
	weeklyData := aggregateWeeklyDeployments(allRuns, from, to)
	weeklyFailures := aggregateWeeklyDeployments(failedRuns, from, to)
	if saveRawDataFlag {
		if err := saveDeploymentData(weeklyData, ""); err == nil {
			fmt.Printf("\nRaw data saved to: %s\n", savedGithubDataPath(""))
		}
	}
	if len(weeklyData.Periods) > 0 {
		cfg := charts.Config{}
		outputPath := getGithubOutputPath("deployment-frequency", "html")
		if err := charts.DeploymentFrequencyLine(weeklyData, weeklyFailures, cfg, outputPath); err != nil {
			return fmt.Errorf("failed to create chart: %w", err)
		}
		fmt.Printf("\nChart saved to %s\n", outputPath)
		openBrowser(outputPath)
	}

	return nil
}

// aggregateWeeklyDeployments buckets runs into 7-day periods anchored to `to`,
// working backwards — matching the same period logic as jira throughput.
func aggregateWeeklyDeployments(runs []gh.WorkflowRun, from, to time.Time) metrics.ThroughputResult {
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	stop := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)

	var periods []metrics.ThroughputPeriod
	for end.After(stop) {
		start := end.AddDate(0, 0, -7)
		if start.Before(stop) {
			start = stop
		}
		periods = append(periods, metrics.ThroughputPeriod{PeriodStart: start, PeriodEnd: end})
		end = start
	}

	// Reverse to chronological order.
	for i, j := 0, len(periods)-1; i < j; i, j = i+1, j-1 {
		periods[i], periods[j] = periods[j], periods[i]
	}

	// Count runs per period.
	for i := range periods {
		for _, run := range runs {
			t := run.CreatedAt
			if !t.Before(periods[i].PeriodStart) && t.Before(periods[i].PeriodEnd) {
				periods[i].Count++
			}
		}
	}

	result := metrics.ThroughputResult{Periods: periods, Frequency: metrics.FrequencyWeekly}
	for _, p := range periods {
		result.TotalCount += p.Count
	}
	if len(periods) > 0 {
		result.AvgCount = float64(result.TotalCount) / float64(len(periods))
	}
	return result
}

// fetchTeamDeploymentData fetches successful and failed workflow runs for the given team and aggregates by week.
// Returns empty results (not an error) if GitHub workflows are not configured for the team.
func fetchTeamDeploymentData(ctx context.Context, client *gh.Client, org, teamName string, from, to time.Time) (success metrics.ThroughputResult, failures metrics.ThroughputResult) {
	if useSavedDataFlag {
		result, err := loadDeploymentData(teamName)
		if err != nil {
			return metrics.ThroughputResult{}, metrics.ThroughputResult{}
		}
		return result, metrics.ThroughputResult{}
	}

	workflows, err := getConfiguredWorkflowsByTeam(teamName)
	if err != nil {
		return metrics.ThroughputResult{}, metrics.ThroughputResult{}
	}

	repos := make([]string, 0, len(workflows))
	for repo := range workflows {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	var allRuns []gh.WorkflowRun
	var failedRuns []gh.WorkflowRun
	for _, repo := range repos {
		wfFilenames := workflows[repo]
		allWfs, err := client.ListWorkflows(ctx, org, repo)
		if err != nil {
			continue
		}
		wfIndex := make(map[string]int64, len(allWfs))
		for _, wf := range allWfs {
			wfIndex[filepath.Base(wf.Path)] = wf.ID
		}
		for _, wfFilename := range wfFilenames {
			workflowID := wfIndex[wfFilename]
			if workflowID == 0 {
				continue
			}
			runs, err := client.ListWorkflowRuns(ctx, org, repo, workflowID, "", from, to)
			if err != nil {
				continue
			}
			for _, run := range runs {
				switch run.Conclusion {
				case "success":
					allRuns = append(allRuns, run)
				case "failure", "timed_out":
					failedRuns = append(failedRuns, run)
				}
			}
		}
	}
	result := aggregateWeeklyDeployments(allRuns, from, to)
	if saveRawDataFlag {
		_ = saveDeploymentData(result, teamName)
	}
	return result, aggregateWeeklyDeployments(failedRuns, from, to)
}

