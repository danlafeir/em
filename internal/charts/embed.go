package charts

import "embed"

//go:embed static/chart.min.js
var chartJS []byte

//go:embed static/chartjs-adapter-date-fns.bundle.min.js
var dateAdapterJS []byte

//go:embed templates/*.html.tmpl
var templateFS embed.FS
