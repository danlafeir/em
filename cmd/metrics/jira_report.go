package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"em/internal/charts"
	"em/internal/jira"
	pkgmetrics "em/internal/metrics"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate combined metrics report as a single HTML page",
	Long: `Generate an HTML report combining cycle time, throughput, and forecast.

Required:
  em metrics jira config`,
	RunE: runReport,
}

func init() {
	JiraCmd.AddCommand(reportCmd)
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

	completedHistories, mapper := mapIssuesToHistories(completedIssues)

	log("Calculating cycle time metrics...\n")
	cycleResults, keptResults, outlierKeys := computeCycleTimeFromHistories(completedHistories, mapper)

	log("Calculating throughput metrics...\n")
	throughputResult := computeThroughputFromHistories(completedHistories, mapper, pkgmetrics.FrequencyWeekly, from, to)

	// Longest Cycle Time table — last 2 weeks only
	ctRows := buildLongestCTRows(recentResults(keptResults, 14), nil, reportLongestCTLimit)

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
				forecastHistories, forecastMapper := mapIssuesToHistories(forecastIssues)
				fcResult := computeThroughputFromHistories(forecastHistories, forecastMapper, pkgmetrics.FrequencyWeekly, forecastFrom, time.Now())
				forecastThroughput = pkgmetrics.GetWeeklyThroughputValues(fcResult)
			}
		}

		if len(forecastThroughput) > 0 {
			var epics []jira.Issue
			var epicErr error
			sequential := false
			if savedKeys := loadEpicSelection(team); len(savedKeys) > 0 {
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

	ctRows := buildLongestCTRows(recentResults(ct.kept, 14), nil, reportLongestCTLimit)
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
// cycleResults is expected to be already outlier-filtered (IQR-based).
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
	fmt.Printf("Generating JIRA Report...\n")
	fmt.Printf("JQL: %s\n", jql)
	fmt.Printf("Date range: %s to %s\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	data, err := collectJIRAMetricsData(ctx, client, team, jql, from, to, true)
	if err != nil {
		return err
	}

	return renderJIRAReport(team, data)
}

// renderJIRAReport writes the standalone JIRA HTML report from pre-fetched data.
func renderJIRAReport(team string, data jiraMetricsData) error {
	outputPath := getOutputPath(teamOutputName("jira-report", team), "html")

	if err := charts.CombinedReport(data.Summary, data.KeptResults, []float64{50, 85, 95}, data.ThroughputResult, data.LongestCTRows, data.ForecastRows, data.BaseURL, outputPath); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("\nReport generated: %s\n", outputPath)
	openBrowser(outputPath)
	return nil
}

// recentResults returns cycle time results whose EndDate falls within the last days days.
func recentResults(results []pkgmetrics.CycleTimeResult, days int) []pkgmetrics.CycleTimeResult {
	cutoff := time.Now().AddDate(0, 0, -days)
	filtered := make([]pkgmetrics.CycleTimeResult, 0, len(results))
	for _, r := range results {
		if !r.EndDate.Before(cutoff) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

