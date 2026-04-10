package mockjira

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/danlafeir/em/pkg/jira"
)

// SmallDataset returns 5 deterministic issues for exact assertions.
func SmallDataset() *Dataset {
	ds := &Dataset{Changelogs: make(map[string][]jira.ChangelogEntry)}
	base := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	builders := []*IssueBuilder{
		// PROJ-1: Simple happy path, 3-day cycle time
		NewIssue("PROJ-1").
			WithSummary("Simple story").
			CreatedAt(base).
			AddTransition(base.Add(2*time.Hour), "Open", "In Progress").
			AddTransition(base.Add(72*time.Hour), "In Progress", "Done"),

		// PROJ-2: Multi-stage, 7-day cycle time
		NewIssue("PROJ-2").
			WithSummary("Multi-stage story").
			CreatedAt(base.Add(24*time.Hour)).
			AddTransition(base.Add(26*time.Hour), "Open", "In Progress").
			AddTransition(base.Add(96*time.Hour), "In Progress", "In Review").
			AddTransition(base.Add(192*time.Hour), "In Review", "Done"),

		// PROJ-3: Regression (went back to Open), 5-day cycle time
		NewIssue("PROJ-3").
			WithType("Bug").
			WithSummary("Bug with regression").
			CreatedAt(base.Add(48*time.Hour)).
			AddTransition(base.Add(50*time.Hour), "Open", "In Progress").
			AddTransition(base.Add(72*time.Hour), "In Progress", "Open").
			AddTransition(base.Add(96*time.Hour), "Open", "In Progress").
			AddTransition(base.Add(168*time.Hour), "In Progress", "Done"),

		// PROJ-4: WIP (unresolved, still in progress)
		NewIssue("PROJ-4").
			WithSummary("In-progress story").
			CreatedAt(base.Add(72*time.Hour)).
			AddTransition(base.Add(74*time.Hour), "Open", "In Progress"),

		// PROJ-5: Fast completion, 1-day cycle time
		NewIssue("PROJ-5").
			WithSummary("Quick story").
			CreatedAt(base.Add(96*time.Hour)).
			AddTransition(base.Add(98*time.Hour), "Open", "In Progress").
			AddTransition(base.Add(122*time.Hour), "In Progress", "Done"),
	}

	for _, b := range builders {
		issue, changelog := b.Build()
		ds.Issues = append(ds.Issues, issue)
		ds.Changelogs[issue.Key] = changelog
	}

	return ds
}

// PaginationDataset returns 8 issues to force multi-page results with MaxPageSize=3.
// PROJ-6 has 7 changelog entries to test changelog pagination.
func PaginationDataset() *Dataset {
	ds := &Dataset{Changelogs: make(map[string][]jira.ChangelogEntry)}
	base := time.Date(2024, 2, 1, 9, 0, 0, 0, time.UTC)

	// 7 simple issues
	for i := 1; i <= 7; i++ {
		b := NewIssue(issueName("PROJ", i)).
			WithSummary("Pagination test issue").
			CreatedAt(base.Add(time.Duration(i*24) * time.Hour)).
			AddTransition(base.Add(time.Duration(i*24+2)*time.Hour), "Open", "In Progress").
			AddTransition(base.Add(time.Duration(i*24+48)*time.Hour), "In Progress", "Done")
		issue, changelog := b.Build()
		ds.Issues = append(ds.Issues, issue)
		ds.Changelogs[issue.Key] = changelog
	}

	// PROJ-8: Issue with many changelog entries (7 transitions)
	b := NewIssue("PROJ-8").
		WithSummary("Issue with many transitions").
		CreatedAt(base)
	t := base.Add(2 * time.Hour)
	statuses := []string{"Open", "In Progress", "In Review", "In Progress", "In Review", "Testing", "In Review", "Done"}
	for i := 0; i < len(statuses)-1; i++ {
		b.AddTransition(t, statuses[i], statuses[i+1])
		t = t.Add(12 * time.Hour)
	}
	issue, changelog := b.Build()
	ds.Issues = append(ds.Issues, issue)
	ds.Changelogs[issue.Key] = changelog

	return ds
}

// RealisticDataset returns ~50 issues spanning 40 days with varied types and cycle times.
// Dates are anchored to time.Now() so resolved dates always fall within the default 42-day
// query window used by throughput, cycle-time, and other commands.
func RealisticDataset() *Dataset {
	ds := &Dataset{Changelogs: make(map[string][]jira.ChangelogEntry)}
	rng := rand.New(rand.NewSource(42)) // deterministic
	base := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -60)

	types := []string{"Story", "Bug", "Task", "Story", "Story", "Bug"}

	for i := 1; i <= 50; i++ {
		issueType := types[rng.Intn(len(types))]

		// Spread creation over 40 days (so resolved dates land within the 42-day window)
		createdOffset := time.Duration(rng.Intn(40*24)) * time.Hour
		created := base.Add(createdOffset)

		b := NewIssue(issueName("PROJ", i)).
			WithType(issueType).
			WithSummary(issueType + " for testing").
			CreatedAt(created)

		// Start work 1-5 days after creation
		startDelay := time.Duration(1+rng.Intn(5)) * 24 * time.Hour
		startTime := created.Add(startDelay)
		b.AddTransition(startTime, "Open", "In Progress")

		// 80% chance of completing
		if rng.Float64() < 0.8 {
			// Cycle time: 1-20 days
			cycleTime := time.Duration(1+rng.Intn(20)) * 24 * time.Hour

			// 40% go through review
			if rng.Float64() < 0.4 {
				reviewTime := startTime.Add(cycleTime / 2)
				b.AddTransition(reviewTime, "In Progress", "In Review")
				b.AddTransition(startTime.Add(cycleTime), "In Review", "Done")
			} else {
				b.AddTransition(startTime.Add(cycleTime), "In Progress", "Done")
			}
		}

		issue, changelog := b.Build()
		ds.Issues = append(ds.Issues, issue)
		ds.Changelogs[issue.Key] = changelog
	}

	return ds
}

func issueName(project string, num int) string {
	return project + "-" + strconv.Itoa(num)
}
