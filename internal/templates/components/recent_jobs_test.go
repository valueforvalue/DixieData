package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestRecentJobsRendersEmptyState asserts the section stays
// quiet when the user has no recent jobs (new install). The
// empty-state copy is part of the section's contract.
func TestRecentJobsRendersEmptyState(t *testing.T) {
	var buf bytes.Buffer
	if err := RecentJobs(nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No exports or imports yet") {
		t.Errorf("expected empty-state copy; got:\n%s", got)
	}
	if strings.Contains(got, `<ul class="divide-y`) {
		t.Errorf("expected no <ul> when entries is empty; got:\n%s", got)
	}
}

// TestRecentJobsRendersRows locks the row shape: each row
// is an <a> linking to /jobs/{id} with the kind label, the
// status pill, and the finished-at timestamp.
func TestRecentJobsRendersRows(t *testing.T) {
	var buf bytes.Buffer
	entries := []viewmodel.RecentJobEntry{
		{
			ID:          "abc123",
			Kind:        "json_export",
			KindLabel:   "Export JSON",
			Status:      "done",
			StatusLabel: "Done",
			Message:     "1,234 records",
			ResultPath:  "/path/to/file.json",
			StartedAt:   "2026-07-02T19:00:00Z",
			FinishedAt:  "2026-07-02T19:00:30Z",
			DetailURL:   "/jobs/abc123",
		},
		{
			ID:          "def456",
			Kind:        "ddbak_import",
			KindLabel:   "Load Backup",
			Status:      "error",
			StatusLabel: "Error",
			Message:     "file is not a valid backup archive",
			StartedAt:   "2026-07-02T19:01:00Z",
			FinishedAt:  "2026-07-02T19:01:02Z",
			DetailURL:   "/jobs/def456",
		},
	}
	if err := RecentJobs(entries).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `<a href="/jobs/abc123"`) {
		t.Errorf("expected row 1 to link to /jobs/abc123; got:\n%s", got)
	}
	if !strings.Contains(got, `<a href="/jobs/def456"`) {
		t.Errorf("expected row 2 to link to /jobs/def456; got:\n%s", got)
	}
	if !strings.Contains(got, "Export JSON") {
		t.Errorf("expected row 1 kind label; got:\n%s", got)
	}
	if !strings.Contains(got, "Load Backup") {
		t.Errorf("expected row 2 kind label; got:\n%s", got)
	}
	if !strings.Contains(got, "Done") {
		t.Errorf("expected Done pill; got:\n%s", got)
	}
	if !strings.Contains(got, "Error") {
		t.Errorf("expected Error pill; got:\n%s", got)
	}
	// Pill colour classes per status (issue #265 — green for
	// done, red for error, neutral for cancelled, amber for
	// interrupted / unknown). Post-#477 strategy-A the static-hex
	// text-[#29522d] / text-[#6f2c26] classes became
	// text-[var(--theme-...)] equivalents; these needles pin the
	// post-#477 token references so the rendered HTML stays in sync
	// with the theme system.
	if !strings.Contains(got, `text-[#29522d]`) {
		t.Errorf("expected green done pill; got:\n%s", got)
	}
	if !strings.Contains(got, `var(--theme-review-red)`) {
		t.Errorf("expected red error pill; got:\n%s", got)
	}
}