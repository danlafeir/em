// Package charts provides chart generation for metrics visualization.
package charts

import (
	"image/color"
	"os"
	"path/filepath"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"

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

// addPercentileLine adds a horizontal percentile line to the plot.
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
	p.Legend.Add(label+": "+formatDays(value), line)
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

// CFDStackedArea creates a stacked area chart for Cumulative Flow Diagram.
func CFDStackedArea(data metrics.CFDResult, cfg Config) (*plot.Plot, error) {
	p := plot.New()
	p.Title.Text = cfg.Title
	if p.Title.Text == "" {
		p.Title.Text = "Cumulative Flow Diagram"
	}
	p.X.Label.Text = "Date"
	p.Y.Label.Text = "Issue Count"
	p.X.Padding = vg.Points(20)
	p.Y.Padding = vg.Points(20)


	// Define colors for stages (from done to backlog)
	stageColors := []color.Color{
		color.RGBA{R: 76, G: 175, B: 80, A: 200},  // Done - Green
		color.RGBA{R: 139, G: 195, B: 74, A: 200}, // Testing - Light Green
		color.RGBA{R: 255, G: 193, B: 7, A: 200},  // Review - Amber
		color.RGBA{R: 255, G: 152, B: 0, A: 200},  // In Progress - Orange
		color.RGBA{R: 33, G: 150, B: 243, A: 200}, // Analysis - Blue
		color.RGBA{R: 158, G: 158, B: 158, A: 200}, // Backlog - Gray
	}

	// Create stacked areas for each stage (in reverse order for proper stacking)
	for i := len(data.StageNames) - 1; i >= 0; i-- {
		stageName := data.StageNames[i]

		pts := make(plotter.XYs, len(data.DataPoints))
		for j, dp := range data.DataPoints {
			pts[j].X = float64(dp.Date.Unix())
			pts[j].Y = float64(dp.Stages[stageName])
		}

		line, err := plotter.NewLine(pts)
		if err != nil {
			continue
		}

		colorIdx := i
		if colorIdx >= len(stageColors) {
			colorIdx = len(stageColors) - 1
		}
		line.LineStyle.Color = stageColors[colorIdx]
		line.LineStyle.Width = vg.Points(2)
		line.FillColor = stageColors[colorIdx]

		p.Add(line)
		p.Legend.Add(stageName, line)
	}

	p.Legend.Top = true
	p.X.Tick.Marker = dateTicker{}

	return p, nil
}

// BurnupChart creates a burn-up chart with scope and completed lines.
func BurnupChart(completed, scope []plotter.XY, forecastBands []ForecastBand, cfg Config) (*plot.Plot, error) {
	p := plot.New()
	p.Title.Text = cfg.Title
	if p.Title.Text == "" {
		p.Title.Text = "Burn-up Chart"
	}
	p.X.Label.Text = "Date"
	p.Y.Label.Text = "Items"
	p.X.Padding = vg.Points(20)
	p.Y.Padding = vg.Points(20)


	// Scope line
	scopeLine, err := plotter.NewLine(plotter.XYs(scope))
	if err != nil {
		return nil, err
	}
	scopeLine.LineStyle.Color = color.RGBA{R: 158, G: 158, B: 158, A: 255}
	scopeLine.LineStyle.Width = vg.Points(2)
	p.Add(scopeLine)
	p.Legend.Add("Scope", scopeLine)

	// Completed line
	completedLine, err := plotter.NewLine(plotter.XYs(completed))
	if err != nil {
		return nil, err
	}
	completedLine.LineStyle.Color = color.RGBA{R: 66, G: 133, B: 244, A: 255}
	completedLine.LineStyle.Width = vg.Points(2)
	p.Add(completedLine)
	p.Legend.Add("Completed", completedLine)

	// Forecast bands
	bandColors := map[int]color.Color{
		50: color.RGBA{R: 76, G: 175, B: 80, A: 100},
		85: color.RGBA{R: 255, G: 193, B: 7, A: 100},
		95: color.RGBA{R: 244, G: 67, B: 54, A: 100},
	}

	for _, band := range forecastBands {
		pts := make(plotter.XYs, len(band.Points))
		for i, pt := range band.Points {
			pts[i] = pt
		}

		line, err := plotter.NewLine(pts)
		if err != nil {
			continue
		}

		if c, ok := bandColors[band.Percentile]; ok {
			line.LineStyle.Color = c
		}
		line.LineStyle.Width = vg.Points(1.5)
		line.LineStyle.Dashes = []vg.Length{vg.Points(5), vg.Points(3)}

		p.Add(line)
		p.Legend.Add(formatPercentile(band.Percentile), line)
	}

	p.Legend.Top = true
	p.X.Tick.Marker = dateTicker{}

	return p, nil
}

// ForecastBand represents a forecast confidence band.
type ForecastBand struct {
	Percentile int
	Points     []plotter.XY
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

func formatPercentile(p int) string {
	return intToString(p) + "% forecast"
}
