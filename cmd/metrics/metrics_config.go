package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/spf13/cobra"
)

var metricsConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure teams for metrics commands",
	Long: `Add and manage teams used across metrics commands.

Teams configured here are referenced when running jira and github config.

Examples:
  devctl-em metrics config`,
	RunE: runMetricsConfig,
}

func init() {
	MetricsCmd.AddCommand(metricsConfigCmd)
}

func runMetricsConfig(cmd *cobra.Command, args []string) error {
	initConfig()
	reader := bufio.NewReader(os.Stdin)

	for {
		existing := getAllTeams()
		if len(existing) > 0 {
			fmt.Println("Configured teams:")
			for i, t := range existing {
				fmt.Printf("  %d) %s\n", i+1, t)
			}
			fmt.Println()
		}

		fmt.Print("Add team name (or press Enter to finish): ")
		input, _ := reader.ReadString('\n')
		teamName := strings.TrimSpace(input)
		if teamName == "" {
			break
		}

		alreadyExists := false
		for _, t := range existing {
			if t == teamName {
				alreadyExists = true
				break
			}
		}
		if alreadyExists {
			fmt.Printf("Team %q is already configured.\n\n", teamName)
			continue
		}

		names := getAllTeams()
		updated := make([]any, 0, len(names)+1)
		for _, n := range names {
			updated = append(updated, n)
		}
		updated = append(updated, teamName)
		config.SetConfigValue(configNamespace, "team_names", updated)
		if err := config.WriteConfig(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("Added team: %s\n\n", teamName)
	}

	fmt.Println("Done.")
	fmt.Println("Run 'devctl-em metrics jira config' or 'devctl-em metrics github config' to configure service settings per team.")
	return nil
}

// pickTeam prompts the user to select a team from a list of existing teams.
func pickTeam(reader *bufio.Reader, teams []string) (string, error) {
	fmt.Println("Configured teams:")
	for i, t := range teams {
		fmt.Printf("  %d) %s\n", i+1, t)
	}
	fmt.Printf("Select team [1]: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		input = "1"
	}

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(teams) {
		return "", fmt.Errorf("invalid selection")
	}
	return teams[choice-1], nil
}
