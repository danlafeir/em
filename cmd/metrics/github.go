package metrics

import (
	"fmt"
	"sort"
	"time"

	"github.com/danlafeir/cli-go/pkg/secrets"
	"github.com/spf13/cobra"

	"github.com/danlafeir/em/internal/github"
	"github.com/danlafeir/em/internal/output"
)

// GithubCmd is the parent command for all GitHub metrics.
var GithubCmd = &cobra.Command{
	Use:   "github",
	Short: "GitHub metrics",
	Long: `Generate DORA metrics from GitHub Actions.

Required:
  em metrics github config`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Flags for github subcommands (separate from jira flags)
var (
	ghFromFlag string
	ghToFlag   string
)

func init() {
	MetricsCmd.AddCommand(GithubCmd)

	GithubCmd.PersistentFlags().StringVar(&ghFromFlag, "from", "", "Start date (YYYY-MM-DD)")
	GithubCmd.PersistentFlags().StringVar(&ghToFlag, "to", "", "End date (YYYY-MM-DD)")
}

// getGithubClient creates a GitHub client from configuration.
func getGithubClient() (*github.Client, error) {
	if activeMock.githubClient != nil {
		return activeMock.githubClient, nil
	}

	token, err := secrets.Read("github", "api_token")
	if err != nil || token == "" {
		return nil, fmt.Errorf("GitHub API token not configured. Run: em config set github.api_token")
	}

	creds := github.Credentials{
		Token: token,
		Org:   getGithubOrg(),
	}

	return github.NewClient(creds)
}

// getGithubOrg returns the GitHub org from config.
func getGithubOrg() string {
	return getConfigString("github.org")
}

// getGithubTeams returns all configured team names that have github config.
// Falls back to the selected team (from select-team) before listing all teams.
func getGithubTeams() []string {
	if selected := getSelectedTeam(); selected != "" {
		return []string{selected}
	}

	raw := getConfigAny("teams")
	if raw == nil {
		return nil
	}

	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	var teams []string
	for name, v := range rawMap {
		sub, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if _, hasGithub := sub["github"]; hasGithub {
			teams = append(teams, name)
		}
	}
	sort.Strings(teams)
	return teams
}

// getGithubDateRange returns the from/to date range for GitHub commands.
func getGithubDateRange() (time.Time, time.Time, error) {
	return parseDateRange(ghFromFlag, ghToFlag)
}

// getGithubOutputPath returns the output file path.
func getGithubOutputPath(defaultName, defaultExt string) string {
	return output.Path(defaultName + "." + defaultExt)
}

// getConfiguredWorkflowsByTeam reads workflows for a specific team from config.
// Returns a map of repo name → workflow filenames (supports multiple per repo).
func getConfiguredWorkflowsByTeam(team string) (map[string][]string, error) {
	key := fmt.Sprintf("teams.%s.github.workflows", team)
	raw := getConfigAny(key)
	if raw == nil {
		return nil, fmt.Errorf("no workflows configured for team %q. Run: em metrics github config", team)
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
		return nil, fmt.Errorf("no workflows configured for team %q. Run: em metrics github config", team)
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
		return nil, fmt.Errorf("no teams configured. Run: em metrics github config")
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

