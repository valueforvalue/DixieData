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

func TestHandleJobStatusUnknownJobReturns404(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodGet, "/jobs/does-not-exist", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleJobStatusFragmentRoute(t *testing.T) {
	app := newStressApp(t)
	id := app.jobs.Start("unit", func(ctx context.Context, p *jobs.Progress) error { return nil })
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/status", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "job-status-body") {
		t.Fatalf("status fragment should render the job-status-body wrapper; got:\n%s", body)
	}
}

func TestHandleJobCancelUnknownJobReturns404(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodPost, "/jobs/missing/cancel", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleJobArtifactStreamsResultFile(t *testing.T) {
	app := newStressApp(t)
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "export.zip")
	if err := os.WriteFile(artifactPath, []byte("PK\x03\x04sample"), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	var id string
	id = app.jobs.Start("static_archive", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(100, "Done")
		app.jobs.SetResultPath(id, artifactPath)
		return nil
	})
	// Wait for the worker to finish so ResultPath is populated.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := app.jobs.Get(id)
		if snap.Status == jobs.StatusDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/artifact", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "export.zip") {
		t.Fatalf("Content-Disposition should include filename; got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "PK") {
		t.Fatalf("artifact body should stream the file contents")
	}
}

func TestHandleJobArtifactUnknownJobReturns404(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodGet, "/jobs/missing/artifact", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleJobArtifactMissingFileReturns500(t *testing.T) {
	app := newStressApp(t)
	var id string
	id = app.jobs.Start("static_archive", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(100, "Done")
		app.jobs.SetResultPath(id, "/nonexistent/path/that/does/not/exist.zip")
		return nil
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := app.jobs.Get(id)
		if snap.Status == jobs.StatusDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/artifact", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code < 400 {
		t.Fatalf("expected error status for missing file, got %d", rec.Code)
	}
}