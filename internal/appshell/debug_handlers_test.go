package appshell

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// newDebugConsoleApp builds an App with debug mode on and an isolated
// ring buffer pre-loaded with the supplied entries. The App's routes
// are registered so handleDebugConsole / handleClientLogs can be
// invoked through app.ServeHTTP.
func newDebugConsoleApp(t *testing.T, seed []debug.Entry) *App {
	t.Helper()
	dataDir := filepath.Join(testtemp.New(t).Path(), ".dixiedata")
	app := newStressApp(t)
	app.dataDir = dataDir
	app.debugMode.Store(true)
	// Configure a fresh ring buffer; newStressApp didn't call
	// debug.Configure, so we own the global ringBuf for this test.
	if err := debug.Configure(debug.Config{
		LogPath:  filepath.Join(testtemp.New(t).Path(), "app.log.jsonl"),
		RingSize: 100,
		Debug:    true,
	}); err != nil {
		t.Fatalf("debug.Configure: %v", err)
	}
	t.Cleanup(func() { _ = debug.Close() })
	rb := debug.GetRingBuffer()
	if rb == nil {
		t.Fatal("ring buffer not initialised after Configure")
	}
	for _, e := range seed {
		rb.Push(e)
	}
	return app
}

// TestHandleDebugConsoleComponentFilter pins the JS Console filter
// added for issue #557. Seeds three entries (one Go-side component,
// two JS-side components at different levels), GETs the panel with
// ?component=frontend, and asserts the response renders ONLY the two
// JS-side entries. Then verifies the filter composes with ?level=:
// ?level=ERROR&component=frontend must surface exactly one entry.
func TestHandleDebugConsoleComponentFilter(t *testing.T) {
	app := newDebugConsoleApp(t, []debug.Entry{
		{Time: time.Now(), Level: "INFO", Message: "go-side startup", Component: "appshell"},
		{Time: time.Now(), Level: "INFO", Message: "js info msg", Component: "frontend"},
		{Time: time.Now(), Level: "ERROR", Message: "js error msg", Component: "frontend"},
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/console?component=frontend", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "go-side startup") {
		t.Fatalf("component=frontend filter leaked Go-side entry: %s", body)
	}
	if !strings.Contains(body, "js info msg") || !strings.Contains(body, "js error msg") {
		t.Fatalf("component=frontend filter dropped JS entries: %s", body)
	}

	// Composable with ?level=ERROR.
	req2 := httptest.NewRequest(http.MethodGet, "/debug/console?level=ERROR&component=frontend", nil)
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("composable status=%d", rec2.Code)
	}
	body2 := rec2.Body.String()
	if !strings.Contains(body2, "js error msg") {
		t.Fatalf("expected js error msg in ERROR+frontend filter: %s", body2)
	}
	if strings.Contains(body2, "js info msg") {
		t.Fatalf("level=ERROR should have dropped js info msg: %s", body2)
	}
	if strings.Contains(body2, "go-side startup") {
		t.Fatalf("component=frontend should have dropped go-side entry: %s", body2)
	}
}

// TestHandleDebugConsoleComponentFilter_EmptyMeansAll pins the
// fallback: an empty ?component= query param behaves the same as
// omitting it (all components visible). Prevents future regressions
// where a typo in the empty-string compare would silently hide the
// whole panel.
func TestHandleDebugConsoleComponentFilter_EmptyMeansAll(t *testing.T) {
	app := newDebugConsoleApp(t, []debug.Entry{
		{Time: time.Now(), Level: "INFO", Message: "go-side msg", Component: "appshell"},
		{Time: time.Now(), Level: "INFO", Message: "js msg", Component: "frontend"},
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/console?component=", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "go-side msg") || !strings.Contains(body, "js msg") {
		t.Fatalf("empty component filter should show all entries; got %s", body)
	}
}

// TestHandleClientLogsRejectsOversizedBody pins the 256 KB MaxBytesReader
// guard on the /debug/client-logs endpoint (issue #557 defense in
// depth: the frontend self-throttles to 32 KB per batch, but a
// misbehaving or hostile client must not OOM the server).
func TestHandleClientLogsRejectsOversizedBody(t *testing.T) {
	app := newDebugConsoleApp(t, nil)

	// Build a JSON payload larger than 256 KB. A single-entry
	// payload with a giant msg field is the simplest way to trip
	// the cap without exercising the JSON decoder's own size
	// guard.
	huge := strings.Repeat("x", 300*1024)
	payload, err := json.Marshal(map[string]any{
		"entries": []map[string]any{
			{"ts": "2026-01-01T00:00:00Z", "level": "info", "msg": huge, "url": "/"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	if len(payload) <= 256*1024 {
		t.Fatalf("test setup: expected payload > 256KB, got %d", len(payload))
	}

	req := httptest.NewRequest(http.MethodPost, "/debug/client-logs", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize body status=%d (want 413); body=%q", rec.Code, rec.Body.String())
	}
}

// TestHandleClientLogsAcceptsSmallBody pins the happy path: a
// well-formed, sub-cap payload still returns 204 (the existing
// silent-drop shape) so the MaxBytesReader fix does not regress
// the normal flow.
func TestHandleClientLogsAcceptsSmallBody(t *testing.T) {
	app := newDebugConsoleApp(t, nil)

	payload, err := json.Marshal(map[string]any{
		"entries": []map[string]any{
			{"ts": "2026-01-01T00:00:00Z", "level": "info", "msg": "hello", "url": "/"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/debug/client-logs", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("small body status=%d (want 204); body=%q", rec.Code, rec.Body.String())
	}
}