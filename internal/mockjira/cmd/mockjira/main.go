package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"devctl-em/internal/mockjira"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dataset := flag.String("dataset", "realistic", "dataset to use: small, pagination, realistic")
	maxPageSize := flag.Int("max-page-size", 100, "maximum page size for pagination")
	flag.Parse()

	var ds *mockjira.Dataset
	switch *dataset {
	case "small":
		ds = mockjira.SmallDataset()
	case "pagination":
		ds = mockjira.PaginationDataset()
	case "realistic":
		ds = mockjira.RealisticDataset()
	default:
		fmt.Fprintf(os.Stderr, "unknown dataset: %s (options: small, pagination, realistic)\n", *dataset)
		os.Exit(1)
	}

	srv := mockjira.New(ds)
	srv.MaxPageSize = *maxPageSize

	fmt.Println(mockjira.Usage(*addr))
	log.Printf("Serving %s dataset (%d issues) with max page size %d", *dataset, len(ds.Issues), *maxPageSize)

	if err := srv.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}
