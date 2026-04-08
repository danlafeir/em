package metrics

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"em/internal/charts"
	"em/internal/datadog"
)

var datadogMonitorsCmd = &cobra.Command{
	Use:   "monitors",
	Short: "List monitors for a team and whether they have recently triggered",
	Long: `List monitors for a team showing current state and recent alert history.

Required:
  em metrics datadog config`,
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
		from = time.Now().AddDate(0, -1, 0)
	}

	team := getDatadogTeam()
	if team == "" {
		return fmt.Errorf("Datadog team not configured. Use --team or run: em metrics datadog config")
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
	// No team filter on events — we join by monitor_id against the already-team-filtered monitors list.
	events, err := client.ListMonitorEvents(ctx, "", from, to)
	if err != nil {
		return fmt.Errorf("failed to list monitor events: %w", err)
	}

	// Build map: monitor ID → events
	eventsByID := make(map[int64][]datadog.MonitorEvent)
	for _, e := range events {
		if e.MonitorID > 0 {
			eventsByID[e.MonitorID] = append(eventsByID[e.MonitorID], e)
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

	// A monitor is considered "alerted" if it fired recently OR is currently in a non-OK state
	// (e.g. went into Alert before the window and is still there).
	isAlerted := func(r monitorRow) bool {
		if r.fireCount > 0 {
			return true
		}
		s := r.monitor.OverallState
		return s == "Alert" || s == "No Data"
	}

	// Sort: alerted first, then by fire count desc, then alphabetically.
	sort.Slice(rows, func(i, j int) bool {
		ai, aj := isAlerted(rows[i]), isAlerted(rows[j])
		if ai != aj {
			return ai
		}
		if rows[i].fireCount != rows[j].fireCount {
			return rows[i].fireCount > rows[j].fireCount
		}
		return rows[i].monitor.Name < rows[j].monitor.Name
	})

	// Print table
	triggered := 0
	for _, r := range rows {
		if isAlerted(r) {
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
		if isAlerted(r) {
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

