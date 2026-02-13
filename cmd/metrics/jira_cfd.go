package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"devctl-em/internal/output"
	"devctl-em/internal/metrics"
	"devctl-em/internal/workflow"
)

var cfdCmd = &cobra.Command{
	Use:   "cfd",
	Short: "Generate Cumulative Flow Diagram data",
	Long: `Generate Cumulative Flow Diagram (CFD) data showing work items across stages over time.

CFD shows the cumulative count of items that have reached each workflow stage,
helping visualize flow, bottlenecks, and work-in-progress.

Example:
  devctl-em metrics jira cfd --jql "project = MYPROJ" --from 2024-01-01
  devctl-em metrics jira cfd --jql "project = MYPROJ" -o cfd.csv`,
	RunE: runCFD,
}

func init() {
	JiraCmd.AddCommand(cfdCmd)
}

func runCFD(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get JIRA client
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	// Test connection
	if err := client.TestConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to JIRA: %w", err)
	}

	// Get JQL and date range
	jql, err := getJQL()
	if err != nil {
		return err
	}

	from, to, err := getDateRange()
	if err != nil {
		return err
	}

	// Fetch all issues that existed in this time period
	// Include issues created before the end date
	jqlWithDates := fmt.Sprintf("(%s) AND created <= %s", jql, to.Format("2006-01-02"))

	fmt.Printf("Fetching issues from JIRA...\n")
	fmt.Printf("JQL: %s\n", jqlWithDates)

	// Fetch issues with history
	issues, err := client.FetchIssuesWithHistory(ctx, jqlWithDates, func(current, total int) {
		fmt.Printf("\rProcessing issue %d/%d...", current, total)
	})
	if err != nil {
		return fmt.Errorf("failed to fetch issues: %w", err)
	}
	fmt.Println()

	if len(issues) == 0 {
		fmt.Println("No issues found matching the query.")
		return nil
	}

	fmt.Printf("Found %d issues\n\n", len(issues))

	// Get workflow mapper
	mapper := getWorkflowMapper()

	// Map issues to workflow history
	histories := make([]workflow.IssueHistory, len(issues))
	for i, issue := range issues {
		histories[i] = mapper.MapIssueHistory(issue)
	}

	// Calculate CFD
	calculator := metrics.NewCFDCalculator(mapper)
	result := calculator.Calculate(histories, from, to)

	// Print summary
	fmt.Printf("Cumulative Flow Diagram\n")
	fmt.Printf("=======================\n")
	fmt.Printf("Date range: %s to %s\n", from.Format("2006-01-02"), to.Format("2006-01-02"))
	fmt.Printf("Stages: %v\n", result.StageNames)
	fmt.Printf("Data points: %d days\n\n", len(result.DataPoints))

	// Show first and last data points
	if len(result.DataPoints) > 0 {
		first := result.DataPoints[0]
		last := result.DataPoints[len(result.DataPoints)-1]

		fmt.Printf("First day (%s):\n", first.Date.Format("2006-01-02"))
		for _, stage := range result.StageNames {
			fmt.Printf("  %s: %d\n", stage, first.Stages[stage])
		}

		fmt.Printf("\nLast day (%s):\n", last.Date.Format("2006-01-02"))
		for _, stage := range result.StageNames {
			fmt.Printf("  %s: %d\n", stage, last.Stages[stage])
		}
	}

	// Export to CSV
	outputPath := getOutputPath("cfd", "csv")
	if err := exportCFDCSV(result, outputPath); err != nil {
		return fmt.Errorf("failed to export CSV: %w", err)
	}
	fmt.Printf("\nExported to %s\n", outputPath)

	return nil
}

func exportCFDCSV(result metrics.CFDResult, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Date"}
	header = append(header, result.StageNames...)
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, dp := range result.DataPoints {
		row := []string{dp.Date.Format("2006-01-02")}
		for _, stage := range result.StageNames {
			row = append(row, strconv.Itoa(dp.Stages[stage]))
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
