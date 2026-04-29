package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/seed"
)

func main() {
	options := seed.Options{}

	flag.StringVar(&options.DataDir, "data-dir", defaultDataDir(), "Path to the DixieData app data directory")
	flag.IntVar(&options.Soldiers, "soldiers", 250, "Number of generated soldiers")
	flag.Int64Var(&options.Seed, "seed", 1865, "Deterministic random seed")
	flag.BoolVar(&options.Reset, "reset", false, "Remove the existing database and generated image directory before seeding")
	flag.Parse()

	summary, err := seed.Generate(options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Seeded %d soldiers, %d records, and %d images into %s\n", summary.Soldiers, summary.Records, summary.Images, summary.DataDir)
	fmt.Printf("Database: %s\n", summary.DBPath)
	fmt.Printf("Images: %s\n", summary.ImageDir)
}

func defaultDataDir() string {
	return appdata.DefaultDir()
}
