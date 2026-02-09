package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"devctl-em/pkg/metrics"
	"devctl-em/pkg/workflow"
)

var wipCmd = &cobra.Command{
	Use:   "wip",
	Short: "Analyze work-in-progress and aging",
	Long: `Analyze current work-in-progress (WIP) and identify aging items.

Shows:
  - Current WIP count by stage
  - Aging items (items that have been in progress too long)
  - Historical WIP trends

Example:
  devctl-em metrics jira wip --jql "project = MYPROJ"
  devctl-em metrics jira wip --jql "project = MYPROJ" --warning-days 7 --critical-days 14`,
	RunE: runWIP,
}

var (
	warningDaysFlag  int
	criticalDaysFlag int
)

func init() {
	JiraCmd.AddCommand(wipCmd)

	// WIP-specific flags
	wipCmd.Flags().IntVar(&warningDaysFlag, "warning-days", 7, "Days in stage before warning")
	wipCmd.Flags().IntVar(&criticalDaysFlag, "critical-days", 14, "Days in stage before critical")
}

func runWIP(cmd *cobra.Command, args []string) error {
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

	// Get JQL
	jql, err := getJQL()
	if err != nil {
		return err
	}

	// For WIP, we want all open issues
	jqlOpen := fmt.Sprintf("(%s) AND resolution IS EMPTY", jql)

	fmt.Printf("Fetching open issues from JIRA...\n")
	fmt.Printf("JQL: %s\n", jqlOpen)

	// Fetch issues with history
	issues, err := client.FetchIssuesWithHistory(ctx, jqlOpen, func(current, total int) {
		fmt.Printf("\rProcessing issue %d/%d...", current, total)
	})
	if err != nil {
		return fmt.Errorf("failed to fetch issues: %w", err)
	}
	fmt.Println()

	if len(issues) == 0 {
		fmt.Println("No open issues found matching the query.")
		return nil
	}

	fmt.Printf("Found %d open issues\n\n", len(issues))

	// Get workflow mapper
	mapper := getWorkflowMapper()

	// Map issues to workflow history
	histories := make([]workflow.IssueHistory, len(issues))
	for i, issue := range issues {
		histories[i] = mapper.MapIssueHistory(issue)
	}

	// Calculate WIP
	from, to, _ := getDateRange()
	calculator := metrics.NewWIPCalculator(mapper)
	result := calculator.Calculate(histories, from, to)

	// Set thresholds
	thresholds := metrics.AgingThresholds{
		Warning:  time.Duration(warningDaysFlag) * 24 * time.Hour,
		Critical: time.Duration(criticalDaysFlag) * 24 * time.Hour,
	}

	// Categorize items
	healthy, warning, critical := metrics.CategorizeByAge(result.CurrentWIP, thresholds)

	// Print summary
	fmt.Printf("Work-In-Progress Analysis\n")
	fmt.Printf("=========================\n\n")

	fmt.Printf("Current WIP Summary:\n")
	fmt.Printf("  Total items in progress: %d\n", len(result.CurrentWIP))
	fmt.Printf("  Healthy (< %d days):     %d\n", warningDaysFlag, len(healthy))
	fmt.Printf("  Warning (%d-%d days):    %d\n", warningDaysFlag, criticalDaysFlag, len(warning))
	fmt.Printf("  Critical (> %d days):    %d\n", criticalDaysFlag, len(critical))

	// WIP by stage
	fmt.Printf("\nWIP by Stage:\n")
	for _, stage := range mapper.GetStages() {
		if stage.Category == "in_progress" {
			count := len(result.ByStage[stage.Name])
			fmt.Printf("  %s: %d\n", stage.Name, count)
		}
	}

	// Show critical items
	if len(critical) > 0 {
		fmt.Printf("\nCritical Aging Items (> %d days):\n", criticalDaysFlag)
		for _, item := range critical {
			ageDays := int(item.Age.Hours() / 24)
			fmt.Printf("  [%s] %s - %d days in %s\n",
				item.Key, truncate(item.Summary, 50), ageDays, item.Stage)
		}
	}

	// Show warning items
	if len(warning) > 0 {
		fmt.Printf("\nWarning Aging Items (%d-%d days):\n", warningDaysFlag, criticalDaysFlag)
		for _, item := range warning {
			ageDays := int(item.Age.Hours() / 24)
			fmt.Printf("  [%s] %s - %d days in %s\n",
				item.Key, truncate(item.Summary, 50), ageDays, item.Stage)
		}
	}

	// Export to CSV
	outputPath := getOutputPath("wip", "csv")
	if err := exportWIPCSV(result.CurrentWIP, thresholds, outputPath); err != nil {
		return fmt.Errorf("failed to export CSV: %w", err)
	}
	fmt.Printf("\nExported to %s\n", outputPath)

	return nil
}

func exportWIPCSV(items []metrics.WIPItem, thresholds metrics.AgingThresholds, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Issue Key", "Type", "Summary", "Stage", "Age (days)", "Entered Stage", "Status"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, item := range items {
		ageDays := item.Age.Hours() / 24

		status := "healthy"
		if item.Age >= thresholds.Critical {
			status = "critical"
		} else if item.Age >= thresholds.Warning {
			status = "warning"
		}

		row := []string{
			item.Key,
			item.Type,
			item.Summary,
			item.Stage,
			strconv.FormatFloat(ageDays, 'f', 1, 64),
			item.EnteredDate.Format("2006-01-02"),
			status,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
