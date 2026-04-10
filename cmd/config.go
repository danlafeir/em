/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/danlafeir/cli-go/pkg/config"
	"github.com/danlafeir/cli-go/pkg/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/danlafeir/em/cmd/metrics"
)

// configNamespace is the namespace for em config within ~/.em/config.yaml
const configNamespace = "em"

// emConfigDir returns the path to the em config directory (~/.em).
func emConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".em"
	}
	return filepath.Join(home, ".em")
}

// isSecretKey returns true if the key should be stored in the keychain.
func isSecretKey(key string) bool {
	// secretKeys is the explicit list of keys that should be stored in the keychain.
	secretKeys := map[string]bool{
		"api_token": true,
	}
	// Check the last part of the key (e.g., "api_token" from "jira.api_token")
	parts := strings.Split(key, ".")
	lastPart := parts[len(parts)-1]
	return secretKeys[strings.ToLower(lastPart)]
}

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration for em",
	Long: `Manage em configuration. Regular values are stored in ~/.em/config.yaml; sensitive values (api_token) are stored in the system keychain.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := config.InitConfig(emConfigDir()); err != nil {
			return err
		}
		return metrics.EnsureTeamSelected(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a configuration value",
	Long: `Get a configuration value. Secrets are retrieved from the system keychain.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if isSecretKey(key) {
			// Get from keychain - use first part as command
			parts := strings.Split(key, ".")
			command := parts[0]
			secretKey := strings.Join(parts[1:], ".")
			value, err := secrets.Read(command, secretKey)
			if err != nil {
				log.Fatalf("Failed to read secret: %v", err)
			}
			if value == "" {
				fmt.Printf("Secret '%s' not found\n", key)
			} else {
				fmt.Printf("%s = %s\n", key, value)
			}
			return
		}

		// Get from config file
		if err := config.InitConfig(emConfigDir()); err != nil {
			log.Fatalf("Failed to initialize config: %v", err)
		}

		value, exists := config.GetConfigValue(configNamespace, key)
		if exists {
			fmt.Printf("%s = %v\n", key, value)
		} else {
			fmt.Printf("Configuration key '%s' not found\n", key)
		}
	},
}

// setCmd represents the set command
var setCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value",
	Long: `Set a configuration value. Secrets are stored in the system keychain.

For secrets, if no value is provided, you will be prompted to enter it securely.`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		var value string
		if len(args) == 2 {
			value = args[1]
		} else if isSecretKey(key) {
			// Prompt for secret value
			fmt.Printf("Enter value for %s: ", key)
			if term.IsTerminal(int(syscall.Stdin)) {
				byteValue, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Println()
				if err != nil {
					log.Fatalf("Failed to read input: %v", err)
				}
				value = string(byteValue)
			} else {
				reader := bufio.NewReader(os.Stdin)
				input, err := reader.ReadString('\n')
				if err != nil {
					log.Fatalf("Failed to read input: %v", err)
				}
				value = strings.TrimSpace(input)
			}
		} else {
			log.Fatal("Value required for non-secret keys")
		}

		if isSecretKey(key) {
			// Store in keychain - use first part as command
			parts := strings.Split(key, ".")
			command := parts[0]
			secretKey := strings.Join(parts[1:], ".")
			if err := secrets.Write(command, secretKey, value); err != nil {
				log.Fatalf("Failed to store secret: %v", err)
			}
			fmt.Printf("Set %s (stored in keychain)\n", key)
			return
		}

		// Store in config file
		if err := config.InitConfig(emConfigDir()); err != nil {
			log.Fatalf("Failed to initialize config: %v", err)
		}

		config.SetConfigValue(configNamespace, key, value)
		if err := config.WriteConfig(); err != nil {
			log.Fatalf("Failed to write config: %v", err)
		}

		fmt.Printf("Set %s = %s\n", key, value)
	},
}

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete [key]",
	Short: "Delete a configuration value",
	Long: `Delete a configuration value. Secrets are removed from the system keychain.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if isSecretKey(key) {
			// Delete from keychain - use first part as command
			parts := strings.Split(key, ".")
			command := parts[0]
			secretKey := strings.Join(parts[1:], ".")
			if err := secrets.Delete(command, secretKey); err != nil {
				log.Fatalf("Failed to delete secret: %v", err)
			}
			fmt.Printf("Deleted secret '%s'\n", key)
			return
		}

		// Delete from config file
		if err := config.InitConfig(emConfigDir()); err != nil {
			log.Fatalf("Failed to initialize config: %v", err)
		}

		if err := config.DeleteConfigValue(configNamespace, key); err != nil {
			log.Fatalf("Failed to delete config: %v", err)
		}
		fmt.Printf("Deleted configuration key '%s'\n", key)
	},
}

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configuration values",
	Long: `List all configuration values.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := config.InitConfig(emConfigDir()); err != nil {
			log.Fatalf("Failed to initialize config: %v", err)
		}

		// Get the em namespace config
		configData, err := config.FetchConfig(configNamespace)
		if err != nil {
			log.Fatalf("Failed to fetch config: %v", err)
		}

		// Print config values recursively
		printConfigMap("", configData)

		// Print secrets for common commands
		for _, command := range []string{"jira", "github", "gitlab"} {
			secretKeys, err := secrets.List(command)
			if err == nil && len(secretKeys) > 0 {
				for _, key := range secretKeys {
					fmt.Printf("%s.%s = <secret>\n", command, key)
				}
			}
		}
	},
}

// printConfigMap recursively prints config values
func printConfigMap(prefix string, data map[string]interface{}) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		if nested, ok := value.(map[string]interface{}); ok {
			printConfigMap(fullKey, nested)
		} else {
			fmt.Printf("%s = %v\n", fullKey, value)
		}
	}
}

// yamlCmd writes the current config as YAML to stdout.
var yamlCmd = &cobra.Command{
	Use:   "yaml",
	Short: "Print configuration as YAML",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := config.InitConfig(emConfigDir()); err != nil {
			log.Fatalf("Failed to initialize config: %v", err)
		}
		configData, err := config.FetchConfig(configNamespace)
		if err != nil {
			log.Fatalf("Failed to fetch config: %v", err)
		}
		out, err := yaml.Marshal(configData)
		if err != nil {
			log.Fatalf("Failed to marshal config: %v", err)
		}
		os.Stdout.Write(out)
	},
}

func init() {
	// Add subcommands to config
	configCmd.AddCommand(getCmd)
	configCmd.AddCommand(setCmd)
	configCmd.AddCommand(deleteCmd)
	configCmd.AddCommand(listCmd)
	configCmd.AddCommand(yamlCmd)

	// Add config to root
	rootCmd.AddCommand(configCmd)
}
