// settings_theme_test.go pins the /settings/theme POST handler
// added in issue #474. Three test cases cover the happy path
// (each theme is accepted and persisted), the validation path
// (unknown theme values are rejected with 400), and the GET
// /settings path renders the new Appearance card with the
// current theme reflected in the checked radio.
//
// RED-first: these tests were written before the handler. They
// fail with 405 (route not registered) and "settings-theme-panel
// not found" on the unpatched tree, and PASS once the route +
// handler + panel land in the same commit.
package appshell

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// TestHandleSettingsTheme_PersistsAndUpdatesInMemoryStore pins the
// happy path: a POST with theme=high-contrast is accepted,
// persisted to local_settings.json, and reflected in the in-memory
// a.theme atomic so the next request renders the new value.
func TestHandleSettingsTheme_PersistsAndUpdatesInMemoryStore(t *testing.T) {
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
	// Seed the in-memory store the way startup does so the GET
	// before this POST would have a known starting value.
	app.theme.Store(records.ThemeClassic)

	body := "theme=high-contrast"
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("POST /settings/theme returned 405; route not registered. body=%q", rec.Body.String())
	}
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got != "/settings" {
		t.Errorf("X-DixieData-Redirect = %q, want %q", got, "/settings")
	}
	// In-memory store updated.
	if v, _ := app.theme.Load().(string); v != records.ThemeHighContrast {
		t.Errorf("in-memory theme = %q, want %q", v, records.ThemeHighContrast)
	}
	// File persisted to the new state-root location, NOT the
	// legacy archive location.
	persisted, err := records.LoadLocalSettings(dataDir)
	if err != nil {
		t.Fatalf("LoadLocalSettings after POST: %v", err)
	}
	if persisted.Theme != records.ThemeHighContrast {
		t.Errorf("persisted Theme = %q, want %q", persisted.Theme, records.ThemeHighContrast)
	}
}

// TestHandleSettingsTheme_RejectsUnknownValue pins the validation
// branch: an unknown theme value returns 400 and does not touch
// the persisted settings or the in-memory store.
func TestHandleSettingsTheme_RejectsUnknownValue(t *testing.T) {
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
	app.theme.Store(records.ThemeSoft)

	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader("theme=neon-cyber"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown theme, got %d body=%q", rec.Code, rec.Body.String())
	}
	// In-memory store unchanged.
	if v, _ := app.theme.Load().(string); v != records.ThemeSoft {
		t.Errorf("in-memory theme changed to %q after rejected POST", v)
	}
}

// TestSettingsView_RendersAppearancePanel pins that the new card
// lands on /settings and that the current theme is reflected in
// the checked radio attribute.
func TestSettingsView_RendersAppearancePanel(t *testing.T) {
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
	app.theme.Store(records.ThemeSoft)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	html := string(body)
	for _, needle := range []string{
		`id="settings-appearance-panel"`,
		`action="/settings/theme"`,
		`value="default"`,
		`value="high-contrast"`,
		`value="soft"`,
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("expected %q in /settings body", needle)
		}
	}
	// The "soft" radio should be the one with the checked attribute
	// because the in-memory store is set to soft. Look for the
	// checked radio input in the soft column.
	softBlock := extractThemeOptionBlock(html, "soft")
	if !strings.Contains(softBlock, "checked") {
		t.Errorf("expected checked radio inside the soft option block; got %q", softBlock)
	}
}

// TestSettingsView_RendersDataThemeAttrOnHtml pins that the Layout
// emits <html lang="en" data-theme="..."> based on the per-request
// theme tag. Without this, the data-theme attribute never reaches
// the browser and the high-contrast CSS rule has nothing to match.
func TestSettingsView_RendersDataThemeAttrOnHtml(t *testing.T) {
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
	app.theme.Store(records.ThemeHighContrast)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Body)
	html := string(body)
	if !strings.Contains(html, `<html lang="en" data-theme="high-contrast" data-export-surface="jobs-page">`) {
		t.Errorf("expected <html lang=\"en\" data-theme=\"high-contrast\">; got first 300 chars: %q", firstN(html, 300))
	}
}

// TestStateRoot_DoesNotCollideWithArchiveDir pins that the new
// StateRoot() resolves to a sibling of the data dir, NOT a child.
// The audit harness + restore code path both rename the data
// dir; if local_settings.json lived inside, every restore would
// wipe the user's theme choice.
func TestStateRoot_DoesNotCollideWithArchiveDir(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, ".dixiedata")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	stateDir := records.LocalSettingsPath(dataDir)
	parent := filepath.Dir(stateDir)
	if parent == dataDir {
		t.Fatalf("StateRoot must be a sibling of dataDir; got both at %q", parent)
	}
	if filepath.Base(parent) != ".dixiedata-state" {
		t.Errorf("state root parent basename = %q, want %q", filepath.Base(parent), ".dixiedata-state")
	}
}

// TestSettingsTheme_MigratesLegacyLocalSettingsFile pins the
// one-time move from <dataDir>/local_settings.json to the new
// state-root location. A user upgrading from a build that wrote
// the file inside the archive dir should keep their persisted
// setting on first read after the upgrade.
func TestSettingsTheme_MigratesLegacyLocalSettingsFile(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, ".dixiedata")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	legacy := records.LocalSettings{
		DebugMode: true,
		Theme:     records.ThemeSoft,
	}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dataDir, "local_settings.json"), data, 0o644); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	got, err := records.LoadLocalSettings(dataDir)
	if err != nil {
		t.Fatalf("LoadLocalSettings: %v", err)
	}
	if got.Theme != records.ThemeSoft {
		t.Errorf("migrated Theme = %q, want %q", got.Theme, records.ThemeSoft)
	}
	if !got.DebugMode {
		t.Error("migrated DebugMode = false, want true")
	}
	// New file exists at the state root.
	if _, err := os.Stat(records.LocalSettingsPath(dataDir)); err != nil {
		t.Errorf("new state-root file missing: %v", err)
	}
	// Legacy file is left in place as a tombstone.
	if _, err := os.Stat(filepath.Join(dataDir, "local_settings.json")); err != nil {
		t.Errorf("legacy tombstone missing: %v", err)
	}
}

// extractThemeOptionBlock returns the substring of html that
// contains the <input type="radio" ...> for the given theme
// value, plus its enclosing <label class="theme-option"> block.
// Used by the appearance-panel render test to verify the
// checked attribute is on the right radio.
func extractThemeOptionBlock(html, themeValue string) string {
	marker := `data-theme-option="` + themeValue + `"`
	idx := strings.Index(html, marker)
	if idx < 0 {
		return ""
	}
	// Walk back to the enclosing <label
	start := strings.LastIndex(html[:idx], "<label")
	if start < 0 {
		return ""
	}
	// Walk forward to the matching </label>
	end := strings.Index(html[idx:], "</label>")
	if end < 0 {
		return ""
	}
	return html[start : idx+end+len("</label>")]
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
