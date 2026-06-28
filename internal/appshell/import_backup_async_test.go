package appshell

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// TestImportBackupInFlightGuardRedirectsToExistingJob is the
// regression test for issue #133 (second click during a running
// restore). The handler must not crash, must not open a second
// file dialog, and must redirect to the in-flight /jobs/{id} so
// the user lands on the real progress page.
//
// Runs without a real .dixiedata archive because the guard fires
// BEFORE the database is consulted.
func TestImportBackupInFlightGuardRedirectsToExistingJob(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	app.setupRoutes()
	app.importInFlight.Store(true)
	app.importInFlightJobIDSet("restore-job-123")
	t.Cleanup(func() {
		app.importInFlight.Store(false)
		app.importInFlightJobIDClear()
	})

	// No openFileDialogOverride — if the handler opens the dialog
	// anyway, the test will panic on nil func.

	req := httptest.NewRequest(http.MethodPost, "/import/backup", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect to existing /jobs/{id}, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/jobs/restore-job-123" {
		t.Errorf("expected Location=/jobs/restore-job-123, got %q", got)
	}
}

// TestImportBackupInFlightGuardFallsBackToError covers the
// safety path: a.restoreInFlight flag is set but no JobID is
// tracked yet (the worker hasn't reached the Set call yet, e.g.
// a stale flag from a crashed worker). The handler must return
// a 503 error instead of opening a dialog.
func TestImportBackupInFlightGuardFallsBackToError(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	app.setupRoutes()
	// importInFlightJobID deliberately unset.

	app.importInFlight.Store(true)

	req := httptest.NewRequest(http.MethodPost, "/import/backup", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code == http.StatusSeeOther {
		t.Fatalf("expected non-redirect error response, got 303 to %q", rec.Header().Get("Location"))
	}
	if got := rec.Header().Get("X-DixieData-Toast"); !strings.Contains(got, "backup restore") {
		t.Errorf("expected toast to mention backup restore, got %q", got)
	}
}