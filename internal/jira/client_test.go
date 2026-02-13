package jira

import (
	"testing"
	"time"
)

func TestExtractStatusTransitions(t *testing.T) {
	entries := []ChangelogEntry{
		{
			Created: JiraTime{Time: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
			Items: []ChangeItem{
				{Field: "status", FromString: "Open", ToString: "In Progress"},
			},
		},
		{
			Created: JiraTime{Time: time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC)},
			Items: []ChangeItem{
				{Field: "assignee", FromString: "Alice", ToString: "Bob"}, // Not a status change
			},
		},
		{
			Created: JiraTime{Time: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC)},
			Items: []ChangeItem{
				{Field: "status", FromString: "In Progress", ToString: "Done"},
			},
		},
	}

	transitions := ExtractStatusTransitions(entries)

	if len(transitions) != 2 {
		t.Fatalf("Expected 2 transitions, got %d", len(transitions))
	}

	// First transition
	if transitions[0].FromStatus != "Open" {
		t.Errorf("First transition FromStatus = %q, want %q", transitions[0].FromStatus, "Open")
	}
	if transitions[0].ToStatus != "In Progress" {
		t.Errorf("First transition ToStatus = %q, want %q", transitions[0].ToStatus, "In Progress")
	}

	// Second transition
	if transitions[1].FromStatus != "In Progress" {
		t.Errorf("Second transition FromStatus = %q, want %q", transitions[1].FromStatus, "In Progress")
	}
	if transitions[1].ToStatus != "Done" {
		t.Errorf("Second transition ToStatus = %q, want %q", transitions[1].ToStatus, "Done")
	}
}

func TestExtractStatusTransitions_Empty(t *testing.T) {
	transitions := ExtractStatusTransitions([]ChangelogEntry{})

	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions, got %d", len(transitions))
	}
}

func TestExtractStatusTransitions_NoStatusChanges(t *testing.T) {
	entries := []ChangelogEntry{
		{
			Created: JiraTime{Time: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
			Items: []ChangeItem{
				{Field: "priority", FromString: "Medium", ToString: "High"},
				{Field: "assignee", FromString: "", ToString: "Alice"},
			},
		},
	}

	transitions := ExtractStatusTransitions(entries)

	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions, got %d", len(transitions))
	}
}

func TestExtractStatusTransitions_MultipleItemsPerEntry(t *testing.T) {
	entries := []ChangelogEntry{
		{
			Created: JiraTime{Time: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
			Items: []ChangeItem{
				{Field: "assignee", FromString: "", ToString: "Alice"},
				{Field: "status", FromString: "Open", ToString: "In Progress"},
				{Field: "priority", FromString: "Medium", ToString: "High"},
			},
		},
	}

	transitions := ExtractStatusTransitions(entries)

	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].ToStatus != "In Progress" {
		t.Errorf("Expected ToStatus='In Progress', got %q", transitions[0].ToStatus)
	}
}

func TestCalculateBackoff(t *testing.T) {
	client := &Client{
		rateLimiter: &RateLimiter{
			BaseDelay:  2 * time.Second,
			MaxDelay:   30 * time.Second,
			MaxRetries: 5,
		},
	}

	// Test with Retry-After header
	delay := client.calculateBackoff(0, "5")
	if delay != 5*time.Second {
		t.Errorf("With Retry-After=5, expected 5s, got %v", delay)
	}

	// Test without Retry-After header (exponential backoff)
	// Attempt 0: base * 2^0 = 2s (with jitter 0.7-1.3)
	delay = client.calculateBackoff(0, "")
	if delay < 1400*time.Millisecond || delay > 2600*time.Millisecond {
		t.Errorf("Attempt 0: expected ~2s with jitter, got %v", delay)
	}

	// Attempt 2: base * 2^2 = 8s (with jitter 0.7-1.3)
	delay = client.calculateBackoff(2, "")
	if delay < 5600*time.Millisecond || delay > 10400*time.Millisecond {
		t.Errorf("Attempt 2: expected ~8s with jitter, got %v", delay)
	}

	// Test max delay cap
	// Attempt 10: would be 2 * 2^10 = 2048s, but capped at 30s
	delay = client.calculateBackoff(10, "")
	if delay > 30*time.Second {
		t.Errorf("Expected delay capped at 30s, got %v", delay)
	}
}

func TestCredentials_BaseURL(t *testing.T) {
	creds := Credentials{
		Domain:   "mycompany",
		Email:    "user@example.com",
		APIToken: "secret",
	}

	expected := "https://mycompany.atlassian.net"
	if creds.BaseURL() != expected {
		t.Errorf("BaseURL() = %q, want %q", creds.BaseURL(), expected)
	}
}

func TestNewClient(t *testing.T) {
	creds := Credentials{
		Domain:   "test",
		Email:    "test@example.com",
		APIToken: "token",
	}

	client := NewClient(creds)

	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.credentials.Domain != "test" {
		t.Errorf("Expected domain 'test', got %q", client.credentials.Domain)
	}
	if client.rateLimiter == nil {
		t.Error("Expected non-nil rate limiter")
	}
	if client.rateLimiter.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", client.rateLimiter.MaxRetries)
	}
}
