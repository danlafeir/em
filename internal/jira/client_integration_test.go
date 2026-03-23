package jira_test

import (
	"context"
	"testing"

	"em/internal/testutil/mockjira"
	"em/internal/jira"
)

func TestCredentials_BaseURL_Override(t *testing.T) {
	creds := jira.Credentials{
		Domain:          "mycompany",
		Email:           "user@example.com",
		APIToken:        "secret",
		BaseURLOverride: "http://localhost:9999",
	}

	if got := creds.BaseURL(); got != "http://localhost:9999" {
		t.Errorf("BaseURL() = %q, want %q", got, "http://localhost:9999")
	}

	// Without override, falls back to domain
	creds.BaseURLOverride = ""
	if got := creds.BaseURL(); got != "https://mycompany.atlassian.net" {
		t.Errorf("BaseURL() = %q, want %q", got, "https://mycompany.atlassian.net")
	}
}

func TestTestConnection(t *testing.T) {
	srv := mockjira.New(mockjira.SmallDataset())
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestFetchIssuesWithHistory_SmallDataset(t *testing.T) {
	srv := mockjira.New(mockjira.SmallDataset())
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	issues, err := client.FetchIssuesWithHistory(context.Background(), "project = PROJ", nil)
	if err != nil {
		t.Fatalf("FetchIssuesWithHistory failed: %v", err)
	}

	if len(issues) != 5 {
		t.Fatalf("expected 5 issues, got %d", len(issues))
	}

	// PROJ-1: Open → In Progress → Done (2 transitions)
	if len(issues[0].Transitions) != 2 {
		t.Errorf("PROJ-1: expected 2 transitions, got %d", len(issues[0].Transitions))
	}
	if issues[0].Fields.Status.Name != "Done" {
		t.Errorf("PROJ-1: expected status Done, got %q", issues[0].Fields.Status.Name)
	}

	// PROJ-3: Bug with regression (4 transitions)
	if len(issues[2].Transitions) != 4 {
		t.Errorf("PROJ-3: expected 4 transitions, got %d", len(issues[2].Transitions))
	}
	if issues[2].Fields.IssueType.Name != "Bug" {
		t.Errorf("PROJ-3: expected type Bug, got %q", issues[2].Fields.IssueType.Name)
	}

	// PROJ-4: WIP (unresolved)
	if issues[3].Fields.ResolutionDate != nil {
		t.Error("PROJ-4: expected no resolution date (WIP)")
	}
	if issues[3].Fields.Status.StatusCategory.Key != "indeterminate" {
		t.Errorf("PROJ-4: expected indeterminate status category, got %q", issues[3].Fields.Status.StatusCategory.Key)
	}

	// PROJ-5: resolved
	if issues[4].Fields.ResolutionDate == nil {
		t.Error("PROJ-5: expected resolution date")
	}
}

func TestSearchPagination(t *testing.T) {
	srv := mockjira.New(mockjira.PaginationDataset())
	srv.MaxPageSize = 3
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	issues, err := client.FetchIssuesWithHistory(context.Background(), "project = PROJ", nil)
	if err != nil {
		t.Fatalf("FetchIssuesWithHistory failed: %v", err)
	}

	// PaginationDataset has 8 issues
	if len(issues) != 8 {
		t.Fatalf("expected 8 issues, got %d", len(issues))
	}
}

func TestChangelogPagination(t *testing.T) {
	srv := mockjira.New(mockjira.PaginationDataset())
	srv.MaxPageSize = 3
	ts := srv.Start()
	defer ts.Close()

	client := jira.NewClient(jira.Credentials{
		BaseURLOverride: ts.URL,
		Email:           "test@test.com",
		APIToken:        "fake",
	})

	issues, err := client.FetchIssuesWithHistory(context.Background(), "project = PROJ", nil)
	if err != nil {
		t.Fatalf("FetchIssuesWithHistory failed: %v", err)
	}

	// Find PROJ-8 which has 7 changelog entries
	var proj8 *jira.IssueWithHistory
	for i := range issues {
		if issues[i].Key == "PROJ-8" {
			proj8 = &issues[i]
			break
		}
	}
	if proj8 == nil {
		t.Fatal("PROJ-8 not found")
	}

	// With MaxPageSize=3, inline changelog is truncated → client should fetch full changelog
	// PROJ-8 has 7 status transitions
	if len(proj8.Transitions) != 7 {
		t.Errorf("PROJ-8: expected 7 transitions, got %d", len(proj8.Transitions))
	}
}
