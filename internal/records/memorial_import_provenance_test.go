package records

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMemorialImportStampsImportPath is the RED-first regression
// for issue #377 slice 2: every row written by ImportMemorialArchive
// must carry CreatedByImportPath = "memorial_json_import" so a
// "where did this row come from?" investigation can attribute the
// row to the Memorial JSON import path without grepping the call
// graph.
func TestMemorialImportStampsImportPath(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	filePath := filepath.Join(t.TempDir(), "memorials-provenance.json")
	if err := os.WriteFile(filePath, []byte(`[
		{
			"memorial_id":"M-PROV-1",
			"name":"Provenance Target",
			"url":"https://www.findagrave.com/memorial/M-PROV-1/example"
		}
	]`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	summary, err := svc.ImportMemorialArchive(filePath)
	if err != nil {
		t.Fatalf("ImportMemorialArchive: %v", err)
	}
	if summary.Created != 1 {
		t.Fatalf("Created = %d, want 1", summary.Created)
	}

	rows, total, _, err := svc.BrowsePage(BrowseRequest{Scope: BrowseScopeLastImport, PageSize: 50})
	if err != nil {
		t.Fatalf("BrowsePage: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("rows total=%d len=%d, want 1/1", total, len(rows))
	}
	if rows[0].CreatedByImportPath != "memorial_json_import" {
		t.Errorf("CreatedByImportPath = %q, want %q", rows[0].CreatedByImportPath, "memorial_json_import")
	}

	detail, err := svc.GetByID(rows[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if detail.CreatedByImportPath != "memorial_json_import" {
		t.Errorf("round-trip CreatedByImportPath = %q, want %q", detail.CreatedByImportPath, "memorial_json_import")
	}
	if detail.CreatedByVersion == "" {
		t.Error("CreatedByVersion empty; service should default to running binary version")
	}
}
