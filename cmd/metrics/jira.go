package metrics

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"

	"devctl-em/internal/jira"
	"devctl-em/internal/output"
	"devctl-em/internal/workflow"
)

// configNamespace is the namespace for devctl-em config
const configNamespace = "em"

func initConfig() {
	config.InitConfig("")
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
  devctl-em config set jira.domain mycompany
  devctl-em config set jira.email user@company.com
  devctl-em config set jira.api_token

Configure teams with their projects:
  devctl-em config set jira.teams.platform.project PLAT
  devctl-em config set jira.teams.backend.project API
  devctl-em config set jira.teams.backend.jql_filter_for_metrics "project = API AND ..."

JQL resolution order: --jql flag > --project flag > team jql_filter_for_metrics > team project config.
Use --team to filter to a single team; omit it to aggregate all teams.

Examples:
  devctl-em metrics jira cycle-time --team platform
  devctl-em metrics jira throughput --from 2024-01-01 --to 2024-12-31
  devctl-em metrics jira forecast --team backend --epic API-123`,
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
	JiraCmd.PersistentFlags().StringVarP(&formatFlag, "format", "f", "", "Output format (png, csv, xlsx, html)")

	migrateJiraConfig()
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
		return nil, fmt.Errorf("JIRA domain not configured. Run: devctl-em config set jira.domain <domain>")
	}
	if email == "" {
		return nil, fmt.Errorf("JIRA email not configured. Run: devctl-em config set jira.email <email>")
	}
	if token == "" {
		return nil, fmt.Errorf("JIRA API token not configured. Run: devctl-em config set jira.api_token")
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

// migrateJiraConfig migrates old-style jira.project + jira.default_jql
// to the new jira.teams.<slug>.* format, and renames default_jql to
// jql_filter_for_metrics in existing team configs.
func migrateJiraConfig() {
	initConfig()

	changed := false

	// Migrate old top-level keys to team format
	oldProject := getConfigString("jira.project")
	oldJQL := getConfigString("jira.default_jql")
	if oldProject != "" || oldJQL != "" {
		slug := "default"
		if oldProject != "" {
			slug = strings.ToLower(oldProject)
		}

		if oldProject != "" {
			config.SetConfigValue(configNamespace, fmt.Sprintf("jira.teams.%s.project", slug), oldProject)
		}
		if oldJQL != "" {
			config.SetConfigValue(configNamespace, fmt.Sprintf("jira.teams.%s.jql_filter_for_metrics", slug), oldJQL)
		}

		config.DeleteConfigValue(configNamespace, "jira.project")
		config.DeleteConfigValue(configNamespace, "jira.default_jql")
		changed = true
		fmt.Printf("Migrated JIRA config to jira.teams.%s\n", slug)
	}

	// Rename default_jql to jql_filter_for_metrics in existing teams
	if raw := getConfigAny("jira.teams"); raw != nil {
		if rawMap, ok := raw.(map[string]any); ok {
			for slug, v := range rawMap {
				teamMap, ok := v.(map[string]any)
				if !ok {
					continue
				}
				if jql, exists := teamMap["default_jql"]; exists {
					config.SetConfigValue(configNamespace, fmt.Sprintf("jira.teams.%s.jql_filter_for_metrics", slug), jql)
					config.DeleteConfigValue(configNamespace, fmt.Sprintf("jira.teams.%s.default_jql", slug))
					changed = true
					fmt.Printf("Renamed jira.teams.%s.default_jql → jql_filter_for_metrics\n", slug)
				}
			}
		}
	}

	if changed {
		config.WriteConfig()
	}
}

// getJiraTeams returns all configured team slugs, or just the --team flag if set.
func getJiraTeams() []string {
	if jiraTeamFlag != "" {
		return []string{jiraTeamFlag}
	}

	raw := getConfigAny("jira.teams")
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

// getTeamConfigString reads jira.teams.<team>.<key> from config.
func getTeamConfigString(team, key string) string {
	return getConfigString(fmt.Sprintf("jira.teams.%s.%s", team, key))
}

// getJQL returns the JQL query from the flag or config default (no API calls).
func getJQL() (string, error) {
	if jqlFlag != "" {
		return jqlFlag, nil
	}

	teams := getJiraTeams()
	if len(teams) == 0 {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag, --project flag, or configure jira.teams in config")
	}

	var parts []string
	for _, team := range teams {
		if jql := getTeamConfigString(team, "jql_filter_for_metrics"); jql != "" {
			parts = append(parts, "("+jql+")")
		}
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag, --project flag, or set jql_filter_for_metrics for teams in config")
	}
	return strings.Join(parts, " OR "), nil
}

// resolveJQL returns the JQL query to use, with fallback to team project configs.
// When a team's project is set, it queries JIRA for active epics and builds a
// children JQL to scope metrics to child issues of those epics.
func resolveJQL(ctx context.Context, client *jira.Client) (string, error) {
	// 1. --jql flag takes priority
	if jqlFlag != "" {
		return jqlFlag, nil
	}

	// 2. --project flag — single project override
	if projectFlag != "" {
		return resolveProjectEpics(ctx, client, projectFlag)
	}

	// 3. Team-based resolution
	teams := getJiraTeams()
	if len(teams) == 0 {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag, --project flag, or configure jira.teams in config")
	}

	var parts []string
	for _, team := range teams {
		// Team jql_filter_for_metrics takes priority over team project
		if jql := getTeamConfigString(team, "jql_filter_for_metrics"); jql != "" {
			parts = append(parts, "("+jql+")")
			continue
		}
		project := getTeamConfigString(team, "project")
		if project == "" {
			continue
		}
		teamJQL, err := resolveProjectEpics(ctx, client, project)
		if err != nil {
			return "", fmt.Errorf("team %s: %w", team, err)
		}
		parts = append(parts, "("+teamJQL+")")
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag, --project flag, or configure teams with project or jql_filter_for_metrics")
	}
	return strings.Join(parts, " OR "), nil
}

// resolveProjectEpics queries JIRA for active epics in a project and returns
// a JQL that scopes to child issues of those epics.
func resolveProjectEpics(ctx context.Context, client *jira.Client, project string) (string, error) {
	epicJQL := fmt.Sprintf("project = %s AND issuetype = Epic AND resolution IS EMPTY", project)
	epics, err := client.SearchAllIssues(ctx, epicJQL, "key", "")
	if err != nil {
		return "", fmt.Errorf("failed to fetch active epics for project %s: %w", project, err)
	}

	if len(epics) == 0 {
		return "", fmt.Errorf("no active epics found in project %s", project)
	}

	keys := make([]string, len(epics))
	for i, e := range epics {
		keys[i] = e.Key
	}
	keyList := strings.Join(keys, ", ")

	return fmt.Sprintf("\"Epic Link\" in (%s) OR parent in (%s)", keyList, keyList), nil
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
	var from, to time.Time
	var err error

	if fromFlag != "" {
		from, err = time.Parse("2006-01-02", fromFlag)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --from date: %w", err)
		}
	} else {
		// Default to 42 days (6 weeks)
		from = time.Now().AddDate(0, 0, -42)
	}

	if toFlag != "" {
		to, err = time.Parse("2006-01-02", toFlag)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --to date: %w", err)
		}
	} else {
		to = time.Now()
	}

	return from, to, nil
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
