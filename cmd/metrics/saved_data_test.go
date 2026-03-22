package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

// setOutputDir points the output package at a temp directory for the duration
// of a test, restoring the original value on cleanup.
func withTempOutputDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	// output.Path() uses an internal dir variable; we override savedDatadogSLOPath
	// by monkey-patching the env var that output.Path reads, if any, or by
	// directly constructing paths using the same helper.
	// The simplest approach: override the path function via the temp dir.
	origPath := os.Getenv("DEVCTL_OUTPUT_DIR")
	os.Setenv("DEVCTL_OUTPUT_DIR", tmp)
	t.Cleanup(func() { os.Setenv("DEVCTL_OUTPUT_DIR", origPath) })
}

func TestDatadogSLODataRoundTrip(t *testing.T) {
	tmp := t.TempDir()

	results := []sloResult{
		{SLOID: "abc1", App: "checkout", Name: "Checkout Availability", Type: "metric", Target: 99.9, Current: 99.95, Budget: 87.5, Violated: false},
		{SLOID: "def2", App: "checkout", Name: "Checkout Latency", Type: "monitor", Target: 99.5, Current: 98.1, Budget: -30.0, Violated: true},
		{SLOID: "ghi3", App: "", Name: "Untagged SLO", Type: "metric", Target: 99.0, Current: 99.0, Budget: 100.0, Violated: false},
	}
	eventCountByID := map[string]int{
		"abc1": 0,
		"def2": 3,
	}

	path := filepath.Join(tmp, "datadog-slo-data-myteam.csv")

	// Save
	if err := saveDatadogSLODataToPath(results, eventCountByID, path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Load
	gotResults, gotCounts, err := loadDatadogSLODataFromPath(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(gotResults) != len(results) {
		t.Fatalf("expected %d results, got %d", len(results), len(gotResults))
	}

	for i, want := range results {
		got := gotResults[i]
		if got.SLOID != want.SLOID {
			t.Errorf("[%d] SLOID: want %q, got %q", i, want.SLOID, got.SLOID)
		}
		if got.App != want.App {
			t.Errorf("[%d] App: want %q, got %q", i, want.App, got.App)
		}
		if got.Name != want.Name {
			t.Errorf("[%d] Name: want %q, got %q", i, want.Name, got.Name)
		}
		if got.Violated != want.Violated {
			t.Errorf("[%d] Violated: want %v, got %v", i, want.Violated, got.Violated)
		}
		if abs(got.Target-want.Target) > 0.001 {
			t.Errorf("[%d] Target: want %v, got %v", i, want.Target, got.Target)
		}
		if abs(got.Current-want.Current) > 0.001 {
			t.Errorf("[%d] Current: want %v, got %v", i, want.Current, got.Current)
		}
	}

	// Verify event counts round-trip.
	for id, want := range eventCountByID {
		if want == 0 {
			continue // zero counts are not stored
		}
		if got := gotCounts[id]; got != want {
			t.Errorf("eventCount[%s]: want %d, got %d", id, want, got)
		}
	}
	// SLOID with zero event count should not appear in the map.
	if _, ok := gotCounts["abc1"]; ok {
		t.Error("expected abc1 (count=0) to be absent from loaded event counts")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
