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

var burnupCmd = &cobra.Command{
	Use:   "burnup",
	Short: "Generate burn-up chart with forecast",
	Long: `Generate burn-up chart data showing completed work against total scope,
with Monte Carlo forecast bands for projected completion.

Shows:
  - Historical completion progress
  - Total scope over time
  - Monte Carlo forecast bands (50%, 85%, 95% confidence)

Example:
  devctl-em metrics jira burnup --jql "project = MYPROJ" --scope 100
  devctl-em metrics jira burnup --epic MYPROJ-123`,
	RunE: runBurnup,
}

var (
	scopeFlag int
)

func init() {
	JiraCmd.AddCommand(burnupCmd)

	// Burnup-specific flags
	burnupCmd.Flags().StringVar(&epicFlag, "epic", "", "Epic key (auto-determines scope)")
	burnupCmd.Flags().IntVar(&scopeFlag, "scope", 0, "Total scope (number of items)")
}

func runBurnup(cmd *cobra.Command, args []string) error {
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

	var totalScope int
	var epicName string

	// If epic is specified, use it to determine scope
	if epicFlag != "" {
		epicName = epicFlag
		jql = fmt.Sprintf("\"Epic Link\" = %s OR parent = %s", epicFlag, epicFlag)

		// First get total scope (all issues in epic)
		allIssues, err := client.SearchAllIssues(ctx, jql, "status,summary,issuetype,created,resolutiondate", "")
		if err != nil {
			return fmt.Errorf("failed to fetch epic issues: %w", err)
		}
		totalScope = len(allIssues)
		fmt.Printf("Epic %s: %d total items\n", epicFlag, totalScope)
	} else if scopeFlag > 0 {
		totalScope = scopeFlag
	} else {
		return fmt.Errorf("either --epic or --scope must be specified")
	}

	// Fetch all issues in scope
	jqlWithDates := fmt.Sprintf("(%s) AND created <= %s", jql, to.Format("2006-01-02"))

	fmt.Printf("Fetching issues from JIRA...\n")

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

	// Calculate daily burnup data
	burnupData := calculateBurnup(histories, from, to)

	// Calculate remaining items
	completedCount := 0
	for _, h := range histories {
		if h.Completed != nil {
			completedCount++
		}
	}
	remainingItems := totalScope - completedCount

	// Calculate weekly throughput for forecast
	throughputCalc := metrics.NewThroughputCalculator(metrics.FrequencyWeekly)
	throughputResult := throughputCalc.Calculate(histories, from.AddDate(0, 0, -60), to)
	weeklyThroughput := metrics.GetWeeklyThroughputValues(throughputResult)

	// Print summary
	fmt.Printf("Burn-up Analysis\n")
	fmt.Printf("================\n")
	if epicName != "" {
		fmt.Printf("Epic: %s\n", epicName)
	}
	fmt.Printf("Date range: %s to %s\n", from.Format("2006-01-02"), to.Format("2006-01-02"))
	fmt.Printf("Total scope: %d items\n", totalScope)
	fmt.Printf("Completed: %d items (%.1f%%)\n", completedCount, float64(completedCount)/float64(totalScope)*100)
	fmt.Printf("Remaining: %d items\n\n", remainingItems)

	// Show burnup trend
	fmt.Printf("Progress Over Time:\n")
	fmt.Printf("%-12s  %10s  %10s  %10s\n", "Date", "Completed", "Scope", "Progress")
	fmt.Printf("%-12s  %10s  %10s  %10s\n", "----", "---------", "-----", "--------")

	// Show weekly snapshots
	for i, dp := range burnupData {
		if i%7 == 0 || i == len(burnupData)-1 {
			progress := float64(dp.Completed) / float64(dp.TotalScope) * 100
			fmt.Printf("%-12s  %10d  %10d  %9.1f%%\n",
				dp.Date.Format("2006-01-02"),
				dp.Completed,
				dp.TotalScope,
				progress)
		}
	}

	// Run Monte Carlo forecast if there are remaining items
	if remainingItems > 0 && len(weeklyThroughput) > 0 {
		fmt.Printf("\nMonte Carlo Forecast:\n")

		config := metrics.MonteCarloConfig{
			Trials:          10000,
			SimulationStart: time.Now(),
		}

		simulator := metrics.NewMonteCarloSimulator(config, weeklyThroughput)
		forecast, err := simulator.Run(remainingItems)
		if err != nil {
			fmt.Printf("  Could not generate forecast: %v\n", err)
		} else {
			for _, p := range []int{50, 85, 95} {
				date := forecast.Percentiles[p]
				days := forecast.PercentileDays[p]
				fmt.Printf("  %d%% confidence: %s (%d days)\n", p, date.Format("2006-01-02"), days)
			}
		}
	}

	// Export to CSV
	outputPath := getOutputPath("burnup", "csv")
	if err := exportBurnupCSV(burnupData, outputPath); err != nil {
		return fmt.Errorf("failed to export CSV: %w", err)
	}
	fmt.Printf("\nExported to %s\n", outputPath)

	return nil
}

// BurnupDataPoint represents a single point in the burnup chart.
type BurnupDataPoint struct {
	Date       time.Time
	Completed  int
	TotalScope int
}

func calculateBurnup(histories []workflow.IssueHistory, from, to time.Time) []BurnupDataPoint {
	var data []BurnupDataPoint

	current := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	endDate := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, to.Location())

	for !current.After(endDate) {
		completed := 0
		scope := 0

		for _, h := range histories {
			// Count if created by this date
			if !h.Created.After(current) {
				scope++

				// Count if completed by this date
				if h.Completed != nil && !h.Completed.After(current) {
					completed++
				}
			}
		}

		data = append(data, BurnupDataPoint{
			Date:       current,
			Completed:  completed,
			TotalScope: scope,
		})

		current = current.AddDate(0, 0, 1)
	}

	return data
}

func exportBurnupCSV(data []BurnupDataPoint, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Date", "Completed", "Total Scope", "Remaining", "Progress %"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, dp := range data {
		remaining := dp.TotalScope - dp.Completed
		progress := 0.0
		if dp.TotalScope > 0 {
			progress = float64(dp.Completed) / float64(dp.TotalScope) * 100
		}

		row := []string{
			dp.Date.Format("2006-01-02"),
			strconv.Itoa(dp.Completed),
			strconv.Itoa(dp.TotalScope),
			strconv.Itoa(remaining),
			strconv.FormatFloat(progress, 'f', 1, 64),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
