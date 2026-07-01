package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// TestJobStatusViewErrorLabelByKind is the regression net for the
// "Export failed." UI mislabel (issue #217). When a job errors,
// the /jobs/{id} status page used to print "Export failed."
// regardless of the job's Kind. For an import, the user saw
// "Export failed. import failed: …" — confusing because the
// second line is the real error but the first line claims the
// export was the failing operation.
//
// Fix: the templ now calls jobs.FailedVerb(job.Kind, false) for
// the error state and jobs.FailedVerb(job.Kind, true) for the
// cancel state. The helper picks the right verb based on kind:
//   - Imports (backup_import, shared_import, memorial_import,
//     image_import) → "Import failed." / "Import cancelled."
//   - Exports (everything else) → "Export failed." / "Export cancelled."
//   - Unknown kind → "Operation failed." / "Operation cancelled."
//
// This test exercises the templ end-to-end: build a Job, render
// JobStatusView, assert the right verb is in the HTML. It also
// asserts the OLD wrong verb is NOT in the HTML for the
// backup_import case (regression for the user-reported bug).
func TestJobStatusViewErrorLabelByKind(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		cancelled bool
		wantVerb  string
	}{
		{
			name:     "import error",
			kind:     "backup_import",
			wantVerb: "Import failed.",
		},
		{
			name:      "import cancelled",
			kind:      "shared_import",
			cancelled: true,
			wantVerb:  "Import cancelled.",
		},
		{
			name:     "export error",
			kind:     "static_archive",
			wantVerb: "Export failed.",
		},
		{
			name:      "export cancelled",
			kind:      "static_archive",
			cancelled: true,
			wantVerb:  "Export cancelled.",
		},
		{
			name:     "unknown kind error",
			kind:     "future_kind",
			wantVerb: "Operation failed.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := jobs.NewJob("job-test", tt.kind)
			job.StartedAt = time.Now().Add(-2 * time.Second)
			job.FinishedAt = time.Now()
			if tt.cancelled {
				job.Status = jobs.StatusCancelled
			} else {
				job.Status = jobs.StatusError
				job.Error = "synthetic test error"
			}

			var buf bytes.Buffer
			if err := JobStatusView(*job).Render(context.Background(), &buf); err != nil {
				t.Fatalf("JobStatusView render: %v", err)
			}
			html := buf.String()

			if !strings.Contains(html, tt.wantVerb) {
				t.Errorf("HTML must contain %q for kind=%q cancelled=%v; got HTML:\n%s", tt.wantVerb, tt.kind, tt.cancelled, html)
			}
			// Regression: backup_import must NOT show "Export failed.".
			if tt.kind == "backup_import" && strings.Contains(html, "Export failed.") {
				t.Errorf("HTML must NOT contain 'Export failed.' for kind=backup_import; issue #217 — was the bug class user reported; got HTML:\n%s", html)
			}
			// The error text should still appear for errored jobs.
			if !tt.cancelled && tt.kind == "backup_import" && !strings.Contains(html, "synthetic test error") {
				t.Errorf("HTML must contain the error text; got HTML:\n%s", html)
			}
		})
	}
}