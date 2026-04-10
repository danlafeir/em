package charts

import (
	"fmt"
	"html/template"
	"math"

	"em/pkg/metrics"
)

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
