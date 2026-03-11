package metrics

import (
	"testing"
	"time"

	"devctl-em/internal/workflow"
)

func TestPercentile(t *testing.T) {
	// Note: percentile uses idx = (p * len) / 100, so:
	// For 10 elements: 50th -> idx 5, 95th -> idx 9
	tests := []struct {
		name     string
		sorted   []time.Duration
		p        int
		expected time.Duration
	}{
		{
			name:     "empty slice",
			sorted:   []time.Duration{},
			p:        50,
			expected: 0,
		},
		{
			name:     "single element",
			sorted:   []time.Duration{5 * time.Hour},
			p:        50,
			expected: 5 * time.Hour,
		},
		{
			name:     "50th percentile of 10 elements",
			sorted:   []time.Duration{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        50,
			expected: 6, // idx = (50 * 10) / 100 = 5, sorted[5] = 6
		},
		{
			name:     "95th percentile of 10 elements",
			sorted:   []time.Duration{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        95,
			expected: 10, // idx = (95 * 10) / 100 = 9, sorted[9] = 10
		},
		{
			name:     "100th percentile clamps to max",
			sorted:   []time.Duration{1, 2, 3, 4, 5},
			p:        100,
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := percentile(tt.sorted, tt.p)
			if got != tt.expected {
				t.Errorf("percentile(%v, %d) = %v, want %v", tt.sorted, tt.p, got, tt.expected)
			}
		})
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
		delta    float64
	}{
		{0, 0, 0.001},
		{1, 1, 0.001},
		{4, 2, 0.001},
		{9, 3, 0.001},
		{2, 1.414, 0.01},
		{-1, 0, 0.001}, // negative returns 0
	}

	for _, tt := range tests {
		got := sqrt(tt.input)
		if got < tt.expected-tt.delta || got > tt.expected+tt.delta {
			t.Errorf("sqrt(%v) = %v, want %v (±%v)", tt.input, got, tt.expected, tt.delta)
		}
	}
}

func TestCalculateStats_Empty(t *testing.T) {
	stats := CalculateStats([]CycleTimeResult{})

	if stats.Count != 0 {
		t.Errorf("Expected Count=0, got %d", stats.Count)
	}
	if stats.Mean != 0 {
		t.Errorf("Expected Mean=0, got %v", stats.Mean)
	}
}

func TestCalculateStats(t *testing.T) {
	// Create results with known cycle times: 1, 2, 3, 4, 5 days
	results := make([]CycleTimeResult, 5)
	for i := 0; i < 5; i++ {
		results[i] = CycleTimeResult{
			IssueKey:  "TEST-" + string(rune('1'+i)),
			CycleTime: time.Duration(i+1) * 24 * time.Hour,
		}
	}

	stats := CalculateStats(results)

	if stats.Count != 5 {
		t.Errorf("Expected Count=5, got %d", stats.Count)
	}

	// Mean of 1,2,3,4,5 = 3 days
	expectedMean := 3 * 24 * time.Hour
	if stats.Mean != expectedMean {
		t.Errorf("Expected Mean=%v, got %v", expectedMean, stats.Mean)
	}

	// Min = 1 day
	expectedMin := 1 * 24 * time.Hour
	if stats.Min != expectedMin {
		t.Errorf("Expected Min=%v, got %v", expectedMin, stats.Min)
	}

	// Max = 5 days
	expectedMax := 5 * 24 * time.Hour
	if stats.Max != expectedMax {
		t.Errorf("Expected Max=%v, got %v", expectedMax, stats.Max)
	}

	// Median (50th percentile): idx = (50 * 5) / 100 = 2, sorted[2] = 3 days
	expectedMedian := 3 * 24 * time.Hour
	if stats.Median != expectedMedian {
		t.Errorf("Expected Median=%v, got %v", expectedMedian, stats.Median)
	}
}

func TestCalculate_CompletedIssues(t *testing.T) {
	mapper := workflow.NewMapper(workflow.DefaultConfig())
	calc := NewCycleTimeCalculator(mapper)

	base := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	completed := base.Add(5 * 24 * time.Hour)

	histories := []workflow.IssueHistory{
		{
			Key:          "TEST-1",
			Type:         "Story",
			Summary:      "Test story",
			Created:      base,
			Completed:    &completed,
			CurrentStage: "Done",
			Transitions: []workflow.StageTransition{
				{Timestamp: base.Add(1 * time.Hour), FromStage: "Backlog", ToStage: "In Progress"},
				{Timestamp: completed, FromStage: "In Progress", ToStage: "Done"},
			},
		},
	}

	results := calc.Calculate(histories)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.IssueKey != "TEST-1" {
		t.Errorf("expected key TEST-1, got %q", r.IssueKey)
	}

	// Cycle time: from "In Progress" to "Done" = 5 days - 1 hour
	expectedCycleTime := completed.Sub(base.Add(1 * time.Hour))
	if r.CycleTime != expectedCycleTime {
		t.Errorf("expected cycle time %v, got %v", expectedCycleTime, r.CycleTime)
	}
}

func TestCalculate_SkipsIncompleteIssues(t *testing.T) {
	mapper := workflow.NewMapper(workflow.DefaultConfig())
	calc := NewCycleTimeCalculator(mapper)

	base := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)

	histories := []workflow.IssueHistory{
		{
			Key:          "TEST-1",
			Type:         "Story",
			Summary:      "WIP story",
			Created:      base,
			Completed:    nil, // not completed
			CurrentStage: "In Progress",
			Transitions: []workflow.StageTransition{
				{Timestamp: base.Add(1 * time.Hour), FromStage: "Backlog", ToStage: "In Progress"},
			},
		},
	}

	results := calc.Calculate(histories)

	if len(results) != 0 {
		t.Errorf("expected 0 results for incomplete issue, got %d", len(results))
	}
}

func TestCalculate_ExcludesIssuesThatSkipInProgress(t *testing.T) {
	mapper := workflow.NewMapper(workflow.DefaultConfig())
	calc := NewCycleTimeCalculator(mapper)

	base := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	completed := base.Add(3 * 24 * time.Hour)

	// Issue goes directly to Done without ever entering In Progress —
	// cycle time clock never started, so it should be excluded.
	histories := []workflow.IssueHistory{
		{
			Key:          "TEST-1",
			Type:         "Story",
			Summary:      "Direct to done",
			Created:      base,
			Completed:    &completed,
			CurrentStage: "Done",
			Transitions: []workflow.StageTransition{
				{Timestamp: completed, FromStage: "Backlog", ToStage: "Done"},
			},
		},
	}

	results := calc.Calculate(histories)

	if len(results) != 0 {
		t.Fatalf("expected 0 results (card never entered In Progress), got %d", len(results))
	}
}

func TestCalculate_SortsByEndDate(t *testing.T) {
	mapper := workflow.NewMapper(workflow.DefaultConfig())
	calc := NewCycleTimeCalculator(mapper)

	base := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	completed1 := base.Add(10 * 24 * time.Hour) // finishes later
	completed2 := base.Add(3 * 24 * time.Hour)  // finishes earlier

	histories := []workflow.IssueHistory{
		{
			Key: "TEST-1", Type: "Story", Summary: "Late finish",
			Created: base, Completed: &completed1, CurrentStage: "Done",
			Transitions: []workflow.StageTransition{
				{Timestamp: base.Add(1 * time.Hour), FromStage: "Backlog", ToStage: "In Progress"},
				{Timestamp: completed1, FromStage: "In Progress", ToStage: "Done"},
			},
		},
		{
			Key: "TEST-2", Type: "Story", Summary: "Early finish",
			Created: base, Completed: &completed2, CurrentStage: "Done",
			Transitions: []workflow.StageTransition{
				{Timestamp: base.Add(2 * time.Hour), FromStage: "Backlog", ToStage: "In Progress"},
				{Timestamp: completed2, FromStage: "In Progress", ToStage: "Done"},
			},
		},
	}

	results := calc.Calculate(histories)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Should be sorted by end date: TEST-2 first
	if results[0].IssueKey != "TEST-2" {
		t.Errorf("expected TEST-2 first (earlier end date), got %q", results[0].IssueKey)
	}
	if results[1].IssueKey != "TEST-1" {
		t.Errorf("expected TEST-1 second (later end date), got %q", results[1].IssueKey)
	}
}

func TestCycleTimeResult_CycleTimeDays(t *testing.T) {
	result := CycleTimeResult{
		CycleTime: 48 * time.Hour,
	}

	days := result.CycleTimeDays()
	if days != 2.0 {
		t.Errorf("Expected 2.0 days, got %v", days)
	}
}

func TestCycleTimeStats_ToDays(t *testing.T) {
	stats := CycleTimeStats{
		Count:        10,
		Mean:         48 * time.Hour,
		Median:       24 * time.Hour,
		Percentile50: 24 * time.Hour,
		Percentile70: 36 * time.Hour,
		Percentile85: 48 * time.Hour,
		Percentile95: 72 * time.Hour,
		Min:          12 * time.Hour,
		Max:          96 * time.Hour,
		StdDev:       24 * time.Hour,
	}

	days := stats.ToDays()

	if days.Count != 10 {
		t.Errorf("Expected Count=10, got %d", days.Count)
	}
	if days.Mean != 2.0 {
		t.Errorf("Expected Mean=2.0, got %v", days.Mean)
	}
	if days.Median != 1.0 {
		t.Errorf("Expected Median=1.0, got %v", days.Median)
	}
	if days.Min != 0.5 {
		t.Errorf("Expected Min=0.5, got %v", days.Min)
	}
	if days.Max != 4.0 {
		t.Errorf("Expected Max=4.0, got %v", days.Max)
	}
}
