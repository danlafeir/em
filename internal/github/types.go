// Package github provides a client for the GitHub REST API.
package github

import "time"

// Credentials holds GitHub authentication details.
type Credentials struct {
	Token           string // Personal access token or GitHub App token
	Org             string // GitHub organization name
	BaseURLOverride string // If set, use instead of https://api.github.com
}

// BaseURL returns the GitHub API base URL.
func (c *Credentials) BaseURL() string {
	if c.BaseURLOverride != "" {
		return c.BaseURLOverride
	}
	return "https://api.github.com"
}

// Repository represents a GitHub repository.
type Repository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"html_url"`
}

// Workflow represents a GitHub Actions workflow.
type Workflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// WorkflowRun represents a single GitHub Actions workflow run.
type WorkflowRun struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HeadBranch string    `json:"head_branch"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	RunNumber  int       `json:"run_number"`
	HTMLURL    string    `json:"html_url"`
}

// WorkflowListResponse represents the response from listing workflows.
type WorkflowListResponse struct {
	TotalCount int        `json:"total_count"`
	Workflows  []Workflow `json:"workflows"`
}

// WorkflowRunsResponse represents the response from listing workflow runs.
type WorkflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}
