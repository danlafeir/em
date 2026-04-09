package charts

import "html/template"

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
