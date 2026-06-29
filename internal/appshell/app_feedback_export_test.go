package appshell

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleExportFeedbackLogEmptyState covers the regression net for
// issue #137 when no feedback log exists yet. The handler must surface
// an info toast via X-DixieData-Toast so the dispatcher can render it,
// and MUST NOT emit a redirect (the previous behaviour wrote a bare
// body that the dispatcher silently dropped).
//
// The success path through enqueueExport is exercised by the live
// smoke harness (audit/smoke.mjs: share-/export/feedback-log-* assertions)
// against a running dixiedata-web server, because the native save
// dialog is only available under the Wails runtime and cannot be
// mocked from a unit test.
func TestHandleExportFeedbackLogEmptyState(t *testing.T) {
	app := newStressApp(t)

	req := httptest.NewRequest(http.MethodPost, "/export/feedback-log", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-DixieData-Toast"); got == "" {
		t.Fatalf("expected X-DixieData-Toast header on empty state; got headers %v", rec.Header())
	}
	if got := rec.Header().Get("X-DixieData-Toast-Type"); got != "info" {
		t.Fatalf("expected X-DixieData-Toast-Type=info; got %q", got)
	}
	// Defence-in-depth: the empty path must NOT redirect. A redirect
	// here would race the dispatcher's toast-render path and the user
	// would see neither the toast nor any feedback log content.
	if rec.Header().Get("X-DixieData-Redirect") != "" || rec.Header().Get("Location") != "" {
		t.Fatalf("empty-state handler must not redirect; got headers %v", rec.Header())
	}
}