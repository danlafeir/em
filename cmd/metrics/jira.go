package metrics

import (
	"fmt"
	"os"
	"time"

	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"devctl-em/pkg/jira"
	"devctl-em/pkg/workflow"
)

// JiraCmd is the parent command for all JIRA metrics.
var JiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "JIRA agile metrics and forecasting",
	Long: `Generate agile metrics from JIRA Cloud data including:

  - Cycle time analysis with scatter plots and histograms
  - Throughput trends over time
  - Cumulative Flow Diagrams (CFD)
  - Burn-up charts with Monte Carlo forecasting
  - WIP aging analysis

Configure your JIRA connection first:
  devctl-em config set jira.domain mycompany
  devctl-em config set jira.email user@company.com
  devctl-em secrets set jira.api_token your_api_token

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
	domain := viper.GetString("jira.domain")
	email := viper.GetString("jira.email")

	// Try to get token from secrets (keychain) first, fall back to env var
	token, err := secrets.DefaultSecretsProvider.Read("jira", "api_token")
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
		return nil, fmt.Errorf("JIRA API token not configured. Run: devctl-em secrets set jira.api_token <token>")
	}

	return jira.NewClient(jira.Credentials{
		Domain:   domain,
		Email:    email,
		APIToken: token,
	}), nil
}

// getJQL returns the JQL query to use (flag or config default).
func getJQL() (string, error) {
	if jqlFlag != "" {
		return jqlFlag, nil
	}
	jql := viper.GetString("jira.default_jql")
	if jql == "" {
		return "", fmt.Errorf("no JQL query provided. Use --jql flag or set jira.default_jql in config")
	}
	return jql, nil
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
		// Default to 90 days ago
		from = time.Now().AddDate(0, -3, 0)
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
	stages := viper.Get("workflow.stages")
	if stages == nil {
		// Use default configuration
		return workflow.NewMapper(workflow.DefaultConfig())
	}

	// Parse custom configuration
	config := workflow.DefaultConfig()

	// Override with custom stages if provided
	if stagesSlice, ok := stages.([]interface{}); ok {
		config.Stages = nil
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
				config.Stages = append(config.Stages, stage)
			}
		}
	}

	// Override cycle time config if provided
	if started := viper.GetString("workflow.cycle_time.started"); started != "" {
		config.CycleTime.Started = started
	}
	if completed := viper.GetString("workflow.cycle_time.completed"); completed != "" {
		config.CycleTime.Completed = completed
	}

	return workflow.NewMapper(config)
}

// getOutputPath returns the output file path with default extension.
func getOutputPath(defaultName, defaultExt string) string {
	if outputFlag != "" {
		return outputFlag
	}
	return defaultName + "." + defaultExt
}

// getOutputFormat returns the output format.
func getOutputFormat(defaultFormat string) string {
	if formatFlag != "" {
		return formatFlag
	}
	return defaultFormat
}
