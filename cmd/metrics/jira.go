package metrics

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"

	"devctl-em/internal/output"
	"devctl-em/internal/jira"
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

Set a project to automatically scope metrics to active epics:
  devctl-em config set jira.project MYPROJ

JQL resolution order: --jql flag > jira.default_jql config > jira.project config.
When jira.project is set, metrics are scoped to child issues of active (unresolved) epics.

Examples:
  devctl-em metrics jira cycle-time --jql "project = MYPROJ"
  devctl-em metrics jira throughput --from 2024-01-01 --to 2024-12-31
  devctl-em metrics jira forecast --epic MYPROJ-123`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Common flags for all jira subcommands
var (
	jqlFlag    string
	fromFlag   string
	toFlag     string
	outputFlag string
	formatFlag string
)

func init() {
	// Add jira command to metrics
	MetricsCmd.AddCommand(JiraCmd)

	// Define persistent flags for all jira subcommands
	JiraCmd.PersistentFlags().StringVar(&jqlFlag, "jql", "", "JQL query (overrides config default)")
	JiraCmd.PersistentFlags().StringVar(&fromFlag, "from", "", "Start date (YYYY-MM-DD)")
	JiraCmd.PersistentFlags().StringVar(&toFlag, "to", "", "End date (YYYY-MM-DD)")
	JiraCmd.PersistentFlags().StringVarP(&outputFlag, "output", "o", "", "Output file path")
	JiraCmd.PersistentFlags().StringVarP(&formatFlag, "format", "f", "", "Output format (png, csv, xlsx, html)")
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

// getJQL returns the JQL query from the flag or config default (no API calls).
func getJQL() (string, error) {
	if jqlFlag != "" {
		return jqlFlag, nil
	}
	jql := getConfigString("jira.default_jql")
	if jql == "" {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag or set jira.default_jql or jira.project in config")
	}
	return jql, nil
}

// resolveJQL returns the JQL query to use, with fallback to jira.project config.
// When jira.project is set, it queries JIRA for active epics and builds a
// children JQL to scope metrics to child issues of those epics.
func resolveJQL(ctx context.Context, client *jira.Client) (string, error) {
	// 1. --jql flag takes priority
	if jqlFlag != "" {
		return jqlFlag, nil
	}

	// 2. jira.default_jql config
	if jql := getConfigString("jira.default_jql"); jql != "" {
		return jql, nil
	}

	// 3. jira.project config — query for active epics
	project := getConfigString("jira.project")
	if project == "" {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag or set jira.default_jql or jira.project in config")
	}

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
	if jql := getConfigString("jira.default_jql"); jql != "" {
		return jql, nil
	}
	project := getConfigString("jira.project")
	if project == "" {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag or set jira.default_jql or jira.project in config")
	}
	return fmt.Sprintf("project = %s", project), nil
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
