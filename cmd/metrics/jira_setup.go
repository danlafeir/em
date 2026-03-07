package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"devctl-em/internal/jira"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var jiraSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive JIRA configuration",
	Long: `Interactively configure JIRA connection and team projects.

Prompts for:
  - JIRA domain (e.g. mycompany for mycompany.atlassian.net)
  - Email address
  - API token (stored in system keychain)
  - Team project keys

Existing values are shown and can be kept by pressing Enter.

Examples:
  devctl-em metrics jira setup
  devctl-em metrics jira setup --team my-team`,
	RunE: runJiraSetup,
}

func init() {
	JiraCmd.AddCommand(jiraSetupCmd)
}

func runJiraSetup(cmd *cobra.Command, args []string) error {
	initConfig()
	reader := bufio.NewReader(os.Stdin)

	// 1. Domain
	currentDomain := getConfigString("jira.domain")
	domain, err := promptValue(reader, "JIRA domain", currentDomain)
	if err != nil {
		return err
	}
	if domain != currentDomain {
		config.SetConfigValue(configNamespace, "jira.domain", domain)
	}

	// 2. Email
	currentEmail := getConfigString("jira.email")
	email, err := promptValue(reader, "JIRA email", currentEmail)
	if err != nil {
		return err
	}
	if email != currentEmail {
		config.SetConfigValue(configNamespace, "jira.email", email)
	}

	// 3. API token (keychain)
	existingToken, _ := secrets.Read("jira", "api_token")
	if existingToken != "" {
		fmt.Println("API token: configured")
		fmt.Print("Re-enter API token? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) == "y" {
			if err := promptAndStoreToken(); err != nil {
				return err
			}
		}
	} else {
		fmt.Print("Enter JIRA API token: ")
		if err := promptAndStoreToken(); err != nil {
			return err
		}
	}

	// 4. Teams loop
	for {
		team, err := resolveJiraSetupTeam(reader)
		if err != nil {
			return err
		}

		currentProject := getTeamConfigString(team, "project")
		project, err := promptValue(reader, fmt.Sprintf("Project key for team %q", team), currentProject)
		if err != nil {
			return err
		}
		config.SetConfigValue(configNamespace, fmt.Sprintf("jira.teams.%s.project", team), project)
		fmt.Printf("Set jira.teams.%s.project = %s\n", team, project)

		// Offer board-based JQL selection
		if err := promptBoardJQL(reader, team, project); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch boards: %v\n", err)
		}

		fmt.Print("Add another team? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			break
		}
	}

	// Save config
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("JIRA configuration saved.")
	return nil
}

// promptValue shows the current value and prompts for a new one.
// Pressing Enter keeps the current value.
func promptValue(reader *bufio.Reader, label, current string) (string, error) {
	if current != "" {
		fmt.Printf("%s [%s]: ", label, current)
	} else {
		fmt.Printf("%s: ", label)
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)

	if input == "" {
		if current == "" {
			return "", fmt.Errorf("%s is required", label)
		}
		return current, nil
	}
	return input, nil
}

// promptAndStoreToken reads a password from the terminal and stores it in the keychain.
func promptAndStoreToken() error {
	var token string
	if term.IsTerminal(int(syscall.Stdin)) {
		byteValue, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read token: %w", err)
		}
		token = string(byteValue)
	} else {
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read token: %w", err)
		}
		token = strings.TrimSpace(input)
	}

	if token == "" {
		return fmt.Errorf("API token is required")
	}

	if err := secrets.Write("jira", "api_token", token); err != nil {
		return fmt.Errorf("failed to store API token: %w", err)
	}
	fmt.Println("API token stored in keychain.")
	return nil
}

// resolveJiraSetupTeam determines which team to configure.
// If --team is set, use that. Otherwise list existing teams or prompt for a new slug.
func resolveJiraSetupTeam(reader *bufio.Reader) (string, error) {
	if jiraTeamFlag != "" {
		return jiraTeamFlag, nil
	}

	existingTeams := getJiraTeams()

	if len(existingTeams) > 0 {
		fmt.Println("Configured teams:")
		for i, t := range existingTeams {
			project := getTeamConfigString(t, "project")
			if project != "" {
				fmt.Printf("  %d) %s (project: %s)\n", i+1, t, project)
			} else {
				fmt.Printf("  %d) %s\n", i+1, t)
			}
		}
		fmt.Printf("  %d) Add new team\n", len(existingTeams)+1)
		fmt.Printf("Select team [%d]: ", len(existingTeams)+1)

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			input = strconv.Itoa(len(existingTeams) + 1)
		}

		choice, err := strconv.Atoi(input)
		if err == nil && choice >= 1 && choice <= len(existingTeams) {
			return existingTeams[choice-1], nil
		}
	}

	fmt.Print("Enter team name: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return "", fmt.Errorf("team name is required")
	}

	return input, nil
}

// promptBoardJQL queries JIRA for boards in the project and lets the user
// pick one to use its filter JQL as the team's jql_filter_for_metrics.
func promptBoardJQL(reader *bufio.Reader, team, projectKey string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	boards, err := client.ListBoards(ctx, projectKey)
	if err != nil {
		return err
	}

	if len(boards) == 0 {
		fmt.Println("No boards found for project", projectKey)
		return nil
	}

	fmt.Println("\nBoards found:")
	for i, b := range boards {
		fmt.Printf("  %d) %s (%s)\n", i+1, b.Name, b.Type)
	}
	fmt.Printf("  0) Skip — don't set default JQL\n")
	fmt.Print("Select board [0]: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" || input == "0" {
		return nil
	}

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(boards) {
		fmt.Println("Invalid selection, skipping board JQL.")
		return nil
	}

	board := boards[choice-1]
	return fetchAndStoreBoardJQL(ctx, client, team, board)
}

// fetchAndStoreBoardJQL retrieves the board's filter JQL and stores it as the team's jql_filter_for_metrics.
func fetchAndStoreBoardJQL(ctx context.Context, client *jira.Client, team string, board jira.Board) error {
	boardCfg, err := client.GetBoardConfiguration(ctx, board.ID)
	if err != nil {
		return fmt.Errorf("getting board config: %w", err)
	}

	filter, err := client.GetFilter(ctx, boardCfg.Filter.ID)
	if err != nil {
		return fmt.Errorf("getting filter: %w", err)
	}

	key := fmt.Sprintf("jira.teams.%s.jql_filter_for_metrics", team)
	jql := stripOrderBy(filter.JQL)
	config.SetConfigValue(configNamespace, key, jql)
	fmt.Printf("Set %s = %s\n", key, jql)
	return nil
}

func stripOrderBy(jql string) string {
	idx := strings.Index(strings.ToUpper(jql), "ORDER ")
	if idx < 0 {
		return jql
	}
	return strings.TrimSpace(jql[:idx])
}
