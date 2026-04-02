// Package metrics provides agile metrics calculations.
package metrics

import (
	"sort"
	"time"

	"em/internal/workflow"
)

// CycleTimeResult holds cycle time calculation for a single issue.
type CycleTimeResult struct {
	IssueKey     string
	IssueType    string
	Summary      string
	CycleTime    time.Duration
	StartDate    time.Time
	EndDate      time.Time
	StageDetails map[string]time.Duration // Time spent in each stage
	// Raw JIRA fields
	Assignee string
	Priority string
	Labels   []string
	EpicKey  string
}

// CycleTimeStats holds statistical summary of cycle times.
type CycleTimeStats struct {
	Count       int
	Mean        time.Duration
	Median      time.Duration
	Percentile50 time.Duration
	Percentile70 time.Duration
	Percentile85 time.Duration
	Percentile95 time.Duration
	Min         time.Duration
	Max         time.Duration
	StdDev      time.Duration
}

// CycleTimeCalculator calculates cycle time metrics.
type CycleTimeCalculator struct {
	mapper     *workflow.Mapper
	startStage string
	endStage   string
}

// NewCycleTimeCalculator creates a new cycle time calculator.
func NewCycleTimeCalculator(mapper *workflow.Mapper) *CycleTimeCalculator {
	start, end := mapper.GetCycleTimeStages()
	return &CycleTimeCalculator{
		mapper:     mapper,
		startStage: start,
		endStage:   end,
	}
}

// Calculate computes cycle time for each completed issue.
func (c *CycleTimeCalculator) Calculate(histories []workflow.IssueHistory) []CycleTimeResult {
	var results []CycleTimeResult

	for _, history := range histories {
		// Only calculate for completed issues
		if history.Completed == nil {
			continue
		}

		result := c.calculateForIssue(history)
		if result != nil {
			results = append(results, *result)
		}
	}

	// Sort by end date
	sort.Slice(results, func(i, j int) bool {
		return results[i].EndDate.Before(results[j].EndDate)
	})

	return results
}

// calculateForIssue computes cycle time for a single issue.
func (c *CycleTimeCalculator) calculateForIssue(history workflow.IssueHistory) *CycleTimeResult {
	var startTime, endTime time.Time
	foundStart := false
	foundEnd := false

	// Find when issue entered start stage and end stage
	for _, t := range history.Transitions {
		if !foundStart && t.ToStage == c.startStage {
			startTime = t.Timestamp
			foundStart = true
		}
		if t.ToStage == c.endStage {
			endTime = t.Timestamp
			foundEnd = true
		}
	}

	// If the card never moved to In Progress, exclude it — we only measure
	// time from when work actually started, not from creation/backlog entry.
	if !foundStart {
		return nil
	}

	// If we never found end stage, use completion time
	if !foundEnd && history.Completed != nil {
		endTime = *history.Completed
		foundEnd = true
	}

	if !foundEnd {
		return nil
	}

	// Skip issues where end is before start (backdated or migrated timestamps)
	if !endTime.After(startTime) {
		return nil
	}

	// Calculate time in each stage
	stageDetails := c.mapper.TimeInStage(history)

	return &CycleTimeResult{
		IssueKey:     history.Key,
		IssueType:    history.Type,
		Summary:      history.Summary,
		CycleTime:    endTime.Sub(startTime),
		StartDate:    startTime,
		EndDate:      endTime,
		StageDetails: stageDetails,
		Assignee:     history.Assignee,
		Priority:     history.Priority,
		Labels:       history.Labels,
		EpicKey:      history.EpicKey,
	}
}

// CalculateStats computes statistical summary of cycle times.
func CalculateStats(results []CycleTimeResult) CycleTimeStats {
	if len(results) == 0 {
		return CycleTimeStats{}
	}

	// Extract cycle times as durations
	durations := make([]time.Duration, len(results))
	for i, r := range results {
		durations[i] = r.CycleTime
	}

	// Sort for percentile calculations
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Calculate mean
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	mean := total / time.Duration(len(durations))

	// Calculate standard deviation
	var sumSquares float64
	for _, d := range durations {
		diff := float64(d - mean)
		sumSquares += diff * diff
	}
	stdDev := time.Duration(0)
	if len(durations) > 1 {
		variance := sumSquares / float64(len(durations)-1)
		stdDev = time.Duration(sqrt(variance))
	}

	return CycleTimeStats{
		Count:        len(results),
		Mean:         mean,
		Median:       percentile(durations, 50),
		Percentile50: percentile(durations, 50),
		Percentile70: percentile(durations, 70),
		Percentile85: percentile(durations, 85),
		Percentile95: percentile(durations, 95),
		Min:          durations[0],
		Max:          durations[len(durations)-1],
		StdDev:       stdDev,
	}
}

// percentile calculates the nth percentile of sorted durations.
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// sqrt calculates square root (avoiding math import for simple case).
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// CycleTimeDays returns cycle time in days (float64).
func (r CycleTimeResult) CycleTimeDays() float64 {
	return r.CycleTime.Hours() / 24
}

// StatsDays returns stats in days for easier reading.
type StatsDays struct {
	Count        int
	Mean         float64
	Median       float64
	Percentile50 float64
	Percentile70 float64
	Percentile85 float64
	Percentile95 float64
	Min          float64
	Max          float64
	StdDev       float64
}

// ToDays converts CycleTimeStats to StatsDays.
func (s CycleTimeStats) ToDays() StatsDays {
	return StatsDays{
		Count:        s.Count,
		Mean:         s.Mean.Hours() / 24,
		Median:       s.Median.Hours() / 24,
		Percentile50: s.Percentile50.Hours() / 24,
		Percentile70: s.Percentile70.Hours() / 24,
		Percentile85: s.Percentile85.Hours() / 24,
		Percentile95: s.Percentile95.Hours() / 24,
		Min:          s.Min.Hours() / 24,
		Max:          s.Max.Hours() / 24,
		StdDev:       s.StdDev.Hours() / 24,
	}
}
