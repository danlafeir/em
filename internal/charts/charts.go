// Package charts provides HTML chart generation for metrics visualization.
package charts

import (
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
	Remaining  int
	Forecast50 string
	Forecast85 string
	Forecast95 string
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

// DeploymentWeek holds deployment count for a single week.
type DeploymentWeek struct {
	WeekStart time.Time
	Count     int
}

// SnykIssueWeek holds vulnerability counts by severity for a single week.
type SnykIssueWeek struct {
	WeekStart              time.Time
	Critical, High, Medium, Low int
}

// tableRow is used by table templates.
type tableRow struct {
	Cells   []string
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

// CycleTimeScatter creates an HTML scatter plot of cycle times.
func CycleTimeScatter(data []metrics.CycleTimeResult, percentiles []float64, cfg Config, path string) error {
	title := cfg.Title
	if title == "" {
		title = "Cycle Time Scatter Plot"
	}

	// Build scatter data points
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

	// Add percentile lines
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
	return writeHTML(path, "chart.html.tmpl", map[string]any{
		"Title":        title,
		"ChartJS":      cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":   mustJSON(chartConfig),
	})
}

// ThroughputLine creates an HTML line chart of throughput over time.
func ThroughputLine(data metrics.ThroughputResult, cfg Config, path string) error {
	title := cfg.Title
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
		points[i] = point{X: p.PeriodStart.Format("2006-01-02"), Y: p.Count}
		xs[i] = float64(p.PeriodStart.Unix())
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

	// Add trend line
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
	return writeHTML(path, "chart.html.tmpl", map[string]any{
		"Title":        title,
		"ChartJS":      cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":   mustJSON(chartConfig),
	})
}

// LongestCycleTimeTable creates an HTML table of longest cycle times.
func LongestCycleTimeTable(rows []LongestCycleTimeRow, title string, path string) error {
	if title == "" {
		title = "Longest Cycle Times"
	}

	tRows := make([]tableRow, len(rows))
	for i, r := range rows {
		outlierMark := ""
		if r.Outlier {
			outlierMark = "*"
		}
		tRows[i] = tableRow{
			Cells:   []string{outlierMark, r.Key, r.Summary, r.Days, r.Started, r.Completed},
			Outlier: r.Outlier,
		}
	}

	return writeHTML(path, "table.html.tmpl", map[string]any{
		"Title":   title,
		"Headers": []string{"", "Key", "Title", "Days", "Started", "Done"},
		"Rows":    tRows,
	})
}

// ForecastTable creates an HTML table of epic forecasts.
func ForecastTable(rows []ForecastRow, path string) error {
	tRows := make([]tableRow, len(rows))
	for i, r := range rows {
		tRows[i] = tableRow{
			Cells: []string{r.EpicKey, r.Summary, fmt.Sprintf("%d", r.Remaining), r.Forecast50, r.Forecast85, r.Forecast95},
		}
	}

	return writeHTML(path, "table.html.tmpl", map[string]any{
		"Title":   "Epic Forecast",
		"Headers": []string{"Epic", "Title", "Remaining", "50%", "85%", "95%"},
		"Rows":    tRows,
	})
}

// DeploymentFrequencyLine creates an HTML line chart of deployment frequency.
func DeploymentFrequencyLine(weeks []DeploymentWeek, cfg Config, path string) error {
	title := cfg.Title
	if title == "" {
		title = "Deployment Frequency"
	}

	type point struct {
		X string `json:"x"`
		Y int    `json:"y"`
	}
	points := make([]point, len(weeks))
	xs := make([]float64, len(weeks))
	ys := make([]float64, len(weeks))
	for i, w := range weeks {
		points[i] = point{X: w.WeekStart.Format("2006-01-02"), Y: w.Count}
		xs[i] = float64(w.WeekStart.Unix())
		ys[i] = float64(w.Count)
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
						"text":    "Deployments",
					},
					"beginAtZero": true,
				},
			},
		},
	}

	cjs, dajs := jsStrings()
	return writeHTML(path, "chart.html.tmpl", map[string]any{
		"Title":        title,
		"ChartJS":      cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":   mustJSON(chartConfig),
	})
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
		"Title":        title,
		"ChartJS":      cjs,
		"DateAdapterJS": dajs,
		"ConfigJSON":   mustJSON(chartConfig),
	})
}

// CombinedReport renders a 2x2 HTML report with cycle time, throughput, longest CT, and forecast.
func CombinedReport(
	cycleTimeData []metrics.CycleTimeResult,
	cycleTimePercentiles []float64,
	throughputData metrics.ThroughputResult,
	longestCTRows []LongestCycleTimeRow,
	forecastRows []ForecastRow,
	path string,
) error {
	// Build cycle time chart config
	cycleTimeConfig := buildCycleTimeConfig(cycleTimeData, "Cycle Time Distribution")
	throughputConfig := buildThroughputConfig(throughputData, "Weekly Throughput")

	// Build longest CT table rows
	ctTableRows := make([]tableRow, len(longestCTRows))
	for i, r := range longestCTRows {
		outlierMark := ""
		if r.Outlier {
			outlierMark = "*"
		}
		ctTableRows[i] = tableRow{
			Cells:   []string{outlierMark, r.Key, r.Summary, r.Days, r.Started, r.Completed},
			Outlier: r.Outlier,
		}
	}

	// Build forecast table rows
	fcTableRows := make([]tableRow, len(forecastRows))
	for i, r := range forecastRows {
		fcTableRows[i] = tableRow{
			Cells: []string{r.EpicKey, r.Summary, fmt.Sprintf("%d", r.Remaining), r.Forecast50, r.Forecast85, r.Forecast95},
		}
	}

	cjs, dajs := jsStrings()
	return writeHTML(path, "report.html.tmpl", map[string]any{
		"ChartJS":             cjs,
		"DateAdapterJS":       dajs,
		"CycleTimeConfigJSON": mustJSON(cycleTimeConfig),
		"ThroughputConfigJSON": mustJSON(throughputConfig),
		"LongestCTTitle":      "Longest Cycle Times",
		"LongestCTHeaders":    []string{"", "Key", "Title", "Days", "Started", "Done"},
		"LongestCTRows":       ctTableRows,
		"ForecastHeaders":     []string{"Epic", "Title", "Remaining", "50%", "85%", "95%"},
		"ForecastRows":        fcTableRows,
	})
}

func buildCycleTimeConfig(data []metrics.CycleTimeResult, title string) map[string]any {
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

	return map[string]any{
		"type": "scatter",
		"data": map[string]any{"datasets": datasets},
		"options": map[string]any{
			"responsive": true,
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
					"title": map[string]any{"display": true, "text": "Completion Date"},
				},
				"y": map[string]any{
					"title":       map[string]any{"display": true, "text": "Cycle Time (days)"},
					"beginAtZero": true,
				},
			},
		},
	}
}

func buildThroughputConfig(data metrics.ThroughputResult, title string) map[string]any {
	type point struct {
		X string `json:"x"`
		Y int    `json:"y"`
	}

	points := make([]point, len(data.Periods))
	xs := make([]float64, len(data.Periods))
	ys := make([]float64, len(data.Periods))
	for i, p := range data.Periods {
		points[i] = point{X: p.PeriodStart.Format("2006-01-02"), Y: p.Count}
		xs[i] = float64(p.PeriodStart.Unix())
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

	return map[string]any{
		"type": "line",
		"data": map[string]any{"datasets": datasets},
		"options": map[string]any{
			"responsive": true,
			"plugins": map[string]any{
				"title": map[string]any{
					"display": true,
					"text":    title,
					"font":    map[string]any{"size": 16},
				},
			},
			"scales": map[string]any{
				"x": map[string]any{
					"type":  "time",
					"time":  map[string]any{"unit": "week"},
					"ticks": map[string]any{"source": "data"},
					"title": map[string]any{"display": true, "text": "Period"},
				},
				"y": map[string]any{
					"title":       map[string]any{"display": true, "text": "Items Completed"},
					"beginAtZero": true,
				},
			},
		},
	}
}

func formatDays(d float64) string {
	if d < 1 {
		return "<1 day"
	}
	return fmt.Sprintf("%.1f days", d)
}
