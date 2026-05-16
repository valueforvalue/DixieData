package services

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/models"
)

func TestExportService_ExportJSON(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	_, _ = soldierSvc.Create(models.Soldier{FirstName: "Robert", LastName: "Lee", Rank: "General"})
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
}

func TestExportService_ExportCSV(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	_, _ = soldierSvc.Create(models.Soldier{FirstName: "P.G.T.", LastName: "Beauregard", Unit: "Army of the Potomac (CSA)"})
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
		"pension_id": true, "application_id": true, "middle_name": true,
		"last_name": true, "rank_in": true, "rank_out": true, "pension_state": true, "birth_date": true, "death_date": true, "buried_in": true,
	}
	for _, col := range header {
		delete(expected, col)
	}
	if len(expected) > 0 {
		t.Errorf("CSV missing columns: %v", expected)
	}
}

func TestExportService_ExportSoldierPDFForSpouseEntry(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	outPath := filepath.Join(t.TempDir(), "wife.pdf")
	err := exportSvc.ExportSoldierPDF(outPath, models.Soldier{
		DisplayID:     "TDM65-DXD-00002",
		EntryType:     "widow",
		FirstName:     "Martha",
		LastName:      "Taylor",
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
	if !strings.Contains(text, "Married To") || !strings.Contains(text, "John Taylor") {
		t.Fatalf("pdf missing spouse reference")
	}
	if !strings.Contains(text, "Maiden Name") || !strings.Contains(text, "Cole") {
		t.Fatalf("pdf missing maiden name")
	}
	if !strings.Contains(text, "Pension ID") || !strings.Contains(text, "WP-42") || !strings.Contains(text, "Application ID") || !strings.Contains(text, "WA-42") {
		t.Fatalf("pdf missing widow pension identifiers")
	}
}

func TestExportService_ExportImages(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	tempDir := t.TempDir()
	firstPath := filepath.Join(tempDir, "front.png")
	secondPath := filepath.Join(tempDir, "back.png")
	if err := os.WriteFile(firstPath, []byte("front-image"), 0o644); err != nil {
		t.Fatalf("write first image: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("back-image"), 0o644); err != nil {
		t.Fatalf("write second image: %v", err)
	}

	outPath := filepath.Join(tempDir, "images.zip")
	err := exportSvc.ExportImages(outPath, []models.Image{
		{FileName: "front.png", FilePath: firstPath},
		{FileName: "back.png", FilePath: secondPath},
	})
	if err != nil {
		t.Fatalf("ExportImages: %v", err)
	}

	reader, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	if len(reader.File) != 2 {
		t.Fatalf("zip contains %d files, want 2", len(reader.File))
	}

	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	if !names["front.png"] || !names["back.png"] {
		t.Fatalf("zip missing expected image names: %v", names)
	}
}

func TestExportService_ExportSoldierPDF(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

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
	if !strings.Contains(text, "DB Path: images\\\\pension-42\\\\portrait.png") {
		t.Fatalf("pdf missing image db path: %q", text)
	}
	if !strings.Contains(text, "Full Path:") || !strings.Contains(text, "portrait.png") {
		t.Fatalf("pdf missing full image path")
	}
	if !strings.Contains(text, "Hollywood Cemetery") {
		t.Fatalf("pdf missing buried-in text")
	}
	if !strings.Contains(text, "Includes Images") || !strings.Contains(text, "true") {
		t.Fatalf("pdf missing report metadata")
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
	if strings.Contains(text, "DB Path:") || strings.Contains(text, "portrait.png") {
		t.Fatalf("image details should be omitted from no-images PDF")
	}
	if !strings.Contains(text, "Includes Images") || !strings.Contains(text, "false") {
		t.Fatalf("pdf missing no-images metadata")
	}
}

func TestExportService_ExportMonthlyAnniversaryPDF(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

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

func TestImagePathForPDFSkipsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jpg")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := imagePathForPDF(models.Image{ResolvedPath: path}); got != "" {
		t.Fatalf("imagePathForPDF returned %q for empty file", got)
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
	if !strings.Contains(text, "BEGIN:VCALENDAR") || !strings.Contains(text, "END:VCALENDAR") {
		t.Fatalf("ics missing calendar wrapper: %q", text)
	}
	if !strings.Contains(text, "SUMMARY:DixieData Anniversary: John Smith") {
		t.Fatalf("ics missing summary: %q", text)
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
