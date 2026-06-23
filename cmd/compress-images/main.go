// Command compress-images re-encodes stored images in a DixieData data
// directory to JPEG quality 85. Idempotent: skips images that already
// have a compressed_at timestamp. Run from the same machine that
// hosts the DixieData app. Issue #73.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/db"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("compress-images", flag.ContinueOnError)
	dataDir := fs.String("data-dir", os.Getenv("DIXIEDATA_DATA_DIR"), "path to DixieData data directory (the one containing dixiedata.db); default: DIXIEDATA_DATA_DIR env")
	dryRun := fs.Bool("dry-run", false, "discover candidates and print the plan, do not modify any files")
	limit := fs.Int("limit", 0, "process at most N images (0 = no limit)")
	format := fs.String("format", "human", "output format: human or json")
	if err := fs.Parse(args); err != nil {
		// ContinueOnError returns flag.ErrHelp on --help; treat that as
		// a normal usage path so main() exits 0 instead of 1.
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	dir, err := resolveDataDir(*dataDir)
	if err != nil {
		return err
	}
	database, err := db.Open(dir)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()
	compress := archive.NewCompressService(database)
	candidates, err := compress.DiscoverUncompressed(dir)
	if err != nil {
		return fmt.Errorf("discover: %w", err)
	}
	if *limit > 0 && len(candidates) > *limit {
		candidates = candidates[:*limit]
	}
	if *dryRun {
		printDryRun(*format, candidates)
		return nil
	}
	relPaths := make([]string, len(candidates))
	for i, c := range candidates {
		relPaths[i] = c.RelativePath
	}
	report, err := compress.CompressParallel(dir, relPaths, 4, func(r archive.CompressedResult, err error) {
		if err == nil {
			fmt.Printf("compressed %s: %d -> %d bytes\n", r.RelativePath, r.OriginalBytes, r.CompressedBytes)
		}
	})
	if err != nil {
		return fmt.Errorf("compress: %w", err)
	}
	printReport(*format, report)
	return nil
}

func resolveDataDir(given string) (string, error) {
	if given != "" {
		return given, nil
	}
	return "", errors.New("--data-dir or DIXIEDATA_DATA_DIR is required")
}

func printDryRun(format string, candidates []archive.CompressibleImage) {
	if format == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(candidates)
		return
	}
	fmt.Printf("Found %d uncompressed images.\n", len(candidates))
	for _, c := range candidates {
		fmt.Printf("  %s (%d bytes, %s)\n", c.RelativePath, c.Size, c.ModifiedAt)
	}
}

func printReport(format string, report archive.CompressionReport) {
	if format == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return
	}
	fmt.Printf("Compressed %d, skipped %d.\n", report.Compressed, report.Skipped)
	fmt.Printf("Bytes: %d -> %d (saved %d).\n", report.OriginalBytes, report.FinalBytes, report.OriginalBytes-report.FinalBytes)
	if len(report.Errors) > 0 {
		fmt.Printf("Errors:\n")
		for _, e := range report.Errors {
			fmt.Printf("  %s\n", e)
		}
	}
}
