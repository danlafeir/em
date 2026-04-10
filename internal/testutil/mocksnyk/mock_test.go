package mocksnyk_test

import (
	"context"
	"testing"
	"time"

	"github.com/danlafeir/em/internal/testutil/mocksnyk"
)

func TestSmallDataset_listIssues(t *testing.T) {
	ds := mocksnyk.SmallDataset()
	ts := mocksnyk.New(ds).Start()
	defer ts.Close()

	client := mocksnyk.NewClient(ts)
	ctx := context.Background()

	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	orgs, err := client.ListOrgs(ctx)
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	if len(orgs) == 0 {
		t.Fatal("expected at least one org")
	}

	from := time.Now().AddDate(0, 0, -90)
	to := time.Now()
	issues, err := client.ListIssues(ctx, from, to)
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected issues")
	}
}

func TestSmallDataset_openCounts(t *testing.T) {
	ds := mocksnyk.SmallDataset()
	ts := mocksnyk.New(ds).Start()
	defer ts.Close()

	client := mocksnyk.NewClient(ts)
	ctx := context.Background()

	counts, err := client.CountOpenIssues(ctx)
	if err != nil {
		t.Fatalf("CountOpenIssues: %v", err)
	}

	// SmallDataset open non-ignored: snyk-1 (critical/fixable/Functional),
	// snyk-2 (high/fixable/PoC), snyk-3 (medium/unfixable/PoC), snyk-4 (low/unfixable).
	// Ignored: snyk-5 (high/unfixable/Functional).
	if counts.Total == 0 {
		t.Errorf("expected non-zero open issue count, got %+v", counts)
	}

	// ExploitableFixable: snyk-1 + snyk-2 = 2 (greater than 1)
	if counts.ExploitableFixable < 2 {
		t.Errorf("expected ExploitableFixable >= 2, got %d", counts.ExploitableFixable)
	}
	// ExploitableHigh: snyk-2 = 1
	if counts.ExploitableHigh == 0 {
		t.Errorf("expected ExploitableHigh > 0, got %d", counts.ExploitableHigh)
	}
	// ExploitableCritical: snyk-1 = 1
	if counts.ExploitableCritical == 0 {
		t.Errorf("expected ExploitableCritical > 0, got %d", counts.ExploitableCritical)
	}
	// ExploitableIgnoredUnfixable: snyk-5 (ignored/Functional) = 1
	if counts.ExploitableIgnoredUnfixable == 0 {
		t.Errorf("expected ExploitableIgnoredUnfixable > 0, got %d", counts.ExploitableIgnoredUnfixable)
	}
}

func TestLoadFromIssuesCSV(t *testing.T) {
	ds, err := mocksnyk.LoadFromIssuesCSV("mock-org", "Mock Org", "testdata/issues.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Issues) == 0 {
		t.Fatal("expected issues from CSV")
	}

	openCount := 0
	for _, i := range ds.Issues {
		if i.Status == "open" {
			openCount++
		}
	}
	if openCount == 0 {
		t.Error("expected some open issues in CSV")
	}
}
