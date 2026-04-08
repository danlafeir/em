package mocksnyk

import (
	"fmt"
	"math/rand"
	"time"
)

// SmallDataset returns a small deterministic dataset with ~20 issues
// anchored to the current time so resolved issues fall within any recent date range.
func SmallDataset() *Dataset {
	ds := NewDataset("mock-org-id", "Mock Org")
	base := time.Now().UTC().Truncate(24 * time.Hour)

	// Mix of open and resolved issues across severities
	builders := []*IssueBuilder{
		NewIssue("snyk-1").WithTitle("SQL Injection").WithSeverity("critical").AsFixable().WithExploitability("Functional"),
		NewIssue("snyk-2").WithTitle("Prototype Pollution").WithSeverity("high").AsFixable().WithExploitability("Proof of Concept"),
		NewIssue("snyk-3").WithTitle("ReDoS").WithSeverity("medium"),
		NewIssue("snyk-4").WithTitle("Outdated Dependency").WithSeverity("low"),
		NewIssue("snyk-5").WithTitle("Path Traversal").WithSeverity("high").AsIgnored(),

		// Resolved issues
		NewIssue("snyk-6").WithTitle("RCE in lodash").WithSeverity("critical").AsFixable().
			WithStatus("resolved").
			WithCreatedAt(base.Add(-60 * 24 * time.Hour)).
			WithResolvedAt(base.Add(-10 * 24 * time.Hour)),
		NewIssue("snyk-7").WithTitle("SSRF vulnerability").WithSeverity("high").
			WithStatus("resolved").
			WithCreatedAt(base.Add(-45 * 24 * time.Hour)).
			WithResolvedAt(base.Add(-5 * 24 * time.Hour)),
	}

	for _, b := range builders {
		b.WithCreatedAt(base.Add(-30 * 24 * time.Hour))
		issue, _ := b.Build()
		ds.Issues = append(ds.Issues, issue)
	}

	return ds
}

// RealisticDataset returns ~100 issues with realistic severity/fixability distribution.
func RealisticDataset() *Dataset {
	ds := NewDataset("prod-org-id", "Production Org")

	// Add multiple projects (simulate multiple repos)
	ds.Projects = []Project{
		{ID: "project-1", TargetID: "target-1"},
		{ID: "project-2", TargetID: "target-2"},
		{ID: "project-3", TargetID: "target-2"}, // two projects, same target (dedup test)
	}

	rng := rand.New(rand.NewSource(42)) // deterministic seed, relative dates
	// Anchor 90 days before today so all data falls within any recent default range
	base := time.Now().UTC().Truncate(24 * time.Hour)

	severities := []string{"critical", "high", "high", "medium", "medium", "medium", "low"}
	titles := []string{
		"SQL Injection", "Cross-Site Scripting", "Prototype Pollution",
		"Remote Code Execution", "Path Traversal", "ReDoS",
		"Outdated Dependency", "Insecure Deserialization", "Buffer Overflow",
		"Directory Traversal", "SSRF", "XXE Injection",
	}
	exploitMaturity := []string{
		"", "", "", "", // ~40% no known exploit
		"Proof of Concept", "Proof of Concept", // ~20% PoC
		"Functional",  // ~10% functional
		"High",        // ~10% high
	}

	for i := 1; i <= 100; i++ {
		severity := severities[rng.Intn(len(severities))]
		title := titles[rng.Intn(len(titles))]
		maturity := exploitMaturity[rng.Intn(len(exploitMaturity))]

		createdOffset := time.Duration(rng.Intn(90)) * 24 * time.Hour
		createdAt := base.Add(-createdOffset)

		b := NewIssue(fmt.Sprintf("snyk-%03d", i)).
			WithTitle(title).
			WithSeverity(severity).
			WithCreatedAt(createdAt).
			WithExploitability(maturity)

		// ~40% fixable
		if rng.Float64() < 0.4 {
			b.AsFixable()
		}

		// ~10% ignored
		if rng.Float64() < 0.1 {
			b.AsIgnored()
		}

		// ~25% resolved within the past 30 days
		if rng.Float64() < 0.25 {
			resolvedAt := createdAt.Add(time.Duration(1+rng.Intn(30)) * 24 * time.Hour)
			b.WithStatus("resolved").WithResolvedAt(resolvedAt)
		}

		issue, _ := b.Build()
		ds.Issues = append(ds.Issues, issue)
	}

	return ds
}
