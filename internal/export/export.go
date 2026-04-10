// Package export provides data export functionality.
package export

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/xuri/excelize/v2"

	"em/internal/output"
	"em/pkg/metrics"
)

// CycleTimeCSV exports cycle time results to CSV.
func CycleTimeCSV(results []metrics.CycleTimeResult, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Header
	header := []string{"Issue Key", "Type", "Summary", "Start Date", "End Date", "Cycle Time (days)"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Data
	for _, r := range results {
		row := []string{
			r.IssueKey,
			r.IssueType,
			r.Summary,
			r.StartDate.Format("2006-01-02"),
			r.EndDate.Format("2006-01-02"),
			strconv.FormatFloat(r.CycleTimeDays(), 'f', 1, 64),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// ThroughputCSV exports throughput results to CSV.
func ThroughputCSV(result metrics.ThroughputResult, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Header
	header := []string{"Period Start", "Period End", "Items Completed"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Data
	for _, p := range result.Periods {
		row := []string{
			p.PeriodStart.Format("2006-01-02"),
			p.PeriodEnd.Format("2006-01-02"),
			strconv.Itoa(p.Count),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// CycleTimeExcel exports cycle time results to Excel.
func CycleTimeExcel(results []metrics.CycleTimeResult, stats metrics.CycleTimeStats, path string) error {
	f := excelize.NewFile()
	defer f.Close()

	// Data sheet
	dataSheet := "Cycle Time Data"
	f.SetSheetName("Sheet1", dataSheet)

	// Headers
	headers := []string{"Issue Key", "Type", "Summary", "Start Date", "End Date", "Cycle Time (days)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(dataSheet, cell, h)
	}

	// Style headers
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#4285F4"}, Pattern: 1},
	})
	f.SetCellStyle(dataSheet, "A1", "F1", headerStyle)

	// Data
	for i, r := range results {
		row := i + 2
		f.SetCellValue(dataSheet, fmt.Sprintf("A%d", row), r.IssueKey)
		f.SetCellValue(dataSheet, fmt.Sprintf("B%d", row), r.IssueType)
		f.SetCellValue(dataSheet, fmt.Sprintf("C%d", row), r.Summary)
		f.SetCellValue(dataSheet, fmt.Sprintf("D%d", row), r.StartDate.Format("2006-01-02"))
		f.SetCellValue(dataSheet, fmt.Sprintf("E%d", row), r.EndDate.Format("2006-01-02"))
		f.SetCellValue(dataSheet, fmt.Sprintf("F%d", row), r.CycleTimeDays())
	}

	// Stats sheet
	statsSheet := "Statistics"
	f.NewSheet(statsSheet)

	statsDays := stats.ToDays()
	statsData := [][]interface{}{
		{"Metric", "Value (days)"},
		{"Count", stats.Count},
		{"Mean", statsDays.Mean},
		{"Median", statsDays.Median},
		{"50th Percentile", statsDays.Percentile50},
		{"70th Percentile", statsDays.Percentile70},
		{"85th Percentile", statsDays.Percentile85},
		{"95th Percentile", statsDays.Percentile95},
		{"Min", statsDays.Min},
		{"Max", statsDays.Max},
		{"Std Dev", statsDays.StdDev},
	}

	for i, row := range statsData {
		for j, val := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			f.SetCellValue(statsSheet, cell, val)
		}
	}

	f.SetCellStyle(statsSheet, "A1", "B1", headerStyle)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return f.SaveAs(path)
}

// ThroughputExcel exports throughput results to Excel.
func ThroughputExcel(result metrics.ThroughputResult, path string) error {
	f := excelize.NewFile()
	defer f.Close()

	dataSheet := "Throughput"
	f.SetSheetName("Sheet1", dataSheet)

	// Headers
	headers := []string{"Period Start", "Period End", "Items"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(dataSheet, cell, h)
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#4285F4"}, Pattern: 1},
	})
	f.SetCellStyle(dataSheet, "A1", "C1", headerStyle)

	// Data
	for i, p := range result.Periods {
		row := i + 2
		f.SetCellValue(dataSheet, fmt.Sprintf("A%d", row), p.PeriodStart.Format("2006-01-02"))
		f.SetCellValue(dataSheet, fmt.Sprintf("B%d", row), p.PeriodEnd.Format("2006-01-02"))
		f.SetCellValue(dataSheet, fmt.Sprintf("C%d", row), p.Count)
	}

	// Summary
	summaryRow := len(result.Periods) + 3
	f.SetCellValue(dataSheet, fmt.Sprintf("A%d", summaryRow), "Summary")
	f.SetCellValue(dataSheet, fmt.Sprintf("A%d", summaryRow+1), "Total Items")
	f.SetCellValue(dataSheet, fmt.Sprintf("B%d", summaryRow+1), result.TotalCount)
	f.SetCellValue(dataSheet, fmt.Sprintf("A%d", summaryRow+2), "Average")
	f.SetCellValue(dataSheet, fmt.Sprintf("B%d", summaryRow+2), result.AvgCount)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return f.SaveAs(path)
}

