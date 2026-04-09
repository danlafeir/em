package charts

import (
	"html/template"
	"time"
)

// SnykIssueWeek holds open vulnerability counts at the end of a week.
type SnykIssueWeek struct {
	WeekStart        time.Time
	Total            int
	Fixable          int
	Unfixable        int
	IgnoredFixable   int
	IgnoredUnfixable int
}

// SnykSummary holds aggregate vulnerability counts for the summary bar.
type SnykSummary struct {
	Critical         int
	High             int
	Medium           int
	Low              int
	Fixable          int
	Unfixable        int
	IgnoredFixable   int
	IgnoredUnfixable int
	// Exploitable counts (Proof of Concept maturity or higher)
	ExploitableCritical         int
	ExploitableHigh             int
	ExploitableMedium           int
	ExploitableLow              int
	ExploitableFixable          int
	ExploitableUnfixable        int
	ExploitableIgnoredFixable   int
	ExploitableIgnoredUnfixable int
	ExploitableTotal            int
}

// snykIssuesChartConfig builds the Chart.js config for the Snyk issues stacked bar chart.
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
			"label":           "Ignored Unfixable",
			"data":            ignoredUnfixable,
			"backgroundColor": "#16a34a",
			"stack":           "issues",
		},
		{
			"label":           "Unfixable",
			"data":            unfixable,
			"backgroundColor": "#7c3aed",
			"stack":           "issues",
		},
		{
			"label":           "Fixable",
			"data":            fixable,
			"backgroundColor": "#ea580c",
			"stack":           "issues",
		},
		{
			"label":           "Ignored Fixable",
			"data":            ignoredFixable,
			"backgroundColor": "#dc2626",
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

// SnykIssuesLineHTML returns a self-contained HTML fragment for the Snyk issues chart.
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
	return writeHTML(path, "team_report.html.tmpl", map[string]any{
		"Title":           title,
		"SnykSummaryHTML": chartOrError(SnykSummaryHTML(summary)),
		"SnykChartHTML":   chartOrError(SnykIssuesLineHTML(weeks, "Open Snyk Issues over time")),
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
