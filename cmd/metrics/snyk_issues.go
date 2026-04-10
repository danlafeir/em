package metrics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danlafeir/em/internal/charts"
	"github.com/danlafeir/em/pkg/snyk"
)

var snykIssuesCmd = &cobra.Command{
	Use:   "issues",
	Short: "Vulnerability counts and weekly trends",
	Long: `Show vulnerability counts by severity and generate a weekly trend chart.

Required:
  em metrics snyk config`,
	RunE: runSnykIssues,
}

func init() {
	SnykCmd.AddCommand(snykIssuesCmd)
}

func runSnykIssues(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	from, to, err := getSnykDateRange()
	if err != nil {
		return err
	}

	var client *snyk.Client
	if !useSavedDataFlag {
		client, err = getSnykClient()
		if err != nil {
			return err
		}
		fmt.Println("Testing Snyk connection...")
		if err := client.TestConnection(ctx); err != nil {
			return fmt.Errorf("failed to connect to Snyk: %w", err)
		}
		fmt.Printf("Fetching issues (%s to %s)...\n",
			from.Format("2006-01-02"), to.Format("2006-01-02"))
	}

	issues, resolved, openCounts, err := fetchOrLoadSnykData(ctx, client, from, to)
	if err != nil {
		return fmt.Errorf("failed to fetch Snyk data: %w", err)
	}

	if openCounts.Total == 0 && len(issues) == 0 {
		fmt.Println("\nNo issues found.")
		return nil
	}

	// Print severity summary
	counts := countBySeverity(issues)
	exploitable := countExploitableBySeverity(issues)
	total := counts["critical"] + counts["high"] + counts["medium"] + counts["low"]

	fmt.Printf("\nVulnerability Summary (%d total)\n", total)
	fmt.Printf("================================\n\n")
	fmt.Printf("| %-16s | %5s |\n", "Severity", "Count")
	fmt.Printf("|%s|%s|\n", strings.Repeat("-", 18), strings.Repeat("-", 7))
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		label := strings.ToUpper(sev[:1]) + sev[1:]
		fmt.Printf("| %-16s | %5d |\n", label, counts[sev])
		fmt.Printf("| %-16s | %5d |\n", "  exploitable", exploitable[sev])
	}
	fmt.Printf("|%s|%s|\n", strings.Repeat("-", 18), strings.Repeat("-", 7))
	fmt.Printf("| %-16s | %5d |\n", "Total", total)

	fmt.Printf("\n| %-16s | %5s |\n", "Fix Status", "Count")
	fmt.Printf("|%s|%s|\n", strings.Repeat("-", 18), strings.Repeat("-", 7))
	fmt.Printf("| %-16s | %5d |\n", "Fixable", openCounts.Fixable)
	fmt.Printf("| %-16s | %5d |\n", "  exploitable", openCounts.ExploitableFixable)
	fmt.Printf("| %-16s | %5d |\n", "Unfixable", openCounts.Unfixable)

	// Generate weekly trend chart
	weeks := bucketByWeek(issues, resolved, openCounts.Total, openCounts.Fixable, openCounts.IgnoredFixable, openCounts.IgnoredUnfixable, from, to)
	if len(weeks) > 0 {
		cfg := charts.Config{Title: "Snyk Issues — Weekly Trend"}
		chartPath := getSnykOutputPath("snyk-issues", "html")
		if err := charts.SnykIssuesLine(weeks, cfg, chartPath); err != nil {
			return fmt.Errorf("failed to create chart: %w", err)
		}
		fmt.Printf("\nChart saved to %s\n", chartPath)
		openBrowser(chartPath)
	}

	return nil
}

func countBySeverity(issues []snyk.Issue) map[string]int {
	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}
	for _, issue := range issues {
		sev := strings.ToLower(issue.Severity)
		if _, ok := counts[sev]; ok {
			counts[sev]++
		}
	}
	return counts
}

func countExploitableBySeverity(issues []snyk.Issue) map[string]int {
	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}
	for _, issue := range issues {
		if !snyk.IsExploitable(issue.Exploitability) {
			continue
		}
		sev := strings.ToLower(issue.Severity)
		if _, ok := counts[sev]; ok {
			counts[sev]++
		}
	}
	return counts
}

// weekDelta holds the net change in open issues for a single week.
type weekDelta struct {
	WeekStart          time.Time
	Net                int // created - resolved
	FixableNet         int // fixable created - fixable resolved
	IgnoredFixableNet  int
	IgnoredUnfixableNet int
}

// bucketByWeek computes the true total of open vulnerabilities at each week's end.
// It anchors on currentOpen (the live count right now) and walks backwards through
// the weeks, reversing each week's net change to reconstruct the historical total.
func bucketByWeek(issues []snyk.Issue, resolved []snyk.Issue, currentOpen, currentFixable, currentIgnoredFixable, currentIgnoredUnfixable int, from, to time.Time) []charts.SnykIssueWeek {
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
		if issue.IsIgnored {
			if issue.IsFixable {
				d.IgnoredFixableNet++
			} else {
				d.IgnoredUnfixableNet++
			}
		} else if issue.IsFixable {
			d.FixableNet++
		}
	}
	for _, issue := range resolved {
		if !issue.ResolvedAt.IsZero() {
			d := getOrCreate(issue.ResolvedAt)
			d.Net--
			if issue.IsIgnored {
				if issue.IsFixable {
					d.IgnoredFixableNet--
				} else {
					d.IgnoredUnfixableNet--
				}
			} else if issue.IsFixable {
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

	// Walk backwards from current counts to reconstruct historical totals.
	weeks := make([]charts.SnykIssueWeek, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		if i == len(sorted)-1 {
			weeks[i] = charts.SnykIssueWeek{
				WeekStart:        sorted[i].WeekStart,
				Total:            currentOpen,
				Fixable:          currentFixable,
				Unfixable:        currentOpen - currentFixable,
				IgnoredFixable:   currentIgnoredFixable,
				IgnoredUnfixable: currentIgnoredUnfixable,
			}
		} else {
			total := max(0, weeks[i+1].Total-sorted[i+1].Net)
			fixable := max(0, weeks[i+1].Fixable-sorted[i+1].FixableNet)
			ignoredFixable := max(0, weeks[i+1].IgnoredFixable-sorted[i+1].IgnoredFixableNet)
			ignoredUnfixable := max(0, weeks[i+1].IgnoredUnfixable-sorted[i+1].IgnoredUnfixableNet)
			weeks[i] = charts.SnykIssueWeek{
				WeekStart:        sorted[i].WeekStart,
				Total:            total,
				Fixable:          fixable,
				Unfixable:        total - fixable,
				IgnoredFixable:   ignoredFixable,
				IgnoredUnfixable: ignoredUnfixable,
			}
		}
	}

	return weeks
}

