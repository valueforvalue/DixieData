package templates

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestJobStatusViewArtifactOpenButtonNoTargetBlank is the regression
// test for issue #129 (continued). The earlier fix replaced
// target="_blank" with a download attribute, but that produced a
// second, confusing download in the Wails desktop flow because the
// file is already at the user's chosen destination. The 2026-06-30
// revision (issue #166) removed the 'Open file' button entirely
// because runtime.BrowserOpenURL does nothing in the user's
// runtime; the Copy-path button is the only artifact affordance.
// The page must NOT carry target=_blank, href to /artifact, a
// download attribute, or the POST form against /jobs/{id}/open.
func TestJobStatusViewArtifactOpenButtonNoTargetBlank(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "june-2026.ddbak")
	if err := os.WriteFile(resultPath, []byte("ddbak placeholder"), 0o644); err != nil {
		t.Fatalf("seed ddbak file: %v", err)
	}
	job := jobs.NewJob("job-ddbak", "backup_archive")
	job.Status = jobs.StatusDone
	job.StartedAt = time.Now().Add(-2 * time.Second)
	job.FinishedAt = time.Now()
	job.ResultPath = resultPath

	var buf bytes.Buffer
	if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusView render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, `action="/jobs/job-ddbak/open"`) {
		t.Errorf("status page must NOT wire artifact to POST /jobs/{id}/open (issue #166 — button removed); got HTML:\n%s", html)
	}
	if strings.Contains(html, ">Open file<") {
		t.Errorf("status page must NOT render 'Open file' button (issue #166); got HTML:\n%s", html)
	}
	if !strings.Contains(html, `data-copy-path=`) {
		t.Errorf("status page must expose a Copy-path button (data-copy-path); got HTML:\n%s", html)
	}
	if strings.Contains(html, `target="_blank"`) {
		t.Errorf("status page must NOT use target=_blank for any artifact (issue #129); got HTML:\n%s", html)
	}
	if strings.Contains(html, `href="/jobs/job-ddbak/artifact"`) {
		t.Errorf("status page must NOT link to /jobs/{id}/artifact (downloads are confusing in Wails where the file is already at the chosen path); got HTML:\n%s", html)
	}
	if strings.Contains(html, `download="`) {
		t.Errorf("status page must NOT carry download= (the file is already saved, not re-downloaded); got HTML:\n%s", html)
	}
}

// TestJobStatusViewSummaryCardHasDismissAndShowReport is the
// regression test for issue #131. The redesigned status page
// must show a summary card on the done state with Dismiss and
// Show report buttons (primary CTAs) and demote the artifact
// link (Open / Save) to a secondary action.
func TestJobStatusViewSummaryCardHasDismissAndShowReport(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "june-2026.ddbak")
	if err := os.WriteFile(resultPath, []byte("ddbak placeholder"), 0o644); err != nil {
		t.Fatalf("seed ddbak file: %v", err)
	}
	job := jobs.NewJob("job-ddbak", "backup_archive")
	job.Status = jobs.StatusDone
	job.StartedAt = time.Now().Add(-3 * time.Second)
	job.FinishedAt = time.Now()
	job.ResultPath = resultPath

	var buf bytes.Buffer
	if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusView render: %v", err)
	}
	html := buf.String()

	// Primary CTAs must be present.
	if !strings.Contains(html, ">Dismiss<") {
		t.Errorf("summary card must include a Dismiss button; got HTML:\n%s", html)
	}
	if !strings.Contains(html, `>Show report<`) {
		t.Errorf("summary card must include a Show report button; got HTML:\n%s", html)
	}
	// Dismiss button must route to the kind-specific dismiss
	// path (backup_archive -> /share). The attribute is HTML-
	// escaped so check both encodings.
	if !strings.Contains(html, `/share`) || !strings.Contains(html, `Dismiss`) {
		t.Errorf("Dismiss button must route to /share for backup_archive; got HTML:\n%s", html)
	}
	// Show report button must link to /jobs/{id}/report.
	if !strings.Contains(html, `/jobs/job-ddbak/report`) {
		t.Errorf("Show report button must link to /jobs/{id}/report; got HTML:\n%s", html)
	}
	// Summary headline must include the size (so the user sees
	// the structured payload from job.Summary()).
	if !strings.Contains(html, "Backup archive complete") {
		t.Errorf("summary headline missing for backup_archive; got HTML:\n%s", html)
	}
	// Detail lines must include size + duration.
	if !strings.Contains(html, "Size:") || !strings.Contains(html, "Duration:") {
		t.Errorf("summary detail lines missing; got HTML:\n%s", html)
	}
}

// TestJobStatusViewStillShowsProgressWhileRunning ensures the
// progress bar still appears for queued/running jobs after the
// redesign (issue #131 should not regress the in-progress UX).
func TestJobStatusViewStillShowsProgressWhileRunning(t *testing.T) {
	job := jobs.NewJob("job-running", "static_archive")
	job.Status = jobs.StatusRunning
	job.Progress = 42

	var buf bytes.Buffer
	if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusView render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `aria-label="Export progress"`) {
		t.Errorf("progress bar must still render while running; got HTML:\n%s", html)
	}
	if !strings.Contains(html, `width: 42%`) {
		t.Errorf("progress width must reflect the running percentage; got HTML:\n%s", html)
	}
}

// TestJobStatusFragmentArtifactOpenForm is the polling-fragment
// counterpart for the /jobs/{id} status page. The fragment is what
// htmx swaps into the layout progress slot every 2s while the page
// polls for terminal state. The 2026-06-30 revision (issue #166)
// removed the 'Open file' button from the layout slot because
// runtime.BrowserOpenURL does nothing in the user's runtime.
// Fragment must NOT carry the POST form against /jobs/{id}/open,
// target=_blank, href to /artifact, or a download attribute.
func TestJobStatusFragmentArtifactOpenForm(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "june-2026.ddbak")
	if err := os.WriteFile(resultPath, []byte("ddbak placeholder"), 0o644); err != nil {
		t.Fatalf("seed ddbak file: %v", err)
	}
	job := jobs.NewJob("job-ddbak", "backup_archive")
	job.Status = jobs.StatusDone
	job.StartedAt = time.Now().Add(-2 * time.Second)
	job.FinishedAt = time.Now()
	job.ResultPath = resultPath

	var buf bytes.Buffer
	if err := JobStatusFragment(*job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusFragment render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, `action="/jobs/job-ddbak/open"`) {
		t.Errorf("fragment must NOT wire artifact to POST /jobs/{id}/open (issue #166 — button removed); got HTML:\n%s", html)
	}
	if strings.Contains(html, ">Open file<") {
		t.Errorf("fragment must NOT render 'Open file' button (issue #166); got HTML:\n%s", html)
	}
	if strings.Contains(html, `target="_blank"`) {
		t.Errorf("fragment must NOT use target=_blank; got HTML:\n%s", html)
	}
}

// TestJobStatusViewPollsForUpdates is the regression test for the
// "static archive status page never updates" bug. Before the fix
// JobStatusView rendered a static snapshot (the body had no
// hx-get / hx-trigger), so the page froze at the value captured
// in the 303 redirect even while the job ran to completion in the
// background. Static_archive exports are fast enough that the
// page almost always landed after the job had already finished,
// which made the symptom obvious: status forever reads
// "running" / "queued" while the artifact sits ready in /jobs/{id}.
//
// The fix extracts the body into jobStatusBody so both the full
// page (JobStatusView) and the polling fragment (JobStatusFragment)
// share one source of truth. The body wires the 2s hx-get against
// /jobs/{id}/status so the full page auto-polls exactly the same
// way the fragment does.
//
// Asserts:
//  1. The page's #job-status-body wrapper carries hx-get against
//     /jobs/{id}/status (proves the poll is wired).
//  2. The trigger is "every 2s" (proves it polls periodically).
//  3. When the job is in a terminal state (done) the body emits
//     hx-trigger="none" so polling stops once the user sees
//     the summary card.
//  4. The fragment and the page render the same hx-get URL so
//     a swap cannot land on a stale target.
func TestJobStatusViewPollsForUpdates(t *testing.T) {
	t.Run("running job wires the poll", func(t *testing.T) {
		job := jobs.NewJob("job-running", "static_archive")
		job.Status = jobs.StatusRunning
		job.Progress = 35

		var buf bytes.Buffer
		if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
			t.Fatalf("JobStatusView render: %v", err)
		}
		html := buf.String()

		if !strings.Contains(html, `hx-get="/jobs/job-running/status"`) {
			t.Errorf("running JobStatusView must hx-get the /jobs/{id}/status endpoint; the landing page was a static snapshot before the body extraction, so a fast export like static_archive finished while the page sat there. Got HTML:\n%s", html)
		}
		if !strings.Contains(html, `hx-trigger="every 2s"`) {
			t.Errorf("running JobStatusView must poll every 2s; got HTML:\n%s", html)
		}
		if !strings.Contains(html, `id="job-status-body"`) {
			t.Errorf("running JobStatusView must render #job-status-body so htmx can find its swap target; got HTML:\n%s", html)
		}
	})

	t.Run("done job stops polling", func(t *testing.T) {
		dir := t.TempDir()
		resultPath := filepath.Join(dir, "archive.zip")
		if err := os.WriteFile(resultPath, []byte("PK\x03\x04 placeholder"), 0o644); err != nil {
			t.Fatalf("seed zip: %v", err)
		}
		job := jobs.NewJob("job-done", "static_archive")
		job.Status = jobs.StatusDone
		job.StartedAt = time.Now().Add(-2 * time.Second)
		job.FinishedAt = time.Now()
		job.ResultPath = resultPath

		var buf bytes.Buffer
		if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
			t.Fatalf("JobStatusView render: %v", err)
		}
		html := buf.String()

		if !strings.Contains(html, `hx-trigger="none"`) {
			t.Errorf("terminal JobStatusView must stop polling (hx-trigger=none); got HTML:\n%s", html)
		}
		if strings.Contains(html, `hx-get="/jobs/job-done/status"`) {
			t.Errorf("terminal JobStatusView must NOT keep polling; got HTML:\n%s", html)
		}
	})

	t.Run("view and fragment share the poll url", func(t *testing.T) {
		// The view's first poll and every subsequent swap target
		// must resolve to the same URL. If the view rendered one
		// URL and the fragment another, the first swap would land
		// on a target that the next fragment response could not
		// update. The body extraction guarantees both render
		// through jobStatusBody so they cannot drift.
		job := jobs.NewJob("job-aligned", "static_archive")
		job.Status = jobs.StatusRunning
		job.Progress = 10

		var viewBuf, fragBuf bytes.Buffer
		if err := JobStatusView(*job).Render(context.Background(), &viewBuf); err != nil {
			t.Fatalf("JobStatusView render: %v", err)
		}
		if err := JobStatusFragment(*job).Render(context.Background(), &fragBuf); err != nil {
			t.Fatalf("JobStatusFragment render: %v", err)
		}

		viewHTML := viewBuf.String()
		fragHTML := fragBuf.String()

		viewPoll := strings.Contains(viewHTML, `hx-get="/jobs/job-aligned/status"`)
		fragPoll := strings.Contains(fragHTML, `hx-get="/jobs/job-aligned/status"`)
		if !viewPoll || !fragPoll {
			t.Errorf("view and fragment must both wire hx-get=/jobs/job-aligned/status; viewPoll=%v fragPoll=%v\n--- view ---\n%s\n--- fragment ---\n%s", viewPoll, fragPoll, viewHTML, fragHTML)
		}
	})
}

// TestJobStatusViewOmitsJobsOverlay locks down the choice to hide
// the floating jobs-progress popup on /jobs/{id}. The page renders
// the job's progress bar inline; the popup would either duplicate
// the data (when the active job matches) or poll silently for a
// different job (when it doesn't). Layout has a variadic
// omitJobsOverlay option; JobStatusView passes true so the popup
// is stripped. Other call sites (@Layout("Browse"), @Layout(...))
// continue to render the popup.
func TestJobStatusViewOmitsJobsOverlay(t *testing.T) {
	job := jobs.NewJob("job-overlay-omit", "static_archive")
	job.Status = jobs.StatusRunning
	job.Progress = 50

	var buf bytes.Buffer
	if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusView render: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, "data-jobs-progress-region") {
		t.Errorf("/jobs/{id} must omit the floating jobs-progress popup; got HTML:\n%s", html)
	}
}

// TestSharePageKeepsJobsOverlay is the inverse pin: the share page
// (and any other page that does NOT pass omitJobsOverlay) must
// keep emitting the popup so the active background export's
// progress card still appears. The audit smoke test
// 'jobs-progress-overlay-survives-polls' exercises this end-to-end.
func TestSharePageKeepsJobsOverlay(t *testing.T) {
	var buf bytes.Buffer
	if err := ShareView(viewmodel.GoogleStatus{}, nil, nil, viewmodel.ArchiveCounts{}, false).Render(context.Background(), &buf); err != nil {
		t.Fatalf("ShareView render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "data-jobs-progress-region") {
		t.Errorf("/share page must keep the floating jobs-progress popup; got HTML:\n%s", html)
	}
}

// TestJobStatusViewRendersAwaitingConfirmationCard locks down the
// Memorial-JSON confirm-before-run UX. When a job is in
// StatusQueued + AwaitingConfirmation (set by Registry.StartManual),
// /jobs/{id} renders a 'Confirm and proceed' + 'Cancel import'
// card with the preflight summary, instead of the standard
// progress widget.
func TestJobStatusViewRendersAwaitingConfirmationCard(t *testing.T) {
	j := jobs.NewJob("job-memorial-confirm", "memorial_import")
	j.Status = jobs.StatusQueued
	j.AwaitingConfirmation = true
	j.Message = "Awaiting confirmation: will create 50, skip 12, fail 3 (of 65 rows)."

	var buf bytes.Buffer
	if err := JobStatusView(*j).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusView render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Awaiting confirmation") {
		t.Errorf("confirmation card must show the 'Awaiting confirmation' badge; got HTML:\n%s", html)
	}
	if !strings.Contains(html, j.Message) {
		t.Errorf("confirmation card must include the seeded preflight Message %q; got HTML:\n%s", j.Message, html)
	}
	if !strings.Contains(html, `action="/jobs/job-memorial-confirm/confirm"`) {
		t.Errorf("confirmation card must wire a POST to /jobs/{id}/confirm; got HTML:\n%s", html)
	}
	if !strings.Contains(html, "data-job-confirm") {
		t.Errorf("confirmation card must expose data-job-confirm hook for tests; got HTML:\n%s", html)
	}
	if !strings.Contains(html, `action="/jobs/job-memorial-confirm/cancel"`) {
		t.Errorf("confirmation card must wire a POST to /jobs/{id}/cancel; got HTML:\n%s", html)
	}
	if !strings.Contains(html, "data-job-cancel") {
		t.Errorf("confirmation card must expose data-job-cancel hook; got HTML:\n%s", html)
	}
	if strings.Contains(html, "aria-label=\"Export progress\"") {
		t.Errorf("confirmation card must NOT include the progress widget; got HTML:\n%s", html)
	}
}