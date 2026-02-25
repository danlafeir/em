package metrics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"gonum.org/v1/plot"

	"devctl-em/internal/charts"
	"devctl-em/internal/jira"
	pkgmetrics "devctl-em/internal/metrics"
	"devctl-em/internal/workflow"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate combined metrics report as a single PNG",
	Long: `Generate a single PNG report combining cycle time scatter, throughput trend,
and Monte Carlo epic forecast.

Uses the last 6 weeks of data by default. Output is a single PNG image.

Example:
  devctl-em metrics jira report
  devctl-em metrics jira report --from 2024-01-01 -o report.png`,
	RunE: runReport,
}

func init() {
	JiraCmd.AddCommand(reportCmd)
}

func runReport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getJiraClient()
	if err != nil {
		return err
	}

	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to JIRA: %w", err)
	}

	jql, err := resolveJQL(ctx, client)
	if err != nil {
		return err
	}

	from, to, err := getDateRange()
	if err != nil {
		return err
	}

	outputPath := getOutputPath("jira-report", "png")

	fmt.Printf("Generating JIRA Report...\n")
	fmt.Printf("JQL: %s\n", jql)
	fmt.Printf("Date range: %s to %s\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	// Fetch completed issues
	fmt.Printf("Fetching issues from JIRA...\n")
	jqlCompleted := fmt.Sprintf("(%s) AND resolved >= %s AND resolved <= %s",
		jql, from.Format("2006-01-02"), to.Format("2006-01-02"))

	completedIssues, err := client.FetchIssuesWithHistory(ctx, jqlCompleted, func(current, total int) {
		fmt.Printf("\rProcessing completed issues: %d/%d...", current, total)
	})
	if err != nil {
		return fmt.Errorf("failed to fetch completed issues: %w", err)
	}
	fmt.Println()
	fmt.Printf("\nFound %d completed issues\n\n", len(completedIssues))

	mapper := getWorkflowMapper()

	completedHistories := make([]workflow.IssueHistory, len(completedIssues))
	for i, issue := range completedIssues {
		completedHistories[i] = mapper.MapIssueHistory(issue)
	}

	// 1. Cycle Time
	fmt.Printf("Calculating cycle time metrics...\n")
	cycleCalc := pkgmetrics.NewCycleTimeCalculator(mapper)
	cycleResults := cycleCalc.Calculate(completedHistories)

	chartCfg := charts.DefaultConfig()
	chartCfg.Title = "Cycle Time Distribution"

	var cycleTimePlot *plot.Plot
	if len(cycleResults) > 0 {
		cycleTimePlot, err = charts.CycleTimeScatter(cycleResults, []float64{50, 85, 95}, chartCfg)
		if err != nil {
			return fmt.Errorf("failed to create cycle time chart: %w", err)
		}
	}

	// 2. Throughput
	fmt.Printf("Calculating throughput metrics...\n")
	throughputCalc := pkgmetrics.NewThroughputCalculator(pkgmetrics.FrequencyWeekly)
	throughputResult := throughputCalc.Calculate(completedHistories, from, to)

	chartCfg.Title = "Weekly Throughput"
	var throughputPlot *plot.Plot
	if len(throughputResult.Periods) > 0 {
		throughputPlot, err = charts.ThroughputLine(throughputResult, chartCfg)
		if err != nil {
			return fmt.Errorf("failed to create throughput chart: %w", err)
		}
	}

	// 3. Longest Cycle Time table
	var longestCTPlot *plot.Plot
	if len(cycleResults) > 0 {
		sorted := make([]pkgmetrics.CycleTimeResult, len(cycleResults))
		copy(sorted, cycleResults)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].CycleTime > sorted[j].CycleTime
		})
		n := len(sorted)
		if n > 10 {
			n = 10
		}
		var ctRows []charts.LongestCycleTimeRow
		for _, r := range sorted[:n] {
			ctRows = append(ctRows, charts.LongestCycleTimeRow{
				Key:       r.IssueKey,
				Summary:   r.Summary,
				Days:      fmt.Sprintf("%.1f", r.CycleTimeDays()),
				Started:   r.StartDate.Format("Jan 02"),
				Completed: r.EndDate.Format("Jan 02"),
			})
		}
		longestCTPlot = charts.LongestCycleTimeTable(ctRows, fmt.Sprintf("Longest Cycle Times — %s to %s", from.Format("Jan 02"), to.Format("Jan 02")))
	}

	// 4. Forecast table
	var forecastPlot *plot.Plot
	if len(throughputResult.Periods) > 0 {
		weeklyThroughput := pkgmetrics.GetWeeklyThroughputValues(throughputResult)

		if len(weeklyThroughput) > 0 {
			rows, forecastErr := discoverAndForecastEpics(ctx, client, mapper, weeklyThroughput)
			if forecastErr != nil {
				fmt.Printf("Warning: forecast unavailable: %v\n", forecastErr)
			} else if len(rows) > 0 {
				forecastPlot = charts.ForecastTable(rows)
			}
		}
	}

	if err := charts.CombinedReport(cycleTimePlot, throughputPlot, longestCTPlot, forecastPlot, outputPath); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("\nReport generated: %s\n", outputPath)
	return nil
}

// discoverAndForecastEpics finds open epics and runs Monte Carlo forecasts.
func discoverAndForecastEpics(ctx context.Context, client *jira.Client, mapper *workflow.Mapper, weeklyThroughput []int) ([]charts.ForecastRow, error) {
	projectJQL, err := getProjectJQL()
	if err != nil {
		return nil, err
	}

	epicJQL := fmt.Sprintf("(%s) AND issuetype = Epic AND resolution IS EMPTY ORDER BY key", projectJQL)
	epics, err := client.SearchAllIssues(ctx, epicJQL, "summary,status", "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch epics: %w", err)
	}

	if len(epics) == 0 {
		return nil, nil
	}

	fmt.Printf("Found %d open epics, forecasting...\n", len(epics))

	var rows []charts.ForecastRow
	for _, epic := range epics {
		row := forecastEpicRow(ctx, client, mapper, epic, weeklyThroughput)
		if row != nil {
			rows = append(rows, *row)
		}
	}

	return rows, nil
}

func forecastEpicRow(ctx context.Context, client *jira.Client, mapper *workflow.Mapper, epic jira.Issue, weeklyThroughput []int) *charts.ForecastRow {
	jql := fmt.Sprintf("\"Epic Link\" = %s OR parent = %s", epic.Key, epic.Key)
	issues, err := client.SearchAllIssues(ctx, jql, "status,summary,issuetype", "")
	if err != nil {
		return nil
	}

	var remaining int
	for _, issue := range issues {
		if !mapper.IsCompleted(issue.Fields.Status.Name) {
			remaining++
		}
	}

	if remaining == 0 {
		return nil
	}

	config := pkgmetrics.MonteCarloConfig{
		Trials:          10000,
		SimulationStart: time.Now(),
	}

	simulator := pkgmetrics.NewMonteCarloSimulator(config, weeklyThroughput)
	result, err := simulator.Run(remaining)
	if err != nil {
		return nil
	}

	return &charts.ForecastRow{
		EpicKey:    epic.Key,
		Summary:    epic.Fields.Summary,
		Remaining:  remaining,
		Forecast50: result.Percentiles[50].Format("Jan 02"),
		Forecast85: result.Percentiles[85].Format("Jan 02"),
		Forecast95: result.Percentiles[95].Format("Jan 02"),
	}
}
