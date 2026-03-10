package metrics

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/spf13/cobra"
)

var selectTeamCmd = &cobra.Command{
	Use:   "select-team",
	Short: "Set the active team for metrics commands",
	Long: `Set the active team assumed by metrics subcommands.

When a team is selected, jira and github metrics commands use it
without requiring --team. Use 0 to clear the selection and revert
to all configured teams.

Examples:
  devctl-em metrics select-team
  devctl-em metrics select-team my-team`,
	RunE: runSelectTeam,
}

func init() {
	MetricsCmd.AddCommand(selectTeamCmd)
}

func runSelectTeam(cmd *cobra.Command, args []string) error {
	initConfig()

	// Direct argument
	if len(args) == 1 {
		team := args[0]
		return saveSelectedTeam(team)
	}

	current := getSelectedTeam()
	if current != "" {
		fmt.Printf("Current team: %s\n\n", current)
	}

	teams := getAllTeams()
	if len(teams) == 0 {
		return fmt.Errorf("no teams configured. Run: devctl-em metrics jira config or devctl-em metrics github config")
	}

	fmt.Println("Configured teams:")
	for i, t := range teams {
		marker := ""
		if t == current {
			marker = " (current)"
		}
		fmt.Printf("  %d) %s%s\n", i+1, t, marker)
	}
	fmt.Printf("  0) Clear selection (use all teams)\n")

	if current != "" {
		fmt.Printf("Select team [current: %s]: ", current)
	} else {
		fmt.Printf("Select team: ")
	}

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" && current != "" {
		fmt.Printf("Kept: %s\n", current)
		return nil
	}

	if input == "0" {
		return saveSelectedTeam("")
	}

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(teams) {
		return fmt.Errorf("invalid selection")
	}

	return saveSelectedTeam(teams[choice-1])
}

func saveSelectedTeam(team string) error {
	if team == "" {
		config.SetConfigValue(configNamespace, "selected_team", "")
		if err := config.WriteConfig(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("Team selection cleared.")
		return nil
	}

	config.SetConfigValue(configNamespace, "selected_team", team)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("Selected team: %s\n", team)
	return nil
}

// getAllTeams returns all team names from config regardless of which services are configured.
func getAllTeams() []string {
	raw := getConfigAny("teams")
	if raw == nil {
		return nil
	}

	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	teams := make([]string, 0, len(rawMap))
	for name := range rawMap {
		teams = append(teams, name)
	}
	sort.Strings(teams)
	return teams
}
