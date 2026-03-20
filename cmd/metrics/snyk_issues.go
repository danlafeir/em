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

	resolved, err := client.ListResolvedIssues(ctx, from, to)
	if err != nil {
		return fmt.Errorf("failed to list resolved issues: %w", err)
	}

	fmt.Println("Counting total open issues...")
	openCounts, err := client.CountOpenIssues(ctx)
	if err != nil {
		return fmt.Errorf("failed to count open issues: %w", err)
	}

	if openCounts.Total == 0 && len(issues) == 0 {
		fmt.Println("\nNo issues found.")
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
	weeks := bucketByWeek(issues, resolved, openCounts.Total, openCounts.Fixable, from, to)
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

// weekDelta holds the net change in open issues for a single week.
type weekDelta struct {
	WeekStart   time.Time
	Net         int // created - resolved
	FixableNet  int // fixable created - fixable resolved
}

// bucketByWeek computes the true total of open vulnerabilities at each week's end.
// It anchors on currentOpen (the live count right now) and walks backwards through
// the weeks, reversing each week's net change to reconstruct the historical total.
func bucketByWeek(issues []snyk.Issue, resolved []snyk.Issue, currentOpen, currentFixable int, from, to time.Time) []charts.SnykIssueWeek {
	type weekKey struct {
		year int
		week int
	}

	deltas := map[weekKey]*weekDelta{}

	// Initialize all weeks in range aligned to Monday
	current := from.Truncate(24 * time.Hour)
	for current.Weekday() != time.Monday {
		current = current.AddDate(0, 0, -1)
	}
	for !current.After(to) {
		y, w := current.ISOWeek()
		deltas[weekKey{y, w}] = &weekDelta{WeekStart: current}
		current = current.AddDate(0, 0, 7)
	}

	weekMonday := func(t time.Time) time.Time {
		d := t.Truncate(24 * time.Hour)
		for d.Weekday() != time.Monday {
			d = d.AddDate(0, 0, -1)
		}
		return d
	}

	getOrCreate := func(t time.Time) *weekDelta {
		y, w := t.ISOWeek()
		k := weekKey{y, w}
		if d, ok := deltas[k]; ok {
			return d
		}
		d := &weekDelta{WeekStart: weekMonday(t)}
		deltas[k] = d
		return d
	}

	// Net change per week: +1 for each issue created, -1 for each resolved
	for _, issue := range issues {
		d := getOrCreate(issue.CreatedAt)
		d.Net++
		if issue.IsFixable {
			d.FixableNet++
		}
	}
	for _, issue := range resolved {
		if !issue.ResolvedAt.IsZero() {
			d := getOrCreate(issue.ResolvedAt)
			d.Net--
			if issue.IsFixable {
				d.FixableNet--
			}
		}
	}

	// Sort weeks oldest → newest
	sorted := make([]*weekDelta, 0, len(deltas))
	for _, d := range deltas {
		sorted = append(sorted, d)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].WeekStart.Before(sorted[j].WeekStart)
	})

	// Walk backwards from currentOpen/currentFixable to reconstruct historical totals.
	weeks := make([]charts.SnykIssueWeek, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		if i == len(sorted)-1 {
			weeks[i] = charts.SnykIssueWeek{
				WeekStart: sorted[i].WeekStart,
				Total:     currentOpen,
				Fixable:   currentFixable,
				Unfixable: currentOpen - currentFixable,
			}
		} else {
			total := max(0, weeks[i+1].Total-sorted[i+1].Net)
			fixable := max(0, weeks[i+1].Fixable-sorted[i+1].FixableNet)
			weeks[i] = charts.SnykIssueWeek{
				WeekStart: sorted[i].WeekStart,
				Total:     total,
				Fixable:   fixable,
				Unfixable: total - fixable,
			}
		}
	}

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
