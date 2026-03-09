package metrics

import (
	"fmt"
	"sort"
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
  devctl-em config set github.api_token

Then run config to pick deploy workflows per team:
  devctl-em metrics github config --team my-team

Config structure:
  github.teams.<team>.workflows:
    repo-a: deploy.yml
    repo-b: release.yaml

Examples:
  devctl-em metrics github deployment-frequency --from 2025-01-01
  devctl-em metrics github deployment-frequency --team platform
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
	GithubCmd.PersistentFlags().StringVar(&ghTeamFlag, "team", "", "GitHub team slug (filters to one team)")

	GithubCmd.PersistentFlags().StringVarP(&ghOutputFlag, "output", "o", "", "Output file path")
	GithubCmd.PersistentFlags().StringVarP(&ghFormatFlag, "format", "f", "", "Output format (csv)")
}

// getGithubClient creates a GitHub client from configuration.
func getGithubClient() (*github.Client, error) {
	token, err := secrets.Read("github", "api_token")
	if err != nil || token == "" {
		return nil, fmt.Errorf("GitHub API token not configured. Run: devctl-em config set github.api_token")
	}

	creds := github.Credentials{
		Token: token,
		Org:   getGithubOrg(),
	}

	return github.NewClient(creds)
}

// getGithubOrg returns the GitHub org from flag or config.
func getGithubOrg() string {
	if ghOrgFlag != "" {
		return ghOrgFlag
	}
	return getConfigString("github.org")
}

// getGithubTeams returns all configured team slugs, or just the --team flag if set.
func getGithubTeams() []string {
	if ghTeamFlag != "" {
		return []string{ghTeamFlag}
	}

	raw := getConfigAny("github.teams")
	if raw == nil {
		return nil
	}

	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	teams := make([]string, 0, len(rawMap))
	for slug := range rawMap {
		teams = append(teams, slug)
	}
	sort.Strings(teams)
	return teams
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

// getConfiguredWorkflowsByTeam reads workflows for a specific team from config.
// Returns a map of repo name → workflow filenames (supports multiple per repo).
func getConfiguredWorkflowsByTeam(team string) (map[string][]string, error) {
	key := fmt.Sprintf("github.teams.%s.workflows", team)
	raw := getConfigAny(key)
	if raw == nil {
		return nil, fmt.Errorf("no workflows configured for team %q. Run: devctl-em metrics github config --team %s", team, team)
	}

	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid config format for %s", key)
	}

	workflows := make(map[string][]string, len(rawMap))
	for repo, wf := range rawMap {
		switch v := wf.(type) {
		case string:
			if v != "" {
				workflows[repo] = []string{v}
			}
		case []any:
			var files []string
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					files = append(files, s)
				}
			}
			if len(files) > 0 {
				workflows[repo] = files
			}
		}
	}

	if len(workflows) == 0 {
		return nil, fmt.Errorf("no workflows configured for team %q. Run: devctl-em metrics github config --team %s", team, team)
	}

	return workflows, nil
}

// teamWorkflows holds workflows for a specific team.
type teamWorkflows struct {
	Team      string
	Workflows map[string][]string
}

// getAllConfiguredWorkflows returns workflows for all teams (or just --team if set).
func getAllConfiguredWorkflows() ([]teamWorkflows, error) {
	teams := getGithubTeams()
	if len(teams) == 0 {
		return nil, fmt.Errorf("no teams configured. Run: devctl-em metrics github config --team <team>")
	}

	var result []teamWorkflows
	for _, team := range teams {
		wf, err := getConfiguredWorkflowsByTeam(team)
		if err != nil {
			return nil, err
		}
		result = append(result, teamWorkflows{Team: team, Workflows: wf})
	}

	return result, nil
}

