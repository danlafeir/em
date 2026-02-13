package metrics

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/internal/charts"
	"devctl-em/internal/export"
	pkgmetrics "devctl-em/internal/metrics"
	"devctl-em/internal/workflow"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate comprehensive metrics report",
	Long: `Generate a comprehensive HTML report with all JIRA agile metrics.

Includes:
  - Cycle time analysis with scatter plot
  - Throughput trends
  - Work-in-progress analysis
  - Monte Carlo forecast (if applicable)

Example:
  devctl-em metrics jira report --jql "project = MYPROJ" -o report.html
  devctl-em metrics jira report --jql "project = MYPROJ" --from 2024-01-01`,
	RunE: runReport,
}

var (
	reportTitleFlag string
)

func init() {
	JiraCmd.AddCommand(reportCmd)

	reportCmd.Flags().StringVar(&reportTitleFlag, "title", "", "Report title")
}

func runReport(cmd *cobra.Command, args []string) error {
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

	// Determine output path
	outputPath := getOutputPath("jira-metrics-report", "html")
	outputDir := filepath.Dir(outputPath)

	fmt.Printf("Generating comprehensive metrics report...\n")
	fmt.Printf("JQL: %s\n", jql)
	fmt.Printf("Date range: %s to %s\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	// Fetch all issues (both completed and open)
	fmt.Printf("Fetching issues from JIRA...\n")

	// Completed issues for cycle time and throughput
	jqlCompleted := fmt.Sprintf("(%s) AND resolved >= %s AND resolved <= %s",
		jql, from.Format("2006-01-02"), to.Format("2006-01-02"))

	completedIssues, err := client.FetchIssuesWithHistory(ctx, jqlCompleted, func(current, total int) {
		fmt.Printf("\rProcessing completed issues: %d/%d...", current, total)
	})
	if err != nil {
		return fmt.Errorf("failed to fetch completed issues: %w", err)
	}
	fmt.Println()

	// Open issues for WIP
	jqlOpen := fmt.Sprintf("(%s) AND resolution IS EMPTY", jql)
	openIssues, err := client.FetchIssuesWithHistory(ctx, jqlOpen, func(current, total int) {
		fmt.Printf("\rProcessing open issues: %d/%d...", current, total)
	})
	if err != nil {
		return fmt.Errorf("failed to fetch open issues: %w", err)
	}
	fmt.Println()

	fmt.Printf("\nFound %d completed issues and %d open issues\n\n", len(completedIssues), len(openIssues))

	// Get workflow mapper
	mapper := getWorkflowMapper()

	// Map issues to workflow history
	completedHistories := make([]workflow.IssueHistory, len(completedIssues))
	for i, issue := range completedIssues {
		completedHistories[i] = mapper.MapIssueHistory(issue)
	}

	openHistories := make([]workflow.IssueHistory, len(openIssues))
	for i, issue := range openIssues {
		openHistories[i] = mapper.MapIssueHistory(issue)
	}

	// Prepare report sections
	var sections []export.HTMLSection

	// 1. Cycle Time Analysis
	fmt.Printf("Calculating cycle time metrics...\n")
	cycleCalc := pkgmetrics.NewCycleTimeCalculator(mapper)
	cycleResults := cycleCalc.Calculate(completedHistories)

	if len(cycleResults) > 0 {
		stats := pkgmetrics.CalculateStats(cycleResults)

		// Generate scatter chart
		chartPath := filepath.Join(outputDir, "cycle-time-scatter.png")
		chartCfg := charts.DefaultConfig()
		chartCfg.Title = "Cycle Time Distribution"

		plot, err := charts.CycleTimeScatter(cycleResults, []float64{50, 85, 95}, chartCfg)
		if err == nil {
			if err := charts.SaveChart(plot, chartPath, chartCfg); err == nil {
				fmt.Printf("  Generated: %s\n", chartPath)
			}
		}

		sections = append(sections, export.HTMLSection{
			Title:   "Cycle Time Analysis",
			Content: export.FormatStatsHTML(stats),
		})
	}

	// 2. Throughput Analysis
	fmt.Printf("Calculating throughput metrics...\n")
	throughputCalc := pkgmetrics.NewThroughputCalculator(pkgmetrics.FrequencyWeekly)
	throughputResult := throughputCalc.Calculate(completedHistories, from, to)

	if len(throughputResult.Periods) > 0 {
		// Generate throughput chart
		chartPath := filepath.Join(outputDir, "throughput-trend.png")
		chartCfg := charts.DefaultConfig()
		chartCfg.Title = "Weekly Throughput"

		plot, err := charts.ThroughputLine(throughputResult, chartCfg)
		if err == nil {
			if err := charts.SaveChart(plot, chartPath, chartCfg); err == nil {
				fmt.Printf("  Generated: %s\n", chartPath)
			}
		}

		sections = append(sections, export.HTMLSection{
			Title:   "Throughput Trend",
			Content: export.FormatThroughputHTML(throughputResult),
		})
	}

	// 3. WIP Analysis
	fmt.Printf("Calculating WIP metrics...\n")
	wipCalc := pkgmetrics.NewWIPCalculator(mapper)
	wipResult := wipCalc.Calculate(openHistories, from, to)

	if len(wipResult.CurrentWIP) > 0 {
		thresholds := pkgmetrics.DefaultAgingThresholds()
		healthy, warning, critical := pkgmetrics.CategorizeByAge(wipResult.CurrentWIP, thresholds)

		wipHTML := fmt.Sprintf(`
            <div class="stat-grid">
                <div class="stat-card">
                    <div class="stat-value">%d</div>
                    <div class="stat-label">Total WIP</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value" style="color: #28a745">%d</div>
                    <div class="stat-label">Healthy</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value" style="color: #ffc107">%d</div>
                    <div class="stat-label">Warning</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value" style="color: #dc3545">%d</div>
                    <div class="stat-label">Critical</div>
                </div>
            </div>
        `, len(wipResult.CurrentWIP), len(healthy), len(warning), len(critical))

		// Add critical items table if any
		if len(critical) > 0 {
			wipHTML += `<h3>Critical Aging Items</h3><table><tr><th>Issue</th><th>Stage</th><th>Age (days)</th></tr>`
			for _, item := range critical {
				ageDays := int(item.Age.Hours() / 24)
				wipHTML += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%d</td></tr>`,
					item.Key, item.Stage, ageDays)
			}
			wipHTML += `</table>`
		}

		sections = append(sections, export.HTMLSection{
			Title:   "Work-In-Progress",
			Content: wipHTML,
		})
	}

	// 4. Forecast (if there's remaining work)
	if len(openHistories) > 0 && len(throughputResult.Periods) > 0 {
		fmt.Printf("Running Monte Carlo forecast...\n")

		weeklyThroughput := pkgmetrics.GetWeeklyThroughputValues(throughputResult)

		config := pkgmetrics.MonteCarloConfig{
			Trials:          10000,
			SimulationStart: time.Now(),
		}

		simulator := pkgmetrics.NewMonteCarloSimulator(config, weeklyThroughput)
		forecast, err := simulator.Run(len(openHistories))

		if err == nil {
			sections = append(sections, export.HTMLSection{
				Title:   "Completion Forecast",
				Content: export.FormatForecastHTML(forecast),
			})
		}
	}

	// Generate HTML report
	title := reportTitleFlag
	if title == "" {
		title = "JIRA Agile Metrics Report"
	}

	fmt.Printf("\nGenerating HTML report...\n")
	if err := export.HTMLReport(title, sections, outputPath); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("\nReport generated: %s\n", outputPath)

	// Also export CSV files
	if len(cycleResults) > 0 {
		csvPath := filepath.Join(outputDir, "cycle-time-data.csv")
		if err := export.CycleTimeCSV(cycleResults, csvPath); err == nil {
			fmt.Printf("Exported: %s\n", csvPath)
		}
	}

	if len(throughputResult.Periods) > 0 {
		csvPath := filepath.Join(outputDir, "throughput-data.csv")
		if err := export.ThroughputCSV(throughputResult, csvPath); err == nil {
			fmt.Printf("Exported: %s\n", csvPath)
		}
	}

	return nil
}
