package appshell

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// TestJobReportHandlerReturnsSummaryForFinishedJob is the
// regression test for the /jobs/{id}/report route that issue
// #131 wired the "Show report" button to. The handler must
// return 200 + the printable report view (JobReportView), which
// carries the summary headline + artifact metadata + a
// printable layout. Returns 404 when the job is unknown so a
// stale link does not silently render an empty page.
func TestJobReportHandlerReturnsSummaryForFinishedJob(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	app.setupRoutes()

	dir := t.TempDir()
	resultPath := filepath.Join(dir, "june-2026.ddbak")
	if err := os.WriteFile(resultPath, []byte("ddbak placeholder"), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	jobID := app.jobs.Start("backup_archive", func(_ context.Context, _ *jobs.Progress) error {
		return nil
	})
	app.jobs.SetResultPath(jobID, resultPath)
	// Wait for the worker goroutine to mark the job StatusDone so
	// the report handler's terminal-state branch renders. Without
	// this the test races the worker on slow CI runners and
	// snapshots a "queued (in progress)" body that fails the
	// "Backup archive complete" needle.
	waitDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(waitDeadline) {
		if snap, ok := app.jobs.Get(jobID); ok && snap.Status == jobs.StatusDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID+"/report", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"Job Report",
		"Backup archive complete",
		"Artifact",
		resultPath,
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("report body missing %q\nbody:\n%s", needle, body)
		}
	}
}

// TestJobReportHandler404OnUnknownJob verifies the unknown-job
// branch so a stale "Show report" link does not render an
// empty page.
func TestJobReportHandler404OnUnknownJob(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/jobs/nonexistent/report", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown job, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestJobReportHandlerRejectsNonGet pins down the method
// handler so a POST to /jobs/{id}/report (which the button
// never sends, but a hostile client could) returns 405 instead
// of silently rendering the report.
func TestJobReportHandlerRejectsNonGet(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	app.setupRoutes()

	jobID := app.jobs.Start("static_archive", func(_ context.Context, _ *jobs.Progress) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/jobs/"+jobID+"/report", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestJobReportHandlerSurvivesRunningJob covers the in-progress
// edge case: while a job is still running the user can navigate
// to /report and get a minimal "in progress" page (no panic).
func TestJobReportHandlerSurvivesRunningJob(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	app.setupRoutes()

	jobID := app.jobs.Start("static_archive", func(_ context.Context, _ *jobs.Progress) error {
		return nil
	})
	// Worker returns immediately; the job is briefly queued or
	// running. Just before shutdown the status may be either.

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID+"/report", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even for in-progress job, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), jobID) {
		t.Errorf("report body should reference the job ID %q; got body:\n%s", jobID, rec.Body.String())
	}
}

// TestJobReportHandlerRendersErrorSectionForFailedJob ensures
// the report view surfaces the error string when the job
// terminated with an error, so a user opening the report from
// a link in their email can see what went wrong without going
// back to the status page.
func TestJobReportHandlerRendersErrorSectionForFailedJob(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	app.setupRoutes()

	workerDone := make(chan struct{})
	jobID := app.jobs.Start("static_archive", func(_ context.Context, _ *jobs.Progress) error {
		defer close(workerDone)
		return assertErr("export timed out at 60s")
	})
	// Wait for the worker to complete (and mark the job as
	// errored) before querying the report route. The jobs
	// registry runs workers in goroutines so Start returns
	// before the job reaches its terminal state.
	<-workerDone

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID+"/report", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "export timed out at 60s") {
		t.Errorf("report body should surface the worker error; got:\n%s", rec.Body.String())
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }