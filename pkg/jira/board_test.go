package jira_test

import (
	"context"
	"testing"

	"github.com/danlafeir/em/pkg/jira"
	"github.com/danlafeir/em/internal/testutil/mockjira"
)

func datasetWithBoards() *mockjira.Dataset {
	ds := mockjira.SmallDataset()
	ds.Boards = []jira.Board{
		{ID: 100, Name: "Team Board", Type: "kanban"},
		{ID: 200, Name: "Sprint Board", Type: "scrum"},
	}
	ds.Filters = map[string]jira.Filter{
		"1000": {ID: "1000", JQL: "project = PROJ ORDER BY rank"},
		"2000": {ID: "2000", JQL: "project = PROJ AND type = Bug"},
	}
	return ds
}

func TestListBoards(t *testing.T) {
	srv := mockjira.New(datasetWithBoards())
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	boards, err := client.ListBoards(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("ListBoards failed: %v", err)
	}

	if len(boards) != 2 {
		t.Fatalf("expected 2 boards, got %d", len(boards))
	}

	if boards[0].Name != "Team Board" {
		t.Errorf("expected first board 'Team Board', got %q", boards[0].Name)
	}
	if boards[0].Type != "kanban" {
		t.Errorf("expected type 'kanban', got %q", boards[0].Type)
	}
	if boards[1].Name != "Sprint Board" {
		t.Errorf("expected second board 'Sprint Board', got %q", boards[1].Name)
	}
}

func TestListBoards_Pagination(t *testing.T) {
	ds := datasetWithBoards()
	// Add more boards to force pagination
	for i := 3; i <= 10; i++ {
		ds.Boards = append(ds.Boards, jira.Board{
			ID:   i * 100,
			Name: "Board " + string(rune('A'+i)),
			Type: "kanban",
		})
	}

	srv := mockjira.New(ds)
	srv.MaxPageSize = 3
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	boards, err := client.ListBoards(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("ListBoards failed: %v", err)
	}

	if len(boards) != 10 {
		t.Fatalf("expected 10 boards, got %d", len(boards))
	}
}

func TestListBoards_Empty(t *testing.T) {
	ds := mockjira.SmallDataset()
	// No boards set

	srv := mockjira.New(ds)
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	boards, err := client.ListBoards(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("ListBoards failed: %v", err)
	}

	if len(boards) != 0 {
		t.Errorf("expected 0 boards, got %d", len(boards))
	}
}

func TestGetBoardConfiguration(t *testing.T) {
	srv := mockjira.New(datasetWithBoards())
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	config, err := client.GetBoardConfiguration(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetBoardConfiguration failed: %v", err)
	}

	// Mock returns filter ID = boardID * 10
	if config.Filter.ID != "1000" {
		t.Errorf("expected filter ID '1000', got %q", config.Filter.ID)
	}
}

func TestGetFilter(t *testing.T) {
	srv := mockjira.New(datasetWithBoards())
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	filter, err := client.GetFilter(context.Background(), "1000")
	if err != nil {
		t.Fatalf("GetFilter failed: %v", err)
	}

	if filter.JQL != "project = PROJ ORDER BY rank" {
		t.Errorf("expected JQL 'project = PROJ ORDER BY rank', got %q", filter.JQL)
	}
}

func TestGetFilter_NotFound(t *testing.T) {
	srv := mockjira.New(datasetWithBoards())
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	_, err := client.GetFilter(context.Background(), "99999")
	if err == nil {
		t.Fatal("expected error for non-existent filter")
	}
}

func TestBoardToFilterJQL_EndToEnd(t *testing.T) {
	srv := mockjira.New(datasetWithBoards())
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	ctx := context.Background()

	// Simulate the full setup flow: list boards → get config → get filter JQL
	boards, err := client.ListBoards(ctx, "PROJ")
	if err != nil {
		t.Fatalf("ListBoards failed: %v", err)
	}

	board := boards[0] // Team Board, ID=100

	config, err := client.GetBoardConfiguration(ctx, board.ID)
	if err != nil {
		t.Fatalf("GetBoardConfiguration failed: %v", err)
	}

	filter, err := client.GetFilter(ctx, config.Filter.ID)
	if err != nil {
		t.Fatalf("GetFilter failed: %v", err)
	}

	if filter.JQL != "project = PROJ ORDER BY rank" {
		t.Errorf("expected board filter JQL, got %q", filter.JQL)
	}
}
