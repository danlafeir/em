package mockdatadog

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/danlafeir/em/internal/datadog"
)

// SmallDataset returns a small deterministic dataset with a handful of monitors and SLOs
// anchored to the current time so events fall within any recent default date range.
func SmallDataset() *Dataset {
	ds := NewDataset()
	base := time.Now().UTC().Truncate(24 * time.Hour)

	// Monitors
	ds.Monitors = []datadog.Monitor{
		NewMonitor(1, "API Error Rate").WithTags("team:platform", "app:api-service").Build(),
		NewMonitor(2, "DB Connection Pool").WithTags("team:platform", "app:api-service").AsAlerted().Build(),
		NewMonitor(3, "Memory Usage").WithTags("team:platform", "app:web-app").Build(),
	}

	// Monitor events (recent alerts)
	ds.MonitorEvents = []MonitorEventRecord{
		{
			ID:        "me-001",
			MonitorID: 2,
			Title:     "DB Connection Pool triggered",
			Status:    "Alert",
			Timestamp: base.Add(-2 * 24 * time.Hour),
			Tags:      []string{"team:platform", "app:api-service"},
		},
		{
			ID:        "me-002",
			MonitorID: 2,
			Title:     "DB Connection Pool triggered",
			Status:    "Alert",
			Timestamp: base.Add(-5 * 24 * time.Hour),
			Tags:      []string{"team:platform", "app:api-service"},
		},
	}

	// SLOs
	slos := []struct {
		id      string
		app     string
		name    string
		current float64
		budget  float64
		events  int
	}{
		{"slo-001", "api-service", "API Availability", 99.95, 80.0, 0},
		{"slo-002", "api-service", "API Latency p99", 98.5, -15.0, 3},   // violated
		{"slo-003", "web-app", "Web Availability", 99.99, 95.0, 0},
		{"slo-004", "web-app", "Checkout Latency", 99.8, 40.0, 1},
	}

	for _, s := range slos {
		b := NewSLO(s.id, s.app, s.name).WithCurrent(s.current).WithBudget(s.budget)
		if s.current < 99.9 {
			b.WithTarget(99.9)
		}
		slo, history := b.Build()
		ds.SLOs = append(ds.SLOs, slo)
		ds.SLOHistory[s.id] = history

		for i := range s.events {
			ts := base.Add(-time.Duration(i+1) * 24 * time.Hour)
			ds.SLOEvents = append(ds.SLOEvents, SLOEventRecord{
				ID:        fmt.Sprintf("slo-event-%s-%d", s.id, i),
				SLOID:     s.id,
				Title:     fmt.Sprintf("SLO violated: %s", s.name),
				Timestamp: ts,
				Tags:      []string{"slo_id:" + s.id, "team:platform", "app:" + s.app},
			})
		}
	}

	return ds
}

// RealisticDataset returns a richer dataset with ~10 monitors and ~20 SLOs
// anchored to the current time.
func RealisticDataset() *Dataset {
	ds := NewDataset()
	base := time.Now().UTC().Truncate(24 * time.Hour)
	rng := rand.New(rand.NewSource(42))

	apps := []string{"api-service", "web-app", "payments", "auth-service", "worker"}
	monitorTypes := []string{"metric alert", "service check", "log alert"}
	states := []string{"OK", "OK", "OK", "OK", "Alert", "Warn"} // ~33% non-OK

	var monitorID int64 = 1
	for _, app := range apps {
		for j := range 2 {
			names := []string{
				fmt.Sprintf("%s Error Rate", app),
				fmt.Sprintf("%s Latency p99", app),
				fmt.Sprintf("%s Memory", app),
			}
			name := names[j%len(names)]
			state := states[rng.Intn(len(states))]
			m := NewMonitor(monitorID, name).
				WithType(monitorTypes[rng.Intn(len(monitorTypes))]).
				WithState(state).
				WithTags("team:platform", "app:"+app).
				Build()
			ds.Monitors = append(ds.Monitors, m)

			// Generate recent events for alerted monitors
			if state == "Alert" || state == "Warn" {
				count := 1 + rng.Intn(4)
				for k := range count {
					ts := base.Add(-time.Duration(rng.Intn(14)) * 24 * time.Hour)
					ds.MonitorEvents = append(ds.MonitorEvents, MonitorEventRecord{
						ID:        fmt.Sprintf("me-%d-%d", monitorID, k),
						MonitorID: monitorID,
						Title:     fmt.Sprintf("%s triggered", name),
						Status:    state,
						Timestamp: ts,
						Tags:      []string{"team:platform", "app:" + app},
					})
				}
			}
			monitorID++
		}
	}

	// SLOs: 3-5 per app
	sloTargets := []float64{99.9, 99.5, 99.0}
	var sloIdx int
	for _, app := range apps {
		count := 3 + rng.Intn(3)
		for j := range count {
			sloID := fmt.Sprintf("slo-%03d", sloIdx+1)
			names := []string{
				fmt.Sprintf("%s Availability", app),
				fmt.Sprintf("%s Latency", app),
				fmt.Sprintf("%s Success Rate", app),
				fmt.Sprintf("%s Throughput", app),
				fmt.Sprintf("%s Error Budget", app),
			}
			name := names[j%len(names)]
			target := sloTargets[rng.Intn(len(sloTargets))]

			// ~20% violated
			var current, budget float64
			var eventCount int
			if rng.Float64() < 0.2 {
				current = target - 0.5 - rng.Float64()*2
				budget = -(5.0 + rng.Float64()*40)
				eventCount = 1 + rng.Intn(5)
			} else {
				current = target + rng.Float64()*(100-target)
				budget = 20.0 + rng.Float64()*80
				eventCount = 0
			}

			slo, history := NewSLO(sloID, app, name).
				WithTarget(target).
				WithCurrent(current).
				WithBudget(budget).
				Build()
			ds.SLOs = append(ds.SLOs, slo)
			ds.SLOHistory[sloID] = history

			for k := range eventCount {
				ts := base.Add(-time.Duration(rng.Intn(14)) * 24 * time.Hour)
				ds.SLOEvents = append(ds.SLOEvents, SLOEventRecord{
					ID:        fmt.Sprintf("slo-event-%s-%d", sloID, k),
					SLOID:     sloID,
					Title:     fmt.Sprintf("SLO violated: %s", name),
					Timestamp: ts,
					Tags:      []string{"slo_id:" + sloID, "team:platform", "app:" + app},
				})
			}
			sloIdx++
		}
	}

	return ds
}
