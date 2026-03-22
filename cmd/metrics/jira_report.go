package metrics

import (
	"context"
	"fmt"
	"strings"
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
	if useSavedDataFlag {
		return loadJIRAMetricsData(team, client)
	}

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
	cycleResults, keptResults, outlierKeys := computeCycleTimeFromHistories(completedHistories, mapper)

	log("Calculating throughput metrics...\n")
	throughputCalc := pkgmetrics.NewThroughputCalculator(pkgmetrics.FrequencyWeekly, mapper)
	throughputResult := throughputCalc.Calculate(completedHistories, from, to)

	// Longest Cycle Time table
	ctRows := buildLongestCTRows(cycleResults, outlierKeys, reportLongestCTLimit)

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
			var epics []jira.Issue
			var epicErr error
			sequential := false
			if selectEpicsFlag {
				epics, epicErr = fetchOpenEpics(ctx, client, team)
				if epicErr == nil && len(epics) > 0 {
					if selected, selErr := promptEpicSelection(epics); selErr != nil {
						log("Warning: epic selection skipped: %v\n", selErr)
						epics = nil
					} else {
						saveEpicSelection(team, selected)
						epics = selected
						sequential = true
					}
				}
			} else if savedKeys := loadEpicSelection(team); len(savedKeys) > 0 {
				epics, epicErr = fetchEpicsByKeys(ctx, client, savedKeys)
				sequential = true
			} else {
				epics, epicErr = fetchOpenEpics(ctx, client, team)
			}

			if epicErr != nil {
				log("Warning: forecast unavailable: %v\n", epicErr)
			} else if len(epics) > 0 {
				log("Found %d epic(s), forecasting...\n", len(epics))
				allForecasts := computeEpicForecasts(ctx, client, epics, forecastThroughput, sequential)
				for _, f := range allForecasts {
					if f.Error != "" || f.RemainingItems == 0 {
						continue
					}
					forecastRows = append(forecastRows, epicForecastToRow(f))
				}
			}
		}
	}

	// Summary metrics
	summary := buildSummary(keptResults, throughputResult)
	summary.ActiveEpics = countActiveEpics(ctx, client, jql)

	// Save data for future --use-saved-data runs (best effort).
	_ = saveJiraCycleTimeData(cycleResults, outlierKeys, team)
	_ = saveJiraThroughputData(throughputResult, team)
	_ = saveJiraForecastData(forecastRows, team)

	return jiraMetricsData{
		KeptResults:      keptResults,
		ThroughputResult: throughputResult,
		LongestCTRows:    ctRows,
		ForecastRows:     forecastRows,
		Summary:          summary,
		BaseURL:          client.BaseURL(),
	}, nil
}

// loadJIRAMetricsData reconstructs jiraMetricsData from saved CSVs without API calls.
func loadJIRAMetricsData(team string, client *jira.Client) (jiraMetricsData, error) {
	fmt.Printf("Loading JIRA data from saved CSVs (team: %q)...\n", team)

	ct, err := loadJiraCycleTimeData(team)
	if err != nil {
		return jiraMetricsData{}, err
	}
	throughputResult, err := loadJiraThroughputData(team)
	if err != nil {
		return jiraMetricsData{}, err
	}
	forecastRows, _ := loadJiraForecastData(team) // optional

	ctRows := buildLongestCTRows(ct.all, ct.outlierKeys, reportLongestCTLimit)
	summary := buildSummary(ct.kept, throughputResult)

	baseURL := ""
	if client != nil {
		baseURL = client.BaseURL()
	} else {
		domain := getConfigString("jira.domain")
		if domain != "" {
			baseURL = "https://" + domain + ".atlassian.net"
		}
	}

	return jiraMetricsData{
		KeptResults:      ct.kept,
		ThroughputResult: throughputResult,
		LongestCTRows:    ctRows,
		ForecastRows:     forecastRows,
		Summary:          summary,
		BaseURL:          baseURL,
	}, nil
}

// buildSummary computes the avg cycle time and avg throughput summary values.
// Cycle time mean excludes outliers beyond 2 standard deviations.
func buildSummary(cycleResults []pkgmetrics.CycleTimeResult, throughput pkgmetrics.ThroughputResult) charts.ReportSummary {
	avgCT := "—"
	if len(cycleResults) > 0 {
		stats := pkgmetrics.CalculateStats(cycleResults)
		mean := stats.Mean
		stdDev := stats.StdDev
		if stdDev > 0 {
			var total time.Duration
			var count int
			threshold := 2 * stdDev
			for _, r := range cycleResults {
				if r.CycleTime >= mean-threshold && r.CycleTime <= mean+threshold {
					total += r.CycleTime
					count++
				}
			}
			if count > 0 {
				mean = total / time.Duration(count)
			}
		}
		days := mean.Hours() / 24
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

// countActiveEpics fetches open epics scoped to baseJQL, then counts those with at least
// one child card whose changelog shows it was ever transitioned to the cycle start stage
// or later (e.g. "In Progress" and beyond).
// Returns 0 on any error (best-effort).
func countActiveEpics(ctx context.Context, client *jira.Client, baseJQL string) int {
	epicJQL := fmt.Sprintf("(%s) AND issuetype = Epic AND resolution IS EMPTY", baseJQL)
	epics, err := client.SearchAllIssues(ctx, epicJQL, "summary", "")
	if err != nil || len(epics) == 0 {
		return 0
	}

	keys := make([]string, len(epics))
	for i, e := range epics {
		keys[i] = e.Key
	}

	childJQL := fmt.Sprintf("parent in (%s)", strings.Join(keys, ","))
	children, err := client.SearchAllIssues(ctx, childJQL, "parent", "changelog")
	if err != nil {
		return 0
	}

	mapper := getWorkflowMapper()
	startStage, _ := mapper.GetCycleTimeStages()
	startOrder := mapper.GetStageOrder(startStage)

	epicSet := make(map[string]bool, len(children))
	for _, issue := range children {
		if issue.Changelog == nil {
			continue
		}
		for _, entry := range issue.Changelog.Histories {
			for _, item := range entry.Items {
				if item.Field != "status" {
					continue
				}
				if mapper.GetStageOrder(mapper.GetStage(item.ToString)) >= startOrder {
					if p := issue.Fields.Parent; p != nil && p.Key != "" {
						epicSet[p.Key] = true
					}
				}
			}
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

