package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"em/internal/jira"

	"github.com/danlafeir/cli-go/pkg/config"
	"github.com/danlafeir/cli-go/pkg/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var jiraConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Interactive JIRA configuration",
	Long: `Interactively configure JIRA connection and team project settings.`,
	RunE: runJiraConfig,
}

func init() {
	JiraCmd.AddCommand(jiraConfigCmd)
}

func runJiraConfig(cmd *cobra.Command, args []string) error {
	initConfig()
	if err := ensureTeamSelected(cmd, args); err != nil {
		return err
	}
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
		if err := promptAndStoreToken(); err != nil {
			return err
		}
	}

	// 4. Configure selected team
	team := getSelectedTeam()

	currentProject := getTeamConfigString(team, "project")
	project, err := promptValue(reader, fmt.Sprintf("Project key for team %q", team), currentProject)
	if err != nil {
		return err
	}
	config.SetConfigValue(configNamespace, fmt.Sprintf("teams.%s.jira.project", team), project)
	fmt.Printf("Set teams.%s.jira.project = %s\n", team, project)

	// Offer board-based JQL selection
	if err := promptBoardJQL(reader, team, project); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch boards: %v\n", err)
	}

	// 5. Epic priorities — select and order open epics for sequential forecasting
	fmt.Println()
	if err := promptEpicPriorities(reader, team); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not configure epic priorities: %v\n", err)
	}

	// 6. Parallel work capacity (used for forecasting)
	currentWorkers := getTeamConfigString(team, "work_threads")
	if currentWorkers == "" {
		currentWorkers = "1"
	}
	workersInput, err := promptValue(reader, "Issues worked in parallel", currentWorkers)
	if err != nil {
		return err
	}
	if n, err := strconv.Atoi(workersInput); err != nil || n <= 0 {
		fmt.Fprintf(os.Stderr, "Warning: invalid work thread count %q, keeping %s\n", workersInput, currentWorkers)
	} else {
		config.SetConfigValue(configNamespace, fmt.Sprintf("teams.%s.jira.work_threads", team), workersInput)
	}

	// 7. Workflow — which JIRA status names mark the start and end of cycle time
	fmt.Println()
	currentStarted := getConfigString("workflow.cycle_time.started")
	if currentStarted == "" {
		currentStarted = "In Progress"
	}
	startedInput, err := promptValue(reader, "JIRA status name when work starts", currentStarted)
	if err != nil {
		return err
	}
	config.SetConfigValue(configNamespace, "workflow.cycle_time.started", startedInput)

	currentCompleted := getConfigString("workflow.cycle_time.completed")
	if currentCompleted == "" {
		currentCompleted = "Closed"
	}
	completedInput, err := promptValue(reader, "JIRA status name when work is done", currentCompleted)
	if err != nil {
		return err
	}
	config.SetConfigValue(configNamespace, "workflow.cycle_time.completed", completedInput)

	// Save config
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("JIRA configuration saved.")
	return nil
}

// promptEpicPriorities fetches open epics for the team and lets the user select
// and order them. The result is saved as the team's epic selection (priority order).
func promptEpicPriorities(reader *bufio.Reader, team string) error {
	fmt.Print("Configure epic priority order for forecasting? [Y/n]: ")
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) == "n" {
		return nil
	}

	client, err := getJiraClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
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
	fmt.Print("Enter JIRA API token: ")
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

	key := fmt.Sprintf("teams.%s.jira.jql_filter_for_metrics", team)
	jql, _ := splitOrderBy(filter.JQL)
	config.SetConfigValue(configNamespace, key, jql)
	fmt.Printf("Set %s = %s\n", key, jql)
	return nil
}
