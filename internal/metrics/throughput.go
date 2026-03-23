package metrics

import (
	"sort"
	"time"

	"em/internal/workflow"
)

// ThroughputPeriod represents throughput for a time period.
type ThroughputPeriod struct {
	PeriodStart time.Time // Start of the period
	PeriodEnd   time.Time // End of the period
	Count     int      // Number of items completed
	IssueKeys []string // Keys of completed issues
}

// ThroughputResult holds the complete throughput analysis.
type ThroughputResult struct {
	Periods     []ThroughputPeriod
	TotalCount int
	AvgCount   float64
	Frequency  ThroughputFrequency
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
	frequency  ThroughputFrequency
	startStage string // only count issues that entered this stage; empty = no filter
}

// NewThroughputCalculator creates a new throughput calculator.
// Pass a workflow.Mapper to apply the same In Progress filter used by cycle time:
// issues that never entered the start stage are excluded from the count.
func NewThroughputCalculator(frequency ThroughputFrequency, mapper ...*workflow.Mapper) *ThroughputCalculator {
	tc := &ThroughputCalculator{frequency: frequency}
	if len(mapper) > 0 && mapper[0] != nil {
		tc.startStage, _ = mapper[0].GetCycleTimeStages()
	}
	return tc
}

// Calculate computes throughput over time periods.
func (tc *ThroughputCalculator) Calculate(histories []workflow.IssueHistory, from, to time.Time) ThroughputResult {
	// Filter to completed issues within date range that entered the start stage.
	var completed []workflow.IssueHistory
	for _, h := range histories {
		if h.Completed == nil || h.Completed.Before(from) || h.Completed.After(to) {
			continue
		}
		if tc.startStage != "" && !hasTransitionTo(h, tc.startStage) {
			continue
		}
		completed = append(completed, h)
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
	}

	if len(periods) > 0 {
		result.AvgCount = float64(result.TotalCount) / float64(len(periods))
	}

	return result
}

// hasTransitionTo reports whether the issue ever transitioned into the given stage.
func hasTransitionTo(h workflow.IssueHistory, stage string) bool {
	for _, t := range h.Transitions {
		if t.ToStage == stage {
			return true
		}
	}
	return false
}

// generatePeriods creates time periods based on frequency.
// For weekly/biweekly, periods are anchored to `to` and built backwards so the
// last bucket always ends on the execution date. For daily/monthly the periods
// are built forward from `from` as before.
func (tc *ThroughputCalculator) generatePeriods(from, to time.Time) []ThroughputPeriod {
	switch tc.frequency {
	case FrequencyWeekly, FrequencyBiweekly:
		return tc.generatePeriodsFromEnd(from, to)
	}

	// Daily / monthly: build forward from from.
	var periods []ThroughputPeriod
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

// generatePeriodsFromEnd builds fixed-width buckets anchored to `to`, working
// backwards so that the final bucket always ends exactly on the execution date.
func (tc *ThroughputCalculator) generatePeriodsFromEnd(from, to time.Time) []ThroughputPeriod {
	days := 7
	if tc.frequency == FrequencyBiweekly {
		days = 14
	}

	// Truncate to/from to midnight.
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	stop := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())

	var periods []ThroughputPeriod
	for end.After(stop) {
		start := end.AddDate(0, 0, -days)
		if start.Before(stop) {
			start = stop
		}
		periods = append(periods, ThroughputPeriod{PeriodStart: start, PeriodEnd: end})
		end = start
	}

	// Reverse to chronological order.
	for i, j := 0, len(periods)-1; i < j; i, j = i+1, j-1 {
		periods[i], periods[j] = periods[j], periods[i]
	}
	return periods
}

// normalizeToPeriodStart adjusts a time to the start of its period (daily/monthly only).
func (tc *ThroughputCalculator) normalizeToPeriodStart(t time.Time) time.Time {
	switch tc.frequency {
	case FrequencyDaily:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
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

// WeekStart returns the Monday of the ISO week containing t (at midnight).
// Use this to normalize a date range start before building JQL and calling
// Calculate, so the first bucket is always a full week.
func WeekStart(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, t.Location())
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
	Periods     int
	TotalItems  int
	AvgItems    float64
	MinItems    int
	MaxItems    int
	MedianItems int
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
		AvgItems:    result.AvgCount,
		MinItems:    minItems,
		MaxItems:    maxItems,
		MedianItems: medianItems,
	}
}
