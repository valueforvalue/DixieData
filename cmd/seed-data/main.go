// Command seed-data populates a DixieData directory with deterministic
// sample soldiers, records, images, and (on v58+ schemas) Event
// Records, Articles, and Tags. Useful for fresh-install smoke tests,
// fixture generation for the audit harness, and ad-hoc Local Archive
// surface verification.
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
	flag.IntVar(&options.Soldiers, "soldiers", 250, "Number of generated soldiers (ignored when --skip-soldiers is set)")
	flag.Int64Var(&options.Seed, "seed", 1865, "Deterministic random seed")
	flag.BoolVar(&options.Reset, "reset", false, "Remove the existing database and generated image directory before seeding")
	flag.BoolVar(&options.SkipSoldiers, "skip-soldiers", false, "Skip the soldier creation loop; use when the target archive already has soldiers and you only want to seed the v58-v65 entity surface (Tags / Events / Articles)")
	flag.IntVar(&options.Tags, "tags", 0, "Number of tags to seed from the vocabulary (0 = full vocabulary, currently 30)")
	flag.IntVar(&options.Articles, "articles", 0, "Number of Article rows to seed (0 = legacy default of 1-2 random)")
	flag.IntVar(&options.Events, "events", 0, "Number of Event Record rows to seed (0 = legacy default of ~20% of soldier count)")
	flag.Parse()

	summary, err := seed.Generate(options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Seeded %d soldiers, %d records, and %d images into %s\n", summary.Soldiers, summary.Records, summary.Images, summary.DataDir)
	fmt.Printf("Database: %s\n", summary.DBPath)
	fmt.Printf("Images: %s\n", summary.ImageDir)
	if summary.Tags > 0 || summary.Events > 0 || summary.Articles > 0 {
		fmt.Printf("v58-v65 surface: %d tags, %d events, %d articles\n", summary.Tags, summary.Events, summary.Articles)
		fmt.Printf("  junctions: %d person_record_tags, %d event_person_links, %d event_sources, %d article_refs\n",
			summary.PersonRecordTags, summary.EventLinks, summary.EventSources, summary.ArticleRefs)
	}
}

func defaultDataDir() string {
	return appdata.DefaultDir()
}