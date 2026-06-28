// job_slot_swap_test.go — guards the persistent progress slot
// against the "stuck at 5%" regression. PR #0 of the stabilization
// sprint introduced the persistent progress region
// (`<div class="progress-region" data-progress-region>` in the
// layout). The slot fragment (job_slot_fragment.templ) targets it
// with `hx-target="[data-progress-region]"` so subsequent polls
// swap back into the layout slot.
//
// If the fragment's `hx-swap` is `outerHTML`, the first poll
// replaces the layout's progress slot div with a new fragment
// div that lacks the `data-progress-region` attribute. Every poll
// thereafter hits `htmx:targetError, [data-progress-region]` and
// the visible progress freezes at the value captured in the first
// poll. This test guards the swap strategy by parsing the rendered
// fragment HTML and asserting innerHTML is in use.
package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// TestJobStatusSlotFragmentUsesInnerHTMLSwap asserts that the
// JobStatusSlotFragment uses hx-swap="innerHTML" against the
// layout's persistent progress region. If a future commit
// regresses this to outerHTML, the "stuck at 5%" symptom returns:
// the layout's data-progress-region div gets replaced with the
// fragment's <div class="card"> on the first poll, and every
// subsequent poll fails to find the target.
func TestJobStatusSlotFragmentUsesInnerHTMLSwap(t *testing.T) {
	job := jobs.NewJob("job-abc", "static_archive")
	job.Status = jobs.StatusRunning
	job.Progress = 42
	job.Message = "Gathering images"

	var buf strings.Builder
	ctx := context.Background()
	if err := JobStatusSlotFragment(*job).Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `hx-target="[data-progress-region]"`) {
		t.Errorf("fragment must target the persistent progress region [data-progress-region]; got:\n%s", html)
	}
	if !strings.Contains(html, `hx-swap="innerHTML"`) {
		t.Errorf("fragment must use hx-swap=innerHTML against the layout's persistent progress region; outerHTML strips the target's data-progress-region attribute on the first poll, causing htmx:targetError and stuck-at-5-percent regressions. Rendered HTML:\n%s", html)
	}
	if strings.Contains(html, `hx-swap="outerHTML"`) {
		t.Errorf("fragment must NOT use hx-swap=\"outerHTML\" against [data-progress-region]; see TestJobStatusSlotFragmentUsesInnerHTMLSwap doc comment. Rendered HTML:\n%s", html)
	}
}

// TestJobStatusSlotFragmentTerminalStateStopsPolling asserts that
// when the job reaches a terminal state (done / error / cancelled /
// interrupted), the fragment emits hx-trigger="none" so htmx
// stops polling. Without this, the slot fragment would keep
// hitting /jobs/{id}/slot for completed jobs and the persistent
// progress bar would never go away.
func TestJobStatusSlotFragmentTerminalStateStopsPolling(t *testing.T) {
	for _, status := range []string{
		jobs.StatusDone,
		jobs.StatusError,
		jobs.StatusCancelled,
		jobs.StatusInterrupted,
	} {
		t.Run(string(status), func(t *testing.T) {
			job := jobs.NewJob("job-abc", "static_archive")
			job.Status = status
			job.Progress = 100

			var buf strings.Builder
			if err := JobStatusSlotFragment(*job).Render(context.Background(), &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			html := buf.String()

			if !strings.Contains(html, `hx-trigger="none"`) {
				t.Errorf("terminal status %q must emit hx-trigger=\"none\" to stop polling; rendered HTML:\n%s", status, html)
			}
			if strings.Contains(html, `hx-get=`) {
				t.Errorf("terminal status %q must not include an hx-get (no further polls needed); rendered HTML:\n%s", status, html)
			}
		})
	}
}