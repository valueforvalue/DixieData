package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestAppServeHTTPStartupError(t *testing.T) {
	app := NewApp()
	app.startupErr = errors.New("startup failed")
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "startup failed") {
		t.Fatalf("expected startup error in response, got %q", rec.Body.String())
	}
}

func TestParseSoldierFormRejectsInvalidMonth(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soldiers", strings.NewReader(url.Values{
		"death_month": {"13"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := parseSoldierForm(req, 0)
	if err == nil || !strings.Contains(err.Error(), "death_month") {
		t.Fatalf("expected death_month validation error, got %v", err)
	}
}

func TestParseSoldierFormIncludesBurialAndRecords(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soldiers", strings.NewReader(url.Values{
		"buried_in":      {"Hollywood Cemetery"},
		"record_type":    {"Service Record", "Burial Ledger"},
		"record_app_id":  {"APP-1", "APP-2"},
		"record_details": {"Filed with the regiment.", "Interred in Richmond."},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	soldier, err := parseSoldierForm(req, 0)
	if err != nil {
		t.Fatalf("parseSoldierForm: %v", err)
	}
	if soldier.BuriedIn != "Hollywood Cemetery" {
		t.Fatalf("BuriedIn = %q", soldier.BuriedIn)
	}
	if len(soldier.Records) != 2 {
		t.Fatalf("records len = %d", len(soldier.Records))
	}
	if soldier.Records[1].RecordType != "Burial Ledger" {
		t.Fatalf("second record type = %q", soldier.Records[1].RecordType)
	}
}

func TestParseBoundedIntRejectsInvalidValues(t *testing.T) {
	if _, err := parseBoundedInt("0", "month", 1, 12); err == nil {
		t.Fatal("expected invalid month error")
	}
	if _, err := parseBoundedInt("abc", "month", 1, 12); err == nil {
		t.Fatal("expected invalid month parse error")
	}
}

func TestSelectedSoldierImagesUsesSelectedIDs(t *testing.T) {
	soldier := models.Soldier{
		Images: []models.Image{
			{ID: 4, FileName: "front.png", FilePath: `images\record-1\front.png`},
			{ID: 7, FileName: "back.png", FilePath: `images\record-1\back.png`},
		},
	}

	images, err := selectedSoldierImages(soldier, []string{"7"}, `C:\Development\DixieData\.dixiedata`)
	if err != nil {
		t.Fatalf("selectedSoldierImages: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("selected %d images, want 1", len(images))
	}
	if images[0].ID != 7 {
		t.Fatalf("selected image id = %d, want 7", images[0].ID)
	}
	if images[0].FilePath != filepath.Join(`C:\Development\DixieData\.dixiedata`, `images\record-1\back.png`) {
		t.Fatalf("resolved file path = %q", images[0].FilePath)
	}
}

func TestImageArchiveNameUsesDisplayID(t *testing.T) {
	name := imageArchiveName(models.Soldier{DisplayID: "PENSION 42"})
	if name != "PENSION-42-images.zip" {
		t.Fatalf("archive name = %q", name)
	}
}

func TestImageScreenshotNameUsesImageFileName(t *testing.T) {
	name := imageScreenshotName("record 42 front.png")
	if name != "record-42-front-screenshot.png" {
		t.Fatalf("screenshot name = %q", name)
	}
}

func TestSoldierPDFNameUsesDisplayID(t *testing.T) {
	name := soldierPDFName(models.Soldier{DisplayID: "DD 100"})
	if name != "DD-100.pdf" {
		t.Fatalf("soldier pdf name = %q", name)
	}
}

func TestMonthPDFNameUsesMonthName(t *testing.T) {
	name := monthPDFName(4)
	if name != "April-report.pdf" {
		t.Fatalf("month pdf name = %q", name)
	}
}

func TestBackupArchiveNameIncludesDate(t *testing.T) {
	name := backupArchiveName(time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC))
	if name != "dixiedata-backup-2026-04-28.zip" {
		t.Fatalf("backup archive name = %q", name)
	}
}

func TestExportLinkMarkupIncludesFileURL(t *testing.T) {
	markup := exportLinkMarkup("PDF ready:", `C:\Development\DixieData\build\bin\DixieData.pdf`)
	if !strings.Contains(markup, `file:///C:/Development/DixieData/build/bin/DixieData.pdf`) {
		t.Fatalf("markup missing file URL: %q", markup)
	}
	if !strings.Contains(markup, `C:\Development\DixieData\build\bin\DixieData.pdf`) {
		t.Fatalf("markup missing display path: %q", markup)
	}
}

func TestLoadQuotesParsesEmbeddedShape(t *testing.T) {
	quotes, err := loadQuotes([]byte(`{"civil_war_quotes":[{"author":"A","text":"B","context":"C"}]}`))
	if err != nil {
		t.Fatalf("loadQuotes: %v", err)
	}
	if len(quotes) != 1 || quotes[0].Author != "A" || quotes[0].Text != "B" {
		t.Fatalf("quotes = %#v", quotes)
	}
}

func TestSelectQuoteOfDayUsesDayOfYear(t *testing.T) {
	quote := selectQuoteOfDay([]models.Quote{
		{Author: "One"},
		{Author: "Two"},
		{Author: "Three"},
	}, time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC))
	if quote.Author != "Two" {
		t.Fatalf("quote author = %q", quote.Author)
	}
}
