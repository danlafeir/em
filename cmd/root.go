/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"os"

	"devctl-em/cmd/metrics"
	"github.com/spf13/cobra"
)



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
     export JIRA_API_TOKEN=your_token

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
	// Add metrics command
	rootCmd.AddCommand(metrics.MetricsCmd)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.devctl-em.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}


