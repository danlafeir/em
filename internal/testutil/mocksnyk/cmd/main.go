package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/danlafeir/em/internal/testutil/mocksnyk"
)

func main() {
	addr := flag.String("addr", ":8082", "listen address")
	dataset := flag.String("dataset", "realistic", "dataset: small, realistic, or path to a Snyk issues CSV")
	orgID := flag.String("org-id", "mock-org-id", "Snyk org ID (used when loading from CSV)")
	orgName := flag.String("org-name", "Mock Org", "Snyk org name (used when loading from CSV)")
	maxPageSize := flag.Int("max-page-size", 100, "maximum page size for pagination")
	flag.Parse()

	var (
		ds  *mocksnyk.Dataset
		err error
	)

	switch *dataset {
	case "small":
		ds = mocksnyk.SmallDataset()
	case "realistic":
		ds = mocksnyk.RealisticDataset()
	default:
		// Treat as a CSV file path
		ds, err = mocksnyk.LoadFromIssuesCSV(*orgID, *orgName, *dataset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading CSV %s: %v\n", *dataset, err)
			os.Exit(1)
		}
	}

	srv := mocksnyk.New(ds)
	srv.MaxPageSize = *maxPageSize

	openCount := 0
	for _, i := range ds.Issues {
		if i.Status == "open" {
			openCount++
		}
	}

	fmt.Println(mocksnyk.Usage(*addr))
	log.Printf("Serving %s dataset (%d total issues, %d open) with max page size %d",
		*dataset, len(ds.Issues), openCount, *maxPageSize)

	if err := srv.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}
