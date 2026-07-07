package records

import (
	"errors"
	"fmt"
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

// writeMemorialEnvelope writes a Memorial Archive file in the
// v1 envelope shape (issue #383 Slice 1). Used by the
// format-version tests below. The scriptVersion / scriptName
// fields round-trip into MemorialImportFormat so the import
// summary surfaces them in the UI / CLI --json output.
func writeMemorialEnvelope(t *testing.T, dir, filename, formatVersion, scriptVersion, scriptName string, body string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	payload := fmt.Sprintf(`{"format_version":%q,"script_version":%q,"script_name":%q,"entries":%s}`,
		formatVersion, scriptVersion, scriptName, body)
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestMemorialImportEnvelopeV1_Succeeds(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	body := `[
		{"memorial_id":"M-V1-1","name":"Vee One Soldier","url":"https://www.findagrave.com/memorial/1/","biography":"v1 envelope test"}
	]`
	path := writeMemorialEnvelope(t, t.TempDir(), "memorials_v1.json", "memorial_v1", "1.0", "Find A Grave Ambient Scraper", body)

	summary, err := svc.ImportMemorialArchive(path)
	if err != nil {
		t.Fatalf("ImportMemorialArchive: %v", err)
	}
	if summary.Format.FormatVersion != "memorial_v1" {
		t.Errorf("Format.FormatVersion = %q, want memorial_v1", summary.Format.FormatVersion)
	}
	if summary.Format.ScriptVersion != "1.0" {
		t.Errorf("Format.ScriptVersion = %q, want 1.0", summary.Format.ScriptVersion)
	}
	if summary.Format.ScriptName != "Find A Grave Ambient Scraper" {
		t.Errorf("Format.ScriptName = %q, want Find A Grave Ambient Scraper", summary.Format.ScriptName)
	}
	if summary.ImportedByAppVersion == "" {
		t.Error("ImportedByAppVersion should be populated from buildinfo.AppVersion")
	}
	if summary.Created != 1 || summary.Failed != 0 {
		t.Errorf("Created=%d Failed=%d, want 1/0", summary.Created, summary.Failed)
	}
}

func TestMemorialImportMajorBump_RefusesWithTypedError(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	body := `[{"memorial_id":"M-FUTURE","name":"Future Soldier","url":"https://www.findagrave.com/memorial/999/","biography":"v2 from the future"}]`
	path := writeMemorialEnvelope(t, t.TempDir(), "memorials_v2.json", "memorial_v2", "2.0", "Find A Grave Ambient Scraper", body)

	_, err := svc.ImportMemorialArchive(path)
	if err == nil {
		t.Fatal("expected ErrMemorialFormatMismatch on major bump, got nil")
	}
	if !errors.Is(err, ErrMemorialFormatMismatch) {
		t.Fatalf("err = %v, want errors.Is(err, ErrMemorialFormatMismatch)", err)
	}
	if !strings.Contains(err.Error(), "memorial_v2") {
		t.Errorf("err message = %q, want to mention memorial_v2", err.Error())
	}
}

func TestMemorialImportMinorBump_WarnsButImports(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	body := `[{"memorial_id":"M-MINOR","name":"Minor Bump","url":"https://www.findagrave.com/memorial/123/","biography":"minor bump test"}]`
	path := writeMemorialEnvelope(t, t.TempDir(), "memorials_v1.1.json", "memorial_v1.1", "1.1", "Find A Grave Ambient Scraper", body)

	summary, err := svc.ImportMemorialArchive(path)
	if err != nil {
		t.Fatalf("minor bump should warn but import, got err: %v", err)
	}
	if len(summary.Warnings) == 0 {
		t.Errorf("expected at least one warning on minor bump, got 0")
	}
	if summary.Created != 1 {
		t.Errorf("Created=%d, want 1 (minor bump should still import)", summary.Created)
	}
}

func TestMemorialImportPreV1_BareArrayStillImports(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	// Bare-array fixture — the pre-v1 shape. Existing TestMemorialImportPreviewAndImport
	// already pins this works; this test additionally asserts the
	// new format fields surface the warning so the user knows
	// their archive is unversioned.
	path := filepath.Join(t.TempDir(), "memorials_legacy.json")
	if err := os.WriteFile(path, []byte(`[
		{"memorial_id":"M-LEGACY","name":"Legacy Soldier","url":"https://www.findagrave.com/memorial/42/","biography":"pre-v1 archive"}
	]`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	preview, err := svc.PreviewMemorialArchive(path)
	if err != nil {
		t.Fatalf("PreviewMemorialArchive: %v", err)
	}
	if preview.Format.FormatVersion != "" {
		t.Errorf("Format.FormatVersion = %q, want empty for bare-array pre-v1", preview.Format.FormatVersion)
	}
	if len(preview.Warnings) == 0 {
		t.Fatal("expected pre-v1 warning, got none")
	}
	if !strings.Contains(preview.Warnings[0], "format_version") {
		t.Errorf("warning = %q, want it to mention format_version", preview.Warnings[0])
	}
}

func TestMemorialImportRoundTrip_EnvelopeFieldsReadBack(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	body := `[
		{"memorial_id":"M-RT-1","name":"Round Trip One","url":"https://www.findagrave.com/memorial/1001/","biography":"round-trip test 1"},
		{"memorial_id":"M-RT-2","name":"Round Trip Two","url":"https://www.findagrave.com/memorial/1002/","biography":"round-trip test 2"}
	]`
	path := writeMemorialEnvelope(t, t.TempDir(), "memorials_rt.json", "memorial_v1", "1.0", "Find A Grave Ambient Scraper", body)

	preview, err := svc.PreviewMemorialArchive(path)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Format.FormatVersion != "memorial_v1" {
		t.Errorf("preview Format.FormatVersion = %q", preview.Format.FormatVersion)
	}
	if preview.TotalRows != 2 {
		t.Errorf("preview TotalRows = %d, want 2", preview.TotalRows)
	}

	summary, err := svc.ImportMemorialArchive(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if summary.Created != 2 {
		t.Errorf("summary Created = %d, want 2", summary.Created)
	}
	if summary.ImportedByAppVersion == "" {
		t.Error("summary ImportedByAppVersion should be populated")
	}
}
