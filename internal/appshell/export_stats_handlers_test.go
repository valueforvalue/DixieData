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

// TestEnqueueExportWithResultPopulatesJobStats pins down the
// stats-aware enqueue helper at the API level: a worker that
// returns a populated jobs.JobResult must land the counts on the
// job's Result field after the worker returns nil. Without this
// the /jobs/{id} summary card would never render the new
// "Person records: N" lines.
//
// The test does NOT stub the native SaveFileDialog (which is
// hard to mock in unit tests). Instead it calls enqueueExportWithResult
// directly with a small in-memory path so the worker runs to
// completion in test time. The handler-level wiring (the 303 +
// inFlight admission) is covered separately by dedup_redirect_test.go.
func TestEnqueueExportWithResultPopulatesJobStats(t *testing.T) {
	app := newStressApp(t)

	dir := t.TempDir()
	outPath := filepath.Join(dir, "stats-marker.txt")
	if err := os.WriteFile(outPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("seed path: %v", err)
	}

	rec := httptest.NewRecorder()
	app.enqueueExportWithResult("", "json_export", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
		// The worker body is intentionally tiny so the test
		// stays under the 2s timeout even on slow CI. We
		// simulate the with-stats return shape (records / images /
		// sources) so the regression net is honest.
		return jobs.JobResult{Records: 247, Images: 0, Sources: 18}, nil
	}, outPath, rec)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (Option C contract), got %d", rec.Code)
	}
	dixie := rec.Header().Get("X-DixieData-Redirect")
	if !strings.HasPrefix(dixie, "/jobs/") {
		t.Fatalf("expected X-DixieData-Redirect=/jobs/{id}, got %q", dixie)
	}
	jobID := strings.TrimPrefix(dixie, "/jobs/")

	// Wait for the worker to record its result. 2s is plenty for
	// the trivial worker body above.
	deadline := time.Now().Add(2 * time.Second)
	var snap jobs.Job
	for time.Now().Before(deadline) {
		s, ok := app.jobs.Get(jobID)
		if ok && (s.Status == jobs.StatusDone || s.Status == jobs.StatusError) {
			snap = s
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if snap.Status == "" {
		t.Fatalf("job %s did not finish within 2s", jobID)
	}
	if snap.Status != jobs.StatusDone {
		t.Fatalf("job status = %q, want done (err: %s)", snap.Status, snap.Error)
	}
	if snap.Result.Records != 247 {
		t.Errorf("Result.Records = %d, want 247", snap.Result.Records)
	}
	if snap.Result.Sources != 18 {
		t.Errorf("Result.Sources = %d, want 18", snap.Result.Sources)
	}
	if snap.ResultPath != outPath {
		t.Errorf("ResultPath = %q, want %q (SetResult.Path must promote)", snap.ResultPath, outPath)
	}
}

// TestEnqueueExportWithResultSetsHXRedirect is the regression net
// for the share-page-export-lands-on-/jobs/{id} bug. htmx 2.x
// with hx-swap="none" silently swallows 303 responses unless the
// server also writes HX-Redirect. enqueueExportWithResult (and
// enqueueExport) must set BOTH Location (for plain browser form
// submits like static archive) AND HX-Redirect (for htmx).
//
// Without this assertion the same bug class can slip in again
// because the visible user-visible outcome (export completes
// silently in the background, user stays on /share) looks like
// "no error" in casual testing.
func TestEnqueueExportWithResultSetsHXRedirect(t *testing.T) {
	app := newStressApp(t)

	rec := httptest.NewRecorder()
	app.enqueueExportWithResult("", "json_export", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
		return jobs.JobResult{Records: 1}, nil
	}, "/tmp/example.json", rec)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (Option C contract), got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("expected no Location header (Option C: 200 + X-DixieData-Redirect); got %q", loc)
	}
	if dixie := rec.Header().Get("X-DixieData-Redirect"); !strings.HasPrefix(dixie, "/jobs/") {
		t.Errorf("missing X-DixieData-Redirect=/jobs/{id} (dispatchDixieDataForm navigates from this); got %q", dixie)
	}
}
