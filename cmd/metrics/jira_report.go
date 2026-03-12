package metrics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/internal/charts"
	"devctl-em/internal/jira"
	pkgmetrics "devctl-em/internal/metrics"
	"devctl-em/internal/workflow"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate combined metrics report as a single HTML page",
	Long: `Generate an HTML report combining cycle time scatter, throughput trend,
and Monte Carlo epic forecast.

When teams are configured, generates one report per team. Use --team to
generate for a single team, or --jql/--project to bypass team iteration.

Uses the last 6 weeks of data by default. Output is an HTML file.

Example:
  devctl-em metrics jira report
  devctl-em metrics jira report --team platform
  devctl-em metrics jira report --from 2024-01-01 -o report.html`,
	RunE: runReport,
}

func init() {
	JiraCmd.AddCommand(reportCmd)
	reportCmd.Flags().BoolVar(&selectEpicsFlag, "select", false, "Interactively select which epics to include in the forecast")
}

func runReport(cmd *cobra.Command, args []string) error {
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
		return generateReport(ctx, client, team, jql, from, to)
	})
}

// jiraMetricsData holds computed JIRA metrics ready for chart rendering.
type jiraMetricsData struct {
	KeptResults      []pkgmetrics.CycleTimeResult
	ThroughputResult pkgmetrics.ThroughputResult
	LongestCTRows    []charts.LongestCycleTimeRow
	ForecastRows     []charts.ForecastRow
	Summary          charts.ReportSummary
	BaseURL          string
}

// collectJIRAMetricsData fetches and computes JIRA metrics for a single team/JQL.
// When verbose is true, progress is printed to stdout.
func collectJIRAMetricsData(ctx context.Context, client *jira.Client, team, jql string, from, to time.Time, verbose bool) (jiraMetricsData, error) {
	log := func(format string, args ...any) {
		if verbose {
			fmt.Printf(format, args...)
		}
	}
	progress := func(current, total int) {
		if verbose {
			fmt.Printf("\rProcessing completed issues: %d/%d...", current, total)
		}
	}

	jqlCompleted := jqlWithDateRange(
		fmt.Sprintf("(%s) AND issuetype in (Story, Spike, Bug, Defect)", jql),
		from.Format("2006-01-02"), to.Format("2006-01-02"),
	)

	log("Fetching issues from JIRA...\n")
	completedIssues, err := client.FetchIssuesWithHistory(ctx, jqlCompleted, progress)
	if err != nil {
		return jiraMetricsData{}, fmt.Errorf("failed to fetch completed issues: %w", err)
	}
	if verbose {
		fmt.Println()
	}
	log("\nFound %d completed issues\n\n", len(completedIssues))

	mapper := getWorkflowMapper()
	completedHistories := make([]workflow.IssueHistory, len(completedIssues))
	for i, issue := range completedIssues {
		completedHistories[i] = mapper.MapIssueHistory(issue)
	}

	log("Calculating cycle time metrics...\n")
	cycleCalc := pkgmetrics.NewCycleTimeCalculator(mapper)
	cycleResults := cycleCalc.Calculate(completedHistories)
	keptResults, outlierResults := pkgmetrics.FilterCycleTimeOutliers(cycleResults, 2.0)

	log("Calculating throughput metrics...\n")
	throughputCalc := pkgmetrics.NewThroughputCalculator(pkgmetrics.FrequencyWeekly, mapper)
	throughputResult := throughputCalc.Calculate(completedHistories, from, to)

	// Longest Cycle Time table
	var ctRows []charts.LongestCycleTimeRow
	if len(cycleResults) > 0 {
		outlierKeys := make(map[string]bool, len(outlierResults))
		for _, r := range outlierResults {
			outlierKeys[r.IssueKey] = true
		}
		sorted := make([]pkgmetrics.CycleTimeResult, 0, len(cycleResults))
		for _, r := range cycleResults {
			if r.IssueType != "Epic" {
				sorted = append(sorted, r)
			}
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].CycleTime > sorted[j].CycleTime
		})
		n := min(len(sorted), 5)
		for _, r := range sorted[:n] {
			ctRows = append(ctRows, charts.LongestCycleTimeRow{
				Key:       r.IssueKey,
				Summary:   r.Summary,
				Days:      fmt.Sprintf("%.1f", r.CycleTimeDays()),
				Started:   r.StartDate.Format("Jan 02"),
				Completed: r.EndDate.Format("Jan 02"),
				Outlier:   outlierKeys[r.IssueKey],
			})
		}
	}

	// Forecast — use 90-day throughput window for Monte Carlo
	var forecastRows []charts.ForecastRow
	{
		const forecastHistoryDays = 90
		forecastFrom := pkgmetrics.WeekStart(time.Now().AddDate(0, 0, -forecastHistoryDays))

		var forecastThroughput []int
		if !from.After(forecastFrom) {
			forecastThroughput = pkgmetrics.GetWeeklyThroughputValues(throughputResult)
		} else {
			forecastJQL := jqlWithDateRange(jql, forecastFrom.Format("2006-01-02"), time.Now().Format("2006-01-02"))
			log("Fetching 120-day throughput history for forecast...\n")
			forecastIssues, fetchErr := client.FetchIssuesWithHistory(ctx, forecastJQL, func(current, total int) {
				if verbose {
					fmt.Printf("\rProcessing forecast issues: %d/%d...", current, total)
				}
			})
			if fetchErr != nil {
				log("Warning: forecast unavailable: %v\n", fetchErr)
			} else {
				if verbose {
					fmt.Println()
				}
				forecastHistories := make([]workflow.IssueHistory, len(forecastIssues))
				for i, issue := range forecastIssues {
					forecastHistories[i] = mapper.MapIssueHistory(issue)
				}
				fc := pkgmetrics.NewThroughputCalculator(pkgmetrics.FrequencyWeekly, mapper)
				fcResult := fc.Calculate(forecastHistories, forecastFrom, time.Now())
				forecastThroughput = pkgmetrics.GetWeeklyThroughputValues(fcResult)
			}
		}

		if len(forecastThroughput) > 0 {
			epics, epicErr := fetchOpenEpics(ctx, client, team)
			if epicErr != nil {
				log("Warning: forecast unavailable: %v\n", epicErr)
			} else if len(epics) > 0 {
				sequential := false
				if selectEpicsFlag {
					if selected, selErr := promptEpicSelection(epics); selErr != nil {
						log("Warning: epic selection skipped: %v\n", selErr)
					} else {
						saveEpicSelection(team, selected)
						epics = selected
						sequential = true
					}
				} else {
					epics = applyEpicSelection(epics, team)
					sequential = hasEpicSelection(team)
				}

				log("Found %d open epics, forecasting...\n", len(epics))
				mapper := getWorkflowMapper()

				var pending []EpicForecast
				for _, epic := range epics {
					f := fetchEpicCounts(ctx, client, mapper, epic)
					if f.RemainingItems == 0 || f.Error != "" {
						continue
					}
					pending = append(pending, f)
				}

				var forecasts []EpicForecast
				if sequential {
					forecasts = runSequentialSimulation(pending, forecastThroughput)
				} else {
					forecasts = runIndependentSimulation(pending, forecastThroughput)
				}

				for _, f := range forecasts {
					if f.Error != "" {
						continue
					}
					forecastRows = append(forecastRows, charts.ForecastRow{
						EpicKey:    f.EpicKey,
						Summary:    f.EpicSummary,
						Completed:  f.CompletedItems,
						Total:      f.TotalItems,
						Remaining:  f.RemainingItems,
						Forecast50: f.Forecast50.Format("Jan 02"),
						Forecast85: f.Forecast85.Format("Jan 02"),
						Forecast95: f.Forecast95.Format("Jan 02"),
					})
				}
			}
		}
	}

	// Summary metrics
	summary := buildSummary(keptResults, throughputResult)
	summary.ActiveEpics = countActiveEpics(ctx, client, jql)

	return jiraMetricsData{
		KeptResults:      keptResults,
		ThroughputResult: throughputResult,
		LongestCTRows:    ctRows,
		ForecastRows:     forecastRows,
		Summary:          summary,
		BaseURL:          client.BaseURL(),
	}, nil
}

// buildSummary computes the avg cycle time and avg throughput summary values.
func buildSummary(cycleResults []pkgmetrics.CycleTimeResult, throughput pkgmetrics.ThroughputResult) charts.ReportSummary {
	avgCT := "—"
	if len(cycleResults) > 0 {
		stats := pkgmetrics.CalculateStats(cycleResults)
		days := stats.Mean.Hours() / 24
		if days < 1 {
			avgCT = "<1 day"
		} else {
			avgCT = fmt.Sprintf("%.1f days", days)
		}
	}
	avgTP := "—"
	if throughput.AvgCount > 0 {
		avgTP = fmt.Sprintf("%.1f", throughput.AvgCount)
	}
	return charts.ReportSummary{
		AvgCycleTime:  avgCT,
		AvgThroughput: avgTP,
	}
}

// countActiveEpics queries for issues currently in progress and counts distinct parent epics.
// Returns 0 on any error (best-effort).
func countActiveEpics(ctx context.Context, client *jira.Client, baseJQL string) int {
	activeJQL := fmt.Sprintf("(%s) AND issuetype in (Story, Spike, Bug, Defect) AND statusCategory = \"In Progress\"", baseJQL)
	issues, err := client.SearchAllIssues(ctx, activeJQL, "parent,issuetype", "")
	if err != nil {
		return 0
	}
	epicSet := make(map[string]bool, len(issues))
	for _, issue := range issues {
		if p := issue.Fields.Parent; p != nil && p.Key != "" && p.Fields.IssueType.Name == "Epic" {
			epicSet[p.Key] = true
		} else if e := issue.Fields.Epic; e != nil && e.Key != "" {
			epicSet[e.Key] = true
		}
	}
	return len(epicSet)
}

// generateReport generates a single combined HTML report for the given JQL.
func generateReport(ctx context.Context, client *jira.Client, team, jql string, from, to time.Time) error {
	outputPath := getOutputPath(teamOutputName("jira-report", team), "html")

	fmt.Printf("Generating JIRA Report...\n")
	fmt.Printf("JQL: %s\n", jql)
	fmt.Printf("Date range: %s to %s\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	data, err := collectJIRAMetricsData(ctx, client, team, jql, from, to, true)
	if err != nil {
		return err
	}

	if err := charts.CombinedReport(data.Summary, data.KeptResults, []float64{50, 85, 95}, data.ThroughputResult, data.LongestCTRows, data.ForecastRows, data.BaseURL, outputPath); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("\nReport generated: %s\n", outputPath)
	openBrowser(outputPath)
	return nil
}

