package seed

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// TestGenerateStampsSeedProvenance is the RED-first regression
// for issue #377 slice 2: every row written by the seed CLI tool
// must carry CreatedByImportPath = "seed" so a "where did this
// row come from?" investigation can attribute the row to the
// bulk seed importer without grepping the call graph.
func TestGenerateStampsSeedProvenance(t *testing.T) {
	dataDir := t.TempDir()

	summary, err := Generate(Options{
		DataDir:  dataDir,
		Soldiers: 3,
		Seed:     99,
		Reset:    true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if summary.Soldiers != 3 {
		t.Fatalf("soldiers=%d want 3", summary.Soldiers)
	}

	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	rows, err := database.Conn().Query(`SELECT display_id, created_by_version, created_by_import_path FROM soldiers ORDER BY id`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var displayID, version, path string
		if err := rows.Scan(&displayID, &version, &path); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if path != "seed" {
			t.Errorf("soldier %s created_by_import_path = %q, want %q", displayID, path, "seed")
		}
		if version == "" {
			t.Errorf("soldier %s created_by_version empty; service should default to running binary version", displayID)
		}
		count++
	}
	if count != 3 {
		t.Errorf("row count = %d, want 3", count)
	}
}
