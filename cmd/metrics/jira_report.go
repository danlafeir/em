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

// generateReport generates a single combined HTML report for the given JQL.
func generateReport(ctx context.Context, client *jira.Client, team, jql string, from, to time.Time) error {
	outputPath := getOutputPath(teamOutputName("jira-report", team), "html")

	fmt.Printf("Generating JIRA Report...\n")
	fmt.Printf("JQL: %s\n", jql)
	fmt.Printf("Date range: %s to %s\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	// Normalize from to the start of its ISO week so the first throughput
	// bucket is always a full 7-day period and its JQL fetch is consistent.
	fromWeek := pkgmetrics.WeekStart(from)

	// Fetch completed issues
	fmt.Printf("Fetching issues from JIRA...\n")
	jqlCompleted := jqlWithDateRange(
		fmt.Sprintf("(%s) AND issuetype = Story", jql),
		fromWeek.Format("2006-01-02"), to.Format("2006-01-02"),
	)

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

	// Filter outliers from scatter chart (still shown in longest CT table)
	keptResults, outlierResults := pkgmetrics.FilterCycleTimeOutliers(cycleResults, 2.0)

	// 2. Throughput
	fmt.Printf("Calculating throughput metrics...\n")
	throughputCalc := pkgmetrics.NewThroughputCalculator(pkgmetrics.FrequencyWeekly)
	throughputResult := throughputCalc.Calculate(completedHistories, fromWeek, to)

	// 3. Longest Cycle Time table — combine kept + outliers, mark outliers
	var ctRows []charts.LongestCycleTimeRow
	if len(cycleResults) > 0 {
		outlierKeys := make(map[string]bool, len(outlierResults))
		for _, r := range outlierResults {
			outlierKeys[r.IssueKey] = true
		}

		sorted := make([]pkgmetrics.CycleTimeResult, len(cycleResults))
		copy(sorted, cycleResults)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].CycleTime > sorted[j].CycleTime
		})
		n := len(sorted)
		if n > 10 {
			n = 10
		}
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

	// 4. Forecast table — use 90-day throughput window for Monte Carlo
	var forecastRows []charts.ForecastRow
	{
		const forecastHistoryDays = 90
		forecastFrom := pkgmetrics.WeekStart(time.Now().AddDate(0, 0, -forecastHistoryDays))

		var forecastThroughput []int
		if !fromWeek.After(forecastFrom) {
			forecastThroughput = pkgmetrics.GetWeeklyThroughputValues(throughputResult)
		} else {
			forecastJQL := jqlWithDateRange(jql, forecastFrom.Format("2006-01-02"), time.Now().Format("2006-01-02"))

			fmt.Printf("Fetching 90-day throughput history for forecast...\n")
			forecastIssues, fetchErr := client.FetchIssuesWithHistory(ctx, forecastJQL, func(current, total int) {
				fmt.Printf("\rProcessing forecast issues: %d/%d...", current, total)
			})
			if fetchErr != nil {
				fmt.Printf("Warning: forecast unavailable: %v\n", fetchErr)
			} else {
				fmt.Println()
				forecastHistories := make([]workflow.IssueHistory, len(forecastIssues))
				for i, issue := range forecastIssues {
					forecastHistories[i] = mapper.MapIssueHistory(issue)
				}
				fc := pkgmetrics.NewThroughputCalculator(pkgmetrics.FrequencyWeekly)
				fcResult := fc.Calculate(forecastHistories, forecastFrom, time.Now())
				forecastThroughput = pkgmetrics.GetWeeklyThroughputValues(fcResult)
			}
		}

		forecastThroughput = pkgmetrics.FilterOutliers(forecastThroughput, 2.0)

		if len(forecastThroughput) > 0 {
			epics, epicErr := fetchOpenEpics(ctx, client, team)
			if epicErr != nil {
				fmt.Printf("Warning: forecast unavailable: %v\n", epicErr)
			} else if len(epics) > 0 {
				if selectEpicsFlag {
					if selected, selErr := promptEpicSelection(epics); selErr != nil {
						fmt.Printf("Warning: epic selection skipped: %v\n", selErr)
					} else {
						saveEpicSelection(team, selected)
						epics = selected
					}
				} else {
					epics = applyEpicSelection(epics, team)
				}
				fmt.Printf("Found %d open epics, forecasting...\n", len(epics))
				for _, epic := range epics {
					f := forecastEpic(ctx, client, mapper, epic, forecastThroughput)
					if f.RemainingItems == 0 || f.Error != "" {
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

	if err := charts.CombinedReport(keptResults, []float64{50, 85, 95}, throughputResult, ctRows, forecastRows, outputPath); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("\nReport generated: %s\n", outputPath)
	charts.OpenBrowser(outputPath)
	return nil
}

