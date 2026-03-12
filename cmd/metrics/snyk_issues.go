package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/internal/charts"
	"devctl-em/internal/output"
	"devctl-em/internal/snyk"
)

var snykIssuesCmd = &cobra.Command{
	Use:   "issues",
	Short: "Vulnerability counts and weekly trends",
	Long: `Show aggregate vulnerability counts by severity and generate a weekly trend chart.

Examples:
  devctl-em metrics snyk issues
  devctl-em metrics snyk issues --from 2025-01-01 --to 2025-06-30
  devctl-em metrics snyk issues -f csv -o issues.csv`,
	RunE: runSnykIssues,
}

func init() {
	SnykCmd.AddCommand(snykIssuesCmd)
}

func runSnykIssues(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("Fetching issues (%s to %s)...\n",
		from.Format("2006-01-02"), to.Format("2006-01-02"))

	issues, err := client.ListIssues(ctx, from, to)
	if err != nil {
		return fmt.Errorf("failed to list issues: %w", err)
	}

	if len(issues) == 0 {
		fmt.Println("\nNo issues found in the specified date range.")
		return nil
	}

	// CSV export
	if getSnykOutputFormat("table") == "csv" {
		outputPath := getSnykOutputPath("snyk-issues", "csv")
		if err := exportSnykIssuesCSV(issues, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("Exported to %s\n", outputPath)
		return nil
	}

	// Print severity summary
	counts := countBySeverity(issues)
	total := counts["critical"] + counts["high"] + counts["medium"] + counts["low"]

	fmt.Printf("\nVulnerability Summary (%d total)\n", total)
	fmt.Printf("================================\n\n")
	fmt.Printf("| %-10s | %5s |\n", "Severity", "Count")
	fmt.Printf("|%s|%s|\n", strings.Repeat("-", 12), strings.Repeat("-", 7))
	fmt.Printf("| %-10s | %5d |\n", "Critical", counts["critical"])
	fmt.Printf("| %-10s | %5d |\n", "High", counts["high"])
	fmt.Printf("| %-10s | %5d |\n", "Medium", counts["medium"])
	fmt.Printf("| %-10s | %5d |\n", "Low", counts["low"])
	fmt.Printf("|%s|%s|\n", strings.Repeat("-", 12), strings.Repeat("-", 7))
	fmt.Printf("| %-10s | %5d |\n", "Total", total)

	// Generate weekly trend chart
	weeks := bucketByWeek(issues, from, to)
	if len(weeks) > 0 {
		cfg := charts.Config{Title: "Snyk Issues — Weekly Trend"}
		chartPath := getSnykOutputPath("snyk-issues", "html")
		if err := charts.SnykIssuesLine(weeks, cfg, chartPath); err != nil {
			return fmt.Errorf("failed to create chart: %w", err)
		}
		fmt.Printf("\nChart saved to %s\n", chartPath)
		charts.OpenBrowser(chartPath)
	}

	return nil
}

func countBySeverity(issues []snyk.Issue) map[string]int {
	counts := map[string]int{
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
	}
	for _, issue := range issues {
		sev := strings.ToLower(issue.Severity)
		if _, ok := counts[sev]; ok {
			counts[sev]++
		}
	}
	return counts
}

// bucketByWeek groups issues by ISO week and severity.
func bucketByWeek(issues []snyk.Issue, from, to time.Time) []charts.SnykIssueWeek {
	type weekKey struct {
		year int
		week int
	}

	buckets := map[weekKey]*charts.SnykIssueWeek{}

	// Initialize all weeks in range
	current := from.Truncate(24 * time.Hour)
	// Align to Monday
	for current.Weekday() != time.Monday {
		current = current.AddDate(0, 0, -1)
	}
	for !current.After(to) {
		y, w := current.ISOWeek()
		k := weekKey{y, w}
		if _, ok := buckets[k]; !ok {
			buckets[k] = &charts.SnykIssueWeek{WeekStart: current}
		}
		current = current.AddDate(0, 0, 7)
	}

	// Bucket issues
	for _, issue := range issues {
		y, w := issue.CreatedAt.ISOWeek()
		k := weekKey{y, w}
		bucket, ok := buckets[k]
		if !ok {
			// Find Monday of this week
			ws := issue.CreatedAt.Truncate(24 * time.Hour)
			for ws.Weekday() != time.Monday {
				ws = ws.AddDate(0, 0, -1)
			}
			bucket = &charts.SnykIssueWeek{WeekStart: ws}
			buckets[k] = bucket
		}
		switch strings.ToLower(issue.Severity) {
		case "critical":
			bucket.Critical++
		case "high":
			bucket.High++
		case "medium":
			bucket.Medium++
		case "low":
			bucket.Low++
		}
	}

	// Sort by week start
	weeks := make([]charts.SnykIssueWeek, 0, len(buckets))
	for _, w := range buckets {
		weeks = append(weeks, *w)
	}
	sort.Slice(weeks, func(i, j int) bool {
		return weeks[i].WeekStart.Before(weeks[j].WeekStart)
	})

	return weeks
}

func exportSnykIssuesCSV(issues []snyk.Issue, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Date", "Severity", "Type", "Status", "Title"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, issue := range issues {
		row := []string{
			issue.CreatedAt.Format("2006-01-02"),
			issue.Severity,
			issue.IssueType,
			issue.Status,
			issue.Title,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
