package archive

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
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
	"github.com/valueforvalue/DixieData/internal/peopleinfo"
	"github.com/valueforvalue/DixieData/pkg/render"
	"github.com/xuri/excelize/v2"
)

type rasterizerFunc func(pdfPath, outputDir string) ([]string, error)

func (f rasterizerFunc) Rasterize(pdfPath, outputDir string) ([]string, error) {
	return f(pdfPath, outputDir)
}

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
	if value, err := workbook.GetCellValue("Archive Export", "F2"); err != nil || value != "STC38-00001" {
		t.Fatalf("display ID cell = %q err=%v", value, err)
	}
	if value, err := workbook.GetCellValue("Archive Export", "I2"); err != nil || value != "" {
		t.Fatalf("linked spouse display should be empty for the soldier row before spouse backfill check: %q err=%v", value, err)
	}
	if value, err := workbook.GetCellValue("Archive Export", "AB2"); err != nil || value != "1831-11-09T00:00:00Z" {
		t.Fatalf("birth date cell = %q err=%v", value, err)
	}
	if value, err := workbook.GetCellValue("Linked Relationships", "A2"); err != nil || value != "STC38-00002" {
		t.Fatalf("linked spouse sheet missing widow row: %q err=%v", value, err)
	}
	if value, err := workbook.GetCellValue("Linked Relationships", "F2"); err != nil || value != "Capt. John Hood" {
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
		"entry_type": true, "spouse_soldier_id": true, "relationship_label": true, "maiden_name": true,
		"pension_id": true, "application_id": true, "prefix": true, "middle_name": true,
		"last_name": true, "rank_in": true, "rank_out": true, "pension_state": true, "confederate_home_status": true, "confederate_home_name": true, "birth_date": true, "death_date": true, "birth_info": true, "buried_in": true, "biography": true,
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
		DisplayID:        "PENSION-0042",
		Prefix:           "Gen.",
		FirstName:        "Robert",
		MiddleName:       "Edward",
		LastName:         "Lee",
		Suffix:           "Sr.",
		Unit:             "Army of Northern Virginia",
		BirthDate:        "01/19/1807",
		DeathDate:        "10/12/1870",
		BirthInfo:        "Stratford Hall, Virginia",
		BuriedIn:         "Hollywood Cemetery",
		Biography:        "Confederate general whose record biography should travel with archive exports.",
		Notes:            "Detailed archive note.",
		NeedsReview:      true,
		ReviewReason:     "Potential duplicate from archive merge.",
		AddedBy:          "STC38",
		LastEditedBy:     "JCM87",
		LastEditedAt:     "2026-05-16T16:00:00Z",
		LastEditedFields: "notes,review_reason",
		Records: []models.Record{
			{RecordType: "Pension", AppID: "42", Details: "Filed in 1880."},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	widow, err := soldierSvc.Create(models.Soldier{
		FirstName:       "Mary",
		LastName:        "Lee",
		EntryType:       "widow",
		SpouseSoldierID: soldier.ID,
		MaidenName:      "Custis",
		PensionID:       "WP-42",
		ApplicationID:   "WA-42",
	})
	if err != nil {
		t.Fatalf("Create widow: %v", err)
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
	if _, ok := entries["viewer.html"]; !ok {
		t.Fatalf("archive missing viewer.html: %v", entries)
	}
	if _, ok := entries["archive_data.js"]; !ok {
		t.Fatalf("archive missing archive_data.js: %v", entries)
	}
	if _, ok := entries["images/PENSION-0042/portrait.png"]; !ok {
		t.Fatalf("archive missing copied image: %v", entries)
	}
	if !strings.Contains(entries["archive_data.js"], "window.DIXIE_DATA = [") || !strings.Contains(entries["archive_data.js"], "./images/PENSION-0042/portrait.png") {
		t.Fatalf("archive_data.js missing expected archive payload: %s", entries["archive_data.js"])
	}
	if !strings.Contains(entries["archive_data.js"], `"spouseDisplayId": "PENSION-0042"`) {
		t.Fatalf("archive_data.js missing spouse link to soldier: %s", entries["archive_data.js"])
	}
	if !strings.Contains(entries["archive_data.js"], widow.DisplayID) || !strings.Contains(entries["archive_data.js"], `"reviewReason": "Potential duplicate from archive merge."`) || !strings.Contains(entries["archive_data.js"], `"addedBy": "STC38"`) {
		t.Fatalf("archive_data.js missing full-detail fields: %s", entries["archive_data.js"])
	}
	if !strings.Contains(entries["archive_data.js"], `"biography": "Confederate general whose record biography should travel with archive exports."`) {
		t.Fatalf("archive_data.js missing biography field: %s", entries["archive_data.js"])
	}
	if !strings.Contains(entries["index.html"], "S. Carter&#39;s Civil War Research Archive") {
		t.Fatalf("index.html missing owner title: %s", entries["index.html"])
	}
	if !strings.Contains(entries["viewer.html"], "Family Links") || !strings.Contains(entries["viewer.html"], "Archive Metadata") {
		t.Fatalf("viewer.html missing expanded detail sections: %s", entries["viewer.html"])
	}
	if !strings.Contains(entries["viewer.html"], "function showDetailScreen(record, index, visibleCount, allRecords)") ||
		!strings.Contains(entries["viewer.html"], "renderDetail(record, allRecords)") ||
		!strings.Contains(entries["viewer.html"], "showDetailScreen(records[matchIndex], finalVisibleIndex, filteredRecords.length, records);") {
		t.Fatalf("viewer.html missing static detail render wiring fix: %s", entries["viewer.html"])
	}
	if !strings.Contains(entries["index.html"], "Made with DixieData | Version: "+buildinfo.AppVersion+" | Build: "+buildinfo.BuildIdentity()) {
		t.Fatalf("index.html missing version/build footer")
	}
	if !strings.Contains(entries["viewer.html"], "return text || 'Unknown';") {
		t.Fatalf("viewer.html should show Unknown for blank dates: %s", entries["viewer.html"])
	}
	if !strings.Contains(entries["viewer.html"], "['Prefix', blankDetailValue(record.prefix)]") ||
		!strings.Contains(entries["viewer.html"], "['Unit', blankDetailValue(record.unit)]") {
		t.Fatalf("viewer.html should keep blank name/service fields blank: %s", entries["viewer.html"])
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
	}, PDFOptions{IncludeImages: true})
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
		DisplayID:          "PENSION-42",
		FirstName:          "Robert",
		LastName:           "Lee",
		Rank:               "General",
		Unit:               "Army of Northern Virginia",
		BuriedIn:           "Hollywood Cemetery",
		Biography:          "Landscape biography should appear in single-record export. https://example.com/bio",
		PDFExcerptOverride: "Landscape override should be ignored.",
		AddedBy:            "J. Morris",
		LastEditedBy:       "J. Morris",
		LastEditedAt:       "2026-05-30T02:38:13Z",
		LastEditedFields:   "entry_type,last_edited_fields",
		Notes:              "Scratch note should stay out of single-record export.",
		Records:            []models.Record{{RecordType: "Pension", AppID: "42", Details: "Filed in 1880. https://example.com/record."}},
		Images:             []models.Image{{FileName: "portrait.png", FilePath: `images\pension-42\portrait.png`, ResolvedPath: imagePath, Caption: "Portrait"}},
	}, PDFOptions{IncludeImages: true})
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
	if !strings.Contains(text, "Portrait") {
		t.Fatalf("pdf missing image caption")
	}
	if !strings.Contains(text, "S. Carter's Civil War Research Archive") {
		t.Fatalf("pdf missing standardized branding")
	}
	if !strings.Contains(text, "https://example.com/bio") || !strings.Contains(text, "https://example.com/record") {
		t.Fatalf("pdf missing expected URL text")
	}
	if !strings.Contains(text, "/URI (https://example.com/bio)") || !strings.Contains(text, "/URI (https://example.com/record)") {
		t.Fatalf("pdf missing expected clickable link annotations")
	}
	if !strings.Contains(text, "Biography") || !strings.Contains(text, "Landscape biography should appear in single-record export.") {
		t.Fatalf("landscape pdf should use biography content")
	}
	if !strings.Contains(text, "Full Biography") {
		t.Fatalf("landscape pdf should append a full biography page")
	}
	pageCount := len(regexp.MustCompile(`/Type /Page\b`).FindAll(data, -1))
	if pageCount < 2 {
		t.Fatalf("pageCount = %d, want at least 2 pages for landscape biography export", pageCount)
	}
	for _, forbidden := range []string{"Added By", "Last Edited By", "Last Edited At", "Last Edited Fields", "J. Morris", "entry_type,last_edited_fields", "portrait.png", "Primary Image", "Scratch note should stay out of single-record export.", "https://example.com/notes", "Landscape override should be ignored."} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("pdf should omit audit metadata %q", forbidden)
		}
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
	pageCount := len(regexp.MustCompile(`/Type /Page\b`).FindAll(data, -1))
	if pageCount != 1 {
		t.Fatalf("pageCount = %d, want 1 when landscape biography is blank", pageCount)
	}
}

func TestExportService_ExportSoldierPDFPrinterFriendly(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	imagePath := filepath.Join(t.TempDir(), "portrait.png")
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "soldier-printer-friendly.pdf")
	err := exportSvc.ExportSoldierPDF(outPath, models.Soldier{
		DisplayID:          "PENSION-42",
		FirstName:          "Robert",
		LastName:           "Lee",
		PensionState:       "N/A",
		SpouseSoldierID:    5,
		SpouseName:         "Mary Lee",
		Biography:          "Full biography should stay out when excerpt override exists.",
		PDFExcerptOverride: "Portrait biography excerpt with [[PENSION-42]] and https://example.com/bio.",
		Notes:              "Scratch note should never appear in portrait PDF.",
		Records:            []models.Record{{RecordType: "Pension", Details: "Filed in 1880. https://example.com/record."}},
		Images:             []models.Image{{FileName: "portrait.png", FilePath: `images\pension-42\portrait.png`, ResolvedPath: imagePath, Caption: "Portrait caption"}},
	}, PDFOptions{Orientation: "P", PrinterFriendly: true, IncludeImages: true})
	if err != nil {
		t.Fatalf("ExportSoldierPDF printer friendly: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{
		"https://example.com/bio",
		"https://example.com/record",
		"/URI (https://example.com/bio)",
		"/URI (https://example.com/record)",
		"portrait.png",
		"Report Metadata",
		"Made with DixieData",
		"Primary Image",
		"Scratch note should never appear in portrait PDF.",
		"Full biography should stay out when excerpt override exists.",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("printer friendly PDF should omit %q", forbidden)
		}
	}
	if strings.Contains(text, "Pension State") || strings.Contains(text, "Linked Spouse Record") {
		t.Fatalf("printer friendly PDF should omit placeholder and link-only fields")
	}
	if !strings.Contains(text, "PENSION-42") {
		t.Fatalf("printer friendly PDF should keep display id in title block")
	}
	if !strings.Contains(text, "Biography") || !strings.Contains(text, "Portrait biography excerpt") || !strings.Contains(text, "PENSION-42") {
		t.Fatalf("printer friendly portrait PDF should use biography excerpt")
	}
	if !strings.Contains(text, "Portrait caption") {
		t.Fatalf("printer friendly portrait PDF should preserve image caption")
	}
}

func TestExportService_ExportSoldierPDFPrinterFriendlyPortraitFits1200CharExcerpt(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	imagePath := filepath.Join(t.TempDir(), "portrait.png")
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	override := strings.Repeat("Portrait override text for compact portrait layout. ", 25)[:1200]
	outPath := filepath.Join(t.TempDir(), "soldier-printer-friendly-1200.pdf")
	err := exportSvc.ExportSoldierPDF(outPath, models.Soldier{
		DisplayID:          "PENSION-1200",
		FirstName:          "Robert",
		LastName:           "Lee",
		PensionState:       "N/A",
		SpouseSoldierID:    5,
		SpouseName:         "Mary Lee",
		Biography:          "Full biography should stay out when excerpt override exists.",
		PDFExcerptOverride: override,
		Notes:              "Scratch note should never appear in portrait PDF.",
		Records:            []models.Record{{RecordType: "Pension", Details: "Filed in 1880."}},
		Images:             []models.Image{{FileName: "portrait.png", FilePath: `images\pension-1200\portrait.png`, ResolvedPath: imagePath, Caption: "Portrait caption"}},
	}, PDFOptions{Orientation: "P", PrinterFriendly: true, IncludeImages: true})
	if err != nil {
		t.Fatalf("ExportSoldierPDF printer friendly: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	text := string(data)
	if !strings.Contains(text, "Portrait override text for compact portrait layout.") {
		t.Fatalf("printer friendly portrait PDF should include the override text")
	}
	pageCount := len(regexp.MustCompile(`/Type /Page\b`).FindAll(data, -1))
	if pageCount != 1 {
		t.Fatalf("expected representative 1200-char portrait excerpt PDF to stay on one page, got %d pages", pageCount)
	}
}

func TestExportService_ExportSoldierPDFOmitsPrimaryImageJPEGFileNameStoredAsCaption(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	imageName := "11558933_9fdc5984-1f88-40d7-be17-146e1caaaaf1.jpeg"
	imagePath := filepath.Join(t.TempDir(), imageName)
	if err := os.WriteFile(imagePath, jpegFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "soldier-primary-image-no-filename.pdf")
	err := exportSvc.ExportSoldierPDF(outPath, models.Soldier{
		DisplayID:     "DXD-00054",
		FirstName:     "James",
		MiddleName:    "A.",
		LastName:      "Myers",
		BirthDate:     "1843",
		DeathDate:     "1934-07-31",
		BirthInfo:     "1843, Alabama, USA",
		BuriedIn:      "Letitia Cemetery, Lawton, Comanche County, Oklahoma, USA",
		EntryType:     "soldier",
		RankIn:        "Private",
		RankOut:       "Pvt.",
		Unit:          "Co. C, 19th AL Infantry Regiment, C.S.A.",
		PensionState:  "Oklahoma",
		PensionID:     "P4187",
		ApplicationID: "A5046",
		Images: []models.Image{{
			FileName:     imageName,
			FilePath:     `images\dxd-00054\11558933_9fdc5984-1f88-40d7-be17-146e1caaaaf1.jpeg`,
			ResolvedPath: imagePath,
			Caption:      imageName,
			IsPrimary:    true,
		}},
	}, PDFOptions{Orientation: "P", IncludeImages: true})
	if err != nil {
		t.Fatalf("ExportSoldierPDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), imageName) {
		t.Fatalf("pdf should not contain primary image file name %q", imageName)
	}
	if strings.Contains(string(data), "Primary Image") {
		t.Fatalf("portrait pdf should omit primary image heading")
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
	}, PDFOptions{})
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

	imagePath := filepath.Join(t.TempDir(), "registry-portrait.png")
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

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
		Biography:             "Registry biography should appear in printable export.",
		Notes:                 "Registry scratch note should stay out of printable export.",
	})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if err := soldierSvc.AddImage(first.ID, "registry-portrait.png", imagePath, "Registry portrait"); err != nil {
		t.Fatalf("AddImage first: %v", err)
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
	if !strings.Contains(text, "Capt. John Bell Hood, Jr.") || !strings.Contains(text, "Registry biography should appear in printable export.") {
		t.Fatalf("registry PDF missing expected record content")
	}
	if !strings.Contains(text, "Pension State") || !strings.Contains(text, "Texas") || !strings.Contains(text, "Trustee") || !strings.Contains(text, "Texas Confederate Home") {
		t.Fatalf("registry PDF missing expanded status fields")
	}
	if !strings.Contains(text, "Registry portrait") {
		t.Fatalf("registry PDF missing captioned primary image")
	}
	if !strings.Contains(text, "Linked Spouse Record") || !strings.Contains(text, "John Bell Hood") || !strings.Contains(text, "Maiden Name") || !strings.Contains(text, "Jones") {
		t.Fatalf("registry PDF missing spouse linkage fields")
	}
	for _, forbidden := range []string{"Registry scratch note should stay out of printable export.", "Primary Image"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("registry PDF should omit %q", forbidden)
		}
	}
	if strings.Contains(text, "Report Metadata") {
		t.Fatalf("registry PDF should not render report metadata as a standalone section")
	}
	if !strings.Contains(text, "database-pdf v") {
		t.Fatalf("registry PDF should carry concise metadata in the footer")
	}
}

func TestExportService_ExportFullDatabasePDFAppendsFullBiographyPageWhenEnabled(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	biography := strings.Repeat("Full biography sentence. ", 80) + "FINAL BIOGRAPHY LINE"
	_, err := soldierSvc.Create(models.Soldier{
		FirstName: "Elias",
		LastName:  "Turner",
		Biography: biography,
		Notes:     "Internal scratch note should stay out.",
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	withoutAppendixPath := filepath.Join(t.TempDir(), "registry-default.pdf")
	if err := exportSvc.ExportFullDatabasePDF(withoutAppendixPath, PrintSettings{}); err != nil {
		t.Fatalf("ExportFullDatabasePDF default: %v", err)
	}
	withoutAppendixData, err := os.ReadFile(withoutAppendixPath)
	if err != nil {
		t.Fatalf("ReadFile default: %v", err)
	}
	if strings.Contains(string(withoutAppendixData), "FINAL BIOGRAPHY LINE") {
		t.Fatalf("default printable export should keep full biography appendix off")
	}

	withAppendixPath := filepath.Join(t.TempDir(), "registry-full-biography.pdf")
	if err := exportSvc.ExportFullDatabasePDF(withAppendixPath, PrintSettings{FullBiographyPage: true}); err != nil {
		t.Fatalf("ExportFullDatabasePDF full biography: %v", err)
	}
	withAppendixData, err := os.ReadFile(withAppendixPath)
	if err != nil {
		t.Fatalf("ReadFile full biography: %v", err)
	}
	text := string(withAppendixData)
	if !strings.Contains(text, "Full Biography Appendix") || !strings.Contains(text, "FINAL BIOGRAPHY LINE") {
		t.Fatalf("full biography appendix missing expected full text")
	}
	if strings.Contains(text, "Internal scratch note should stay out.") {
		t.Fatalf("full biography appendix should not leak internal notes")
	}
	pageCount := len(regexp.MustCompile(`/Type /Page\b`).FindAll(withAppendixData, -1))
	if pageCount < 3 {
		t.Fatalf("pageCount = %d, want at least 3 pages (title, record, biography appendix)", pageCount)
	}
}

func TestExportService_ExportAnalyticsSummaryPDF(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	outPath := filepath.Join(t.TempDir(), "analytics-report.pdf")
	err := exportSvc.ExportAnalyticsSummaryPDF(outPath, AnalyticsSnapshot{
		RecordTypes: models.ArchiveCounts{
			TotalSoldiers:    12,
			TotalWivesWidows: 4,
		},
		CemeteryDensity:         []AnalyticsCount{{Label: "Oak Hill Cemetery", Count: 7}},
		ConfederateHomeStatus:   []AnalyticsCount{{Label: "Inmate", Count: 3}},
		ConfederateHomeNames:    []AnalyticsCount{{Label: "Texas Confederate Home", Count: 2}},
		PensionDistribution:     []AnalyticsCount{{Label: "Texas", Count: 5}},
		UnitRepresentation:      []AnalyticsCount{{Label: "1st Texas Infantry", Count: 4}},
		BirthDecadeDistribution: []AnalyticsCount{{Label: "1830s", Count: 6}},
		DeathDecadeDistribution: []AnalyticsCount{{Label: "1900s", Count: 2}},
	}, PDFOptions{})
	if err != nil {
		t.Fatalf("ExportAnalyticsSummaryPDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	for _, needle := range []string{
		"Archive Summary Report",
		"Top Cemeteries",
		"Oak Hill Cemetery",
		"Confederate Home Participation",
		"Texas Confederate Home",
		"Record Types",
		"Soldiers: 12",
		"Spouses",
		"Wives & Widows",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("analytics PDF missing %s", needle)
		}
	}
	if strings.Contains(text, "Report Metadata") {
		t.Fatalf("analytics PDF should not render report metadata as a standalone section")
	}
	if !strings.Contains(text, "analytics-pdf v") {
		t.Fatalf("analytics PDF should carry concise metadata in the footer")
	}
}

func TestExportService_ExportFullDatabasePDFUsesMultiPageFallbackForLongRecords(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	longNote := strings.Repeat("This is a long note intended to pressure the printable export so the renderer must add continuation pages instead of shrinking to unreadable text. ", 80)
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
		AddedBy:               "J. Morris",
		LastEditedBy:          "J. Morris",
		LastEditedAt:          "2026-05-30T02:38:13Z",
		LastEditedFields:      "notes,records",
		Notes:                 longNote,
		Records: []models.Record{
			{RecordType: "Pension", AppID: "900", Details: longNote},
			{RecordType: "Correspondence", AppID: "901", Details: longNote},
			{RecordType: "Unit History", AppID: "902", Details: longNote},
			{RecordType: "Muster Roll", AppID: "903", Details: longNote},
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
	if pageCount < 4 {
		t.Fatalf("pageCount = %d, want at least 4 pages (title, multi-page record, metadata)", pageCount)
	}
	text := string(data)
	for _, forbidden := range []string{"Added By", "Last Edited By", "Last Edited At", "Last Edited Fields", "J. Morris", "notes,records"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("registry PDF should omit audit metadata %q", forbidden)
		}
	}
}

func TestExportService_ExportSoldierJPGWritesSiblingPages(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	exportSvc.rasterizer = rasterizerFunc(func(pdfPath, outputDir string) ([]string, error) {
		if _, err := os.Stat(pdfPath); err != nil {
			t.Fatalf("generated PDF missing: %v", err)
		}
		pdfData, err := os.ReadFile(pdfPath)
		if err != nil {
			t.Fatalf("ReadFile pdfPath: %v", err)
		}
		pdfText := string(pdfData)
		for _, needle := range []string{"JPG biography should match PDF renderer.", "JPG portrait", "Full Biography"} {
			if !strings.Contains(pdfText, needle) {
				t.Fatalf("JPG source PDF missing %q", needle)
			}
		}
		pageCount := len(regexp.MustCompile(`/Type /Page\b`).FindAll(pdfData, -1))
		if pageCount < 2 {
			t.Fatalf("JPG source PDF pageCount = %d, want at least 2", pageCount)
		}
		for _, forbidden := range []string{"Primary Image", "JPG scratch note should stay out.", "JPG override should be ignored."} {
			if strings.Contains(pdfText, forbidden) {
				t.Fatalf("JPG source PDF should omit %q", forbidden)
			}
		}
		first := filepath.Join(outputDir, "page-001.jpg")
		second := filepath.Join(outputDir, "page-002.jpg")
		if err := os.WriteFile(first, []byte("page-1"), 0o644); err != nil {
			t.Fatalf("WriteFile first: %v", err)
		}
		if err := os.WriteFile(second, []byte("page-2"), 0o644); err != nil {
			t.Fatalf("WriteFile second: %v", err)
		}
		return []string{first, second}, nil
	})

	imagePath := filepath.Join(t.TempDir(), "jpg-portrait.png")
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	soldier := models.Soldier{
		DisplayID:          "STC38-00001",
		FirstName:          "John",
		LastName:           "Taylor",
		Biography:          "JPG biography should match PDF renderer.",
		PDFExcerptOverride: "JPG override should be ignored.",
		Notes:              "JPG scratch note should stay out.",
		Images:             []models.Image{{FileName: "jpg-portrait.png", FilePath: `images\stc38-00001\jpg-portrait.png`, ResolvedPath: imagePath, Caption: "JPG portrait"}},
	}
	outputPath := filepath.Join(t.TempDir(), "record.jpg")
	paths, err := exportSvc.ExportSoldierJPG(outputPath, soldier, PDFOptions{Orientation: "L", IncludeImages: true})
	if err != nil {
		t.Fatalf("ExportSoldierJPG: %v", err)
	}

	expected := []string{
		outputPath,
		filepath.Join(filepath.Dir(outputPath), "record-page-002.jpg"),
	}
	if len(paths) != len(expected) {
		t.Fatalf("paths = %v, want %v", paths, expected)
	}
	for i := range expected {
		if paths[i] != expected[i] {
			t.Fatalf("path %d = %q, want %q", i, paths[i], expected[i])
		}
		data, err := os.ReadFile(paths[i])
		if err != nil {
			t.Fatalf("ReadFile %q: %v", paths[i], err)
		}
		if len(data) == 0 {
			t.Fatalf("rendered JPG %q was empty", paths[i])
		}
	}
}

func TestExportService_ExportSoldierJPGRemovesStaleSiblingPages(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	exportSvc.rasterizer = rasterizerFunc(func(pdfPath, outputDir string) ([]string, error) {
		first := filepath.Join(outputDir, "page-001.jpg")
		if err := os.WriteFile(first, []byte("fresh-page"), 0o644); err != nil {
			t.Fatalf("WriteFile first: %v", err)
		}
		return []string{first}, nil
	})

	outputPath := filepath.Join(t.TempDir(), "record.jpg")
	stalePage := filepath.Join(filepath.Dir(outputPath), "record-page-002.jpg")
	if err := os.WriteFile(outputPath, []byte("old-page-1"), 0o644); err != nil {
		t.Fatalf("WriteFile outputPath: %v", err)
	}
	if err := os.WriteFile(stalePage, []byte("old-page-2"), 0o644); err != nil {
		t.Fatalf("WriteFile stalePage: %v", err)
	}

	if _, err := exportSvc.ExportSoldierJPG(outputPath, models.Soldier{DisplayID: "STC38-00001"}, PDFOptions{Orientation: "L", IncludeImages: true}); err != nil {
		t.Fatalf("ExportSoldierJPG: %v", err)
	}
	if _, err := os.Stat(stalePage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale sibling page should be removed, stat err = %v", err)
	}
}

func TestExportService_ExportSoldierJPGLeavesNoPartialOutputsOnRasterFailure(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	exportSvc.rasterizer = rasterizerFunc(func(pdfPath, outputDir string) ([]string, error) {
		first := filepath.Join(outputDir, "page-001.jpg")
		if err := os.WriteFile(first, []byte("partial"), 0o644); err != nil {
			t.Fatalf("WriteFile first: %v", err)
		}
		return nil, errors.New("render failed")
	})

	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "record.jpg")
	_, err := exportSvc.ExportSoldierJPG(outputPath, models.Soldier{DisplayID: "STC38-00001"}, PDFOptions{Orientation: "L", IncludeImages: true})
	if err == nil || !strings.Contains(err.Error(), "render failed") {
		t.Fatalf("ExportSoldierJPG err = %v, want render failed", err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("unexpected output files after failure: %v", entries)
	}
}

func TestImagePathForPDFSkipsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jpg")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := render.ImagePathForPDF(models.Image{ResolvedPath: path}); got != "" {
		t.Fatalf("imagePathForPDF returned %q for empty file", got)
	}
}

func TestExportService_ExportFullDatabasePDFCanLimitToSelectedRecords(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	first, err := soldierSvc.Create(models.Soldier{FirstName: "John", LastName: "Bell"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := soldierSvc.Create(models.Soldier{FirstName: "Mary", LastName: "Carter"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "selected-registry.pdf")
	settings := PrintSettings{ExportAll: false, SelectedIDs: []int64{second.ID}}
	if err := exportSvc.ExportFullDatabasePDF(outPath, settings); err != nil {
		t.Fatalf("ExportFullDatabasePDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "John Bell") {
		t.Fatalf("selected export should omit unselected record")
	}
	if !strings.Contains(text, "Mary Carter") {
		t.Fatalf("selected export missing selected record content")
	}
	if first.ID == second.ID {
		t.Fatalf("test setup failed: duplicate IDs")
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
	if strings.Contains(text, "Report Metadata") || !strings.Contains(text, "database-pdf v") {
		t.Fatalf("grouped PDF should use footer-only metadata")
	}
	if strings.Index(text, "Samuel Carter") > strings.Index(text, "James Adams") {
		t.Fatalf("records were not ordered by birth year within the grouped section")
	}
}

func TestExportService_ExportFullDatabasePDFGroupsByBurialLocation(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	for _, soldier := range []models.Soldier{
		{FirstName: "William", LastName: "Brown", BuriedIn: "", Unit: "A Company"},
		{FirstName: "James", LastName: "Adams", BuriedIn: "Oak Hill Cemetery, McAlester, OK", Unit: "B Company"},
		{FirstName: "Samuel", LastName: "Carter", BuriedIn: "Oak Hill Cemetery, McAlester, OK", Unit: "C Company"},
	} {
		if _, err := soldierSvc.Create(soldier); err != nil {
			t.Fatalf("Create soldier: %v", err)
		}
	}

	outPath := filepath.Join(t.TempDir(), "burial-grouped-registry.pdf")
	settings := PrintSettings{GroupByBuriedIn: true}
	if err := exportSvc.ExportFullDatabasePDF(outPath, settings); err != nil {
		t.Fatalf("ExportFullDatabasePDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "Grouped by Burial Location") || !strings.Contains(text, "Cemetery: Oak Hill Cemetery, McAlester, OK") {
		t.Fatalf("grouped PDF missing burial-location divider pages")
	}
	if !strings.Contains(text, "Cemetery: Location Unknown") {
		t.Fatalf("grouped PDF missing unknown burial-location section")
	}
	if strings.Contains(text, "Report Metadata") || !strings.Contains(text, "database-pdf v") {
		t.Fatalf("grouped PDF should use footer-only metadata")
	}
	if strings.Index(text, "James Adams") > strings.Index(text, "Samuel Carter") {
		t.Fatalf("records were not ordered alphabetically within the burial-location group")
	}
	if strings.Index(text, "Cemetery: Oak Hill Cemetery, McAlester, OK") < strings.Index(text, "Cemetery: Location Unknown") {
		// expected order
	} else {
		t.Fatalf("unknown burial-location section did not sort last")
	}
}

func TestExportService_ExportFullDatabasePDFFiltersByStructuredScope(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	for _, soldier := range []models.Soldier{
		{FirstName: "John", LastName: "Able", EntryType: "soldier", BuriedIn: "Oak Hill Cemetery", Unit: "A Company", PensionState: "Texas", ConfederateHomeStatus: "Trustee"},
		{FirstName: "Mary", LastName: "Baker", EntryType: "soldier", BuriedIn: "Oak Hill Cemetery", Unit: "C Company", PensionState: "Texas", ConfederateHomeStatus: "Trustee"},
		{FirstName: "Samuel", LastName: "Carter", EntryType: "soldier", BuriedIn: "Rose Hill Cemetery", Unit: "B Company", PensionState: "Texas", ConfederateHomeStatus: "Trustee"},
		{FirstName: "Mark", LastName: "Dunn", EntryType: "soldier", BuriedIn: "", Unit: "D Company", PensionState: "Texas", ConfederateHomeStatus: "Trustee"},
	} {
		if _, err := soldierSvc.Create(soldier); err != nil {
			t.Fatalf("Create soldier: %v", err)
		}
	}

	outPath := filepath.Join(t.TempDir(), "filtered-registry.pdf")
	settings := PrintSettings{
		Scope:                       PrintScopeFiltered,
		FilterBuriedIn:              []string{"Oak Hill Cemetery", render.PrintFilterUnknownValue},
		FilterEntryTypes:            []string{"soldier"},
		FilterUnits:                 []string{"A Company", "D Company"},
		FilterPensionStates:         []string{"Texas"},
		FilterConfederateHomeStatus: []string{"Trustee"},
	}
	if err := exportSvc.ExportFullDatabasePDF(outPath, settings); err != nil {
		t.Fatalf("ExportFullDatabasePDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	for _, needle := range []string{"John Able", "Mark Dunn"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("filtered PDF missing %q", needle)
		}
	}
	for _, forbidden := range []string{"Mary Baker", "Samuel Carter"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("filtered PDF should omit %q", forbidden)
		}
	}
}

func TestExportService_ExportFullDatabasePDFFilteredScopeWithoutFiltersFallsBackToAll(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	configureExportIdentity(t, d)

	for _, soldier := range []models.Soldier{
		{FirstName: "John", LastName: "Able"},
		{FirstName: "Mary", LastName: "Baker"},
	} {
		if _, err := soldierSvc.Create(soldier); err != nil {
			t.Fatalf("Create soldier: %v", err)
		}
	}

	outPath := filepath.Join(t.TempDir(), "filtered-fallback-all.pdf")
	if err := exportSvc.ExportFullDatabasePDF(outPath, PrintSettings{Scope: PrintScopeFiltered}); err != nil {
		t.Fatalf("ExportFullDatabasePDF: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	for _, needle := range []string{"John Able", "Mary Baker"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("filtered scope without filters should still include %q", needle)
		}
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

			_, _, gotWidth, gotHeight, ok := render.FitPDFImageToBounds(imagePath, 10, 20, test.maxWidth, test.maxHeight)
			if !ok {
				t.Fatalf("fitPDFImageToBounds returned ok=false")
			}
			if gotWidth != test.wantWidth || gotHeight != test.wantHeight {
				t.Fatalf("fitPDFImageToBounds = %.2fx%.2f, want %.2fx%.2f", gotWidth, gotHeight, test.wantWidth, test.wantHeight)
			}
		})
	}
}

func TestFirstRecordCardImagePrefersPrimaryImage(t *testing.T) {
	secondaryPath := filepath.Join(t.TempDir(), "secondary.png")
	writeSizedPNGFixture(t, secondaryPath, 120, 120)
	primaryPath := filepath.Join(t.TempDir(), "primary.png")
	writeSizedPNGFixture(t, primaryPath, 200, 100)

	path, label := render.FirstRecordCardImage(models.Soldier{
		Images: []models.Image{
			{FileName: "secondary.png", Caption: "Secondary", ResolvedPath: secondaryPath},
			{FileName: "primary.png", Caption: "Primary Portrait", ResolvedPath: primaryPath, IsPrimary: true},
		},
	}, false)
	if path != primaryPath || label != "Primary Portrait" {
		t.Fatalf("firstRecordCardImage = (%q, %q)", path, label)
	}
}

func TestFirstRecordCardImageDoesNotFallBackToFileName(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "primary.png")
	writeSizedPNGFixture(t, imagePath, 200, 100)

	path, label := render.FirstRecordCardImage(models.Soldier{
		Images: []models.Image{
			{FileName: "primary.png", ResolvedPath: imagePath, IsPrimary: true},
		},
	}, false)
	if path != imagePath || label != "" {
		t.Fatalf("firstRecordCardImage = (%q, %q)", path, label)
	}
}

func TestUsesPortraitCompactRecordCardLayout(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "portrait.png")
	writeSizedPNGFixture(t, imagePath, 200, 100)

	soldierWithImage := models.Soldier{
		Images: []models.Image{{
			FileName:     "portrait.png",
			FilePath:     `images\stc38-00001\portrait.png`,
			ResolvedPath: imagePath,
			Caption:      "Portrait",
			IsPrimary:    true,
		}},
	}

	if !render.UsesPortraitCompactRecordCardLayout(soldierWithImage, PDFOptions{Orientation: "P", IncludeImages: true}) {
		t.Fatalf("portrait layout with image should use compact portrait columns")
	}
	if render.UsesPortraitCompactRecordCardLayout(soldierWithImage, PDFOptions{Orientation: "L", IncludeImages: true}) {
		t.Fatalf("landscape layout should not use compact portrait columns")
	}
	if render.UsesPortraitCompactRecordCardLayout(soldierWithImage, PDFOptions{Orientation: "P", IncludeImages: false}) {
		t.Fatalf("portrait layout without images enabled should fall back to stacked layout")
	}
	if render.UsesPortraitCompactRecordCardLayout(models.Soldier{}, PDFOptions{Orientation: "P", IncludeImages: true}) {
		t.Fatalf("portrait layout without an image should fall back to stacked layout")
	}
}

func TestImagePathForPDFSkipsUnsupportedFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portrait.webp")
	if err := os.WriteFile(path, []byte("not-a-pdf-image"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := render.ImagePathForPDF(models.Image{ResolvedPath: path}); got != "" {
		t.Fatalf("imagePathForPDF returned %q for unsupported format", got)
	}
}

func TestImagePathForPDFSkipsCorruptPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.png")
	if err := os.WriteFile(path, []byte("not-a-real-png"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := render.ImagePathForPDF(models.Image{ResolvedPath: path}); got != "" {
		t.Fatalf("imagePathForPDF returned %q for corrupt PNG", got)
	}
}

func TestPDFTextSegments(t *testing.T) {
	segments := render.PDFTextSegments("See https://example.com/test, then http://example.org.")
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
	if err := exportSvc.ExportICalendar(outPath, models.DefaultCalendarEventPreferences()); err != nil {
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
	if !strings.Contains(text, "X-WR-TIMEZONE:"+buildinfo.CalendarTimeZone) {
		t.Fatalf("ics missing calendar timezone header: %q", text)
	}
	if !strings.Contains(text, "SUMMARY:Memorial Anniversary: John Smith") {
		t.Fatalf("ics missing summary: %q", text)
	}
	if !strings.Contains(normalizedText, "DESCRIPTION:Record ID: CSA-1") ||
		!strings.Contains(normalizedText, "\\nUnit: 1st Virginia") ||
		!strings.Contains(normalizedText, "\\nBuried In: Richmond") ||
		!strings.Contains(normalizedText, "\\nOriginal Death Date: April 9\\, 1862") {
		t.Fatalf("ics missing cleaned description: %q", text)
	}
	location, err := time.LoadLocation(buildinfo.CalendarTimeZone)
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	expectedStart := nextGoogleAnniversaryDate(models.Soldier{DeathMonth: 4, DeathDay: 9}, time.Now().In(location)).Format("20060102") + "T090000"
	if !strings.Contains(text, "DTSTART;TZID="+buildinfo.CalendarTimeZone+":"+expectedStart) {
		t.Fatalf("ics missing start date: %q", text)
	}
	if !strings.Contains(text, "DTEND;TZID="+buildinfo.CalendarTimeZone+":"+nextGoogleAnniversaryDate(models.Soldier{DeathMonth: 4, DeathDay: 9}, time.Now().In(location)).Format("20060102")+"T100000") {
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

func TestSoldierDisplayNameUsesVisibleNameFormatting(t *testing.T) {
	soldier := models.Soldier{
		EntryType:            "soldier",
		Prefix:               "Capt.",
		ShowPrefixBeforeName: false,
		FirstName:            "John",
		LastName:             "Smith",
		RankOut:              "Captain",
		Unit:                 "1st Virginia",
	}

	if got := peopleinfo.SoldierDisplayName(soldier); got != "John Smith" {
		t.Fatalf("peopleinfo.SoldierDisplayName() = %q, want %q", got, "John Smith")
	}
}

func TestRegistryEntryLinesUseServiceSummaryFormatting(t *testing.T) {
	lines := render.RegistryEntryLines(models.Soldier{
		EntryType: "soldier",
		RankOut:   "Captain",
		Unit:      "1st Virginia",
	})

	found := false
	for _, line := range lines {
		if line.Label == "Service Summary" {
			found = true
			if line.Value != "Captain 1st Virginia" {
				t.Fatalf("service summary = %q, want %q", line.Value, "Captain 1st Virginia")
			}
		}
	}
	if !found {
		t.Fatalf("registry entry lines missing service summary")
	}
}

func TestRegistryEntryLinesNormalizePensionStateNA(t *testing.T) {
	lines := render.RegistryEntryLines(models.Soldier{
		EntryType:    "soldier",
		PensionState: "None",
		PensionID:    "P-123",
	})

	for _, line := range lines {
		if line.Label == "Pension / Application" {
			if line.Value != "N/A | P-123" {
				t.Fatalf("pension summary = %q, want %q", line.Value, "N/A | P-123")
			}
			return
		}
	}
	t.Fatalf("registry entry lines missing pension summary")
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

func jpegFixture() []byte {
	imageRect := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			imageRect.Set(x, y, color.RGBA{R: 180, G: 120, B: 70, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, imageRect, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return buf.Bytes()
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
