package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"em/internal/charts"
	pkgmetrics "em/internal/metrics"
	snykpkg "em/internal/snyk"
)

var metricsReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a combined engineering metrics report",
	Long: `Generate a combined report across all configured data sources.

Required:
  At least one source configured (run "em metrics config")`,
	RunE: runMetricsReport,
}

func init() {
	MetricsCmd.AddCommand(metricsReportCmd)
}

const sectionDivider = "────────────────────────────────────────"

func runMetricsReport(cmd *cobra.Command, args []string) error {
	skipBrowserOpen = true
	defer func() { skipBrowserOpen = false }()

	jiraOK := isJiraConfigured()
	githubOK := isGithubConfigured()
	snykOK := isSnykConfigured()
	datadogOK := isDatadogConfigured()

	var unconfigured []string
	if !jiraOK {
		unconfigured = append(unconfigured, "JIRA")
	}
	if !githubOK {
		unconfigured = append(unconfigured, "GitHub")
	}
	if !snykOK {
		unconfigured = append(unconfigured, "Snyk")
	}
	if len(unconfigured) > 0 {
		fmt.Printf("Skipping unconfigured: %s\n", strings.Join(unconfigured, ", "))
		fmt.Println("Run `em metrics config` to set them up.")
		fmt.Println()
	}

	first := true
	sep := func() {
		if !first {
			fmt.Println()
			fmt.Println(sectionDivider)
			fmt.Println()
		}
		first = false
	}

	if githubOK {
		sep()
		if err := runDeploymentFrequency(cmd, args); err != nil {
			fmt.Printf("Warning: GitHub report failed: %v\n", err)
		}
	}

	// Fetch JIRA data once and reuse it for both the standalone report and the
	// combined team report, so the Monte Carlo simulation only runs once.
	var cachedJIRAData *jiraMetricsData
	if jiraOK {
		sep()
		ctx := context.Background()
		team := getSelectedTeam()
		from, to, err := getDateRange()
		if err != nil {
			fmt.Printf("Warning: JIRA report failed: %v\n", err)
		} else {
			fmt.Println("JIRA Metrics")
			fmt.Println(sectionDivider)
			fmt.Println()
			data, err := fetchJIRADataForReport(ctx, team, from, to)
			if err != nil {
				fmt.Printf("Warning: JIRA report failed: %v\n", err)
			} else {
				cachedJIRAData = &data
				if err := renderJIRAReport(team, data); err != nil {
					fmt.Printf("Warning: JIRA report failed: %v\n", err)
				}
			}
		}
	}

	if snykOK {
		sep()
		if err := runSnykReport(cmd, args); err != nil {
			fmt.Printf("Warning: Snyk report failed: %v\n", err)
		}
	}

	if jiraOK || githubOK || snykOK || datadogOK {
		sep()
		skipBrowserOpen = false
		if err := generateCombinedTeamReport(cachedJIRAData); err != nil {
			fmt.Printf("Warning: combined report skipped: %v\n", err)
		}
	}

	return nil
}

// isJiraConfigured returns true if the minimum JIRA credentials are present.
func isJiraConfigured() bool {
	_, err := getJiraClient()
	return err == nil
}

// isGithubConfigured returns true if a GitHub token and org are present.
func isGithubConfigured() bool {
	_, err := getGithubClient()
	return err == nil && getGithubOrg() != ""
}

// isSnykConfigured returns true if the minimum Snyk credentials are present.
func isSnykConfigured() bool {
	_, err := getSnykClient()
	return err == nil
}

// isDatadogConfigured returns true if Datadog API/app keys and a team are present.
func isDatadogConfigured() bool {
	_, err := getDatadogClient()
	return err == nil && getDatadogTeam() != ""
}

// generateCombinedTeamReport fetches JIRA and GitHub data for the selected team
// and writes a combined <team>-report.html.
// If cachedJIRAData is non-nil it is used as-is, skipping the JIRA fetch.
func generateCombinedTeamReport(cachedJIRAData *jiraMetricsData) error {
	ctx := context.Background()
	team := getSelectedTeam()

	from, to, err := getDateRange()
	if err != nil {
		return err
	}

	var jiraData jiraMetricsData
	if cachedJIRAData != nil {
		jiraData = *cachedJIRAData
	} else {
		jiraData, err = fetchJIRADataForReport(ctx, team, from, to)
		if err != nil {
			return err
		}
	}

	deployments := fetchGitHubDeploymentsForReport(ctx, team, from, to)

	snykSummary, snykWeeks := fetchSnykDataForReport(ctx, from, to)
	sloSections := fetchDatadogSLOsForReport(ctx, from, to)

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
		snykSummary,
		snykWeeks,
		sloSections,
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
		jql, err = resolveTeamJQL(team)
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
	if useSavedDataFlag {
		result, err := loadDeploymentData(team)
		if err != nil {
			return pkgmetrics.ThroughputResult{}
		}
		return result
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

// fetchSnykDataForReport fetches Snyk open counts and weekly trend for the combined report.
// Returns zero values silently if Snyk is not configured or data is unavailable.
func fetchSnykDataForReport(ctx context.Context, from, to time.Time) (charts.SnykSummary, []charts.SnykIssueWeek) {
	var snykCl *snykpkg.Client
	if !useSavedDataFlag {
		var err error
		snykCl, err = getSnykClient()
		if err != nil {
			return charts.SnykSummary{}, nil
		}
		if err := snykCl.TestConnection(ctx); err != nil {
			return charts.SnykSummary{}, nil
		}
	}
	issues, resolved, openCounts, err := fetchOrLoadSnykData(ctx, snykCl, from, to)
	if err != nil {
		return charts.SnykSummary{}, nil
	}
	summary := charts.SnykSummary{
		Critical:            openCounts.Critical,
		High:                openCounts.High,
		Medium:              openCounts.Medium,
		Low:                 openCounts.Low,
		Fixable:             openCounts.Fixable,
		Unfixable:           openCounts.Unfixable,
		IgnoredFixable:      openCounts.IgnoredFixable,
		IgnoredUnfixable:    openCounts.IgnoredUnfixable,
		ExploitableCritical: openCounts.ExploitableCritical,
		ExploitableHigh:     openCounts.ExploitableHigh,
		ExploitableMedium:   openCounts.ExploitableMedium,
		ExploitableLow:      openCounts.ExploitableLow,
		ExploitableFixable:          openCounts.ExploitableFixable,
		ExploitableUnfixable:        openCounts.ExploitableUnfixable,
		ExploitableIgnoredFixable:   openCounts.ExploitableIgnoredFixable,
		ExploitableIgnoredUnfixable: openCounts.ExploitableIgnoredUnfixable,
		ExploitableTotal:    openCounts.ExploitableCritical + openCounts.ExploitableHigh + openCounts.ExploitableMedium + openCounts.ExploitableLow,
	}
	weeks := bucketByWeek(issues, resolved, openCounts.Total, openCounts.Fixable, openCounts.IgnoredFixable, openCounts.IgnoredUnfixable, from, to)
	return summary, weeks
}

// fetchDatadogSLOsForReport fetches SLO data for the selected team and returns widget sections.
// Returns nil silently if Datadog is not configured or data is unavailable.
func fetchDatadogSLOsForReport(ctx context.Context, from, to time.Time) []charts.WidgetSection {
	client, err := getDatadogClient()
	if err != nil {
		return nil
	}
	team := getDatadogTeam()
	if team == "" {
		return nil
	}
	if err := client.TestConnection(ctx); err != nil {
		return nil
	}
	// Default SLO window to last 14 days when no explicit flag was set.
	sloFrom := from
	if ddFromFlag == "" {
		sloFrom = time.Now().AddDate(0, 0, -14)
	}
	results, eventCountByID, err := fetchSLORawData(ctx, client, team, sloFrom, to, false)
	if err != nil || len(results) == 0 {
		return nil
	}
	return sloResultsToWidgetSections(results, eventCountByID)
}
