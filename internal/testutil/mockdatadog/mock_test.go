package mockdatadog_test

import (
	"context"
	"testing"
	"time"

	"github.com/danlafeir/em/internal/testutil/mockdatadog"
)

func TestSmallDataset_monitors(t *testing.T) {
	ds := mockdatadog.SmallDataset()
	ts := mockdatadog.New(ds).Start()
	defer ts.Close()

	client := mockdatadog.NewClient(ts)
	ctx := context.Background()

	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	monitors, err := client.ListMonitors(ctx, "team:platform")
	if err != nil {
		t.Fatalf("ListMonitors: %v", err)
	}
	if len(monitors) == 0 {
		t.Fatal("expected at least one monitor")
	}

	from := time.Now().AddDate(0, 0, -30)
	to := time.Now()
	events, err := client.ListMonitorEvents(ctx, "", from, to)
	if err != nil {
		t.Fatalf("ListMonitorEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected monitor events")
	}
}

func TestSmallDataset_slos(t *testing.T) {
	ds := mockdatadog.SmallDataset()
	ts := mockdatadog.New(ds).Start()
	defer ts.Close()

	client := mockdatadog.NewClient(ts)
	ctx := context.Background()

	from := time.Now().AddDate(0, 0, -14)
	to := time.Now()

	slos, err := client.ListSLOs(ctx, "team:platform")
	if err != nil {
		t.Fatalf("ListSLOs: %v", err)
	}
	if len(slos) == 0 {
		t.Fatal("expected at least one SLO")
	}

	// Fetch history for the first SLO
	history, err := client.GetSLOHistory(ctx, slos[0].ID, from, to)
	if err != nil {
		t.Fatalf("GetSLOHistory: %v", err)
	}
	if history == nil {
		t.Fatal("expected SLO history")
	}

	sloEvents, err := client.ListSLOEvents(ctx, from, to)
	if err != nil {
		t.Fatalf("ListSLOEvents: %v", err)
	}
	if len(sloEvents) == 0 {
		t.Fatal("expected SLO events")
	}
}

func TestLoadFromSLOCSV(t *testing.T) {
	ds, err := mockdatadog.LoadFromSLOCSV("testdata/slos.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.SLOs) == 0 {
		t.Fatal("expected SLOs from CSV")
	}

	// Check that violated SLOs have associated events
	violated := 0
	for _, slo := range ds.SLOs {
		if h, ok := ds.SLOHistory[slo.ID]; ok && h.SLIValue < slo.Thresholds[0].Target {
			violated++
		}
	}
	if violated == 0 {
		t.Error("expected some violated SLOs in CSV")
	}
}
