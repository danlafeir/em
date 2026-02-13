package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"devctl-em/internal/output"
	"devctl-em/pkg/metrics"
	"devctl-em/pkg/workflow"
)

var throughputCmd = &cobra.Command{
	Use:   "throughput",
	Short: "Generate throughput trend analysis",
	Long: `Analyze throughput (items completed per period) over time.

Generates:
  - Weekly/monthly throughput counts
  - Statistical summary (average, min, max)
  - CSV export with period-by-period data

Example:
  devctl-em metrics jira throughput --jql "project = MYPROJ" --from 2024-01-01
  devctl-em metrics jira throughput --frequency weekly -o throughput.csv`,
	RunE: runThroughput,
}

var (
	frequencyFlag string
)

func init() {
	JiraCmd.AddCommand(throughputCmd)

	// Throughput-specific flags
	throughputCmd.Flags().StringVar(&frequencyFlag, "frequency", "weekly", "Aggregation frequency: daily, weekly, biweekly, monthly")
}

func runThroughput(cmd *cobra.Command, args []string) error {
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

	// Parse frequency
	var frequency metrics.ThroughputFrequency
	switch frequencyFlag {
	case "daily":
		frequency = metrics.FrequencyDaily
	case "weekly":
		frequency = metrics.FrequencyWeekly
	case "biweekly":
		frequency = metrics.FrequencyBiweekly
	case "monthly":
		frequency = metrics.FrequencyMonthly
	default:
		frequency = metrics.FrequencyWeekly
	}

	// Calculate throughput
	calculator := metrics.NewThroughputCalculator(frequency)
	result := calculator.Calculate(histories, from, to)

	// Calculate statistics
	stats := metrics.CalculateThroughputStats(result)

	// Print summary
	fmt.Printf("Throughput Analysis\n")
	fmt.Printf("===================\n")
	fmt.Printf("Date range: %s to %s\n", from.Format("2006-01-02"), to.Format("2006-01-02"))
	fmt.Printf("Frequency: %s\n\n", frequencyFlag)

	fmt.Printf("Statistics:\n")
	fmt.Printf("  Total periods:  %d\n", stats.Periods)
	fmt.Printf("  Total items:    %d\n", stats.TotalItems)
	fmt.Printf("  Total points:   %.1f\n", stats.TotalPoints)
	fmt.Printf("  Avg items:      %.1f per %s\n", stats.AvgItems, frequencyFlag[:len(frequencyFlag)-2])
	fmt.Printf("  Avg points:     %.1f per %s\n", stats.AvgPoints, frequencyFlag[:len(frequencyFlag)-2])
	fmt.Printf("  Min items:      %d\n", stats.MinItems)
	fmt.Printf("  Max items:      %d\n", stats.MaxItems)
	fmt.Printf("  Median items:   %d\n", stats.MedianItems)

	// Print period breakdown
	fmt.Printf("\nPeriod Breakdown:\n")
	fmt.Printf("%-12s  %6s  %8s\n", "Period", "Items", "Points")
	fmt.Printf("%-12s  %6s  %8s\n", "------", "-----", "------")
	for _, p := range result.Periods {
		fmt.Printf("%-12s  %6d  %8.1f\n",
			p.PeriodStart.Format("2006-01-02"),
			p.Count,
			p.Points)
	}

	// Export to CSV
	outputFormat := getOutputFormat("csv")
	if outputFormat == "csv" || outputFormat == "xlsx" {
		outputPath := getOutputPath("throughput", "csv")
		if err := exportThroughputCSV(result, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("\nExported to %s\n", outputPath)
	}

	return nil
}

func exportThroughputCSV(result metrics.ThroughputResult, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Period Start", "Period End", "Items Completed", "Story Points", "Issue Keys"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, p := range result.Periods {
		issueKeys := ""
		for i, key := range p.IssueKeys {
			if i > 0 {
				issueKeys += ", "
			}
			issueKeys += key
		}

		row := []string{
			p.PeriodStart.Format("2006-01-02"),
			p.PeriodEnd.Format("2006-01-02"),
			strconv.Itoa(p.Count),
			strconv.FormatFloat(p.Points, 'f', 1, 64),
			issueKeys,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
