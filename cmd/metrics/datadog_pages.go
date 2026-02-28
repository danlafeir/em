package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"devctl-em/internal/datadog"
	"devctl-em/internal/output"
)

var datadogPagesCmd = &cobra.Command{
	Use:   "pages",
	Short: "On-call page response metrics",
	Long: `List on-call pages with acknowledgment and resolution times.

Examples:
  devctl-em metrics datadog pages
  devctl-em metrics datadog pages --from 2025-01-01 --to 2025-06-30
  devctl-em metrics datadog pages -f csv -o pages.csv`,
	RunE: runDatadogPages,
}

func init() {
	DatadogCmd.AddCommand(datadogPagesCmd)
}

func runDatadogPages(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getDatadogClient()
	if err != nil {
		return err
	}

	team := getDatadogTeam()
	if team == "" {
		return fmt.Errorf("Datadog team not configured. Run: devctl-em config set datadog.team <team>")
	}

	fmt.Println("Testing Datadog connection...")
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to Datadog: %w", err)
	}

	from, to, err := getDatadogDateRange()
	if err != nil {
		return err
	}

	fmt.Printf("Fetching pages for team %q (%s to %s)...\n",
		team, from.Format("2006-01-02"), to.Format("2006-01-02"))

	pages, err := client.ListPages(ctx, team, from, to)
	if err != nil {
		return fmt.Errorf("failed to list pages: %w", err)
	}

	if len(pages) == 0 {
		fmt.Println("\nNo pages found in the specified date range.")
		return nil
	}

	// Sort by created date descending
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].CreatedAt.After(pages[j].CreatedAt)
	})

	// CSV export
	if getDatadogOutputFormat("table") == "csv" {
		outputPath := getDatadogOutputPath("pages", "csv")
		if err := exportPagesCSV(pages, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("Exported to %s\n", outputPath)
		return nil
	}

	// Print table
	fmt.Printf("\nOn-Call Pages (%d total)\n", len(pages))
	fmt.Printf("========================\n\n")

	// Calculate column widths
	titleW := 5 // "Title"
	respW := 9  // "Responder"
	for _, p := range pages {
		t := truncateStr(p.Title, 40)
		if len(t) > titleW {
			titleW = len(t)
		}
		if len(p.Responder) > respW {
			respW = len(p.Responder)
		}
	}

	// Header
	fmt.Printf("| %-10s | %-7s | %-*s | %11s | %15s | %-*s |\n",
		"Date", "Urgency", titleW, "Title", "Time to Ack", "Time to Resolve", respW, "Responder")
	fmt.Printf("|%s|%s|%s|%s|%s|%s|\n",
		strings.Repeat("-", 12),
		strings.Repeat("-", 9),
		strings.Repeat("-", titleW+2),
		strings.Repeat("-", 13),
		strings.Repeat("-", 17),
		strings.Repeat("-", respW+2))

	for _, p := range pages {
		ackDur := ""
		if !p.AcknowledgedAt.IsZero() {
			ackDur = fmtPageDuration(int64(p.AcknowledgedAt.Sub(p.CreatedAt).Seconds()))
		}
		resolveDur := ""
		if !p.ResolvedAt.IsZero() {
			resolveDur = fmtPageDuration(int64(p.ResolvedAt.Sub(p.CreatedAt).Seconds()))
		}
		fmt.Printf("| %-10s | %-7s | %-*s | %11s | %15s | %-*s |\n",
			p.CreatedAt.Format("2006-01-02"),
			p.Urgency,
			titleW, truncateStr(p.Title, 40),
			ackDur,
			resolveDur,
			respW, p.Responder)
	}

	// Summary stats
	printPagesSummary(pages)

	return nil
}

func printPagesSummary(pages []datadog.Page) {
	var ackSeconds, resolveSeconds []float64
	for _, p := range pages {
		if !p.AcknowledgedAt.IsZero() {
			ackSeconds = append(ackSeconds, p.AcknowledgedAt.Sub(p.CreatedAt).Seconds())
		}
		if !p.ResolvedAt.IsZero() {
			resolveSeconds = append(resolveSeconds, p.ResolvedAt.Sub(p.CreatedAt).Seconds())
		}
	}

	fmt.Printf("\nSummary\n")
	fmt.Printf("-------\n")
	fmt.Printf("Total pages: %d\n", len(pages))

	if len(ackSeconds) > 0 {
		sort.Float64s(ackSeconds)
		fmt.Printf("Time to Ack  — Median: %s, P90: %s\n",
			fmtPageDuration(int64(percentile(ackSeconds, 50))),
			fmtPageDuration(int64(percentile(ackSeconds, 90))))
	}

	if len(resolveSeconds) > 0 {
		sort.Float64s(resolveSeconds)
		fmt.Printf("Time to Resolve — Median: %s, P90: %s\n",
			fmtPageDuration(int64(percentile(resolveSeconds, 50))),
			fmtPageDuration(int64(percentile(resolveSeconds, 90))))
	}
}

// percentile returns the p-th percentile from a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p / 100 * float64(len(sorted)-1)
	lower := int(idx)
	if lower >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lower)
	return sorted[lower] + frac*(sorted[lower+1]-sorted[lower])
}

// fmtPageDuration formats seconds into a human-readable duration like "3m" or "1h 23m".
func fmtPageDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	m := seconds / 60
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	h := m / 60
	rm := m % 60
	if rm == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, rm)
}

// truncateStr shortens a string to maxLen, adding "..." if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func exportPagesCSV(pages []datadog.Page, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Date", "Urgency", "Title", "Time to Ack (s)", "Time to Resolve (s)", "Responder"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, p := range pages {
		ack := ""
		if !p.AcknowledgedAt.IsZero() {
			ack = fmt.Sprintf("%.0f", p.AcknowledgedAt.Sub(p.CreatedAt).Seconds())
		}
		resolve := ""
		if !p.ResolvedAt.IsZero() {
			resolve = fmt.Sprintf("%.0f", p.ResolvedAt.Sub(p.CreatedAt).Seconds())
		}
		row := []string{
			p.CreatedAt.Format("2006-01-02"),
			p.Urgency,
			p.Title,
			ack,
			resolve,
			p.Responder,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
