/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package metrics

import (
	"github.com/spf13/cobra"
)

// MetricsCmd represents the metrics command
var MetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Generate engineering metrics and reports",
	Long: `Generate engineering metrics and reports from various data sources.

Available data sources:
  jira    - JIRA Cloud agile metrics
  github  - GitHub DORA metrics

Quick Start:
  1. Add teams:
     devctl-em metrics config

  2. Configure connections:
     devctl-em metrics jira config
     devctl-em metrics github config`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	MetricsCmd.PersistentFlags().BoolVar(&useSavedDataFlag, "use-saved-data", false,
		"Skip upstream API calls and regenerate reports from previously saved CSVs")
}
