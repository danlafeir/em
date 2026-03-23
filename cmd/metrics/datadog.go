package metrics

import (
	"fmt"
	"os"
	"time"

	"github.com/danlafeir/cli-go/pkg/secrets"
	"github.com/spf13/cobra"

	"em/internal/datadog"
	"em/internal/output"
)

// DatadogCmd is the parent command for all Datadog metrics.
var DatadogCmd = &cobra.Command{
	Hidden: true,
	Use:    "datadog",
	Short: "Datadog operational metrics",
	Long: `Generate operational metrics from Datadog data.

Available metrics:
  - monitors (monitor alert frequency)
  - slos     (SLO violation tracking)

Setup:
  em config set datadog.team my-team
  em config set datadog.api_key
  em config set datadog.app_key

Examples:
  em metrics datadog monitors
  em metrics datadog slos`,
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
		return nil, fmt.Errorf("Datadog API key not configured. Run: em config set datadog.api_key")
	}
	if appKey == "" {
		return nil, fmt.Errorf("Datadog App key not configured. Run: em config set datadog.app_key")
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

// getDatadogTeam returns the Datadog team from flag, falling back to the selected team.
func getDatadogTeam() string {
	if ddTeamFlag != "" {
		return ddTeamFlag
	}
	return getSelectedTeam()
}

// getDatadogDateRange returns the from/to date range for Datadog commands.
func getDatadogDateRange() (time.Time, time.Time, error) {
	return parseDateRange(ddFromFlag, ddToFlag)
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
