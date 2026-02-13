package metrics

import (
	"sort"
	"time"

	"devctl-em/internal/workflow"
)

// CFDDataPoint represents a single point in the Cumulative Flow Diagram.
type CFDDataPoint struct {
	Date   time.Time
	Stages map[string]int // Stage name -> cumulative count at or beyond this stage
}

// CFDResult holds the complete CFD analysis.
type CFDResult struct {
	DataPoints []CFDDataPoint
	StageNames []string // In workflow order
	DateRange  struct {
		From time.Time
		To   time.Time
	}
}

// CFDCalculator calculates Cumulative Flow Diagram data.
type CFDCalculator struct {
	mapper *workflow.Mapper
}

// NewCFDCalculator creates a new CFD calculator.
func NewCFDCalculator(mapper *workflow.Mapper) *CFDCalculator {
	return &CFDCalculator{mapper: mapper}
}

// Calculate generates CFD data points for the given date range.
func (c *CFDCalculator) Calculate(histories []workflow.IssueHistory, from, to time.Time) CFDResult {
	stageNames := c.mapper.GetStageNames()

	result := CFDResult{
		StageNames: stageNames,
	}
	result.DateRange.From = from
	result.DateRange.To = to

	// Generate daily data points
	current := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	endDate := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, to.Location())

	for !current.After(endDate) {
		dataPoint := CFDDataPoint{
			Date:   current,
			Stages: make(map[string]int),
		}

		// Initialize all stages to 0
		for _, stage := range stageNames {
			dataPoint.Stages[stage] = 0
		}

		// Count issues in each stage at this point in time
		for _, history := range histories {
			// Skip issues not yet created
			if history.Created.After(current) {
				continue
			}

			// Determine which stage the issue was in at this time
			stage := c.getStageAtTime(history, current)
			if stage != "" {
				dataPoint.Stages[stage]++
			}
		}

		// Convert to cumulative (each stage includes all issues at or beyond that stage)
		// This creates the stacked area effect
		cumulativeCount := 0
		for i := len(stageNames) - 1; i >= 0; i-- {
			stageName := stageNames[i]
			cumulativeCount += dataPoint.Stages[stageName]
			dataPoint.Stages[stageName] = cumulativeCount
		}

		result.DataPoints = append(result.DataPoints, dataPoint)
		current = current.AddDate(0, 0, 1)
	}

	return result
}

// getStageAtTime determines which stage an issue was in at a specific time.
func (c *CFDCalculator) getStageAtTime(history workflow.IssueHistory, at time.Time) string {
	if len(history.Transitions) == 0 {
		// No transitions, issue has always been in current stage
		return history.CurrentStage
	}

	// Find the most recent transition before or at the given time
	var currentStage string

	// Start with the stage before the first transition
	if len(history.Transitions) > 0 {
		// The issue started in the "from" stage of the first transition
		// But we need to infer what stage it was created in
		// Typically this is "Backlog" or the first stage
		currentStage = c.mapper.GetStageNames()[0] // Default to first stage
	}

	for _, t := range history.Transitions {
		if t.Timestamp.After(at) {
			break
		}
		currentStage = t.ToStage
	}

	return currentStage
}

// WIPDataPoint represents work-in-progress at a point in time.
type WIPDataPoint struct {
	Date     time.Time
	WIPCount int
	ByStage  map[string]int
}

// WIPItem represents a single item currently in progress.
type WIPItem struct {
	Key         string
	Type        string
	Summary     string
	Stage       string
	Age         time.Duration // Time in current stage
	EnteredDate time.Time
}

// WIPResult holds WIP analysis results.
type WIPResult struct {
	CurrentWIP []WIPItem
	Historical []WIPDataPoint
	ByStage    map[string][]WIPItem
}

// WIPCalculator calculates work-in-progress metrics.
type WIPCalculator struct {
	mapper *workflow.Mapper
}

// NewWIPCalculator creates a new WIP calculator.
func NewWIPCalculator(mapper *workflow.Mapper) *WIPCalculator {
	return &WIPCalculator{mapper: mapper}
}

// Calculate analyzes current and historical WIP.
func (w *WIPCalculator) Calculate(histories []workflow.IssueHistory, from, to time.Time) WIPResult {
	result := WIPResult{
		ByStage: make(map[string][]WIPItem),
	}

	now := time.Now()

	// Analyze current WIP
	for _, h := range histories {
		// Skip completed items
		if h.Completed != nil {
			continue
		}

		// Determine when the issue entered its current stage
		enteredDate := h.Created
		for _, t := range h.Transitions {
			if t.ToStage == h.CurrentStage {
				enteredDate = t.Timestamp
			}
		}

		item := WIPItem{
			Key:         h.Key,
			Type:        h.Type,
			Summary:     h.Summary,
			Stage:       h.CurrentStage,
			Age:         now.Sub(enteredDate),
			EnteredDate: enteredDate,
		}

		result.CurrentWIP = append(result.CurrentWIP, item)
		result.ByStage[h.CurrentStage] = append(result.ByStage[h.CurrentStage], item)
	}

	// Sort current WIP by age (oldest first)
	sort.Slice(result.CurrentWIP, func(i, j int) bool {
		return result.CurrentWIP[i].Age > result.CurrentWIP[j].Age
	})

	// Calculate historical WIP (daily snapshots)
	current := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	endDate := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, to.Location())

	cfdCalc := NewCFDCalculator(w.mapper)

	for !current.After(endDate) {
		wipCount := 0
		byStage := make(map[string]int)

		for _, h := range histories {
			// Skip issues not yet created
			if h.Created.After(current) {
				continue
			}

			// Skip issues already completed
			if h.Completed != nil && h.Completed.Before(current) {
				continue
			}

			// Get stage at this time
			stage := cfdCalc.getStageAtTime(h, current)

			// Only count "in progress" stages (not backlog or done)
			for _, s := range w.mapper.GetStages() {
				if s.Name == stage && s.Category == "in_progress" {
					wipCount++
					byStage[stage]++
					break
				}
			}
		}

		result.Historical = append(result.Historical, WIPDataPoint{
			Date:     current,
			WIPCount: wipCount,
			ByStage:  byStage,
		})

		current = current.AddDate(0, 0, 1)
	}

	return result
}

// AgingThresholds defines thresholds for aging categories.
type AgingThresholds struct {
	Warning  time.Duration // Items older than this are "warning"
	Critical time.Duration // Items older than this are "critical"
}

// DefaultAgingThresholds returns sensible defaults.
func DefaultAgingThresholds() AgingThresholds {
	return AgingThresholds{
		Warning:  7 * 24 * time.Hour,  // 7 days
		Critical: 14 * 24 * time.Hour, // 14 days
	}
}

// CategorizeByAge categorizes WIP items by age.
func CategorizeByAge(items []WIPItem, thresholds AgingThresholds) (healthy, warning, critical []WIPItem) {
	for _, item := range items {
		switch {
		case item.Age >= thresholds.Critical:
			critical = append(critical, item)
		case item.Age >= thresholds.Warning:
			warning = append(warning, item)
		default:
			healthy = append(healthy, item)
		}
	}
	return
}
