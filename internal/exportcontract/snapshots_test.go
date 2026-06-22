package exportcontract

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/pkg/exportbridge"
	"github.com/valueforvalue/DixieData/pkg/render"
)

// snapshotCase describes one PDF we want to capture and compare
// against a snapshot file.
type snapshotCase struct {
	name        string                 // used as the snapshot filename without extension
	template    string                 // typst template name
	orientation string                 // "L" or "P"
	mode        string                 // "record" or "bulk"
	recordID    int64                  // for record mode
	settings    func() render.PrintSettings // for bulk mode
}

// runSnapshotCase executes a snapshot case: builds the renderer
// against the fixture, calls the appropriate bridge entry point,
// returns the rendered PDF bytes.
func runSnapshotCase(t *testing.T, fixtureDir, typstPath, templatesDir string, c snapshotCase) []byte {
	t.Helper()
	r, err := exportbridge.NewBulkRenderer(fixtureDir, fixtureDir)
	if err != nil {
		t.Fatalf("NewBulkRenderer: %v", err)
	}
	defer r.Close()
	typst := render.NewTypstRenderer(typstPath, filepath.Dir(templatesDir))
	reg := render.NewRegistry(typst, templatesDir)
	r.SetRegistry(reg)

	ctx := context.Background()
	switch c.mode {
	case "record":
		soldier, err := r.GetByID(c.recordID)
		if err != nil {
			t.Fatalf("GetByID(%d): %v", c.recordID, err)
		}
		opts := render.PDFOptions{
			Orientation:     c.orientation,
			PrinterFriendly: true,
			IncludeImages:   true,
		}
		var buf bytes.Buffer
		if err := r.RenderSingle(ctx, *soldier, opts, nopWriteCloser{&buf}); err != nil {
			t.Fatalf("RenderSingle: %v", err)
		}
		return buf.Bytes()
	case "bulk":
		var buf bytes.Buffer
		settings := c.settings()
		if _, err := r.RenderBulk(ctx, settings, nopWriteCloser{&buf}); err != nil {
			t.Fatalf("RenderBulk: %v", err)
		}
		return buf.Bytes()
	}
	t.Fatalf("unknown mode %q", c.mode)
	return nil
}

// nopWriteCloser wraps a bytes.Buffer so it satisfies io.WriteCloser.
// The bridge detects WriteCloser and would otherwise try to call
// .Name() on it.
type nopWriteCloser struct{ *bytes.Buffer }

func (nopWriteCloser) Close() error { return nil }

// compareOrUpdate compares got to the snapshot file. When
// UPDATE_SNAPSHOTS=1, writes the new content. Returns the path to
// the snapshot file either way.
func compareOrUpdate(t *testing.T, snapshotPath string, got []byte) {
	t.Helper()
	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		if err := os.WriteFile(snapshotPath, got, 0o644); err != nil {
			t.Fatalf("write snapshot %q: %v", snapshotPath, err)
		}
		t.Logf("snapshot updated: %s (%d bytes)", snapshotPath, len(got))
		return
	}
	want, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot %q: %v (run with UPDATE_SNAPSHOTS=1 to create)", snapshotPath, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("snapshot mismatch for %s\n  want: %d bytes\n  got:  %d bytes\nRun with UPDATE_SNAPSHOTS=1 to update.",
			snapshotPath, len(want), len(got))
	}
}

// snapshotCases lists every snapshot the contract tests produce.
// Each (template, orientation, mode) tuple renders once.
func snapshotCases(t *testing.T) []snapshotCase {
	return []snapshotCase{
		{
			name:        "soldier-landscape",
			template:    "soldier_landscape",
			orientation: "L",
			mode:        "record",
			recordID:    1,
		},
		{
			name:        "soldier-portrait",
			template:    "soldier_portrait",
			orientation: "P",
			mode:        "record",
			recordID:    1,
		},
		{
			name:        "widow-landscape",
			template:    "widow_landscape",
			orientation: "L",
			mode:        "record",
			recordID:    2,
		},
		{
			name:        "widow-portrait",
			template:    "widow_portrait",
			orientation: "P",
			mode:        "record",
			recordID:    2,
		},
		{
			name:        "wife-landscape",
			template:    "spouse_landscape",
			orientation: "L",
			mode:        "record",
			recordID:    3,
		},
		{
			name:        "wife-portrait",
			template:    "spouse_portrait",
			orientation: "P",
			mode:        "record",
			recordID:    3,
		},
		{
			name:        "linked-person-landscape",
			template:    "spouse_landscape",
			orientation: "L",
			mode:        "record",
			recordID:    4,
		},
		{
			name:        "linked-person-portrait",
			template:    "spouse_portrait",
			orientation: "P",
			mode:        "record",
			recordID:    4,
		},
		{
			name:        "bulk-landscape",
			template:    "bulk_soldier",
			orientation: "L",
			mode:        "bulk",
			settings: func() render.PrintSettings {
				return render.PrintSettings{
					Orientation: "L",
					SortBy:      archive.PrintSortLastName,
				}.Normalize()
			},
		},
		{
			name:        "bulk-portrait",
			template:    "bulk_soldier",
			orientation: "P",
			mode:        "bulk",
			settings: func() render.PrintSettings {
				return render.PrintSettings{
					Orientation: "P",
					SortBy:      archive.PrintSortLastName,
				}.Normalize()
			},
		},
		{
			name:        "grouped-by-pension-state",
			template:    "bulk_soldier",
			orientation: "L",
			mode:        "bulk",
			settings: func() render.PrintSettings {
				return render.PrintSettings{
					Orientation:        "L",
					SortBy:             archive.PrintSortLastName,
					GroupByPensionState: true,
				}.Normalize()
			},
		},
	}
}

// TestArchiveContractSnapshots pins the byte-identical PDF
// output of internal/archive.ExportService (called via the bridge)
// against snapshots on disk. Run from this package directly.
func TestArchiveContractSnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping contract snapshots in -short mode")
	}
	fixtureDir := FixturePath(t)
	typstPath := mustFindTypstBinary(t)
	templatesDir := mustFindTemplatesDir(t)

	for _, c := range snapshotCases(t) {
		t.Run(c.name, func(t *testing.T) {
			got := runSnapshotCase(t, fixtureDir, typstPath, templatesDir, c)
			snapshotPath := filepath.Join("testdata", "snapshots", c.name+".pdf")
			compareOrUpdate(t, snapshotPath, got)
		})
	}
}

// mustFindTypstBinary walks up from the cwd to find bin/typst-*.
func mustFindTypstBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{"typst-windows.exe", "typst-macos", "typst-linux"}
	dir, _ := os.Getwd()
	for i := 0; i < 8; i++ {
		for _, name := range candidates {
			candidate := filepath.Join(dir, "bin", name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("no typst binary found; skipping contract snapshots")
	return ""
}

// mustFindTemplatesDir walks up from the cwd to find templates/.
func mustFindTemplatesDir(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "templates")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(candidate, "soldier_landscape.typ")); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("no templates/ directory found; skipping contract snapshots")
	return ""
}