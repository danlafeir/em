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

	client, err := getJiraClient()
	if err != nil {
		return err
	}

	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to JIRA: %w", err)
	}

	// --epic and --remaining bypass team iteration
	if epicFlag != "" {
		return runSingleEpicForecast(ctx, client, epicFlag)
	}
	if remainingFlag > 0 {
		return runManualForecast(ctx, client, remainingFlag)
	}

	// Default: forecast all open epics, iterating per team
	return withTeamIteration(ctx, client, func(team, jql string) error {
		return runAllEpicsForecast(ctx, client, team, jql)
	})
}

func runAllEpicsForecast(ctx context.Context, client *jira.Client, team, throughputJQLBase string) error {
	fmt.Println("Discovering open epics...")

	// Get project-level JQL for epic discovery
	var baseJQL string
	var err error
	if team != "" {
		baseJQL, err = getTeamProjectJQL(team)
	} else {
		baseJQL, err = getProjectJQL()
	}
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
	historyEnd := time.Now()
	historyStart := historyEnd.AddDate(0, 0, -historyDaysFlag)

	throughputJQL := fmt.Sprintf("(%s) AND resolved >= %s AND resolved <= %s",
		throughputJQLBase, historyStart.Format("2006-01-02"), historyEnd.Format("2006-01-02"))

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
		if forecast.RemainingItems == 0 {
			continue
		}
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
		return forecasts[i].Forecast85.Before(forecasts[j].Forecast85)
	})

	// Print summary table
	fmt.Printf("\n")
	fmt.Printf("Epic Forecast Summary\n")
	fmt.Printf("=====================\n\n")

	summaryWidth := 40
	// Header
	fmt.Printf("| %-14s | %-*s | %-6s | %-6s | %-6s | %-10s | %-10s | %-10s |\n",
		"Epic", summaryWidth, "Summary", "Done", "Left", "Prog%", "50%", "85%", "95%")
	fmt.Printf("|%s|%s|%s|%s|%s|%s|%s|%s|\n",
		strings.Repeat("_", 16),
		strings.Repeat("_", summaryWidth+2),
		strings.Repeat("_", 8),
		strings.Repeat("_", 8),
		strings.Repeat("_", 8),
		strings.Repeat("_", 12),
		strings.Repeat("_", 12),
		strings.Repeat("_", 12))

	for _, f := range forecasts {
		if f.Error != "" {
			fmt.Printf("| %-14s | %-*s | %-6s | %-6s | %-6s | %-10s | %-10s | %-10s |\n",
				f.EpicKey, summaryWidth, truncate(f.EpicSummary, summaryWidth),
				"-", "-", "-", f.Error, "", "")
			continue
		}

		lines := wrapString(f.EpicSummary, summaryWidth)
		for l, line := range lines {
			if l == 0 {
				fmt.Printf("| %-14s | %-*s | %6d | %6d | %5.0f%% | %-10s | %-10s | %-10s |\n",
					f.EpicKey, summaryWidth, line,
					f.CompletedItems, f.RemainingItems, f.Progress,
					f.Forecast50.Format("Jan 02"),
					f.Forecast85.Format("Jan 02"),
					f.Forecast95.Format("Jan 02"))
			} else {
				fmt.Printf("| %-14s | %-*s | %6s | %6s | %6s | %-10s | %-10s | %-10s |\n",
					"", summaryWidth, line, "", "", "", "", "", "")
			}
		}
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

	// Export PNG chart
	var rows []charts.ForecastRow
	for _, f := range forecasts {
		if f.Error != "" {
			continue
		}
		rows = append(rows, charts.ForecastRow{
			EpicKey:    f.EpicKey,
			Summary:    f.EpicSummary,
			Remaining:  f.RemainingItems,
			Forecast50: f.Forecast50.Format("Jan 02"),
			Forecast85: f.Forecast85.Format("Jan 02"),
			Forecast95: f.Forecast95.Format("Jan 02"),
		})
	}
	if len(rows) > 0 {
		p := charts.ForecastTable(rows)
		pngPath := getOutputPath(teamOutputName("epic-forecasts", team), "png")
		if err := charts.SaveChart(p, pngPath, charts.DefaultConfig()); err == nil {
			fmt.Printf("Chart saved to %s\n", pngPath)
		}
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
