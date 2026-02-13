package metrics

import (
	"testing"
	"time"
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
