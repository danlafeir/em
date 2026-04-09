package metrics

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// MonteCarloConfig configures the simulation.
type MonteCarloConfig struct {
	Trials           int        // Number of simulations (default: 10000)
	ThroughputWindow int        // Days of history to sample from (default: 60)
	SimulationStart  time.Time  // When to start simulation (default: now)
	Deadline         *time.Time // Optional deadline to check against
	WorkThreads      int        // Number of issues the team works on in parallel; multiplies sampled weekly throughput (default: 1)
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

	workers := mc.config.WorkThreads
	if workers <= 0 {
		workers = 1
	}

	// Run simulations
	completionDates := make([]time.Time, mc.config.Trials)

	for trial := 0; trial < mc.config.Trials; trial++ {
		remaining := remainingItems
		currentDate := mc.config.SimulationStart

		// Simulate week by week until all items complete
		for remaining > 0 {
			// Randomly sample a historical throughput value, scaled by workers
			weeklyThroughput := nonZeroThroughput[rand.Intn(len(nonZeroThroughput))] * workers
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

// RunSequential runs Monte Carlo simulations for a prioritized list of epics,
// treating work as sequential: each epic starts only after all prior epics complete.
// Returns one ForecastResult per epic in the same order as remainingItems.
func (mc *MonteCarloSimulator) RunSequential(remainingItems []int) ([]*ForecastResult, error) {
	if len(mc.throughput) == 0 {
		return nil, fmt.Errorf("no throughput data available for simulation")
	}
	if len(remainingItems) == 0 {
		return nil, nil
	}

	var nonZeroThroughput []int
	var totalThroughput int
	for _, t := range mc.throughput {
		totalThroughput += t
		if t > 0 {
			nonZeroThroughput = append(nonZeroThroughput, t)
		}
	}
	if len(nonZeroThroughput) == 0 {
		return nil, fmt.Errorf("no non-zero throughput data available")
	}
	avgThroughput := float64(totalThroughput) / float64(len(mc.throughput))

	n := len(remainingItems)
	completionDates := make([][]time.Time, n)
	for i := range completionDates {
		completionDates[i] = make([]time.Time, mc.config.Trials)
	}

	workers := mc.config.WorkThreads
	if workers <= 0 {
		workers = 1
	}

	for trial := 0; trial < mc.config.Trials; trial++ {
		currentDate := mc.config.SimulationStart
		for i, items := range remainingItems {
			remaining := items
			for remaining > 0 {
				w := nonZeroThroughput[rand.Intn(len(nonZeroThroughput))] * workers
				remaining -= w
				currentDate = currentDate.AddDate(0, 0, 7)
			}
			completionDates[i][trial] = currentDate
		}
	}

	results := make([]*ForecastResult, n)
	for i, items := range remainingItems {
		dates := make([]time.Time, mc.config.Trials)
		copy(dates, completionDates[i])
		sort.Slice(dates, func(a, b int) bool {
			return dates[a].Before(dates[b])
		})
		r := &ForecastResult{
			TargetItems:       items,
			RemainingItems:    items,
			TrialsRun:         mc.config.Trials,
			Percentiles:       make(map[int]time.Time),
			PercentileDays:    make(map[int]int),
			ThroughputSamples: len(mc.throughput),
			AvgThroughput:     avgThroughput,
		}
		for _, p := range []int{50, 70, 85, 95} {
			idx := (p * mc.config.Trials) / 100
			if idx >= len(dates) {
				idx = len(dates) - 1
			}
			r.Percentiles[p] = dates[idx]
			r.PercentileDays[p] = int(dates[idx].Sub(mc.config.SimulationStart).Hours() / 24)
		}
		results[i] = r
	}
	return results, nil
}

// WorkThreadsForPercentile maps a confidence percentile to a work thread count given a total.
// Higher percentiles (more conservative) use fewer work threads to model less parallelism.
//
//	50th  → totalWorkThreads  (fully parallel — optimistic)
//	70th  → ¾ × threads       (floor)
//	85th  → ½ × threads       (ceiling)
//	95th  → 1                 (fully sequential — pessimistic)
func WorkThreadsForPercentile(totalWorkThreads, percentile int) int {
	if totalWorkThreads <= 1 {
		return 1
	}
	switch {
	case percentile >= 95:
		return 1
	case percentile >= 85:
		return max(1, (totalWorkThreads+1)/2) // ceiling of half
	case percentile >= 70:
		return max(1, (totalWorkThreads*3)/4) // floor of three-quarters
	default:
		return totalWorkThreads
	}
}

// RunMultiPercentile runs a separate simulation per confidence percentile, each with
// a work thread count determined by WorkThreadsForPercentile. The median (p50) of each
// per-thread simulation becomes that percentile's completion date.
//
// When totalWorkThreads <= 1, falls back to the standard Run (percentiles from one distribution).
func (mc *MonteCarloSimulator) RunMultiPercentile(remaining, totalWorkThreads int) (*ForecastResult, error) {
	if totalWorkThreads <= 1 {
		return mc.Run(remaining)
	}

	var totalTP int
	for _, t := range mc.throughput {
		totalTP += t
	}
	result := &ForecastResult{
		TargetItems:       remaining,
		RemainingItems:    remaining,
		TrialsRun:         mc.config.Trials,
		Percentiles:       make(map[int]time.Time),
		PercentileDays:    make(map[int]int),
		ThroughputSamples: len(mc.throughput),
		AvgThroughput:     float64(totalTP) / float64(len(mc.throughput)),
	}

	cache := make(map[int]*ForecastResult)
	for _, p := range []int{50, 70, 85, 95} {
		w := WorkThreadsForPercentile(totalWorkThreads, p)
		r, ok := cache[w]
		if !ok {
			cfg := mc.config
			cfg.WorkThreads = w
			var err error
			r, err = NewMonteCarloSimulator(cfg, mc.throughput).Run(remaining)
			if err != nil {
				return nil, err
			}
			cache[w] = r
		}
		result.Percentiles[p] = r.Percentiles[50]
		result.PercentileDays[p] = r.PercentileDays[50]
	}

	if mc.config.Deadline != nil {
		result.DeadlineDate = mc.config.Deadline
		if r, ok := cache[totalWorkThreads]; ok {
			result.DeadlineConfidence = r.DeadlineConfidence
		}
	}

	return result, nil
}

// RunSequentialMultiPercentile is the sequential-epics equivalent of RunMultiPercentile.
// Each percentile uses the work thread count from WorkThreadsForPercentile; runs are cached by
// thread count to avoid redundant simulations.
//
// When totalWorkThreads <= 1, falls back to RunSequential.
func (mc *MonteCarloSimulator) RunSequentialMultiPercentile(remainingItems []int, totalWorkThreads int) ([]*ForecastResult, error) {
	if totalWorkThreads <= 1 {
		return mc.RunSequential(remainingItems)
	}
	if len(remainingItems) == 0 {
		return nil, nil
	}

	var totalTP int
	for _, t := range mc.throughput {
		totalTP += t
	}
	avgTP := float64(totalTP) / float64(len(mc.throughput))

	n := len(remainingItems)
	results := make([]*ForecastResult, n)
	for i, items := range remainingItems {
		results[i] = &ForecastResult{
			TargetItems:       items,
			RemainingItems:    items,
			TrialsRun:         mc.config.Trials,
			Percentiles:       make(map[int]time.Time),
			PercentileDays:    make(map[int]int),
			ThroughputSamples: len(mc.throughput),
			AvgThroughput:     avgTP,
		}
	}

	cache := make(map[int][]*ForecastResult)
	for _, p := range []int{50, 70, 85, 95} {
		w := WorkThreadsForPercentile(totalWorkThreads, p)
		cached, ok := cache[w]
		if !ok {
			cfg := mc.config
			cfg.WorkThreads = w
			var err error
			cached, err = NewMonteCarloSimulator(cfg, mc.throughput).RunSequential(remainingItems)
			if err != nil {
				return nil, err
			}
			cache[w] = cached
		}
		for i, r := range cached {
			results[i].Percentiles[p] = r.Percentiles[50]
			results[i].PercentileDays[p] = r.PercentileDays[50]
		}
	}

	return results, nil
}

// FormatForecast returns a human-readable forecast summary.
func FormatForecast(result *ForecastResult) string {
	var b strings.Builder

	b.WriteString("Monte Carlo Forecast\n")
	b.WriteString("====================\n")
	fmt.Fprintf(&b, "Remaining items: %d\n", result.RemainingItems)
	fmt.Fprintf(&b, "Simulations run: %d\n", result.TrialsRun)
	fmt.Fprintf(&b, "Throughput samples: %d weeks\n", result.ThroughputSamples)
	fmt.Fprintf(&b, "Average throughput: %.1f items/week\n\n", result.AvgThroughput)

	b.WriteString("Completion Date Forecast:\n")
	for _, p := range []int{50, 70, 85, 95} {
		date := result.Percentiles[p]
		days := result.PercentileDays[p]
		fmt.Fprintf(&b, "  %d%% confidence: %s (%d days)\n", p, date.Format("2006-01-02"), days)
	}

	if result.DeadlineDate != nil {
		fmt.Fprintf(&b, "\nDeadline: %s\n", result.DeadlineDate.Format("2006-01-02"))
		fmt.Fprintf(&b, "Probability of meeting deadline: %.1f%%\n", result.DeadlineConfidence*100)
	}

	return b.String()
}

