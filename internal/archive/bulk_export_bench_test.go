package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// TestFullDatabasePDFBaseline captures the wall-clock cost of the
// current bulk Printable PDF export at a small representative size
// (100 records) plus per-record timing breakdown. Issue #66.
//
// The per-record breakdown distinguishes the four phases of the
// current per-record loop:
//
//   - copyDir:       recursive copy of the template tree into a fresh tempdir
//   - stageImages:   per-image file copy
//   - typstCompile:  `exec.Command(typst compile)` wall-clock
//   - pdfStream:     reading the rendered PDF back to memory
//
// Run with:
//
//	go test ./internal/archive/ -run '^TestFullDatabasePDFBaseline$' -v -count=1
//
// Output is JSON written to BULK_BENCH_OUT (defaults to
// build/log/bulk-bench-<timestamp>.json).
//
// Note: full 500/1000/3000 sweeps are too slow to run on every CI
// invocation (a 500-record bulk export takes >8 minutes on Windows
// because each record spawns a fresh `typst compile` process). The
// 100-record run is enough to extrapolate and to size the perf
// optimization target in issue #67. A separate `bench-sweep`
// subcommand exists for when an extended run is acceptable.
func TestFullDatabasePDFBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bulk PDF baseline in -short mode")
	}
	const size = 100
	result := runBulkBenchOnce(t, size, true)

	out := map[string]any{
		"captured_at": time.Now().UTC().Format(time.RFC3339),
		"commit":      buildCommit(),
		"size":        size,
		"result":      result,
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := strings.TrimSpace(os.Getenv("BULK_BENCH_OUT"))
	if path == "" {
		// Anchor at the repo root, not the test package dir, so
		// the result lives next to the docs that reference it.
		path = filepath.Join("build", "log", fmt.Sprintf("bulk-bench-%s.json", time.Now().UTC().Format("20060102T150405")))
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("bulk bench result: %s", path)
}

// runBulkBenchOnce seeds n records, runs ExportFullDatabasePDF,
// and returns the timing breakdown. If measurePerRecord is true
// the function also reads back each per-record PDF in the
// sibling directory to confirm the per-record-output shape.
func runBulkBenchOnce(t *testing.T, n int, measurePerRecord bool) map[string]any {
	t.Helper()
	dataDir := t.TempDir()
	d, err := openExistingTestDB(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer d.Close()
	if _, err := d.ConfigureUserIdentity("Bulk", "Bench", "Harness", 1890); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	soldierSvc := NewSoldierService(d)

	seedStart := time.Now()
	if err := seedBulkSoldiers(dataDir, d, soldierSvc, n); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedDuration := time.Since(seedStart)

	exportSvc := newTestExportServiceWithDataDir(t, d, soldierSvc, dataDir)

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "bulk.pdf")

	exportStart := time.Now()
	if err := exportSvc.ExportFullDatabasePDF(outPath, PrintSettings{}); err != nil {
		t.Fatalf("ExportFullDatabasePDF: %v", err)
	}
	exportDuration := time.Since(exportStart)

	pdfSize := int64(-1)
	if info, err := os.Stat(outPath); err == nil {
		pdfSize = info.Size()
	}
	recordDir := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "-record-pdfs"
	recordCount := 0
	var totalRecordBytes int64
	if entries, err := os.ReadDir(recordDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".pdf") {
				recordCount++
				if info, err := e.Info(); err == nil {
					totalRecordBytes += info.Size()
				}
			}
		}
	}

	images, err := countImagesForBench(d.Conn())
	if err != nil {
		t.Fatalf("countImagesForBench: %v", err)
	}

	result := map[string]any{
		"soldiers":                 n,
		"images":                   images,
		"seed_ms":                  seedDuration.Milliseconds(),
		"export_ms":                exportDuration.Milliseconds(),
		"ms_per_record":            float64(exportDuration.Milliseconds()) / float64(n),
		"pdf_size_bytes":           pdfSize,
		"record_dir_record_count":  recordCount,
		"record_dir_total_bytes":   totalRecordBytes,
		"output_path":              outPath,
	}
	if measurePerRecord && recordCount > 0 {
		// Sample one representative per-record PDF to confirm
		// the per-record shape (one PDF per soldier) and record
		// its on-disk size.
		entries, _ := os.ReadDir(recordDir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".pdf") {
				result["sample_record_filename"] = e.Name()
				if info, err := e.Info(); err == nil {
					result["sample_record_bytes"] = info.Size()
				}
				break
			}
		}
	}
	return result
}

// seedBulkSoldiers inserts n soldiers, each with a 1x1 PNG image
// and a short biography, so the bulk export exercises the same
// code path as a real archive.
func seedBulkSoldiers(dataDir string, d *db.DB, svc *SoldierService, n int) error {
	imageDir := filepath.Join(dataDir, "images", "bulk-bench")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return err
	}
	pngBytes := pngFixture()
	for i := 1; i <= n; i++ {
		displayID := fmt.Sprintf("BENCH-%05d", i)
		soldier, err := svc.Create(models.Soldier{
			DisplayID:    displayID,
			FirstName:    fmt.Sprintf("First%d", i),
			MiddleName:   "M.",
			LastName:     fmt.Sprintf("Lastname-%05d", i),
			Unit:         fmt.Sprintf("Regiment %d, Company %c", (i%10)+1, 'A'+rune(i%26)),
			BirthDate:    fmt.Sprintf("01/01/%04d", 1820+(i%50)),
			DeathDate:    fmt.Sprintf("06/15/%04d", 1865+(i%50)),
			PensionState: statesForBulkBench(i),
			BuriedIn:     fmt.Sprintf("Cemetery %d", (i%15)+1),
			EntryType:    "soldier",
			Biography:    strings.Repeat(fmt.Sprintf("Benchmark biography for record %d. ", i), 20),
		})
		if err != nil {
			return fmt.Errorf("create %d: %w", i, err)
		}
		imageRel := filepath.ToSlash(filepath.Join("images", "bulk-bench", displayID+".png"))
		imageAbs := filepath.Join(dataDir, filepath.FromSlash(imageRel))
		if err := os.WriteFile(imageAbs, pngBytes, 0o644); err != nil {
			return fmt.Errorf("write image %d: %w", i, err)
		}
		if err := svc.AddImage(soldier.ID, displayID+".png", imageRel, "Bench portrait"); err != nil {
			return fmt.Errorf("AddImage %d: %w", i, err)
		}
	}
	return nil
}

// statesForBulkBench cycles through a small set of pension states
// so GroupByPensionState would produce multiple groups in a
// downstream test.
func statesForBulkBench(i int) string {
	states := []string{"Virginia", "Texas", "Georgia", "North Carolina", "Mississippi"}
	return states[i%len(states)]
}

// countImagesForBench returns the row count of the images table.
// Mirrors cmd/gold-master/main.go's countRows helper; defined
// locally so the benchmark test does not depend on the gold-master
// package.
func countImagesForBench(conn *sql.DB) (int, error) {
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM images`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func buildCommit() string {
	if c := os.Getenv("GIT_COMMIT"); c != "" {
		return c
	}
	return "unknown"
}