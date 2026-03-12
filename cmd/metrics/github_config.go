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
			return fmt.Errorf("no teams configured. Run: devctl-em metrics config to add a team first")
		}
		return fmt.Errorf("no team selected. Run: devctl-em metrics select-team")
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

	// Load current config once for the team
	currentWorkflows, _ := getConfiguredWorkflowsByTeam(teamName)

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
				continue
			}
			fmt.Println()
			continue
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

		// Save this repo's selection immediately
		repoKey := fmt.Sprintf("teams.%s.github.workflows.%s", teamName, repo.Name)
		var repoValue any = chosen
		if len(chosen) == 1 {
			repoValue = chosen[0]
		}
		config.SetConfigValue(configNamespace, repoKey, repoValue)
		if err := config.WriteConfig(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		saved++
		fmt.Printf("  Selected: %s\n\n", strings.Join(chosen, ", "))
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

