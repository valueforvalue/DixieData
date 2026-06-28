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

// seedArtifactJob creates a job with an artifact at the
// given path and waits for it to reach StatusDone. Returns
// the job ID. Used by the Content-Disposition tests below.
func seedArtifactJob(t *testing.T, app *App, kind, artifactPath string) string {
	t.Helper()
	if err := os.WriteFile(artifactPath, []byte("seed"), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	var id string
	id = app.jobs.Start(kind, func(ctx context.Context, p *jobs.Progress) error {
		p.Set(100, "Done")
		app.jobs.SetResultPath(id, artifactPath)
		return nil
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := app.jobs.Get(id)
		if snap.Status == jobs.StatusDone {
			return id
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %q did not reach StatusDone in time", id)
	return ""
}

// TestHandleJobArtifactInlineForViewableTypes locks the fix
// for the "blank tab after Open" bug: a finished export's
// "Open" link points at /jobs/{id}/artifact and opens in a
// new tab. PDFs, JPGs, etc. must be served inline so the
// browser RENDERS them in the new tab rather than
// downloading and leaving the tab blank.
func TestHandleJobArtifactInlineForViewableTypes(t *testing.T) {
	cases := []struct {
		ext         string
		wantDisp    string
		wantContain string
	}{
		{".pdf", "inline", "application/pdf"},
		{".jpg", "inline", "image/jpeg"},
		{".jpeg", "inline", "image/jpeg"},
		{".png", "inline", "image/png"},
		{".html", "inline", "text/html"},
		{".txt", "inline", "text/plain"},
	}
	for _, tc := range cases {
		app := newStressApp(t)
		dir := t.TempDir()
		path := filepath.Join(dir, "export"+tc.ext)
		id := seedArtifactJob(t, app, "static_archive", path)

		req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/artifact", nil)
		rec := httptest.NewRecorder()
		app.handleJobStatus(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("ext=%s: expected 200, got %d", tc.ext, rec.Code)
		}
		disp := rec.Header().Get("Content-Disposition")
		if !strings.Contains(disp, tc.wantDisp) {
			t.Errorf("ext=%s: Content-Disposition should be %q, got %q", tc.ext, tc.wantDisp, disp)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, tc.wantContain) {
			t.Errorf("ext=%s: Content-Type should contain %q, got %q", tc.ext, tc.wantContain, ct)
		}
	}
}

// TestHandleJobArtifactAttachmentForDownloadTypes locks the
// other half of the fix: .ddbak, .ddshare, .zip, .csv, .ics
// must still download (Content-Disposition: attachment) so
// the browser saves them to disk instead of trying to
// render them as HTML. .json is intentionally NOT in this
// list because browsers render JSON natively (it's in the
// inline map).
func TestHandleJobArtifactAttachmentForDownloadTypes(t *testing.T) {
	cases := []struct {
		ext string
		ct  string
	}{
		{".ddbak", "application/octet-stream"},
		{".ddshare", "application/octet-stream"},
		{".zip", "application/octet-stream"},
		{".csv", "application/octet-stream"},
		{".ics", "application/octet-stream"},
	}
	for _, tc := range cases {
		app := newStressApp(t)
		dir := t.TempDir()
		path := filepath.Join(dir, "export"+tc.ext)
		id := seedArtifactJob(t, app, "static_archive", path)

		req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/artifact", nil)
		rec := httptest.NewRecorder()
		app.handleJobStatus(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("ext=%s: expected 200, got %d", tc.ext, rec.Code)
		}
		disp := rec.Header().Get("Content-Disposition")
		if !strings.HasPrefix(disp, "attachment;") {
			t.Errorf("ext=%s: Content-Disposition should start with attachment;, got %q", tc.ext, disp)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, tc.ct) {
			t.Errorf("ext=%s: Content-Type should contain %q, got %q", tc.ext, tc.ct, ct)
		}
	}
}

// TestJobArtifactHeaders_Unit is a fast check on the
// header-selector helper that doesn't stand up a job. The
// real job-based tests above cover the wire path.
func TestJobArtifactHeaders_Unit(t *testing.T) {
	cases := []struct {
		path        string
		wantDisp    string
		wantContain string
	}{
		{"/tmp/export.pdf", "inline", "application/pdf"},
		{"/tmp/export.JPG", "inline", "image/jpeg"}, // case-insensitive
		{"/tmp/export.ddbak", "attachment", "octet-stream"},
		{"/tmp/with spaces.ddbak", "attachment", "octet-stream"},
		{"/tmp/quote\".ddbak", "attachment", "octet-stream"},
		{"/tmp/no-extension", "attachment", "octet-stream"}, // no ext = download
	}
	for _, tc := range cases {
		disp, ct := jobArtifactHeaders(tc.path)
		if !strings.Contains(disp, tc.wantDisp) {
			t.Errorf("path=%q: disposition should contain %q, got %q", tc.path, tc.wantDisp, disp)
		}
		if !strings.Contains(ct, tc.wantContain) {
			t.Errorf("path=%q: content-type should contain %q, got %q", tc.path, tc.wantContain, ct)
		}
	}
}

func TestRenderActiveJobReturns204WhenNoActiveJobs(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodGet, "/jobs/active", nil)
	rec := httptest.NewRecorder()
	app.renderActiveJob(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 when no active jobs, got %d", rec.Code)
	}
}

func TestRenderActiveJobReturnsSlotFragmentForLatest(t *testing.T) {
	app := newStressApp(t)
	// Long-running worker so the test can observe the running job
	// before it finishes and disappears from MostRecentActive.
	hold := make(chan struct{})
	id := app.jobs.Start("static_archive", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(50, "halfway")
		<-hold
		return nil
	})
	defer close(hold)
	// Wait for the worker to mark the job running.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := app.jobs.Get(id)
		if snap.Status == jobs.StatusRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	req := httptest.NewRequest(http.MethodGet, "/jobs/active", nil)
	rec := httptest.NewRecorder()
	app.renderActiveJob(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with active job, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data-progress-region") {
		t.Fatalf("slot fragment should target [data-progress-region]; got:\n%s", body)
	}
	if !strings.Contains(body, "static_archive") && !strings.Contains(body, "Static web archive") {
		t.Fatalf("slot fragment should show job label; got:\n%s", body)
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