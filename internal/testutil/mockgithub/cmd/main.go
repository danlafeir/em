package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"em/internal/testutil/mockgithub"
)

func main() {
	addr := flag.String("addr", ":8081", "listen address")
	dataset := flag.String("dataset", "realistic", "dataset: small, realistic, or path to a deployment CSV")
	maxPageSize := flag.Int("max-page-size", 100, "maximum page size for pagination")
	flag.Parse()

	var (
		ds  *mockgithub.Dataset
		err error
	)

	switch *dataset {
	case "small":
		ds = mockgithub.SmallDataset()
	case "realistic":
		ds = mockgithub.RealisticDataset()
	default:
		// Treat as a CSV file path
		ds, err = mockgithub.LoadFromDeploymentCSV(*dataset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading CSV %s: %v\n", *dataset, err)
			os.Exit(1)
		}
	}

	srv := mockgithub.New(ds)
	srv.MaxPageSize = *maxPageSize

	totalRuns := 0
	for _, runs := range ds.Runs {
		totalRuns += len(runs)
	}

	fmt.Println(mockgithub.Usage(*addr))
	log.Printf("Serving %s dataset (%d repos, %d workflow runs) with max page size %d",
		*dataset, len(ds.Repos), totalRuns, *maxPageSize)

	if err := srv.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}
