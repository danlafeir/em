// Package export provides data export functionality.
package export

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/xuri/excelize/v2"

	"devctl-em/internal/output"
	"devctl-em/pkg/metrics"
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
	header := []string{"Issue Key", "Type", "Summary", "Start Date", "End Date", "Cycle Time (days)", "Story Points"}
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
			strconv.FormatFloat(r.StoryPoints, 'f', 1, 64),
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
	header := []string{"Period Start", "Period End", "Items Completed", "Story Points"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Data
	for _, p := range result.Periods {
		row := []string{
			p.PeriodStart.Format("2006-01-02"),
			p.PeriodEnd.Format("2006-01-02"),
			strconv.Itoa(p.Count),
			strconv.FormatFloat(p.Points, 'f', 1, 64),
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
	headers := []string{"Issue Key", "Type", "Summary", "Start Date", "End Date", "Cycle Time (days)", "Story Points"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(dataSheet, cell, h)
	}

	// Style headers
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#4285F4"}, Pattern: 1},
	})
	f.SetCellStyle(dataSheet, "A1", "G1", headerStyle)

	// Data
	for i, r := range results {
		row := i + 2
		f.SetCellValue(dataSheet, fmt.Sprintf("A%d", row), r.IssueKey)
		f.SetCellValue(dataSheet, fmt.Sprintf("B%d", row), r.IssueType)
		f.SetCellValue(dataSheet, fmt.Sprintf("C%d", row), r.Summary)
		f.SetCellValue(dataSheet, fmt.Sprintf("D%d", row), r.StartDate.Format("2006-01-02"))
		f.SetCellValue(dataSheet, fmt.Sprintf("E%d", row), r.EndDate.Format("2006-01-02"))
		f.SetCellValue(dataSheet, fmt.Sprintf("F%d", row), r.CycleTimeDays())
		f.SetCellValue(dataSheet, fmt.Sprintf("G%d", row), r.StoryPoints)
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
	headers := []string{"Period Start", "Period End", "Items", "Points"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(dataSheet, cell, h)
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#4285F4"}, Pattern: 1},
	})
	f.SetCellStyle(dataSheet, "A1", "D1", headerStyle)

	// Data
	for i, p := range result.Periods {
		row := i + 2
		f.SetCellValue(dataSheet, fmt.Sprintf("A%d", row), p.PeriodStart.Format("2006-01-02"))
		f.SetCellValue(dataSheet, fmt.Sprintf("B%d", row), p.PeriodEnd.Format("2006-01-02"))
		f.SetCellValue(dataSheet, fmt.Sprintf("C%d", row), p.Count)
		f.SetCellValue(dataSheet, fmt.Sprintf("D%d", row), p.Points)
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

// HTMLReport generates an HTML report with metrics and charts.
func HTMLReport(title string, sections []HTMLSection, path string) error {
	file, err := output.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write HTML header
	fmt.Fprintf(file, `<!DOCTYPE html>
<html>
<head>
    <title>%s</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .report-header {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            padding: 30px;
            border-radius: 10px;
            margin-bottom: 30px;
        }
        .report-header h1 {
            margin: 0 0 10px 0;
        }
        .report-header .date {
            opacity: 0.9;
        }
        .section {
            background: white;
            border-radius: 10px;
            padding: 25px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .section h2 {
            color: #333;
            border-bottom: 2px solid #667eea;
            padding-bottom: 10px;
            margin-top: 0;
        }
        table {
            width: 100%%;
            border-collapse: collapse;
            margin: 15px 0;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background: #667eea;
            color: white;
        }
        tr:hover {
            background: #f8f9fa;
        }
        .stat-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
            margin: 20px 0;
        }
        .stat-card {
            background: #f8f9fa;
            border-radius: 8px;
            padding: 15px;
            text-align: center;
        }
        .stat-value {
            font-size: 28px;
            font-weight: bold;
            color: #667eea;
        }
        .stat-label {
            color: #666;
            font-size: 14px;
        }
        .chart-container {
            text-align: center;
            margin: 20px 0;
        }
        .chart-container img {
            max-width: 100%%;
            border-radius: 8px;
        }
        .badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 12px;
            font-weight: 500;
        }
        .badge-healthy { background: #d4edda; color: #155724; }
        .badge-warning { background: #fff3cd; color: #856404; }
        .badge-critical { background: #f8d7da; color: #721c24; }
        .footer {
            text-align: center;
            color: #666;
            margin-top: 40px;
            padding: 20px;
        }
    </style>
</head>
<body>
    <div class="report-header">
        <h1>%s</h1>
        <div class="date">Generated on %s</div>
    </div>
`, title, title, time.Now().Format("January 2, 2006 at 3:04 PM"))

	// Write sections
	for _, section := range sections {
		fmt.Fprintf(file, `    <div class="section">
        <h2>%s</h2>
        %s
    </div>
`, section.Title, section.Content)
	}

	// Footer
	fmt.Fprintf(file, `    <div class="footer">
        Generated by devctl-em | JIRA Agile Metrics
    </div>
</body>
</html>`)

	return nil
}

// HTMLSection represents a section in an HTML report.
type HTMLSection struct {
	Title   string
	Content string
}

// FormatStatsHTML formats cycle time stats as HTML.
func FormatStatsHTML(stats metrics.CycleTimeStats) string {
	days := stats.ToDays()
	return fmt.Sprintf(`
        <div class="stat-grid">
            <div class="stat-card">
                <div class="stat-value">%d</div>
                <div class="stat-label">Issues Analyzed</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">%.1f</div>
                <div class="stat-label">Mean (days)</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">%.1f</div>
                <div class="stat-label">Median (days)</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">%.1f</div>
                <div class="stat-label">85th Percentile</div>
            </div>
        </div>
        <table>
            <tr><th>Percentile</th><th>Cycle Time (days)</th></tr>
            <tr><td>50th</td><td>%.1f</td></tr>
            <tr><td>70th</td><td>%.1f</td></tr>
            <tr><td>85th</td><td>%.1f</td></tr>
            <tr><td>95th</td><td>%.1f</td></tr>
            <tr><td>Min</td><td>%.1f</td></tr>
            <tr><td>Max</td><td>%.1f</td></tr>
        </table>
    `, stats.Count, days.Mean, days.Median, days.Percentile85,
		days.Percentile50, days.Percentile70, days.Percentile85, days.Percentile95,
		days.Min, days.Max)
}

// FormatThroughputHTML formats throughput stats as HTML.
func FormatThroughputHTML(result metrics.ThroughputResult) string {
	stats := metrics.CalculateThroughputStats(result)
	return fmt.Sprintf(`
        <div class="stat-grid">
            <div class="stat-card">
                <div class="stat-value">%d</div>
                <div class="stat-label">Total Items</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">%.1f</div>
                <div class="stat-label">Avg per Period</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">%d</div>
                <div class="stat-label">Min</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">%d</div>
                <div class="stat-label">Max</div>
            </div>
        </div>
    `, stats.TotalItems, stats.AvgItems, stats.MinItems, stats.MaxItems)
}

// FormatForecastHTML formats forecast results as HTML.
func FormatForecastHTML(result *metrics.ForecastResult) string {
	html := fmt.Sprintf(`
        <div class="stat-grid">
            <div class="stat-card">
                <div class="stat-value">%d</div>
                <div class="stat-label">Remaining Items</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">%.1f</div>
                <div class="stat-label">Avg Throughput/Week</div>
            </div>
        </div>
        <table>
            <tr><th>Confidence</th><th>Completion Date</th><th>Days from Now</th></tr>
    `, result.RemainingItems, result.AvgThroughput)

	for _, p := range []int{50, 70, 85, 95} {
		html += fmt.Sprintf(`
            <tr>
                <td>%d%%</td>
                <td>%s</td>
                <td>%d</td>
            </tr>
        `, p, result.Percentiles[p].Format("Jan 2, 2006"), result.PercentileDays[p])
	}

	html += "</table>"

	if result.DeadlineDate != nil {
		badgeClass := "badge-healthy"
		if result.DeadlineConfidence < 0.5 {
			badgeClass = "badge-critical"
		} else if result.DeadlineConfidence < 0.85 {
			badgeClass = "badge-warning"
		}
		html += fmt.Sprintf(`
            <p>Deadline: %s
               <span class="badge %s">%.0f%% confidence</span>
            </p>
        `, result.DeadlineDate.Format("Jan 2, 2006"), badgeClass, result.DeadlineConfidence*100)
	}

	return html
}
