/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"em/cmd/metrics"
	"github.com/danlafeir/cli-go/pkg/update"
	"github.com/spf13/cobra"
)

// These are provided by main.go
var BuildGitHash string
var BuildLatestHash string

// updateConfig returns the update configuration for em
var updateConfig = update.Config{
	AppName: "em",
	Repo:    "danlafeir/em",
	BinDir:  "bin",
}

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update em to the latest version",
	Run: func(cmd *cobra.Command, args []string) {
		update.RunUpdateWithConfig(updateConfig, BuildGitHash, cmd)
	},
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "em",
	Short: "Engineering manager CLI tools for metrics and reporting",
	Long: `em provides CLI tools for engineering managers to generate
metrics reports and insights from JIRA and other sources.

Quick Start:
  1. Configure connections:
     em metrics jira config
     em metrics github config

  2. Generate a report:
     em metrics jira report`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(metrics.MetricsCmd)
	rootCmd.AddCommand(updateCmd)

	// Disable default commands
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}


