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

	"em/internal/metrics"
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

// SnykIssueWeek holds open vulnerability counts at the end of a week.
type SnykIssueWeek struct {
	WeekStart       time.Time
	Total           int
	Fixable         int
	Unfixable       int
	IgnoredFixable  int
	IgnoredUnfixable int
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
	var points []point
	for _, ct := range data {
		y := math.Round(ct.CycleTimeDays()*10) / 10
		if y == 0 {
			continue
		}
		points = append(points, point{
			X: ct.EndDate.Format("2006-01-02"),
			Y: y,
		})
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

// Widget is a single square status tile in a widget grid page.
type Widget struct {
	Name       string // displayed at the top of the widget
	Definition string // optional secondary line below the name (e.g. "SLI 99.88% / target 99.90%")
	Value      string // large number or short status text
	Label      string // small descriptive label below the value
	StateClass string // "widget-alerted" (red) or "widget-ok" (green)
}

// WidgetSection is a labeled group of widgets within a widget page.
type WidgetSection struct {
	Title   string
	Widgets []Widget
}

// WidgetPageData holds the data for a full widget grid HTML page.
// Use Sections to group widgets under service/app headings; use Widgets for a flat list.
type WidgetPageData struct {
	Title    string
	Subtitle string
	Sections []WidgetSection // optional grouped layout
	Widgets  []Widget        // flat layout (used when Sections is empty)
}

// WidgetPage writes an HTML page of square status widgets to path.
func WidgetPage(data WidgetPageData, path string) error {
	return writeHTML(path, "widgets.html.tmpl", data)
}

// SLOWidgetSectionsHTML returns an embeddable HTML fragment of SLO widget sections.
func SLOWidgetSectionsHTML(sections []WidgetSection) (template.HTML, error) {
	return renderHTML("fragment_slo_widgets.html.tmpl", sections)
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

// SnykSummary holds aggregate vulnerability counts for the summary bar.
type SnykSummary struct {
	Critical        int
	High            int
	Medium          int
	Low             int
	Fixable         int
	Unfixable       int
	IgnoredFixable  int
	IgnoredUnfixable int
	// Exploitable counts (Proof of Concept maturity or higher)
	ExploitableCritical int
	ExploitableHigh     int
	ExploitableMedium   int
	ExploitableLow      int
	ExploitableFixable  int
	ExploitableTotal    int
}

// snykIssuesChartConfig builds the Chart.js config for the Snyk issues stacked area chart.
func snykIssuesChartConfig(weeks []SnykIssueWeek, title string) map[string]any {
	labels := make([]string, len(weeks))
	unfixable := make([]int, len(weeks))
	ignoredUnfixable := make([]int, len(weeks))
	fixable := make([]int, len(weeks))
	ignoredFixable := make([]int, len(weeks))
	for i, w := range weeks {
		labels[i] = w.WeekStart.Format("Jan 2")
		unfixable[i] = w.Unfixable
		ignoredUnfixable[i] = w.IgnoredUnfixable
		fixable[i] = w.Fixable
		ignoredFixable[i] = w.IgnoredFixable
	}

	datasets := []map[string]any{
		{
			"label":           "Unfixable",
			"data":            unfixable,
			"backgroundColor": "rgba(185, 28, 28, 0.9)",
			"stack":           "issues",
		},
		{
			"label":           "Ignored Unfixable",
			"data":            ignoredUnfixable,
			"backgroundColor": "rgba(109, 40, 217, 0.85)",
			"stack":           "issues",
		},
		{
			"label":           "Fixable",
			"data":            fixable,
			"backgroundColor": "rgba(15, 118, 110, 0.9)",
			"stack":           "issues",
		},
		{
			"label":           "Ignored Fixable",
			"data":            ignoredFixable,
			"backgroundColor": "rgba(29, 78, 216, 0.85)",
			"stack":           "issues",
		},
	}

	return map[string]any{
		"type": "bar",
		"data": map[string]any{
			"labels":   labels,
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
				"subtitle": map[string]any{
					"display": true,
					"text":    "Plotting when issues were introduced and how fixability changed over time. When an issue is resolved, all bar graphs are reduced.",
					"font":    map[string]any{"size": 12},
					"color":   "#888888",
					"padding": map[string]any{"bottom": 10},
				},
			},
			"scales": map[string]any{
				"x": map[string]any{
					"stacked": true,
					"title": map[string]any{
						"display": true,
						"text":    "Week",
					},
				},
				"y": map[string]any{
					"stacked":     true,
					"beginAtZero": true,
					"title": map[string]any{
						"display": true,
						"text":    "Open Issues",
					},
				},
			},
		},
	}
}

// SnykIssuesLineHTML returns a self-contained HTML fragment for the Snyk issues line chart.
func SnykIssuesLineHTML(weeks []SnykIssueWeek, title string) (template.HTML, error) {
	if title == "" {
		title = "Open Snyk Issues over time"
	}
	cjs, dajs := jsStrings()
	return renderHTML("fragment_chart.html.tmpl", map[string]any{
		"CanvasID":      "snyk-chart",
		"ChartJS":       cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":    mustJSON(snykIssuesChartConfig(weeks, title)),
	})
}

// SnykSummaryHTML returns a self-contained HTML fragment for the Snyk vulnerability summary bar.
func SnykSummaryHTML(s SnykSummary) (template.HTML, error) {
	return renderHTML("fragment_snyk_summary.html.tmpl", s)
}

// SnykSectionReport renders a standalone HTML page that matches the Snyk section
// in the combined team report — same template, same visual style.
func SnykSectionReport(summary SnykSummary, weeks []SnykIssueWeek, title, path string) error {
	if title == "" {
		title = "Snyk Security Report"
	}
	summaryHTML, err := SnykSummaryHTML(summary)
	if err != nil {
		return err
	}
	chartHTML, err := SnykIssuesLineHTML(weeks, "Open Snyk Issues over time")
	if err != nil {
		return err
	}
	return writeHTML(path, "team_report.html.tmpl", map[string]any{
		"Title":           title,
		"SnykSummaryHTML": summaryHTML,
		"SnykChartHTML":   chartHTML,
	})
}

// SnykIssuesLine creates a multi-line HTML chart of Snyk issues by severity.
func SnykIssuesLine(weeks []SnykIssueWeek, cfg Config, path string) error {
	title := cfg.Title
	if title == "" {
		title = "Open Snyk Issues over time"
	}

	cjs, dajs := jsStrings()
	return writeHTML(path, "chart.html.tmpl", map[string]any{
		"Title":         title,
		"ChartJS":       cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":    mustJSON(snykIssuesChartConfig(weeks, title)),
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
	snykSummary SnykSummary,
	snykWeeks []SnykIssueWeek,
	sloSections []WidgetSection,
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
	var snykSummaryHTML, snykChartHTML template.HTML
	if len(snykWeeks) > 0 {
		snykSummaryHTML, err = SnykSummaryHTML(snykSummary)
		if err != nil {
			return err
		}
		snykChartHTML, err = SnykIssuesLineHTML(snykWeeks, "Open Snyk Issues over time")
		if err != nil {
			return err
		}
	}
	var sloHTML template.HTML
	if len(sloSections) > 0 {
		sloHTML, err = SLOWidgetSectionsHTML(sloSections)
		if err != nil {
			return err
		}
	}
	return writeHTML(path, "team_report.html.tmpl", map[string]any{
		"Title":           title,
		"SummaryHTML":     summaryHTML,
		"DeploymentHTML":  dfHTML,
		"CycleTimeHTML":   ctHTML,
		"ThroughputHTML":  tpHTML,
		"LongestCTHTML":   longestHTML,
		"ForecastHTML":    forecastHTML,
		"SnykSummaryHTML": snykSummaryHTML,
		"SnykChartHTML":   snykChartHTML,
		"DatadogHTML":     sloHTML,
	})
}

func formatDays(d float64) string {
	if d < 1 {
		return "<1 day"
	}
	return fmt.Sprintf("%.1f days", d)
}
