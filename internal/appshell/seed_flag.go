// seed_flag.go — headless fixture seeder for DixieData.
//
// Run via `dixiedata --seed [flags]` (CLI flag) or
// `DIXIEDATA_SEED=1 dixiedata [flags]` (env var). Returns exit
// code:
//
//	0 — seed completed
//	1 — seed failed (data dir unwritable, db open failed, etc.)
//
// Why this exists alongside the standalone `cmd/seed-data`
// binary: the in-app CLI is the most discoverable surface for
// QA + RC1 cohort users who want to populate a fresh DB
// without installing Go. The standalone binary remains for
// scripted CI + the `go test` fixture path; both call
// `internal/seed.Generate` so the output is identical.
//
// Future work (issue #667): promote to a `dixiedata seed`
// positional subcommand. For now the flag form keeps the
// dispatcher order simple — same shape as `--smoke`.
package appshell

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/seed"
)

// SeedOptions is the resolved options for a `--seed` run.
// Mirrors cmd/seed-data's flag surface 1:1 so the standalone
// binary + the in-app flag produce identical output.
type SeedOptions struct {
	DataDir         string
	Soldiers        int
	Seed            int64
	Reset           bool
	SkipSoldiers    bool
	Tags            int
	Articles        int
	ArticlesFormat  string // "plain" | "markdown"
	Events          int
	JSON            bool   // print a JSON summary instead of text
}

// HasSeedFlag returns true when the os.Args slice contains
// `--seed` or `--seed-json`. main.go uses this to decide
// whether to enter the seed path or call wails.Run.
func HasSeedFlag(args []string) bool {
	for _, a := range args {
		if a == "--seed" || a == "--seed-json" {
			return true
		}
	}
	return false
}

// EnvRequestsSeed returns true when DIXIEDATA_SEED=1 is set.
// Used by main.go so a launcher script can request seed mode
// without rebuilding the arg vector.
func EnvRequestsSeed() bool {
	v := strings.TrimSpace(os.Getenv("DIXIEDATA_SEED"))
	return v == "1" || strings.EqualFold(v, "true")
}

// WantsSeedJSON returns true when --seed-json was passed.
func WantsSeedJSON(args []string) bool {
	for _, a := range args {
		if a == "--seed-json" {
			return true
		}
	}
	return false
}

// ParseSeedOptions walks os.Args looking for `--seed` or
// `--seed-json`, then parses the trailing args as flag-style
// key/value pairs. Recognized flags:
//
//	--data-dir <path>     target archive directory (default: appdata.DefaultDir())
//	--soldiers <int>      number of soldiers (default 250; 0 = skip via --skip-soldiers)
//	--seed <int>          deterministic RNG seed (default 1865)
//	--reset               wipe existing DB + images before seeding
//	--skip-soldiers       skip the soldier loop; add only Tags/Events/Articles
//	--tags <int>          number of tags (0 = full vocabulary, ~30)
//	--articles <int>      number of article rows (0 = legacy default of 1-2)
//	--articles-format <s> "plain" | "markdown" (default markdown — showcases the
//	                      goldmark render path; the fixture exists to exercise it)
//	--events <int>        number of event records (0 = ~20% of soldier count)
//
// Unknown flags are ignored. A bare `--seed` with no flags
// runs with the defaults (250 soldiers, full v58-v65 surface,
// markdown articles, reset OFF).
func ParseSeedOptions(args []string) SeedOptions {
	opts := SeedOptions{
		DataDir:        appdata.DefaultDir(),
		Soldiers:       250,
		Seed:           1865,
		ArticlesFormat: "markdown",
	}
	// Walk args looking for flag-style key/value pairs that
	// follow the first occurrence of --seed / --seed-json.
	// We allow any ordering: `dixiedata --seed --reset --soldiers 100`
	// and `dixiedata --soldiers 100 --seed --reset` both work.
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--seed", "--seed-json":
			i++
			continue
		case "--data-dir":
			if i+1 < len(args) {
				opts.DataDir = args[i+1]
				i += 2
				continue
			}
		case "--soldiers":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					opts.Soldiers = n
				}
				i += 2
				continue
			}
		case "--seed-value", "--rng-seed":
			// Reserved alias: avoid colliding with the --seed
			// trigger flag. Real seed value is --rng-seed.
			if i+1 < len(args) {
				if n, err := strconv.ParseInt(args[i+1], 10, 64); err == nil {
					opts.Seed = n
				}
				i += 2
				continue
			}
		case "--reset":
			opts.Reset = true
			i++
			continue
		case "--skip-soldiers":
			opts.SkipSoldiers = true
			i++
			continue
		case "--tags":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					opts.Tags = n
				}
				i += 2
				continue
			}
		case "--articles":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					opts.Articles = n
				}
				i += 2
				continue
			}
		case "--articles-format":
			if i+1 < len(args) {
				switch strings.ToLower(args[i+1]) {
				case "plain":
					opts.ArticlesFormat = "plain"
				case "markdown":
					opts.ArticlesFormat = "markdown"
				}
				i += 2
				continue
			}
		case "--events":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					opts.Events = n
				}
				i += 2
				continue
			}
		}
		i++
	}
	return opts
}

// RunSeed is the entry point main.go calls when --seed is
// detected. It translates SeedOptions into the
// internal/seed.Options struct + invokes the shared
// Generate function. Returns an exit code (0 success, 1
// failure) and prints a summary in either text or JSON
// depending on opts.JSON.
//
// The signal-aware context from main.go lets the seed run
// bail cleanly on Ctrl+C — same pattern as RunSmoke.
func RunSeed(ctx context.Context, opts SeedOptions) (string, int) {
	seedOpts := seed.Options{
		DataDir:      opts.DataDir,
		Soldiers:     opts.Soldiers,
		Seed:         opts.Seed,
		Reset:        opts.Reset,
		SkipSoldiers: opts.SkipSoldiers,
		Tags:         opts.Tags,
		Articles:     opts.Articles,
		Events:       opts.Events,
	}
	switch opts.ArticlesFormat {
	case "plain":
		seedOpts.ArticlesFormat = seed.ArticleBodyPlain
	case "markdown", "":
		// Default: markdown. The article fixture exists to
		// showcase the goldmark render path (issue #523); the
		// plain path is for users who want to see the legacy
		// <p>-wrapped prose form.
		seedOpts.ArticlesFormat = seed.ArticleBodyMarkdown
	}

	summary, err := seed.Generate(seedOpts)
	if err != nil {
		msg := fmt.Sprintf("seed failed: %v", err)
		fmt.Fprintln(os.Stderr, msg)
		return msg, 1
	}

	if opts.JSON {
		// Minimal JSON summary so CI scripts can parse without
		// pulling in a YAML/JSON library.
		fmt.Printf(`{"data_dir":%q,"db":%q,"images":%q,"soldiers":%d,"records":%d,"images_count":%d,"tags":%d,"events":%d,"articles":%d,"person_record_tags":%d,"event_person_links":%d,"event_sources":%d,"article_refs":%d}`+"\n",
			summary.DataDir, summary.DBPath, summary.ImageDir,
			summary.Soldiers, summary.Records, summary.Images,
			summary.Tags, summary.Events, summary.Articles,
			summary.PersonRecordTags, summary.EventLinks, summary.EventSources, summary.ArticleRefs)
	} else {
		fmt.Printf("Seeded %d soldiers, %d records, and %d images into %s\n", summary.Soldiers, summary.Records, summary.Images, summary.DataDir)
		fmt.Printf("Database: %s\n", summary.DBPath)
		fmt.Printf("Images: %s\n", summary.ImageDir)
		if summary.Tags > 0 || summary.Events > 0 || summary.Articles > 0 {
			fmt.Printf("v58-v65 surface: %d tags, %d events, %d articles\n", summary.Tags, summary.Events, summary.Articles)
			fmt.Printf("  junctions: %d person_record_tags, %d event_person_links, %d event_sources, %d article_refs\n",
				summary.PersonRecordTags, summary.EventLinks, summary.EventSources, summary.ArticleRefs)
		}
	}
	return "", 0
}

// ensure flag is imported; the standalone cmd/seed-data binary
// uses the stdlib flag package but the in-app flag form parses
// manually so we can interleave the --seed trigger with the
// flag values in any order. The `flag` import here keeps the
// package compilable if we later swap to stdlib flag.Parse
// with a hidden sentinel.
var _ = flag.ErrHelp
