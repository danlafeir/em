package metrics

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

// MonteCarloConfig configures the simulation.
type MonteCarloConfig struct {
	Trials           int           // Number of simulations (default: 10000)
	ThroughputWindow int           // Days of history to sample from (default: 60)
	SimulationStart  time.Time     // When to start simulation (default: now)
	Deadline         *time.Time    // Optional deadline to check against
}

// ForecastResult holds Monte Carlo simulation results.
type ForecastResult struct {
	TargetItems        int
	RemainingItems     int
	TrialsRun          int
	Percentiles        map[int]time.Time // Percentile -> completion date
	PercentileDays     map[int]int       // Percentile -> days from now
	DeadlineDate       *time.Time
	DeadlineConfidence float64 // Probability of meeting deadline (0-1)
	ThroughputSamples  int     // Number of throughput samples used
	AvgThroughput      float64 // Average weekly throughput
}

// MonteCarloSimulator runs Monte Carlo simulations for forecasting.
type MonteCarloSimulator struct {
	config     MonteCarloConfig
	throughput []int // Historical weekly throughput values
}

// NewMonteCarloSimulator creates a simulator with historical throughput data.
func NewMonteCarloSimulator(config MonteCarloConfig, weeklyThroughput []int) *MonteCarloSimulator {
	return &MonteCarloSimulator{
		config:     config,
		throughput: weeklyThroughput,
	}
}

// DefaultMonteCarloConfig returns sensible defaults.
func DefaultMonteCarloConfig() MonteCarloConfig {
	return MonteCarloConfig{
		Trials:           10000,
		ThroughputWindow: 60,
		SimulationStart:  time.Now(),
	}
}

// Run executes the Monte Carlo simulation.
func (mc *MonteCarloSimulator) Run(remainingItems int) (*ForecastResult, error) {
	if len(mc.throughput) == 0 {
		return nil, fmt.Errorf("no throughput data available for simulation")
	}

	if remainingItems <= 0 {
		return nil, fmt.Errorf("remaining items must be positive, got %d", remainingItems)
	}

	// Filter out zero-throughput weeks for sampling (but keep them for average calculation)
	var nonZeroThroughput []int
	var totalThroughput int
	for _, t := range mc.throughput {
		totalThroughput += t
		if t > 0 {
			nonZeroThroughput = append(nonZeroThroughput, t)
		}
	}

	// If all weeks had zero throughput, we can't forecast
	if len(nonZeroThroughput) == 0 {
		return nil, fmt.Errorf("no non-zero throughput data available for simulation")
	}

	avgThroughput := float64(totalThroughput) / float64(len(mc.throughput))

	// Run simulations
	completionDates := make([]time.Time, mc.config.Trials)

	for trial := 0; trial < mc.config.Trials; trial++ {
		remaining := remainingItems
		currentDate := mc.config.SimulationStart

		// Simulate week by week until all items complete
		for remaining > 0 {
			// Randomly sample a historical throughput value
			weeklyThroughput := nonZeroThroughput[rand.Intn(len(nonZeroThroughput))]
			remaining -= weeklyThroughput
			currentDate = currentDate.AddDate(0, 0, 7)
		}

		completionDates[trial] = currentDate
	}

	// Sort completion dates
	sort.Slice(completionDates, func(i, j int) bool {
		return completionDates[i].Before(completionDates[j])
	})

	// Calculate percentiles
	result := &ForecastResult{
		TargetItems:       remainingItems,
		RemainingItems:    remainingItems,
		TrialsRun:         mc.config.Trials,
		Percentiles:       make(map[int]time.Time),
		PercentileDays:    make(map[int]int),
		ThroughputSamples: len(mc.throughput),
		AvgThroughput:     avgThroughput,
	}

	// Common percentiles: 50th, 70th, 85th, 95th
	for _, p := range []int{50, 70, 85, 95} {
		idx := (p * mc.config.Trials) / 100
		if idx >= len(completionDates) {
			idx = len(completionDates) - 1
		}
		result.Percentiles[p] = completionDates[idx]
		result.PercentileDays[p] = int(completionDates[idx].Sub(mc.config.SimulationStart).Hours() / 24)
	}

	// Check deadline if provided
	if mc.config.Deadline != nil {
		result.DeadlineDate = mc.config.Deadline
		hitCount := 0
		for _, date := range completionDates {
			if !date.After(*mc.config.Deadline) {
				hitCount++
			}
		}
		result.DeadlineConfidence = float64(hitCount) / float64(mc.config.Trials)
	}

	return result, nil
}

// FormatForecast returns a human-readable forecast summary.
func FormatForecast(result *ForecastResult) string {
	var output string

	output += fmt.Sprintf("Monte Carlo Forecast\n")
	output += fmt.Sprintf("====================\n")
	output += fmt.Sprintf("Remaining items: %d\n", result.RemainingItems)
	output += fmt.Sprintf("Simulations run: %d\n", result.TrialsRun)
	output += fmt.Sprintf("Throughput samples: %d weeks\n", result.ThroughputSamples)
	output += fmt.Sprintf("Average throughput: %.1f items/week\n\n", result.AvgThroughput)

	output += fmt.Sprintf("Completion Date Forecast:\n")
	for _, p := range []int{50, 70, 85, 95} {
		date := result.Percentiles[p]
		days := result.PercentileDays[p]
		output += fmt.Sprintf("  %d%% confidence: %s (%d days)\n", p, date.Format("2006-01-02"), days)
	}

	if result.DeadlineDate != nil {
		output += fmt.Sprintf("\nDeadline: %s\n", result.DeadlineDate.Format("2006-01-02"))
		output += fmt.Sprintf("Probability of meeting deadline: %.1f%%\n", result.DeadlineConfidence*100)
	}

	return output
}

// BurnupData represents data for a burn-up chart.
type BurnupData struct {
	Date           time.Time
	Completed      int
	TotalScope     int
	RemainingItems int
}

// ForecastBand represents a forecast confidence band for burn-up charts.
type ForecastBand struct {
	Percentile int
	DataPoints []BurnupData
}

// GenerateBurnupWithForecast creates burn-up data with Monte Carlo forecast bands.
func GenerateBurnupWithForecast(
	completed []BurnupData,
	remainingItems int,
	totalScope int,
	weeklyThroughput []int,
	config MonteCarloConfig,
) ([]BurnupData, []ForecastBand, error) {
	if len(weeklyThroughput) == 0 {
		return completed, nil, nil
	}

	// Filter to non-zero throughput for sampling
	var nonZeroThroughput []int
	for _, t := range weeklyThroughput {
		if t > 0 {
			nonZeroThroughput = append(nonZeroThroughput, t)
		}
	}

	if len(nonZeroThroughput) == 0 {
		return completed, nil, nil
	}

	// Run simulations for each percentile
	percentiles := []int{50, 85, 95}
	bands := make([]ForecastBand, len(percentiles))

	for pi, p := range percentiles {
		bands[pi] = ForecastBand{
			Percentile: p,
			DataPoints: make([]BurnupData, 0),
		}

		// Run trials and collect completion trajectories
		trajectories := make([][]int, config.Trials)

		for trial := 0; trial < config.Trials; trial++ {
			remaining := remainingItems
			completed := totalScope - remainingItems
			trajectory := []int{completed}

			for remaining > 0 {
				throughput := nonZeroThroughput[rand.Intn(len(nonZeroThroughput))]
				remaining -= throughput
				if remaining < 0 {
					remaining = 0
				}
				completed = totalScope - remaining
				trajectory = append(trajectory, completed)
			}

			trajectories[trial] = trajectory
		}

		// Find the percentile trajectory
		// For each week, find the percentile completion value
		maxWeeks := 0
		for _, t := range trajectories {
			if len(t) > maxWeeks {
				maxWeeks = len(t)
			}
		}

		startDate := config.SimulationStart
		lastCompleted := len(completed)
		if lastCompleted > 0 {
			bands[pi].DataPoints = append(bands[pi].DataPoints, completed[lastCompleted-1])
		}

		for week := 1; week < maxWeeks; week++ {
			// Collect values at this week across all trials
			values := make([]int, 0, config.Trials)
			for _, t := range trajectories {
				if week < len(t) {
					values = append(values, t[week])
				} else {
					values = append(values, totalScope) // Completed
				}
			}

			sort.Ints(values)
			idx := (p * len(values)) / 100
			if idx >= len(values) {
				idx = len(values) - 1
			}

			bands[pi].DataPoints = append(bands[pi].DataPoints, BurnupData{
				Date:           startDate.AddDate(0, 0, week*7),
				Completed:      values[idx],
				TotalScope:     totalScope,
				RemainingItems: totalScope - values[idx],
			})

			// Stop if we've reached 100% for this percentile
			if values[idx] >= totalScope {
				break
			}
		}
	}

	return completed, bands, nil
}
