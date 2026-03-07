package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/spf13/cobra"

	"devctl-em/internal/github"
)

var ghConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure deploy workflows for a team's repositories",
	Long: `Interactively select which GitHub Actions workflow represents a deployment
for each repository owned by a team.

Selections are stored under github.teams.<team>.workflows in config,
used by deployment-frequency.

Prerequisites:
  devctl-em config set github.org myorg
  devctl-em config set github.api_token

Examples:
  devctl-em metrics github config --team my-team
  devctl-em metrics github config`,
	RunE: runGhConfig,
}

func init() {
	GithubCmd.AddCommand(ghConfigCmd)
}

func runGhConfig(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getGithubClient()
	if err != nil {
		return err
	}

	fmt.Println("Testing GitHub connection...")
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to GitHub: %w", err)
	}

	org := getGithubOrg()
	if org == "" {
		return fmt.Errorf("GitHub org not configured. Run: devctl-em config set github.org <org>")
	}

	team, err := resolveGhConfigTeam(ctx, client, org)
	if err != nil {
		return err
	}

	fmt.Printf("Fetching repositories for %s/%s...\n", org, team)
	repos, err := client.ListTeamRepos(ctx, org, team)
	if err != nil {
		return fmt.Errorf("failed to list team repos: %w", err)
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found for this team.")
		return nil
	}

	fmt.Printf("Found %d repositories\n\n", len(repos))

	reader := bufio.NewReader(os.Stdin)
	selections := make(map[string]string)

	notAccessible := 0
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

		fmt.Printf("  Workflows:\n")
		suggested := suggestDeployWorkflow(workflows)
		for i, wf := range workflows {
			filename := filepath.Base(wf.Path)
			fmt.Printf("    %d) %s (%s)\n", i+1, wf.Name, filename)
		}
		fmt.Printf("    0) Skip this repo\n")

		if suggested > 0 {
			suggestedWf := workflows[suggested-1]
			fmt.Printf("  Select deploy workflow [%d - %s]: ", suggested, suggestedWf.Name)
		} else {
			fmt.Printf("  Select deploy workflow [0]: ")
		}

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			if suggested > 0 {
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

		choice, err := strconv.Atoi(input)
		if err != nil || choice < 1 || choice > len(workflows) {
			fmt.Printf("  Invalid choice, skipping.\n\n")
			continue
		}

		wf := workflows[choice-1]
		filename := filepath.Base(wf.Path)
		selections[repo.Name] = filename
		fmt.Printf("  Selected: %s\n\n", filename)
	}

	if notAccessible > 0 && notAccessible == len(repos) {
		fmt.Println("All repositories returned 404. Your token may need the 'actions:read' permission.")
		return nil
	}

	if len(selections) == 0 {
		fmt.Println("No workflows selected.")
		return nil
	}

	// Convert to interface map for config storage
	configMap := make(map[string]any, len(selections))
	for k, v := range selections {
		configMap[k] = v
	}

	initConfig()
	configKey := fmt.Sprintf("github.teams.%s.workflows", team)
	config.SetConfigValue(configNamespace, configKey, configMap)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Saved %d workflow selections for team %q.\n", len(selections), team)
	return nil
}

// resolveGhConfigTeam determines which team to configure.
// If --team is set, use that. Otherwise prompt the user to pick from
// configured teams or select from the GitHub org's teams via API.
func resolveGhConfigTeam(ctx context.Context, client *github.Client, org string) (string, error) {
	if ghTeamFlag != "" {
		return ghTeamFlag, nil
	}

	reader := bufio.NewReader(os.Stdin)

	// Check for existing teams
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

	// Fetch teams from GitHub API and let the user pick
	return selectTeamFromAPI(ctx, client, org, existingTeams)
}

// selectTeamFromAPI fetches org teams via the GitHub API and presents them
// for selection. Falls back to manual slug entry on error or empty results.
func selectTeamFromAPI(ctx context.Context, client *github.Client, org string, exclude []string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Fetching teams for %s...\n", org)
	apiTeams, err := client.ListUserTeams(ctx, org)
	if err != nil {
		fmt.Printf("Could not fetch teams: %v\n", err)
		return promptTeamSlug(reader)
	}

	// Exclude already-configured teams
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
