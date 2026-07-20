package appshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// TestHandleSettingsExportSurface_PersistsAndUpdatesInMemoryStore
// pins issue #534 happy path: a POST with export_surface=toast-only
// is accepted, persisted to local_settings.json, and reflected in
// the in-memory a.exportSurface atomic so the next request
// renders <html data-export-surface="toast-only">.
func TestHandleSettingsExportSurface_PersistsAndUpdatesInMemoryStore(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()
	// Seed the in-memory store the way startup does.
	app.exportSurface.Store("jobs-page")

	body := "export_surface=toast-only"
	req := httptest.NewRequest(http.MethodPost, "/settings/export-surface", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("POST /settings/export-surface returned 405; route not registered. body=%q", rec.Body.String())
	}
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got != "/settings/appearance" {
		t.Errorf("X-DixieData-Redirect = %q, want %q", got, "/settings/appearance")
	}
	if v, _ := app.exportSurface.Load().(string); v != "toast-only" {
		t.Errorf("in-memory exportSurface = %q, want %q", v, "toast-only")
	}
	persisted, err := records.LoadLocalSettings(dataDir)
	if err != nil {
		t.Fatalf("LoadLocalSettings after POST: %v", err)
	}
	if persisted.ExportSurface != "toast-only" {
		t.Errorf("persisted ExportSurface = %q, want %q", persisted.ExportSurface, "toast-only")
	}
}

// TestHandleSettingsExportSurface_RejectsUnknownValue pins the
// validation branch: an unknown export_surface value returns 400
// and does not touch the persisted settings or the in-memory
// store (per issue #553's "never silently downgrade" policy).
func TestHandleSettingsExportSurface_RejectsUnknownValue(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()
	app.exportSurface.Store("jobs-page")

	req := httptest.NewRequest(http.MethodPost, "/settings/export-surface", strings.NewReader("export_surface=neon-cyber"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status=%d; want %d", rec.Code, http.StatusBadRequest)
	}
	if v, _ := app.exportSurface.Load().(string); v != "jobs-page" {
		t.Errorf("in-memory exportSurface = %q after rejected POST; want unchanged %q", v, "jobs-page")
	}
}

// TestHandleSettingsExportSurface_DefaultJobsPage pins the
// dispatcher's zero-value contract: when the user hasn't picked
// a preference, every request renders
// <html data-export-surface="jobs-page"> so the dispatcher
// preserves today's navigation behavior.
func TestHandleSettingsExportSurface_DefaultJobsPage(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()
	// No app.exportSurface.Store call: zero value, dispatcher
	// must fall back to "jobs-page".

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `data-export-surface="jobs-page"`) {
		t.Errorf("default /settings render missing data-export-surface=\"jobs-page\"; got body[0..300]: %s", rec.Body.String()[:min(300, len(rec.Body.String()))])
	}
}

// TestHandleSettingsExportSurface_ToastOnlyInHtml pins the second
// half of the contract: when the user picks "toast-only", the
// next request renders <html data-export-surface="toast-only">
// so the dispatcher suppresses the post-export navigation.
func TestHandleSettingsExportSurface_ToastOnlyInHtml(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()
	app.exportSurface.Store("toast-only")

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `data-export-surface="toast-only"`) {
		t.Errorf("/settings render missing data-export-surface=\"toast-only\"; got body[0..300]: %s", rec.Body.String()[:min(300, len(rec.Body.String()))])
	}
}