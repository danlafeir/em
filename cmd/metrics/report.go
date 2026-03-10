package metrics

import (
	"fmt"

	"github.com/spf13/cobra"
)

var metricsReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a combined engineering metrics report",
	Long: `Generate a combined report across all configured data sources.

Runs GitHub deployment frequency and JIRA metrics reports in sequence.
Each section runs independently — a failure in one does not stop the other.

Example:
  devctl-em metrics report
  devctl-em metrics report --team platform`,
	RunE: runMetricsReport,
}

func init() {
	MetricsCmd.AddCommand(metricsReportCmd)
}

func runMetricsReport(cmd *cobra.Command, args []string) error {
	fmt.Println("=== GitHub: Deployment Frequency ===")
	fmt.Println()
	if err := runDeploymentFrequency(cmd, args); err != nil {
		fmt.Printf("Warning: GitHub report skipped: %v\n", err)
	}

	fmt.Println()
	fmt.Println("=== JIRA: Metrics Report ===")
	fmt.Println()
	if err := runReport(cmd, args); err != nil {
		fmt.Printf("Warning: JIRA report skipped: %v\n", err)
	}

	return nil
}
