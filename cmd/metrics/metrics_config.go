package metrics

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var metricsConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Interactive configuration for all metrics services",
	Long: `Runs the interactive configuration for each metrics service in sequence.

Configures JIRA, GitHub, and Snyk in order. Each section can be skipped
by pressing Ctrl+C, but errors are shown and configuration continues.

Examples:
  em metrics config`,
	RunE: runMetricsConfig,
}

func init() {
	MetricsCmd.AddCommand(metricsConfigCmd)
}

// EnsureTeamSelected checks that at least one team exists and one is selected.
// If not, it runs the select-team flow inline before returning.
func EnsureTeamSelected(cmd *cobra.Command, args []string) error {
	return ensureTeamSelected(cmd, args)
}

func ensureTeamSelected(cmd *cobra.Command, args []string) error {
	teams := getAllTeams()
	selected := getSelectedTeam()
	if len(teams) == 0 || selected == "" {
		if len(teams) == 0 {
			fmt.Println("No teams configured yet. Let's set one up first.")
		} else {
			fmt.Println("No team selected. Please select a team before configuring.")
		}
		fmt.Println()
		if err := runSelectTeam(cmd, args); err != nil {
			return fmt.Errorf("team selection required: %w", err)
		}
		fmt.Println()
	}
	return nil
}

func runMetricsConfig(cmd *cobra.Command, args []string) error {
	initConfig()
	if err := ensureTeamSelected(cmd, args); err != nil {
		return err
	}

	fmt.Println("=== JIRA ===")
	if err := runJiraConfig(cmd, args); err != nil {
		fmt.Printf("Warning: JIRA config failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("=== GitHub ===")
	if err := runGhConfig(cmd, args); err != nil {
		fmt.Printf("Warning: GitHub config failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("=== Snyk ===")
	if err := runSnykConfig(cmd, args); err != nil {
		fmt.Printf("Warning: Snyk config failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Configuration complete.")
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
