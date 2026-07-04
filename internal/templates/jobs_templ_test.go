package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// Regression net for issue #316 slice 3 \u2014 polling must stop when a
// background job reaches a terminal state. Both fragments
// (jobs.JobStatusFragment on /jobs/{id} and
// job_slot_fragment.JobStatusSlotFragment for the layout overlay)
// have an identical if/else branch in templ:
//
//   if job.Status == StatusDone || StatusError || StatusCancelled
//      || StatusInterrupted {
//       hx-trigger="none"
//   } else { hx-get=... hx-trigger="every 2s" hx-swap=... hx-target=... }
//
// A JS regex cannot read the templ `if` branch condition because
// templ compiles it to generated Go. The only way to verify the
// branch is to render the fragment for each JobStatus and grep
// the output. This is the load-bearing test for §1.5 of
// docs/COMMON_BUGS.md (the polling-doesnt-stop bug class).
//
// Each test is parameterized across all 5 JobStatus values via
// t.Run subtests. Terminal states (Done/Error/Cancelled/Interrupted)
// must contain hx-trigger="none" and MUST NOT contain
// hx-trigger="every 2s". Running state must contain the polling
// attrs and MUST NOT contain hx-trigger="none".

// TestJobsFragmentStopsPollingOnTerminalState covers the canonical
// polling fragment served from /jobs/{id}/status. JobStatusFragment
// wraps jobStatusBody \u2014 for StatusDone, body renders jobSummaryCard
// (and components.Button), so the test exercises a real render
// graph, not a synthetic stub.
func TestJobsFragmentStopsPollingOnTerminalState(t *testing.T) {
	cases := []struct {
		name              string
		status            string
		wantStopPolling   bool
	}{
		{"StatusDone", jobs.StatusDone, true},
		{"StatusError", jobs.StatusError, true},
		{"StatusCancelled", jobs.StatusCancelled, true},
		{"StatusInterrupted", jobs.StatusInterrupted, true},
		{"StatusRunning", jobs.StatusRunning, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := jobs.Job{ID: "test-job", Kind: "static_archive", Status: tc.status}
			var buf bytes.Buffer
			if err := JobStatusFragment(job).Render(context.Background(), &buf); err != nil {
				t.Fatalf("render JobStatusFragment: %v", err)
			}
			got := buf.String()
			if tc.wantStopPolling {
				if !strings.Contains(got, `hx-trigger="none"`) {
					t.Fatalf("%s: missing hx-trigger=\"none\"\nfull: %s", tc.name, got)
				}
				if strings.Contains(got, `hx-trigger="every 2s"`) {
					t.Fatalf("%s: terminal fragment must not carry polling hx-trigger=\"every 2s\"\nfull: %s", tc.name, got)
				}
			} else {
				if strings.Contains(got, `hx-trigger="none"`) {
					t.Fatalf("%s: running fragment must not carry hx-trigger=\"none\"\nfull: %s", tc.name, got)
				}
				if !strings.Contains(got, `hx-trigger="every 2s"`) {
					t.Fatalf("%s: missing polling hx-trigger=\"every 2s\"\nfull: %s", tc.name, got)
				}
			}
		})
	}
}

// TestJobsSlotFragmentStopsPollingOnTerminalState covers the
// compact layout overlay fragment served from /jobs/{id}/slot.
// This is the popup the user sees at the top of the screen while
// a job runs; without polling-stop behavior the popup would
// re-fetch forever after the job is done. Same if/else branch
// shape as JobStatusFragment; distinct render path (different
// templ function, no jobSummaryCard expansion).
func TestJobsSlotFragmentStopsPollingOnTerminalState(t *testing.T) {
	cases := []struct {
		name              string
		status            string
		wantStopPolling   bool
	}{
		{"StatusDone", jobs.StatusDone, true},
		{"StatusError", jobs.StatusError, true},
		{"StatusCancelled", jobs.StatusCancelled, true},
		{"StatusInterrupted", jobs.StatusInterrupted, true},
		{"StatusRunning", jobs.StatusRunning, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := jobs.Job{ID: "test-job", Kind: "static_archive", Status: tc.status}
			var buf bytes.Buffer
			if err := JobStatusSlotFragment(job).Render(context.Background(), &buf); err != nil {
				t.Fatalf("render JobStatusSlotFragment: %v", err)
			}
			got := buf.String()
			if tc.wantStopPolling {
				if !strings.Contains(got, `hx-trigger="none"`) {
					t.Fatalf("%s: missing hx-trigger=\"none\"\nfull: %s", tc.name, got)
				}
				if strings.Contains(got, `hx-trigger="every 2s"`) {
					t.Fatalf("%s: terminal fragment must not carry polling hx-trigger=\"every 2s\"\nfull: %s", tc.name, got)
				}
			} else {
				if strings.Contains(got, `hx-trigger="none"`) {
					t.Fatalf("%s: running fragment must not carry hx-trigger=\"none\"\nfull: %s", tc.name, got)
				}
				if !strings.Contains(got, `hx-trigger="every 2s"`) {
					t.Fatalf("%s: missing polling hx-trigger=\"every 2s\"\nfull: %s", tc.name, got)
				}
			}
		})
	}
}
