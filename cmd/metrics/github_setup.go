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

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure deploy workflows for a team's repositories",
	Long: `Interactively select which GitHub Actions workflow represents a deployment
for each repository owned by a team.

Selections are stored under github.teams.<team>.workflows in config,
used by deployment-frequency.

Prerequisites:
  devctl-em config set github.org myorg
  devctl-em config set github.api_token

Examples:
  devctl-em metrics github setup --team my-team
  devctl-em metrics github setup`,
	RunE: runSetup,
}

func init() {
	GithubCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
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

	team, err := resolveSetupTeam()
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

// resolveSetupTeam determines which team to set up.
// If --team is set, use that. Otherwise prompt the user to pick from
// configured teams or enter a new team slug.
func resolveSetupTeam() (string, error) {
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
