package charts

import (
	"strings"
	"testing"
	"time"

	"github.com/danlafeir/em/pkg/metrics"
)

// -- linearRegression ---------------------------------------------------------

func TestLinearRegression_PerfectFit(t *testing.T) {
	// y = 2x + 1
	xs := []float64{1, 2, 3, 4, 5}
	ys := []float64{3, 5, 7, 9, 11}

	slope, intercept := linearRegression(xs, ys)

	if abs(slope-2.0) > 0.001 {
		t.Errorf("slope = %v, want 2.0", slope)
	}
	if abs(intercept-1.0) > 0.001 {
		t.Errorf("intercept = %v, want 1.0", intercept)
	}
}

func TestLinearRegression_HorizontalLine(t *testing.T) {
	// All y equal → slope 0, intercept = mean(y)
	xs := []float64{1, 2, 3}
	ys := []float64{5, 5, 5}

	slope, intercept := linearRegression(xs, ys)

	if abs(slope) > 0.001 {
		t.Errorf("slope = %v, want 0", slope)
	}
	if abs(intercept-5.0) > 0.001 {
		t.Errorf("intercept = %v, want 5.0", intercept)
	}
}

func TestLinearRegression_VerticalDegenerateCase(t *testing.T) {
	// All x equal → degenerate, returns slope=0, intercept=mean(y)
	xs := []float64{3, 3, 3}
	ys := []float64{1, 2, 3}

	slope, intercept := linearRegression(xs, ys)

	if abs(slope) > 0.001 {
		t.Errorf("slope = %v, want 0 for degenerate case", slope)
	}
	if abs(intercept-2.0) > 0.001 {
		t.Errorf("intercept = %v, want 2.0 (mean of y)", intercept)
	}
}

// -- CycleTimeScatterHTML -----------------------------------------------------

func TestCycleTimeScatterHTML_ReturnsHTML(t *testing.T) {
	base := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	data := []metrics.CycleTimeResult{
		{IssueKey: "PROJ-1", Summary: "Fix bug", EndDate: base, CycleTime: 3 * 24 * time.Hour},
		{IssueKey: "PROJ-2", Summary: "Add feature", EndDate: base.Add(7 * 24 * time.Hour), CycleTime: 5 * 24 * time.Hour},
	}

	html, err := CycleTimeScatterHTML(data, []float64{50, 85, 95}, "My CT Chart")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(html)

	if len(s) == 0 {
		t.Error("expected non-empty HTML")
	}
	if !strings.Contains(s, "My CT Chart") {
		t.Error("expected HTML to contain chart title")
	}
	if !strings.Contains(s, "canvas") {
		t.Error("expected HTML to contain a canvas element")
	}
}

func TestCycleTimeScatterHTML_DefaultTitle(t *testing.T) {
	html, err := CycleTimeScatterHTML(nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(html), "Cycle Time Scatter Plot") {
		t.Error("expected default title in HTML")
	}
}

func TestCycleTimeScatterHTML_EmptyData(t *testing.T) {
	html, err := CycleTimeScatterHTML([]metrics.CycleTimeResult{}, nil, "Empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(string(html)) == 0 {
		t.Error("expected non-empty HTML even with no data")
	}
}

// -- ThroughputLineHTML -------------------------------------------------------

func TestThroughputLineHTML_ReturnsHTML(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	data := metrics.ThroughputResult{
		Periods: []metrics.ThroughputPeriod{
			{PeriodStart: base, PeriodEnd: base.Add(7 * 24 * time.Hour), Count: 5},
			{PeriodStart: base.Add(7 * 24 * time.Hour), PeriodEnd: base.Add(14 * 24 * time.Hour), Count: 8},
		},
		Frequency: metrics.FrequencyWeekly,
	}

	html, err := ThroughputLineHTML(data, "Weekly Throughput")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(html)

	if !strings.Contains(s, "Weekly Throughput") {
		t.Error("expected HTML to contain chart title")
	}
	if !strings.Contains(s, "canvas") {
		t.Error("expected HTML to contain a canvas element")
	}
}

func TestThroughputLineHTML_DefaultTitle(t *testing.T) {
	html, err := ThroughputLineHTML(metrics.ThroughputResult{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(html), "Throughput Over Time") {
		t.Error("expected default title in HTML")
	}
}

// -- LongestCycleTimeTableHTML ------------------------------------------------

func TestLongestCycleTimeTableHTML_RendersRows(t *testing.T) {
	rows := []LongestCycleTimeRow{
		{Key: "PROJ-10", Summary: "Long running task", Days: "12.5", Started: "Jan 01", Completed: "Jan 13", Outlier: false},
		{Key: "PROJ-11", Summary: "Outlier item", Days: "45.0", Started: "Nov 01", Completed: "Dec 16", Outlier: true},
	}

	html, err := LongestCycleTimeTableHTML(rows, "Top Cycle Times", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(html)

	if !strings.Contains(s, "Top Cycle Times") {
		t.Error("expected title in HTML")
	}
	if !strings.Contains(s, "PROJ-10") {
		t.Error("expected PROJ-10 in HTML")
	}
	if !strings.Contains(s, "Long running task") {
		t.Error("expected summary in HTML")
	}
	if !strings.Contains(s, "12.5") {
		t.Error("expected days value in HTML")
	}
}

func TestLongestCycleTimeTableHTML_WithJIRALinks(t *testing.T) {
	rows := []LongestCycleTimeRow{
		{Key: "PROJ-42", Summary: "Task with link", Days: "5.0", Started: "Jan 01", Completed: "Jan 06"},
	}

	html, err := LongestCycleTimeTableHTML(rows, "", "https://acme.atlassian.net")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(html)

	if !strings.Contains(s, `href="https://acme.atlassian.net/browse/PROJ-42"`) {
		t.Errorf("expected JIRA link in HTML, got:\n%s", s)
	}
	if !strings.Contains(s, `target="_blank"`) {
		t.Error("expected target=_blank on link")
	}
}

func TestLongestCycleTimeTableHTML_NoLinksWhenNoBaseURL(t *testing.T) {
	rows := []LongestCycleTimeRow{
		{Key: "PROJ-1", Summary: "No link", Days: "3.0", Started: "Jan 01", Completed: "Jan 04"},
	}

	html, err := LongestCycleTimeTableHTML(rows, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(html), "<a ") {
		t.Error("expected no anchor tags when jiraBaseURL is empty")
	}
}

func TestLongestCycleTimeTableHTML_DefaultTitle(t *testing.T) {
	html, err := LongestCycleTimeTableHTML(nil, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(html), "Longest Cycle Times") {
		t.Error("expected default title")
	}
}

// -- ForecastTableHTML --------------------------------------------------------

func TestForecastTableHTML_RendersRows(t *testing.T) {
	rows := []ForecastRow{
		{
			EpicKey:    "EPIC-1",
			Summary:    "Big feature",
			Completed:  3,
			Total:      10,
			Remaining:  7,
			Forecast50: "Apr 15",
			Forecast85: "Apr 29",
			Forecast95: "May 06",
		},
	}

	html, err := ForecastTableHTML(rows, "Epic Forecast", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(html)

	if !strings.Contains(s, "Epic Forecast") {
		t.Error("expected title in HTML")
	}
	if !strings.Contains(s, "EPIC-1") {
		t.Error("expected epic key in HTML")
	}
	if !strings.Contains(s, "Big feature") {
		t.Error("expected summary in HTML")
	}
	if !strings.Contains(s, "Apr 15") {
		t.Error("expected P50 forecast date in HTML")
	}
	if !strings.Contains(s, "3/10") {
		t.Error("expected progress label in HTML")
	}
}

func TestForecastTableHTML_WithJIRALinks(t *testing.T) {
	rows := []ForecastRow{
		{EpicKey: "EPIC-99", Summary: "Linked epic", Completed: 1, Total: 5, Remaining: 4,
			Forecast50: "May 01", Forecast85: "May 15", Forecast95: "May 22"},
	}

	html, err := ForecastTableHTML(rows, "", "https://acme.atlassian.net")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(html)

	if !strings.Contains(s, `href="https://acme.atlassian.net/browse/EPIC-99"`) {
		t.Errorf("expected JIRA link, got:\n%s", s)
	}
}

func TestForecastTableHTML_DefaultTitle(t *testing.T) {
	html, err := ForecastTableHTML(nil, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(html), "Epic Forecast") {
		t.Error("expected default title")
	}
}

// -- helpers ------------------------------------------------------------------

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
