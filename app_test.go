package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/db"
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

func TestAppServeHTTPMethodOverride(t *testing.T) {
	app := NewApp()
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/override", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Method))
	})

	req := httptest.NewRequest(http.MethodPost, "/override", nil)
	req.Header.Set("X-HTTP-Method-Override", http.MethodPut)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != http.MethodPut {
		t.Fatalf("method=%q want %q", rec.Body.String(), http.MethodPut)
	}
}

func TestAppServeHTTPMethodOverrideFromFormValue(t *testing.T) {
	app := NewApp()
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/override", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Method))
	})

	req := httptest.NewRequest(http.MethodPost, "/override", strings.NewReader(url.Values{
		"_method": {http.MethodPut},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != http.MethodPut {
		t.Fatalf("method=%q want %q", rec.Body.String(), http.MethodPut)
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
		"display_id":     {"DXD-00001"},
		"middle_name":    {"Thomas"},
		"rank_in":        {"Private"},
		"rank_out":       {"Captain"},
		"pension_state":  {"Virginia"},
		"pension_id":     {"P12345"},
		"application_id": {"A12345"},
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
	if soldier.DisplayID != "DXD-00001" || soldier.MiddleName != "Thomas" || soldier.RankIn != "Private" || soldier.RankOut != "Captain" || soldier.PensionState != "Virginia" || soldier.PensionID != "P12345" || soldier.ApplicationID != "A12345" {
		t.Fatalf("unexpected parsed fields: %#v", soldier)
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
	if !strings.Contains(markup, `data-open-external="true"`) {
		t.Fatalf("markup missing external-link flag: %q", markup)
	}
	if !strings.Contains(markup, `C:\Development\DixieData\build\bin\DixieData.pdf`) {
		t.Fatalf("markup missing display path: %q", markup)
	}
}

func TestNormalizeChromeOpenTarget(t *testing.T) {
	if got, err := normalizeChromeOpenTarget("https://example.com"); err != nil || got != "https://example.com" {
		t.Fatalf("https target = %q, %v", got, err)
	}
	if got, err := normalizeChromeOpenTarget(`C:\Users\value\OneDrive\Documents\CSA-000226.pdf`); err != nil || got != "file:///C:/Users/value/OneDrive/Documents/CSA-000226.pdf" {
		t.Fatalf("file target = %q, %v", got, err)
	}
	if _, err := normalizeChromeOpenTarget("not-a-link"); err == nil {
		t.Fatal("expected invalid target error")
	}
}

func TestIsFileOpenTarget(t *testing.T) {
	if !isFileOpenTarget("file:///C:/Users/value/OneDrive/Documents/CSA-000226.pdf") {
		t.Fatal("expected file URL to use system opener")
	}
	if isFileOpenTarget("https://example.com") {
		t.Fatal("expected web URL to stay on Chrome path")
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

func TestInitializeLocalDataRecreatesFreshArchive(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}

	if _, err := app.soldiers.Create(models.Soldier{FirstName: "Demo", LastName: "Soldier"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	imageDir := filepath.Join(dataDir, "images", "demo")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(imageDir, "sample.txt"), []byte("demo"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := app.initializeLocalData(); err != nil {
		t.Fatalf("initializeLocalData: %v", err)
	}
	defer app.shutdown(context.Background())

	soldiers, total, err := app.soldiers.List(1, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(soldiers) != 0 || total != 0 {
		t.Fatalf("expected fresh archive, got %d soldiers total=%d", len(soldiers), total)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "images", "demo", "sample.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected image tree to be removed, stat err=%v", err)
	}
}

func TestSaveUploadedImagesAcceptsMultipleFiles(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}

	created, err := app.soldiers.Create(models.Soldier{DisplayID: "CSA-TEST", FirstName: "Test", LastName: "Soldier"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	body, contentType := multipartRequestBody(t, map[string][]byte{
		"first.jpg":  pngFixture(),
		"second.png": pngFixture(),
	})
	req := httptest.NewRequest(http.MethodPost, "/soldiers", body)
	req.Header.Set("Content-Type", contentType)
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}

	if err := app.saveUploadedImages(req, *created); err != nil {
		t.Fatalf("saveUploadedImages: %v", err)
	}

	soldier, err := app.soldiers.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(soldier.Images) != 2 {
		t.Fatalf("images len = %d, want 2", len(soldier.Images))
	}
	if soldier.Images[0].FileName != "CSA-TEST-img-001.jpg" || soldier.Images[1].FileName != "CSA-TEST-img-002.png" {
		t.Fatalf("unexpected stored image names: %#v", soldier.Images)
	}
}

func TestSaveUploadedImagesDoesNotTrustZeroHeaderSize(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}

	created, err := app.soldiers.Create(models.Soldier{DisplayID: "CSA-HEADER", FirstName: "Header", LastName: "Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	body, contentType := multipartRequestBody(t, map[string][]byte{
		"timesheet.jpeg": pngFixture(),
	})
	req := httptest.NewRequest(http.MethodPost, "/soldiers", body)
	req.Header.Set("Content-Type", contentType)
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	req.MultipartForm.File["images"][0].Size = 0

	if err := app.saveUploadedImages(req, *created); err != nil {
		t.Fatalf("saveUploadedImages with zero header size: %v", err)
	}

	soldier, err := app.soldiers.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(soldier.Images) != 1 {
		t.Fatalf("images len = %d, want 1", len(soldier.Images))
	}
	if soldier.Images[0].FileName != "CSA-HEADER-img-001.jpeg" {
		t.Fatalf("stored image name = %q", soldier.Images[0].FileName)
	}
}

func TestImportImagePathsCopiesMultipleFiles(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}

	created, err := app.soldiers.Create(models.Soldier{DisplayID: "CSA-NATIVE", FirstName: "Native", LastName: "Import"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sourceDir := t.TempDir()
	firstPath := filepath.Join(sourceDir, "first.jpeg")
	secondPath := filepath.Join(sourceDir, "second.png")
	if err := os.WriteFile(firstPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(secondPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}

	imported, err := app.importImagePaths(*created, []string{firstPath, secondPath})
	if err != nil {
		t.Fatalf("importImagePaths: %v", err)
	}
	if imported != 2 {
		t.Fatalf("imported = %d, want 2", imported)
	}

	soldier, err := app.soldiers.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(soldier.Images) != 2 {
		t.Fatalf("images len = %d, want 2", len(soldier.Images))
	}
	if soldier.Images[0].FileName != "CSA-NATIVE-img-001.jpeg" || soldier.Images[1].FileName != "CSA-NATIVE-img-002.png" {
		t.Fatalf("unexpected imported image names: %#v", soldier.Images)
	}
	for _, image := range soldier.Images {
		fullPath := filepath.Join(dataDir, filepath.FromSlash(image.FilePath))
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("Stat %s: %v", fullPath, err)
		}
		if info.Size() == 0 {
			t.Fatalf("copied file %s is empty", fullPath)
		}
	}
}

func TestNextStoredImageSequence(t *testing.T) {
	recordDir := t.TempDir()
	for _, name := range []string{"DXD-00001-img-001.jpg", "DXD-00001-img-003.png", "legacy-file-name.jpeg"} {
		if err := os.WriteFile(filepath.Join(recordDir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	next, err := nextStoredImageSequence(recordDir, "DXD-00001")
	if err != nil {
		t.Fatalf("nextStoredImageSequence: %v", err)
	}
	if next != 4 {
		t.Fatalf("next sequence = %d, want 4", next)
	}
}

func TestRotateImageFileClockwise(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.jpg")
	if err := writeJPEGFixture(path, 2, 3); err != nil {
		t.Fatalf("writeJPEGFixture: %v", err)
	}

	if err := rotateImageFile(path, true); err != nil {
		t.Fatalf("rotateImageFile: %v", err)
	}

	width, height, err := imageDimensions(path)
	if err != nil {
		t.Fatalf("imageDimensions: %v", err)
	}
	if width != 3 || height != 2 {
		t.Fatalf("dimensions = %dx%d, want 3x2", width, height)
	}
}

func TestImageImportRedirectPath(t *testing.T) {
	if got := imageImportRedirectPath(42, "edit"); got != "/soldiers/42/edit" {
		t.Fatalf("edit redirect = %q", got)
	}
	if got := imageImportRedirectPath(42, "detail"); got != "/soldiers/42" {
		t.Fatalf("default redirect = %q", got)
	}
}

func TestHandleUpdateSoldierRendersFormErrorOnUploadFailure(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{DisplayID: "CSA-TEST", FirstName: "Edit", LastName: "Target"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("display_id", created.DisplayID); err != nil {
		t.Fatalf("WriteField display_id: %v", err)
	}
	if err := writer.WriteField("first_name", created.FirstName); err != nil {
		t.Fatalf("WriteField first_name: %v", err)
	}
	if err := writer.WriteField("last_name", created.LastName); err != nil {
		t.Fatalf("WriteField last_name: %v", err)
	}
	part, err := writer.CreateFormFile("images", "timesheet.jpeg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Copy empty file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/soldiers/1", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-HTTP-Method-Override", http.MethodPut)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "Save failed.") || !strings.Contains(rec.Body.String(), "timesheet.jpeg") {
		t.Fatalf("expected inline upload error form, got %q", rec.Body.String())
	}
}

func multipartRequestBody(t *testing.T, files map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, data := range files {
		part, err := writer.CreateFormFile("images", name)
		if err != nil {
			t.Fatalf("CreateFormFile %s: %v", name, err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatalf("Write form file %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}

func pngFixture() []byte {
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0x99, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0xC9, 0xFE, 0x92,
		0xEF, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
		0x44, 0xAE, 0x42, 0x60, 0x82,
	}
}

func writeJPEGFixture(path string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(20 * x), G: uint8(40 * y), B: 120, A: 255})
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return jpeg.Encode(file, img, &jpeg.Options{Quality: 90})
}

func imageDimensions(path string) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	img, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}
	return img.Width, img.Height, nil
}
