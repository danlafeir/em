// Package mockgithub provides an in-process GitHub API mock for testing.
package mockgithub

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/danlafeir/em/internal/github"
)

// Dataset holds all mock GitHub data served by the Server.
type Dataset struct {
	Org      string
	Teams    []github.Team
	Repos    []github.Repository
	Workflows map[string][]github.Workflow   // "owner/repo" → workflows
	Runs      map[string][]github.WorkflowRun // "owner/repo/<workflowID>" → runs
}

// NewDataset creates a Dataset with a single org/team/repo/workflow.
func NewDataset(org, teamSlug, owner, repo string) *Dataset {
	return &Dataset{
		Org: org,
		Teams: []github.Team{
			{
				ID:           1,
				Slug:         teamSlug,
				Name:         teamSlug,
				Organization: github.TeamOrg{Login: org},
			},
		},
		Repos: []github.Repository{
			{
				ID:       1,
				Name:     repo,
				FullName: owner + "/" + repo,
				Owner:    github.RepositoryOwner{Login: owner},
				Private:  false,
				Archived: false,
				HTMLURL:  "https://github.com/" + owner + "/" + repo,
			},
		},
		Workflows: map[string][]github.Workflow{
			owner + "/" + repo: {
				{ID: 1, Name: "Deploy", Path: ".github/workflows/deploy.yml", State: "active"},
				{ID: 2, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
			},
		},
		Runs: make(map[string][]github.WorkflowRun),
	}
}

// AddRun adds a workflow run to the dataset.
func (ds *Dataset) AddRun(owner, repo string, workflowID int64, run github.WorkflowRun) {
	key := runsKey(owner, repo, workflowID)
	ds.Runs[key] = append(ds.Runs[key], run)
}

func runsKey(owner, repo string, workflowID int64) string {
	return fmt.Sprintf("%s/%s/%d", owner, repo, workflowID)
}

// WorkflowRunBuilder provides a fluent API for constructing test workflow runs.
type WorkflowRunBuilder struct {
	id         int64
	name       string
	status     string
	conclusion string
	branch     string
	createdAt  time.Time
	updatedAt  time.Time
}

// NewRun starts building a new workflow run.
func NewRun(id int64) *WorkflowRunBuilder {
	return &WorkflowRunBuilder{
		id:         id,
		name:       "Deploy",
		status:     "completed",
		conclusion: "success",
		branch:     "main",
		createdAt:  time.Now(),
		updatedAt:  time.Now().Add(5 * time.Minute),
	}
}

func (b *WorkflowRunBuilder) WithName(name string) *WorkflowRunBuilder {
	b.name = name
	return b
}

func (b *WorkflowRunBuilder) WithConclusion(c string) *WorkflowRunBuilder {
	b.conclusion = c
	return b
}

func (b *WorkflowRunBuilder) WithBranch(branch string) *WorkflowRunBuilder {
	b.branch = branch
	return b
}

func (b *WorkflowRunBuilder) WithTimes(createdAt, updatedAt time.Time) *WorkflowRunBuilder {
	b.createdAt = createdAt
	b.updatedAt = updatedAt
	return b
}

// Build returns the constructed WorkflowRun.
func (b *WorkflowRunBuilder) Build() github.WorkflowRun {
	return github.WorkflowRun{
		ID:         b.id,
		Name:       b.name,
		Status:     b.status,
		Conclusion: b.conclusion,
		HeadBranch: b.branch,
		CreatedAt:  b.createdAt,
		UpdatedAt:  b.updatedAt,
		RunNumber:  int(b.id),
		HTMLURL:    fmt.Sprintf("https://github.com/mock-owner/mock-repo/actions/runs/%d", b.id),
	}
}

// LoadFromDeploymentCSV loads a Dataset from a GitHub deployment throughput CSV.
// The CSV format is: period_start,period_end,count (output of --save-raw-data).
// Each row's count is expanded into individual workflow runs spread across the period.
func LoadFromDeploymentCSV(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening deployment CSV %s: %w", path, err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading deployment CSV: %w", err)
	}

	ds := NewDataset("mock-org", "mock-team", "mock-owner", "mock-repo")

	var runID int64 = 1
	for _, row := range rows[1:] {
		if len(row) < 3 {
			continue
		}
		start, err1 := time.Parse(time.RFC3339, row[0])
		end, err2 := time.Parse(time.RFC3339, row[1])
		count, err3 := strconv.Atoi(row[2])
		if err1 != nil || err2 != nil || err3 != nil || count <= 0 {
			continue
		}

		// Spread runs evenly across the period
		interval := end.Sub(start) / time.Duration(count)
		for i := 0; i < count; i++ {
			t := start.Add(time.Duration(i) * interval)
			run := NewRun(runID).WithTimes(t, t.Add(5*time.Minute)).Build()
			ds.AddRun("mock-owner", "mock-repo", 1, run)
			runID++
		}
	}

	return ds, nil
}
