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

var datadogMonitorsCmd = &cobra.Command{
	Use:   "monitors",
	Short: "Monitor alert frequency and MTTF metrics",
	Long: `List monitors that have fired, with alert frequency and mean time to recovery.

Filters to monitors that transitioned to Alert, Warn, or No Data within the date range.
Use --team to filter by a Datadog team tag.

Examples:
  devctl-em metrics datadog monitors
  devctl-em metrics datadog monitors --from 2025-01-01 --to 2025-06-30
  devctl-em metrics datadog monitors --team my-team
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

	fmt.Println("Testing Datadog connection...")
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to Datadog: %w", err)
	}

	from, to, err := getDatadogDateRange()
	if err != nil {
		return err
	}

	team := getDatadogTeam()
	tagsQuery := ""
	if team != "" {
		tagsQuery = "tags:team:" + team
	}

	fmt.Printf("Fetching monitor alerts (%s to %s)...\n", from.Format("2006-01-02"), to.Format("2006-01-02"))

	events, err := client.ListMonitorEvents(ctx, tagsQuery, from, to)
	if err != nil {
		return fmt.Errorf("failed to list monitor events: %w", err)
	}

	if len(events) == 0 {
		fmt.Println("\nNo monitor alerts found in the specified date range.")
		return nil
	}

	// Sort by timestamp descending
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	// CSV export
	if getDatadogOutputFormat("table") == "csv" {
		outputPath := getDatadogOutputPath("monitors", "csv")
		if err := exportMonitorsCSV(events, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("Exported to %s\n", outputPath)
		return nil
	}

	// Count alerts per monitor
	type monitorStats struct {
		name  string
		count int
		last  string
	}
	byMonitor := make(map[string]*monitorStats)
	for _, e := range events {
		name := e.MonitorName
		if s, ok := byMonitor[name]; ok {
			s.count++
		} else {
			byMonitor[name] = &monitorStats{
				name:  name,
				count: 1,
				last:  e.Timestamp.Format("2006-01-02"),
			}
		}
	}

	sorted := make([]*monitorStats, 0, len(byMonitor))
	for _, s := range byMonitor {
		sorted = append(sorted, s)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	// Print table
	fmt.Printf("\nMonitor Alerts (%d events, %d unique monitors)\n", len(events), len(sorted))
	fmt.Printf("================================================\n\n")

	nameW := 12
	for _, s := range sorted {
		t := truncateStr(s.name, 60)
		if len(t) > nameW {
			nameW = len(t)
		}
	}

	fmt.Printf("| %-*s | %6s | %-10s |\n", nameW, "Monitor", "Alerts", "Last Fired")
	fmt.Printf("|%s|%s|%s|\n",
		strings.Repeat("-", nameW+2),
		strings.Repeat("-", 8),
		strings.Repeat("-", 12))

	for _, s := range sorted {
		fmt.Printf("| %-*s | %6d | %-10s |\n",
			nameW, truncateStr(s.name, nameW),
			s.count,
			s.last)
	}

	fmt.Printf("\nTotal alert events: %d\n", len(events))

	return nil
}

func exportMonitorsCSV(events []datadog.MonitorEvent, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Timestamp", "Monitor", "Status", "Priority", "Tags"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, e := range events {
		row := []string{
			e.Timestamp.Format("2006-01-02T15:04:05Z"),
			e.MonitorName,
			e.Status,
			e.Priority,
			strings.Join(e.Tags, ", "),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
