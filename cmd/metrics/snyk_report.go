package metrics

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"devctl-em/internal/charts"
)

var snykReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a Snyk security metrics report as a single HTML page",
	Long: `Generate an HTML report showing aggregate vulnerability counts and weekly trend.

Uses the last 6 weeks of data by default. Output is an HTML file.

Examples:
  devctl-em metrics snyk report
  devctl-em metrics snyk report --from 2025-01-01 --to 2025-06-30
  devctl-em metrics snyk report -o snyk-report.html`,
	RunE: runSnykReport,
}

func init() {
	SnykCmd.AddCommand(snykReportCmd)
}

func runSnykReport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getSnykClient()
	if err != nil {
		return err
	}

	fmt.Println("Testing Snyk connection...")
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to Snyk: %w", err)
	}

	from, to, err := getSnykDateRange()
	if err != nil {
		return err
	}

	fmt.Printf("Fetching issues (%s to %s)...\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	issues, err := client.ListIssues(ctx, from, to)
	if err != nil {
		return fmt.Errorf("failed to list issues: %w", err)
	}

	counts := countBySeverity(issues)
	summary := charts.SnykSummary{
		Critical: counts["critical"],
		High:     counts["high"],
		Medium:   counts["medium"],
		Low:      counts["low"],
	}

	weeks := bucketByWeek(issues, from, to)

	outputPath := getSnykOutputPath("snyk-report", "html")

	orgName := getConfigString("snyk.org_name")
	title := "Snyk Security Report"
	if orgName != "" {
		title = orgName + " — Snyk Security Report"
	}

	if err := charts.SnykReport(summary, weeks, title, outputPath); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("\nReport generated: %s\n", outputPath)
	charts.OpenBrowser(outputPath)
	return nil
}
