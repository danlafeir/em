// Package mocksnyk provides an in-process Snyk REST API mock for testing.
package mocksnyk

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"em/internal/snyk"
)

// Dataset holds all mock Snyk data served by the Server.
type Dataset struct {
	OrgID    string
	OrgName  string
	Issues   []snyk.Issue // all issues (open + resolved)
	Projects []Project    // project → target mapping
}

// Project maps a project ID to a target ID (used for open counts deduplication).
type Project struct {
	ID       string
	TargetID string
}

// NewDataset creates a minimal Dataset with one org and one project.
func NewDataset(orgID, orgName string) *Dataset {
	return &Dataset{
		OrgID:   orgID,
		OrgName: orgName,
		Projects: []Project{
			{ID: "project-1", TargetID: "target-1"},
		},
	}
}

// IssueBuilder provides a fluent API for constructing test Snyk issues.
type IssueBuilder struct {
	id         string
	title      string
	severity   string
	issueType  string
	status     string
	isFixable  bool
	isIgnored  bool
	createdAt  time.Time
	resolvedAt time.Time
	projectID  string
}

// NewIssue starts building a new Snyk issue.
func NewIssue(id string) *IssueBuilder {
	return &IssueBuilder{
		id:        id,
		title:     "Vulnerability in " + id,
		severity:  "high",
		issueType: "vuln",
		status:    "open",
		createdAt: time.Now().Add(-30 * 24 * time.Hour),
		projectID: "project-1",
	}
}

func (b *IssueBuilder) WithTitle(title string) *IssueBuilder {
	b.title = title
	return b
}

func (b *IssueBuilder) WithSeverity(s string) *IssueBuilder {
	b.severity = s
	return b
}

func (b *IssueBuilder) WithType(t string) *IssueBuilder {
	b.issueType = t
	return b
}

func (b *IssueBuilder) WithStatus(s string) *IssueBuilder {
	b.status = s
	return b
}

func (b *IssueBuilder) AsFixable() *IssueBuilder {
	b.isFixable = true
	return b
}

func (b *IssueBuilder) AsIgnored() *IssueBuilder {
	b.isIgnored = true
	return b
}

func (b *IssueBuilder) WithCreatedAt(t time.Time) *IssueBuilder {
	b.createdAt = t
	return b
}

func (b *IssueBuilder) WithResolvedAt(t time.Time) *IssueBuilder {
	b.resolvedAt = t
	return b
}

func (b *IssueBuilder) WithProject(projectID string) *IssueBuilder {
	b.projectID = projectID
	return b
}

// Build returns the constructed snyk.Issue and the associated project ID.
func (b *IssueBuilder) Build() (snyk.Issue, string) {
	return snyk.Issue{
		ID:         b.id,
		Title:      b.title,
		Severity:   b.severity,
		IssueType:  b.issueType,
		Status:     b.status,
		IsFixable:  b.isFixable,
		IsIgnored:  b.isIgnored,
		CreatedAt:  b.createdAt,
		ResolvedAt: b.resolvedAt,
	}, b.projectID
}

// issueWithProject pairs an issue with its project ID (for JSON:API serialization).
type issueWithProject struct {
	Issue     snyk.Issue
	ProjectID string
}

// issues returns all issues with their project IDs.
func (ds *Dataset) issues() []issueWithProject {
	// We store issues without project IDs, so use "project-1" for all.
	// For datasets loaded from CSV, all issues belong to project-1.
	out := make([]issueWithProject, len(ds.Issues))
	for i, issue := range ds.Issues {
		out[i] = issueWithProject{Issue: issue, ProjectID: "project-1"}
	}
	return out
}

// LoadFromIssuesCSV loads issues from a Snyk issues CSV file.
// CSV format: id,title,severity,type,status,is_fixable,is_ignored,created_at,resolved_at
// (compatible with --save-raw-data output)
func LoadFromIssuesCSV(orgID, orgName, path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening issues CSV %s: %w", path, err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading issues CSV: %w", err)
	}

	ds := NewDataset(orgID, orgName)
	for _, row := range rows[1:] {
		if len(row) < 9 {
			continue
		}
		isFixable, _ := strconv.ParseBool(row[5])
		isIgnored, _ := strconv.ParseBool(row[6])
		createdAt, _ := time.Parse(time.RFC3339, row[7])
		resolvedAt, _ := time.Parse(time.RFC3339, row[8])
		ds.Issues = append(ds.Issues, snyk.Issue{
			ID:         row[0],
			Title:      row[1],
			Severity:   row[2],
			IssueType:  row[3],
			Status:     row[4],
			IsFixable:  isFixable,
			IsIgnored:  isIgnored,
			CreatedAt:  createdAt,
			ResolvedAt: resolvedAt,
		})
	}
	return ds, nil
}
