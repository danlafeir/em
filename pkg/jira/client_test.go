package jira

import (
	"encoding/json"
	"testing"
	"time"

	"em/pkg/httputil"
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
	// Backoff logic lives in httputil; verify it works via the shared package.
	r := &httputil.RateLimiter{
		BaseDelay:  2 * time.Second,
		MaxDelay:   30 * time.Second,
		MaxRetries: 5,
	}

	if delay := r.Backoff(0, "5"); delay != 5*time.Second {
		t.Errorf("With Retry-After=5, expected 5s, got %v", delay)
	}

	if delay := r.Backoff(0, ""); delay < 1400*time.Millisecond || delay > 2600*time.Millisecond {
		t.Errorf("Attempt 0: expected ~2s with jitter, got %v", delay)
	}

	if delay := r.Backoff(2, ""); delay < 5600*time.Millisecond || delay > 10400*time.Millisecond {
		t.Errorf("Attempt 2: expected ~8s with jitter, got %v", delay)
	}

	if delay := r.Backoff(10, ""); delay > 30*time.Second {
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

func TestCredentials_BaseURL_Override(t *testing.T) {
	creds := Credentials{
		Domain:          "ignored",
		BaseURLOverride: "https://jira.internal.example.com",
	}

	expected := "https://jira.internal.example.com"
	if creds.BaseURL() != expected {
		t.Errorf("BaseURL() with override = %q, want %q", creds.BaseURL(), expected)
	}
}

func TestClient_BaseURL(t *testing.T) {
	client := NewClient(Credentials{Domain: "acme"})
	want := "https://acme.atlassian.net"
	if got := client.BaseURL(); got != want {
		t.Errorf("Client.BaseURL() = %q, want %q", got, want)
	}
}

func TestClient_BrowseURL(t *testing.T) {
	client := NewClient(Credentials{Domain: "acme"})
	want := "https://acme.atlassian.net/browse/PROJ-123"
	if got := client.BrowseURL("PROJ-123"); got != want {
		t.Errorf("Client.BrowseURL() = %q, want %q", got, want)
	}
}

func TestJiraTime_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, jt JiraTime)
	}{
		{
			name:  "null",
			input: `null`,
			check: func(t *testing.T, jt JiraTime) {
				if !jt.Time.IsZero() {
					t.Errorf("expected zero time for null, got %v", jt.Time)
				}
			},
		},
		{
			name:  "empty string",
			input: `""`,
			check: func(t *testing.T, jt JiraTime) {
				if !jt.Time.IsZero() {
					t.Errorf("expected zero time for empty string, got %v", jt.Time)
				}
			},
		},
		{
			name:  "format with milliseconds and timezone offset",
			input: `"2024-01-15T10:30:00.000+0100"`,
			check: func(t *testing.T, jt JiraTime) {
				if jt.Year() != 2024 || jt.Month() != 1 || jt.Day() != 15 {
					t.Errorf("expected 2024-01-15, got %v", jt.Time)
				}
			},
		},
		{
			name:  "format with milliseconds and Z",
			input: `"2024-06-01T09:00:00.000Z"`,
			check: func(t *testing.T, jt JiraTime) {
				if jt.Year() != 2024 || jt.Month() != 6 || jt.Day() != 1 {
					t.Errorf("expected 2024-06-01, got %v", jt.Time)
				}
				if jt.Hour() != 9 {
					t.Errorf("expected hour 9, got %d", jt.Hour())
				}
			},
		},
		{
			name:  "format without milliseconds and timezone",
			input: `"2024-03-10T14:00:00-0500"`,
			check: func(t *testing.T, jt JiraTime) {
				if jt.Year() != 2024 || jt.Month() != 3 || jt.Day() != 10 {
					t.Errorf("expected 2024-03-10, got %v", jt.Time)
				}
			},
		},
		{
			name:  "format without milliseconds and Z",
			input: `"2024-12-31T23:59:59Z"`,
			check: func(t *testing.T, jt JiraTime) {
				if jt.Year() != 2024 || jt.Month() != 12 || jt.Day() != 31 {
					t.Errorf("expected 2024-12-31, got %v", jt.Time)
				}
			},
		},
		{
			name:    "invalid format",
			input:   `"not-a-date"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var jt JiraTime
			err := json.Unmarshal([]byte(tt.input), &jt)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalJSON(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, jt)
			}
		})
	}
}

func TestJiraTime_MarshalJSON(t *testing.T) {
	t.Run("zero time marshals to null", func(t *testing.T) {
		jt := JiraTime{}
		data, err := json.Marshal(jt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != "null" {
			t.Errorf("MarshalJSON() = %q, want %q", data, "null")
		}
	})

	t.Run("non-zero time marshals to ISO string", func(t *testing.T) {
		jt := JiraTime{Time: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)}
		data, err := json.Marshal(jt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should start and end with quotes
		s := string(data)
		if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
			t.Errorf("MarshalJSON() = %q, expected quoted string", s)
		}
		// Should round-trip
		var rt JiraTime
		if err := json.Unmarshal(data, &rt); err != nil {
			t.Fatalf("round-trip unmarshal failed: %v", err)
		}
		if !rt.Time.Equal(jt.Time) {
			t.Errorf("round-trip: got %v, want %v", rt.Time, jt.Time)
		}
	})
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
