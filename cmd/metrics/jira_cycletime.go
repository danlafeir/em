package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"em/internal/charts"
	"em/internal/jira"
	"em/internal/metrics"
	"em/internal/output"
	"em/internal/workflow"
)

var cycleTimeCmd = &cobra.Command{
	Use:   "cycle-time",
	Short: "Generate cycle time analysis",
	Long: `Analyze cycle time for completed issues.

Generates:
  - Statistical summary (mean, median, percentiles)
  - CSV export with per-issue details
  - Scatter plot showing cycle time over time (if HTML format)

Example:
  em metrics jira cycle-time --jql "project = MYPROJ" --from 2024-01-01
  em metrics jira cycle-time --jql "project = MYPROJ AND issuetype in (Story, Spike, Bug, Defect)" -o cycletime.csv`,
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
	fmt.Println("JIRA Metrics")
	fmt.Println(sectionDivider)
	fmt.Println()

	ctx := context.Background()

	var client *jira.Client
	if !useSavedDataFlag {
		var err error
		client, err = getJiraClient()
		if err != nil {
			return err
		}
		if err := client.TestConnection(ctx); err != nil {
			return fmt.Errorf("failed to connect to JIRA: %w", err)
		}
	}

	from, to, err := getDateRange()
	if err != nil {
		return err
	}

	return withTeamIteration(ctx, client, func(team, jql string) error {
		return generateCycleTime(ctx, client, team, jql, from, to)
	})
}

func generateCycleTime(ctx context.Context, client *jira.Client, team, jql string, from, to time.Time) error {
	var results []metrics.CycleTimeResult

	if useSavedDataFlag {
		fmt.Printf("Loading JIRA cycle time data from saved CSV (team: %q)...\n", team)
		ct, err := loadJiraCycleTimeData(team)
		if err != nil {
			return fmt.Errorf("use-saved-data: %w", err)
		}
		results = ct.all
	} else {
		// Add date filter to JQL, excluding Epics
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

		fmt.Printf("Found %d issues\n\n", len(histories))

		var outlierKeys map[string]bool
		results, _, outlierKeys = computeCycleTimeFromHistories(histories, mapper)
		if saveRawDataFlag {
			if err := saveJiraCycleTimeData(results, outlierKeys, team); err == nil {
				fmt.Printf("Raw data saved to: %s\n", savedJiraCycleTimePath(team))
			}
		}
	}

	if len(results) == 0 {
		fmt.Println("No completed issues found for cycle time calculation.")
		return nil
	}

	stats := metrics.CalculateStats(results)
	statsDays := stats.ToDays()

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

	outputName := teamOutputName("cycle-time", team)
	outputFormat := getOutputFormat("html")
	switch outputFormat {
	case "csv", "xlsx":
		outputPath := getOutputPath(outputName, "csv")
		if err := exportCycleTimeCSV(results, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("\nExported to %s\n", outputPath)
	case "html":
		cfg := charts.Config{}
		outputPath := getOutputPath(outputName, "html")
		if err := charts.CycleTimeScatter(results, nil, cfg, outputPath); err != nil {
			return fmt.Errorf("failed to generate chart: %w", err)
		}
		fmt.Printf("\nChart saved to %s\n", outputPath)
		charts.OpenBrowser(outputPath)
	}

	return nil
}

func exportCycleTimeCSV(results []metrics.CycleTimeResult, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Issue Key", "Type", "Summary", "Start Date", "End Date", "Cycle Time (days)"}
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
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// computeCycleTimeFromHistories calculates cycle times for all issues and splits
// them into all results, outlier-filtered results, and an outlier key set.
// The 2σ threshold is applied consistently across all callers.
func computeCycleTimeFromHistories(histories []workflow.IssueHistory, mapper *workflow.Mapper) (all, kept []metrics.CycleTimeResult, outlierKeys map[string]bool) {
	calculator := metrics.NewCycleTimeCalculator(mapper)
	all = calculator.Calculate(histories)
	kept, outliers := metrics.FilterCycleTimeOutliers(all, 2.0)
	outlierKeys = make(map[string]bool, len(outliers))
	for _, r := range outliers {
		outlierKeys[r.IssueKey] = true
	}
	return
}

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	days := d.Hours() / 24
	if days < 1 {
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
	return fmt.Sprintf("%.1f days", days)
}
