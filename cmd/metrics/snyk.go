package metrics

import (
	"fmt"
	"os"
	"time"

	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"

	"devctl-em/internal/output"
	"devctl-em/internal/snyk"
)

// SnykCmd is the parent command for all Snyk metrics.
var SnykCmd = &cobra.Command{
	Use:   "snyk",
	Short: "Snyk security metrics",
	Long: `Generate security metrics from Snyk data.

Available metrics:
  - issues   (vulnerability counts and weekly trends)

Configure Snyk first:
  devctl-em metrics snyk config

Examples:
  devctl-em metrics snyk issues
  devctl-em metrics snyk issues --from 2025-01-01 --to 2025-06-30`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Flags for snyk subcommands
var (
	snykFromFlag   string
	snykToFlag     string
	snykOutputFlag string
	snykFormatFlag string
)

func init() {
	MetricsCmd.AddCommand(SnykCmd)

	SnykCmd.PersistentFlags().StringVar(&snykFromFlag, "from", "", "Start date (YYYY-MM-DD)")
	SnykCmd.PersistentFlags().StringVar(&snykToFlag, "to", "", "End date (YYYY-MM-DD)")
	SnykCmd.PersistentFlags().StringVarP(&snykOutputFlag, "output", "o", "", "Output file path")
	SnykCmd.PersistentFlags().StringVarP(&snykFormatFlag, "format", "f", "", "Output format (csv)")
}

// getSnykClient creates a Snyk client from configuration.
func getSnykClient() (*snyk.Client, error) {
	token, err := secrets.Read("snyk", "api_token")
	if err != nil || token == "" {
		token = os.Getenv("SNYK_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("Snyk API token not configured. Run: devctl-em metrics snyk config")
	}

	orgID := getConfigString("snyk.org_id")
	if orgID == "" {
		return nil, fmt.Errorf("Snyk org ID not configured. Run: devctl-em metrics snyk config")
	}

	site := getConfigString("snyk.site")
	if site == "" {
		site = "api.snyk.io"
	}

	creds := snyk.Credentials{
		Token: token,
		OrgID: orgID,
		Site:  site,
	}

	return snyk.NewClient(creds), nil
}

// getSnykDateRange returns the from/to date range for Snyk commands.
func getSnykDateRange() (time.Time, time.Time, error) {
	return parseDateRange(snykFromFlag, snykToFlag)
}

// getSnykOutputPath returns the output file path.
func getSnykOutputPath(defaultName, defaultExt string) string {
	if snykOutputFlag != "" {
		return snykOutputFlag
	}
	return output.Path(defaultName + "." + defaultExt)
}

// getSnykOutputFormat returns the output format.
func getSnykOutputFormat(defaultFormat string) string {
	if snykFormatFlag != "" {
		return snykFormatFlag
	}
	return defaultFormat
}
