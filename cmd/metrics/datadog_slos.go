package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"devctl-em/internal/output"
)

var datadogSLOsCmd = &cobra.Command{
	Use:   "slos",
	Short: "SLO violation tracking",
	Long: `Check SLOs for violations and display current status.

Only violated SLOs (where current SLI value < target) are shown.

Examples:
  devctl-em metrics datadog slos
  devctl-em metrics datadog slos --from 2025-01-01 --to 2025-06-30
  devctl-em metrics datadog slos -f csv -o slos.csv`,
	RunE: runDatadogSLOs,
}

func init() {
	DatadogCmd.AddCommand(datadogSLOsCmd)
}

type sloResult struct {
	App     string
	Name    string
	Type    string
	Target  float64
	Current float64
	Budget  float64
}

func runDatadogSLOs(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("Fetching SLOs for team %q (%s to %s)...\n",
		team, from.Format("2006-01-02"), to.Format("2006-01-02"))

	tagsQuery := "team:" + team
	slos, err := client.ListSLOs(ctx, tagsQuery)
	if err != nil {
		return fmt.Errorf("failed to list SLOs: %w", err)
	}

	if len(slos) == 0 {
		fmt.Println("\nNo SLOs found for the specified team.")
		return nil
	}

	fmt.Printf("Checking %d SLOs...\n", len(slos))

	var violated []sloResult
	var totalChecked int

	for _, slo := range slos {
		history, err := client.GetSLOHistory(ctx, slo.ID, from, to)
		if err != nil {
			fmt.Printf("  Warning: could not fetch history for %q: %v\n", slo.Name, err)
			continue
		}
		totalChecked++

		// Find the primary target (first threshold)
		target := 99.9
		if len(slo.Thresholds) > 0 {
			target = slo.Thresholds[0].Target
		}

		current := history.SLIValue * 100 // API returns as decimal
		if current < target {
			violated = append(violated, sloResult{
				App:     extractSLOApp(slo.Tags),
				Name:    slo.Name,
				Type:    slo.Type,
				Target:  target,
				Current: current,
				Budget:  history.ErrorBudgetRemaining * 100,
			})
		}
	}

	// CSV export
	if getDatadogOutputFormat("table") == "csv" {
		outputPath := getDatadogOutputPath("slos", "csv")
		if err := exportSLOsCSV(violated, outputPath); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
		fmt.Printf("Exported to %s\n", outputPath)
		return nil
	}

	// Print results
	fmt.Printf("\nSLO Violations\n")
	fmt.Printf("==============\n\n")

	if len(violated) == 0 {
		fmt.Printf("No SLO violations found. All %d SLOs are meeting their targets.\n", totalChecked)
		return nil
	}

	// Group by application
	grouped := groupSLOsByApp(violated)

	// Sort app names for deterministic output
	apps := make([]string, 0, len(grouped))
	for app := range grouped {
		apps = append(apps, app)
	}
	sort.Strings(apps)

	for _, app := range apps {
		results := grouped[app]
		label := app
		if label == "" {
			label = "(untagged)"
		}
		fmt.Printf("%s\n%s\n\n", label, strings.Repeat("-", len(label)))

		// Calculate column widths
		nameW := 8 // "SLO Name"
		typeW := 4 // "Type"
		for _, r := range results {
			if len(r.Name) > nameW {
				nameW = len(r.Name)
			}
			if len(r.Type) > typeW {
				typeW = len(r.Type)
			}
		}
		if nameW > 50 {
			nameW = 50
		}

		// Header
		fmt.Printf("| %-*s | %-*s | %7s | %7s | %7s | %8s |\n",
			nameW, "SLO Name", typeW, "Type", "Target", "Current", "Budget", "Status")
		fmt.Printf("|%s|%s|%s|%s|%s|%s|\n",
			strings.Repeat("-", nameW+2),
			strings.Repeat("-", typeW+2),
			strings.Repeat("-", 9),
			strings.Repeat("-", 9),
			strings.Repeat("-", 9),
			strings.Repeat("-", 10))

		for _, r := range results {
			name := truncate(r.Name, 50)
			fmt.Printf("| %-*s | %-*s | %6.2f%% | %6.2f%% | %6.1f%% | %8s |\n",
				nameW, name,
				typeW, r.Type,
				r.Target,
				r.Current,
				r.Budget,
				"VIOLATED")
		}
		fmt.Println()
	}

	// Summary
	fmt.Printf("Summary\n")
	fmt.Printf("-------\n")
	fmt.Printf("SLOs checked: %d\n", totalChecked)
	fmt.Printf("Violated: %d (across %d apps)\n", len(violated), len(grouped))

	return nil
}

func exportSLOsCSV(results []sloResult, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Application", "SLO Name", "Type", "Target (%)", "Current (%)", "Budget Remaining (%)", "Status"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, r := range results {
		row := []string{
			r.App,
			r.Name,
			r.Type,
			fmt.Sprintf("%.2f", r.Target),
			fmt.Sprintf("%.2f", r.Current),
			fmt.Sprintf("%.1f", r.Budget),
			"VIOLATED",
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// extractSLOApp extracts an application name from SLO tags.
// Looks for "service:", "app:", or "application:" tag prefixes.
func extractSLOApp(tags []string) string {
	for _, prefix := range []string{"service:", "app:", "application:"} {
		for _, tag := range tags {
			if strings.HasPrefix(tag, prefix) {
				return tag[len(prefix):]
			}
		}
	}
	return ""
}

// groupSLOsByApp groups SLO results by application name.
func groupSLOsByApp(results []sloResult) map[string][]sloResult {
	grouped := make(map[string][]sloResult)
	for _, r := range results {
		grouped[r.App] = append(grouped[r.App], r)
	}
	return grouped
}
