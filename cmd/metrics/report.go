package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/internal/charts"
	pkgmetrics "devctl-em/internal/metrics"
)

var metricsReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a combined engineering metrics report",
	Long: `Generate a combined report across all configured data sources.

Runs GitHub deployment frequency and JIRA metrics reports in sequence.
Each section runs independently — a failure in one does not stop the other.
Also generates a combined <team>-report.html with both sections.

Example:
  devctl-em metrics report
  devctl-em metrics report --team platform`,
	RunE: runMetricsReport,
}

func init() {
	MetricsCmd.AddCommand(metricsReportCmd)
}

const sectionDivider = "────────────────────────────────────────"

func runMetricsReport(cmd *cobra.Command, args []string) error {
	skipBrowserOpen = true
	defer func() { skipBrowserOpen = false }()

	if err := runDeploymentFrequency(cmd, args); err != nil {
		fmt.Printf("Warning: GitHub report skipped: %v\n", err)
	}

	fmt.Println()
	fmt.Println(sectionDivider)
	fmt.Println()
	if err := runReport(cmd, args); err != nil {
		fmt.Printf("Warning: JIRA report skipped: %v\n", err)
	}

	fmt.Println()
	fmt.Println(sectionDivider)
	fmt.Println()
	skipBrowserOpen = false
	if err := generateCombinedTeamReport(); err != nil {
		fmt.Printf("Warning: combined report skipped: %v\n", err)
	}

	return nil
}

// generateCombinedTeamReport fetches JIRA and GitHub data for the selected team
// and writes a combined <team>-report.html.
func generateCombinedTeamReport() error {
	ctx := context.Background()
	team := getSelectedTeam()

	from, to, err := getDateRange()
	if err != nil {
		return err
	}

	jiraData, err := fetchJIRADataForReport(ctx, team, from, to)
	if err != nil {
		return err
	}

	deployments := fetchGitHubDeploymentsForReport(ctx, team, from, to)

	title := "Engineering Report"
	if team != "" {
		title = team + " — Engineering Report"
	}
	outputPath := getOutputPath(teamOutputName("report", team), "html")

	if err := charts.CombinedTeamReport(
		title,
		jiraData.Summary,
		deployments,
		jiraData.KeptResults,
		[]float64{50, 85, 95},
		jiraData.ThroughputResult,
		jiraData.LongestCTRows,
		jiraData.ForecastRows,
		jiraData.BaseURL,
		outputPath,
	); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	fmt.Printf("Combined report: %s\n", outputPath)
	openBrowser(outputPath)
	return nil
}

func fetchJIRADataForReport(ctx context.Context, team string, from, to time.Time) (jiraMetricsData, error) {
	client, err := getJiraClient()
	if err != nil {
		return jiraMetricsData{}, fmt.Errorf("JIRA: %w", err)
	}
	if err := client.TestConnection(ctx); err != nil {
		return jiraMetricsData{}, fmt.Errorf("JIRA connection: %w", err)
	}

	var jql string
	if jqlFlag != "" {
		jql = jqlFlag
	} else {
		jql, err = resolveTeamJQL(ctx, client, team)
		if err != nil {
			return jiraMetricsData{}, fmt.Errorf("JIRA JQL: %w", err)
		}
	}

	fmt.Printf("Generating combined team report...\n")
	return collectJIRAMetricsData(ctx, client, team, jql, from, to, false)
}

func fetchGitHubDeploymentsForReport(ctx context.Context, team string, from, to time.Time) pkgmetrics.ThroughputResult {
	if team == "" {
		return pkgmetrics.ThroughputResult{}
	}
	client, err := getGithubClient()
	if err != nil {
		return pkgmetrics.ThroughputResult{}
	}
	org := getGithubOrg()
	if org == "" {
		return pkgmetrics.ThroughputResult{}
	}
	return fetchTeamDeploymentData(ctx, client, org, team, from, to)
}

