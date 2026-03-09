package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"devctl-em/internal/github"
)

var ghConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Interactive GitHub configuration",
	Long: `Interactively configure GitHub connection and team deploy workflows.

Prompts for:
  - GitHub organization
  - API token (stored in system keychain)
  - Deploy workflow per repository for each team

Existing values are shown and can be kept by pressing Enter.

Examples:
  devctl-em metrics github config
  devctl-em metrics github config --team my-team`,
	RunE: runGhConfig,
}

func init() {
	GithubCmd.AddCommand(ghConfigCmd)
}

func runGhConfig(cmd *cobra.Command, args []string) error {
	initConfig()
	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()

	// 1. Org
	currentOrg := getConfigString("github.org")
	org, err := promptValue(reader, "GitHub organization", currentOrg)
	if err != nil {
		return err
	}
	if org != currentOrg {
		config.SetConfigValue(configNamespace, "github.org", org)
	}

	// 2. API token (keychain)
	existingToken, _ := secrets.Read("github", "api_token")
	if existingToken != "" {
		fmt.Println("API token: configured")
		fmt.Print("Re-enter API token? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) == "y" {
			if err := promptAndStoreGhToken(); err != nil {
				return err
			}
		}
	} else {
		fmt.Print("Enter GitHub API token: ")
		if err := promptAndStoreGhToken(); err != nil {
			return err
		}
	}

	// Save org before testing connection
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 3. Test connection
	client, err := getGithubClient()
	if err != nil {
		return err
	}
	fmt.Println("Testing GitHub connection...")
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to GitHub: %w", err)
	}
	fmt.Println("Connected successfully.")

	// 4. Team workflow loop
	for {
		if err := runGhTeamConfig(ctx, reader, client, org); err != nil {
			return err
		}

		fmt.Print("\nConfigure another team? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			break
		}
	}

	fmt.Println("GitHub configuration saved.")
	return nil
}

// runGhTeamConfig handles workflow selection for a single team.
func runGhTeamConfig(ctx context.Context, reader *bufio.Reader, client *github.Client, org string) error {
	team, err := resolveGhConfigTeam(ctx, client, org)
	if err != nil {
		return err
	}

	fmt.Printf("\nFetching repositories for %s/%s...\n", org, team)
	repos, err := client.ListTeamRepos(ctx, org, team)
	if err != nil {
		return fmt.Errorf("failed to list team repos: %w", err)
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found for this team.")
		return nil
	}

	fmt.Printf("Found %d repositories\n\n", len(repos))

	selections := make(map[string][]string)
	notAccessible := 0

	// Load current config once for the team
	currentWorkflows, _ := getConfiguredWorkflowsByTeam(team)

	for _, repo := range repos {
		if repo.Archived {
			continue
		}

		fmt.Printf("--- %s ---\n", repo.Name)

		owner := repo.Owner.Login
		if owner == "" {
			owner = org
		}
		workflows, err := client.ListWorkflows(ctx, owner, repo.Name)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				fmt.Printf("  Actions not accessible, skipping.\n\n")
				notAccessible++
			} else {
				fmt.Printf("  Error listing workflows: %v\n\n", err)
			}
			continue
		}

		if len(workflows) == 0 {
			fmt.Printf("  No workflows found, skipping.\n\n")
			continue
		}

		current := currentWorkflows[repo.Name] // []string, nil if not set

		// Build a set of current filenames for quick lookup
		currentSet := make(map[string]bool, len(current))
		for _, f := range current {
			currentSet[f] = true
		}

		fmt.Printf("  Workflows (use comma-separated numbers to select multiple):\n")
		suggested := suggestDeployWorkflow(workflows)
		for i, wf := range workflows {
			filename := filepath.Base(wf.Path)
			marker := ""
			if currentSet[filename] {
				marker = " (current)"
			}
			fmt.Printf("    %d) %s (%s)%s\n", i+1, wf.Name, filename, marker)
		}
		fmt.Printf("    0) Skip this repo\n")

		if len(current) > 0 {
			fmt.Printf("  Select deploy workflow(s) [current: %s]: ", strings.Join(current, ", "))
		} else if suggested > 0 {
			fmt.Printf("  Select deploy workflow(s) [%d - %s]: ", suggested, workflows[suggested-1].Name)
		} else {
			fmt.Printf("  Select deploy workflow(s) [0]: ")
		}

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			if len(current) > 0 {
				selections[repo.Name] = current
				fmt.Printf("  Kept: %s\n\n", strings.Join(current, ", "))
				continue
			} else if suggested > 0 {
				input = strconv.Itoa(suggested)
			} else {
				fmt.Println()
				continue
			}
		}
		if input == "0" {
			fmt.Println()
			continue
		}

		// Parse comma-separated indices
		var chosen []string
		for _, part := range strings.Split(input, ",") {
			part = strings.TrimSpace(part)
			choice, err := strconv.Atoi(part)
			if err != nil || choice < 1 || choice > len(workflows) {
				fmt.Printf("  Invalid choice %q, skipping.\n\n", part)
				chosen = nil
				break
			}
			chosen = append(chosen, filepath.Base(workflows[choice-1].Path))
		}

		if len(chosen) == 0 {
			continue
		}

		selections[repo.Name] = chosen
		fmt.Printf("  Selected: %s\n\n", strings.Join(chosen, ", "))
	}

	if notAccessible > 0 && notAccessible == len(repos) {
		fmt.Println("All repositories returned 404. Your token may need the 'actions:read' permission.")
		return nil
	}

	if len(selections) == 0 {
		fmt.Println("No workflows selected.")
		return nil
	}

	configMap := make(map[string]any, len(selections))
	for k, v := range selections {
		if len(v) == 1 {
			configMap[k] = v[0] // store single value as string for readability
		} else {
			configMap[k] = v
		}
	}

	configKey := fmt.Sprintf("github.teams.%s.workflows", team)
	config.SetConfigValue(configNamespace, configKey, configMap)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Saved %d workflow selections for team %q.\n", len(selections), team)
	return nil
}

// promptAndStoreGhToken reads a GitHub token from the terminal and stores it in the keychain.
func promptAndStoreGhToken() error {
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

	if err := secrets.Write("github", "api_token", token); err != nil {
		return fmt.Errorf("failed to store API token: %w", err)
	}
	fmt.Println("API token stored in keychain.")
	return nil
}

// resolveGhConfigTeam determines which team to configure.
func resolveGhConfigTeam(ctx context.Context, client *github.Client, org string) (string, error) {
	if ghTeamFlag != "" {
		return ghTeamFlag, nil
	}

	reader := bufio.NewReader(os.Stdin)
	existingTeams := getGithubTeams()

	if len(existingTeams) > 0 {
		fmt.Println("Configured teams:")
		for i, t := range existingTeams {
			fmt.Printf("  %d) %s\n", i+1, t)
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

	return selectTeamFromAPI(ctx, client, org, existingTeams)
}

// selectTeamFromAPI fetches org teams via the GitHub API and presents them for selection.
func selectTeamFromAPI(ctx context.Context, client *github.Client, org string, exclude []string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Fetching teams for %s...\n", org)
	apiTeams, err := client.ListUserTeams(ctx, org)
	if err != nil {
		fmt.Printf("Could not fetch teams: %v\n", err)
		return promptTeamSlug(reader)
	}

	excludeSet := make(map[string]bool, len(exclude))
	for _, t := range exclude {
		excludeSet[t] = true
	}
	var available []github.Team
	for _, t := range apiTeams {
		if !excludeSet[t.Slug] {
			available = append(available, t)
		}
	}

	if len(available) == 0 {
		fmt.Println("No additional teams found in this org.")
		return promptTeamSlug(reader)
	}

	fmt.Println("Available teams:")
	for i, t := range available {
		fmt.Printf("  %d) %s (%s)\n", i+1, t.Name, t.Slug)
	}
	fmt.Printf("Select team [1]: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		input = "1"
	}

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(available) {
		return "", fmt.Errorf("invalid selection")
	}

	return available[choice-1].Slug, nil
}

// promptTeamSlug asks the user to manually enter a team slug.
func promptTeamSlug(reader *bufio.Reader) (string, error) {
	fmt.Print("Enter team slug: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return "", fmt.Errorf("team slug is required. Use --team flag or enter a slug when prompted")
	}

	return input, nil
}

// suggestDeployWorkflow returns the 1-based index of a workflow whose name or
// filename contains "prod", "deploy", or "release". Returns 0 if no match.
func suggestDeployWorkflow(workflows []github.Workflow) int {
	for i, wf := range workflows {
		name := strings.ToLower(wf.Name)
		filename := strings.ToLower(filepath.Base(wf.Path))
		for _, keyword := range []string{"prod", "deploy", "release"} {
			if strings.Contains(name, keyword) || strings.Contains(filename, keyword) {
				return i + 1
			}
		}
	}
	return 0
}
