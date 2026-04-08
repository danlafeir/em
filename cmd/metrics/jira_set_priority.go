package metrics

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var setPriorityCmd = &cobra.Command{
	Use:   "set-priority",
	Short: "Select which epics to include in forecasts",
	Long: `Interactively select which open epics to prioritize for forecasting.

Required:
  em metrics jira config`,
	RunE: runSetPriority,
}

func init() {
	JiraCmd.AddCommand(setPriorityCmd)
}

func runSetPriority(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getJiraClient()
	if err != nil {
		return err
	}

	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to JIRA: %w", err)
	}

	return withTeamIteration(ctx, client, func(team, jql string) error {
		fmt.Println("Discovering open epics...")
		epics, err := fetchOpenEpics(ctx, client, team)
		if err != nil {
			return err
		}
		if len(epics) == 0 {
			fmt.Println("No open epics found.")
			return nil
		}

		selected, err := promptEpicSelection(epics)
		if err != nil {
			return err
		}
		saveEpicSelection(team, selected)
		fmt.Printf("Priority saved: %d epic(s) selected for forecasting.\n", len(selected))
		return nil
	})
}
