// exports_handlers_subset_test.go covers the ?subset=1 branch of
// handleExportSharedArchive (issue #182). The validation runs BEFORE
// the native SaveFileDialog opens, so these tests do not need a
// real database, services, or a dialog override. They exercise the
// form-parsing + empty/invalid-id paths in isolation.
//
// Future handler-level tests (the happy path, the dedup-collision
// path, the X-DixieData-Redirect contract) live alongside the
// existing handleExportSharedArchive tests in a dedicated file
// once the full app is wired with services. For now the validation
// contract is the highest-risk seam — a refactor that drops the
// empty-id check would silently let the user open a native dialog
// with nothing to export.
package appshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestExportSharedArchiveSubsetEmptyIDsReturns400 locks the
// "Add at least one Person Record to the Share queue before
// exporting." validation. An empty selected_ids POST must
// reject the request with 400 + X-DixieData-Toast BEFORE the
// native SaveFileDialog opens (issue #182 acceptance criterion).
func TestExportSharedArchiveSubsetEmptyIDsReturns400(t *testing.T) {
	app := NewApp()
	req := httptest.NewRequest(http.MethodPost, "/export/shared-archive?subset=1",
		strings.NewReader("selected_ids=&selected_ids="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.handleExportSharedArchive(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty selected_ids: status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	toast := rec.Header().Get("X-DixieData-Toast")
	if !strings.Contains(toast, "Add at least one Person Record") {
		t.Fatalf("empty selected_ids: toast = %q, want a Share Queue message", toast)
	}
}

// TestExportSharedArchiveSubsetInvalidIDsReturns400 locks the
// "Invalid Share Queue selection." validation. A non-numeric
// selected_ids value must reject the request with 400 and a
// distinct toast from the empty-id case.
func TestExportSharedArchiveSubsetInvalidIDsReturns400(t *testing.T) {
	app := NewApp()
	req := httptest.NewRequest(http.MethodPost, "/export/shared-archive?subset=1",
		strings.NewReader("selected_ids=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.handleExportSharedArchive(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid selected_ids: status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	toast := rec.Header().Get("X-DixieData-Toast")
	if !strings.Contains(toast, "Invalid Share Queue selection") {
		t.Fatalf("invalid selected_ids: toast = %q, want an invalid-selection message", toast)
	}
}

// TestExportSharedArchiveSubsetMethodGuardRejectsGET locks the
// HTTP method guard inside the handler. A GET to the subset URL
// must return 405 regardless of query params. Matches the
// postOnlyPaths integration test's surface-level check at the
// handler level.
func TestExportSharedArchiveSubsetMethodGuardRejectsGET(t *testing.T) {
	app := NewApp()
	req := httptest.NewRequest(http.MethodGet, "/export/shared-archive?subset=1", nil)
	rec := httptest.NewRecorder()

	app.handleExportSharedArchive(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET subset: status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
