package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
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
	selectEpicsFlag bool
)

func init() {
	JiraCmd.AddCommand(forecastCmd)

	// Forecast-specific flags
	forecastCmd.Flags().StringVar(&epicFlag, "epic", "", "Epic key to forecast (fetches remaining items automatically)")
	forecastCmd.Flags().IntVar(&remainingFlag, "remaining", 0, "Number of remaining items to complete")
	forecastCmd.Flags().StringVar(&deadlineFlag, "deadline", "", "Target deadline date (YYYY-MM-DD)")
	forecastCmd.Flags().IntVar(&trialsFlag, "trials", 10000, "Number of Monte Carlo simulations")
	forecastCmd.Flags().IntVar(&historyDaysFlag, "history-days", 120, "Days of historical throughput to sample from")
	forecastCmd.Flags().BoolVar(&allEpicsFlag, "all", false, "Forecast all open epics (default when no other flags)")
	forecastCmd.Flags().BoolVar(&selectEpicsFlag, "select", false, "Interactively select which epics to forecast")
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

	return withTeamIteration(ctx, client, func(team, jql string) error {
		if epicFlag != "" {
			return runSingleEpicForecast(ctx, client, jql, epicFlag)
		}
		if remainingFlag > 0 {
			return runManualForecast(ctx, client, jql, remainingFlag)
		}
		if selectEpicsFlag {
			return runSelectEpicsForecast(ctx, client, team, jql)
		}
		return runAllEpicsForecast(ctx, client, team, jql)
	})
}

func fetchOpenEpics(ctx context.Context, client *jira.Client, team string) ([]jira.Issue, error) {
	var baseJQL string
	var err error
	if team != "" {
		baseJQL, err = getTeamProjectJQL(team)
	} else {
		baseJQL, err = getProjectJQL()
	}
	if err != nil {
		return nil, err
	}

	epicJQL := fmt.Sprintf("(%s) AND issuetype = Epic AND resolution IS EMPTY ORDER BY key", baseJQL)
	fmt.Printf("JQL: %s\n\n", epicJQL)

	epics, err := client.SearchAllIssues(ctx, epicJQL, "summary,status", "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch epics: %w", err)
	}
	return epics, nil
}

func loadWeeklyThroughput(ctx context.Context, client *jira.Client, throughputJQLBase string) ([]int, error) {
	historyEnd := time.Now()
	historyStart := metrics.WeekStart(historyEnd.AddDate(0, 0, -historyDaysFlag))

	throughputJQL := jqlWithDateRange(throughputJQLBase, historyStart.Format("2006-01-02"), historyEnd.Format("2006-01-02"))

	fmt.Println("Fetching historical throughput data...")
	completedIssues, err := client.FetchIssuesWithHistory(ctx, throughputJQL, func(current, total int) {
		fmt.Printf("\rProcessing: %d/%d issues...", current, total)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch throughput data: %w", err)
	}
	fmt.Println()

	if len(completedIssues) == 0 {
		return nil, fmt.Errorf("no historical throughput data found - cannot forecast")
	}

	mapper := getWorkflowMapper()
	histories := make([]workflow.IssueHistory, len(completedIssues))
	for i, issue := range completedIssues {
		histories[i] = mapper.MapIssueHistory(issue)
	}

	throughputCalc := metrics.NewThroughputCalculator(metrics.FrequencyWeekly, mapper)
	throughputResult := throughputCalc.Calculate(histories, historyStart, historyEnd)
	weeklyThroughput := metrics.GetWeeklyThroughputValues(throughputResult)


	if len(weeklyThroughput) == 0 {
		return nil, fmt.Errorf("no throughput data available for simulation")
	}

	avgThroughput := float64(sum(weeklyThroughput)) / float64(len(weeklyThroughput))
	fmt.Printf("\nHistorical throughput: %.1f items/week (from %d weeks)\n\n", avgThroughput, len(weeklyThroughput))

	return weeklyThroughput, nil
}

func hasEpicSelection(team string) bool {
	return len(loadEpicSelection(team)) > 0
}

// fetchEpicCounts fetches the issue counts for an epic without running simulation.
func fetchEpicCounts(ctx context.Context, client *jira.Client, mapper *workflow.Mapper, epic jira.Issue) EpicForecast {
	forecast := EpicForecast{
		EpicKey:     epic.Key,
		EpicSummary: epic.Fields.Summary,
	}
	jql := fmt.Sprintf("\"Epic Link\" = %s OR parent = %s", epic.Key, epic.Key)
	issues, err := client.SearchAllIssues(ctx, jql, "status,summary,issuetype", "")
	if err != nil {
		forecast.Error = "fetch failed"
		return forecast
	}
	forecast.TotalItems = len(issues)
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
	return forecast
}

// runIndependentSimulation runs a separate Monte Carlo simulation for each epic.
func runIndependentSimulation(pending []EpicForecast, weeklyThroughput []int) []EpicForecast {
	workers := getConfiguredWorkThreads()
	config := metrics.MonteCarloConfig{
		Trials:          trialsFlag,
		SimulationStart: time.Now(),
	}
	simulator := metrics.NewMonteCarloSimulator(config, weeklyThroughput)
	forecasts := make([]EpicForecast, 0, len(pending))
	for _, f := range pending {
		result, err := simulator.RunMultiPercentile(f.RemainingItems, workers)
		if err != nil {
			f.Error = "sim failed"
			forecasts = append(forecasts, f)
			continue
		}
		f.Forecast50 = result.Percentiles[50]
		f.Forecast85 = result.Percentiles[85]
		f.Forecast95 = result.Percentiles[95]
		f.Days85 = result.PercentileDays[85]
		forecasts = append(forecasts, f)
	}
	return forecasts
}

// runSequentialSimulation runs one Monte Carlo simulation across all epics in order,
// so each epic's dates account for all higher-priority work completing first.
func runSequentialSimulation(pending []EpicForecast, weeklyThroughput []int) []EpicForecast {
	workers := getConfiguredWorkThreads()
	remainingItems := make([]int, len(pending))
	for i, f := range pending {
		remainingItems[i] = f.RemainingItems
	}
	config := metrics.MonteCarloConfig{
		Trials:          trialsFlag,
		SimulationStart: time.Now(),
	}
	simulator := metrics.NewMonteCarloSimulator(config, weeklyThroughput)
	results, err := simulator.RunSequentialMultiPercentile(remainingItems, workers)
	if err != nil {
		return runIndependentSimulation(pending, weeklyThroughput)
	}
	forecasts := make([]EpicForecast, len(pending))
	for i, f := range pending {
		forecasts[i] = f
		r := results[i]
		forecasts[i].Forecast50 = r.Percentiles[50]
		forecasts[i].Forecast85 = r.Percentiles[85]
		forecasts[i].Forecast95 = r.Percentiles[95]
		forecasts[i].Days85 = r.PercentileDays[85]
	}
	return forecasts
}

func runEpicForecasts(ctx context.Context, client *jira.Client, epics []jira.Issue, weeklyThroughput []int, team string, preserveOrder bool) error {
	mapper := getWorkflowMapper()

	fmt.Println("Forecasting epics...")
	var allFetched []EpicForecast
	var pending []EpicForecast
	for i, epic := range epics {
		fmt.Printf("\r[%d/%d] Fetching %s...", i+1, len(epics), epic.Key)
		f := fetchEpicCounts(ctx, client, mapper, epic)
		allFetched = append(allFetched, f)
		if f.RemainingItems > 0 && f.Error == "" {
			pending = append(pending, f)
		}
	}
	fmt.Println()

	// Run simulation only on incomplete epics, then merge results back by key.
	simByKey := make(map[string]EpicForecast, len(pending))
	if len(pending) > 0 {
		var simulated []EpicForecast
		if preserveOrder {
			fmt.Println("Running sequential Monte Carlo simulation...")
			simulated = runSequentialSimulation(pending, weeklyThroughput)
		} else {
			simulated = runIndependentSimulation(pending, weeklyThroughput)
		}
		for _, f := range simulated {
			simByKey[f.EpicKey] = f
		}
	}

	// Reconstruct the full list in original order.
	forecasts := make([]EpicForecast, 0, len(allFetched))
	for _, f := range allFetched {
		if sim, ok := simByKey[f.EpicKey]; ok {
			forecasts = append(forecasts, sim)
		} else {
			forecasts = append(forecasts, f)
		}
	}

	// When not preserving order, sort by forecast date (completed and errors last).
	if !preserveOrder {
		sort.Slice(forecasts, func(i, j int) bool {
			fi, fj := forecasts[i], forecasts[j]
			if fi.Error != "" || fi.RemainingItems == 0 {
				return false
			}
			if fj.Error != "" || fj.RemainingItems == 0 {
				return true
			}
			return fi.Forecast85.Before(fj.Forecast85)
		})
	}

	// Print summary
	fmt.Printf("\nEpic Forecast Summary\n")
	fmt.Printf("=====================\n\n")

	const barWidth = 20
	const keyWidth = 14

	for _, f := range forecasts {
		if f.Error != "" {
			fmt.Printf("%-*s  %s\n             (error: %s)\n\n",
				keyWidth, f.EpicKey, f.EpicSummary, f.Error)
			continue
		}

		filled := barWidth
		if f.TotalItems > 0 {
			filled = int(float64(f.CompletedItems) / float64(f.TotalItems) * barWidth)
		}
		bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
		progress := fmt.Sprintf("%d/%d", f.CompletedItems, f.TotalItems)
		indent := strings.Repeat(" ", keyWidth+2)

		fmt.Printf("%-*s  %s\n", keyWidth, f.EpicKey, f.EpicSummary)
		if f.RemainingItems == 0 {
			fmt.Printf("%s%s %s  ·  Done\n\n", indent, bar, progress)
		} else {
			fmt.Printf("%s%s %s  ·  50%%: %s  85%%: %s  95%%: %s\n\n",
				indent, bar, progress,
				f.Forecast50.Format("Jan 02"),
				f.Forecast85.Format("Jan 02"),
				f.Forecast95.Format("Jan 02"))
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

	// Export HTML chart
	var rows []charts.ForecastRow
	for _, f := range forecasts {
		if f.Error != "" {
			continue
		}
		row := charts.ForecastRow{
			EpicKey:   f.EpicKey,
			Summary:   f.EpicSummary,
			Completed: f.CompletedItems,
			Total:     f.TotalItems,
			Remaining: f.RemainingItems,
		}
		if f.RemainingItems == 0 {
			row.Forecast50 = "Done"
			row.Forecast85 = "Done"
			row.Forecast95 = "Done"
		} else {
			row.Forecast50 = f.Forecast50.Format("Jan 02")
			row.Forecast85 = f.Forecast85.Format("Jan 02")
			row.Forecast95 = f.Forecast95.Format("Jan 02")
		}
		rows = append(rows, row)
	}
	if saveRawDataFlag && len(rows) > 0 {
		if err := saveJiraForecastData(rows, team); err == nil {
			fmt.Printf("Raw data saved to: %s\n", savedJiraForecastPath(team))
		}
		if err := saveJiraForecastThroughput(weeklyThroughput, team); err == nil {
			fmt.Printf("Raw data saved to: %s\n", savedJiraForecastThroughputPath(team))
		}
	}

	if len(rows) > 0 {
		htmlPath := getOutputPath(teamOutputName("epic-forecasts", team), "html")
		if err := charts.ForecastTable(rows, client.BaseURL(), htmlPath); err == nil {
			fmt.Printf("Chart saved to %s\n", htmlPath)
			charts.OpenBrowser(htmlPath)
		}
	}

	return nil
}

func runAllEpicsForecast(ctx context.Context, client *jira.Client, team, throughputJQLBase string) error {
	fmt.Println("Discovering open epics...")

	epics, err := fetchOpenEpics(ctx, client, team)
	if err != nil {
		return err
	}

	if len(epics) == 0 {
		fmt.Println("No open epics found.")
		return nil
	}

	sequential := hasEpicSelection(team)
	epics = applyEpicSelection(epics, team)
	if !hasEpicSelection(team) {
		epics = sortEpicsByInProgress(ctx, client, epics)
	}
	fmt.Printf("Found %d open epics\n\n", len(epics))

	weeklyThroughput, err := loadWeeklyThroughput(ctx, client, throughputJQLBase)
	if err != nil {
		return err
	}

	return runEpicForecasts(ctx, client, epics, weeklyThroughput, team, sequential)
}

// sortEpicsByInProgress sorts epics descending by number of child cards currently in progress.
// Epics with more active work appear first. Falls back to original order on error.
func sortEpicsByInProgress(ctx context.Context, client *jira.Client, epics []jira.Issue) []jira.Issue {
	if len(epics) == 0 {
		return epics
	}
	keys := make([]string, len(epics))
	for i, e := range epics {
		keys[i] = e.Key
	}
	jql := fmt.Sprintf("parent in (%s) AND statusCategory = \"In Progress\"", strings.Join(keys, ","))
	children, err := client.SearchAllIssues(ctx, jql, "parent", "")
	if err != nil {
		return epics
	}
	counts := make(map[string]int, len(epics))
	for _, child := range children {
		if p := child.Fields.Parent; p != nil && p.Key != "" {
			counts[p.Key]++
		}
	}
	sorted := make([]jira.Issue, len(epics))
	copy(sorted, epics)
	sort.SliceStable(sorted, func(i, j int) bool {
		return counts[sorted[i].Key] > counts[sorted[j].Key]
	})
	return sorted
}

// applyEpicSelection filters epics to the saved selection for the team, preserving
// the saved order. Falls back to the full list if no selection is saved or if all
// saved epics have since been closed.
func applyEpicSelection(epics []jira.Issue, team string) []jira.Issue {
	keys := loadEpicSelection(team)
	if len(keys) == 0 {
		return epics
	}
	byKey := make(map[string]jira.Issue, len(epics))
	for _, e := range epics {
		byKey[e.Key] = e
	}
	var selected []jira.Issue
	for _, k := range keys {
		if e, ok := byKey[k]; ok {
			selected = append(selected, e)
		}
	}
	if len(selected) == 0 {
		// All saved epics are closed — fall back to all open epics
		return epics
	}
	fmt.Printf("Using saved epic selection (%d epic(s)). Run with --select to change.\n\n", len(selected))
	return selected
}

// promptEpicSelection shows a numbered list of epics and returns the user's selection.
func promptEpicSelection(epics []jira.Issue) ([]jira.Issue, error) {
	fmt.Printf("Found %d open epics:\n\n", len(epics))
	for i, epic := range epics {
		fmt.Printf("  %2d. %-14s  %s\n", i+1, epic.Key, truncate(epic.Fields.Summary, 60))
	}

	fmt.Printf("\nEnter epic numbers to forecast (comma-separated, e.g. 1,3,5): ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return nil, fmt.Errorf("no epics selected")
	}

	var selected []jira.Issue
	seen := make(map[int]bool)
	for part := range strings.SplitSeq(input, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 || n > len(epics) {
			return nil, fmt.Errorf("invalid selection: %q (must be 1-%d)", strings.TrimSpace(part), len(epics))
		}
		if !seen[n] {
			seen[n] = true
			selected = append(selected, epics[n-1])
		}
	}

	fmt.Printf("\nSelected %d epic(s)\n\n", len(selected))
	return selected, nil
}

func runSelectEpicsForecast(ctx context.Context, client *jira.Client, team, throughputJQLBase string) error {
	fmt.Println("Discovering open epics...")

	epics, err := fetchOpenEpics(ctx, client, team)
	if err != nil {
		return err
	}

	if len(epics) == 0 {
		fmt.Println("No open epics found.")
		return nil
	}

	selected, err := promptEpicSelection(epics)
	if err != nil {
		return err
	}
	saveEpicSelection(team, selected)

	weeklyThroughput, err := loadWeeklyThroughput(ctx, client, throughputJQLBase)
	if err != nil {
		return err
	}

	return runEpicForecasts(ctx, client, selected, weeklyThroughput, team, true)
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

func runSingleEpicForecast(ctx context.Context, client *jira.Client, throughputJQL, epicKey string) error {
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

	return runManualForecast(ctx, client, throughputJQL, remaining)
}

func runManualForecast(ctx context.Context, client *jira.Client, throughputJQL string, remaining int) error {
	historyEnd := time.Now()
	historyStart := metrics.WeekStart(historyEnd.AddDate(0, 0, -historyDaysFlag))

	jqlWithDates := jqlWithDateRange(throughputJQL, historyStart.Format("2006-01-02"), historyEnd.Format("2006-01-02"))

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
	throughputCalc := metrics.NewThroughputCalculator(metrics.FrequencyWeekly, mapper)
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
	result, err := simulator.RunMultiPercentile(remaining, getConfiguredWorkThreads())
	if err != nil {
		return fmt.Errorf("simulation failed: %w", err)
	}

	fmt.Print(metrics.FormatForecast(result))

	fmt.Printf("\nHistorical Data:\n")
	fmt.Printf("  Throughput window: %d days\n", historyDaysFlag)
	fmt.Printf("  Weeks sampled: %d\n", len(weeklyThroughput))
	fmt.Printf("  Weekly throughput range: %d to %d items\n",
		minInt(weeklyThroughput), maxInt(weeklyThroughput))

	if saveRawDataFlag {
		if err := saveJiraForecastThroughput(weeklyThroughput, ""); err == nil {
			fmt.Printf("\nRaw data saved to: %s\n", savedJiraForecastThroughputPath(""))
		}
	}

	return nil
}

// getConfiguredWorkThreads reads jira.work_threads from config (default 1).
func getConfiguredWorkThreads() int {
	if raw := getConfigString("jira.work_threads"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 1
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
