package metrics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"em/internal/charts"
	"em/internal/jira"
	"em/internal/metrics"
)

const defaultLongestCTLimit = 10
const reportLongestCTLimit = 5

var longestCycleTimeCmd = &cobra.Command{
	Use:   "longest-cycle-time",
	Short: "List issues with the longest cycle times",
	Long: `Show the top 10 issues with the longest cycle times in the last 6 weeks.

Example:
  em metrics jira longest-cycle-time
  em metrics jira longest-cycle-time --from 2024-01-01`,
	RunE: runLongestCycleTime,
}

func init() {
	JiraCmd.AddCommand(longestCycleTimeCmd)
}

func runLongestCycleTime(cmd *cobra.Command, args []string) error {
	fmt.Println("JIRA Metrics")
	fmt.Println(sectionDivider)
	fmt.Println()

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
	jqlWithDates := jqlWithDateRange(
		fmt.Sprintf("(%s) AND issuetype in (Story, Spike, Bug, Defect)", jql),
		from.Format("2006-01-02"), to.Format("2006-01-02"),
	)

	fmt.Printf("Fetching issues from JIRA...\n")
	fmt.Printf("JQL: %s\n", jqlWithDates)

	histories, mapper, err := fetchAndMapIssues(ctx, client, jqlWithDates)
	if err != nil {
		return fmt.Errorf("failed to fetch issues: %w", err)
	}

	if len(histories) == 0 {
		fmt.Println("No issues found matching the query.")
		return nil
	}

	all, kept, _ := computeCycleTimeFromHistories(histories, mapper)

	if len(all) == 0 {
		fmt.Println("No completed issues found for cycle time calculation.")
		return nil
	}

	if outlierCount := len(all) - len(kept); outlierCount > 0 {
		fmt.Printf("Removed %d outlier(s) (beyond 2σ from mean)\n", outlierCount)
	}

	rows := buildLongestCTRows(kept, nil, defaultLongestCTLimit)

	fmt.Printf("\nTop %d Longest Cycle Times\n", len(rows))
	fmt.Printf("=========================\n\n")

	titleWidth := 50
	fmt.Printf("| %-16s | %-*s | %-10s | %-10s | %-10s |\n",
		"Key", titleWidth, "Title", "Cycle Time", "Started", "Completed")
	fmt.Printf("|%s|%s|%s|%s|%s|\n",
		strings.Repeat("_", 18),
		strings.Repeat("_", titleWidth+2),
		strings.Repeat("_", 12),
		strings.Repeat("_", 12),
		strings.Repeat("_", 12))

	for _, r := range rows {
		lines := wrapString(r.Summary, titleWidth)
		for l, line := range lines {
			if l == 0 {
				fmt.Printf("| %-16s | %-*s | %-10s | %-10s | %-10s |\n",
					r.Key, titleWidth, line, r.Days+" d", r.Started, r.Completed)
			} else {
				fmt.Printf("| %-16s | %-*s | %-10s | %-10s | %-10s |\n",
					"", titleWidth, line, "", "", "")
			}
		}
	}

	outputName := teamOutputName("longest-cycle-time", team)
	outputFormat := getOutputFormat("html")
	if outputFormat == "html" {
		title := fmt.Sprintf("Longest Cycle Times — %s to %s", from.Format("Jan 02"), to.Format("Jan 02"))
		outputPath := getOutputPath(outputName, "html")
		if err := charts.LongestCycleTimeTable(rows, title, client.BaseURL(), outputPath); err != nil {
			return fmt.Errorf("failed to save chart: %w", err)
		}
		fmt.Printf("\nChart saved to %s\n", outputPath)
		charts.OpenBrowser(outputPath)
	}

	return nil
}

// buildLongestCTRows sorts results by cycle time descending, drops epics, and
// returns up to limit rows as chart-ready structs. Pass a non-nil outlierKeys
// map to mark outliers visually (they are still included). Pass nil to omit marking.
func buildLongestCTRows(results []metrics.CycleTimeResult, outlierKeys map[string]bool, limit int) []charts.LongestCycleTimeRow {
	filtered := make([]metrics.CycleTimeResult, 0, len(results))
	for _, r := range results {
		if r.IssueType != "Epic" && r.CycleTimeDays() > 0 {
			filtered = append(filtered, r)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CycleTime > filtered[j].CycleTime
	})
	n := min(len(filtered), limit)
	rows := make([]charts.LongestCycleTimeRow, 0, n)
	for _, r := range filtered[:n] {
		rows = append(rows, charts.LongestCycleTimeRow{
			Key:       r.IssueKey,
			Summary:   r.Summary,
			Days:      fmt.Sprintf("%.1f", r.CycleTimeDays()),
			Started:   r.StartDate.Format("Jan 02"),
			Completed: r.EndDate.Format("Jan 02"),
			Outlier:   outlierKeys[r.IssueKey],
		})
	}
	return rows
}
