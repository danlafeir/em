// Package charts provides HTML chart generation for metrics visualization.
package charts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"time"

	"devctl-em/internal/metrics"
)

// Config holds common chart configuration.
type Config struct {
	Title string
}

// ForecastRow holds data for one row in the forecast table.
type ForecastRow struct {
	EpicKey    string
	Summary    string
	Completed  int
	Total      int
	Remaining  int
	Forecast50 string
	Forecast85 string
	Forecast95 string
}

// forecastTableRow is the HTML template view of a ForecastRow.
type forecastTableRow struct {
	EpicHTML      template.HTML
	ProgressVal   int
	ProgressMax   int
	ProgressLabel string
	Forecast50    string
	Forecast85    string
	Forecast95    string
}

// LongestCycleTimeRow holds data for one row in the longest cycle time table.
type LongestCycleTimeRow struct {
	Key       string
	Summary   string
	Days      string
	Started   string
	Completed string
	Outlier   bool
}

// SnykIssueWeek holds vulnerability counts by severity for a single week.
type SnykIssueWeek struct {
	WeekStart              time.Time
	Critical, High, Medium, Low int
}

// tableRow is used by table templates.
type tableRow struct {
	Cells   []template.HTML
	Outlier bool
}

func writeHTML(path string, tmplName string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmpl, err := template.ParseFS(templateFS, "templates/"+tmplName)
	if err != nil {
		return fmt.Errorf("parse template %s: %w", tmplName, err)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func renderHTML(tmplName string, data any) (template.HTML, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/"+tmplName)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", tmplName, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

func writePageHTML(path, title string, content template.HTML) error {
	return writeHTML(path, "page.html.tmpl", map[string]any{
		"Title":   title,
		"Content": content,
	})
}

func mustJSON(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b)
}

func jsStrings() (template.JS, template.JS) {
	return template.JS(chartJS), template.JS(dateAdapterJS)
}

// linearRegression computes slope and intercept for y = slope*x + intercept.
func linearRegression(xs, ys []float64) (slope, intercept float64) {
	n := float64(len(xs))
	var sumX, sumY, sumXY, sumX2 float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
	}
	denom := n*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-12 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return
}

// CycleTimeScatterHTML returns a self-contained HTML fragment for the cycle time scatter chart.
func CycleTimeScatterHTML(data []metrics.CycleTimeResult, percentiles []float64, title string) (template.HTML, error) {
	if title == "" {
		title = "Cycle Time Scatter Plot"
	}

	type point struct {
		X string  `json:"x"`
		Y float64 `json:"y"`
	}
	points := make([]point, len(data))
	for i, ct := range data {
		points[i] = point{
			X: ct.EndDate.Format("2006-01-02"),
			Y: math.Round(ct.CycleTimeDays()*10) / 10,
		}
	}

	datasets := []map[string]any{
		{
			"label":           "Cycle Time",
			"data":            points,
			"backgroundColor": "rgba(66, 133, 244, 0.7)",
			"borderColor":     "rgba(66, 133, 244, 1)",
			"pointRadius":     4,
			"showLine":        false,
		},
	}

	if len(data) > 1 {
		stats := metrics.CalculateStats(data)
		statsDays := stats.ToDays()

		type pctLine struct {
			label string
			value float64
			color string
		}
		lines := []pctLine{
			{"50th: " + formatDays(statsDays.Percentile50), statsDays.Percentile50, "rgba(76, 175, 80, 0.8)"},
			{"85th: " + formatDays(statsDays.Percentile85), statsDays.Percentile85, "rgba(255, 152, 0, 0.8)"},
			{"95th: " + formatDays(statsDays.Percentile95), statsDays.Percentile95, "rgba(244, 67, 54, 0.8)"},
		}

		xMin := data[0].EndDate.Format("2006-01-02")
		xMax := data[len(data)-1].EndDate.Format("2006-01-02")

		for _, l := range lines {
			datasets = append(datasets, map[string]any{
				"label":       l.label,
				"data":        []point{{X: xMin, Y: l.value}, {X: xMax, Y: l.value}},
				"type":        "line",
				"showLine":    true,
				"pointRadius": 0,
				"borderColor": l.color,
				"borderWidth": 2,
				"borderDash":  []int{6, 3},
			})
		}
	}

	chartConfig := map[string]any{
		"type": "scatter",
		"data": map[string]any{
			"datasets": datasets,
		},
		"options": map[string]any{
			"responsive":          true,
			"maintainAspectRatio": false,
			"plugins": map[string]any{
				"title": map[string]any{
					"display": true,
					"text":    title,
					"font":    map[string]any{"size": 16},
				},
			},
			"scales": map[string]any{
				"x": map[string]any{
					"type": "time",
					"time": map[string]any{"unit": "day"},
					"title": map[string]any{
						"display": true,
						"text":    "Completion Date",
					},
				},
				"y": map[string]any{
					"title": map[string]any{
						"display": true,
						"text":    "Cycle Time (days)",
					},
					"beginAtZero": true,
				},
			},
		},
	}

	cjs, dajs := jsStrings()
	return renderHTML("fragment_chart.html.tmpl", map[string]any{
		"CanvasID":      "ct-chart",
		"ChartJS":       cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":    mustJSON(chartConfig),
	})
}

// ThroughputLineHTML returns a self-contained HTML fragment for the throughput line chart.
func ThroughputLineHTML(data metrics.ThroughputResult, title string) (template.HTML, error) {
	if title == "" {
		title = "Throughput Over Time"
	}

	type point struct {
		X string `json:"x"`
		Y int    `json:"y"`
	}
	points := make([]point, len(data.Periods))
	xs := make([]float64, len(data.Periods))
	ys := make([]float64, len(data.Periods))
	for i, p := range data.Periods {
		points[i] = point{X: p.PeriodEnd.Format("2006-01-02"), Y: p.Count}
		xs[i] = float64(p.PeriodEnd.Unix())
		ys[i] = float64(p.Count)
	}

	datasets := []map[string]any{
		{
			"label":           "Throughput",
			"data":            points,
			"borderColor":     "rgba(66, 133, 244, 1)",
			"backgroundColor": "rgba(66, 133, 244, 0.1)",
			"borderWidth":     2,
			"pointRadius":     4,
			"fill":            true,
		},
	}

	if len(points) >= 2 {
		slope, intercept := linearRegression(xs, ys)
		trendPoints := []point{
			{X: points[0].X, Y: int(math.Round(slope*xs[0] + intercept))},
			{X: points[len(points)-1].X, Y: int(math.Round(slope*xs[len(xs)-1] + intercept))},
		}
		datasets = append(datasets, map[string]any{
			"label":       "Trend",
			"data":        trendPoints,
			"borderColor": "rgba(244, 67, 54, 0.8)",
			"borderWidth": 1.5,
			"borderDash":  []int{6, 3},
			"pointRadius": 0,
			"fill":        false,
		})
	}

	chartConfig := map[string]any{
		"type": "line",
		"data": map[string]any{
			"datasets": datasets,
		},
		"options": map[string]any{
			"responsive":          true,
			"maintainAspectRatio": false,
			"plugins": map[string]any{
				"title": map[string]any{
					"display": true,
					"text":    title,
					"font":    map[string]any{"size": 16},
				},
				"subtitle": map[string]any{
					"display": true,
					"text":    fmt.Sprintf("Avg: %.1f items/week", data.AvgCount),
					"color":   "rgba(0,0,0,0.5)",
					"font":    map[string]any{"size": 12},
					"padding": map[string]any{"bottom": 8},
				},
			},
			"scales": map[string]any{
				"x": map[string]any{
					"type":  "time",
					"time":  map[string]any{"unit": "week"},
					"ticks": map[string]any{"source": "data"},
					"title": map[string]any{
						"display": true,
						"text":    "Period",
					},
				},
				"y": map[string]any{
					"title": map[string]any{
						"display": true,
						"text":    "Items Completed",
					},
					"beginAtZero": true,
				},
			},
		},
	}

	cjs, dajs := jsStrings()
	return renderHTML("fragment_chart.html.tmpl", map[string]any{
		"CanvasID":      "tp-chart",
		"ChartJS":       cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":    mustJSON(chartConfig),
	})
}

// LongestCycleTimeTableHTML returns a self-contained HTML fragment for the CT table.
func LongestCycleTimeTableHTML(rows []LongestCycleTimeRow, title, jiraBaseURL string) (template.HTML, error) {
	if title == "" {
		title = "Longest Cycle Times"
	}
	tRows := make([]tableRow, len(rows))
	for i, r := range rows {
		outlierMark := template.HTML("")
		if r.Outlier {
			outlierMark = "*"
		}
		keyHTML := template.HTML(template.HTMLEscapeString(r.Key) + ": " + template.HTMLEscapeString(r.Summary))
		if jiraBaseURL != "" {
			href := template.HTMLEscapeString(jiraBaseURL + "/browse/" + r.Key)
			keyHTML = template.HTML(`<a href="` + href + `" target="_blank">` + template.HTMLEscapeString(r.Key) + `</a>: ` + template.HTMLEscapeString(r.Summary))
		}
		tRows[i] = tableRow{
			Cells:   []template.HTML{outlierMark, keyHTML, template.HTML(template.HTMLEscapeString(r.Days)), template.HTML(template.HTMLEscapeString(r.Started)), template.HTML(template.HTMLEscapeString(r.Completed))},
			Outlier: r.Outlier,
		}
	}
	return renderHTML("fragment_ct_table.html.tmpl", map[string]any{
		"Title":   title,
		"Headers": []string{"", "Epic", "Days", "Started", "Done"},
		"Rows":    tRows,
	})
}

// ForecastTableHTML returns a self-contained HTML fragment for the forecast table.
func ForecastTableHTML(rows []ForecastRow, title, jiraBaseURL string) (template.HTML, error) {
	if title == "" {
		title = "Epic Forecast"
	}
	tRows := make([]forecastTableRow, len(rows))
	for i, r := range rows {
		epicHTML := template.HTML(template.HTMLEscapeString(r.EpicKey) + ": " + template.HTMLEscapeString(r.Summary))
		if jiraBaseURL != "" {
			href := template.HTMLEscapeString(jiraBaseURL + "/browse/" + r.EpicKey)
			epicHTML = template.HTML(`<a href="` + href + `" target="_blank">` + template.HTMLEscapeString(r.EpicKey) + `</a>: ` + template.HTMLEscapeString(r.Summary))
		}
		tRows[i] = forecastTableRow{
			EpicHTML:      epicHTML,
			ProgressVal:   r.Completed,
			ProgressMax:   r.Total,
			ProgressLabel: fmt.Sprintf("%d/%d", r.Completed, r.Total),
			Forecast50:    r.Forecast50,
			Forecast85:    r.Forecast85,
			Forecast95:    r.Forecast95,
		}
	}
	return renderHTML("forecast.html.tmpl", map[string]any{
		"Title": title,
		"Rows":  tRows,
	})
}

// CycleTimeScatter creates an HTML scatter plot of cycle times.
func CycleTimeScatter(data []metrics.CycleTimeResult, percentiles []float64, cfg Config, path string) error {
	title := cfg.Title
	if title == "" {
		title = "Cycle Time Scatter Plot"
	}
	content, err := CycleTimeScatterHTML(data, percentiles, title)
	if err != nil {
		return err
	}
	return writePageHTML(path, title, content)
}

// ThroughputLine creates an HTML line chart of throughput over time.
func ThroughputLine(data metrics.ThroughputResult, cfg Config, path string) error {
	title := cfg.Title
	if title == "" {
		title = "Throughput Over Time"
	}
	content, err := ThroughputLineHTML(data, title)
	if err != nil {
		return err
	}
	return writePageHTML(path, title, content)
}

// LongestCycleTimeTable creates an HTML table of longest cycle times.
func LongestCycleTimeTable(rows []LongestCycleTimeRow, title, jiraBaseURL, path string) error {
	content, err := LongestCycleTimeTableHTML(rows, title, jiraBaseURL)
	if err != nil {
		return err
	}
	if title == "" {
		title = "Longest Cycle Times"
	}
	return writePageHTML(path, title, content)
}

// ForecastTable creates an HTML table of epic forecasts.
func ForecastTable(rows []ForecastRow, jiraBaseURL, path string) error {
	content, err := ForecastTableHTML(rows, "Epic Forecast", jiraBaseURL)
	if err != nil {
		return err
	}
	return writePageHTML(path, "Epic Forecast", content)
}

// DeploymentFrequencyLineHTML returns a self-contained HTML fragment for the deployment frequency line chart.
func DeploymentFrequencyLineHTML(data metrics.ThroughputResult, title string) (template.HTML, error) {
	if title == "" {
		title = "Deployment Frequency"
	}

	type point struct {
		X string `json:"x"`
		Y int    `json:"y"`
	}
	points := make([]point, len(data.Periods))
	xs := make([]float64, len(data.Periods))
	ys := make([]float64, len(data.Periods))
	for i, p := range data.Periods {
		points[i] = point{X: p.PeriodEnd.Format("2006-01-02"), Y: p.Count}
		xs[i] = float64(p.PeriodEnd.Unix())
		ys[i] = float64(p.Count)
	}

	datasets := []map[string]any{
		{
			"label":           "Deployments",
			"data":            points,
			"borderColor":     "rgba(66, 133, 244, 1)",
			"backgroundColor": "rgba(66, 133, 244, 0.1)",
			"borderWidth":     2,
			"pointRadius":     4,
			"fill":            true,
		},
	}

	if len(points) >= 2 {
		slope, intercept := linearRegression(xs, ys)
		trendPoints := []point{
			{X: points[0].X, Y: int(math.Round(slope*xs[0] + intercept))},
			{X: points[len(points)-1].X, Y: int(math.Round(slope*xs[len(xs)-1] + intercept))},
		}
		datasets = append(datasets, map[string]any{
			"label":       "Trend",
			"data":        trendPoints,
			"borderColor": "rgba(244, 67, 54, 0.8)",
			"borderWidth": 1.5,
			"borderDash":  []int{6, 3},
			"pointRadius": 0,
			"fill":        false,
		})
	}

	chartConfig := map[string]any{
		"type": "line",
		"data": map[string]any{
			"datasets": datasets,
		},
		"options": map[string]any{
			"responsive":          true,
			"maintainAspectRatio": false,
			"plugins": map[string]any{
				"title": map[string]any{
					"display": true,
					"text":    title,
					"font":    map[string]any{"size": 16},
				},
				"subtitle": map[string]any{
					"display": true,
					"text":    fmt.Sprintf("Avg: %.1f deploys/week", data.AvgCount),
					"color":   "rgba(0,0,0,0.5)",
					"font":    map[string]any{"size": 12},
					"padding": map[string]any{"bottom": 8},
				},
			},
			"scales": map[string]any{
				"x": map[string]any{
					"type":  "time",
					"time":  map[string]any{"unit": "week"},
					"ticks": map[string]any{"source": "data"},
					"title": map[string]any{
						"display": true,
						"text":    "Period",
					},
				},
				"y": map[string]any{
					"title": map[string]any{
						"display": true,
						"text":    "Deployments",
					},
					"beginAtZero": true,
				},
			},
		},
	}

	cjs, dajs := jsStrings()
	return renderHTML("fragment_chart.html.tmpl", map[string]any{
		"CanvasID":      "df-chart",
		"ChartJS":       cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":    mustJSON(chartConfig),
	})
}

// DeploymentFrequencyLine creates an HTML page with a deployment frequency line chart.
func DeploymentFrequencyLine(data metrics.ThroughputResult, cfg Config, path string) error {
	title := cfg.Title
	if title == "" {
		title = "Deployment Frequency"
	}
	content, err := DeploymentFrequencyLineHTML(data, title)
	if err != nil {
		return err
	}
	return writePageHTML(path, title, content)
}

// SnykIssuesLine creates a multi-line HTML chart of Snyk issues by severity.
func SnykIssuesLine(weeks []SnykIssueWeek, cfg Config, path string) error {
	title := cfg.Title
	if title == "" {
		title = "Snyk Issues — Weekly Trend"
	}

	type point struct {
		X string `json:"x"`
		Y int    `json:"y"`
	}

	type seriesDef struct {
		name  string
		color string
		val   func(SnykIssueWeek) int
	}

	series := []seriesDef{
		{"Critical", "rgba(220, 38, 38, 1)", func(w SnykIssueWeek) int { return w.Critical }},
		{"High", "rgba(234, 88, 12, 1)", func(w SnykIssueWeek) int { return w.High }},
		{"Medium", "rgba(202, 138, 4, 1)", func(w SnykIssueWeek) int { return w.Medium }},
		{"Low", "rgba(37, 99, 235, 1)", func(w SnykIssueWeek) int { return w.Low }},
	}

	var datasets []map[string]any
	for _, s := range series {
		pts := make([]point, len(weeks))
		for i, w := range weeks {
			pts[i] = point{X: w.WeekStart.Format("2006-01-02"), Y: s.val(w)}
		}
		datasets = append(datasets, map[string]any{
			"label":       s.name,
			"data":        pts,
			"borderColor": s.color,
			"borderWidth": 2,
			"pointRadius": 3,
			"fill":        false,
		})
	}

	chartConfig := map[string]any{
		"type": "line",
		"data": map[string]any{
			"datasets": datasets,
		},
		"options": map[string]any{
			"responsive": true,
			"plugins": map[string]any{
				"title": map[string]any{
					"display": true,
					"text":    title,
					"font":    map[string]any{"size": 18},
				},
			},
			"scales": map[string]any{
				"x": map[string]any{
					"type": "time",
					"time": map[string]any{"unit": "week"},
					"title": map[string]any{
						"display": true,
						"text":    "Week",
					},
				},
				"y": map[string]any{
					"title": map[string]any{
						"display": true,
						"text":    "Issues",
					},
					"beginAtZero": true,
				},
			},
		},
	}

	cjs, dajs := jsStrings()
	return writeHTML(path, "chart.html.tmpl", map[string]any{
		"Title":         title,
		"ChartJS":       cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":    mustJSON(chartConfig),
	})
}

// CombinedReport renders a 2x2 HTML report with cycle time, throughput, longest CT, and forecast.
func CombinedReport(
	summary ReportSummary,
	cycleTimeData []metrics.CycleTimeResult,
	cycleTimePercentiles []float64,
	throughputData metrics.ThroughputResult,
	longestCTRows []LongestCycleTimeRow,
	forecastRows []ForecastRow,
	jiraBaseURL string,
	path string,
) error {
	summaryHTML, err := ReportSummaryHTML(summary)
	if err != nil {
		return err
	}
	ctHTML, err := CycleTimeScatterHTML(cycleTimeData, cycleTimePercentiles, "Cycle Time Distribution")
	if err != nil {
		return err
	}
	tpHTML, err := ThroughputLineHTML(throughputData, "Weekly Throughput")
	if err != nil {
		return err
	}
	longestHTML, err := LongestCycleTimeTableHTML(longestCTRows, "Longest Cycle Times", jiraBaseURL)
	if err != nil {
		return err
	}
	forecastHTML, err := ForecastTableHTML(forecastRows, "Epic Forecast", jiraBaseURL)
	if err != nil {
		return err
	}
	return writeHTML(path, "report.html.tmpl", map[string]any{
		"SummaryHTML":    summaryHTML,
		"CycleTimeHTML":  ctHTML,
		"ThroughputHTML": tpHTML,
		"LongestCTHTML":  longestHTML,
		"ForecastHTML":   forecastHTML,
	})
}

// ReportSummary holds the key metrics displayed in the summary bar.
type ReportSummary struct {
	AvgCycleTime string
	AvgThroughput string
	ActiveEpics  int
}

// ReportSummaryHTML returns a self-contained HTML fragment for the summary bar.
func ReportSummaryHTML(s ReportSummary) (template.HTML, error) {
	return renderHTML("fragment_summary.html.tmpl", s)
}

// CombinedTeamReport renders an HTML report with GitHub deployment frequency and JIRA metrics sections.
func CombinedTeamReport(
	title string,
	summary ReportSummary,
	deploymentData metrics.ThroughputResult,
	cycleTimeData []metrics.CycleTimeResult,
	cycleTimePercentiles []float64,
	throughputData metrics.ThroughputResult,
	longestCTRows []LongestCycleTimeRow,
	forecastRows []ForecastRow,
	jiraBaseURL string,
	path string,
) error {
	var dfHTML template.HTML
	if len(deploymentData.Periods) > 0 {
		var err error
		dfHTML, err = DeploymentFrequencyLineHTML(deploymentData, "Deployment Frequency")
		if err != nil {
			return err
		}
	}
	ctHTML, err := CycleTimeScatterHTML(cycleTimeData, cycleTimePercentiles, "Cycle Time Distribution")
	if err != nil {
		return err
	}
	tpHTML, err := ThroughputLineHTML(throughputData, "Weekly Throughput")
	if err != nil {
		return err
	}
	longestHTML, err := LongestCycleTimeTableHTML(longestCTRows, "Longest Cycle Times", jiraBaseURL)
	if err != nil {
		return err
	}
	forecastHTML, err := ForecastTableHTML(forecastRows, "Epic Forecast", jiraBaseURL)
	if err != nil {
		return err
	}
	summaryHTML, err := ReportSummaryHTML(summary)
	if err != nil {
		return err
	}
	return writeHTML(path, "team_report.html.tmpl", map[string]any{
		"Title":          title,
		"SummaryHTML":    summaryHTML,
		"DeploymentHTML": dfHTML,
		"CycleTimeHTML":  ctHTML,
		"ThroughputHTML": tpHTML,
		"LongestCTHTML":  longestHTML,
		"ForecastHTML":   forecastHTML,
	})
}

func formatDays(d float64) string {
	if d < 1 {
		return "<1 day"
	}
	return fmt.Sprintf("%.1f days", d)
}
