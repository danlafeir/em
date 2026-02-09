/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/danlafeir/devctl/pkg/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// secretsCmd represents the secrets command
var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Manage secrets stored in system keychain",
	Long: `Manage secrets for devctl-em stored securely in your system keychain.

Use this command to set, get, list, or delete secrets.
Secrets are stored in the macOS Keychain (or system equivalent).

Examples:
  devctl-em secrets set jira.api_token
  devctl-em secrets get jira.api_token
  devctl-em secrets list jira
  devctl-em secrets delete jira.api_token`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Use 'devctl-em secrets --help' for available subcommands")
	},
}

// secretsGetCmd represents the get command
var secretsGetCmd = &cobra.Command{
	Use:   "get [command.key]",
	Short: "Get a secret value",
	Long: `Get a secret value from the system keychain.

Examples:
  devctl-em secrets get jira.api_token`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		parts := strings.Split(key, ".")
		if len(parts) != 2 {
			log.Fatal("Key must be in format 'command.key'")
		}

		command, secretKey := parts[0], parts[1]
		value, err := secrets.DefaultSecretsProvider.Read(command, secretKey)
		if err != nil {
			log.Fatalf("Failed to read secret: %v", err)
		}

		if value == "" {
			fmt.Printf("Secret '%s' not found\n", key)
		} else {
			fmt.Printf("%s = %s\n", key, value)
		}
	},
}

// secretsSetCmd represents the set command
var secretsSetCmd = &cobra.Command{
	Use:   "set [command.key] [value]",
	Short: "Set a secret value",
	Long: `Set a secret value in the system keychain.

If no value is provided, you will be prompted to enter it securely.

Examples:
  devctl-em secrets set jira.api_token
  devctl-em secrets set jira.api_token my_token_value`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		parts := strings.Split(key, ".")
		if len(parts) != 2 {
			log.Fatal("Key must be in format 'command.key'")
		}

		command, secretKey := parts[0], parts[1]

		var value string
		if len(args) == 2 {
			value = args[1]
		} else {
			// Prompt for value securely
			fmt.Printf("Enter value for %s: ", key)
			if term.IsTerminal(int(syscall.Stdin)) {
				byteValue, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Println() // Print newline after password input
				if err != nil {
					log.Fatalf("Failed to read input: %v", err)
				}
				value = string(byteValue)
			} else {
				// Non-terminal input (pipe)
				reader := bufio.NewReader(os.Stdin)
				input, err := reader.ReadString('\n')
				if err != nil {
					log.Fatalf("Failed to read input: %v", err)
				}
				value = strings.TrimSpace(input)
			}
		}

		if err := secrets.DefaultSecretsProvider.Write(command, secretKey, value); err != nil {
			log.Fatalf("Failed to store secret: %v", err)
		}

		fmt.Printf("Secret '%s' stored in keychain\n", key)
	},
}

// secretsListCmd represents the list command
var secretsListCmd = &cobra.Command{
	Use:   "list [command]",
	Short: "List secrets for a command",
	Long: `List all secret keys stored for a command.

Examples:
  devctl-em secrets list jira`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		command := args[0]
		keys, err := secrets.DefaultSecretsProvider.List(command)
		if err != nil {
			log.Fatalf("Failed to list secrets: %v", err)
		}

		if len(keys) == 0 {
			fmt.Printf("No secrets found for '%s'\n", command)
			return
		}

		fmt.Printf("Secrets for '%s':\n", command)
		for _, key := range keys {
			fmt.Printf("  %s.%s\n", command, key)
		}
	},
}

// secretsDeleteCmd represents the delete command
var secretsDeleteCmd = &cobra.Command{
	Use:   "delete [command.key]",
	Short: "Delete a secret",
	Long: `Delete a secret from the system keychain.

Examples:
  devctl-em secrets delete jira.api_token`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		parts := strings.Split(key, ".")
		if len(parts) != 2 {
			log.Fatal("Key must be in format 'command.key'")
		}

		command, secretKey := parts[0], parts[1]
		if err := secrets.DefaultSecretsProvider.Delete(command, secretKey); err != nil {
			log.Fatalf("Failed to delete secret: %v", err)
		}

		fmt.Printf("Secret '%s' deleted\n", key)
	},
}

func init() {
	// Add subcommands to secrets
	secretsCmd.AddCommand(secretsGetCmd)
	secretsCmd.AddCommand(secretsSetCmd)
	secretsCmd.AddCommand(secretsListCmd)
	secretsCmd.AddCommand(secretsDeleteCmd)

	// Add secrets to root
	rootCmd.AddCommand(secretsCmd)
}
