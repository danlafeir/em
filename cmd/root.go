/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"devctl-em/cmd/metrics"
	"github.com/danlafeir/devctl/pkg/update"
	"github.com/spf13/cobra"
)

// These are provided by main.go
var BuildGitHash string
var BuildLatestHash string

// updateConfig returns the update configuration for devctl-em
var updateConfig = update.Config{
	AppName: "devctl-em",
	Repo:    "danlafeir/devctl-em",
}



// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update devctl-em to the latest version",
	Run: func(cmd *cobra.Command, args []string) {
		update.RunUpdateWithConfig(updateConfig, BuildGitHash, cmd)
	},
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "devctl-em",
	Short: "Engineering manager CLI tools for metrics and reporting",
	Long: `devctl-em provides CLI tools for engineering managers to generate
metrics reports and insights from JIRA and other sources.

Features:
  - JIRA agile metrics (cycle time, throughput, CFD, WIP)
  - Monte Carlo forecasting for epic completion
  - Automated report generation (HTML, CSV, Excel)

Quick Start:
  1. Configure JIRA connection:
     devctl-em config set jira.domain mycompany
     devctl-em config set jira.email user@company.com
     devctl-em config set jira.api_token

  2. Generate a report:
     devctl-em metrics jira report --jql "project = MYPROJ"`,
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
}


