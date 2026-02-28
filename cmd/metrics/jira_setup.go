package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

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

	fmt.Print("Enter team slug: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return "", fmt.Errorf("team slug is required")
	}

	return input, nil
}
