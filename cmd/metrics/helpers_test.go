package metrics

import (
	"testing"
	"time"

	"github.com/danlafeir/em/pkg/metrics"
	"github.com/danlafeir/em/pkg/snyk"
)

// --- parseDateRange ---

func TestParseDateRange_Defaults(t *testing.T) {
	from, to, err := parseDateRange("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// default from ≈ now-42d
	expected := time.Now().AddDate(0, 0, -42)
	if diff := from.Sub(expected); diff < 0 {
		diff = -diff
	} else if diff > time.Second {
		t.Errorf("default from %v not within 1s of now-42d (%v)", from, expected)
	}
	if !to.After(from) {
		t.Errorf("expected to > from, got from=%v to=%v", from, to)
	}
}

func TestParseDateRange_Explicit(t *testing.T) {
	from, to, err := parseDateRange("2025-01-15", "2025-03-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := from.Format("2006-01-02"); got != "2025-01-15" {
		t.Errorf("from: want 2025-01-15, got %s", got)
	}
	if got := to.Format("2006-01-02"); got != "2025-03-31" {
		t.Errorf("to: want 2025-03-31, got %s", got)
	}
}

func TestParseDateRange_InvalidFrom(t *testing.T) {
	if _, _, err := parseDateRange("not-a-date", ""); err == nil {
		t.Fatal("expected error for invalid --from")
	}
}

func TestParseDateRange_InvalidTo(t *testing.T) {
	if _, _, err := parseDateRange("", "not-a-date"); err == nil {
		t.Fatal("expected error for invalid --to")
	}
}

// --- jqlWithDateRange / splitOrderBy ---

func TestJQLWithDateRange(t *testing.T) {
	tests := []struct {
		name string
		jql  string
		want string
	}{
		{
			name: "simple",
			jql:  "project = PROJ",
			want: "resolved >= 2025-01-01 AND resolved <= 2025-03-31 AND (project = PROJ)",
		},
		{
			name: "preserves ORDER BY",
			jql:  "project = PROJ ORDER BY created DESC",
			want: "resolved >= 2025-01-01 AND resolved <= 2025-03-31 AND (project = PROJ) ORDER BY created DESC",
		},
		{
			name: "compound JQL",
			jql:  "project = PROJ AND issuetype = Story",
			want: "resolved >= 2025-01-01 AND resolved <= 2025-03-31 AND (project = PROJ AND issuetype = Story)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := jqlWithDateRange(tc.jql, "2025-01-01", "2025-03-31")
			if got != tc.want {
				t.Errorf("\nwant %q\ngot  %q", tc.want, got)
			}
		})
	}
}

func TestSplitOrderBy(t *testing.T) {
	tests := []struct {
		input   string
		filter  string
		orderBy string
	}{
		{"project = PROJ", "project = PROJ", ""},
		{"project = PROJ ORDER BY created DESC", "project = PROJ", "ORDER BY created DESC"},
		{"project = PROJ order by created", "project = PROJ", "order by created"},
		{"ORDER BY created", "", "ORDER BY created"},
	}
	for _, tc := range tests {
		filter, orderBy := splitOrderBy(tc.input)
		if filter != tc.filter {
			t.Errorf("input=%q: filter want %q got %q", tc.input, tc.filter, filter)
		}
		if orderBy != tc.orderBy {
			t.Errorf("input=%q: orderBy want %q got %q", tc.input, tc.orderBy, orderBy)
		}
	}
}

// --- wrapString ---

func TestWrapString_Short(t *testing.T) {
	got := wrapString("short", 20)
	if len(got) != 1 || got[0] != "short" {
		t.Errorf("expected [\"short\"], got %v", got)
	}
}

func TestWrapString_LinesWithinWidth(t *testing.T) {
	lines := wrapString("the quick brown fox jumps over the lazy dog", 10)
	for _, l := range lines {
		if len(l) > 10 {
			t.Errorf("line %q exceeds maxWidth 10", l)
		}
	}
}

func TestWrapString_PreservesWords(t *testing.T) {
	input := "the quick brown fox"
	lines := wrapString(input, 10)
	joined := ""
	for i, l := range lines {
		if i > 0 {
			joined += " "
		}
		joined += l
	}
	if joined != input {
		t.Errorf("joined %q != original %q", joined, input)
	}
}

// --- parseThroughputFrequency ---

func TestParseThroughputFrequency(t *testing.T) {
	tests := []struct {
		flag string
		want metrics.ThroughputFrequency
	}{
		{"daily", metrics.FrequencyDaily},
		{"weekly", metrics.FrequencyWeekly},
		{"biweekly", metrics.FrequencyBiweekly},
		{"monthly", metrics.FrequencyMonthly},
		{"", metrics.FrequencyWeekly},   // default
		{"bogus", metrics.FrequencyWeekly}, // unknown → weekly
	}
	for _, tc := range tests {
		got := parseThroughputFrequency(tc.flag)
		if got != tc.want {
			t.Errorf("parseThroughputFrequency(%q) = %v, want %v", tc.flag, got, tc.want)
		}
	}
}

// --- extractSLOApp ---

func TestExtractSLOApp(t *testing.T) {
	tests := []struct {
		tags []string
		want string
	}{
		{[]string{"service:checkout", "team:platform"}, "checkout"},
		{[]string{"team:platform", "app:payments"}, "payments"},
		{[]string{"application:auth"}, "auth"},
		{[]string{"team:platform"}, ""},
		{nil, ""},
	}
	for _, tc := range tests {
		got := extractSLOApp(tc.tags)
		if got != tc.want {
			t.Errorf("extractSLOApp(%v) = %q, want %q", tc.tags, got, tc.want)
		}
	}
}

// --- groupSLOsByApp ---

func TestGroupSLOsByApp(t *testing.T) {
	results := []sloResult{
		{App: "checkout", Name: "A"},
		{App: "checkout", Name: "B"},
		{App: "payments", Name: "C"},
		{App: "", Name: "D"},
	}
	grouped := groupSLOsByApp(results)
	if len(grouped["checkout"]) != 2 {
		t.Errorf("checkout: want 2, got %d", len(grouped["checkout"]))
	}
	if len(grouped["payments"]) != 1 {
		t.Errorf("payments: want 1, got %d", len(grouped["payments"]))
	}
	if len(grouped[""]) != 1 {
		t.Errorf("untagged: want 1, got %d", len(grouped[""]))
	}
}

// --- countBySeverity ---

func TestCountBySeverity(t *testing.T) {
	issues := []snyk.Issue{
		{Severity: "critical"},
		{Severity: "critical"},
		{Severity: "high"},
		{Severity: "Medium"}, // case-insensitive
		{Severity: "low"},
		{Severity: "unknown"}, // ignored
	}
	got := countBySeverity(issues)
	if got["critical"] != 2 {
		t.Errorf("critical: want 2, got %d", got["critical"])
	}
	if got["high"] != 1 {
		t.Errorf("high: want 1, got %d", got["high"])
	}
	if got["medium"] != 1 {
		t.Errorf("medium: want 1, got %d", got["medium"])
	}
	if got["low"] != 1 {
		t.Errorf("low: want 1, got %d", got["low"])
	}
}

// --- truncateStr ---

func TestTruncateStr(t *testing.T) {
	if got := truncateStr("short", 10); got != "short" {
		t.Errorf("want %q, got %q", "short", got)
	}
	got := truncateStr("this is a very long string", 10)
	if len(got) != 10 {
		t.Errorf("want len 10, got %d (%q)", len(got), got)
	}
	if got[7:] != "..." {
		t.Errorf("want trailing ..., got %q", got)
	}
}
