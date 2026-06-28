package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

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
	job := jobs.NewJob("job-ddbak", "backup_archive")
	job.Status = jobs.StatusDone
	job.ResultPath = "/Users/me/Backups/june-2026.ddbak"

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
	job := jobs.NewJob("job-pdf", "database_pdf")
	job.Status = jobs.StatusDone
	job.ResultPath = "/tmp/june-report.pdf"

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

// TestJobStatusFragmentDdbakArtifactUsesDownload is the
// polling-fragment counterpart. The fragment is what htmx swaps in
// every 2s while the page polls for terminal state, so it must
// agree with the full view.
func TestJobStatusFragmentDdbakArtifactUsesDownload(t *testing.T) {
	job := jobs.NewJob("job-ddbak", "backup_archive")
	job.Status = jobs.StatusDone
	job.ResultPath = "/Users/me/Backups/june-2026.ddbak"

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