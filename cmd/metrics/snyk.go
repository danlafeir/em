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

Setup:
  devctl-em config set snyk.org_id <org-id>
  devctl-em config set snyk.api_token
  devctl-em config set snyk.team <team-tag>

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
	snykTeamFlag   string
	snykOutputFlag string
	snykFormatFlag string
)

func init() {
	MetricsCmd.AddCommand(SnykCmd)

	SnykCmd.PersistentFlags().StringVar(&snykFromFlag, "from", "", "Start date (YYYY-MM-DD)")
	SnykCmd.PersistentFlags().StringVar(&snykToFlag, "to", "", "End date (YYYY-MM-DD)")
	SnykCmd.PersistentFlags().StringVar(&snykTeamFlag, "team", "", "Snyk project team tag (overrides config)")
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
		return nil, fmt.Errorf("Snyk API token not configured. Run: devctl-em config set snyk.api_token")
	}

	orgID := getConfigString("snyk.org_id")
	if orgID == "" {
		return nil, fmt.Errorf("Snyk org ID not configured. Run: devctl-em config set snyk.org_id <org-id>")
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

// getSnykTeam returns the Snyk team tag from flag or config.
func getSnykTeam() string {
	if snykTeamFlag != "" {
		return snykTeamFlag
	}
	return getConfigString("snyk.team")
}

// getSnykDateRange returns the from/to date range for Snyk commands.
func getSnykDateRange() (time.Time, time.Time, error) {
	var from, to time.Time
	var err error

	if snykFromFlag != "" {
		from, err = time.Parse("2006-01-02", snykFromFlag)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --from date: %w", err)
		}
	} else {
		// Default to 42 days (6 weeks)
		from = time.Now().AddDate(0, 0, -42)
	}

	if snykToFlag != "" {
		to, err = time.Parse("2006-01-02", snykToFlag)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --to date: %w", err)
		}
	} else {
		to = time.Now()
	}

	return from, to, nil
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
