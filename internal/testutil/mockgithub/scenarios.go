package mockgithub

import (
	"math/rand"
	"time"

	"em/internal/github"
)

// SmallDataset returns a small deterministic dataset with ~3 weeks of deployments.
// Useful for quick sanity checks and exact assertions.
func SmallDataset() *Dataset {
	ds := NewDataset("acme", "platform", "acme-org", "api-service")
	base := time.Date(2024, 1, 8, 10, 0, 0, 0, time.UTC) // Monday

	var id int64 = 1

	// Week 1: 3 deploys
	for i := 0; i < 3; i++ {
		t := base.Add(time.Duration(i) * 24 * time.Hour)
		ds.AddRun("acme-org", "api-service", 1,
			NewRun(id).WithTimes(t, t.Add(4*time.Minute)).Build())
		id++
	}

	// Week 2: 2 deploys (one failure)
	week2 := base.Add(7 * 24 * time.Hour)
	ds.AddRun("acme-org", "api-service", 1,
		NewRun(id).WithTimes(week2, week2.Add(3*time.Minute)).Build())
	id++
	ds.AddRun("acme-org", "api-service", 1,
		NewRun(id).WithConclusion("failure").WithTimes(week2.Add(48*time.Hour), week2.Add(48*time.Hour+2*time.Minute)).Build())
	id++

	// Week 3: 4 deploys
	week3 := base.Add(14 * 24 * time.Hour)
	for i := 0; i < 4; i++ {
		t := week3.Add(time.Duration(i) * 24 * time.Hour)
		ds.AddRun("acme-org", "api-service", 1,
			NewRun(id).WithTimes(t, t.Add(5*time.Minute)).Build())
		id++
	}

	return ds
}

// RealisticDataset returns ~12 weeks of deployment data for two repos.
func RealisticDataset() *Dataset {
	ds := NewDataset("acme", "platform", "acme-org", "api-service")

	// Add a second repo for the team
	ds.Repos = append(ds.Repos, github.Repository{
		ID:       2,
		Name:     "web-app",
		FullName: "acme-org/web-app",
		Owner:    github.RepositoryOwner{Login: "acme-org"},
		HTMLURL:  "https://github.com/acme-org/web-app",
	})
	ds.Workflows["acme-org/web-app"] = []github.Workflow{
		{ID: 3, Name: "Deploy", Path: ".github/workflows/deploy.yml", State: "active"},
	}

	rng := rand.New(rand.NewSource(42)) // deterministic
	base := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)

	var id int64 = 1

	for week := 0; week < 12; week++ {
		weekStart := base.Add(time.Duration(week*7) * 24 * time.Hour)

		// api-service: 2–6 deployments per week, weekdays, 9am–5pm
		count := 2 + rng.Intn(5)
		for i := 0; i < count; i++ {
			dayOffset := rng.Intn(5)
			hourOffset := 9 + rng.Intn(8)
			t := weekStart.Add(time.Duration(dayOffset)*24*time.Hour + time.Duration(hourOffset)*time.Hour)
			run := NewRun(id).WithTimes(t, t.Add(time.Duration(3+rng.Intn(8))*time.Minute))
			if rng.Float64() < 0.1 { // ~10% failure rate
				run.WithConclusion("failure")
			}
			ds.AddRun("acme-org", "api-service", 1, run.Build())
			id++
		}

		// web-app: 1–3 deployments per week
		webCount := 1 + rng.Intn(3)
		for i := 0; i < webCount; i++ {
			dayOffset := rng.Intn(5)
			t := weekStart.Add(time.Duration(dayOffset)*24*time.Hour + 14*time.Hour)
			run := NewRun(id).WithTimes(t, t.Add(time.Duration(5+rng.Intn(10))*time.Minute))
			ds.AddRun("acme-org", "web-app", 3, run.Build())
			id++
		}
	}

	return ds
}
