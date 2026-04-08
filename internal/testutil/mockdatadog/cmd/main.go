package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"em/internal/testutil/mockdatadog"
)

func main() {
	addr := flag.String("addr", ":8083", "listen address")
	dataset := flag.String("dataset", "realistic", "dataset: small, realistic, or path to a Datadog SLO CSV")
	flag.Parse()

	var (
		ds  *mockdatadog.Dataset
		err error
	)

	switch *dataset {
	case "small":
		ds = mockdatadog.SmallDataset()
	case "realistic":
		ds = mockdatadog.RealisticDataset()
	default:
		// Treat as a CSV file path
		ds, err = mockdatadog.LoadFromSLOCSV(*dataset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading CSV %s: %v\n", *dataset, err)
			os.Exit(1)
		}
	}

	srv := mockdatadog.New(ds)

	violatedCount := 0
	for _, slo := range ds.SLOs {
		if h, ok := ds.SLOHistory[slo.ID]; ok && len(slo.Thresholds) > 0 && h.SLIValue < slo.Thresholds[0].Target {
			violatedCount++
		}
	}

	fmt.Println(mockdatadog.Usage(*addr))
	log.Printf("Serving %s dataset (%d monitors, %d SLOs, %d violated)",
		*dataset, len(ds.Monitors), len(ds.SLOs), violatedCount)

	if err := srv.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}
