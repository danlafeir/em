package mockgithub_test

import (
	"context"
	"testing"
	"time"

	"em/internal/testutil/mockgithub"
)

func TestSmallDataset_workflowRuns(t *testing.T) {
	ds := mockgithub.SmallDataset()
	ts := mockgithub.New(ds).Start()
	defer ts.Close()

	client, err := mockgithub.NewClient(ts)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	repos, err := client.ListTeamRepos(ctx, "acme", "platform")
	if err != nil {
		t.Fatalf("ListTeamRepos: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("expected at least one repo")
	}

	workflows, err := client.ListWorkflows(ctx, "acme-org", "api-service")
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) == 0 {
		t.Fatal("expected at least one workflow")
	}

	from := time.Now().AddDate(0, 0, -90)
	to := time.Now()
	runs, err := client.ListWorkflowRuns(ctx, "acme-org", "api-service", workflows[0].ID, "main", from, to)
	if err != nil {
		t.Fatalf("ListWorkflowRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected workflow runs")
	}
}

func TestLoadFromDeploymentCSV(t *testing.T) {
	ds, err := mockgithub.LoadFromDeploymentCSV("testdata/deployments.csv")
	if err != nil {
		t.Fatal(err)
	}

	totalRuns := 0
	for _, runs := range ds.Runs {
		totalRuns += len(runs)
	}

	// testdata has 12 rows with counts summing to 51
	if totalRuns != 51 {
		t.Errorf("expected 51 runs from CSV, got %d", totalRuns)
	}
}
