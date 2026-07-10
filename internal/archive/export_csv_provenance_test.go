package archive

// Issue #383 Slice 2 (CSV): the CSV export must carry a
// discoverable format-version stamp in the per-row metadata
// block (the first row, which doubles as the header).
//
// The stamp lands in a new column `format_version` between
// `export_version` and `generated_at`. External readers
// (Excel, Sheets, Numbers) see it as just another header
// column, but a DixieData re-import can detect "this CSV
// was written by a DixieData that understood format_version
// csv_v1" vs "this CSV was written by a pre-#383 DixieData
// and has no format_version column".
//
// The RED-first regression net here proves:
//   1. The header row carries `format_version` as a column
//      between `export_version` and `generated_at`.
//   2. Every per-soldier row populates format_version with
//      the constant from buildinfo.CSVFormatVersion.
//   3. The CSV starts with the per-row metadata block
//      (app_version, schema_version, export_version,
//      format_version, generated_at) so a re-import can
//      detect all four version fields.

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

func TestExportCSVStampsFormatVersionInMetadataRow(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)
	exportSvc := newTestExportServiceWithRegistry(t, d, svc)

	if _, err := svc.Create(models.Soldier{
		FirstName: "CSV",
		LastName:  "Stamp",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	outputPath := filepath.Join(testtemp.New(t).Path(), "stamp.csv")
	if err := exportSvc.ExportCSV(outputPath); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}

	f, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("CSV has %d rows; want at least 2 (header + at least 1 soldier)", len(rows))
	}
	header := rows[0]
	wantColumn := "format_version"
	col := indexOf(header, wantColumn)
	if col < 0 {
		t.Fatalf("header %v does not contain %q column", header, wantColumn)
	}
	// Column must sit between export_version and generated_at
	// so the version block is contiguous.
	evCol := indexOf(header, "export_version")
	gaCol := indexOf(header, "generated_at")
	if !(evCol < col && col < gaCol) {
		t.Errorf("format_version column at %d, but expected between export_version (%d) and generated_at (%d)",
			col, evCol, gaCol)
	}

	// Every per-soldier row must stamp format_version with
	// the buildinfo constant.
	for i, row := range rows[1:] {
		if len(row) <= col {
			t.Errorf("row %d has %d columns, expected at least %d for format_version", i, len(row), col+1)
			continue
		}
		if got := strings.TrimSpace(row[col]); got != buildinfo.CSVFormatVersion {
			t.Errorf("row %d format_version = %q, want %q", i, got, buildinfo.CSVFormatVersion)
		}
	}
}

func indexOf(haystack []string, needle string) int {
	for i, s := range haystack {
		if s == needle {
			return i
		}
	}
	return -1
}
