// Package charts provides chart generation for metrics visualization.
package charts

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/font"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/text"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"

	"devctl-em/internal/metrics"
)

// Config holds common chart configuration.
type Config struct {
	Title  string
	Width  vg.Length
	Height vg.Length
	XLabel string
	YLabel string
}

// DefaultConfig returns sensible chart defaults.
func DefaultConfig() Config {
	return Config{
		Width:  30 * vg.Centimeter,
		Height: 18 * vg.Centimeter,
	}
}

// SaveChart saves a plot to the specified file.
// Format is determined by file extension (.png, .svg, .pdf).
func SaveChart(p *plot.Plot, filename string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	return p.Save(cfg.Width, cfg.Height, filename)
}

// CycleTimeScatter creates a scatter plot of cycle times over time.
func CycleTimeScatter(data []metrics.CycleTimeResult, percentiles []float64, cfg Config) (*plot.Plot, error) {
	p := plot.New()
	p.Title.Text = cfg.Title
	if p.Title.Text == "" {
		p.Title.Text = "Cycle Time Scatter Plot"
	}
	p.X.Label.Text = "Completion Date"
	p.Y.Label.Text = "Cycle Time (days)"
	p.X.Padding = vg.Points(0)
	p.Y.Padding = vg.Points(0)


	// Convert data to XY points
	pts := make(plotter.XYs, len(data))
	var cycleTimes []float64

	for i, ct := range data {
		pts[i].X = float64(ct.EndDate.Unix())
		pts[i].Y = ct.CycleTimeDays()
		cycleTimes = append(cycleTimes, ct.CycleTimeDays())
	}

	// Create scatter plot
	scatter, err := plotter.NewScatter(pts)
	if err != nil {
		return nil, err
	}
	scatter.GlyphStyle.Shape = draw.CircleGlyph{}
	scatter.GlyphStyle.Radius = vg.Points(3)
	scatter.GlyphStyle.Color = color.RGBA{R: 66, G: 133, B: 244, A: 255} // Google Blue

	p.Add(scatter)

	// Add percentile lines
	if len(cycleTimes) > 0 && len(pts) > 1 {
		stats := metrics.CalculateStats(data)
		statsDays := stats.ToDays()

		// Add horizontal lines for percentiles
		addPercentileLine(p, pts, statsDays.Percentile50, "50th", color.RGBA{R: 76, G: 175, B: 80, A: 255})
		addPercentileLine(p, pts, statsDays.Percentile85, "85th", color.RGBA{R: 255, G: 152, B: 0, A: 255})
		addPercentileLine(p, pts, statsDays.Percentile95, "95th", color.RGBA{R: 244, G: 67, B: 54, A: 255})
	}

	// Format X axis as dates
	p.X.Tick.Marker = dateTicker{}

	return p, nil
}

// addPercentileLine adds a horizontal percentile line with an inline label.
func addPercentileLine(p *plot.Plot, pts plotter.XYs, value float64, label string, c color.Color) {
	if len(pts) < 2 {
		return
	}

	line, err := plotter.NewLine(plotter.XYs{
		{X: pts[0].X, Y: value},
		{X: pts[len(pts)-1].X, Y: value},
	})
	if err != nil {
		return
	}

	line.LineStyle.Color = c
	line.LineStyle.Width = vg.Points(1.5)
	line.LineStyle.Dashes = []vg.Length{vg.Points(5), vg.Points(3)}

	p.Add(line)

	// Inline label at the right end of the line
	labelPt := plotter.XYs{{X: pts[len(pts)-1].X, Y: value}}
	labels, err := plotter.NewLabels(plotter.XYLabels{
		XYs:    labelPt,
		Labels: []string{label + ": " + formatDays(value)},
	})
	if err != nil {
		return
	}
	labels.TextStyle[0].Color = c
	labels.TextStyle[0].Font.Variant = "Sans"
	labels.Offset.X = vg.Points(4)
	labels.Offset.Y = vg.Points(-2)
	p.Add(labels)
}

// ThroughputLine creates a line chart of throughput over time.
func ThroughputLine(data metrics.ThroughputResult, cfg Config) (*plot.Plot, error) {
	p := plot.New()
	p.Title.Text = cfg.Title
	if p.Title.Text == "" {
		p.Title.Text = "Throughput Over Time"
	}
	p.X.Label.Text = "Period"
	p.Y.Label.Text = "Items Completed"
	p.X.Padding = vg.Points(0)
	p.Y.Padding = vg.Points(0)


	// Convert data to XY points
	pts := make(plotter.XYs, len(data.Periods))
	for i, period := range data.Periods {
		pts[i].X = float64(period.PeriodStart.Unix())
		pts[i].Y = float64(period.Count)
	}

	// Create line plot
	line, err := plotter.NewLine(pts)
	if err != nil {
		return nil, err
	}
	line.LineStyle.Color = color.RGBA{R: 66, G: 133, B: 244, A: 255}
	line.LineStyle.Width = vg.Points(2)

	// Add points
	scatter, err := plotter.NewScatter(pts)
	if err != nil {
		return nil, err
	}
	scatter.GlyphStyle.Shape = draw.CircleGlyph{}
	scatter.GlyphStyle.Radius = vg.Points(4)
	scatter.GlyphStyle.Color = color.RGBA{R: 66, G: 133, B: 244, A: 255}

	// Add average line
	avgLine, err := plotter.NewLine(plotter.XYs{
		{X: pts[0].X, Y: data.AvgCount},
		{X: pts[len(pts)-1].X, Y: data.AvgCount},
	})
	if err == nil {
		avgLine.LineStyle.Color = color.RGBA{R: 244, G: 67, B: 54, A: 200}
		avgLine.LineStyle.Width = vg.Points(1.5)
		avgLine.LineStyle.Dashes = []vg.Length{vg.Points(5), vg.Points(3)}
		p.Add(avgLine)
		p.Legend.Add("Average: "+formatFloat(data.AvgCount), avgLine)
	}

	p.Add(line, scatter)
	p.Legend.Add("Throughput", line)

	// Format X axis as dates
	p.X.Tick.Marker = dateTicker{}

	return p, nil
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

// genericTablePlotter renders a table as text annotations on a plot canvas.
type genericTablePlotter struct {
	headers    []string
	colFracs   []float64
	rows       [][]string
	noTruncate bool
}

func (t genericTablePlotter) Plot(c draw.Canvas, p *plot.Plot) {
	hdlr := text.Plain{
		Fonts: font.DefaultCache,
	}

	headerFont := font.Font{Typeface: "Liberation", Variant: "Sans", Size: vg.Points(11)}
	bodyFont := font.Font{Typeface: "Liberation", Variant: "Sans", Size: vg.Points(10)}

	headerStyle := draw.TextStyle{
		Color:   color.RGBA{R: 255, G: 255, B: 255, A: 255},
		Font:    headerFont,
		Handler: hdlr,
	}
	bodyStyle := draw.TextStyle{
		Color:   color.RGBA{R: 51, G: 51, B: 51, A: 255},
		Font:    bodyFont,
		Handler: hdlr,
	}

	width := c.Max.X - c.Min.X
	rowHeight := vg.Points(18)
	headerHeight := vg.Points(22)

	// Draw header background
	headerY := c.Max.Y - headerHeight
	headerPath := vg.Path{}
	headerPath.Move(vg.Point{X: c.Min.X, Y: headerY})
	headerPath.Line(vg.Point{X: c.Max.X, Y: headerY})
	headerPath.Line(vg.Point{X: c.Max.X, Y: c.Max.Y})
	headerPath.Line(vg.Point{X: c.Min.X, Y: c.Max.Y})
	headerPath.Close()
	c.SetColor(color.RGBA{R: 102, G: 126, B: 234, A: 255})
	c.Fill(headerPath)

	// Draw header text
	for i, h := range t.headers {
		pt := vg.Point{
			X: c.Min.X + vg.Length(t.colFracs[i])*width,
			Y: headerY + vg.Points(5),
		}
		c.FillText(headerStyle, pt, h)
	}

	// Draw data rows
	for r, row := range t.rows {
		y := headerY - vg.Length(r+1)*rowHeight

		// Alternate row background
		if r%2 == 0 {
			bgPath := vg.Path{}
			bgPath.Move(vg.Point{X: c.Min.X, Y: y})
			bgPath.Line(vg.Point{X: c.Max.X, Y: y})
			bgPath.Line(vg.Point{X: c.Max.X, Y: y + rowHeight})
			bgPath.Line(vg.Point{X: c.Min.X, Y: y + rowHeight})
			bgPath.Close()
			c.SetColor(color.RGBA{R: 245, G: 245, B: 245, A: 255})
			c.Fill(bgPath)
		}

		for i, v := range row {
			colStart := vg.Length(t.colFracs[i]) * width
			if !t.noTruncate {
				var colEnd vg.Length
				if i+1 < len(t.colFracs) {
					colEnd = vg.Length(t.colFracs[i+1]) * width
				} else {
					colEnd = width
				}
				avail := colEnd - colStart - vg.Points(4)
				v = truncateToFit(v, bodyStyle, avail)
			}

			pt := vg.Point{
				X: c.Min.X + colStart,
				Y: y + vg.Points(4),
			}
			c.FillText(bodyStyle, pt, v)
		}
	}
}

// truncateToFit truncates s with "..." if its rendered width exceeds avail.
func truncateToFit(s string, sty draw.TextStyle, avail vg.Length) string {
	if sty.Width(s) <= avail {
		return s
	}
	for i := len(s) - 1; i > 0; i-- {
		t := s[:i] + "..."
		if sty.Width(t) <= avail {
			return t
		}
	}
	return "..."
}

func (t genericTablePlotter) DataRange() (xmin, xmax, ymin, ymax float64) {
	return 0, 1, 0, 1
}

// ForecastTable creates a plot that renders a forecast table.
func ForecastTable(rows []ForecastRow) *plot.Plot {
	p := plot.New()
	p.Title.Text = "Epic Forecast"
	p.HideAxes()

	tableRows := make([][]string, len(rows))
	for i, row := range rows {
		tableRows[i] = []string{
			row.EpicKey,
			row.Summary,
			fmt.Sprintf("%d", row.Remaining),
			row.Forecast50,
			row.Forecast85,
			row.Forecast95,
		}
	}

	p.Add(genericTablePlotter{
		headers:  []string{"Epic", "Summary", "Left", "50%", "85%", "95%"},
		colFracs: []float64{0.02, 0.12, 0.62, 0.70, 0.80, 0.90},
		rows:     tableRows,
	})

	return p
}

// LongestCycleTimeRow holds data for one row in the longest cycle time table.
type LongestCycleTimeRow struct {
	Key       string
	Summary   string
	Days      string
	Started   string
	Completed string
}

// LongestCycleTimeTable creates a plot that renders a longest cycle time table.
func LongestCycleTimeTable(rows []LongestCycleTimeRow, title string, noTruncate bool) *plot.Plot {
	p := plot.New()
	p.Title.Text = title
	if p.Title.Text == "" {
		p.Title.Text = "Longest Cycle Times"
	}
	p.HideAxes()

	tableRows := make([][]string, len(rows))
	for i, row := range rows {
		tableRows[i] = []string{row.Key, row.Summary, row.Days, row.Started, row.Completed}
	}

	p.Add(genericTablePlotter{
		headers:    []string{"Key", "Summary", "Days", "Started", "Done"},
		colFracs:   []float64{0.02, 0.12, 0.72, 0.80, 0.90},
		rows:       tableRows,
		noTruncate: noTruncate,
	})

	return p
}

// CombinedReport renders cycle time, throughput, longest cycle time, and forecast plots into a single PNG.
func CombinedReport(cycleTimePlot, throughputPlot, longestCTPlot, forecastPlot *plot.Plot, path string) error {
	const (
		width  = 40 * vg.Centimeter
		height = 30 * vg.Centimeter
	)
	var (
		pad  = vg.Points(10)
		gapX = vg.Points(15)
		gapY = vg.Points(15)
	)

	img := vgimg.New(width, height)
	dc := draw.New(img)

	// Manually divide canvas into 2x2 quadrants to avoid plot.Align
	// axis-alignment issues with hidden-axis table plots.
	// Left column (charts) gets 40%, right column (tables) gets 60%.
	totalW := dc.Max.X - dc.Min.X - 2*pad - gapX
	colW := [2]vg.Length{totalW * 0.4, totalW * 0.6}
	cellH := (dc.Max.Y - dc.Min.Y - 2*pad - gapY) / 2

	quadrant := func(row, col int) draw.Canvas {
		var minX vg.Length
		if col == 0 {
			minX = dc.Min.X + pad
		} else {
			minX = dc.Min.X + pad + colW[0] + gapX
		}
		minY := dc.Max.Y - pad - vg.Length(row+1)*cellH - vg.Length(row)*gapY
		return draw.Crop(dc,
			minX-dc.Min.X,
			-(dc.Max.X - (minX + colW[col])),
			minY-dc.Min.Y,
			-(dc.Max.Y - (minY + cellH)),
		)
	}

	panels := [2][2]*plot.Plot{
		{cycleTimePlot, longestCTPlot},
		{throughputPlot, forecastPlot},
	}

	for r := 0; r < 2; r++ {
		for c := 0; c < 2; c++ {
			if panels[r][c] != nil {
				panels[r][c].Draw(quadrant(r, c))
			}
		}
	}

	// Draw divider lines between sections
	dividerColor := color.RGBA{R: 200, G: 200, B: 200, A: 255}
	dividerWidth := vg.Points(2)

	// Vertical divider
	midX := dc.Min.X + pad + colW[0] + gapX/2
	vLine := vg.Path{}
	vLine.Move(vg.Point{X: midX, Y: dc.Min.Y + pad})
	vLine.Line(vg.Point{X: midX, Y: dc.Max.Y - pad})
	dc.SetLineWidth(dividerWidth)
	dc.SetColor(dividerColor)
	dc.Stroke(vLine)

	// Horizontal divider
	midY := dc.Max.Y - pad - cellH - gapY/2
	hLine := vg.Path{}
	hLine.Move(vg.Point{X: dc.Min.X + pad, Y: midY})
	hLine.Line(vg.Point{X: dc.Max.X - pad, Y: midY})
	dc.Stroke(hLine)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	pngCanvas := vgimg.PngCanvas{Canvas: img}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = pngCanvas.WriteTo(f)
	return err
}

// dateTicker formats X axis as dates.
type dateTicker struct{}

func (dateTicker) Ticks(min, max float64) []plot.Tick {
	var ticks []plot.Tick

	minTime := time.Unix(int64(min), 0)
	maxTime := time.Unix(int64(max), 0)

	// Determine appropriate tick interval based on range
	duration := maxTime.Sub(minTime)
	var interval time.Duration

	switch {
	case duration > 365*24*time.Hour:
		interval = 30 * 24 * time.Hour // Monthly
	case duration > 90*24*time.Hour:
		interval = 14 * 24 * time.Hour // Bi-weekly
	case duration > 30*24*time.Hour:
		interval = 7 * 24 * time.Hour // Weekly
	default:
		interval = 24 * time.Hour // Daily
	}

	current := minTime.Truncate(24 * time.Hour)
	for !current.After(maxTime) {
		ticks = append(ticks, plot.Tick{
			Value: float64(current.Unix()),
			Label: current.Format("Jan 02"),
		})
		current = current.Add(interval)
	}

	return ticks
}

func formatDays(d float64) string {
	if d < 1 {
		return "<1 day"
	}
	return formatFloat(d) + " days"
}

func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return string(rune('0' + int(f)%10))
	}
	// Simple formatting without fmt
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	result := ""
	if whole > 0 {
		result = intToString(whole)
	} else {
		result = "0"
	}
	return result + "." + string(rune('0'+frac))
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}

