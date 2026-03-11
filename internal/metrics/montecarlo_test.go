package metrics

import (
	"testing"
	"time"
)

func TestMonteCarloSimulator_NoThroughput(t *testing.T) {
	config := DefaultMonteCarloConfig()
	sim := NewMonteCarloSimulator(config, []int{})

	_, err := sim.Run(10)
	if err == nil {
		t.Error("Expected error for empty throughput data")
	}
}

func TestMonteCarloSimulator_ZeroThroughput(t *testing.T) {
	config := DefaultMonteCarloConfig()
	sim := NewMonteCarloSimulator(config, []int{0, 0, 0})

	_, err := sim.Run(10)
	if err == nil {
		t.Error("Expected error for all-zero throughput data")
	}
}

func TestMonteCarloSimulator_ZeroRemaining(t *testing.T) {
	config := DefaultMonteCarloConfig()
	sim := NewMonteCarloSimulator(config, []int{5, 10, 8})

	_, err := sim.Run(0)
	if err == nil {
		t.Error("Expected error for zero remaining items")
	}
}

func TestMonteCarloSimulator_NegativeRemaining(t *testing.T) {
	config := DefaultMonteCarloConfig()
	sim := NewMonteCarloSimulator(config, []int{5, 10, 8})

	_, err := sim.Run(-5)
	if err == nil {
		t.Error("Expected error for negative remaining items")
	}
}

func TestMonteCarloSimulator_BasicRun(t *testing.T) {
	config := MonteCarloConfig{
		Trials:          1000,
		SimulationStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	// Consistent throughput of 5 items/week
	sim := NewMonteCarloSimulator(config, []int{5, 5, 5, 5, 5})

	result, err := sim.Run(20)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.TrialsRun != 1000 {
		t.Errorf("Expected 1000 trials, got %d", result.TrialsRun)
	}
	if result.RemainingItems != 20 {
		t.Errorf("Expected 20 remaining items, got %d", result.RemainingItems)
	}
	if result.ThroughputSamples != 5 {
		t.Errorf("Expected 5 throughput samples, got %d", result.ThroughputSamples)
	}
	if result.AvgThroughput != 5.0 {
		t.Errorf("Expected avg throughput 5.0, got %v", result.AvgThroughput)
	}

	// With 5 items/week, 20 items should take 4 weeks
	// All percentiles should be around 4 weeks (28 days)
	for _, p := range []int{50, 70, 85, 95} {
		days := result.PercentileDays[p]
		if days < 21 || days > 35 {
			t.Errorf("Percentile %d: expected ~28 days, got %d", p, days)
		}
	}
}

func TestMonteCarloSimulator_WithDeadline(t *testing.T) {
	startDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	deadline := time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC) // ~6 weeks

	config := MonteCarloConfig{
		Trials:          1000,
		SimulationStart: startDate,
		Deadline:        &deadline,
	}

	// 5 items/week, 20 items = 4 weeks, well before deadline
	sim := NewMonteCarloSimulator(config, []int{5, 5, 5, 5, 5})

	result, err := sim.Run(20)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.DeadlineDate == nil {
		t.Error("Expected deadline date to be set")
	}

	// Should have high confidence of meeting deadline
	if result.DeadlineConfidence < 0.9 {
		t.Errorf("Expected high deadline confidence, got %v", result.DeadlineConfidence)
	}
}

func TestMonteCarloSimulator_TightDeadline(t *testing.T) {
	startDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	deadline := time.Date(2024, 6, 8, 0, 0, 0, 0, time.UTC) // Only 1 week

	config := MonteCarloConfig{
		Trials:          1000,
		SimulationStart: startDate,
		Deadline:        &deadline,
	}

	// 5 items/week, 20 items = 4 weeks, deadline is only 1 week
	sim := NewMonteCarloSimulator(config, []int{5, 5, 5, 5, 5})

	result, err := sim.Run(20)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have low confidence of meeting deadline
	if result.DeadlineConfidence > 0.1 {
		t.Errorf("Expected low deadline confidence, got %v", result.DeadlineConfidence)
	}
}

func TestMonteCarloSimulator_VariableThroughput(t *testing.T) {
	config := MonteCarloConfig{
		Trials:          10000,
		SimulationStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	// Variable throughput: 2-10 items/week
	sim := NewMonteCarloSimulator(config, []int{2, 5, 10, 3, 8, 4, 7, 6})

	result, err := sim.Run(30)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Percentiles should be in ascending order
	if result.PercentileDays[50] > result.PercentileDays[70] {
		t.Error("50th percentile should be <= 70th percentile")
	}
	if result.PercentileDays[70] > result.PercentileDays[85] {
		t.Error("70th percentile should be <= 85th percentile")
	}
	if result.PercentileDays[85] > result.PercentileDays[95] {
		t.Error("85th percentile should be <= 95th percentile")
	}
}

func TestRunSequential_EmptyThroughput(t *testing.T) {
	sim := NewMonteCarloSimulator(DefaultMonteCarloConfig(), []int{})
	_, err := sim.RunSequential([]int{10, 20})
	if err == nil {
		t.Error("expected error for empty throughput data")
	}
}

func TestRunSequential_EmptyEpics(t *testing.T) {
	sim := NewMonteCarloSimulator(DefaultMonteCarloConfig(), []int{5, 5, 5})
	results, err := sim.RunSequential([]int{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty epics, got %v", results)
	}
}

func TestRunSequential_OrderPreserved(t *testing.T) {
	config := MonteCarloConfig{
		Trials:          1000,
		SimulationStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	sim := NewMonteCarloSimulator(config, []int{5, 5, 5, 5, 5})

	results, err := sim.RunSequential([]int{10, 10, 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Each epic must finish no earlier than the previous one (sequential)
	for i := 1; i < len(results); i++ {
		p50prev := results[i-1].Percentiles[50]
		p50curr := results[i].Percentiles[50]
		if p50curr.Before(p50prev) {
			t.Errorf("epic %d P50 (%v) is before epic %d P50 (%v) — should be sequential",
				i, p50curr, i-1, p50prev)
		}
	}
}

func TestRunSequential_LaterEpicsFinishLater(t *testing.T) {
	config := MonteCarloConfig{
		Trials:          2000,
		SimulationStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	// Consistent throughput so results are predictable
	sim := NewMonteCarloSimulator(config, []int{5, 5, 5, 5, 5})

	results, err := sim.RunSequential([]int{5, 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second epic must finish strictly after first (each needs at least 1 week)
	if !results[1].Percentiles[50].After(results[0].Percentiles[50]) {
		t.Errorf("second epic P50 (%v) should be after first epic P50 (%v)",
			results[1].Percentiles[50], results[0].Percentiles[50])
	}
}

func TestRunSequential_ResultsHaveCorrectRemainingItems(t *testing.T) {
	config := MonteCarloConfig{
		Trials:          500,
		SimulationStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	sim := NewMonteCarloSimulator(config, []int{5, 5, 5})

	items := []int{10, 20, 15}
	results, err := sim.RunSequential(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, r := range results {
		if r.RemainingItems != items[i] {
			t.Errorf("result[%d].RemainingItems = %d, want %d", i, r.RemainingItems, items[i])
		}
		if r.TrialsRun != 500 {
			t.Errorf("result[%d].TrialsRun = %d, want 500", i, r.TrialsRun)
		}
	}
}

func TestFormatForecast(t *testing.T) {
	result := &ForecastResult{
		RemainingItems:    25,
		TrialsRun:         10000,
		ThroughputSamples: 12,
		AvgThroughput:     5.5,
		Percentiles: map[int]time.Time{
			50: time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC),
			70: time.Date(2024, 7, 22, 0, 0, 0, 0, time.UTC),
			85: time.Date(2024, 7, 29, 0, 0, 0, 0, time.UTC),
			95: time.Date(2024, 8, 5, 0, 0, 0, 0, time.UTC),
		},
		PercentileDays: map[int]int{
			50: 30,
			70: 37,
			85: 44,
			95: 51,
		},
	}

	output := FormatForecast(result)

	// Check that key information is present
	if len(output) == 0 {
		t.Error("Expected non-empty output")
	}

	expectedStrings := []string{
		"Monte Carlo Forecast",
		"Remaining items: 25",
		"Simulations run: 10000",
		"50% confidence",
		"95% confidence",
	}

	for _, s := range expectedStrings {
		if !contains(output, s) {
			t.Errorf("Expected output to contain %q", s)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
