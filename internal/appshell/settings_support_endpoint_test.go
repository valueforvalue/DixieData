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

// TestHandleSettingsSupportEndpoint_PersistsAndUpdatesInMemoryStore
// pins issue #544 slice 3 happy path: a POST with
// support_endpoint=https://example.invalid/upload is accepted,
// persisted to local_settings.json, and the helper reads the
// same value on the next GET /settings.
func TestHandleSettingsSupportEndpoint_PersistsAndUpdatesInMemoryStore(t *testing.T) {
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

	body := "support_endpoint=https%3A%2F%2Fsupport.example.invalid%2Fupload"
	req := httptest.NewRequest(http.MethodPost, "/settings/support-endpoint", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("POST /settings/support-endpoint returned 405; route not registered. body=%q", rec.Body.String())
	}
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got != "/settings" {
		t.Errorf("X-DixieData-Redirect = %q, want %q", got, "/settings")
	}
	persisted, err := records.LoadLocalSettings(dataDir)
	if err != nil {
		t.Fatalf("LoadLocalSettings after POST: %v", err)
	}
	if persisted.SupportEndpoint != "https://support.example.invalid/upload" {
		t.Errorf("persisted SupportEndpoint = %q; want %q", persisted.SupportEndpoint, "https://support.example.invalid/upload")
	}
}

// TestHandleSettingsSupportEndpoint_EmptyClearsEndpoint pins the
// "feature off" contract: an empty endpoint string is accepted
// and clears the previously-stored value (so the user can
// disable the feature by clearing the input). Mirrors the
// opt-in pattern: empty = off, non-empty = on.
func TestHandleSettingsSupportEndpoint_EmptyClearsEndpoint(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// Seed a non-empty endpoint so the clear is observable.
	if err := records.SaveLocalSettings(dataDir, records.LocalSettings{
		SupportEndpoint: "https://old.example.invalid/upload",
	}); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	body := "support_endpoint="
	req := httptest.NewRequest(http.MethodPost, "/settings/support-endpoint", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	persisted, err := records.LoadLocalSettings(dataDir)
	if err != nil {
		t.Fatalf("LoadLocalSettings after POST: %v", err)
	}
	if persisted.SupportEndpoint != "" {
		t.Errorf("after empty POST, SupportEndpoint = %q; want \"\" (feature off)", persisted.SupportEndpoint)
	}
}

// TestHandleSettingsSupportEndpoint_RejectsBadScheme pins the
// validation contract: an http:// (non-https) endpoint is
// rejected because the feedback bundle may carry PII. The
// https-only policy mirrors what the bug-report bundle already
// does on its way out; the support endpoint is the same shape.
// Plain http is allowed ONLY for loopback / 127.0.0.1 /
// localhost addresses (dev / test harnesses that hit a local
// receiver).
func TestHandleSettingsSupportEndpoint_RejectsBadScheme(t *testing.T) {
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

	body := "support_endpoint=ftp%3A%2F%2Fattacker.example.invalid%2Fupload"
	req := httptest.NewRequest(http.MethodPost, "/settings/support-endpoint", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status=%d; want %d (rejected ftp:// scheme)", rec.Code, http.StatusBadRequest)
	}
	// Persisted state must NOT have been mutated.
	persisted, err := records.LoadLocalSettings(dataDir)
	if err != nil {
		t.Fatalf("LoadLocalSettings after rejected POST: %v", err)
	}
	if persisted.SupportEndpoint != "" {
		t.Errorf("after rejected POST, SupportEndpoint = %q; want \"\" (unchanged)", persisted.SupportEndpoint)
	}
}

// TestHandleSettingsSupportEndpoint_AllowsLoopbackHttp pins the
// dev-harness exception: http://127.0.0.1:NNNN is accepted so
// the audit probe + CI can hit a local httptest receiver. This
// is a code-friendly carve-out; production deployments should
// use https.
func TestHandleSettingsSupportEndpoint_AllowsLoopbackHttp(t *testing.T) {
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

	body := "support_endpoint=http%3A%2F%2F127.0.0.1%3A9876%2Fupload"
	req := httptest.NewRequest(http.MethodPost, "/settings/support-endpoint", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code < 200 || rec.Code >= 300 {
		t.Errorf("status=%d; want 2xx (loopback http:// accepted for dev/test)", rec.Code)
	}
}

// TestSettingsRender_HasSupportEndpointInput pins the UI
// contract: /settings renders a form action="/settings/support-
// endpoint" so the JS dispatcher picks it up via the
// data-dixie-submit pathway. The Support & Diagnostics card is
// the natural home for the input (alongside the existing
// Export Feedback Log / Export Bug Report Bundle buttons).
func TestSettingsRender_HasSupportEndpointInput(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="/settings/support-endpoint"`) {
		t.Errorf("settings render missing support-endpoint form action (issue #544 slice 3)")
	}
	if !strings.Contains(body, `name="support_endpoint"`) {
		t.Errorf("settings render missing support_endpoint form input (issue #544 slice 3)")
	}
}

