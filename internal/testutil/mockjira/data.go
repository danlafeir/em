package mockjira

import (
	"fmt"
	"time"

	"devctl-em/internal/jira"
)

// Dataset holds a collection of mock JIRA issues and their changelogs.
type Dataset struct {
	Issues     []jira.Issue
	Changelogs map[string][]jira.ChangelogEntry // keyed by issue key
}

// IssueBuilder provides a fluent API for constructing realistic test issues.
type IssueBuilder struct {
	key         string
	issueType   string
	summary     string
	points      *float64
	createdAt   time.Time
	transitions []transition
}

type transition struct {
	at   time.Time
	from string
	to   string
}

// NewIssue starts building a new test issue.
func NewIssue(key string) *IssueBuilder {
	return &IssueBuilder{
		key:       key,
		issueType: "Story",
		summary:   key + " summary",
		createdAt: time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC),
	}
}

func (b *IssueBuilder) WithType(t string) *IssueBuilder {
	b.issueType = t
	return b
}

func (b *IssueBuilder) WithSummary(s string) *IssueBuilder {
	b.summary = s
	return b
}

func (b *IssueBuilder) WithPoints(p float64) *IssueBuilder {
	b.points = &p
	return b
}

func (b *IssueBuilder) CreatedAt(t time.Time) *IssueBuilder {
	b.createdAt = t
	return b
}

func (b *IssueBuilder) AddTransition(at time.Time, from, to string) *IssueBuilder {
	b.transitions = append(b.transitions, transition{at: at, from: from, to: to})
	return b
}

// Build constructs the jira.Issue and corresponding changelog entries.
func (b *IssueBuilder) Build() (jira.Issue, []jira.ChangelogEntry) {
	// Determine current status from last transition
	currentStatus := "Open"
	if len(b.transitions) > 0 {
		currentStatus = b.transitions[len(b.transitions)-1].to
	}

	statusCat := statusCategory(currentStatus)

	// Determine resolution date
	var resDate *jira.JiraTime
	if statusCat.Key == "done" {
		last := b.transitions[len(b.transitions)-1].at
		resDate = &jira.JiraTime{Time: last}
	}

	issue := jira.Issue{
		Key:  b.key,
		ID:   fmt.Sprintf("1%s", b.key[len(b.key)-1:]),
		Self: fmt.Sprintf("https://mock.atlassian.net/rest/api/3/issue/%s", b.key),
		Fields: jira.Fields{
			Summary: b.summary,
			Status: jira.Status{
				ID:             "1",
				Name:           currentStatus,
				StatusCategory: statusCat,
			},
			IssueType: jira.IssueType{
				ID:   issueTypeID(b.issueType),
				Name: b.issueType,
			},
			Created:        jira.JiraTime{Time: b.createdAt},
			Updated:        jira.JiraTime{Time: time.Now()},
			ResolutionDate: resDate,
			Project: jira.Project{
				ID:   "10000",
				Key:  "PROJ",
				Name: "Test Project",
			},
			StoryPoints: b.points,
		},
	}

	// Build changelog entries
	var entries []jira.ChangelogEntry
	for i, tr := range b.transitions {
		entry := jira.ChangelogEntry{
			ID:      fmt.Sprintf("%s-%d", b.key, i+1),
			Created: jira.JiraTime{Time: tr.at},
			Author: jira.User{
				AccountID:   "user1",
				DisplayName: "Test User",
				Active:      true,
			},
			Items: []jira.ChangeItem{
				{
					Field:      "status",
					FieldType:  "jira",
					FromString: tr.from,
					ToString:   tr.to,
				},
			},
		}
		// Mix in an assignee change on the first transition to test filtering
		if i == 0 {
			entry.Items = append(entry.Items, jira.ChangeItem{
				Field:      "assignee",
				FieldType:  "jira",
				FromString: "",
				ToString:   "Test User",
			})
		}
		entries = append(entries, entry)
	}

	return issue, entries
}

func statusCategory(status string) jira.StatusCategory {
	switch status {
	case "Done":
		return jira.StatusCategory{ID: 3, Key: "done", Name: "Done"}
	case "Open", "Backlog", "To Do":
		return jira.StatusCategory{ID: 2, Key: "new", Name: "To Do"}
	default:
		return jira.StatusCategory{ID: 4, Key: "indeterminate", Name: "In Progress"}
	}
}

func issueTypeID(t string) string {
	switch t {
	case "Bug":
		return "10001"
	case "Task":
		return "10002"
	case "Epic":
		return "10003"
	default:
		return "10000"
	}
}
