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

// TestHandleJobStatusFullPageWiresThePoll is the end-to-end net
// for the "static archive status page never updates" bug. The
// full-page route /jobs/{id} is what the user lands on after the
// 303 redirect from the export handler; before the body
// extraction it rendered a static snapshot (no hx-get /
// hx-trigger), so a job that finished during the redirect
// window left the page reading "running" forever even though
// the artifact sat ready in /jobs/{id}/artifact. The fix wires
// the same 2s poll into the full page that the fragment uses.
//
// The test holds the job open on a channel so the worker
// cannot transition to a terminal state during the render.
// Without that hold the static_archive worker returns
// immediately and the rendered page legitimately emits
// hx-trigger="none" (because the body has the same terminal
// branch as the fragment). The point of the test is that the
// running-state branch wires the poll; the terminal branch is
// covered by TestJobStatusViewPollsForUpdates/done_job_stops_polling.
func TestHandleJobStatusFullPageWiresThePoll(t *testing.T) {
	app := newStressApp(t)
	hold := make(chan struct{})
	id := app.jobs.Start("static_archive", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(50, "Gathering images")
		<-hold
		return nil
	})
	t.Cleanup(func() { close(hold) })
	// Wait for the worker to mark the job running so the body
	// chooses the poll branch (not the terminal-stop branch).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := app.jobs.Get(id)
		if snap.Status == jobs.StatusRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id, nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `hx-get="/jobs/`+id+`/status"`) {
		t.Fatalf("full /jobs/{id} page must wire hx-get against the polling endpoint so the page auto-updates while the job runs; the body extraction in templates/jobs.templ guarantees both the view and the fragment share the same source of truth. Got body:\n%s", body)
	}
	if !strings.Contains(body, `hx-trigger="every 2s"`) {
		t.Errorf("full /jobs/{id} page must poll every 2s; got body:\n%s", body)
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
	// Bind jobID into the closure via a buffered channel. The worker
	// blocks on a channel receive until the test goroutine has the
	// id from Start() and writes it to the channel. This eliminates
	// the race where the worker goroutine fires before Start returns
	// and `id` is still empty — pre-fix this manifested as either
	// a panic in the worker (`Load().(string)` on a nil atomic.Value)
// or a 409 in the handler (ResultPath was set against an empty id
	// and became a silent no-op, so the snapshot had no ResultPath).
	// See commit ec451f4's follow-up for the original regression net;
	// the channel-based approach is simpler than atomic.Value polling
	// and avoids the nil-load panic that started surfacing on the
	// GitHub Actions runner after the reloadServices() change.
	idCh := make(chan string, 1)
	id := app.jobs.Start(kind, func(ctx context.Context, p *jobs.Progress) error {
		var id string
		select {
		case id = <-idCh:
		case <-ctx.Done():
			return ctx.Err()
		}
		p.Set(100, "Done")
		app.jobs.SetResultPath(id, artifactPath)
		return nil
	})
	idCh <- id
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

// TestHandleJobArtifactInlineForViewableTypes locks the
// artifact-endpoint's inline disposition for viewable types
// (PDF, images, HTML, text). JSON is intentionally NOT in this
// list because browsers render JSON natively and the artifact
// endpoint treats it as inline via the disposition helper.
// (See TestJobArtifactHeaders_Unit for the JSON header check.)
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
// attachment branch for binary and large-text exports that
// shouldn't be rendered inline (.ddbak, .ddshare, .zip, .csv,
// .ics). The artifact endpoint sets Content-Disposition:
// attachment so the browser downloads the file.
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
	// before it finishes and disappears from MostRecentActive. Use
	// a non-silent kind; static_archive is in jobs.SilentKinds and
	// is covered separately by TestRenderActiveJobSuppressesSilentKinds.
	hold := make(chan struct{})
	id := app.jobs.Start("json_export", func(ctx context.Context, p *jobs.Progress) error {
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
	if !strings.Contains(body, "data-jobs-progress-region") {
		t.Fatalf("slot fragment should target [data-jobs-progress-region]; got:\n%s", body)
	}
	if !strings.Contains(body, "json_export") {
		t.Fatalf("slot fragment should show job label; got:\n%s", body)
	}
}

// TestRenderActiveJobSuppressesSilentKinds is the end-to-end net
// for the static-archive popup regression: when the most recent
// running job is a kind in jobs.SilentKinds, /jobs/active returns
// 204 No Content so the layout popup stays empty. The user lands
// on /jobs/{id} via the standard 303 and never sees the floating
// card whose "Open result" button would otherwise open a blank
// tab (zip artifacts don't preview well in a new tab).
func TestRenderActiveJobSuppressesSilentKinds(t *testing.T) {
	app := newStressApp(t)
	hold := make(chan struct{})
	id := app.jobs.Start("static_archive", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(50, "halfway")
		<-hold
		return nil
	})
	defer close(hold)
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
	if rec.Code != http.StatusNoContent {
		t.Fatalf("silent static_archive should NOT render a popup card; got status %d body:\n%s", rec.Code, rec.Body.String())
	}
	// The job itself is still poll-able at /jobs/{id}, so the
	// user is not stranded — the 303 from the export handler
	// already took them there.
	recJob := httptest.NewRecorder()
	app.handleJobStatus(recJob, httptest.NewRequest(http.MethodGet, "/jobs/"+id, nil))
	if recJob.Code != http.StatusOK {
		t.Fatalf("silent job should still render /jobs/%s (status %d)", id, recJob.Code)
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

// TestRenderJobStatusShowsExportStatsOnSummaryCard is the e2e net
// for the new stats lines. When a worker calls SetResult with
// populated counts, the /jobs/{id} summary card must render the
// "Person records: N" (and "Images: N" / "Source records: N")
// lines so the user sees what the export actually contained.
//
// Without this regression net the Summary() extension could be
// silently dropped (e.g. a future refactor moves the helper into
// a kind that no longer calls it) and the user would see only
// size + duration on the card.
func TestRenderJobStatusShowsExportStatsOnSummaryCard(t *testing.T) {
	app := newStressApp(t)
	var id string
	id = app.jobs.Start("backup_archive", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(100, "Done")
		// Mirror the real worker: a backup manifest with 247
		// soldiers, 312 images, 18 source records.
		app.jobs.SetResult(id, jobs.JobResult{
			Records: 247,
			Images:  312,
			Sources: 18,
		})
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
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id, nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Person records: 247") {
		t.Errorf("summary card missing 'Person records: 247' line; got body:\n%s", body)
	}
	if !strings.Contains(body, "Images: 312") {
		t.Errorf("summary card missing 'Images: 312' line; got body:\n%s", body)
	}
	if !strings.Contains(body, "Source records: 18") {
		t.Errorf("summary card missing 'Source records: 18' line; got body:\n%s", body)
	}
}

// TestRenderJobStatusShowsImportStatsOnSummaryCard is the
// import-side counterpart. A shared-import worker that records
// Added/Merged/Skipped/Conflicts must render those counts on the
// /jobs/{id} summary card; a backup-import worker that records
// ReplacedRecords + MigrationRan must render the replace + schema
// line.
func TestRenderJobStatusShowsImportStatsOnSummaryCard(t *testing.T) {
	t.Run("shared import", func(t *testing.T) {
		app := newStressApp(t)
		var id string
		id = app.jobs.Start("shared_import", func(ctx context.Context, p *jobs.Progress) error {
			p.Set(100, "Done")
			app.jobs.SetResult(id, jobs.JobResult{
				Added:          5,
				Merged:         3,
				Skipped:        12,
				Conflicts:      2,
				ImagesImported: 14,
			})
			return nil
		})
		waitJobDone(t, app, id)
		body := renderJobBody(t, app, id)
		if !strings.Contains(body, "5 added, 3 merged, 12 skipped") {
			t.Errorf("missing merge headline; got body:\n%s", body)
		}
		if !strings.Contains(body, "Conflicts staged for review: 2") {
			t.Errorf("missing conflicts reminder; got body:\n%s", body)
		}
		if !strings.Contains(body, "Images imported: 14") {
			t.Errorf("missing images imported line; got body:\n%s", body)
		}
	})
	t.Run("backup restore", func(t *testing.T) {
		app := newStressApp(t)
		var id string
		id = app.jobs.Start("backup_import", func(ctx context.Context, p *jobs.Progress) error {
			p.Set(100, "Done")
			app.jobs.SetResult(id, jobs.JobResult{
				ReplacedRecords: 247,
				ReplacedImages:  312,
				BackupSchema:    5,
				CurrentSchema:   7,
				MigrationRan:    true,
			})
			return nil
		})
		waitJobDone(t, app, id)
		body := renderJobBody(t, app, id)
		if !strings.Contains(body, "Replaced: 247 records, 312 images") {
			t.Errorf("missing replaced line; got body:\n%s", body)
		}
		if !strings.Contains(body, "Schema migrated: backup v5 → current v7") {
			t.Errorf("missing migration line; got body:\n%s", body)
		}
	})
}

func waitJobDone(t *testing.T, app *App, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, ok := app.jobs.Get(id)
		if ok && snap.Status == jobs.StatusDone {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish within 1s", id)
}

func renderJobBody(t *testing.T, app *App, id string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id, nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	return rec.Body.String()
}
// TestStreamJobLogMissingReturns404 covers the case where the user
// hits /jobs/{id}/log for a job whose worker never populated
// Result.LogPath. The endpoint must respond 404 so the user
// sees a clear "no log available" instead of a 200 with an
// empty body. Closes #159 regression net.
func TestStreamJobLogMissingReturns404(t *testing.T) {
	app := newStressApp(t)
	id := app.jobs.Start("memorial_import", func(ctx context.Context, p *jobs.Progress) error { return nil })
	// Drain the job so the SetResult doesn't fight the test.
	waitJobDone(t, app, id)

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/log", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing LogPath, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

// TestStreamJobLogRejectsPathOutsideTempDir covers the path-traversal
// defence: even if the job registry is tricked into storing an
// attacker-controlled LogPath (which the current code paths
// prevent — only the backend writes it via writeMemorialImportErrorLog
// in os.TempDir()), the handler must refuse to stream files outside
// os.TempDir(). Returning 403 (not 404) makes the rejection visible
// to the user without leaking the path.
func TestStreamJobLogRejectsPathOutsideTempDir(t *testing.T) {
	app := newStressApp(t)
	id := app.jobs.Start("memorial_import", func(ctx context.Context, p *jobs.Progress) error { return nil })
	waitJobDone(t, app, id)

	// Seed an out-of-tempdir path. Pick a path that is guaranteed
	// outside os.TempDir() regardless of platform — the repo's own
	// go.mod lives at the repo root, well above any temp
	// directory on Windows or POSIX.
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	app.jobs.SetResult(id, jobs.JobResult{LogPath: filepath.Join(repoRoot, "go.mod")})

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/log", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for path outside os.TempDir(), got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

// TestStreamJobLogStreamsFile is the happy path: a job with a
// LogPath inside os.TempDir() must return 200 + the file bytes +
// Content-Disposition: attachment header. This is the net for
// the missing-affordance bug in docs/ui-map/gaps.md: the
// backend had the data, only the route + handler were missing.
func TestStreamJobLogStreamsFile(t *testing.T) {
	app := newStressApp(t)
	id := app.jobs.Start("memorial_import", func(ctx context.Context, p *jobs.Progress) error { return nil })
	waitJobDone(t, app, id)

	// Write a real temp file and seed the job with its path.
	tmpFile, err := os.CreateTemp("", "dixiedata-memorial-import-*.log")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	want := "row=1 memorial_id=\"abc\" name=\"John\" error=\"bad date\"\nrow=2 memorial_id=\"def\" name=\"Jane\" error=\"missing unit\"\n"
	if _, err := tmpFile.WriteString(want); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	app.jobs.SetResult(id, jobs.JobResult{LogPath: tmpFile.Name()})

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/log", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != want {
		t.Errorf("body mismatch:\nwant=%q\ngot=%q", want, got)
	}
	disp := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disp, "attachment;") {
		t.Errorf("Content-Disposition should be attachment; got %q", disp)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type should be text/plain; got %q", ct)
	}
}

// TestStreamJobLogRejectsMissingFileOnDisk returns 410 (Gone) when
// LogPath is set but the file has been removed (e.g. temp cleanup
// between job completion and user clicking the link).
func TestStreamJobLogRejectsMissingFileOnDisk(t *testing.T) {
	app := newStressApp(t)
	id := app.jobs.Start("memorial_import", func(ctx context.Context, p *jobs.Progress) error { return nil })
	waitJobDone(t, app, id)

	// Create + immediately delete so the path looks valid but the
	// file is gone.
	tmpFile, err := os.CreateTemp("", "dixiedata-memorial-import-gone-*.log")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	gonePath := tmpFile.Name()
	tmpFile.Close()
	os.Remove(gonePath)
	app.jobs.SetResult(id, jobs.JobResult{LogPath: gonePath})

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/log", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410 for missing file, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}
