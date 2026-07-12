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

// TestBootThemeScript covers the issue #483 follow-up fix: the
// /boot-theme.js endpoint that the static frontend/index.html shell
// fetches via a blocking <script> in <head> so <html data-theme> is
// set before the first paint (the Wails asset server serves
// index.html as a static asset for "/", bypassing the Go Layout that
// would otherwise set data-theme server-side).
//
// Four sub-cases:
//   - warm mux + persisted High Contrast: route returns JS setting
//     data-theme="high-contrast".
//   - pre-mux cold-start race (NewApp, no Startup, settings on disk):
//     the ServeHTTP pre-mux switch backfills from disk so the shell
//     gets the right theme before Startup stores the atomic.
//   - truly fresh app (no settings file): falls back to "default".
//   - non-GET rejected with 405.
func TestBootThemeScript(t *testing.T) {
	t.Run("warm mux echoes persisted High Contrast theme", func(t *testing.T) {
		app := NewApp()
		app.theme.Store(records.ThemeHighContrast)
		app.setupRoutes()

		req := httptest.NewRequest(http.MethodGet, "/boot-theme.js", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
			t.Fatalf("Content-Type=%q want text/javascript", ct)
		}
		body := rec.Body.String()
		want := `document.documentElement.setAttribute('data-theme',"high-contrast");`
		if !strings.Contains(body, want) {
			t.Fatalf("body must set data-theme to high-contrast; got:\n%s", body)
		}
	})

	t.Run("pre-mux cold start backfills High Contrast from disk", func(t *testing.T) {
		// Simulate the Wails WebView2 first-paint race: the shell's
		// blocking <script src="/boot-theme.js"> fires before
		// App.Startup() runs, so a.theme is nil but local_settings.json
		// on disk carries theme="high-contrast". The pre-mux switch
		// in ServeHTTP must serve the disk-backfilled JS.
		parent := t.TempDir()
		dataDir := filepath.Join(parent, "data")
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			t.Fatalf("mkdir dataDir: %v", err)
		}
		if err := os.MkdirAll(appdata.StateRoot(dataDir), 0o755); err != nil {
			t.Fatalf("mkdir stateRoot: %v", err)
		}
		if err := records.SaveLocalSettings(dataDir, records.LocalSettings{
			Theme: records.ThemeHighContrast,
		}); err != nil {
			t.Fatalf("save settings: %v", err)
		}

		app := NewApp()
		app.dataDir = dataDir
		// a.mux stays nil (Startup not called) -- the pre-mux path.

		req := httptest.NewRequest(http.MethodGet, "/boot-theme.js", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d want %d (pre-mux switch must serve boot-theme.js)", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"high-contrast"`) {
			t.Fatalf("pre-mux boot-theme.js must backfill high-contrast from disk; got:\n%s", body)
		}
	})

	t.Run("fresh app with no settings file falls back to default", func(t *testing.T) {
		parent := t.TempDir()
		dataDir := filepath.Join(parent, "data")
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			t.Fatalf("mkdir dataDir: %v", err)
		}

		app := NewApp()
		app.dataDir = dataDir
		// a.mux nil, no settings file on disk.

		req := httptest.NewRequest(http.MethodGet, "/boot-theme.js", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"default"`) {
			t.Fatalf("boot-theme.js must fall back to default when no settings file; got:\n%s", body)
		}
	})

	t.Run("rejects non-GET", func(t *testing.T) {
		app := NewApp()
		app.theme.Store(records.ThemeDefault)
		app.setupRoutes()

		req := httptest.NewRequest(http.MethodPost, "/boot-theme.js", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})
}

// TestIndexHTMLReferencesBootThemeScript pins that the static
// frontend/index.html shell carries the blocking
// <script src="/boot-theme.js"> in <head> so the persisted theme is
// applied before the first paint. A regression that removes the tag,
// moves it out of <head>, or makes it deferred (which would let the
// body paint before the theme is set) fails this test.
func TestIndexHTMLReferencesBootThemeScript(t *testing.T) {
	data, err := os.ReadFile("../../frontend/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	body := string(data)

	// Must reference /boot-theme.js via a blocking <script>.
	if !strings.Contains(body, `<script src="/boot-theme.js"></script>`) {
		t.Fatalf("index.html must reference /boot-theme.js via a blocking <script> in <head>; got:\n%s", body)
	}
	// Must NOT be deferred: a deferred script runs after the body
	// parses + paints, reintroducing the FOUC.
	if strings.Contains(body, `<script defer src="/boot-theme.js">`) ||
		strings.Contains(body, `<script src="/boot-theme.js" defer>`) {
		t.Fatalf("index.html must NOT defer /boot-theme.js — a deferred script runs after first paint; got:\n%s", body)
	}
	// The boot-theme script tag must live in <head> (before </head>)
	// so it runs before the body parses + paints. Checking against
	// </head> avoids false matches from the explanatory comment
	// block (which mentions both the script and the body tag as
	// text).
	headEnd := strings.Index(body, "</head>")
	if headEnd == -1 {
		t.Fatalf("index.html missing </head>; got:\n%s", body)
	}
	bootTag := `<script src="/boot-theme.js"></script>`
	bootIdx := strings.Index(body, bootTag)
	if bootIdx == -1 {
		t.Fatalf("index.html missing %s; got:\n%s", bootTag, body)
	}
	if bootIdx > headEnd {
		t.Fatalf("boot-theme.js script tag must be in <head> (before </head>); bootIdx=%d headEnd=%d", bootIdx, headEnd)
	}
	// The body's htmx load trigger must live after </head>.
	if !strings.Contains(body[headEnd:], `<body hx-get="/calendar"`) {
		t.Fatalf("index.html missing <body hx-get=\"/calendar\" after </head>; got:\n%s", body)
	}
}