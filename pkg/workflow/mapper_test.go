package workflow

import (
	"testing"
	"time"
)

func TestGetStage(t *testing.T) {
	mapper := NewMapper(DefaultConfig())

	tests := []struct {
		status   string
		expected string
	}{
		{"Open", "Backlog"},
		{"To Do", "Backlog"},
		{"In Progress", "In Progress"},
		{"in progress", "In Progress"}, // case insensitive
		{"IN PROGRESS", "In Progress"},
		{"Done", "Done"},
		{"Closed", "Done"},
		{"Code Review", "Review"},
		{"In QA", "Testing"},
		{"Unknown Status", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := mapper.GetStage(tt.status)
			if got != tt.expected {
				t.Errorf("GetStage(%q) = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

func TestIsCompleted(t *testing.T) {
	mapper := NewMapper(DefaultConfig())

	tests := []struct {
		status   string
		expected bool
	}{
		{"Done", true},
		{"Closed", true},
		{"Resolved", true},
		{"In Progress", false},
		{"Open", false},
		{"Unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := mapper.IsCompleted(tt.status)
			if got != tt.expected {
				t.Errorf("IsCompleted(%q) = %v, want %v", tt.status, got, tt.expected)
			}
		})
	}
}

func TestGetStageOrder(t *testing.T) {
	mapper := NewMapper(DefaultConfig())

	tests := []struct {
		stage    string
		expected int
	}{
		{"Backlog", 0},
		{"In Progress", 2},
		{"Done", 5},
		{"NonExistent", -1},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			got := mapper.GetStageOrder(tt.stage)
			if got != tt.expected {
				t.Errorf("GetStageOrder(%q) = %d, want %d", tt.stage, got, tt.expected)
			}
		})
	}
}

func TestTimeInStage_NoTransitions(t *testing.T) {
	mapper := NewMapper(DefaultConfig())

	created := time.Now().Add(-48 * time.Hour)
	completed := time.Now()

	history := IssueHistory{
		Key:          "TEST-1",
		Created:      created,
		Completed:    &completed,
		CurrentStage: "Done",
		Transitions:  nil,
	}

	result := mapper.TimeInStage(history)

	if len(result) != 1 {
		t.Errorf("Expected 1 stage, got %d", len(result))
	}

	if _, ok := result["Done"]; !ok {
		t.Error("Expected 'Done' stage in result")
	}

	expectedDuration := completed.Sub(created)
	if result["Done"] != expectedDuration {
		t.Errorf("Expected duration %v, got %v", expectedDuration, result["Done"])
	}
}

func TestTimeInStage_WithTransitions(t *testing.T) {
	mapper := NewMapper(DefaultConfig())

	created := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	transition1 := time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC) // 1 day in Backlog
	transition2 := time.Date(2024, 1, 5, 9, 0, 0, 0, time.UTC) // 3 days in In Progress
	completed := time.Date(2024, 1, 6, 9, 0, 0, 0, time.UTC)   // 1 day in Done

	history := IssueHistory{
		Key:          "TEST-1",
		Created:      created,
		Completed:    &completed,
		CurrentStage: "Done",
		Transitions: []StageTransition{
			{Timestamp: transition1, FromStage: "Backlog", ToStage: "In Progress"},
			{Timestamp: transition2, FromStage: "In Progress", ToStage: "Done"},
		},
	}

	result := mapper.TimeInStage(history)

	expectedBacklog := 24 * time.Hour
	expectedInProgress := 3 * 24 * time.Hour
	expectedDone := 24 * time.Hour

	if result["Backlog"] != expectedBacklog {
		t.Errorf("Backlog: expected %v, got %v", expectedBacklog, result["Backlog"])
	}
	if result["In Progress"] != expectedInProgress {
		t.Errorf("In Progress: expected %v, got %v", expectedInProgress, result["In Progress"])
	}
	if result["Done"] != expectedDone {
		t.Errorf("Done: expected %v, got %v", expectedDone, result["Done"])
	}
}

func TestGetCycleTimeStages(t *testing.T) {
	mapper := NewMapper(DefaultConfig())

	start, end := mapper.GetCycleTimeStages()

	if start != "In Progress" {
		t.Errorf("Expected start stage 'In Progress', got %q", start)
	}
	if end != "Done" {
		t.Errorf("Expected end stage 'Done', got %q", end)
	}
}

func TestGetStageNames(t *testing.T) {
	mapper := NewMapper(DefaultConfig())

	names := mapper.GetStageNames()

	expected := []string{"Backlog", "Analysis", "In Progress", "Review", "Testing", "Done"}

	if len(names) != len(expected) {
		t.Errorf("Expected %d stages, got %d", len(expected), len(names))
	}

	for i, name := range expected {
		if names[i] != name {
			t.Errorf("Stage %d: expected %q, got %q", i, name, names[i])
		}
	}
}

func TestCustomConfig(t *testing.T) {
	config := Config{
		Stages: []Stage{
			{Name: "Todo", Statuses: []string{"New", "Open"}, Category: "todo", Order: 0},
			{Name: "Working", Statuses: []string{"Active"}, Category: "in_progress", Order: 1},
			{Name: "Complete", Statuses: []string{"Finished"}, Category: "done", Order: 2},
		},
		CycleTime: CycleTimeConfig{
			Started:   "Working",
			Completed: "Complete",
		},
	}

	mapper := NewMapper(config)

	if mapper.GetStage("New") != "Todo" {
		t.Errorf("Expected 'Todo', got %q", mapper.GetStage("New"))
	}
	if mapper.GetStage("Active") != "Working" {
		t.Errorf("Expected 'Working', got %q", mapper.GetStage("Active"))
	}
	if !mapper.IsCompleted("Finished") {
		t.Error("Expected 'Finished' to be completed")
	}

	start, end := mapper.GetCycleTimeStages()
	if start != "Working" || end != "Complete" {
		t.Errorf("Expected Working/Complete, got %s/%s", start, end)
	}
}
