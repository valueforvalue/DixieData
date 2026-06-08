package records

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestMemorialImportPreviewAndImport(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	filePath := filepath.Join(t.TempDir(), "memorials.json")
	if err := os.WriteFile(filePath, []byte(`[
		{
			"memorial_id":"M-100",
			"name":"Ella Lou Wade Morris",
			"url":"https://www.findagrave.com/memorial/M-100/example",
			"birth_date":"21 Jan 1930",
			"birth_location":"Ringling, Oklahoma, USA",
			"death_date":"18 Sep 2025",
			"death_location":"Oklahoma, USA",
			"burial_cemetery":"Ringling Memorial Cemetery",
			"burial_location":"Ringling, Oklahoma, USA",
			"biography":"Example memorial biography",
			"family_parents":["Charlie Wade","Ara Wade"],
			"family_spouse":"James Morgan Morris",
			"family_children":["Child One","Child Two"],
			"scraped_at":"2026-06-08T20:02:16.191Z"
		},
		{
			"memorial_id":"M-100",
			"name":"Duplicate Entry",
			"url":"https://www.findagrave.com/memorial/M-100/example-duplicate"
		},
		{
			"memorial_id":"",
			"name":"Missing Memorial Id"
		}
	]`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	preview, err := svc.PreviewMemorialArchive(filePath)
	if err != nil {
		t.Fatalf("PreviewMemorialArchive: %v", err)
	}
	if preview.TotalRows != 3 || preview.WouldCreate != 1 || preview.WouldSkip != 1 || preview.WouldFail != 1 {
		t.Fatalf("unexpected preview summary: %#v", preview)
	}
	if len(preview.Issues) != 1 || !strings.Contains(preview.Issues[0].Error, "memorial_id is required") {
		t.Fatalf("preview issues = %#v", preview.Issues)
	}

	summary, err := svc.ImportMemorialArchive(filePath)
	if err != nil {
		t.Fatalf("ImportMemorialArchive: %v", err)
	}
	if summary.BatchID == "" {
		t.Fatal("expected batch id")
	}
	if summary.TotalRows != 3 || summary.Created != 1 || summary.Skipped != 1 || summary.Failed != 1 {
		t.Fatalf("unexpected import summary: %#v", summary)
	}

	lastImportRows, total, _, err := svc.BrowsePage(BrowseRequest{Scope: BrowseScopeLastImport, PageSize: 50})
	if err != nil {
		t.Fatalf("BrowsePage last import: %v", err)
	}
	if total != 1 || len(lastImportRows) != 1 {
		t.Fatalf("last import rows total=%d len=%d", total, len(lastImportRows))
	}
	imported := lastImportRows[0]
	if imported.FirstName != "Ella" || imported.LastName != "Morris" {
		t.Fatalf("unexpected imported name: %#v", imported)
	}
	if imported.BirthDate != "01/21/1930" || imported.DeathDate != "09/18/2025" {
		t.Fatalf("unexpected canonical dates: birth=%q death=%q", imported.BirthDate, imported.DeathDate)
	}
	if imported.BirthInfo != "Ringling, Oklahoma, USA" {
		t.Fatalf("BirthInfo = %q", imported.BirthInfo)
	}
	if !strings.Contains(imported.Notes, "Imported via Memorial JSON") || !strings.Contains(imported.Notes, "Family Parents: Charlie Wade; Ara Wade") {
		t.Fatalf("Notes = %q", imported.Notes)
	}
	detail, err := svc.GetByID(imported.ID)
	if err != nil {
		t.Fatalf("GetByID imported: %v", err)
	}
	if len(detail.Records) != 1 || detail.Records[0].RecordType != memorialRecordType || detail.Records[0].AppID != "M-100" {
		t.Fatalf("records = %#v", detail.Records)
	}
}

func TestMemorialImportSkipsExistingMemorialID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	existing, err := svc.Create(models.Soldier{
		FirstName: "Existing",
		LastName:  "Record",
		Records: []models.Record{{
			RecordType: memorialRecordType,
			AppID:      "M-200",
			Details:    "https://www.findagrave.com/memorial/M-200/existing",
		}},
	})
	if err != nil {
		t.Fatalf("Create existing: %v", err)
	}
	if existing.ID < 1 {
		t.Fatalf("invalid existing id: %d", existing.ID)
	}

	filePath := filepath.Join(t.TempDir(), "memorials-existing.json")
	if err := os.WriteFile(filePath, []byte(`[
		{
			"memorial_id":"M-200",
			"name":"Should Be Skipped",
			"url":"https://www.findagrave.com/memorial/M-200/new"
		}
	]`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	summary, err := svc.ImportMemorialArchive(filePath)
	if err != nil {
		t.Fatalf("ImportMemorialArchive: %v", err)
	}
	if summary.Created != 0 || summary.Skipped != 1 || summary.Failed != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}
