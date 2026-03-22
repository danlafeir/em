package datadog

import (
	"encoding/json"
	"testing"
	"time"
)

// ---- sloErrorBudget ----

func TestSLOErrorBudget_Float(t *testing.T) {
	var b sloErrorBudget
	if err := json.Unmarshal([]byte(`99.5`), &b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if float64(b) != 99.5 {
		t.Errorf("expected 99.5, got %v", b)
	}
}

func TestSLOErrorBudget_Object(t *testing.T) {
	var b sloErrorBudget
	if err := json.Unmarshal([]byte(`{"remaining":42.0}`), &b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if float64(b) != 42.0 {
		t.Errorf("expected 42.0, got %v", b)
	}
}

func TestSLOErrorBudget_Invalid(t *testing.T) {
	var b sloErrorBudget
	// Should not error — falls back to zero.
	if err := json.Unmarshal([]byte(`"unexpected string"`), &b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if float64(b) != 0 {
		t.Errorf("expected 0 fallback, got %v", b)
	}
}

// ---- eventV2Timestamp ----

func TestEventV2Timestamp_Integer(t *testing.T) {
	var ts eventV2Timestamp
	if err := json.Unmarshal([]byte(`1700000000`), &ts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Unix(1700000000, 0).UTC()
	if !ts.Equal(want) {
		t.Errorf("expected %v, got %v", want, ts.Time)
	}
}

func TestEventV2Timestamp_QuotedInteger(t *testing.T) {
	var ts eventV2Timestamp
	if err := json.Unmarshal([]byte(`"1700000000"`), &ts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Unix(1700000000, 0).UTC()
	if !ts.Equal(want) {
		t.Errorf("expected %v, got %v", want, ts.Time)
	}
}

func TestEventV2Timestamp_RFC3339(t *testing.T) {
	var ts eventV2Timestamp
	if err := json.Unmarshal([]byte(`"2023-11-14T22:13:20Z"`), &ts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC)
	if !ts.Equal(want) {
		t.Errorf("expected %v, got %v", want, ts.Time)
	}
}

// ---- tag helpers ----

func TestSLOIDFromEventTags(t *testing.T) {
	cases := []struct {
		tags []string
		want string
	}{
		{[]string{"slo_id:abc123", "team:foo"}, "abc123"},
		{[]string{"team:foo"}, ""},
		{nil, ""},
		{[]string{"slo_id:"}, ""},
	}
	for _, c := range cases {
		if got := sloIDFromEventTags(c.tags); got != c.want {
			t.Errorf("tags %v: expected %q, got %q", c.tags, c.want, got)
		}
	}
}

func TestMonitorIDFromEventTags(t *testing.T) {
	cases := []struct {
		tags []string
		want int64
	}{
		{[]string{"monitor_id:12345", "team:foo"}, 12345},
		{[]string{"team:foo"}, 0},
		{nil, 0},
		{[]string{"monitor_id:notanumber"}, 0},
	}
	for _, c := range cases {
		if got := monitorIDFromEventTags(c.tags); got != c.want {
			t.Errorf("tags %v: expected %d, got %d", c.tags, c.want, got)
		}
	}
}
