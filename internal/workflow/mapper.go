// Package workflow provides workflow stage mapping for JIRA statuses.
package workflow

import (
	"strings"
	"time"

	"em/internal/jira"
)

// Stage represents a workflow stage that groups multiple JIRA statuses.
type Stage struct {
	Name     string   // Stage name (e.g., "In Progress", "Done")
	Statuses []string // JIRA status names that map to this stage
	Category string   // Category: "todo", "in_progress", "done"
	Order    int      // Position in workflow (0 = earliest, higher = later)
}

// CycleTimeConfig defines which stages mark the start and end of cycle time.
type CycleTimeConfig struct {
	Started   string // Stage name where cycle time starts
	Completed string // Stage name where cycle time ends
}

// Config holds the complete workflow configuration.
type Config struct {
	Stages    []Stage
	CycleTime CycleTimeConfig
}

// Mapper maps JIRA statuses to workflow stages.
type Mapper struct {
	config      Config
	statusToStage map[string]string // lowercase status -> stage name
	stageOrder    map[string]int    // stage name -> order
}

// NewMapper creates a new workflow mapper from configuration.
func NewMapper(config Config) *Mapper {
	m := &Mapper{
		config:        config,
		statusToStage: make(map[string]string),
		stageOrder:    make(map[string]int),
	}

	for _, stage := range config.Stages {
		m.stageOrder[stage.Name] = stage.Order
		for _, status := range stage.Statuses {
			m.statusToStage[strings.ToLower(status)] = stage.Name
		}
	}

	return m
}

// DefaultConfig returns a sensible default workflow configuration.
func DefaultConfig() Config {
	return Config{
		Stages: []Stage{
			{Name: "In Progress", Statuses: []string{"In Progress", "In Development", "Doing", "Active"}, Category: "in_progress", Order: 0},
			{Name: "Closed", Statuses: []string{"Closed", "Done", "Resolved", "Complete", "Released"}, Category: "done", Order: 1},
		},
		CycleTime: CycleTimeConfig{
			Started:   "In Progress",
			Completed: "Closed",
		},
	}
}

// GetStage returns the stage name for a JIRA status.
func (m *Mapper) GetStage(status string) string {
	if stage, ok := m.statusToStage[strings.ToLower(status)]; ok {
		return stage
	}
	return "Unknown"
}

// GetStageOrder returns the order of a stage (lower = earlier in workflow).
func (m *Mapper) GetStageOrder(stageName string) int {
	if order, ok := m.stageOrder[stageName]; ok {
		return order
	}
	return -1
}

// IsCompleted returns true if the status maps to the "done" category.
func (m *Mapper) IsCompleted(status string) bool {
	stageName := m.GetStage(status)
	for _, stage := range m.config.Stages {
		if stage.Name == stageName {
			return stage.Category == "done"
		}
	}
	return false
}

// StageTransition represents a transition between workflow stages.
type StageTransition struct {
	Timestamp time.Time
	FromStage string
	ToStage   string
}

// IssueHistory represents an issue with its workflow history.
type IssueHistory struct {
	Key          string
	Type         string
	Summary      string
	Created      time.Time
	Completed    *time.Time
	CurrentStage string
	Transitions  []StageTransition
	// Raw JIRA fields preserved for export
	Assignee string
	Priority string
	Labels   []string
	EpicKey  string
}

// MapIssueHistory converts a JIRA issue with status transitions to workflow stage history.
func (m *Mapper) MapIssueHistory(issue jira.IssueWithHistory) IssueHistory {
	history := IssueHistory{
		Key:          issue.Key,
		Type:         issue.Fields.IssueType.Name,
		Summary:      issue.Fields.Summary,
		Created:      issue.Fields.Created.Time,
		CurrentStage: m.GetStage(issue.Fields.Status.Name),
		Labels:       issue.Fields.Labels,
	}

	if issue.Fields.Assignee != nil {
		history.Assignee = issue.Fields.Assignee.DisplayName
	}
	if issue.Fields.Priority != nil {
		history.Priority = issue.Fields.Priority.Name
	}
	if issue.Fields.Parent != nil {
		history.EpicKey = issue.Fields.Parent.Key
	} else if issue.Fields.Epic != nil {
		history.EpicKey = issue.Fields.Epic.Key
	}

	if issue.Fields.ResolutionDate != nil && !issue.Fields.ResolutionDate.Time.IsZero() {
		t := issue.Fields.ResolutionDate.Time
		history.Completed = &t
	}

	// Map status transitions to stage transitions
	for _, t := range issue.Transitions {
		fromStage := m.GetStage(t.FromStatus)
		toStage := m.GetStage(t.ToStatus)

		// Only record if stage actually changed
		if fromStage != toStage {
			history.Transitions = append(history.Transitions, StageTransition{
				Timestamp: t.Timestamp,
				FromStage: fromStage,
				ToStage:   toStage,
			})
		}
	}

	return history
}

// TimeInStage calculates how long an issue spent in each stage.
func (m *Mapper) TimeInStage(history IssueHistory) map[string]time.Duration {
	result := make(map[string]time.Duration)

	if len(history.Transitions) == 0 {
		// No transitions, all time in current stage
		duration := time.Since(history.Created)
		if history.Completed != nil {
			duration = history.Completed.Sub(history.Created)
		}
		result[history.CurrentStage] = duration
		return result
	}

	// Calculate time from creation to first transition
	firstTransition := history.Transitions[0]
	initialStage := firstTransition.FromStage
	result[initialStage] = firstTransition.Timestamp.Sub(history.Created)

	// Calculate time between transitions
	for i := 0; i < len(history.Transitions); i++ {
		t := history.Transitions[i]
		var endTime time.Time

		if i+1 < len(history.Transitions) {
			endTime = history.Transitions[i+1].Timestamp
		} else if history.Completed != nil {
			endTime = *history.Completed
		} else {
			endTime = time.Now()
		}

		duration := endTime.Sub(t.Timestamp)
		result[t.ToStage] += duration
	}

	return result
}

// GetCycleTimeStages returns the start and end stages for cycle time calculation.
func (m *Mapper) GetCycleTimeStages() (start, end string) {
	return m.config.CycleTime.Started, m.config.CycleTime.Completed
}

// GetStages returns all configured stages in order.
func (m *Mapper) GetStages() []Stage {
	return m.config.Stages
}

// GetStageNames returns stage names in workflow order.
func (m *Mapper) GetStageNames() []string {
	names := make([]string, len(m.config.Stages))
	for i, s := range m.config.Stages {
		names[i] = s.Name
	}
	return names
}
