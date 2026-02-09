package metrics

import (
	"sort"
	"time"

	"devctl-em/pkg/workflow"
)

// ThroughputPeriod represents throughput for a time period.
type ThroughputPeriod struct {
	PeriodStart time.Time // Start of the period
	PeriodEnd   time.Time // End of the period
	Count       int       // Number of items completed
	Points      float64   // Story points completed
	IssueKeys   []string  // Keys of completed issues
}

// ThroughputResult holds the complete throughput analysis.
type ThroughputResult struct {
	Periods     []ThroughputPeriod
	TotalCount  int
	TotalPoints float64
	AvgCount    float64
	AvgPoints   float64
	Frequency   ThroughputFrequency
}

// ThroughputFrequency defines the aggregation period.
type ThroughputFrequency string

const (
	FrequencyDaily   ThroughputFrequency = "daily"
	FrequencyWeekly  ThroughputFrequency = "weekly"
	FrequencyBiweekly ThroughputFrequency = "biweekly"
	FrequencyMonthly ThroughputFrequency = "monthly"
)

// ThroughputCalculator calculates throughput metrics.
type ThroughputCalculator struct {
	frequency ThroughputFrequency
}

// NewThroughputCalculator creates a new throughput calculator.
func NewThroughputCalculator(frequency ThroughputFrequency) *ThroughputCalculator {
	return &ThroughputCalculator{frequency: frequency}
}

// Calculate computes throughput over time periods.
func (tc *ThroughputCalculator) Calculate(histories []workflow.IssueHistory, from, to time.Time) ThroughputResult {
	// Filter to completed issues within date range
	var completed []workflow.IssueHistory
	for _, h := range histories {
		if h.Completed != nil && !h.Completed.Before(from) && !h.Completed.After(to) {
			completed = append(completed, h)
		}
	}

	// Sort by completion date
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].Completed.Before(*completed[j].Completed)
	})

	// Generate time periods
	periods := tc.generatePeriods(from, to)

	// Assign issues to periods
	for i := range periods {
		for _, h := range completed {
			if !h.Completed.Before(periods[i].PeriodStart) && h.Completed.Before(periods[i].PeriodEnd) {
				periods[i].Count++
				periods[i].Points += h.StoryPoints
				periods[i].IssueKeys = append(periods[i].IssueKeys, h.Key)
			}
		}
	}

	// Calculate totals and averages
	result := ThroughputResult{
		Periods:   periods,
		Frequency: tc.frequency,
	}

	for _, p := range periods {
		result.TotalCount += p.Count
		result.TotalPoints += p.Points
	}

	if len(periods) > 0 {
		result.AvgCount = float64(result.TotalCount) / float64(len(periods))
		result.AvgPoints = result.TotalPoints / float64(len(periods))
	}

	return result
}

// generatePeriods creates time periods based on frequency.
func (tc *ThroughputCalculator) generatePeriods(from, to time.Time) []ThroughputPeriod {
	var periods []ThroughputPeriod

	// Normalize start to beginning of period
	current := tc.normalizeToPeriodStart(from)

	for current.Before(to) {
		periodEnd := tc.addPeriod(current)
		if periodEnd.After(to) {
			periodEnd = to
		}

		periods = append(periods, ThroughputPeriod{
			PeriodStart: current,
			PeriodEnd:   periodEnd,
		})

		current = periodEnd
	}

	return periods
}

// normalizeToPeriodStart adjusts a time to the start of its period.
func (tc *ThroughputCalculator) normalizeToPeriodStart(t time.Time) time.Time {
	switch tc.frequency {
	case FrequencyDaily:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	case FrequencyWeekly:
		// Start week on Monday
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, t.Location())
	case FrequencyBiweekly:
		// Start on Monday, aligned to even weeks
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, t.Location())
		_, week := monday.ISOWeek()
		if week%2 == 1 {
			monday = monday.AddDate(0, 0, -7)
		}
		return monday
	case FrequencyMonthly:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	default:
		return t
	}
}

// addPeriod adds one period to a time.
func (tc *ThroughputCalculator) addPeriod(t time.Time) time.Time {
	switch tc.frequency {
	case FrequencyDaily:
		return t.AddDate(0, 0, 1)
	case FrequencyWeekly:
		return t.AddDate(0, 0, 7)
	case FrequencyBiweekly:
		return t.AddDate(0, 0, 14)
	case FrequencyMonthly:
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 7)
	}
}

// GetWeeklyThroughputValues returns just the count values for Monte Carlo.
func GetWeeklyThroughputValues(result ThroughputResult) []int {
	values := make([]int, len(result.Periods))
	for i, p := range result.Periods {
		values[i] = p.Count
	}
	return values
}

// ThroughputStats calculates statistical summary of throughput.
type ThroughputStats struct {
	Periods      int
	TotalItems   int
	TotalPoints  float64
	AvgItems     float64
	AvgPoints    float64
	MinItems     int
	MaxItems     int
	MedianItems  int
}

// CalculateThroughputStats computes statistics from throughput result.
func CalculateThroughputStats(result ThroughputResult) ThroughputStats {
	if len(result.Periods) == 0 {
		return ThroughputStats{}
	}

	counts := make([]int, len(result.Periods))
	minItems := result.Periods[0].Count
	maxItems := result.Periods[0].Count

	for i, p := range result.Periods {
		counts[i] = p.Count
		if p.Count < minItems {
			minItems = p.Count
		}
		if p.Count > maxItems {
			maxItems = p.Count
		}
	}

	sort.Ints(counts)
	medianItems := counts[len(counts)/2]

	return ThroughputStats{
		Periods:     len(result.Periods),
		TotalItems:  result.TotalCount,
		TotalPoints: result.TotalPoints,
		AvgItems:    result.AvgCount,
		AvgPoints:   result.AvgPoints,
		MinItems:    minItems,
		MaxItems:    maxItems,
		MedianItems: medianItems,
	}
}
