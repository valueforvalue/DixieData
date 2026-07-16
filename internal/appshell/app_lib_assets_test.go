// app_lib_assets_test.go -- issue #609 regression net.
//
// Verifies the appshell serves /lib/debounce.js +
// /lib/clipboard.js from the embedded frontend asset
// filesystem (or ./frontend on disk if the embed was
// bypassed). Before this slice, both URLs returned 404
// from dixiedata-web -- even though the Wails binary
// served them correctly via //go:embed. The audit-harness
// therefore couldn't drive any feature that depended on
// window.__dixieDebounce or window.__dixieCopyText,
// including the article preview modal (#607) and the
// browse filter debounce (issue #573).
package appshell

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

// TestServeHTTP_LibAssets_ProjectRoot verifies the
// /lib/* routes serve the correct file from
// ./frontend on disk (the headless fallback path
// dixiedata-web + the test harness use, since they
// bypass WithFrontendAssets). Pins the bytes' shape
// (must contain the function name so a copy/paste
// mismatch catches the symlink) + the Content-Type
// (must be text/javascript so the browser runs the
// script as a module).
func TestServeHTTP_LibAssets_ProjectRoot(t *testing.T) {
	app := NewApp()
	req := httptest.NewRequest(http.MethodGet, "/lib/debounce.js", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Fatalf("Content-Type=%q, want text/javascript", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "function debounce") && !strings.Contains(body, "debounce(") {
		t.Fatalf("body did not contain debounce content (issue #609 — /lib/debounce.js serving wrong bytes?)")
	}
}

// TestServeHTTP_LibAssets_Clipboard is the sibling
// assertion for clipboard.js. Per the same contract.
// Grounded so a future refactor that drops the lib
// (or renames it) trips the test before a silent
// regression lands.
func TestServeHTTP_LibAssets_Clipboard(t *testing.T) {
	app := NewApp()
	req := httptest.NewRequest(http.MethodGet, "/lib/clipboard.js", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Fatalf("Content-Type=%q, want text/javascript", got)
	}
}

// TestServeHTTP_LibAssets_RouteSize pins the
// "file is being read" verification end to end:
// the bytes the response sends must match the
// bytes the file on disk has. Catches a future
// refactor that mounts the wrong path or reads
// a stub. Also catches filesystem-level drift
// (a user accidentally trims frontend/lib/).
func TestServeHTTP_LibAssets_RouteSize(t *testing.T) {
	app := NewApp()
	for _, name := range []string{"debounce.js", "clipboard.js"} {
		url := "/lib/" + name
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%q (issue #609)", name, rec.Code, rec.Body.String())
		}
		// Read the on-disk file (the headless fallback
		// path; the Wails binary reads from the embed
		// and never goes near the disk). Both should
		// match in length + content. Use appdata.ProjectRoot
		// so the test resolves the repo root regardless
		// of the working directory go test was invoked from.
		root, err := appdata.ProjectRoot()
		if err != nil {
			t.Fatalf("locate repo root: %v", err)
		}
		path := filepath.Join(root, "frontend", "lib", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read disk fallback %s: %v", path, err)
		}
		if got := rec.Body.Len(); got != len(data) {
			t.Errorf("%s length mismatch: response=%d, disk=%d (issue #609 — handler reading wrong source?)", name, got, len(data))
		}
	}
}

// TestServeHTTP_LibAssets_RejectsNonGet pins the
// HTTP-method contract: the route is GET/HEAD-only;
// POST/PUT/PATCH/DELETE return 405. Mirrors every
// other frontend-asset route's gating.
func TestServeHTTP_LibAssets_RejectsNonGet(t *testing.T) {
	app := NewApp()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/lib/debounce.js", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /lib/debounce.js status=%d, want %d", method, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}
