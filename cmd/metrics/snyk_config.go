package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/danlafeir/devctl/pkg/config"
	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"devctl-em/internal/snyk"
)

var snykConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Interactive Snyk configuration",
	Long: `Interactively configure Snyk connection and team project tags.

Prompts for:
  - Snyk site (optional, defaults to api.snyk.io)
  - API token (stored in system keychain)
  - Snyk org (selected from your accessible organizations)
  - Team project tag (team-specific, set via select-team first)

Existing values are shown and can be kept by pressing Enter.

Examples:
  devctl-em metrics snyk config`,
	RunE: runSnykConfig,
}

func init() {
	SnykCmd.AddCommand(snykConfigCmd)
}

func runSnykConfig(cmd *cobra.Command, args []string) error {
	initConfig()
	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()

	// 1. Site (global, optional)
	currentSite := getConfigString("snyk.site")
	displaySite := currentSite
	if displaySite == "" {
		displaySite = "api.snyk.io"
	}
	fmt.Printf("Snyk site [%s]: ", displaySite)
	siteInput, _ := reader.ReadString('\n')
	siteInput = strings.TrimSpace(siteInput)
	if siteInput == "" {
		siteInput = displaySite
	}
	if siteInput == "api.snyk.io" {
		config.SetConfigValue(configNamespace, "snyk.site", "")
	} else if siteInput != currentSite {
		config.SetConfigValue(configNamespace, "snyk.site", siteInput)
	}

	// 2. API token (keychain)
	existingToken, _ := secrets.Read("snyk", "api_token")
	if existingToken != "" {
		fmt.Println("API token: configured")
		fmt.Print("Re-enter API token? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) == "y" {
			if err := promptAndStoreSnykToken(); err != nil {
				return err
			}
		}
	} else {
		if err := promptAndStoreSnykToken(); err != nil {
			return err
		}
	}

	// Save site before testing
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 3. Test connection (token + site only — no org ID needed yet)
	token, _ := secrets.Read("snyk", "api_token")
	if token == "" {
		token = os.Getenv("SNYK_TOKEN")
	}
	authClient := snyk.NewAuthClient(token, siteInput)
	fmt.Println("Testing Snyk connection...")
	if err := authClient.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to Snyk: %w", err)
	}
	fmt.Println("Connected successfully.")

	// 4. Org selection
	orgID, err := resolveSnykOrg(ctx, reader, authClient)
	if err != nil {
		return err
	}
	if orgID == "" {
		return fmt.Errorf("Snyk org ID is required")
	}
	config.SetConfigValue(configNamespace, "snyk.org_id", orgID)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 5. Team-specific: Snyk team tag
	team := getSelectedTeam()
	if team == "" {
		if len(getAllTeams()) == 0 {
			fmt.Println("\nNo teams configured. Run: devctl-em metrics config to add a team.")
		} else {
			fmt.Println("\nNo team selected. Run: devctl-em metrics select-team to configure team-specific settings.")
		}
		fmt.Println("Snyk configuration saved.")
		return nil
	}

	currentTag := getConfigString(fmt.Sprintf("teams.%s.snyk.team", team))
	fmt.Printf("\nConfiguring Snyk for team %q\n", team)
	tag, err := promptValue(reader, "Snyk project tag (team:<tag> value)", currentTag)
	if err != nil {
		return err
	}
	config.SetConfigValue(configNamespace, fmt.Sprintf("teams.%s.snyk.team", team), tag)
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("Set Snyk team tag for %q: %s\n", team, tag)

	fmt.Println("Snyk configuration saved.")
	return nil
}

// resolveSnykOrg lists the authenticated user's Snyk orgs and lets them pick one.
// Falls back to manual entry if the API call fails.
func resolveSnykOrg(ctx context.Context, reader *bufio.Reader, client *snyk.Client) (string, error) {
	existing := getConfigString("snyk.org_id")
	if existing != "" {
		fmt.Printf("Snyk org ID: %s\n", existing)
		fmt.Print("Change org? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			return existing, nil
		}
	}

	fmt.Println("Fetching Snyk organizations...")
	orgs, err := client.ListOrgs(ctx)
	if err != nil || len(orgs) == 0 {
		if err != nil {
			fmt.Printf("Could not fetch orgs: %v\n", err)
		} else {
			fmt.Println("No organizations found.")
		}
		fmt.Print("Enter Snyk org ID manually: ")
		input, _ := reader.ReadString('\n')
		return strings.TrimSpace(input), nil
	}

	fmt.Println("Snyk organizations:")
	for i, o := range orgs {
		fmt.Printf("  %d) %s (%s)\n", i+1, o.Name, o.ID)
	}
	fmt.Print("Select org [1]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		input = "1"
	}
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(orgs) {
		return "", fmt.Errorf("invalid selection")
	}
	return orgs[choice-1].ID, nil
}

func promptAndStoreSnykToken() error {
	fmt.Print("Enter Snyk API token: ")
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

	if err := secrets.Write("snyk", "api_token", token); err != nil {
		return fmt.Errorf("failed to store API token: %w", err)
	}
	fmt.Println("API token stored in keychain.")
	return nil
}
