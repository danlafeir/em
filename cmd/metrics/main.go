/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package metrics

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// parseDateRange parses --from and --to flag strings into time.Time values.
// Empty strings fall back to 42 days ago and now respectively.
func parseDateRange(fromStr, toStr string) (time.Time, time.Time, error) {
	var from, to time.Time
	var err error

	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --from date: %w", err)
		}
	} else {
		from = time.Now().AddDate(0, 0, -42)
	}

	if toStr != "" {
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --to date: %w", err)
		}
	} else {
		to = time.Now()
	}

	return from, to, nil
}

// MetricsCmd represents the metrics command
var MetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Generate engineering metrics and reports",
	Long: `Generate engineering metrics and reports from JIRA, GitHub, Snyk, and Datadog.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if mockUpstreamFlag {
			return startMockUpstream()
		}
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if mockUpstreamFlag {
			stopMockUpstream()
		}
		return nil
	},
}

// saveRawDataFlag saves raw calculation data to CSVs after a live API fetch.
// Use with --use-saved-data on subsequent runs to replay reports without API calls.
var saveRawDataFlag bool

func init() {
	MetricsCmd.PersistentFlags().BoolVar(&useSavedDataFlag, "use-saved-data", false,
		"Skip upstream API calls and regenerate reports from previously saved CSVs")
	MetricsCmd.PersistentFlags().BoolVar(&saveRawDataFlag, "save-raw-data", false,
		"Save raw calculation data to CSV files after fetching (use with --use-saved-data to replay reports)")
	MetricsCmd.PersistentFlags().BoolVar(&mockUpstreamFlag, "mock-upstream", false,
		"Use in-process mock servers instead of real API endpoints (for testing/development without credentials)")
}
