package metrics

import (
	"testing"
	"time"

	"devctl-em/pkg/workflow"
)

func TestNormalizeToPeriodStart_Daily(t *testing.T) {
	tc := NewThroughputCalculator(FrequencyDaily)

	input := time.Date(2024, 6, 15, 14, 30, 45, 0, time.UTC)
	expected := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	got := tc.normalizeToPeriodStart(input)
	if !got.Equal(expected) {
		t.Errorf("normalizeToPeriodStart(daily) = %v, want %v", got, expected)
	}
}

func TestNormalizeToPeriodStart_Weekly(t *testing.T) {
	tc := NewThroughputCalculator(FrequencyWeekly)

	// Wednesday June 19, 2024
	input := time.Date(2024, 6, 19, 14, 30, 0, 0, time.UTC)
	// Should normalize to Monday June 17, 2024
	expected := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)

	got := tc.normalizeToPeriodStart(input)
	if !got.Equal(expected) {
		t.Errorf("normalizeToPeriodStart(weekly) = %v, want %v", got, expected)
	}
}

func TestNormalizeToPeriodStart_Monthly(t *testing.T) {
	tc := NewThroughputCalculator(FrequencyMonthly)

	input := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	expected := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	got := tc.normalizeToPeriodStart(input)
	if !got.Equal(expected) {
		t.Errorf("normalizeToPeriodStart(monthly) = %v, want %v", got, expected)
	}
}

func TestAddPeriod(t *testing.T) {
	tests := []struct {
		frequency ThroughputFrequency
		input     time.Time
		expected  time.Time
	}{
		{
			FrequencyDaily,
			time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			FrequencyWeekly,
			time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 6, 22, 0, 0, 0, 0, time.UTC),
		},
		{
			FrequencyBiweekly,
			time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 6, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			FrequencyMonthly,
			time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.frequency), func(t *testing.T) {
			tc := NewThroughputCalculator(tt.frequency)
			got := tc.addPeriod(tt.input)
			if !got.Equal(tt.expected) {
				t.Errorf("addPeriod(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGeneratePeriods(t *testing.T) {
	tc := NewThroughputCalculator(FrequencyWeekly)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	periods := tc.generatePeriods(from, to)

	// Should have 3 complete weeks
	if len(periods) != 3 {
		t.Errorf("Expected 3 periods, got %d", len(periods))
	}

	// Each period should be 7 days
	for i, p := range periods {
		duration := p.PeriodEnd.Sub(p.PeriodStart)
		if duration != 7*24*time.Hour {
			t.Errorf("Period %d has duration %v, expected 7 days", i, duration)
		}
	}
}

func TestCalculateThroughput(t *testing.T) {
	tc := NewThroughputCalculator(FrequencyWeekly)

	// Create some completed issues
	completed1 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	completed2 := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	completed3 := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)

	histories := []workflow.IssueHistory{
		{Key: "TEST-1", Completed: &completed1, StoryPoints: 3},
		{Key: "TEST-2", Completed: &completed2, StoryPoints: 5},
		{Key: "TEST-3", Completed: &completed3, StoryPoints: 2},
		{Key: "TEST-4", Completed: nil}, // Not completed, should be excluded
	}

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	result := tc.Calculate(histories, from, to)

	if result.TotalCount != 3 {
		t.Errorf("Expected TotalCount=3, got %d", result.TotalCount)
	}
	if result.TotalPoints != 10 {
		t.Errorf("Expected TotalPoints=10, got %v", result.TotalPoints)
	}
	if result.Frequency != FrequencyWeekly {
		t.Errorf("Expected frequency=weekly, got %v", result.Frequency)
	}
}

func TestCalculateThroughputStats(t *testing.T) {
	result := ThroughputResult{
		Periods: []ThroughputPeriod{
			{Count: 2, Points: 5},
			{Count: 4, Points: 10},
			{Count: 3, Points: 8},
			{Count: 1, Points: 3},
			{Count: 5, Points: 12},
		},
		TotalCount:  15,
		TotalPoints: 38,
		AvgCount:    3,
		AvgPoints:   7.6,
	}

	stats := CalculateThroughputStats(result)

	if stats.Periods != 5 {
		t.Errorf("Expected Periods=5, got %d", stats.Periods)
	}
	if stats.TotalItems != 15 {
		t.Errorf("Expected TotalItems=15, got %d", stats.TotalItems)
	}
	if stats.MinItems != 1 {
		t.Errorf("Expected MinItems=1, got %d", stats.MinItems)
	}
	if stats.MaxItems != 5 {
		t.Errorf("Expected MaxItems=5, got %d", stats.MaxItems)
	}
	if stats.MedianItems != 3 {
		t.Errorf("Expected MedianItems=3, got %d", stats.MedianItems)
	}
}

func TestCalculateThroughputStats_Empty(t *testing.T) {
	result := ThroughputResult{Periods: []ThroughputPeriod{}}
	stats := CalculateThroughputStats(result)

	if stats.Periods != 0 {
		t.Errorf("Expected Periods=0, got %d", stats.Periods)
	}
}

func TestGetWeeklyThroughputValues(t *testing.T) {
	result := ThroughputResult{
		Periods: []ThroughputPeriod{
			{Count: 2},
			{Count: 4},
			{Count: 3},
		},
	}

	values := GetWeeklyThroughputValues(result)

	expected := []int{2, 4, 3}
	if len(values) != len(expected) {
		t.Errorf("Expected %d values, got %d", len(expected), len(values))
	}
	for i, v := range expected {
		if values[i] != v {
			t.Errorf("values[%d] = %d, want %d", i, values[i], v)
		}
	}
}
