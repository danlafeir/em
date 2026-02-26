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

Use "devctl-em metrics [source] --help" for more information about a source.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	// metricsCmd will be added to rootCmd from the parent package

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// metricsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// metricsCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
