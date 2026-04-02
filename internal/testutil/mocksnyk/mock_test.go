package mocksnyk_test

import (
	"context"
	"testing"
	"time"

	"em/internal/testutil/mocksnyk"
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

	from := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
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

	// SmallDataset has 5 open issues: 1 critical, 1 high (ignored), 1 medium, 1 low, 1 high fixable
	// After deduplication and ignored separation: 4 non-ignored open, 1 ignored
	if counts.Total == 0 {
		t.Errorf("expected non-zero open issue count, got %+v", counts)
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
