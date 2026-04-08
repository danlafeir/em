package metrics

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/danlafeir/cli-go/pkg/config"
	"github.com/spf13/cobra"
)

var selectTeamCmd = &cobra.Command{
	Use:   "select-team",
	Short: "Select the active team for metrics commands",
	Long: `Select the active team assumed by all metrics commands. Use 0 to clear and revert to all teams.`,
	RunE: runSelectTeam,
}

func init() {
	MetricsCmd.AddCommand(selectTeamCmd)
}

func runSelectTeam(cmd *cobra.Command, args []string) error {
	initConfig()

	reader := bufio.NewReader(os.Stdin)

	// Direct argument: register if new, then select
	if len(args) == 1 {
		team := args[0]
		if err := registerTeamIfNew(team); err != nil {
			return err
		}
		return saveSelectedTeam(team)
	}

	current := getSelectedTeam()
	if current != "" {
		fmt.Printf("Current team: %s\n\n", current)
	}

	teams := getAllTeams()

	if len(teams) > 0 {
		fmt.Println("Configured teams:")
		for i, t := range teams {
			marker := ""
			if t == current {
				marker = " (current)"
			}
			fmt.Printf("  %d) %s%s\n", i+1, t, marker)
		}
		fmt.Printf("  0) Clear selection (use all teams)\n")
		fmt.Printf("  n) Add a new team\n")

		if current != "" {
			fmt.Printf("Select team [current: %s]: ", current)
		} else {
			fmt.Printf("Select team: ")
		}

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" && current != "" {
			fmt.Printf("Kept: %s\n", current)
			return nil
		}

		if input == "0" {
			return saveSelectedTeam("")
		}

		if input != "n" {
			choice, err := strconv.Atoi(input)
			if err != nil || choice < 1 || choice > len(teams) {
				return fmt.Errorf("invalid selection")
			}
			return saveSelectedTeam(teams[choice-1])
		}
	}

	// New team prompt (either "n" chosen or no teams exist yet)
	fmt.Print("New team name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("team name cannot be empty")
	}
	if err := registerTeamIfNew(name); err != nil {
		return err
	}
	return saveSelectedTeam(name)
}

// registerTeamIfNew adds the team to team_names if it isn't already there.
func registerTeamIfNew(team string) error {
	existing := getAllTeams()
	for _, t := range existing {
		if t == team {
			return nil
		}
	}
	updated := make([]any, 0, len(existing)+1)
	for _, t := range existing {
		updated = append(updated, t)
	}
	updated = append(updated, team)
	config.SetConfigValue(configNamespace, "team_names", updated)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("Added team: %s\n", team)
	return nil
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
	seen := make(map[string]bool)

	// Teams explicitly registered via metrics config.
	// team_names stores string values so original capitalization is preserved.
	if raw := getConfigAny("team_names"); raw != nil {
		switch v := raw.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					seen[s] = true
				}
			}
		case string:
			if v != "" {
				seen[v] = true
			}
		}
	}

	// Build a lookup from the lowercase slug → original name using team_names.
	// Viper lowercases all map keys, so teams.* slugs need this to recover case.
	slugToName := make(map[string]string, len(seen))
	for name := range seen {
		slugToName[strings.ToLower(name)] = name
	}

	// Teams that already have config under teams.*
	if raw := getConfigAny("teams"); raw != nil {
		if rawMap, ok := raw.(map[string]any); ok {
			for slug := range rawMap {
				if original, ok := slugToName[slug]; ok {
					seen[original] = true
				} else {
					seen[slug] = true
				}
			}
		}
	}

	teams := make([]string, 0, len(seen))
	for name := range seen {
		teams = append(teams, name)
	}
	sort.Strings(teams)
	return teams
}
