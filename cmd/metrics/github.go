package metrics

import (
	"fmt"
	"os"
	"time"

	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"

	"devctl-em/internal/github"
	"devctl-em/internal/output"
)

// GithubCmd is the parent command for all GitHub metrics.
var GithubCmd = &cobra.Command{
	Use:   "github",
	Short: "GitHub DORA metrics",
	Long: `Generate DORA metrics from GitHub Actions data.

Available metrics:
  - Deployment frequency (how often code is deployed)

Setup:
  devctl-em config set github.org myorg
  devctl-em config set github.team my-team
  devctl-em config set github.api_token

Then run setup to pick deploy workflows:
  devctl-em metrics github setup

Examples:
  devctl-em metrics github deployment-frequency --from 2025-01-01
  devctl-em metrics github deployment-frequency -f csv`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Flags for github subcommands (separate from jira flags)
var (
	ghFromFlag   string
	ghToFlag     string
	ghOrgFlag    string
	ghTeamFlag   string
	ghOutputFlag string
	ghFormatFlag string
)

func init() {
	MetricsCmd.AddCommand(GithubCmd)

	GithubCmd.PersistentFlags().StringVar(&ghFromFlag, "from", "", "Start date (YYYY-MM-DD)")
	GithubCmd.PersistentFlags().StringVar(&ghToFlag, "to", "", "End date (YYYY-MM-DD)")
	GithubCmd.PersistentFlags().StringVar(&ghOrgFlag, "org", "", "GitHub organization (overrides config)")
	GithubCmd.PersistentFlags().StringVar(&ghTeamFlag, "team", "", "GitHub team slug (overrides config)")
	GithubCmd.PersistentFlags().StringVarP(&ghOutputFlag, "output", "o", "", "Output file path")
	GithubCmd.PersistentFlags().StringVarP(&ghFormatFlag, "format", "f", "", "Output format (csv)")
}

// getGithubClient creates a GitHub client from configuration.
func getGithubClient() (*github.Client, error) {
	token, err := secrets.Read("github", "api_token")
	if err != nil || token == "" {
		token = os.Getenv("GH_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("GitHub API token not configured. Run: devctl-em config set github.api_token")
	}

	creds := github.Credentials{
		Token: token,
		Org:   getGithubOrg(),
	}

	if override := os.Getenv("GITHUB_API_URL"); override != "" {
		creds.BaseURLOverride = override
	}

	return github.NewClient(creds), nil
}

// getGithubOrg returns the GitHub org from flag or config.
func getGithubOrg() string {
	if ghOrgFlag != "" {
		return ghOrgFlag
	}
	return getConfigString("github.org")
}

// getGithubTeam returns the GitHub team slug from flag or config.
func getGithubTeam() string {
	if ghTeamFlag != "" {
		return ghTeamFlag
	}
	return getConfigString("github.team")
}

// getGithubDateRange returns the from/to date range for GitHub commands.
func getGithubDateRange() (time.Time, time.Time, error) {
	var from, to time.Time
	var err error

	if ghFromFlag != "" {
		from, err = time.Parse("2006-01-02", ghFromFlag)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --from date: %w", err)
		}
	} else {
		// Default to 90 days
		from = time.Now().AddDate(0, 0, -90)
	}

	if ghToFlag != "" {
		to, err = time.Parse("2006-01-02", ghToFlag)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --to date: %w", err)
		}
	} else {
		to = time.Now()
	}

	return from, to, nil
}

// getGithubOutputPath returns the output file path.
func getGithubOutputPath(defaultName, defaultExt string) string {
	if ghOutputFlag != "" {
		return ghOutputFlag
	}
	return output.Path(defaultName + "." + defaultExt)
}

// getGithubOutputFormat returns the output format.
func getGithubOutputFormat(defaultFormat string) string {
	if ghFormatFlag != "" {
		return ghFormatFlag
	}
	return defaultFormat
}

// getConfiguredWorkflows reads the github.workflows map from config.
// Returns a map of repo name → workflow filename.
func getConfiguredWorkflows() (map[string]string, error) {
	raw := getConfigAny("github.workflows")
	if raw == nil {
		return nil, fmt.Errorf("no workflows configured. Run: devctl-em metrics github setup")
	}

	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid github.workflows config format")
	}

	workflows := make(map[string]string, len(rawMap))
	for repo, wf := range rawMap {
		wfStr, ok := wf.(string)
		if !ok {
			continue
		}
		workflows[repo] = wfStr
	}

	if len(workflows) == 0 {
		return nil, fmt.Errorf("no workflows configured. Run: devctl-em metrics github setup")
	}

	return workflows, nil
}
