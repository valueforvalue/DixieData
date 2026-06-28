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
)

// TestJobStatusViewDdbakArtifactUsesDownload is the regression test
// for issue #129. Before the fix, the "Open Backup" link used
// target="_blank" + Content-Disposition: attachment, which left the
// user on a blank tab after the browser kicked off a silent
// download. The status page now renders a `download="..."` attribute
// instead so the browser saves the file in the current tab and the
// user never sees a blank tab.
func TestJobStatusViewDdbakArtifactUsesDownload(t *testing.T) {
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

	if !strings.Contains(html, `download="june-2026.ddbak"`) {
		t.Errorf("ddbak artifact must use download attribute; got HTML:\n%s", html)
	}
	if strings.Contains(html, `target="_blank"`) {
		t.Errorf("ddbak artifact must NOT use target=_blank (issue #129); got HTML:\n%s", html)
	}
	if !strings.Contains(html, `href="/jobs/job-ddbak/artifact"`) {
		t.Errorf("artifact href must still point at /jobs/{id}/artifact; got HTML:\n%s", html)
	}
}

// TestJobStatusViewPDFArtifactKeepsBlankTarget pins down the inverse:
// PDFs and other viewable artifacts must continue to render in a new
// tab (commit 2f4d587). The status page distinguishes via
// job.IsViewableArtifact() which maps extensions like .pdf / .jpg
// to inline rendering.
func TestJobStatusViewPDFArtifactKeepsBlankTarget(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "june-report.pdf")
	if err := os.WriteFile(resultPath, []byte("%PDF-1.4 placeholder"), 0o644); err != nil {
		t.Fatalf("seed pdf file: %v", err)
	}
	job := jobs.NewJob("job-pdf", "database_pdf")
	job.Status = jobs.StatusDone
	job.StartedAt = time.Now().Add(-2 * time.Second)
	job.FinishedAt = time.Now()
	job.ResultPath = resultPath

	var buf bytes.Buffer
	if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobStatusView render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `target="_blank"`) {
		t.Errorf("PDF artifact must use target=_blank to render inline in a new tab; got HTML:\n%s", html)
	}
	if strings.Contains(html, `download="`) {
		t.Errorf("PDF artifact must NOT use download attribute; got HTML:\n%s", html)
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

// TestJobStatusFragmentDdbakArtifactUsesDownload is the
// polling-fragment counterpart. The fragment is what htmx swaps in
// every 2s while the page polls for terminal state, so it must
// agree with the full view.
func TestJobStatusFragmentDdbakArtifactUsesDownload(t *testing.T) {
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

	if !strings.Contains(html, `download="june-2026.ddbak"`) {
		t.Errorf("ddbak artifact in fragment must use download attribute; got HTML:\n%s", html)
	}
	if strings.Contains(html, `target="_blank"`) {
		t.Errorf("ddbak artifact in fragment must NOT use target=_blank; got HTML:\n%s", html)
	}
}