package charts

import (
	"html/template"
	"math"

	"em/pkg/metrics"
)

// LongestCycleTimeRow holds data for one row in the longest cycle time table.
type LongestCycleTimeRow struct {
	Key       string
	Summary   string
	Days      string
	Started   string
	Completed string
	Outlier   bool
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
