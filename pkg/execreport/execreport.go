// Package execreport provides the Executive Healthcheck data type and HTML rendering
// for use across CLIs that surface team health metrics.
package execreport

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
)

//go:embed templates/*.html.tmpl
var templateFS embed.FS

// ExecHealthcheck holds the data for the Executive Healthcheck section.
type ExecHealthcheck struct {
	Team            string
	AvgCycleTime    string
	AvgThroughput   string
	ActiveEpics     int
	HasJIRAData     bool
	AvgDeployFreq   string
	LastWeekDeploys int
	HasDeployData   bool
	HideDeployFreq  bool // omits the Deploy Frequency widget entirely when true
	Exploitable     int  // total exploitable vulnerabilities
	Critical        int  // total critical vulnerabilities
	High            int  // total high vulnerabilities
	HasSnykData     bool
}

// ExecHealthcheckHTML returns a self-contained HTML fragment for the Executive Healthcheck section.
func ExecHealthcheckHTML(h ExecHealthcheck) (template.HTML, error) {
	return renderHTML("fragment_executive_healthcheck.html.tmpl", h)
}

// WriteHealthcheckPage renders a standalone HTML page containing one healthcheck
// widget per entry in hcs and writes it to path.
func WriteHealthcheckPage(hcs []ExecHealthcheck, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var body bytes.Buffer
	for _, hc := range hcs {
		if hc.Team != "" {
			fmt.Fprintf(&body, `<div style="font-size:1rem;font-weight:600;color:#333;margin-bottom:6px;">%s</div>`, template.HTMLEscapeString(hc.Team))
		}
		fragment, err := ExecHealthcheckHTML(hc)
		if err != nil {
			return fmt.Errorf("render healthcheck for %q: %w", hc.Team, err)
		}
		body.WriteString(string(fragment))
	}

	page := template.HTML(body.String())
	return writePageHTML(path, "Executive Healthcheck", page)
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
	data := map[string]any{
		"Title":   title,
		"Content": content,
	}
	tmpl, err := template.New("page").Parse(pageTmpl)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

const pageTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}}</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 40px; background: #fafafa; color: #111; }
    h1 { font-size: 1.4rem; font-weight: 700; margin-bottom: 32px; }
  </style>
</head>
<body>
  <h1>{{.Title}}</h1>
  {{.Content}}
</body>
</html>`
