package appshell

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/records"
)

// TestStartupPlaceholderReadsPersistedThemeFromDiskOnColdStart
// reproduces the WebView2 first-paint race reported in the theme
// bug: Wails constructs *App via NewApp() and the WebView2 fires
// its first HTTP request BEFORE App.Startup() runs. At that
// moment a.theme is a zero-value atomic.Value (Load() returns nil)
// and a.dataDir is empty, so renderStartupPlaceholder's nil-safe
// fallback hardcodes data-theme="default" even when the user
// persisted theme="high-contrast" on disk. The user sees a flash
// of the Default palette until the 700ms JS redirect hits the
// warmed mux and lifecycle.go ServeHTTP reads the now-stored
// atomic.
//
// Red-capable loop for /diagnose: drives the actual bug code path
// (NewApp() -> ServeHTTP with mux==nil -> renderStartupPlaceholder)
// and asserts the user's exact symptom (Default theme rendered
// despite persisted High Contrast). Goes red on this bug and will
// go green once the placeholder backfills from disk on nil atomic.
func TestStartupPlaceholderReadsPersistedThemeFromDiskOnColdStart(t *testing.T) {
	// Build a data dir + sibling state root holding a persisted
	// High Contrast settings file, exactly as SaveLocalSettings
	// would leave it after the user picks the theme.
	parent := t.TempDir()
	dataDir := filepath.Join(parent, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	stateRoot := appdata.StateRoot(dataDir)
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir stateRoot: %v", err)
	}
	if err := records.SaveLocalSettings(dataDir, records.LocalSettings{
		Theme: records.ThemeHighContrast,
	}); err != nil {
		t.Fatalf("save persisted settings: %v", err)
	}

	// Simulate the race: NewApp() (bare struct, no Startup) but
	// with dataDir set as Startup would set it. a.theme stays
	// zero-value, exactly as it is between NewApp() and the
	// first Store inside Startup().
	app := NewApp()
	app.dataDir = dataDir

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d want %d (placeholder should serve during pre-mux window)", rec.Code, http.StatusAccepted)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-theme="high-contrast"`) {
		t.Fatalf("placeholder must read persisted high-contrast theme from disk when a.theme is still nil (cold-start race); got body:\n%s", body)
	}

	// Add a Soft-theme case so the contract isn't accidentally
	// narrowed to High Contrast only.
	t.Run("Soft theme", func(t *testing.T) {
		parent := t.TempDir()
		dataDir := filepath.Join(parent, "data")
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			t.Fatalf("mkdir dataDir: %v", err)
		}
		if err := os.MkdirAll(appdata.StateRoot(dataDir), 0o755); err != nil {
			t.Fatalf("mkdir stateRoot: %v", err)
		}
		if err := records.SaveLocalSettings(dataDir, records.LocalSettings{
			Theme: records.ThemeSoft,
		}); err != nil {
			t.Fatalf("save persisted settings: %v", err)
		}

		app := NewApp()
		app.dataDir = dataDir

		req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if !strings.Contains(rec.Body.String(), `data-theme="soft"`) {
			t.Fatalf("placeholder must read persisted soft theme from disk; got body:\n%s", rec.Body.String())
		}
	})

	// Corrupt or missing settings file must fall back to default,
	// never panic or render an empty data-theme attribute.
	t.Run("missing settings file falls back to soft", func(t *testing.T) {
		parent := t.TempDir()
		dataDir := filepath.Join(parent, "data")
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			t.Fatalf("mkdir dataDir: %v", err)
		}

		app := NewApp()
		app.dataDir = dataDir

		req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		// Issue #494: Soft is the new default for fresh installs.
		// The pre-mux placeholder fallback resolves to ThemeSoft
		// when no settings file exists.
		if !strings.Contains(rec.Body.String(), `data-theme="soft"`) {
			t.Fatalf("placeholder must fall back to soft when no settings file exists; got body:\n%s", rec.Body.String())
		}
	})
}