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
)

// Config holds common chart configuration.
type Config struct {
	Title string
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

// chartOrError returns h if err is nil, otherwise an inline error card styled
// consistently with the rest of the report.
func chartOrError(h template.HTML, err error) template.HTML {
	if err == nil {
		return h
	}
	msg := template.HTMLEscapeString(err.Error())
	return template.HTML(`<div style="border:1px solid #fca5a5;border-radius:8px;padding:20px 24px;background:#fef2f2;">` +
		`<div style="font-size:.75rem;font-weight:600;color:#dc2626;text-transform:uppercase;letter-spacing:.06em;margin-bottom:6px;">Chart unavailable</div>` +
		`<div style="font-size:.8rem;font-family:monospace;color:#b91c1c;">` + msg + `</div>` +
		`</div>`)
}

func formatDays(d float64) string {
	if d < 1 {
		return "<1 day"
	}
	return fmt.Sprintf("%.1f days", d)
}
