package metrics

import (
	"fmt"
	"os"
	"time"

	"github.com/danlafeir/cli-go/pkg/secrets"
	"github.com/spf13/cobra"

	"em/internal/output"
	"em/pkg/snyk"
)

// SnykCmd is the parent command for all Snyk metrics.
var SnykCmd = &cobra.Command{
	Use:   "snyk",
	Short: "Snyk security metrics",
	Long: `Generate security metrics from Snyk.

Required:
  em metrics snyk config`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Flags for snyk subcommands
var (
	snykFromFlag string
	snykToFlag   string
)

func init() {
	MetricsCmd.AddCommand(SnykCmd)

	SnykCmd.PersistentFlags().StringVar(&snykFromFlag, "from", "", "Start date (YYYY-MM-DD)")
	SnykCmd.PersistentFlags().StringVar(&snykToFlag, "to", "", "End date (YYYY-MM-DD)")
}

// getSnykClient creates a Snyk client from configuration.
func getSnykClient() (*snyk.Client, error) {
	if activeMock.snykClient != nil {
		return activeMock.snykClient, nil
	}

	token, err := secrets.Read("snyk", "api_token")
	if err != nil || token == "" {
		token = os.Getenv("SNYK_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("Snyk API token not configured. Run: em metrics snyk config")
	}

	orgID := getConfigString("snyk.org_id")
	if orgID == "" {
		return nil, fmt.Errorf("Snyk org ID not configured. Run: em metrics snyk config")
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
	return output.Path(defaultName + "." + defaultExt)
}
