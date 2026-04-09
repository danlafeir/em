package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"em/internal/charts"
	"em/internal/jira"
	"em/internal/metrics"
	"em/internal/workflow"
)

var throughputCmd = &cobra.Command{
	Use:   "throughput",
	Short: "Generate throughput trend analysis",
	Long: `Analyze throughput (items completed per period) over time.

Required:
  em metrics jira config`,
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

	return withTeamIteration(func(team, jql string) error {
		return generateThroughput(ctx, client, team, jql, from, to)
	})
}

func generateThroughput(ctx context.Context, client *jira.Client, team, jql string, from, to time.Time) error {
	jqlWithDates := jqlWithDateRange(jql, from.Format("2006-01-02"), to.Format("2006-01-02"))

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

	fmt.Printf("Found %d issues\n\n", len(histories))

	frequency := parseThroughputFrequency(frequencyFlag)
	result := computeThroughputFromHistories(histories, mapper, frequency, from, to)

	if saveRawDataFlag {
		if err := saveJiraThroughputData(result, team); err == nil {
			fmt.Printf("Raw data saved to: %s\n", savedJiraThroughputPath(team))
		}
	}

	stats := metrics.CalculateThroughputStats(result)

	fmt.Printf("Throughput Analysis\n")
	fmt.Printf("===================\n")
	fmt.Printf("Date range: %s to %s\n", from.Format("2006-01-02"), to.Format("2006-01-02"))
	fmt.Printf("Frequency: %s\n\n", frequencyFlag)

	fmt.Printf("Statistics:\n")
	fmt.Printf("  Total periods:  %d\n", stats.Periods)
	fmt.Printf("  Total items:    %d\n", stats.TotalItems)
	fmt.Printf("  Avg items:      %.1f per %s\n", stats.AvgItems, frequencyFlag[:len(frequencyFlag)-2])
	fmt.Printf("  Min items:      %d\n", stats.MinItems)
	fmt.Printf("  Max items:      %d\n", stats.MaxItems)
	fmt.Printf("  Median items:   %d\n", stats.MedianItems)

	fmt.Printf("\nPeriod Breakdown:\n")
	fmt.Printf("%-12s  %6s\n", "Period", "Items")
	fmt.Printf("%-12s  %6s\n", "------", "-----")
	for _, p := range result.Periods {
		fmt.Printf("%-12s  %6d\n",
			p.PeriodStart.Format("2006-01-02"),
			p.Count)
	}

	outputName := teamOutputName("throughput", team)
	outputPath := getOutputPath(outputName, "html")
	cfg := charts.Config{}
	if err := charts.ThroughputLine(result, cfg, outputPath); err != nil {
		return fmt.Errorf("failed to generate chart: %w", err)
	}
	fmt.Printf("\nChart saved to %s\n", outputPath)
	openBrowser(outputPath)

	return nil
}

// computeThroughputFromHistories calculates throughput for the given histories,
// frequency, and date range. This is the shared primitive called by both the
// standalone throughput command and report generators.
func computeThroughputFromHistories(histories []workflow.IssueHistory, mapper *workflow.Mapper, frequency metrics.ThroughputFrequency, from, to time.Time) metrics.ThroughputResult {
	calculator := metrics.NewThroughputCalculator(frequency, mapper)
	return calculator.Calculate(histories, from, to)
}

// parseThroughputFrequency maps the --frequency flag value to a ThroughputFrequency constant.
func parseThroughputFrequency(flag string) metrics.ThroughputFrequency {
	switch flag {
	case "daily":
		return metrics.FrequencyDaily
	case "biweekly":
		return metrics.FrequencyBiweekly
	case "monthly":
		return metrics.FrequencyMonthly
	default:
		return metrics.FrequencyWeekly
	}
}

