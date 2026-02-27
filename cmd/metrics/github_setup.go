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
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure deploy workflows for each team repository",
	Long: `Interactively select which GitHub Actions workflow represents a deployment
for each repository owned by your team.

This stores selections in github.workflows config, used by deployment-frequency.

Prerequisites:
  devctl-em config set github.org myorg
  devctl-em config set github.team my-team
  devctl-em config set github.api_token`,
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
	team := getGithubTeam()

	if org == "" {
		return fmt.Errorf("GitHub org not configured. Run: devctl-em config set github.org <org>")
	}
	if team == "" {
		return fmt.Errorf("GitHub team not configured. Run: devctl-em config set github.team <team>")
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
		for i, wf := range workflows {
			filename := filepath.Base(wf.Path)
			fmt.Printf("    %d) %s (%s)\n", i+1, wf.Name, filename)
		}
		fmt.Printf("    0) Skip this repo\n")
		fmt.Printf("  Select deploy workflow [0]: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" || input == "0" {
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
	config.SetConfigValue(configNamespace, "github.workflows", configMap)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Saved %d workflow selections to config.\n", len(selections))
	return nil
}
