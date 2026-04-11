package charts

import (
	"fmt"
	"html/template"

	"github.com/danlafeir/em/pkg/execreport"
	"github.com/danlafeir/em/pkg/metrics"
)

// ReportSummary holds the key metrics displayed in the summary bar.
type ReportSummary struct {
	AvgCycleTime  string
	AvgThroughput string
	ActiveEpics   int
}

// ReportSummaryHTML returns a self-contained HTML fragment for the summary bar.
func ReportSummaryHTML(s ReportSummary) (template.HTML, error) {
	return renderHTML("fragment_summary.html.tmpl", s)
}

// CombinedReport renders a 2x2 HTML report with cycle time, throughput, longest CT, and forecast.
func CombinedReport(
	summary ReportSummary,
	cycleTimeData []metrics.CycleTimeResult,
	cycleTimePercentiles []float64,
	throughputData metrics.ThroughputResult,
	longestCTRows []LongestCycleTimeRow,
	forecastRows []ForecastRow,
	jiraBaseURL string,
	path string,
) error {
	return writeHTML(path, "report.html.tmpl", map[string]any{
		"SummaryHTML":    chartOrError(ReportSummaryHTML(summary)),
		"CycleTimeHTML":  chartOrError(CycleTimeScatterHTML(cycleTimeData, cycleTimePercentiles, "Cycle Time Distribution")),
		"ThroughputHTML": chartOrError(ThroughputLineHTML(throughputData, "Weekly Throughput")),
		"LongestCTHTML":  chartOrError(LongestCycleTimeTableHTML(longestCTRows, "Longest Cycle Times", jiraBaseURL)),
		"ForecastHTML":   chartOrError(ForecastTableHTML(forecastRows, "Epic Forecast", jiraBaseURL)),
	})
}

// CombinedTeamReport renders an HTML report combining GitHub deployment frequency,
// JIRA metrics, and Snyk vulnerability sections.
func CombinedTeamReport(
	title string,
	summary ReportSummary,
	deploymentData metrics.ThroughputResult,
	deploymentFailures metrics.ThroughputResult,
	cycleTimeData []metrics.CycleTimeResult,
	cycleTimePercentiles []float64,
	throughputData metrics.ThroughputResult,
	longestCTRows []LongestCycleTimeRow,
	forecastRows []ForecastRow,
	jiraBaseURL string,
	snykSummary SnykSummary,
	snykWeeks []SnykIssueWeek,
	path string,
) error {
	var dfHTML template.HTML
	if len(deploymentData.Periods) > 0 {
		dfHTML = chartOrError(DeploymentFrequencyLineHTML(deploymentData, deploymentFailures, "Deployment Frequency"))
	}

	var snykSummaryHTML, snykChartHTML template.HTML
	if len(snykWeeks) > 0 {
		snykSummaryHTML = chartOrError(SnykSummaryHTML(snykSummary))
		snykChartHTML = chartOrError(SnykIssuesLineHTML(snykWeeks, "Open Snyk Issues over time"))
	}

	// Build executive healthcheck from available data.
	avgDeployFreq := "—"
	lastWeekDeploys := 0
	if deploymentData.AvgCount > 0 {
		avgDeployFreq = fmt.Sprintf("%.1f/wk", deploymentData.AvgCount)
	}
	if n := len(deploymentData.Periods); n > 0 {
		lastWeekDeploys = deploymentData.Periods[n-1].Count
	}
	hc := execreport.ExecHealthcheck{
		AvgCycleTime:         summary.AvgCycleTime,
		AvgThroughput:        summary.AvgThroughput,
		ActiveEpics:          summary.ActiveEpics,
		HasJIRAData:          len(cycleTimeData) > 0 || throughputData.AvgCount > 0,
		AvgDeployFreq:        avgDeployFreq,
		LastWeekDeploys:      lastWeekDeploys,
		HasDeployData:        len(deploymentData.Periods) > 0,
		Exploitable: snykSummary.ExploitableTotal,
		Critical:    snykSummary.Critical,
		High:        snykSummary.High,
		HasSnykData:          len(snykWeeks) > 0,
	}

	return writeHTML(path, "team_report.html.tmpl", map[string]any{
		"Title":               title,
		"ExecHealthcheckHTML": chartOrError(execreport.ExecHealthcheckHTML(hc)),
		"SummaryHTML":         chartOrError(ReportSummaryHTML(summary)),
		"DeploymentHTML":      dfHTML,
		"CycleTimeHTML":       chartOrError(CycleTimeScatterHTML(cycleTimeData, cycleTimePercentiles, "Cycle Time Distribution")),
		"ThroughputHTML":      chartOrError(ThroughputLineHTML(throughputData, "Weekly Throughput")),
		"LongestCTHTML":       chartOrError(LongestCycleTimeTableHTML(longestCTRows, "Longest Cycle Times", jiraBaseURL)),
		"ForecastHTML":        chartOrError(ForecastTableHTML(forecastRows, "Epic Forecast", jiraBaseURL)),
		"SnykSummaryHTML":     snykSummaryHTML,
		"SnykChartHTML":       snykChartHTML,
		"DatadogHTML":         "",
	})
}
