package metrics

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"em/internal/charts"
	snykpkg "em/internal/snyk"
)

var snykReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a Snyk security metrics report as a single HTML page",
	Long: `Generate an HTML report of vulnerability counts and weekly trend.

Required:
  em metrics snyk config`,
	RunE: runSnykReport,
}

func init() {
	SnykCmd.AddCommand(snykReportCmd)
}

func runSnykReport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	from, to, err := getSnykDateRange()
	if err != nil {
		return err
	}

	var client *snykpkg.Client
	if !useSavedDataFlag {
		client, err = getSnykClient()
		if err != nil {
			return err
		}
		fmt.Println("Testing Snyk connection...")
		if err := client.TestConnection(ctx); err != nil {
			return fmt.Errorf("failed to connect to Snyk: %w", err)
		}
		fmt.Printf("Fetching issues (%s to %s)...\n", from.Format("2006-01-02"), to.Format("2006-01-02"))
	}

	issues, resolved, openCounts, err := fetchOrLoadSnykData(ctx, client, from, to)
	if err != nil {
		return fmt.Errorf("failed to fetch Snyk data: %w", err)
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
		ExploitableFixable:  openCounts.ExploitableFixable,
		ExploitableTotal:    openCounts.ExploitableCritical + openCounts.ExploitableHigh + openCounts.ExploitableMedium + openCounts.ExploitableLow,
	}

	weeks := bucketByWeek(issues, resolved, openCounts.Total, openCounts.Fixable, openCounts.IgnoredFixable, openCounts.IgnoredUnfixable, from, to)

	outputPath := getSnykOutputPath("snyk-report", "html")

	orgName := getConfigString("snyk.org_name")
	title := "Snyk Security Report"
	if team := getSelectedTeam(); team != "" {
		title = team + " — Snyk Security Report"
	} else if orgName != "" {
		title = orgName + " — Snyk Security Report"
	}

	if err := charts.SnykReport(summary, weeks, title, outputPath); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("\nReport generated: %s\n", outputPath)
	openBrowser(outputPath)
	return nil
}
