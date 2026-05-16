package services

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/xuri/excelize/v2"
)

func configureExportIdentity(t *testing.T, database *db.DB) {
	t.Helper()
	if _, err := database.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
}

func TestExportService_ExportJSON(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	_, _ = soldierSvc.Create(models.Soldier{
		FirstName:        "Robert",
		LastName:         "Lee",
		Rank:             "General",
		Prefix:           "Gen.",
		Suffix:           "Sr.",
		AddedBy:          "STC38",
		LastEditedBy:     "MDC42",
		LastEditedAt:     "2026-05-16 12:00:00",
		LastEditedFields: "prefix,last_edited_by",
	})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "Stonewall", LastName: "Jackson", DisplayID: "PENSION-001"})

	outPath := filepath.Join(t.TempDir(), "export.json")
	if err := exportSvc.ExportJSON(outPath); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open exported file: %v", err)
	}
	defer f.Close()

	data, _ := io.ReadAll(f)
	var doc JSONExportDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}
	if doc.Metadata.AppVersion != buildinfo.AppVersion || doc.Metadata.SchemaVersion != buildinfo.SchemaVersion {
		t.Fatalf("unexpected metadata: %#v", doc.Metadata)
	}
	if len(doc.Soldiers) != 2 {
		t.Errorf("exported %d soldiers, want 2", len(doc.Soldiers))
	}
	if !strings.Contains(string(data), `"prefix"`) || !strings.Contains(string(data), `"suffix"`) || !strings.Contains(string(data), `"added_by"`) || !strings.Contains(string(data), `"last_edited_by"`) || !strings.Contains(string(data), `"last_edited_at"`) {
		t.Fatalf("JSON export missing audited field keys: %s", string(data))
	}
	var audited *models.Soldier
	for index := range doc.Soldiers {
		if doc.Soldiers[index].FirstName == "Robert" && doc.Soldiers[index].LastName == "Lee" {
			audited = &doc.Soldiers[index]
			break
		}
	}
	if audited == nil {
		t.Fatalf("JSON export missing audited soldier: %#v", doc.Soldiers)
	}
	if audited.Prefix != "Gen." || audited.Suffix != "Sr." || audited.AddedBy != "STC38" || audited.LastEditedBy != "MDC42" || audited.LastEditedAt != "2026-05-16T12:00:00Z" {
		t.Fatalf("JSON export missing audited field values: %#v", *audited)
	}
}

func TestExportService_ExportExcel(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	soldier, err := soldierSvc.Create(models.Soldier{
		DisplayID:    "STC38-00001",
		Prefix:       "Capt.",
		FirstName:    "John",
		LastName:     "Hood",
		RankIn:       "Captain",
		RankOut:      "General",
		Unit:         "Texas Brigade",
		PensionState: "Texas",
		BirthDate:    "11/09/1831",
		DeathDate:    "00/00/1879",
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	if _, err := soldierSvc.Create(models.Soldier{
		DisplayID:       "STC38-00002",
		EntryType:       "widow",
		FirstName:       "Mary",
		LastName:        "Hood",
		SpouseSoldierID: soldier.ID,
		MaidenName:      "Jones",
	}); err != nil {
		t.Fatalf("Create spouse: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "export.xlsx")
	if err := exportSvc.ExportExcel(outPath); err != nil {
		t.Fatalf("ExportExcel: %v", err)
	}

	workbook, err := excelize.OpenFile(outPath)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer workbook.Close()

	sheets := workbook.GetSheetList()
	if len(sheets) < 3 || sheets[0] != "Archive Export" {
		t.Fatalf("unexpected workbook sheets: %v", sheets)
	}
	if value, err := workbook.GetCellValue("Archive Export", "F2"); err != nil || value != "TDM65-STC38-00001" {
		t.Fatalf("display ID cell = %q err=%v", value, err)
	}
	if value, err := workbook.GetCellValue("Archive Export", "I2"); err != nil || value != "" {
		t.Fatalf("linked spouse display should be empty for the soldier row before spouse backfill check: %q err=%v", value, err)
	}
	if value, err := workbook.GetCellValue("Archive Export", "AA2"); err != nil || value != "1831-11-09T00:00:00Z" {
		t.Fatalf("birth date cell = %q err=%v", value, err)
	}
	if value, err := workbook.GetCellValue("Linked Spouses", "A2"); err != nil || value != "TDM65-STC38-00002" {
		t.Fatalf("linked spouse sheet missing widow row: %q err=%v", value, err)
	}
	if value, err := workbook.GetCellValue("Linked Spouses", "F2"); err != nil || value != "Capt. John Hood" {
		t.Fatalf("linked spouse sheet missing linked soldier name: %q err=%v", value, err)
	}
}

func TestExportService_ExportCSV(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	_, _ = soldierSvc.Create(models.Soldier{
		FirstName:             "P.G.T.",
		LastName:              "Beauregard",
		Unit:                  "Army of the Potomac (CSA)",
		ConfederateHomeStatus: "Inmate",
		ConfederateHomeName:   "Soldiers Home, Austin",
		BirthInfo:             "Born circa 1830, New Orleans\npossibly St. Bernard Parish",
	})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "Braxton", LastName: "Bragg", Unit: "Army of Tennessee"})

	outPath := filepath.Join(t.TempDir(), "export.csv")
	if err := exportSvc.ExportCSV(outPath); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open exported file: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}

	// Header + 2 data rows
	if len(records) != 3 {
		t.Errorf("CSV has %d rows (incl header), want 3", len(records))
	}

	header := records[0]
	if len(header) < 5 {
		t.Errorf("CSV header too short: %v", header)
	}
	// Verify header contains expected columns
	expected := map[string]bool{
		"app_version": true, "schema_version": true, "export_version": true, "generated_at": true,
		"id": true, "display_id": true, "first_name": true,
		"entry_type": true, "spouse_soldier_id": true, "maiden_name": true,
		"pension_id": true, "application_id": true, "prefix": true, "middle_name": true,
		"last_name": true, "rank_in": true, "rank_out": true, "pension_state": true, "confederate_home_status": true, "confederate_home_name": true, "birth_date": true, "death_date": true, "birth_info": true, "buried_in": true,
		"suffix": true, "added_by": true, "last_edited_by": true, "last_edited_fields": true, "last_edited_at": true, "updated_at": true,
	}
	for _, col := range header {
		delete(expected, col)
	}
	if len(expected) > 0 {
		t.Errorf("CSV missing columns: %v", expected)
	}
	index := map[string]int{}
	for i, col := range header {
		index[col] = i
	}
	if records[1][index["confederate_home_status"]] != "Inmate" || records[1][index["confederate_home_name"]] != "Soldiers Home, Austin" {
		t.Fatalf("CSV missing confederate home values: %v", records[1])
	}
	if records[1][index["birth_info"]] != "Born circa 1830, New Orleans\npossibly St. Bernard Parish" {
		t.Fatalf("birth_info corrupted during CSV round trip: %q", records[1][index["birth_info"]])
	}
}

func TestExportService_ExportStaticArchive(t *testing.T) {
	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	soldierSvc := NewSoldierService(database)
	exportSvc := NewExportService(database, soldierSvc)
	configureExportIdentity(t, database)

	soldier, err := soldierSvc.Create(models.Soldier{
		DisplayID: "PENSION-0042",
		FirstName: "Robert",
		LastName:  "Lee",
		Unit:      "Army of Northern Virginia",
		BuriedIn:  "Hollywood Cemetery",
		Notes:     "Detailed archive note.",
		Records: []models.Record{
			{RecordType: "Pension", AppID: "42", Details: "Filed in 1880."},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	imageRelative := filepath.ToSlash(filepath.Join("images", "PENSION-0042", "portrait.png"))
	imagePath := filepath.Join(dataDir, filepath.FromSlash(imageRelative))
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := soldierSvc.AddImage(soldier.ID, "portrait.png", imageRelative, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "static-archive.zip")
	if err := exportSvc.ExportStaticArchive(outputPath, dataDir); err != nil {
		t.Fatalf("ExportStaticArchive: %v", err)
	}

	reader, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer reader.Close()

	entries := map[string]string{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		entries[file.Name] = string(data)
	}

	if _, ok := entries["index.html"]; !ok {
		t.Fatalf("archive missing index.html: %v", entries)
	}
	if _, ok := entries["data.js"]; !ok {
		t.Fatalf("archive missing data.js: %v", entries)
	}
	if _, ok := entries["images/PENSION-0042/portrait.png"]; !ok {
		t.Fatalf("archive missing copied image: %v", entries)
	}
	if !strings.Contains(entries["data.js"], "const archiveData = [") || !strings.Contains(entries["data.js"], "./images/PENSION-0042/portrait.png") {
		t.Fatalf("data.js missing expected archive payload: %s", entries["data.js"])
	}
	if !strings.Contains(entries["index.html"], "S. Carter&#39;s Civil War Research Archive") {
		t.Fatalf("index.html missing owner title: %s", entries["index.html"])
	}
	if !strings.Contains(entries["index.html"], "Made with DixieData | Version: "+buildinfo.AppVersion+" | Build: "+buildinfo.BuildIdentity()) {
		t.Fatalf("index.html missing version/build footer")
	}
}

func TestExportService_ExportSoldierPDFForSpouseEntry(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	outPath := filepath.Join(t.TempDir(), "wife.pdf")
	err := exportSvc.ExportSoldierPDF(outPath, models.Soldier{
		DisplayID:     "TDM65-DXD-00002",
		EntryType:     "widow",
		Prefix:        "Mrs.",
		FirstName:     "Martha",
		LastName:      "Taylor",
		Suffix:        "Sr.",
		SpouseName:    "John Taylor",
		MaidenName:    "Cole",
		PensionID:     "WP-42",
		ApplicationID: "WA-42",
	})
	if err != nil {
		t.Fatalf("ExportSoldierPDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "Record Type") || !strings.Contains(text, "Widow") {
		t.Fatalf("pdf missing spouse record type")
	}
	if !strings.Contains(text, "Spouse") || !strings.Contains(text, "John Taylor") {
		t.Fatalf("pdf missing spouse reference")
	}
	if !strings.Contains(text, "Maiden Name") || !strings.Contains(text, "Cole") {
		t.Fatalf("pdf missing maiden name")
	}
	if !strings.Contains(text, "Pension ID") || !strings.Contains(text, "WP-42") || !strings.Contains(text, "Application ID") || !strings.Contains(text, "WA-42") {
		t.Fatalf("pdf missing widow pension identifiers")
	}
	if !strings.Contains(text, "S. Carter's Civil War Research Archive") || !strings.Contains(text, "Made with DixieData | Version: "+buildinfo.AppVersion+" | Build: "+buildinfo.BuildIdentity()) {
		t.Fatalf("pdf missing standardized branding")
	}
}

func TestExportService_ExportImages(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	tempDir := t.TempDir()
	firstPath := filepath.Join(tempDir, "front.png")
	secondPath := filepath.Join(tempDir, "back.png")
	if err := os.WriteFile(firstPath, []byte("front-image"), 0o644); err != nil {
		t.Fatalf("write first image: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("back-image"), 0o644); err != nil {
		t.Fatalf("write second image: %v", err)
	}

	outPath := filepath.Join(tempDir, "STC38-00001_Images")
	err := exportSvc.ExportImages(outPath, []models.Image{
		{FileName: "front.png", FilePath: firstPath},
		{FileName: "back.png", FilePath: secondPath},
	})
	if err != nil {
		t.Fatalf("ExportImages: %v", err)
	}

	entries, err := os.ReadDir(outPath)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("copied file count = %d, want 2", len(entries))
	}
	if data, err := os.ReadFile(filepath.Join(outPath, "front.png")); err != nil || string(data) != "front-image" {
		t.Fatalf("front image copy mismatch: %q err=%v", string(data), err)
	}
	if data, err := os.ReadFile(filepath.Join(outPath, "back.png")); err != nil || string(data) != "back-image" {
		t.Fatalf("back image copy mismatch: %q err=%v", string(data), err)
	}
}

func TestExportService_ExportSoldierPDF(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	imagePath := filepath.Join(t.TempDir(), "portrait.png")
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "soldier.pdf")
	err := exportSvc.ExportSoldierPDF(outPath, models.Soldier{
		DisplayID: "PENSION-42",
		FirstName: "Robert",
		LastName:  "Lee",
		Rank:      "General",
		Unit:      "Army of Northern Virginia",
		BuriedIn:  "Hollywood Cemetery",
		Notes:     "Reference https://example.com/notes",
		Records:   []models.Record{{RecordType: "Pension", AppID: "42", Details: "Filed in 1880. https://example.com/record."}},
		Images:    []models.Image{{FileName: "portrait.png", FilePath: `images\pension-42\portrait.png`, ResolvedPath: imagePath, Caption: "Portrait"}},
	})
	if err != nil {
		t.Fatalf("ExportSoldierPDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 || string(data[:4]) != "%PDF" {
		t.Fatalf("output is not a PDF")
	}
	text := string(data)
	if !strings.Contains(text, "Hollywood Cemetery") {
		t.Fatalf("pdf missing buried-in text")
	}
	if !strings.Contains(text, "S. Carter's Civil War Research Archive") {
		t.Fatalf("pdf missing standardized branding")
	}
	if !strings.Contains(text, "https://example.com/notes") || !strings.Contains(text, "https://example.com/record") {
		t.Fatalf("pdf missing expected URL text")
	}
	if !strings.Contains(text, "/URI (https://example.com/notes)") || !strings.Contains(text, "/URI (https://example.com/record)") {
		t.Fatalf("pdf missing expected clickable link annotations")
	}
}

func TestExportService_ExportSoldierPDFWithoutImages(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	imagePath := filepath.Join(t.TempDir(), "portrait.png")
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "soldier-no-images.pdf")
	err := exportSvc.ExportSoldierPDFWithoutImages(outPath, models.Soldier{
		DisplayID: "PENSION-42",
		FirstName: "Robert",
		LastName:  "Lee",
		Images:    []models.Image{{FileName: "portrait.png", FilePath: `images\pension-42\portrait.png`, ResolvedPath: imagePath, Caption: "Portrait"}},
	})
	if err != nil {
		t.Fatalf("ExportSoldierPDFWithoutImages: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "Primary Image") || strings.Contains(text, "portrait.png") {
		t.Fatalf("no-images PDF should not render image panel content")
	}
}

func TestExportService_ExportMonthlyAnniversaryPDF(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	outPath := filepath.Join(t.TempDir(), "monthly.pdf")
	err := exportSvc.ExportMonthlyAnniversaryPDF(outPath, 4, map[int][]models.Soldier{
		9: {
			{DisplayID: "DD-1", FirstName: "John", LastName: "Smith"},
			{DisplayID: "DD-2", FirstName: "James", LastName: "Brown"},
		},
	})
	if err != nil {
		t.Fatalf("ExportMonthlyAnniversaryPDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 || string(data[:4]) != "%PDF" {
		t.Fatalf("output is not a PDF")
	}
}

func TestExportService_ExportFullDatabasePDF(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	first, err := soldierSvc.Create(models.Soldier{
		Prefix:                "Capt.",
		FirstName:             "John",
		MiddleName:            "Bell",
		LastName:              "Hood",
		Suffix:                "Jr.",
		Unit:                  "Texas Brigade",
		RankIn:                "Captain",
		RankOut:               "General",
		PensionState:          "Texas",
		PensionID:             "P-42",
		ApplicationID:         "A-42",
		ConfederateHomeStatus: "Trustee",
		ConfederateHomeName:   "Texas Confederate Home",
		Notes:                 "Registry note.",
	})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := soldierSvc.Create(models.Soldier{
		EntryType:       "widow",
		FirstName:       "Mary",
		LastName:        "Hood",
		SpouseSoldierID: first.ID,
		MaidenName:      "Jones",
	})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if _, err := soldierSvc.GetByID(second.ID); err != nil {
		t.Fatalf("GetByID second: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "registry.pdf")
	if err := exportSvc.ExportFullDatabasePDF(outPath, PrintSettings{}); err != nil {
		t.Fatalf("ExportFullDatabasePDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if len(data) == 0 || string(data[:4]) != "%PDF" {
		t.Fatalf("output is not a PDF")
	}
	if !strings.Contains(text, "Printable Archive Registry") || !strings.Contains(text, "S. Carter's Civil War Research Archive") {
		t.Fatalf("registry PDF missing expected branding")
	}
	if !strings.Contains(text, "Capt. John Bell Hood, Jr.") || !strings.Contains(text, "Registry note.") {
		t.Fatalf("registry PDF missing expected record content")
	}
	if !strings.Contains(text, "Pension State") || !strings.Contains(text, "Texas") || !strings.Contains(text, "Trustee") || !strings.Contains(text, "Texas Confederate Home") {
		t.Fatalf("registry PDF missing expanded status fields")
	}
	if !strings.Contains(text, "Linked Spouse Record") || !strings.Contains(text, "John Bell Hood") || !strings.Contains(text, "Maiden Name") || !strings.Contains(text, "Jones") {
		t.Fatalf("registry PDF missing spouse linkage fields")
	}
}

func TestExportService_ExportFullDatabasePDFFitsSingleRecordPage(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	longNote := strings.Repeat("This is a long note intended to pressure the right column layout without forcing a second page when the printable export shrinks typography appropriately. ", 18)
	_, err := soldierSvc.Create(models.Soldier{
		Prefix:                "Capt.",
		FirstName:             "Thomas",
		LastName:              "Carter",
		Suffix:                "Sr.",
		RankIn:                "Captain",
		RankOut:               "Colonel",
		Unit:                  "Virginia Cavalry",
		PensionState:          "Virginia",
		PensionID:             "WP-900",
		ApplicationID:         "WA-900",
		ConfederateHomeStatus: "Staffer",
		ConfederateHomeName:   "Virginia Soldiers Home",
		Notes:                 longNote,
		Records: []models.Record{
			{RecordType: "Pension", AppID: "900", Details: longNote},
			{RecordType: "Correspondence", AppID: "901", Details: longNote},
		},
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "single-page.pdf")
	if err := exportSvc.ExportFullDatabasePDF(outPath, PrintSettings{}); err != nil {
		t.Fatalf("ExportFullDatabasePDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	pageCount := len(regexp.MustCompile(`/Type /Page\b`).FindAll(data, -1))
	if pageCount != 3 {
		t.Fatalf("pageCount = %d, want 3 pages (title, record, metadata)", pageCount)
	}
}

func TestImagePathForPDFSkipsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jpg")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := imagePathForPDF(models.Image{ResolvedPath: path}); got != "" {
		t.Fatalf("imagePathForPDF returned %q for empty file", got)
	}
}

func TestExportService_ExportFullDatabasePDFAppliesSortAndGrouping(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	for _, soldier := range []models.Soldier{
		{FirstName: "William", LastName: "Brown", Unit: "A Company", PensionState: "Texas", BirthDate: "00/00/1830", DeathDate: "00/00/1864"},
		{FirstName: "James", LastName: "Adams", Unit: "B Company", PensionState: "Alabama", BirthDate: "00/00/1825", DeathDate: "00/00/1865"},
		{FirstName: "Samuel", LastName: "Carter", Unit: "B Company", PensionState: "Alabama", BirthDate: "00/00/1820", DeathDate: "00/00/1863"},
	} {
		if _, err := soldierSvc.Create(soldier); err != nil {
			t.Fatalf("Create soldier: %v", err)
		}
	}

	outPath := filepath.Join(t.TempDir(), "grouped-registry.pdf")
	settings := PrintSettings{
		SortBy:              PrintSortBirthYear,
		GroupByUnit:         true,
		GroupByPensionState: true,
	}
	if err := exportSvc.ExportFullDatabasePDF(outPath, settings); err != nil {
		t.Fatalf("ExportFullDatabasePDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "Grouped by Unit") || !strings.Contains(text, "A Company") || !strings.Contains(text, "B Company") {
		t.Fatalf("grouped PDF missing unit divider pages")
	}
	if !strings.Contains(text, "Grouped by Pension State") || !strings.Contains(text, "Texas") || !strings.Contains(text, "Alabama") {
		t.Fatalf("grouped PDF missing pension-state divider pages")
	}
	if !strings.Contains(text, "Chronological by Birth Year") || !strings.Contains(text, "Unit, Pension State") {
		t.Fatalf("grouped PDF missing print-settings metadata")
	}
	if strings.Index(text, "Samuel Carter") > strings.Index(text, "James Adams") {
		t.Fatalf("records were not ordered by birth year within the grouped section")
	}
}

func TestFitPDFImageToBoundsPreservesAspectRatio(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		maxWidth   float64
		maxHeight  float64
		wantWidth  float64
		wantHeight float64
	}{
		{name: "landscape", width: 200, height: 100, maxWidth: 50, maxHeight: 50, wantWidth: 50, wantHeight: 25},
		{name: "portrait", width: 100, height: 200, maxWidth: 50, maxHeight: 50, wantWidth: 25, wantHeight: 50},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			imagePath := filepath.Join(t.TempDir(), test.name+".png")
			writeSizedPNGFixture(t, imagePath, test.width, test.height)

			_, _, gotWidth, gotHeight, ok := fitPDFImageToBounds(imagePath, 10, 20, test.maxWidth, test.maxHeight)
			if !ok {
				t.Fatalf("fitPDFImageToBounds returned ok=false")
			}
			if gotWidth != test.wantWidth || gotHeight != test.wantHeight {
				t.Fatalf("fitPDFImageToBounds = %.2fx%.2f, want %.2fx%.2f", gotWidth, gotHeight, test.wantWidth, test.wantHeight)
			}
		})
	}
}

func TestImagePathForPDFSkipsUnsupportedFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portrait.webp")
	if err := os.WriteFile(path, []byte("not-a-pdf-image"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := imagePathForPDF(models.Image{ResolvedPath: path}); got != "" {
		t.Fatalf("imagePathForPDF returned %q for unsupported format", got)
	}
}

func TestImagePathForPDFSkipsCorruptPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.png")
	if err := os.WriteFile(path, []byte("not-a-real-png"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := imagePathForPDF(models.Image{ResolvedPath: path}); got != "" {
		t.Fatalf("imagePathForPDF returned %q for corrupt PNG", got)
	}
}

func TestPDFTextSegments(t *testing.T) {
	segments := pdfTextSegments("See https://example.com/test, then http://example.org.")
	if len(segments) != 6 {
		t.Fatalf("segment count = %d, want 6", len(segments))
	}
	if segments[1].Link != "https://example.com/test" {
		t.Fatalf("first link = %#v", segments[1])
	}
	if segments[2].Text != "," || segments[3].Text != " then " {
		t.Fatalf("unexpected middle segments: %#v", segments)
	}
	if segments[4].Link != "http://example.org" || segments[5].Text != "." {
		t.Fatalf("unexpected final segments: %#v", segments)
	}
}

func TestExportService_ExportICalendar(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	_, _ = soldierSvc.Create(models.Soldier{
		DisplayID:  "CSA-1",
		FirstName:  "John",
		LastName:   "Smith",
		Unit:       "1st Virginia",
		BuriedIn:   "Richmond",
		DeathYear:  1862,
		DeathMonth: 4,
		DeathDay:   9,
	})
	_, _ = soldierSvc.Create(models.Soldier{
		DisplayID:  "CSA-2",
		FirstName:  "No",
		LastName:   "Day",
		DeathYear:  1863,
		DeathMonth: 0,
		DeathDay:   0,
	})

	outPath := filepath.Join(t.TempDir(), "anniversaries.ics")
	if err := exportSvc.ExportICalendar(outPath); err != nil {
		t.Fatalf("ExportICalendar: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	normalizedText := strings.ReplaceAll(text, "\r\n ", "")
	if !strings.Contains(text, "BEGIN:VCALENDAR") || !strings.Contains(text, "END:VCALENDAR") {
		t.Fatalf("ics missing calendar wrapper: %q", text)
	}
	if !strings.Contains(text, "SUMMARY:Memorial Anniversary: John Smith") {
		t.Fatalf("ics missing summary: %q", text)
	}
	if !strings.Contains(normalizedText, "DESCRIPTION:Record ID: TDM65-CSA-1\\nEntry Type: Soldier\\nFull Name: John Smith") ||
		!strings.Contains(normalizedText, "\\nUnit: 1st Virginia") ||
		!strings.Contains(normalizedText, "\\nBuried In: Richmond") ||
		!strings.Contains(normalizedText, "\\nOriginal Death Date: April 9\\, 1862") {
		t.Fatalf("ics missing cleaned description: %q", text)
	}
	expectedStart := nextGoogleAnniversaryDate(models.Soldier{DeathMonth: 4, DeathDay: 9}, time.Now()).Format("20060102") + "T090000"
	if !strings.Contains(text, "DTSTART:"+expectedStart) {
		t.Fatalf("ics missing start date: %q", text)
	}
	if !strings.Contains(text, "DTEND:"+nextGoogleAnniversaryDate(models.Soldier{DeathMonth: 4, DeathDay: 9}, time.Now()).Format("20060102")+"T100000") {
		t.Fatalf("ics missing end date: %q", text)
	}
	if !strings.Contains(text, "RRULE:FREQ=YEARLY") {
		t.Fatalf("ics missing recurrence rule: %q", text)
	}
	if !strings.Contains(text, "STATUS:CONFIRMED") || !strings.Contains(text, "TRANSP:TRANSPARENT") {
		t.Fatalf("ics missing calendar metadata: %q", text)
	}
	if !strings.Contains(text, "BEGIN:VALARM") || !strings.Contains(text, "TRIGGER:-P1D") || !strings.Contains(text, "TRIGGER:-PT1H") {
		t.Fatalf("ics missing reminder alarms: %q", text)
	}
	if !strings.Contains(text, "DESCRIPTION:Upcoming memorial anniversary for John Smith") || !strings.Contains(text, "DESCRIPTION:Memorial anniversary in one hour for John Smith") {
		t.Fatalf("ics missing updated alarm descriptions: %q", text)
	}
	if strings.Contains(text, "CSA-2") {
		t.Fatalf("ics should skip soldiers without full month/day")
	}
}

func pngFixture() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0x99, 0x63,
		0xf8, 0xcf, 0xc0, 0x00, 0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
		0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60,
		0x82,
	}
}

func writeSizedPNGFixture(t *testing.T, path string, width, height int) {
	t.Helper()
	imageRect := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			imageRect.Set(x, y, color.RGBA{R: 180, G: 120, B: 70, A: 255})
		}
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer file.Close()
	if err := png.Encode(file, imageRect); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
}
