package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/internal/output"
	"devctl-em/internal/jira"
	"devctl-em/internal/metrics"
	"devctl-em/internal/workflow"
)

var forecastCmd = &cobra.Command{
	Use:   "forecast",
	Short: "Monte Carlo completion forecast",
	Long: `Run Monte Carlo simulation to forecast completion dates.

Uses historical throughput data to simulate possible completion dates
and calculate probability distributions.

When run without flags, forecasts all open epics in your configured projects.

Example:
  devctl-em metrics jira forecast                                    # All open epics
  devctl-em metrics jira forecast --epic MYPROJ-123                  # Single epic
  devctl-em metrics jira forecast --jql "project = MYPROJ" --remaining 25
  devctl-em metrics jira forecast --deadline 2024-12-31              # Check deadline`,
	RunE: runForecast,
}

var (
	epicFlag        string
	remainingFlag   int
	deadlineFlag    string
	trialsFlag      int
	historyDaysFlag int
	allEpicsFlag    bool
)

func init() {
	JiraCmd.AddCommand(forecastCmd)

	// Forecast-specific flags
	forecastCmd.Flags().StringVar(&epicFlag, "epic", "", "Epic key to forecast (fetches remaining items automatically)")
	forecastCmd.Flags().IntVar(&remainingFlag, "remaining", 0, "Number of remaining items to complete")
	forecastCmd.Flags().StringVar(&deadlineFlag, "deadline", "", "Target deadline date (YYYY-MM-DD)")
	forecastCmd.Flags().IntVar(&trialsFlag, "trials", 10000, "Number of Monte Carlo simulations")
	forecastCmd.Flags().IntVar(&historyDaysFlag, "history-days", 60, "Days of historical throughput to sample from")
	forecastCmd.Flags().BoolVar(&allEpicsFlag, "all", false, "Forecast all open epics (default when no other flags)")
}

// EpicForecast holds forecast results for a single epic.
type EpicForecast struct {
	EpicKey       string
	EpicSummary   string
	TotalItems    int
	RemainingItems int
	CompletedItems int
	Progress      float64
	Forecast50    time.Time
	Forecast85    time.Time
	Forecast95    time.Time
	Days85        int
	Error         string
}

func runForecast(cmd *cobra.Command, args []string) error {
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

	// Determine mode: all epics, single epic, or remaining count
	if epicFlag == "" && remainingFlag == 0 {
		// Default: forecast all open epics
		return runAllEpicsForecast(ctx, client)
	} else if epicFlag != "" {
		// Single epic forecast
		return runSingleEpicForecast(ctx, client, epicFlag)
	} else {
		// Manual remaining count
		return runManualForecast(ctx, client, remainingFlag)
	}
}

func runAllEpicsForecast(ctx context.Context, client *jira.Client) error {
	fmt.Println("Discovering open epics...")

	// Get project-level JQL for epic discovery (not children JQL)
	baseJQL, err := getProjectJQL()
	if err != nil {
		return err
	}

	// Find all open epics
	epicJQL := fmt.Sprintf("(%s) AND issuetype = Epic AND resolution IS EMPTY ORDER BY key", baseJQL)
	fmt.Printf("JQL: %s\n\n", epicJQL)

	epics, err := client.SearchAllIssues(ctx, epicJQL, "summary,status", "")
	if err != nil {
		return fmt.Errorf("failed to fetch epics: %w", err)
	}

	if len(epics) == 0 {
		fmt.Println("No open epics found.")
		return nil
	}

	fmt.Printf("Found %d open epics\n\n", len(epics))

	// Get historical throughput data (shared across all forecasts)
	// Use resolveJQL for throughput to scope to active epic children
	throughputBaseJQL, err := resolveJQL(ctx, client)
	if err != nil {
		return err
	}

	historyEnd := time.Now()
	historyStart := historyEnd.AddDate(0, 0, -historyDaysFlag)

	throughputJQL := fmt.Sprintf("(%s) AND resolved >= %s AND resolved <= %s",
		throughputBaseJQL, historyStart.Format("2006-01-02"), historyEnd.Format("2006-01-02"))

	fmt.Println("Fetching historical throughput data...")
	completedIssues, err := client.FetchIssuesWithHistory(ctx, throughputJQL, func(current, total int) {
		fmt.Printf("\rProcessing: %d/%d issues...", current, total)
	})
	if err != nil {
		return fmt.Errorf("failed to fetch throughput data: %w", err)
	}
	fmt.Println()

	if len(completedIssues) == 0 {
		return fmt.Errorf("no historical throughput data found - cannot forecast")
	}

	// Calculate weekly throughput
	mapper := getWorkflowMapper()
	histories := make([]workflow.IssueHistory, len(completedIssues))
	for i, issue := range completedIssues {
		histories[i] = mapper.MapIssueHistory(issue)
	}

	throughputCalc := metrics.NewThroughputCalculator(metrics.FrequencyWeekly)
	throughputResult := throughputCalc.Calculate(histories, historyStart, historyEnd)
	weeklyThroughput := metrics.GetWeeklyThroughputValues(throughputResult)

	if len(weeklyThroughput) == 0 {
		return fmt.Errorf("no throughput data available for simulation")
	}

	avgThroughput := float64(sum(weeklyThroughput)) / float64(len(weeklyThroughput))
	fmt.Printf("\nHistorical throughput: %.1f items/week (from %d weeks)\n\n", avgThroughput, len(weeklyThroughput))

	// Forecast each epic
	var forecasts []EpicForecast

	fmt.Println("Forecasting epics...")
	for i, epic := range epics {
		fmt.Printf("\r[%d/%d] %s...", i+1, len(epics), epic.Key)

		forecast := forecastEpic(ctx, client, mapper, epic, weeklyThroughput)
		forecasts = append(forecasts, forecast)
	}
	fmt.Println()

	// Sort by 85th percentile completion date
	sort.Slice(forecasts, func(i, j int) bool {
		// Put errors at the end
		if forecasts[i].Error != "" && forecasts[j].Error == "" {
			return false
		}
		if forecasts[i].Error == "" && forecasts[j].Error != "" {
			return true
		}
		// Put completed epics at the beginning
		if forecasts[i].RemainingItems == 0 && forecasts[j].RemainingItems > 0 {
			return true
		}
		if forecasts[i].RemainingItems > 0 && forecasts[j].RemainingItems == 0 {
			return false
		}
		return forecasts[i].Forecast85.Before(forecasts[j].Forecast85)
	})

	// Print summary table
	fmt.Printf("\n")
	fmt.Printf("Epic Forecast Summary\n")
	fmt.Printf("=====================\n\n")

	// Header
	fmt.Printf("%-12s  %-40s  %5s  %5s  %6s  %-12s  %-12s  %-12s\n",
		"Epic", "Summary", "Done", "Left", "Prog%", "50%", "85%", "95%")
	fmt.Printf("%-12s  %-40s  %5s  %5s  %6s  %-12s  %-12s  %-12s\n",
		"----", "-------", "----", "----", "-----", "---", "---", "---")

	for _, f := range forecasts {
		summary := truncate(f.EpicSummary, 40)

		if f.Error != "" {
			fmt.Printf("%-12s  %-40s  %5s  %5s  %6s  %s\n",
				f.EpicKey, summary, "-", "-", "-", f.Error)
			continue
		}

		if f.RemainingItems == 0 {
			fmt.Printf("%-12s  %-40s  %5d  %5d  %5.0f%%  %-12s\n",
				f.EpicKey, summary, f.CompletedItems, f.RemainingItems, f.Progress, "COMPLETE")
			continue
		}

		fmt.Printf("%-12s  %-40s  %5d  %5d  %5.0f%%  %-12s  %-12s  %-12s\n",
			f.EpicKey, summary,
			f.CompletedItems, f.RemainingItems, f.Progress,
			f.Forecast50.Format("Jan 02"),
			f.Forecast85.Format("Jan 02"),
			f.Forecast95.Format("Jan 02"))
	}

	// Check deadline if provided
	if deadlineFlag != "" {
		deadline, err := time.Parse("2006-01-02", deadlineFlag)
		if err == nil {
			fmt.Printf("\n\nDeadline Analysis: %s\n", deadline.Format("January 2, 2006"))
			fmt.Printf("------------------\n")

			atRisk := 0
			for _, f := range forecasts {
				if f.Error != "" || f.RemainingItems == 0 {
					continue
				}
				if f.Forecast85.After(deadline) {
					atRisk++
					fmt.Printf("  ⚠  %s: 85%% forecast is %s (deadline miss)\n",
						f.EpicKey, f.Forecast85.Format("Jan 02"))
				}
			}

			if atRisk == 0 {
				fmt.Printf("  ✓ All epics forecast to complete before deadline at 85%% confidence\n")
			}
		}
	}

	// Aggregate forecast across all active epics
	totalRemaining := 0
	for _, f := range forecasts {
		if f.Error == "" && f.RemainingItems > 0 {
			totalRemaining += f.RemainingItems
		}
	}

	if totalRemaining > 0 {
		config := metrics.MonteCarloConfig{
			Trials:          trialsFlag,
			SimulationStart: time.Now(),
		}

		if deadlineFlag != "" {
			if deadline, err := time.Parse("2006-01-02", deadlineFlag); err == nil {
				config.Deadline = &deadline
			}
		}

		simulator := metrics.NewMonteCarloSimulator(config, weeklyThroughput)
		aggResult, err := simulator.Run(totalRemaining)
		if err == nil {
			fmt.Printf("\n\nAggregate Forecast (all active epics)\n")
			fmt.Printf("=====================================\n")
			fmt.Printf("Total remaining items: %d\n", totalRemaining)
			fmt.Printf("Average throughput:    %.1f items/week\n\n", aggResult.AvgThroughput)
			fmt.Printf("  %-20s  %-12s  %s\n", "Confidence", "Date", "Days")
			fmt.Printf("  %-20s  %-12s  %s\n", "----------", "----", "----")
			for _, p := range []int{50, 70, 85, 95} {
				fmt.Printf("  %-20s  %-12s  %d\n",
					fmt.Sprintf("%d%%", p),
					aggResult.Percentiles[p].Format("Jan 02, 2006"),
					aggResult.PercentileDays[p])
			}

			if aggResult.DeadlineDate != nil {
				fmt.Printf("\n  Deadline: %s\n", aggResult.DeadlineDate.Format("January 2, 2006"))
				fmt.Printf("  Probability of meeting deadline: %.1f%%\n", aggResult.DeadlineConfidence*100)
			}
		}
	}

	// Export to CSV
	outputPath := getOutputPath("epic-forecasts", "csv")
	if err := exportForecastsCSV(forecasts, outputPath); err == nil {
		fmt.Printf("\nExported to %s\n", outputPath)
	}

	return nil
}

func forecastEpic(ctx context.Context, client *jira.Client, mapper *workflow.Mapper, epic jira.Issue, weeklyThroughput []int) EpicForecast {
	forecast := EpicForecast{
		EpicKey:     epic.Key,
		EpicSummary: epic.Fields.Summary,
	}

	// Get issues in this epic
	jql := fmt.Sprintf("\"Epic Link\" = %s OR parent = %s", epic.Key, epic.Key)
	issues, err := client.SearchAllIssues(ctx, jql, "status,summary,issuetype", "")
	if err != nil {
		forecast.Error = "fetch failed"
		return forecast
	}

	forecast.TotalItems = len(issues)

	// Count remaining
	for _, issue := range issues {
		if mapper.IsCompleted(issue.Fields.Status.Name) {
			forecast.CompletedItems++
		} else {
			forecast.RemainingItems++
		}
	}

	if forecast.TotalItems > 0 {
		forecast.Progress = float64(forecast.CompletedItems) / float64(forecast.TotalItems) * 100
	}

	if forecast.RemainingItems == 0 {
		return forecast // Already complete
	}

	// Run Monte Carlo
	config := metrics.MonteCarloConfig{
		Trials:          trialsFlag,
		SimulationStart: time.Now(),
	}

	simulator := metrics.NewMonteCarloSimulator(config, weeklyThroughput)
	result, err := simulator.Run(forecast.RemainingItems)
	if err != nil {
		forecast.Error = "sim failed"
		return forecast
	}

	forecast.Forecast50 = result.Percentiles[50]
	forecast.Forecast85 = result.Percentiles[85]
	forecast.Forecast95 = result.Percentiles[95]
	forecast.Days85 = result.PercentileDays[85]

	return forecast
}

func runSingleEpicForecast(ctx context.Context, client *jira.Client, epicKey string) error {
	fmt.Printf("Fetching Epic %s...\n", epicKey)

	// Get issues in this epic
	jql := fmt.Sprintf("\"Epic Link\" = %s OR parent = %s", epicKey, epicKey)
	issues, err := client.SearchAllIssues(ctx, jql, "status,summary,issuetype", "")
	if err != nil {
		return fmt.Errorf("failed to fetch epic issues: %w", err)
	}

	// Count remaining
	mapper := getWorkflowMapper()
	var remaining int
	for _, issue := range issues {
		if !mapper.IsCompleted(issue.Fields.Status.Name) {
			remaining++
		}
	}

	fmt.Printf("Found %d total issues, %d remaining\n", len(issues), remaining)

	if remaining == 0 {
		fmt.Println("Epic is complete - no remaining items to forecast!")
		return nil
	}

	return runManualForecast(ctx, client, remaining)
}

func runManualForecast(ctx context.Context, client *jira.Client, remaining int) error {
	// Get historical throughput data
	jql, err := resolveJQL(ctx, client)
	if err != nil {
		return err
	}

	historyEnd := time.Now()
	historyStart := historyEnd.AddDate(0, 0, -historyDaysFlag)

	jqlWithDates := fmt.Sprintf("(%s) AND resolved >= %s AND resolved <= %s",
		jql, historyStart.Format("2006-01-02"), historyEnd.Format("2006-01-02"))

	fmt.Printf("\nFetching historical throughput data...\n")
	fmt.Printf("JQL: %s\n", jqlWithDates)

	issues, err := client.FetchIssuesWithHistory(ctx, jqlWithDates, func(current, total int) {
		fmt.Printf("\rProcessing issue %d/%d...", current, total)
	})
	if err != nil {
		return fmt.Errorf("failed to fetch issues: %w", err)
	}
	fmt.Println()

	if len(issues) == 0 {
		return fmt.Errorf("no historical throughput data found - cannot forecast")
	}

	// Map to workflow history
	mapper := getWorkflowMapper()
	histories := make([]workflow.IssueHistory, len(issues))
	for i, issue := range issues {
		histories[i] = mapper.MapIssueHistory(issue)
	}

	// Calculate weekly throughput
	throughputCalc := metrics.NewThroughputCalculator(metrics.FrequencyWeekly)
	throughputResult := throughputCalc.Calculate(histories, historyStart, historyEnd)
	weeklyThroughput := metrics.GetWeeklyThroughputValues(throughputResult)

	if len(weeklyThroughput) == 0 {
		return fmt.Errorf("no throughput data available for simulation")
	}

	// Configure Monte Carlo
	config := metrics.MonteCarloConfig{
		Trials:           trialsFlag,
		ThroughputWindow: historyDaysFlag,
		SimulationStart:  time.Now(),
	}

	if deadlineFlag != "" {
		deadline, err := time.Parse("2006-01-02", deadlineFlag)
		if err != nil {
			return fmt.Errorf("invalid deadline format: %w", err)
		}
		config.Deadline = &deadline
	}

	if config.Deadline == nil {
		if defaultDeadline := getConfigString("montecarlo.deadline"); defaultDeadline != "" {
			if deadline, err := time.Parse("2006-01-02", defaultDeadline); err == nil {
				config.Deadline = &deadline
			}
		}
	}

	fmt.Printf("\nRunning Monte Carlo simulation with %d trials...\n\n", config.Trials)

	simulator := metrics.NewMonteCarloSimulator(config, weeklyThroughput)
	result, err := simulator.Run(remaining)
	if err != nil {
		return fmt.Errorf("simulation failed: %w", err)
	}

	fmt.Print(metrics.FormatForecast(result))

	fmt.Printf("\nHistorical Data:\n")
	fmt.Printf("  Throughput window: %d days\n", historyDaysFlag)
	fmt.Printf("  Weeks sampled: %d\n", len(weeklyThroughput))
	fmt.Printf("  Weekly throughput range: %d to %d items\n",
		minInt(weeklyThroughput), maxInt(weeklyThroughput))

	return nil
}

func exportForecastsCSV(forecasts []EpicForecast, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Epic Key", "Summary", "Total Items", "Completed", "Remaining", "Progress %", "50% Date", "85% Date", "95% Date", "Days to 85%", "Status"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, f := range forecasts {
		status := "In Progress"
		if f.Error != "" {
			status = "Error: " + f.Error
		} else if f.RemainingItems == 0 {
			status = "Complete"
		}

		row := []string{
			f.EpicKey,
			f.EpicSummary,
			strconv.Itoa(f.TotalItems),
			strconv.Itoa(f.CompletedItems),
			strconv.Itoa(f.RemainingItems),
			strconv.FormatFloat(f.Progress, 'f', 1, 64),
			f.Forecast50.Format("2006-01-02"),
			f.Forecast85.Format("2006-01-02"),
			f.Forecast95.Format("2006-01-02"),
			strconv.Itoa(f.Days85),
			status,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

func sum(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

func minInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func maxInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
