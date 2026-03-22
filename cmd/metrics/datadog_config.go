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

	"devctl-em/internal/datadog"
)

var datadogConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Interactive Datadog configuration",
	Long: `Interactively configure Datadog connection.

Prompts for:
  - Datadog site (optional, defaults to datadoghq.com)
  - API key (stored in system keychain)
  - App key (stored in system keychain)
  - Team name (used to filter pages and SLOs)

Existing values are shown and can be kept by pressing Enter.

Examples:
  devctl-em metrics datadog config`,
	RunE: runDatadogConfig,
}

func init() {
	DatadogCmd.AddCommand(datadogConfigCmd)
}

func runDatadogConfig(cmd *cobra.Command, args []string) error {
	initConfig()
	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()

	// 1. Site (optional)
	currentSite := getConfigString("datadog.site")
	displaySite := currentSite
	if displaySite == "" {
		displaySite = "datadoghq.com"
	}
	fmt.Printf("Datadog site [%s]: ", displaySite)
	siteInput, _ := reader.ReadString('\n')
	siteInput = strings.TrimSpace(siteInput)
	if siteInput == "" {
		siteInput = displaySite
	}
	if siteInput == "datadoghq.com" {
		config.SetConfigValue(configNamespace, "datadog.site", "")
	} else if siteInput != currentSite {
		config.SetConfigValue(configNamespace, "datadog.site", siteInput)
	}

	// 2. API key (keychain)
	existingAPIKey, _ := secrets.Read("datadog", "api_key")
	if existingAPIKey != "" {
		fmt.Println("API key: configured")
		fmt.Print("Re-enter API key? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) == "y" {
			if err := promptAndStoreDatadogKey("api_key", "API key"); err != nil {
				return err
			}
		}
	} else {
		if err := promptAndStoreDatadogKey("api_key", "API key"); err != nil {
			return err
		}
	}

	// 3. App key (keychain)
	existingAppKey, _ := secrets.Read("datadog", "app_key")
	if existingAppKey != "" {
		fmt.Println("App key: configured")
		fmt.Print("Re-enter App key? [y/N]: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) == "y" {
			if err := promptAndStoreDatadogKey("app_key", "App key"); err != nil {
				return err
			}
		}
	} else {
		if err := promptAndStoreDatadogKey("app_key", "App key"); err != nil {
			return err
		}
	}

	// Save site before testing
	if err := config.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 4. Test connection
	apiKey, _ := secrets.Read("datadog", "api_key")
	if apiKey == "" {
		apiKey = os.Getenv("DD_API_KEY")
	}
	appKey, _ := secrets.Read("datadog", "app_key")
	if appKey == "" {
		appKey = os.Getenv("DD_APP_KEY")
	}
	creds := datadog.Credentials{
		APIKey: apiKey,
		AppKey: appKey,
		Site:   siteInput,
	}
	client := datadog.NewClient(creds)

	fmt.Println("Testing Datadog connection...")
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to Datadog: %w", err)
	}
	fmt.Println("Connected successfully.")

	// 5. Team
	teamInput, err := selectDatadogTeam(reader)
	if err != nil {
		return err
	}
	currentTeam := getConfigString("datadog.team")
	if teamInput != currentTeam {
		config.SetConfigValue(configNamespace, "datadog.team", teamInput)
		if err := config.WriteConfig(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}

	fmt.Println("Datadog configuration saved.")
	return nil
}

// selectDatadogTeam lists configured teams and lets the user pick one or enter a new one.
func selectDatadogTeam(reader *bufio.Reader) (string, error) {
	currentTeam := getConfigString("datadog.team")
	teams := getAllTeams()

	if len(teams) > 0 {
		fmt.Println("Teams:")
		for i, t := range teams {
			marker := ""
			if t == currentTeam {
				marker = " (current)"
			}
			fmt.Printf("  %d) %s%s\n", i+1, t, marker)
		}
		fmt.Printf("  n) Enter a new team\n")

		if currentTeam != "" {
			fmt.Printf("Select team [current: %s]: ", currentTeam)
		} else {
			fmt.Print("Select team: ")
		}

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" && currentTeam != "" {
			return currentTeam, nil
		}

		if input != "n" && input != "" {
			choice, err := strconv.Atoi(input)
			if err != nil || choice < 1 || choice > len(teams) {
				return "", fmt.Errorf("invalid selection")
			}
			return teams[choice-1], nil
		}
	}

	// New team prompt
	fmt.Print("Team name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		if currentTeam != "" {
			return currentTeam, nil
		}
		return "", fmt.Errorf("team name is required")
	}
	return name, nil
}

func promptAndStoreDatadogKey(secretName, displayName string) error {
	fmt.Printf("Enter Datadog %s: ", displayName)
	var value string
	if term.IsTerminal(int(syscall.Stdin)) {
		byteValue, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", displayName, err)
		}
		value = string(byteValue)
	} else {
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", displayName, err)
		}
		value = strings.TrimSpace(input)
	}

	if value == "" {
		return fmt.Errorf("%s is required", displayName)
	}

	if err := secrets.Write("datadog", secretName, value); err != nil {
		return fmt.Errorf("failed to store %s: %w", displayName, err)
	}
	fmt.Printf("%s stored in keychain.\n", displayName)
	return nil
}
