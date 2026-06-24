package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/update"
)

type scratchpadStub struct {
	displayID string
	seed      string
	err       error
}

func (s *scratchpadStub) Open(displayID, seed string) error {
	s.displayID = displayID
	s.seed = seed
	return s.err
}

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

func TestAppServeHTTPStartupPlaceholderAutoRefreshesWithoutMux(t *testing.T) {
	app := NewApp()

	req := httptest.NewRequest(http.MethodGet, "/calendar?scope=recent", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusAccepted)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Loading DixieData...") {
		t.Fatalf("expected loading placeholder, got %q", body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("cache-control=%q", got)
	}
	if got := rec.Header().Get("Refresh"); got != "1; url=/calendar?_dd_boot=1&scope=recent" {
		t.Fatalf("refresh header=%q", got)
	}
	if !strings.Contains(body, `<meta http-equiv="refresh" content="1;url=/calendar?_dd_boot=1&amp;scope=recent">`) {
		t.Fatalf("expected meta refresh, got %q", body)
	}
	if !strings.Contains(body, `window.location.replace("/calendar?_dd_boot=1\u0026scope=recent")`) {
		t.Fatalf("expected inline auto-refresh script, got %q", body)
	}
}

func TestAppServeHTTPStartupServesFrontendAssetsWithoutMux(t *testing.T) {
	app := NewApp().WithFrontendAssets(os.DirFS(repoFixturePath(t, "frontend")))

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("content-type=%q", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "(() =>") {
		t.Fatalf("expected frontend app.js body, got %q", body)
	}
}

func TestAppServeHTTPRedirectsToRecoveryWhenPending(t *testing.T) {
	app := NewApp()
	app.pendingRecovery = &update.RestorePointRecord{ID: "restore-point-1"}
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/recovery" {
		t.Fatalf("location=%q want %q", location, "/recovery")
	}
}

func TestAppServeHTTPAllowsFrontendJSWhenRecoveryPending(t *testing.T) {
	app := NewApp().WithFrontendAssets(os.DirFS(repoFixturePath(t, "frontend")))
	app.pendingRecovery = &update.RestorePointRecord{ID: "restore-point-1"}
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("content-type=%q", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "(() =>") {
		t.Fatalf("expected frontend app.js body, got %q", body)
	}
}

func TestHandleRecoveryRendersRestorePointPrompt(t *testing.T) {
	app := NewApp()
	app.pendingRecovery = &update.RestorePointRecord{
		ID:               "restore-point-1",
		CreatedAt:        "2026-05-31T08:15:04Z",
		SourceAppVersion: "1.2.27",
		TargetAppVersion: "1.2.28",
	}
	app.recoveryFailure = "failed to open database"
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/recovery", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"Update recovery",
		"Restore previous build and Local Archive",
		"failed to open database",
		"v1.2.27",
		"v1.2.28",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("recovery page missing %q: %q", needle, body)
		}
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

func TestHandleVersionReturnsBuildMetadata(t *testing.T) {
	app := NewApp()
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"app":"DixieData"`) || !strings.Contains(rec.Body.String(), `"build_identity"`) {
		t.Fatalf("version body = %q", rec.Body.String())
	}
}

func TestParsePrintSettingsRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/export/database-pdf", strings.NewReader(url.Values{
		"scope":                            {"all"},
		"full_biography_page":              {"1"},
		"sort_by":                          {"birth_year"},
		"group_by_unit":                    {"1"},
		"group_by_confederate_home_status": {"1"},
		"group_by_buried_in":               {"1"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	settings, err := parsePrintSettingsRequest(req)
	if err != nil {
		t.Fatalf("parsePrintSettingsRequest: %v", err)
	}
	if settings.SortBy != archive.PrintSortBirthYear {
		t.Fatalf("SortBy = %q", settings.SortBy)
	}
	if !settings.GroupByUnit || !settings.GroupByConfederateHomeStatus || !settings.GroupByBuriedIn || settings.GroupByPensionState {
		t.Fatalf("unexpected parsed settings: %#v", settings)
	}
	if !settings.FullBiographyPage {
		t.Fatalf("expected full biography page flag, got %#v", settings)
	}
	if settings.Scope != archive.PrintScopeAll || !settings.ExportAll || len(settings.SelectedIDs) != 0 {
		t.Fatalf("unexpected export scope: %#v", settings)
	}
}

func TestParsePrintSettingsRequestForSelectedRecords(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/export/database-pdf", strings.NewReader(url.Values{
		"scope":        {archive.PrintScopeSelected},
		"sort_by":      {"last_name"},
		"selected_ids": {"4", "8"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	settings, err := parsePrintSettingsRequest(req)
	if err != nil {
		t.Fatalf("parsePrintSettingsRequest: %v", err)
	}
	if settings.Scope != archive.PrintScopeSelected || settings.ExportAll {
		t.Fatalf("expected selected-record export, got %#v", settings)
	}
	if !reflect.DeepEqual(settings.SelectedIDs, []int64{4, 8}) {
		t.Fatalf("SelectedIDs = %#v", settings.SelectedIDs)
	}
}

func TestParsePrintSettingsRequestForFilteredScope(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/export/database-pdf", strings.NewReader(url.Values{
		"scope":                          {archive.PrintScopeFiltered},
		"filter_buried_in":               {"Oak Hill Cemetery", "__unknown__"},
		"filter_entry_type":              {"soldier"},
		"filter_pension_state":           {"Texas"},
		"filter_confederate_home_status": {"Trustee"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	settings, err := parsePrintSettingsRequest(req)
	if err != nil {
		t.Fatalf("parsePrintSettingsRequest: %v", err)
	}
	if settings.Scope != archive.PrintScopeFiltered || settings.ExportAll {
		t.Fatalf("expected filtered export scope, got %#v", settings)
	}
	if !reflect.DeepEqual(settings.FilterBuriedIn, []string{"Oak Hill Cemetery"}) {
		t.Fatalf("FilterBuriedIn = %#v", settings.FilterBuriedIn)
	}
	if !reflect.DeepEqual(settings.FilterEntryTypes, []string{"soldier"}) {
		t.Fatalf("FilterEntryTypes = %#v", settings.FilterEntryTypes)
	}
	if !reflect.DeepEqual(settings.FilterPensionStates, []string{"Texas"}) {
		t.Fatalf("FilterPensionStates = %#v", settings.FilterPensionStates)
	}
	if !reflect.DeepEqual(settings.FilterConfederateHomeStatuses, []string{"Trustee"}) {
		t.Fatalf("FilterConfederateHomeStatuses = %#v", settings.FilterConfederateHomeStatuses)
	}
}

func TestHandleCalendarDefaultsToCurrentMonth(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	currentMonth := time.Now().Format("January")
	if !strings.Contains(rec.Body.String(), currentMonth) {
		t.Fatalf("calendar body should default to current month %q, got %q", currentMonth, rec.Body.String())
	}
}

func TestHandleAdvancedSearchByDeathYearMatchesFullDeathDate(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	if _, err := app.soldiers.Create(models.Soldier{
		DisplayID: "PENSION-1864",
		FirstName: "Robert",
		LastName:  "Taylor",
		DeathDate: "05/06/1864",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/soldiers/search/advanced?death_year=1864", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Taylor") {
		t.Fatalf("advanced search body = %q", rec.Body.String())
	}
}

func TestParseSoldierFormRejectsInvalidDeathDate(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soldiers", strings.NewReader(url.Values{
		"death_date": {"13/99/1886"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := parseSoldierForm(req, 0)
	if err == nil || !strings.Contains(err.Error(), "death_date") {
		t.Fatalf("expected death_date validation error, got %v", err)
	}
}

func TestParseSoldierFormIncludesBurialAndRecords(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soldiers", strings.NewReader(url.Values{
		"display_id":     {"DXD-00001"},
		"birth_date":     {"11/00/1836"},
		"death_date":     {"05/12/1909"},
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
	if soldier.DisplayID != "DXD-00001" || soldier.MiddleName != "Thomas" || soldier.RankIn != "Private" || soldier.RankOut != "Captain" || soldier.PensionState != "Virginia" || soldier.PensionID != "P12345" || soldier.ApplicationID != "A12345" || soldier.BirthDate != "11/00/1836" || soldier.DeathDate != "05/12/1909" {
		t.Fatalf("unexpected parsed fields: %#v", soldier)
	}
	if len(soldier.Records) != 2 {
		t.Fatalf("records len = %d", len(soldier.Records))
	}
	if soldier.Records[1].RecordType != "Burial Ledger" {
		t.Fatalf("second record type = %q", soldier.Records[1].RecordType)
	}
}

func TestParseSoldierFormIncludesSpouseFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soldiers", strings.NewReader(url.Values{
		"display_id":        {"DXD-00001"},
		"entry_type":        {"widow"},
		"spouse_soldier_id": {"4"},
		"maiden_name":       {"Taylor"},
		"pension_id":        {"WP-42"},
		"application_id":    {"WA-42"},
		"first_name":        {"Mary"},
		"last_name":         {"Jones"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	soldier, err := parseSoldierForm(req, 12)
	if err != nil {
		t.Fatalf("parseSoldierForm: %v", err)
	}
	if soldier.ID != 12 || soldier.EntryType != "widow" || soldier.SpouseSoldierID != 4 || soldier.MaidenName != "Taylor" || soldier.PensionID != "WP-42" || soldier.ApplicationID != "WA-42" {
		t.Fatalf("unexpected spouse fields: %#v", soldier)
	}
}

func TestHandleEditSoldierPreselectsLinkedSpouse(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	spouse, err := app.soldiers.Create(models.Soldier{
		DisplayID: "DXD-00064",
		FirstName: "George",
		LastName:  "Barnett",
	})
	if err != nil {
		t.Fatalf("Create spouse: %v", err)
	}
	widow, err := app.soldiers.Create(models.Soldier{
		DisplayID:       "DXD-00063",
		EntryType:       "widow",
		SpouseSoldierID: spouse.ID,
		FirstName:       "Missouri",
		LastName:        "Barnett",
	})
	if err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/soldiers/%d/edit", widow.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="spouse_soldier_id"`) {
		t.Fatalf("expected spouse selector in edit form, got %q", body)
	}
	expectedOption := fmt.Sprintf(`<option value="%d" selected>George Barnett (DXD-00064)</option>`, spouse.ID)
	if !strings.Contains(body, expectedOption) {
		t.Fatalf("expected linked spouse option %q in edit form, got %q", expectedOption, body)
	}
}

func TestHandleScrapeFindAGravePopulatesNewSoldierForm(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	source, err := os.ReadFile(repoFixturePath(t, "tests", "testdata", "findagrave-source.html"))
	if err != nil {
		t.Fatalf("ReadFile source: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/soldiers/scrape-findagrave", strings.NewReader(url.Values{
		"findagrave_source": {string(source)},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="first_name" value="Elbert"`) || !strings.Contains(body, `name="middle_name" value="Dixon"`) {
		t.Fatalf("scrape response missing populated name fields: %q", body)
	}
	if !strings.Contains(body, `name="buried_in" value="Antioch Cemetery, Woodruff, Spartanburg County, South Carolina, USA"`) {
		t.Fatalf("scrape response missing burial field: %q", body)
	}
	if !strings.Contains(body, "Review scraped data carefully before saving.") || !strings.Contains(body, "Harriet Clement Anderson") {
		t.Fatalf("scrape response missing warnings/spouse preview: %q", body)
	}
}

func TestHandleScratchpadOpenLaunchesNativeScratchpad(t *testing.T) {
	app := NewApp()
	stub := &scratchpadStub{}
	app.scratchpads = stub

	req := httptest.NewRequest(http.MethodPost, "/scratchpad/open", strings.NewReader(url.Values{
		"display_id":      {"DXD-00001"},
		"scratchpad_seed": {"legacy note"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.handleScratchpadOpen(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if stub.displayID != "DXD-00001" || stub.seed != "legacy note" {
		t.Fatalf("stub got (%q, %q)", stub.displayID, stub.seed)
	}
	if !strings.Contains(rec.Body.String(), "DXD-00001") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestHandleScratchpadOpenRequiresDisplayID(t *testing.T) {
	app := NewApp()
	app.scratchpads = &scratchpadStub{}

	req := httptest.NewRequest(http.MethodPost, "/scratchpad/open", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.handleScratchpadOpen(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusBadRequest)
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

func TestImageExportFolderNameUsesDisplayID(t *testing.T) {
	name := imageExportFolderName(models.Soldier{DisplayID: "PENSION 42"})
	if name != "PENSION-42_Images" {
		t.Fatalf("folder name = %q", name)
	}
}

func TestImageScreenshotNameUsesImageFileName(t *testing.T) {
	name := imageScreenshotName("record 42 front.png")
	if name != "record-42-front-screenshot.png" {
		t.Fatalf("screenshot name = %q", name)
	}
}

func TestSoldierPDFNameUsesDisplayID(t *testing.T) {
	name := soldierPDFName(models.Soldier{DisplayID: "DD 100"}, archive.PDFOptions{Orientation: "L", IncludeImages: true})
	if name != "DD-100-landscape.pdf" {
		t.Fatalf("soldier pdf name = %q", name)
	}
}

func TestSoldierJPGNameIncludesPDFOptionSuffix(t *testing.T) {
	name := soldierJPGName(models.Soldier{DisplayID: "DD 100"}, archive.PDFOptions{Orientation: "L", PrinterFriendly: true, IncludeImages: false})
	if name != "DD-100-printer-friendly-landscape-no-images.jpg" {
		t.Fatalf("soldier jpg name = %q", name)
	}
}

func TestSoldierPDFNameNoImagesUsesDisplayID(t *testing.T) {
	name := soldierPDFNameNoImages(models.Soldier{DisplayID: "DD 100"})
	if name != "DD-100-landscape-no-images.pdf" {
		t.Fatalf("soldier pdf name = %q", name)
	}
}

func TestMonthPDFNameUsesMonthName(t *testing.T) {
	name := monthPDFName(4, archive.PDFOptions{Orientation: "P"})
	if name != "April-report-portrait.pdf" {
		t.Fatalf("month pdf name = %q", name)
	}
}

func TestPDFOptionFilenameSuffixIncludesPrinterFriendlyLandscape(t *testing.T) {
	suffix := pdfOptionFilenameSuffix(archive.PDFOptions{Orientation: "L", PrinterFriendly: true, IncludeImages: true}, false)
	if suffix != "printer-friendly-landscape" {
		t.Fatalf("suffix = %q", suffix)
	}
}

func TestPrintableArchivePDFNameIncludesPrinterFriendlyLandscape(t *testing.T) {
	name := printableArchivePDFName(archive.PrintSettings{Orientation: "L", PrinterFriendly: true})
	if name != "dixiedata-printable-archive-printer-friendly-landscape.pdf" {
		t.Fatalf("printable archive pdf name = %q", name)
	}
}

func TestPrintableArchivePDFNameIncludesFullBiographySuffix(t *testing.T) {
	name := printableArchivePDFName(archive.PrintSettings{Orientation: "L", FullBiographyPage: true})
	if name != "dixiedata-printable-archive-landscape-full-biography.pdf" {
		t.Fatalf("printable archive pdf name = %q", name)
	}
}

func TestBackupArchiveNameIncludesDate(t *testing.T) {
	name := backupArchiveName(time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC))
	if name != "dixiedata-backup-2026-04-28.ddbak" {
		t.Fatalf("backup archive name = %q", name)
	}
}

func TestSharedArchiveNameIncludesDate(t *testing.T) {
	name := sharedArchiveName(time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC))
	if name != "dixiedata-shared-2026-04-28.ddshare" {
		t.Fatalf("shared archive name = %q", name)
	}
}

func TestServeHTTPServesFrontendAssets(t *testing.T) {
	app := NewApp().WithFrontendAssets(os.DirFS(repoFixturePath(t, "frontend")))
	app.setupRoutes()

	for _, path := range []string{"/app.js", "/app.css"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %q", path, rec.Code, rec.Body.String())
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%s body should not be empty", path)
		}
	}
	if !strings.Contains(recorderBodyForPath(t, app, "/app.js"), `const timers = new WeakMap();`) {
		t.Fatalf("/app.js should serve the frontend bootstrap script")
	}
}

func TestServeHTTPServesFrontendAssetsFromProjectRootFallback(t *testing.T) {
	app := NewApp()
	app.setupRoutes()

	for _, path := range []string{"/app.js", "/app.css"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %q", path, rec.Code, rec.Body.String())
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%s body should not be empty", path)
		}
	}
}

func recorderBodyForPath(t *testing.T, app *App, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec.Body.String()
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

func TestSelectQuoteForArchiveRotatesEveryThreeSoldiers(t *testing.T) {
	quote := selectQuoteForArchive([]models.Quote{
		{Author: "One"},
		{Author: "Two"},
		{Author: "Three"},
	}, 4)
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
	if !app.setupRequired {
		t.Fatal("expected fresh archive to require initial setup")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "images", "demo", "sample.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected image tree to be removed, stat err=%v", err)
	}
}

func TestAppServeHTTPRedirectsToInitialSetupWhenRequired(t *testing.T) {
	app := NewApp()
	app.setupRequired = true
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/setup" {
		t.Fatalf("redirect location = %q", location)
	}
}

func TestAppServeHTTPAllowsFrontendCSSWhenSetupRequired(t *testing.T) {
	app := NewApp().WithFrontendAssets(os.DirFS(repoFixturePath(t, "frontend")))
	app.setupRequired = true
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/app.css", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Fatalf("content-type=%q", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "--tw-border-spacing-x") {
		t.Fatalf("expected frontend app.css body, got %q", body)
	}
}

func TestHandleUpdateBootstrapHealthClearsPendingLaunchState(t *testing.T) {
	dataDir := t.TempDir()
	manager := update.NewRestorePointManager(dataDir)
	if err := manager.SaveLaunchState(update.RestorePointRecord{
		ID:               "restore-point-1",
		TargetAppVersion: "1.2.49",
	}); err != nil {
		t.Fatalf("SaveLaunchState: %v", err)
	}

	app := NewApp()
	app.restorePoints = manager
	app.pendingLaunchStateClear = true
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/settings/updates/health/bootstrap", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if app.pendingLaunchStateClear {
		t.Fatal("expected launch-state clear flag to be reset")
	}
	state, err := manager.LoadLaunchState()
	if err != nil {
		t.Fatalf("LoadLaunchState: %v", err)
	}
	if state != nil {
		t.Fatalf("expected launch state to be cleared, got %#v", state)
	}
}

func TestHandleUpdateBootstrapHealthNoPendingStateNoop(t *testing.T) {
	app := NewApp()
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/settings/updates/health/bootstrap", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestAppServeHTTPClearsPendingLaunchStateAfterHealthyCalendarResponse(t *testing.T) {
	dataDir := t.TempDir()
	manager := update.NewRestorePointManager(dataDir)
	if err := manager.SaveLaunchState(update.RestorePointRecord{
		ID:               "restore-point-1",
		TargetAppVersion: "1.2.49",
	}); err != nil {
		t.Fatalf("SaveLaunchState: %v", err)
	}

	app := NewApp()
	app.restorePoints = manager
	app.pendingLaunchStateClear = true
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/calendar", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if app.pendingLaunchStateClear {
		t.Fatal("expected launch-state clear flag to be reset")
	}
	state, err := manager.LoadLaunchState()
	if err != nil {
		t.Fatalf("LoadLaunchState: %v", err)
	}
	if state != nil {
		t.Fatalf("expected launch state to be cleared, got %#v", state)
	}
}

func TestAppServeHTTPPreservesPendingLaunchStateWhenCalendarFails(t *testing.T) {
	dataDir := t.TempDir()
	manager := update.NewRestorePointManager(dataDir)
	if err := manager.SaveLaunchState(update.RestorePointRecord{
		ID:               "restore-point-1",
		TargetAppVersion: "1.2.49",
	}); err != nil {
		t.Fatalf("SaveLaunchState: %v", err)
	}

	app := NewApp()
	app.restorePoints = manager
	app.pendingLaunchStateClear = true
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/calendar", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "calendar failed", http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusInternalServerError)
	}
	if !app.pendingLaunchStateClear {
		t.Fatal("expected launch-state clear flag to remain set")
	}
	state, err := manager.LoadLaunchState()
	if err != nil {
		t.Fatalf("LoadLaunchState: %v", err)
	}
	if state == nil {
		t.Fatal("expected launch state to remain present")
	}
}

func TestAppServeHTTPFailsClosedWhenPendingLaunchStateCannotClear(t *testing.T) {
	app := NewApp()
	app.pendingLaunchStateClear = true
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/calendar", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "restore point manager unavailable") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestAppServeHTTPClearsPendingLaunchStateAfterTrustedBrowseResponse(t *testing.T) {
	dataDir := t.TempDir()
	manager := update.NewRestorePointManager(dataDir)
	if err := manager.SaveLaunchState(update.RestorePointRecord{
		ID:               "restore-point-1",
		TargetAppVersion: "1.2.49",
	}); err != nil {
		t.Fatalf("SaveLaunchState: %v", err)
	}

	app := NewApp()
	app.restorePoints = manager
	app.pendingLaunchStateClear = true
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/browse", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("browse"))
	})

	req := httptest.NewRequest(http.MethodGet, "/browse", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if app.pendingLaunchStateClear {
		t.Fatal("expected launch-state clear flag to be reset")
	}
}

func TestAppServeHTTPDoesNotClearPendingLaunchStateForAssetResponse(t *testing.T) {
	dataDir := t.TempDir()
	manager := update.NewRestorePointManager(dataDir)
	if err := manager.SaveLaunchState(update.RestorePointRecord{
		ID:               "restore-point-1",
		TargetAppVersion: "1.2.49",
	}); err != nil {
		t.Fatalf("SaveLaunchState: %v", err)
	}

	app := NewApp()
	app.restorePoints = manager
	app.pendingLaunchStateClear = true
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("asset"))
	})

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !app.pendingLaunchStateClear {
		t.Fatal("expected launch-state clear flag to remain set")
	}
}

func TestHandleInitialSetupConfiguresIdentityAndPrefix(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(url.Values{
		"first_name":  {"Samuel"},
		"middle_name": {"Thomas"},
		"last_name":   {"Carter"},
		"birth_year":  {"1838"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/calendar" {
		t.Fatalf("redirect location = %q", location)
	}
	if app.setupRequired {
		t.Fatal("expected setup requirement to be cleared")
	}

	identity, err := database.UserIdentity()
	if err != nil {
		t.Fatalf("UserIdentity: %v", err)
	}
	if identity.NodePrefix != "STC38" {
		t.Fatalf("node prefix = %q", identity.NodePrefix)
	}

	created, err := app.soldiers.Create(models.Soldier{FirstName: "Demo", LastName: "Soldier"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(created.DisplayID, "STC38-") {
		t.Fatalf("display ID = %q", created.DisplayID)
	}
}

func TestSoldierListStartsBlankUntilBrowseOrSearch(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{FirstName: "Nathan", LastName: "Forrest"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/soldiers", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Browse Alphabetically") {
		t.Fatalf("expected browse button, got %q", body)
	}
	if strings.Contains(body, created.DisplayID) {
		t.Fatalf("expected initial results to stay blank, got %q", body)
	}
}

func TestBrowseModeShowsAlphabeticalResults(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{FirstName: "Nathan", LastName: "Forrest"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/soldiers/search?browse=1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, created.DisplayID) {
		t.Fatalf("expected browse mode to show results, got %q", body)
	}
	if !strings.Contains(body, "Browse: alphabetical list") {
		t.Fatalf("expected browse summary, got %q", body)
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
	gotNames := map[string]bool{}
	for _, image := range soldier.Images {
		gotNames[image.FileName] = true
	}
	if !gotNames["CSA-TEST-img-001.jpg"] && !gotNames["CSA-TEST-img-001.png"] {
		t.Fatalf("unexpected stored image names: %#v", soldier.Images)
	}
	if !gotNames["CSA-TEST-img-002.jpg"] && !gotNames["CSA-TEST-img-002.png"] {
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
	if soldier.Images[0].Caption != "" {
		t.Fatalf("stored image caption = %q, want empty", soldier.Images[0].Caption)
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
		if image.Caption != "" {
			t.Fatalf("image caption = %q, want empty", image.Caption)
		}
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
	configureTestIdentity(t, app)
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

func TestFlagReviewStatusAddsRecordToQueue(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{DisplayID: "CSA-REVIEW", FirstName: "Queue", LastName: "Target"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	form := url.Values{
		"review_reason": {"Manual note for follow-up research"},
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/soldiers/%d/review/flag", created.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got != fmt.Sprintf("/soldiers/%d", created.ID) {
		t.Fatalf("redirect=%q", got)
	}

	reloaded, err := app.soldiers.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !reloaded.NeedsReview {
		t.Fatalf("expected record to be flagged for review")
	}
	if reloaded.ReviewReason != "Manual note for follow-up research" {
		t.Fatalf("review reason=%q", reloaded.ReviewReason)
	}
}

func TestHandleCompareUsesRecordBackLinkWhenSourceRecordProvided(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	left, err := app.soldiers.Create(models.Soldier{DisplayID: "CMP-LEFT", FirstName: "Ada", LastName: "Cole"})
	if err != nil {
		t.Fatalf("Create left: %v", err)
	}
	right, err := app.soldiers.Create(models.Soldier{DisplayID: "CMP-RIGHT", FirstName: "Thomas", LastName: "Cole"})
	if err != nil {
		t.Fatalf("Create right: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/compare?id1=%d&id2=%d&from=%d", left.ID, right.ID, left.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`data-history-back`,
		fmt.Sprintf(`data-fallback-href="/soldiers/%d"`, left.ID),
		"Back to Person Record",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("comparison view missing %s in %q", needle, body)
		}
	}
}

func TestHandleInsightsDrilldownShowsFilteredRecords(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	if _, err := app.soldiers.Create(models.Soldier{DisplayID: "INS-0001", FirstName: "Andrew", LastName: "Cole", BuriedIn: "Oak Hill Cemetery"}); err != nil {
		t.Fatalf("Create matching: %v", err)
	}
	if _, err := app.soldiers.Create(models.Soldier{DisplayID: "INS-0002", FirstName: "Thomas", LastName: "Reed", BuriedIn: "Hollywood Cemetery"}); err != nil {
		t.Fatalf("Create non-matching: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/insights/drilldown?scope=buried_in&value="+url.QueryEscape("Oak Hill Cemetery"), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Burial Drilldown") || !strings.Contains(body, "INS-0001") || strings.Contains(body, "INS-0002") {
		t.Fatalf("unexpected drilldown response: %q", body)
	}
}

func TestHandleRecentSearchShowsRequestedRecords(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	first, err := app.soldiers.Create(models.Soldier{DisplayID: "REC-0001", FirstName: "Andrew", LastName: "Cole"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := app.soldiers.Create(models.Soldier{DisplayID: "REC-0002", FirstName: "Thomas", LastName: "Reed"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/soldiers/search/recent?ids=%d,%d", second.ID, first.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Recently Accessed") || !strings.Contains(body, "REC-0002") || !strings.Contains(body, "REC-0001") {
		t.Fatalf("unexpected recent search response: %q", body)
	}
	if strings.Index(body, "REC-0002") > strings.Index(body, "REC-0001") {
		t.Fatalf("recent results should preserve requested order: %q", body)
	}
}

func TestHandleUnitCamaraderieShowsLinkedPeers(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	central, err := app.soldiers.Create(models.Soldier{
		DisplayID: "CAM-1001",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "Co. A, 1st Texas Infantry",
	})
	if err != nil {
		t.Fatalf("Create central: %v", err)
	}
	if _, err := app.soldiers.Create(models.Soldier{
		DisplayID: "CAM-1002",
		FirstName: "Thomas",
		LastName:  "Reed",
		Unit:      "Co. B, 1st Texas Infantry",
	}); err != nil {
		t.Fatalf("Create peer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/soldiers/%d/camaraderie", central.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, needle := range []string{"Unit Camaraderie Graph", "CAM-1001", "CAM-1002", "Compare Person Records"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("camaraderie response missing %s: %q", needle, body)
		}
	}
}

func TestHandleServiceTimelineShowsChronology(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{
		DisplayID: "TLM-1001",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "1st Texas Infantry",
		BirthDate: "05/12/1838",
		DeathDate: "11/03/1904",
		Records: []models.Record{
			{RecordType: "Muster Roll", AppID: "APP-1", Details: "Enlisted on 03/11/1862 at Austin."},
			{RecordType: "Pension", AppID: "APP-2", Details: "Filed in 1901 after moving back to Texas."},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/soldiers/%d/timeline", created.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, needle := range []string{"Auto-Built Service Timeline", "TLM-1001", "Muster Roll", "Pension"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("timeline response missing %s: %q", needle, body)
		}
	}
}

func TestHandleResearchLogShowsTasks(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{
		DisplayID: "RLG-1001",
		FirstName: "Andrew",
		LastName:  "Cole",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := app.soldiers.AddResearchTask(created.ID, "Locate pension file", "Check state archive holdings.", "pension"); err != nil {
		t.Fatalf("AddResearchTask: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/soldiers/%d/research-log", created.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, needle := range []string{"Research Log &amp; Missing Evidence", "Locate pension file", "Missing-Evidence Suggestions"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("research log response missing %s: %q", needle, body)
		}
	}
}

func TestHandleConflictLedgerShowsEntries(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{
		DisplayID: "LED-1001",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "1st Texas Infantry",
		PensionID: "P-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	localJSONBytes, err := json.Marshal(map[string]any{"soldier": models.Soldier{
		DisplayID: "LED-1001",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "1st Texas Infantry",
		PensionID: "P-1",
	}})
	if err != nil {
		t.Fatalf("Marshal local: %v", err)
	}
	sourceJSONBytes, err := json.Marshal(map[string]any{"soldier": models.Soldier{
		DisplayID: "SRC-1001",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "2nd Texas Infantry",
		PensionID: "P-9",
	}})
	if err != nil {
		t.Fatalf("Marshal source: %v", err)
	}

	if _, err := database.Conn().Exec(`INSERT INTO merge_review_sessions (id, archive_path, source_root, status, updated_at) VALUES ('session-app-ledger', 'ledger.ddshare', 'C:\\source', 'open', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := database.Conn().Exec(`
		INSERT INTO merge_review_conflicts (session_id, conflict_type, reason, soldier_sync_id, local_soldier_id, local_display_id, source_display_id, local_data, source_data, resolution, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, "session-app-ledger", "soldier-update", "Shared archive changed unit and pension ID.", created.SyncID, created.ID, created.DisplayID, "SRC-1001", string(localJSONBytes), string(sourceJSONBytes), "keep-local"); err != nil {
		t.Fatalf("insert conflict: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/soldiers/%d/conflict-ledger", created.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, needle := range []string{"Merge Review Ledger", "SRC-1001", "pension ID"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("conflict ledger response missing %s: %q", needle, body)
		}
	}
}

func TestHandleResearchPackShowsRelatedRecords(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	central, err := app.soldiers.Create(models.Soldier{
		DisplayID:    "PACK-1001",
		FirstName:    "Andrew",
		LastName:     "Cole",
		PensionState: "Texas",
		BirthInfo:    "Born 1838 in Orange County, Texas.",
	})
	if err != nil {
		t.Fatalf("Create central: %v", err)
	}
	if _, err := app.soldiers.Create(models.Soldier{
		DisplayID:    "PACK-1002",
		FirstName:    "Thomas",
		LastName:     "Reed",
		PensionState: "Texas",
		Unit:         "1st Texas Infantry",
	}); err != nil {
		t.Fatalf("Create match: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/soldiers/%d/research-pack/state", central.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, needle := range []string{"State Research Pack", "Texas", "PACK-1002"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("research pack response missing %s: %q", needle, body)
		}
	}
}

func TestHandleResearchCollectionsShowsHub(t *testing.T) {
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
	configureTestIdentity(t, app)
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{
		DisplayID: "COL-1001",
		FirstName: "Andrew",
		LastName:  "Cole",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := app.soldiers.CreateResearchCollection("Orange County Cluster", "County-focused follow-up list."); err != nil {
		t.Fatalf("CreateResearchCollection: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/research-collections?from=%d", created.ID), nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, needle := range []string{"Named Research Collections", "Orange County Cluster", "Add Current Person Record"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("research collections hub missing %s: %q", needle, body)
		}
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

func configureTestIdentity(t *testing.T, app *App) {
	t.Helper()
	if _, err := app.database.ConfigureUserIdentity("Test", "Harness", "User", 1900); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices after identity: %v", err)
	}
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
