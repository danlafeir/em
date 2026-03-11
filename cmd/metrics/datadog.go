package metrics

import (
	"fmt"
	"os"
	"time"

	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"

	"devctl-em/internal/datadog"
	"devctl-em/internal/output"
)

// DatadogCmd is the parent command for all Datadog metrics.
var DatadogCmd = &cobra.Command{
	Hidden: true,
	Use:    "datadog",
	Short: "Datadog operational metrics",
	Long: `Generate operational metrics from Datadog data.

Available metrics:
  - pages   (on-call page response times)
  - slos    (SLO violation tracking)

Setup:
  devctl-em config set datadog.team my-team
  devctl-em config set datadog.api_key
  devctl-em config set datadog.app_key

Examples:
  devctl-em metrics datadog pages
  devctl-em metrics datadog slos`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Flags for datadog subcommands
var (
	ddFromFlag   string
	ddToFlag     string
	ddTeamFlag   string
	ddOutputFlag string
	ddFormatFlag string
)

func init() {
	MetricsCmd.AddCommand(DatadogCmd)

	DatadogCmd.PersistentFlags().StringVar(&ddFromFlag, "from", "", "Start date (YYYY-MM-DD)")
	DatadogCmd.PersistentFlags().StringVar(&ddToFlag, "to", "", "End date (YYYY-MM-DD)")
	DatadogCmd.PersistentFlags().StringVar(&ddTeamFlag, "team", "", "Datadog team (overrides config)")
	DatadogCmd.PersistentFlags().StringVarP(&ddOutputFlag, "output", "o", "", "Output file path")
	DatadogCmd.PersistentFlags().StringVarP(&ddFormatFlag, "format", "f", "", "Output format (csv)")
}

// getDatadogClient creates a Datadog client from configuration.
func getDatadogClient() (*datadog.Client, error) {
	apiKey, err := secrets.Read("datadog", "api_key")
	if err != nil || apiKey == "" {
		apiKey = os.Getenv("DD_API_KEY")
	}

	appKey, err := secrets.Read("datadog", "app_key")
	if err != nil || appKey == "" {
		appKey = os.Getenv("DD_APP_KEY")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("Datadog API key not configured. Run: devctl-em config set datadog.api_key")
	}
	if appKey == "" {
		return nil, fmt.Errorf("Datadog App key not configured. Run: devctl-em config set datadog.app_key")
	}

	site := getConfigString("datadog.site")
	if site == "" {
		site = "datadoghq.com"
	}

	creds := datadog.Credentials{
		APIKey: apiKey,
		AppKey: appKey,
		Site:   site,
	}

	return datadog.NewClient(creds), nil
}

// getDatadogTeam returns the Datadog team from flag or config.
func getDatadogTeam() string {
	if ddTeamFlag != "" {
		return ddTeamFlag
	}
	return getConfigString("datadog.team")
}

// getDatadogDateRange returns the from/to date range for Datadog commands.
func getDatadogDateRange() (time.Time, time.Time, error) {
	var from, to time.Time
	var err error

	if ddFromFlag != "" {
		from, err = time.Parse("2006-01-02", ddFromFlag)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --from date: %w", err)
		}
	} else {
		// Default to 42 days (6 weeks)
		from = time.Now().AddDate(0, 0, -42)
	}

	if ddToFlag != "" {
		to, err = time.Parse("2006-01-02", ddToFlag)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --to date: %w", err)
		}
	} else {
		to = time.Now()
	}

	return from, to, nil
}

// getDatadogOutputPath returns the output file path.
func getDatadogOutputPath(defaultName, defaultExt string) string {
	if ddOutputFlag != "" {
		return ddOutputFlag
	}
	return output.Path(defaultName + "." + defaultExt)
}

// getDatadogOutputFormat returns the output format.
func getDatadogOutputFormat(defaultFormat string) string {
	if ddFormatFlag != "" {
		return ddFormatFlag
	}
	return defaultFormat
}
