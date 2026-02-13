// Package jira provides a client for the JIRA Cloud REST API.
package jira

import "time"

// Credentials holds JIRA Cloud authentication details.
type Credentials struct {
	Domain          string // e.g., "mycompany" (becomes mycompany.atlassian.net)
	Email           string // User email
	APIToken        string // API token from id.atlassian.com/manage/api-tokens
	BaseURLOverride string // If set, use instead of https://{domain}.atlassian.net
}

// BaseURL returns the JIRA Cloud base URL.
func (c *Credentials) BaseURL() string {
	if c.BaseURLOverride != "" {
		return c.BaseURLOverride
	}
	return "https://" + c.Domain + ".atlassian.net"
}

// SearchOptions configures JQL search requests.
type SearchOptions struct {
	StartAt    int
	MaxResults int
	Fields     string // Comma-separated field names
	Expand     string // changelog, transitions, etc.
}

// PaginatedResponse represents JIRA's standard pagination structure.
type PaginatedResponse struct {
	StartAt    int  `json:"startAt"`
	MaxResults int  `json:"maxResults"`
	Total      int  `json:"total"`
	IsLast     bool `json:"isLast,omitempty"`
}

// SearchResult represents the JQL search response.
type SearchResult struct {
	PaginatedResponse
	Issues []Issue `json:"issues"`
}

// Issue represents a JIRA issue.
type Issue struct {
	Key       string    `json:"key"`
	ID        string    `json:"id"`
	Self      string    `json:"self"`
	Fields    Fields    `json:"fields"`
	Changelog *Changelog `json:"changelog,omitempty"`
}

// Fields contains issue field data.
type Fields struct {
	Summary        string     `json:"summary"`
	Description    any        `json:"description,omitempty"` // Can be string or ADF object
	Status         Status     `json:"status"`
	IssueType      IssueType  `json:"issuetype"`
	Created        JiraTime   `json:"created"`
	Updated        JiraTime   `json:"updated"`
	ResolutionDate *JiraTime  `json:"resolutiondate,omitempty"`
	Project        Project    `json:"project"`
	Assignee       *User      `json:"assignee,omitempty"`
	Reporter       *User      `json:"reporter,omitempty"`
	Priority       *Priority  `json:"priority,omitempty"`
	Labels         []string   `json:"labels,omitempty"`
	Parent         *Parent    `json:"parent,omitempty"` // For subtasks or stories linked to epics
	Epic           *Epic      `json:"epic,omitempty"`   // Epic link (older style)
	StoryPoints    *float64   `json:"customfield_10026,omitempty"` // Configurable field ID
}

// Status represents issue status.
type Status struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	StatusCategory StatusCategory `json:"statusCategory"`
}

// StatusCategory represents the status category.
type StatusCategory struct {
	ID   int    `json:"id"`
	Key  string `json:"key"`  // new, indeterminate, done
	Name string `json:"name"` // To Do, In Progress, Done
}

// IssueType represents issue type (Story, Bug, Epic, etc.).
type IssueType struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Subtask  bool   `json:"subtask"`
	Hierarchical int `json:"hierarchyLevel,omitempty"`
}

// Project represents a JIRA project.
type Project struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// User represents a JIRA user.
type User struct {
	AccountID    string `json:"accountId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress,omitempty"`
	Active       bool   `json:"active"`
}

// Priority represents issue priority.
type Priority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Parent represents the parent issue (for subtasks or epic links).
type Parent struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Summary   string    `json:"summary"`
		Status    Status    `json:"status"`
		IssueType IssueType `json:"issuetype"`
	} `json:"fields"`
}

// Epic represents epic information.
type Epic struct {
	ID   int    `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Changelog represents the changelog for an issue.
type Changelog struct {
	PaginatedResponse
	Histories []ChangelogEntry `json:"histories"`
}

// ChangelogResult represents the standalone changelog API response.
type ChangelogResult struct {
	PaginatedResponse
	Values []ChangelogEntry `json:"values"`
}

// ChangelogEntry represents a single changelog entry.
type ChangelogEntry struct {
	ID      string       `json:"id"`
	Created JiraTime     `json:"created"`
	Author  User         `json:"author"`
	Items   []ChangeItem `json:"items"`
}

// ChangeItem represents a single field change.
type ChangeItem struct {
	Field      string `json:"field"`
	FieldType  string `json:"fieldtype"`
	From       string `json:"from"`
	FromString string `json:"fromString"`
	To         string `json:"to"`
	ToString   string `json:"toString"`
}

// JiraTime handles JIRA's datetime format.
type JiraTime struct {
	time.Time
}

// UnmarshalJSON parses JIRA's datetime format.
func (jt *JiraTime) UnmarshalJSON(data []byte) error {
	str := string(data)
	if str == "null" || str == `""` {
		return nil
	}
	// Remove quotes
	str = str[1 : len(str)-1]

	// JIRA uses ISO 8601 with timezone - try multiple formats
	formats := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05Z",
	}

	var parseErr error
	for _, format := range formats {
		t, err := time.Parse(format, str)
		if err == nil {
			jt.Time = t
			return nil
		}
		parseErr = err
	}
	return parseErr
}

// MarshalJSON formats time for JIRA API.
func (jt JiraTime) MarshalJSON() ([]byte, error) {
	if jt.Time.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + jt.Time.Format("2006-01-02T15:04:05.000-0700") + `"`), nil
}

// StatusTransition represents a status change extracted from changelog.
type StatusTransition struct {
	Timestamp  time.Time
	FromStatus string
	ToStatus   string
}

// IssueWithHistory combines an issue with its parsed status transitions.
type IssueWithHistory struct {
	Issue
	Transitions []StatusTransition
}

// ErrorResponse represents a JIRA API error.
type ErrorResponse struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}
