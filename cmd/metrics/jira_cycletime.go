package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/pkg/metrics"
	"devctl-em/pkg/workflow"
)

var cycleTimeCmd = &cobra.Command{
	Use:   "cycle-time",
	Short: "Generate cycle time analysis",
	Long: `Analyze cycle time for completed issues.

Generates:
  - Statistical summary (mean, median, percentiles)
  - CSV export with per-issue details
  - Scatter plot showing cycle time over time (if PNG format)

Example:
  devctl-em metrics jira cycle-time --jql "project = MYPROJ" --from 2024-01-01
  devctl-em metrics jira cycle-time --jql "project = MYPROJ AND issuetype = Story" -o cycletime.csv`,
	RunE: runCycleTime,
}

var (
	cycleTimePercentiles []int
)

func init() {
	JiraCmd.AddCommand(cycleTimeCmd)

	// Cycle-time specific flags
	cycleTimeCmd.Flags().IntSlice("percentiles", []int{50, 70, 85, 95}, "Percentiles to calculate")
}

func runCycleTime(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get JIRA client
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	// Test connection
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to JIRA: %w", err)
	}

	// Get JQL and date range
	jql, err := getJQL()
	if err != nil {
		return err
	}

	from, to, err := getDateRange()
	if err != nil {
		return err
	}

	// Add date filter to JQL
	jqlWithDates := fmt.Sprintf("(%s) AND resolved >= %s AND resolved <= %s",
		jql, from.Format("2006-01-02"), to.Format("2006-01-02"))

	fmt.Printf("Fetching issues from JIRA...\n")
	fmt.Printf("JQL: %s\n", jqlWithDates)

	// Fetch issues with history
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

	fmt.Printf("Found %d issues\n\n", len(issues))

	// Get workflow mapper
	mapper := getWorkflowMapper()

	// Map issues to workflow history
	histories := make([]workflow.IssueHistory, len(issues))
	for i, issue := range issues {
		histories[i] = mapper.MapIssueHistory(issue)
	}

	// Calculate cycle times
	calculator := metrics.NewCycleTimeCalculator(mapper)
	results := calculator.Calculate(histories)

	if len(results) == 0 {
		fmt.Println("No completed issues found for cycle time calculation.")
		return nil
	}

	// Calculate statistics
	stats := metrics.CalculateStats(results)
	statsDays := stats.ToDays()

	// Print summary
	fmt.Printf("Cycle Time Analysis\n")
	fmt.Printf("===================\n")
	fmt.Printf("Issues analyzed: %d\n", stats.Count)
	fmt.Printf("Date range: %s to %s\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	fmt.Printf("Statistics (in days):\n")
	fmt.Printf("  Mean:    %.1f\n", statsDays.Mean)
	fmt.Printf("  Median:  %.1f\n", statsDays.Median)
	fmt.Printf("  50th %%:  %.1f\n", statsDays.Percentile50)
	fmt.Printf("  70th %%:  %.1f\n", statsDays.Percentile70)
	fmt.Printf("  85th %%:  %.1f\n", statsDays.Percentile85)
	fmt.Printf("  95th %%:  %.1f\n", statsDays.Percentile95)
	fmt.Printf("  Min:     %.1f\n", statsDays.Min)
	fmt.Printf("  Max:     %.1f\n", statsDays.Max)
	fmt.Printf("  Std Dev: %.1f\n", statsDays.StdDev)

	// Export to CSV
	outputFormat := getOutputFormat("csv")
	if outputFormat == "csv" || outputFormat == "xlsx" {
		outputPath := getOutputPath("cycle-time", "csv")
		if err := exportCycleTimeCSV(results, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("\nExported to %s\n", outputPath)
	}

	return nil
}

func exportCycleTimeCSV(results []metrics.CycleTimeResult, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Issue Key", "Type", "Summary", "Start Date", "End Date", "Cycle Time (days)", "Story Points"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, r := range results {
		row := []string{
			r.IssueKey,
			r.IssueType,
			r.Summary,
			r.StartDate.Format("2006-01-02"),
			r.EndDate.Format("2006-01-02"),
			strconv.FormatFloat(r.CycleTimeDays(), 'f', 1, 64),
			strconv.FormatFloat(r.StoryPoints, 'f', 1, 64),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	days := d.Hours() / 24
	if days < 1 {
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
	return fmt.Sprintf("%.1f days", days)
}
