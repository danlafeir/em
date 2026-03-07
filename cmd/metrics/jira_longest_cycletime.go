package metrics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/internal/charts"
	"devctl-em/internal/jira"
	"devctl-em/internal/metrics"
	"devctl-em/internal/workflow"
)

var longestCycleTimeCmd = &cobra.Command{
	Use:   "longest-cycle-time",
	Short: "List issues with the longest cycle times",
	Long: `Show the top 10 issues with the longest cycle times in the last 6 weeks.

Example:
  devctl-em metrics jira longest-cycle-time
  devctl-em metrics jira longest-cycle-time --from 2024-01-01`,
	RunE: runLongestCycleTime,
}

func init() {
	JiraCmd.AddCommand(longestCycleTimeCmd)
}

func runLongestCycleTime(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getJiraClient()
	if err != nil {
		return err
	}

	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to JIRA: %w", err)
	}

	from, to, err := getDateRange()
	if err != nil {
		return err
	}

	return withTeamIteration(ctx, client, func(team, jql string) error {
		return generateLongestCycleTime(ctx, client, team, jql, from, to)
	})
}

func generateLongestCycleTime(ctx context.Context, client *jira.Client, team, jql string, from, to time.Time) error {
	jqlWithDates := jqlWithDateRange(jql, from.Format("2006-01-02"), to.Format("2006-01-02"))

	fmt.Printf("Fetching issues from JIRA...\n")
	fmt.Printf("JQL: %s\n", jqlWithDates)

	issues, err := client.FetchIssuesWithHistory(ctx, jqlWithDates, func(current, total int) {
		fmt.Printf("\rProcessing issue %d/%d...", current, total)
	})
	if err != nil {
		return fmt.Errorf("failed to fetch issues: %w", err)
	}
	fmt.Println()

	if len(issues) == 0 {
		fmt.Println("No issues found matching the query.")
		return nil
	}

	mapper := getWorkflowMapper()

	histories := make([]workflow.IssueHistory, len(issues))
	for i, issue := range issues {
		histories[i] = mapper.MapIssueHistory(issue)
	}

	calculator := metrics.NewCycleTimeCalculator(mapper)
	results := calculator.Calculate(histories)

	if len(results) == 0 {
		fmt.Println("No completed issues found for cycle time calculation.")
		return nil
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CycleTime > results[j].CycleTime
	})

	limit := 10
	if len(results) < limit {
		limit = len(results)
	}
	top := results[:limit]

	fmt.Printf("\nTop %d Longest Cycle Times\n", limit)
	fmt.Printf("=========================\n\n")

	titleWidth := 50
	fmt.Printf("| %-16s | %-*s | %-10s | %-10s | %-10s |\n",
		"Epic", titleWidth, "Title", "Cycle Time", "Started", "Completed")
	fmt.Printf("|%s|%s|%s|%s|%s|\n",
		strings.Repeat("_", 18),
		strings.Repeat("_", titleWidth+2),
		strings.Repeat("_", 12),
		strings.Repeat("_", 12),
		strings.Repeat("_", 12))

	for _, r := range top {
		lines := wrapString(r.Summary, titleWidth)
		for l, line := range lines {
			if l == 0 {
				fmt.Printf("| %-16s | %-*s | %-10s | %-10s | %-10s |\n",
					r.IssueKey,
					titleWidth, line,
					fmt.Sprintf("%.1f d", r.CycleTimeDays()),
					r.StartDate.Format("Jan 02"),
					r.EndDate.Format("Jan 02"))
			} else {
				fmt.Printf("| %-16s | %-*s | %-10s | %-10s | %-10s |\n",
					"", titleWidth, line, "", "", "")
			}
		}
	}

	outputName := teamOutputName("longest-cycle-time", team)
	outputFormat := getOutputFormat("html")
	if outputFormat == "html" {
		var rows []charts.LongestCycleTimeRow
		for _, r := range top {
			rows = append(rows, charts.LongestCycleTimeRow{
				Key:       r.IssueKey,
				Summary:   r.Summary,
				Days:      fmt.Sprintf("%.1f", r.CycleTimeDays()),
				Started:   r.StartDate.Format("Jan 02"),
				Completed: r.EndDate.Format("Jan 02"),
			})
		}
		title := fmt.Sprintf("Longest Cycle Times — %s to %s", from.Format("Jan 02"), to.Format("Jan 02"))
		outputPath := getOutputPath(outputName, "html")
		if err := charts.LongestCycleTimeTable(rows, title, outputPath); err != nil {
			return fmt.Errorf("failed to save chart: %w", err)
		}
		fmt.Printf("\nChart saved to %s\n", outputPath)
		charts.OpenBrowser(outputPath)
	}

	return nil
}
