package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/internal/charts"
	"devctl-em/internal/datadog"
	"devctl-em/internal/output"
)

var datadogMonitorsCmd = &cobra.Command{
	Use:   "monitors",
	Short: "List monitors for a team and whether they have recently triggered",
	Long: `List all monitors for a team, showing current state and whether each
monitor triggered in the last 2 weeks (or the specified date range).

Examples:
  devctl-em metrics datadog monitors
  devctl-em metrics datadog monitors --team my-team
  devctl-em metrics datadog monitors --from 2025-01-01 --to 2025-06-30
  devctl-em metrics datadog monitors -f csv -o monitors.csv`,
	RunE: runDatadogMonitors,
}

func init() {
	DatadogCmd.AddCommand(datadogMonitorsCmd)
}

func runDatadogMonitors(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getDatadogClient()
	if err != nil {
		return err
	}

	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to Datadog: %w", err)
	}

	// Default to last 14 days if no range specified.
	from, to, err := getDatadogDateRange()
	if err != nil {
		return err
	}
	if ddFromFlag == "" {
		from = time.Now().AddDate(0, 0, -14)
	}

	team := getDatadogTeam()
	if team == "" {
		return fmt.Errorf("Datadog team not configured. Use --team or run: devctl-em metrics datadog config")
	}
	teamTag := "team:" + team

	fmt.Printf("Fetching monitors for team %q...\n", team)
	monitors, err := client.ListMonitors(ctx, teamTag)
	if err != nil {
		return fmt.Errorf("failed to list monitors: %w", err)
	}
	if len(monitors) == 0 {
		fmt.Printf("No monitors found for team %q.\n", team)
		return nil
	}

	fmt.Printf("Fetching alert events (%s to %s)...\n",
		from.Format("2006-01-02"), to.Format("2006-01-02"))
	events, err := client.ListMonitorEvents(ctx, "tags:"+teamTag, from, to)
	if err != nil {
		return fmt.Errorf("failed to list monitor events: %w", err)
	}

	// Build map: monitor ID → events
	eventsByID := make(map[int64][]datadog.MonitorEvent)
	for _, e := range events {
		id := monitorIDFromTags(e.Tags)
		if id > 0 {
			eventsByID[id] = append(eventsByID[id], e)
		}
	}

	rows := make([]monitorRow, len(monitors))
	for i, m := range monitors {
		evts := eventsByID[m.ID]
		var last time.Time
		for _, e := range evts {
			if e.Timestamp.After(last) {
				last = e.Timestamp
			}
		}
		rows[i] = monitorRow{monitor: m, fireCount: len(evts), lastFired: last}
	}

	// Sort: triggered first (by count desc), then alphabetically
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].fireCount != rows[j].fireCount {
			return rows[i].fireCount > rows[j].fireCount
		}
		return rows[i].monitor.Name < rows[j].monitor.Name
	})

	if getDatadogOutputFormat("table") == "csv" {
		outputPath := getDatadogOutputPath("monitors", "csv")
		if err := exportMonitorsCSV(rows, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("Exported to %s\n", outputPath)
		return nil
	}

	// Print table
	triggered := 0
	for _, r := range rows {
		if r.fireCount > 0 {
			triggered++
		}
	}

	fmt.Printf("\nMonitors for %q (%s to %s)\n", team,
		from.Format("2006-01-02"), to.Format("2006-01-02"))
	fmt.Printf("%d monitors, %d triggered\n\n", len(rows), triggered)

	nameW := 7 // "Monitor"
	for _, r := range rows {
		n := len(truncateStr(r.monitor.Name, 60))
		if n > nameW {
			nameW = n
		}
	}
	stateW := 8 // "State"

	fmt.Printf("| %-*s | %-*s | %6s | %-10s |\n",
		nameW, "Monitor", stateW, "State", "Fires", "Last Fired")
	fmt.Printf("|%s|%s|%s|%s|\n",
		strings.Repeat("-", nameW+2),
		strings.Repeat("-", stateW+2),
		strings.Repeat("-", 8),
		strings.Repeat("-", 12))

	for _, r := range rows {
		lastFired := ""
		if !r.lastFired.IsZero() {
			lastFired = r.lastFired.Format("2006-01-02")
		}
		fmt.Printf("| %-*s | %-*s | %6d | %-10s |\n",
			nameW, truncateStr(r.monitor.Name, nameW),
			stateW, truncateStr(r.monitor.OverallState, stateW),
			r.fireCount,
			lastFired)
	}

	// Generate HTML widget page
	widgets := make([]charts.Widget, len(rows))
	for i, r := range rows {
		label := "alerts"
		if r.fireCount == 1 {
			label = "alert"
		}
		if !r.lastFired.IsZero() {
			label += " · last " + r.lastFired.Format("Jan 2")
		}
		stateClass := "widget-ok"
		if r.fireCount > 0 {
			stateClass = "widget-alerted"
		}
		widgets[i] = charts.Widget{
			Name:       r.monitor.Name,
			Value:      strconv.Itoa(r.fireCount),
			Label:      label,
			StateClass: stateClass,
		}
	}
	subtitle := fmt.Sprintf("%s to %s · %d monitors, %d triggered",
		from.Format("Jan 2"), to.Format("Jan 2"), len(rows), triggered)
	outputPath := getDatadogOutputPath("monitors", "html")
	if err := charts.WidgetPage(charts.WidgetPageData{
		Title:    "Monitors · " + team,
		Subtitle: subtitle,
		Widgets:  widgets,
	}, outputPath); err != nil {
		return fmt.Errorf("failed to generate HTML: %w", err)
	}
	fmt.Printf("\nReport saved to %s\n", outputPath)
	charts.OpenBrowser(outputPath)

	return nil
}

// monitorIDFromTags extracts the monitor ID from event tags (e.g. "monitor_id:12345").
func monitorIDFromTags(tags []string) int64 {
	for _, t := range tags {
		if after, ok := strings.CutPrefix(t, "monitor_id:"); ok {
			if id, err := strconv.ParseInt(after, 10, 64); err == nil {
				return id
			}
		}
	}
	return 0
}

// truncateStr shortens a string to maxLen, adding "..." if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

type monitorRow struct {
	monitor   datadog.Monitor
	fireCount int
	lastFired time.Time
}

func exportMonitorsCSV(rows []monitorRow, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Monitor", "Type", "Current State", "Fires", "Last Fired", "Tags"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, r := range rows {
		lastFired := ""
		if !r.lastFired.IsZero() {
			lastFired = r.lastFired.Format("2006-01-02")
		}
		row := []string{
			r.monitor.Name,
			r.monitor.Type,
			r.monitor.OverallState,
			strconv.Itoa(r.fireCount),
			lastFired,
			strings.Join(r.monitor.Tags, ", "),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
