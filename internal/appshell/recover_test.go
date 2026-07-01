package appshell

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestServeHTTPRecoversPanic(t *testing.T) {
	app := NewApp()
	app.muxRaw = http.NewServeMux()
	app.mux = recoverMiddleware(app.muxRaw)
	// The panic value carries the test id so that grepping a stack
	// dump across the appshell test suite for `synthetic calendar
	// PDF crash` cannot mistakenly attribute this test's panic to
	// any other test that happens to log a calendar PDF message
	// (the string was previously shared with a calendar-export
	// error log path, which made cross-test grep ambiguous).
	app.muxRaw.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("synthetic calendar PDF crash [recover_test]")
	})

	req := httptest.NewRequest(http.MethodPost, "/boom", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Internal server error") {
		t.Fatalf("body should mention internal server error, got %q", rec.Body.String())
	}

	// Verify the crash log received an entry.
	path := LogCrash(req, "synthetic calendar PDF crash [recover_test]")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("crash log not written: %v (path=%s)", err, path)
	}
	if !strings.Contains(string(data), "synthetic calendar PDF crash [recover_test]") {
		t.Fatalf("crash log missing panic value: %s", data)
	}
	if !strings.Contains(string(data), "/boom") {
		t.Fatalf("crash log missing URL: %s", data)
	}
	if !strings.Contains(string(data), "goroutine") {
		t.Fatalf("crash log missing stack trace: %s", data)
	}
	t.Logf("crash log: %s (%d bytes)", path, len(data))

	// Clean up the synthetic crash log so we don't pollute the user's
	// real data dir during tests.
	_ = os.Remove(path)
}
