package metrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/danlafeir/cli-go/pkg/config"
	"github.com/danlafeir/cli-go/pkg/secrets"
	"github.com/spf13/cobra"

	"em/internal/charts"
	"em/internal/jira"
	"em/internal/output"
	"em/internal/workflow"
)

// configNamespace is the namespace for em config
const configNamespace = "em"

// skipBrowserOpen suppresses openBrowser calls when set to true.
// Used by runMetricsReport so the browser only opens after all reports finish.
var skipBrowserOpen bool

// openBrowser opens the file in the default browser unless suppressed.
func openBrowser(path string) {
	if skipBrowserOpen {
		return
	}
	charts.OpenBrowser(path) //nolint:errcheck
}

func emConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".em"
	}
	return filepath.Join(home, ".em")
}

func initConfig() {
	config.InitConfig(emConfigDir()) //nolint:errcheck
}

// getConfigString returns a string config value from the em namespace.
func getConfigString(key string) string {
	initConfig()
	val, ok := config.GetConfigValue(configNamespace, key)
	if !ok {
		return ""
	}
	s, _ := val.(string)
	return s
}

// getConfigValue returns a config value from the em namespace.
func getConfigAny(key string) interface{} {
	initConfig()
	val, _ := config.GetConfigValue(configNamespace, key)
	return val
}

// JiraCmd is the parent command for all JIRA metrics.
var JiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "JIRA agile metrics and forecasting",
	Long: `Generate agile metrics from JIRA Cloud data including:

  - Cycle time analysis with scatter plots and histograms
  - Throughput trends over time
  - Monte Carlo forecasting

Configure your JIRA connection first:
  em config set jira.domain mycompany
  em config set jira.email user@company.com
  em config set jira.api_token

Configure teams with their projects:
  em config set jira.teams.platform.project PLAT
  em config set jira.teams.backend.project API
  em config set jira.teams.backend.jql_filter_for_metrics "project = API AND ..."

JQL resolution order: --jql flag > --project flag > team jql_filter_for_metrics > team project config.
Use --team to filter to a single team; omit it to aggregate all teams.

Examples:
  em metrics jira cycle-time --team platform
  em metrics jira throughput --from 2024-01-01 --to 2024-12-31
  em metrics jira forecast --team backend --epic API-123`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Common flags for all jira subcommands
var (
	jqlFlag      string
	projectFlag  string
	jiraTeamFlag string
	fromFlag     string
	toFlag       string
	outputFlag   string
	formatFlag   string
)

func init() {
	// Add jira command to metrics
	MetricsCmd.AddCommand(JiraCmd)

	// Define persistent flags for all jira subcommands
	JiraCmd.PersistentFlags().StringVar(&jqlFlag, "jql", "", "JQL query (overrides config default)")
	JiraCmd.PersistentFlags().StringVar(&projectFlag, "project", "", "JIRA project key (overrides config)")
	JiraCmd.PersistentFlags().StringVar(&jiraTeamFlag, "team", "", "JIRA team slug (filters to one team)")
	JiraCmd.PersistentFlags().StringVar(&fromFlag, "from", "", "Start date (YYYY-MM-DD)")
	JiraCmd.PersistentFlags().StringVar(&toFlag, "to", "", "End date (YYYY-MM-DD)")
	JiraCmd.PersistentFlags().StringVarP(&outputFlag, "output", "o", "", "Output file path")
	JiraCmd.PersistentFlags().StringVarP(&formatFlag, "format", "f", "", "Output format (html, csv, xlsx)")

	initConfig()
	if unrecognized := config.ValidateNamespace(configNamespace, emConfigSchema); len(unrecognized) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: unrecognized config keys (will clear namespace): %s\n", strings.Join(unrecognized, ", "))
		config.ClearNamespace(configNamespace)
	}
}

// getJiraClient creates a JIRA client from configuration.
func getJiraClient() (*jira.Client, error) {
	domain := getConfigString("jira.domain")
	email := getConfigString("jira.email")

	// Try to get token from secrets (keychain) first, fall back to env var
	token, err := secrets.Read("jira", "api_token")
	if err != nil || token == "" {
		token = os.Getenv("JIRA_API_TOKEN")
	}

	if domain == "" {
		return nil, fmt.Errorf("JIRA domain not configured. Run: em config set jira.domain <domain>")
	}
	if email == "" {
		return nil, fmt.Errorf("JIRA email not configured. Run: em config set jira.email <email>")
	}
	if token == "" {
		return nil, fmt.Errorf("JIRA API token not configured. Run: em config set jira.api_token")
	}

	creds := jira.Credentials{
		Domain:   domain,
		Email:    email,
		APIToken: token,
	}

	if override := os.Getenv("JIRA_BASE_URL"); override != "" {
		creds.BaseURLOverride = override
	}

	return jira.NewClient(creds), nil
}

// emConfigSchema lists all valid config keys under the "em" namespace.
var emConfigSchema = config.ConfigSchema{
	"selected_team",
	"team_names",
	"jira.domain",
	"jira.email",
	"jira.work_threads",
	"jira.selected_epics",
	"teams.*",
	"teams.*.jira.project",
	"teams.*.jira.jql_filter_for_metrics",
	"teams.*.jira.selected_epics",
	"teams.*.github.slug",
	"teams.*.github.workflows",
	"teams.*.github.workflows.*",
	"workflow.stages",
	"workflow.cycle_time.started",
	"workflow.cycle_time.completed",
	"montecarlo.deadline",
	"github.org",
	"snyk.org_id",
	"snyk.org_name",
	"snyk.site",
	"snyk.team",
	"datadog.site",
	"datadog.team",
}

// getJiraTeams returns all configured team names that have jira config, or just the --team flag if set.
// Falls back to the selected team (from select-team) before listing all teams.
func getJiraTeams() []string {
	if jiraTeamFlag != "" {
		return []string{jiraTeamFlag}
	}
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
		if _, hasJira := sub["jira"]; hasJira {
			teams = append(teams, name)
		}
	}
	sort.Strings(teams)
	return teams
}

// getTeamConfigString reads teams.<team>.jira.<key> from config.
func getTeamConfigString(team, key string) string {
	return getConfigString(fmt.Sprintf("teams.%s.jira.%s", team, key))
}

// getSelectedTeam returns the team set by select-team, or "" if none.
func getSelectedTeam() string {
	return getConfigString("selected_team")
}

// epicSelectionKey returns the config key for the saved epic selection for a team.
func epicSelectionKey(team string) string {
	if team != "" {
		return fmt.Sprintf("teams.%s.jira.selected_epics", team)
	}
	return "jira.selected_epics"
}

// loadEpicSelection returns the saved epic keys for the team, or nil if none saved.
func loadEpicSelection(team string) []string {
	raw := getConfigAny(epicSelectionKey(team))
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		keys := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				keys = append(keys, s)
			}
		}
		return keys
	case string:
		return []string{v}
	}
	return nil
}

// saveEpicSelection persists the selected epic keys for a team.
func saveEpicSelection(team string, epics []jira.Issue) {
	anyKeys := make([]any, len(epics))
	for i, e := range epics {
		anyKeys[i] = e.Key
	}
	config.SetConfigValue(configNamespace, epicSelectionKey(team), anyKeys)
	config.WriteConfig()
}


// getProjectJQL returns a simple "project = PROJ" JQL from config.
// Used by forecast for epic discovery where the full children JQL is not needed.
func getProjectJQL() (string, error) {
	if jqlFlag != "" {
		return jqlFlag, nil
	}

	// --project flag override
	if projectFlag != "" {
		return fmt.Sprintf("project = %s", projectFlag), nil
	}

	// Team-based resolution
	teams := getJiraTeams()
	if len(teams) == 0 {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag, --project flag, or configure jira.teams in config")
	}

	// Check for jql_filter_for_metrics first, then collect projects
	var jqlParts []string
	var projects []string
	for _, team := range teams {
		if jql := getTeamConfigString(team, "jql_filter_for_metrics"); jql != "" {
			jqlParts = append(jqlParts, "("+jql+")")
			continue
		}
		if project := getTeamConfigString(team, "project"); project != "" {
			projects = append(projects, project)
		}
	}

	// Combine: teams with jql_filter_for_metrics OR-ed, projects combined into project in (...)
	if len(projects) > 0 {
		if len(projects) == 1 {
			jqlParts = append(jqlParts, fmt.Sprintf("project = %s", projects[0]))
		} else {
			jqlParts = append(jqlParts, fmt.Sprintf("project in (%s)", strings.Join(projects, ", ")))
		}
	}

	if len(jqlParts) == 0 {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag, --project flag, or configure teams with project or jql_filter_for_metrics")
	}
	return strings.Join(jqlParts, " OR "), nil
}

// getDateRange returns the from/to date range.
func getDateRange() (time.Time, time.Time, error) {
	return parseDateRange(fromFlag, toFlag)
}

// mapIssuesToHistories converts JIRA issues to workflow histories using the
// configured mapper. Returns both the histories and the mapper so callers can
// pass the mapper to compute functions.
func mapIssuesToHistories(issues []jira.IssueWithHistory) ([]workflow.IssueHistory, *workflow.Mapper) {
	mapper := getWorkflowMapper()
	histories := make([]workflow.IssueHistory, len(issues))
	for i, issue := range issues {
		histories[i] = mapper.MapIssueHistory(issue)
	}
	return histories, mapper
}

// fetchAndMapIssues fetches issues with history for the given JQL, prints
// standard progress to stdout, and maps them to workflow histories.
// Use this in individual commands; for verbose/conditional output use
// FetchIssuesWithHistory + mapIssuesToHistories directly.
func fetchAndMapIssues(ctx context.Context, client *jira.Client, jql string) ([]workflow.IssueHistory, *workflow.Mapper, error) {
	issues, err := client.FetchIssuesWithHistory(ctx, jql, func(current, total int) {
		fmt.Printf("\rProcessing issue %d/%d...", current, total)
	})
	if err != nil {
		return nil, nil, err
	}
	fmt.Println()
	histories, mapper := mapIssuesToHistories(issues)
	return histories, mapper, nil
}

// getWorkflowMapper creates a workflow mapper from configuration.
func getWorkflowMapper() *workflow.Mapper {
	// Check for custom workflow configuration
	stages := getConfigAny("workflow.stages")
	if stages == nil {
		// Use default configuration
		return workflow.NewMapper(workflow.DefaultConfig())
	}

	// Parse custom configuration
	wfConfig := workflow.DefaultConfig()

	// Override with custom stages if provided
	if stagesSlice, ok := stages.([]interface{}); ok {
		wfConfig.Stages = nil
		for i, s := range stagesSlice {
			if stageMap, ok := s.(map[string]interface{}); ok {
				stage := workflow.Stage{Order: i}

				if name, ok := stageMap["name"].(string); ok {
					stage.Name = name
				}
				if category, ok := stageMap["category"].(string); ok {
					stage.Category = category
				}
				if statuses, ok := stageMap["statuses"].([]interface{}); ok {
					for _, status := range statuses {
						if statusStr, ok := status.(string); ok {
							stage.Statuses = append(stage.Statuses, statusStr)
						}
					}
				}
				wfConfig.Stages = append(wfConfig.Stages, stage)
			}
		}
	}

	// Override cycle time config if provided
	if started := getConfigString("workflow.cycle_time.started"); started != "" {
		wfConfig.CycleTime.Started = started
	}
	if completed := getConfigString("workflow.cycle_time.completed"); completed != "" {
		wfConfig.CycleTime.Completed = completed
	}

	return workflow.NewMapper(wfConfig)
}

// resolveTeamJQL resolves JQL for a single team. Prefers the board's
// jql_filter_for_metrics, falling back to a plain project = X constraint.
func resolveTeamJQL(team string) (string, error) {
	if jql := getTeamConfigString(team, "jql_filter_for_metrics"); jql != "" {
		return jql, nil
	}
	project := getTeamConfigString(team, "project")
	if project == "" {
		return "", fmt.Errorf("no jql_filter_for_metrics or project configured for team %s", team)
	}
	return fmt.Sprintf("project = %s", project), nil
}

// withTeamIteration runs fn once per configured team, or once with aggregated
// JQL when --jql/--project is set. The callback receives the team slug (empty
// for non-team mode) and the resolved JQL.
func withTeamIteration(ctx context.Context, client *jira.Client, fn func(team, jql string) error) error {
	if jqlFlag != "" {
		return fn("", jqlFlag)
	}
	if projectFlag != "" {
		return fn("", fmt.Sprintf("project = %s", projectFlag))
	}

	teams := getJiraTeams()
	if len(teams) == 0 {
		return fmt.Errorf("no JQL query provided. Use --jql flag, --project flag, or configure jira.teams in config")
	}

	for _, team := range teams {
		fmt.Printf("=== Team: %s ===\n\n", team)

		jql, err := resolveTeamJQL(team)
		if err != nil {
			return fmt.Errorf("team %s: %w", team, err)
		}

		if err := fn(team, jql); err != nil {
			return fmt.Errorf("team %s: %w", team, err)
		}

		fmt.Println()
	}

	return nil
}

// teamOutputName returns defaultName with the team slug appended if non-empty.
func teamOutputName(defaultName, team string) string {
	if team != "" {
		return defaultName + "-" + team
	}
	return defaultName
}

// getTeamProjectJQL returns the base JQL for a single team.
// Prefers jql_filter_for_metrics (board query), falling back to project = X.
func getTeamProjectJQL(team string) (string, error) {
	if jql := getTeamConfigString(team, "jql_filter_for_metrics"); jql != "" {
		return jql, nil
	}
	if project := getTeamConfigString(team, "project"); project != "" {
		return fmt.Sprintf("project = %s", project), nil
	}
	return "", fmt.Errorf("team %s has no jql_filter_for_metrics or project configured", team)
}

// getOutputPath returns the output file path with default extension.
func getOutputPath(defaultName, defaultExt string) string {
	if outputFlag != "" {
		return outputFlag
	}
	return output.Path(defaultName + "." + defaultExt)
}

// getOutputFormat returns the output format.
func getOutputFormat(defaultFormat string) string {
	if formatFlag != "" {
		return formatFlag
	}
	return defaultFormat
}

// jqlWithDateRange prepends resolved date filters to a JQL string. If the JQL
// contains an ORDER BY clause it is moved to the end of the combined query so
// that it remains syntactically valid.
func jqlWithDateRange(jql, from, to string) string {
	query, orderBy := splitOrderBy(jql)
	result := fmt.Sprintf("resolved >= %s AND resolved <= %s AND (%s)", from, to, query)
	if orderBy != "" {
		result += " " + orderBy
	}
	return result
}

// splitOrderBy splits a JQL string into the filter portion and the trailing
// ORDER BY clause (if any). The returned orderBy includes the "ORDER BY" prefix.
func splitOrderBy(jql string) (filter, orderBy string) {
	upper := strings.ToUpper(jql)
	idx := strings.LastIndex(upper, "ORDER BY")
	if idx < 0 {
		return jql, ""
	}
	return strings.TrimSpace(jql[:idx]), strings.TrimSpace(jql[idx:])
}

// wrapString wraps s into lines of at most maxWidth characters, breaking on spaces.
func wrapString(s string, maxWidth int) []string {
	if len(s) <= maxWidth {
		return []string{s}
	}
	var lines []string
	for len(s) > 0 {
		if len(s) <= maxWidth {
			lines = append(lines, s)
			break
		}
		// Find last space within maxWidth
		cut := maxWidth
		for cut > 0 && s[cut] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = maxWidth // no space found, hard break
		}
		lines = append(lines, s[:cut])
		s = s[cut:]
		if len(s) > 0 && s[0] == ' ' {
			s = s[1:]
		}
	}
	return lines
}
