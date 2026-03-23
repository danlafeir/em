package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/danlafeir/cli-go/pkg/config"
	"github.com/danlafeir/cli-go/pkg/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"em/internal/github"
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
  em metrics github config
  em metrics github config --team my-team`,
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

	// 4. Configure selected team
	team := getSelectedTeam()
	if team == "" {
		if len(getAllTeams()) == 0 {
			return fmt.Errorf("no teams configured. Run: em metrics config to add a team first")
		}
		return fmt.Errorf("no team selected. Run: em metrics select-team")
	}

	if err := runGhTeamConfig(ctx, reader, client, org, team); err != nil {
		return err
	}

	fmt.Println("GitHub configuration saved.")
	return nil
}

// resolveGithubSlug resolves the GitHub team slug for a config team.
// Checks for an existing stored slug, then falls back to API lookup.
func resolveGithubSlug(ctx context.Context, reader *bufio.Reader, client *github.Client, org, teamName string) (string, error) {
	existing := getConfigString(fmt.Sprintf("teams.%s.github.slug", teamName))
	if existing != "" {
		fmt.Printf("GitHub team slug: %s\n", existing)
		fmt.Print("Change GitHub team? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			return existing, nil
		}
	}

	fmt.Printf("Fetching GitHub teams for %s...\n", org)
	apiTeams, err := client.ListUserTeams(ctx, org)
	if err != nil || len(apiTeams) == 0 {
		if err != nil {
			fmt.Printf("Could not fetch teams: %v\n", err)
		} else {
			fmt.Println("No GitHub teams found.")
		}
		fmt.Print("Enter GitHub team slug manually: ")
		input, _ := reader.ReadString('\n')
		return strings.TrimSpace(input), nil
	}

	fmt.Println("GitHub teams:")
	for i, t := range apiTeams {
		fmt.Printf("  %d) %s (%s)\n", i+1, t.Name, t.Slug)
	}
	fmt.Print("Select team [1]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		input = "1"
	}
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(apiTeams) {
		return "", fmt.Errorf("invalid selection")
	}
	return apiTeams[choice-1].Slug, nil
}

// runGhTeamConfig handles workflow selection for a single team.
// If repos are already configured it presents a change/add/done menu instead of
// iterating all team repos from scratch.
func runGhTeamConfig(ctx context.Context, reader *bufio.Reader, client *github.Client, org, teamName string) error {
	slug, err := resolveGithubSlug(ctx, reader, client, org, teamName)
	if err != nil {
		return err
	}
	if slug == "" {
		return fmt.Errorf("GitHub team slug is required")
	}

	slugKey := fmt.Sprintf("teams.%s.github.slug", teamName)
	config.SetConfigValue(configNamespace, slugKey, slug)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	currentWorkflows, _ := getConfiguredWorkflowsByTeam(teamName)

	if len(currentWorkflows) > 0 {
		return runGhTeamConfigUpdate(ctx, reader, client, org, slug, teamName, currentWorkflows)
	}
	return runGhTeamConfigFirstTime(ctx, reader, client, org, slug, teamName)
}

// runGhTeamConfigUpdate is shown when repos are already configured. It lets the
// user change an existing repo's workflow, add a new repo, or exit unchanged.
func runGhTeamConfigUpdate(ctx context.Context, reader *bufio.Reader, client *github.Client, org, slug, teamName string, currentWorkflows map[string][]string) error {
	for {
		// Show current configuration
		fmt.Println("\nConfigured repositories:")
		configuredRepos := sortedKeys(currentWorkflows)
		for i, repo := range configuredRepos {
			fmt.Printf("  %d) %-30s  %s\n", i+1, repo, strings.Join(currentWorkflows[repo], ", "))
		}
		fmt.Println()

		fmt.Println("  c) Change a configured repo")
		fmt.Println("  a) Add a repo")
		fmt.Println("  d) Done — keep as-is")
		fmt.Print("Choice [d]: ")

		input, _ := reader.ReadString('\n')
		input = strings.ToLower(strings.TrimSpace(input))
		if input == "" || input == "d" {
			return nil
		}

		switch input {
		case "c":
			if err := promptChangeRepo(ctx, reader, client, org, teamName, configuredRepos, currentWorkflows); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		case "a":
			if err := promptAddRepo(ctx, reader, client, org, slug, teamName, currentWorkflows); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		default:
			fmt.Println("Invalid choice.")
		}
	}
}

// promptChangeRepo lets the user pick one of the already-configured repos and
// re-select its deploy workflow(s).
func promptChangeRepo(ctx context.Context, reader *bufio.Reader, client *github.Client, org, teamName string, configuredRepos []string, currentWorkflows map[string][]string) error {
	fmt.Print("Select repo to change: ")
	input, _ := reader.ReadString('\n')
	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(configuredRepos) {
		return fmt.Errorf("invalid selection")
	}
	repoName := configuredRepos[choice-1]
	saved, err := configureRepoWorkflow(ctx, reader, client, org, teamName, repoName, currentWorkflows)
	if err != nil {
		return err
	}
	if saved {
		refreshed, _ := getConfiguredWorkflowsByTeam(teamName)
		currentWorkflows[repoName] = refreshed[repoName]
	}
	return nil
}

// promptAddRepo fetches team repos from GitHub, filters out already-configured
// ones, and lets the user pick one to configure.
func promptAddRepo(ctx context.Context, reader *bufio.Reader, client *github.Client, org, slug, teamName string, currentWorkflows map[string][]string) error {
	fmt.Printf("Fetching repositories for %s/%s...\n", org, slug)
	repos, err := client.ListTeamRepos(ctx, org, slug)
	if err != nil {
		return fmt.Errorf("failed to list team repos: %w", err)
	}

	var unconfigured []string
	for _, r := range repos {
		if r.Archived {
			continue
		}
		if _, already := currentWorkflows[r.Name]; !already {
			unconfigured = append(unconfigured, r.Name)
		}
	}

	if len(unconfigured) == 0 {
		fmt.Println("All team repositories are already configured.")
		return nil
	}

	fmt.Println("Unconfigured repositories:")
	for i, name := range unconfigured {
		fmt.Printf("  %d) %s\n", i+1, name)
	}
	fmt.Print("Select repo to add: ")
	input, _ := reader.ReadString('\n')
	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(unconfigured) {
		return fmt.Errorf("invalid selection")
	}
	repoName := unconfigured[choice-1]
	saved, err := configureRepoWorkflow(ctx, reader, client, org, teamName, repoName, currentWorkflows)
	if err != nil {
		return err
	}
	if saved {
		refreshed, _ := getConfiguredWorkflowsByTeam(teamName)
		currentWorkflows[repoName] = refreshed[repoName]
	}
	return nil
}

// runGhTeamConfigFirstTime is the original first-run flow: iterate all team repos
// in sequence and prompt for a deploy workflow for each.
func runGhTeamConfigFirstTime(ctx context.Context, reader *bufio.Reader, client *github.Client, org, slug, teamName string) error {
	fmt.Printf("\nFetching repositories for %s/%s...\n", org, slug)
	repos, err := client.ListTeamRepos(ctx, org, slug)
	if err != nil {
		return fmt.Errorf("failed to list team repos: %w", err)
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found for this team.")
		return nil
	}

	fmt.Printf("Found %d repositories\n\n", len(repos))

	notAccessible := 0
	saved := 0
	currentWorkflows := map[string][]string{}

	for _, repo := range repos {
		if repo.Archived {
			continue
		}
		ok, err := configureRepoWorkflow(ctx, reader, client, org, teamName, repo.Name, currentWorkflows)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				notAccessible++
			}
			continue
		}
		if ok {
			saved++
		}
	}

	if notAccessible > 0 && notAccessible == len(repos) {
		fmt.Println("All repositories returned 404. Your token may need the 'actions:read' permission.")
		return nil
	}

	if saved == 0 {
		fmt.Println("No workflows selected.")
	} else {
		fmt.Printf("Saved %d workflow selections for team %q.\n", saved, teamName)
	}
	return nil
}

// configureRepoWorkflow shows available workflows for a single repo and saves the
// user's selection. Returns (true, nil) when a selection was saved.
func configureRepoWorkflow(ctx context.Context, reader *bufio.Reader, client *github.Client, org, teamName, repoName string, currentWorkflows map[string][]string) (bool, error) {
	owner := org
	workflows, err := client.ListWorkflows(ctx, owner, repoName)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			fmt.Printf("  %s: Actions not accessible, skipping.\n\n", repoName)
			return false, fmt.Errorf("404: %w", err)
		}
		fmt.Printf("  %s: Error listing workflows: %v\n\n", repoName, err)
		return false, err
	}

	if len(workflows) == 0 {
		fmt.Printf("  %s: No workflows found, skipping.\n\n", repoName)
		return false, nil
	}

	current := currentWorkflows[repoName]
	currentSet := make(map[string]bool, len(current))
	for _, f := range current {
		currentSet[f] = true
	}

	fmt.Printf("--- %s ---\n", repoName)
	fmt.Printf("  Workflows (use comma-separated numbers to select multiple):\n")
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
	} else {
		fmt.Printf("  Select deploy workflow(s) [0]: ")
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		if len(current) > 0 {
			fmt.Printf("  Kept: %s\n\n", strings.Join(current, ", "))
		} else {
			fmt.Println()
		}
		return false, nil
	}
	if input == "0" {
		fmt.Println()
		return false, nil
	}

	var chosen []string
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		choice, err := strconv.Atoi(part)
		if err != nil || choice < 1 || choice > len(workflows) {
			fmt.Printf("  Invalid choice %q, skipping.\n\n", part)
			return false, nil
		}
		chosen = append(chosen, filepath.Base(workflows[choice-1].Path))
	}

	if len(chosen) == 0 {
		return false, nil
	}

	repoKey := fmt.Sprintf("teams.%s.github.workflows.%s", teamName, repoName)
	var repoValue any
	if len(chosen) == 1 {
		repoValue = chosen[0]
	} else {
		items := make([]any, len(chosen))
		for i, s := range chosen {
			items[i] = s
		}
		repoValue = items
	}
	config.SetConfigValue(configNamespace, repoKey, repoValue)
	if err := config.WriteConfig(); err != nil {
		return false, fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("  Selected: %s\n\n", strings.Join(chosen, ", "))
	return true, nil
}

// sortedKeys returns the keys of a map[string][]string in sorted order.
func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// promptAndStoreGhToken reads a GitHub token from the terminal and stores it in the keychain.
func promptAndStoreGhToken() error {
	fmt.Print("Enter GitHub API token: ")
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

