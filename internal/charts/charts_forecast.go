package charts

import (
	"fmt"
	"html/template"
)

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

// ForecastTable creates an HTML table of epic forecasts.
func ForecastTable(rows []ForecastRow, jiraBaseURL, path string) error {
	content, err := ForecastTableHTML(rows, "Epic Forecast", jiraBaseURL)
	if err != nil {
		return err
	}
	return writePageHTML(path, "Epic Forecast", content)
}
