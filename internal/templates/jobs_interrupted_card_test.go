package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// Regression net for issue #551 — interrupted jobs must render a
// terminal card on /jobs/{id} AND /jobs/{id}/report. Before the fix
// the status page had no branch for jobs.StatusInterrupted (only
// Done / Error / Cancelled), so the page fell through with no
// terminal card and the user landed on what looked like an
// in-progress page that never moves; the polling loop terminated
// correctly (jobs_templ_test.go::TestJobsFragmentStopsPollingOnTerminalState
// pins that) but nothing rendered to replace the running widget.
// The report page's else-arm misclassified `interrupted` as
// `(in progress)` — a status that will NEVER progress further.
//
// These tests render both pages for an interrupted job and assert:
//   - the InterruptedCard partial renders (data-jobs-interrupted)
//   - the worker's last Message appears on the card
//   - the broken `(in progress)` text does NOT appear on either page
//   - the report page carries start/finish timestamps
//
// The third sub-test pins the Done card so the slice-2 implementation
// does not regress the existing terminal-state summary card.

const interruptedMessage = "Moved 3 image(s) into temp trash."

func newInterruptedJob(id, kind string) jobs.Job {
	startedAt := time.Now().Add(-2 * time.Second)
	job := jobs.NewJob(id, kind)
	job.Status = jobs.StatusInterrupted
	job.Progress = 50
	job.Message = interruptedMessage
	job.StartedAt = startedAt
	job.FinishedAt = startedAt.Add(800 * time.Millisecond)
	return *job
}

// TestJobsStatusPage_RendersInterruptedCard asserts the /jobs/{id}
// status page renders the new InterruptedCard partial for a job in
// StatusInterrupted. RED on the pre-fix code: no branch in
// jobStatusBody (internal/templates/jobs.templ:146-199) handled
// StatusInterrupted, so the page rendered only the polling wrapper
// + an empty body, with neither the marker nor the worker's last
// Message.
func TestJobsStatusPage_RendersInterruptedCard(t *testing.T) {
	job := newInterruptedJob("job-int", "image_orphan_cleanup")

	var buf bytes.Buffer
	if err := JobStatusView(job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusView render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `data-jobs-interrupted`) {
		t.Errorf("status page must render the InterruptedCard partial for StatusInterrupted (data-jobs-interrupted marker); got HTML:\n%s", html)
	}
	if !strings.Contains(html, interruptedMessage) {
		t.Errorf("status page InterruptedCard must surface the worker's last Message %q; got HTML:\n%s", interruptedMessage, html)
	}
	if strings.Contains(html, "(in progress)") {
		t.Errorf("status page must NOT classify interrupted as '(in progress)' (issue #551 bug shape); got HTML:\n%s", html)
	}
}

// TestJobsReportPage_RendersInterruptedTerminal asserts the
// /jobs/{id}/report page renders the interrupted branch with the
// Message + start/finish timestamps. RED on the pre-fix code: the
// status-classification else-arm at jobs.templ:243-285 printed
// `{ job.Status } (in progress).` which misclassified interrupted
// (a terminal state) as in-progress.
func TestJobsReportPage_RendersInterruptedTerminal(t *testing.T) {
	job := newInterruptedJob("job-int", "image_orphan_cleanup")

	var buf bytes.Buffer
	if err := JobReportView(job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobReportView render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, "(in progress)") {
		t.Errorf("report page must NOT classify interrupted as '(in progress)' (issue #551 bug shape); got HTML:\n%s", html)
	}
	if !strings.Contains(html, interruptedMessage) {
		t.Errorf("report page interrupted branch must surface the worker's last Message %q; got HTML:\n%s", interruptedMessage, html)
	}
	// Timeline section carries StartedAt + FinishedAt formatted
	// via "2006-01-02 15:04:05 MST". At least one of the two
	// timestamps must be present (StartedAt is always populated
	// for an interrupted job because the registry sets it on
	// Start; FinishedAt is set by NewFromLog for rehydrated
	// snapshots).
	if !strings.Contains(html, "Queued") {
		t.Errorf("report page interrupted branch must carry the Timeline section with the Queued label; got HTML:\n%s", html)
	}
	if !strings.Contains(html, "Finished") {
		t.Errorf("report page interrupted branch must carry the Timeline section with the Finished label; got HTML:\n%s", html)
	}
}

// TestJobsStatusPage_DoneCardUnchanged pins the existing Done
// summary card so the slice-2 implementation cannot regress it.
// The headline + size/duration lines + Dismiss CTA are the
// load-bearing surface for issue #131's redesigned status page;
// any collateral damage from adding InterruptedCard would trip
// these assertions before the user does.
func TestJobsStatusPage_DoneCardUnchanged(t *testing.T) {
	job := jobs.NewJob("job-done", "static_archive")
	job.Status = jobs.StatusDone
	job.StartedAt = time.Now().Add(-2 * time.Second)
	job.FinishedAt = time.Now()
	job.Progress = 100

	var buf bytes.Buffer
	if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusView render: %v", err)
	}
	html := buf.String()

	// Done summary card must still render (jobSummaryCard partial
	// is the source of the headline + Dismiss + Show report).
	if !strings.Contains(html, ">Dismiss<") {
		t.Errorf("Done card must keep the Dismiss button (issue #131 regression pin); got HTML:\n%s", html)
	}
	if !strings.Contains(html, ">Show report<") {
		t.Errorf("Done card must keep the Show report button (issue #131 regression pin); got HTML:\n%s", html)
	}
	// Done card must NOT render the new InterruptedCard marker.
	if strings.Contains(html, `data-jobs-interrupted`) {
		t.Errorf("Done card must NOT render the InterruptedCard marker (status is done, not interrupted); got HTML:\n%s", html)
	}
	if strings.Contains(html, "(in progress)") {
		t.Errorf("Done card must NOT carry the broken '(in progress)' classification; got HTML:\n%s", html)
	}
}