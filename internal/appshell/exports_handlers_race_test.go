package appshell

import (
	"context"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

"github.com/valueforvalue/DixieData/internal/jobs"

	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// TestEnqueueExportChannelHandoffLandsResultPath is the
// regression net for issue #419 (closure-capture race on
// jobID in enqueueExport / enqueueExportWithResult). The previous
// code captured the outer-scope `var jobID` by reference inside
// the worker closure, then wrote to it AFTER a.jobs.Start
// returned. Under -race the detector flagged the unsynchronized
// read (inside the worker) vs the write (in the outer code after
// Start returned) on the same string. Even when not under -race,
// a worker that fired before the outer assignment would read
// jobID = "" and call SetResultPath / SetResult on the wrong
// (empty) job, losing the result path or stats.
//
// The fix uses a one-shot buffered channel as the synchronization
// point: the outer code sends the ID into the channel AFTER
// Start returns; the worker reads it before it needs the value.
// The channel send happens-before the channel receive, so the
// worker always observes the assigned ID without a data race.
//
// This test does NOT exercise the race directly (you can't
// deterministically trigger a closure-capture race in a unit
// test), but it pins the observable contract the fix relies on:
// every concurrent call lands its result path / stats on the
// SAME job ID that was advertised to the HTTP caller via
// X-DixieData-Redirect. Under the pre-fix code a worker that
// raced before the outer assignment would call SetResultPath("",
// path) which silently no-ops in the registry (no job with ID
// "") and then the worker would overwrite the job's ResultPath
// to "" once the outer assignment eventually landed. The test
// asserts ResultPath equals the per-call unique path that was
// passed into the worker, which is impossible if the channel
// handoff is broken.
//
// Run with: go test -race -run TestEnqueueExport ./internal/appshell/
func TestEnqueueExportChannelHandoffLandsResultPath(t *testing.T) {
	app := newStressApp(t)

	const concurrency = 16
	var wg sync.WaitGroup
	jobIDs := make([]string, concurrency)
	resultPaths := make([]string, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine picks a unique temp path so we can
			// verify the worker wrote the SAME path to the SAME
			// job ID it advertised.
			path := filepath.Join(testtemp.New(t).Path(), fmt.Sprintf("race-%02d.json", idx))
			rec := httptest.NewRecorder()
			app.enqueueExport("", "json_export", func(ctx context.Context, p *jobs.Progress) error {
				// Worker body: no real export, just return.
				return nil
			}, path, rec)

			dixie := rec.Header().Get("X-DixieData-Redirect")
			if !strings.HasPrefix(dixie, "/jobs/") {
				t.Errorf("goroutine %d: missing X-DixieData-Redirect=/jobs/{id}, got %q", idx, dixie)
				return
			}
			jobIDs[idx] = strings.TrimPrefix(dixie, "/jobs/")
			resultPaths[idx] = path
		}(i)
	}
	wg.Wait()

	// Wait for all workers to settle (max 2s).
	deadline := time.Now().Add(2 * time.Second)
	for {
		allDone := true
		for _, id := range jobIDs {
			if id == "" {
				continue
			}
			s, ok := app.jobs.Get(id)
			if !ok || (s.Status != jobs.StatusDone && s.Status != jobs.StatusError) {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("workers did not finish within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Every advertised job ID must end up with the per-goroutine
	// unique result path. If the channel handoff is broken a
	// worker would call SetResultPath("", path) (no-op) or
	// SetResultPath(some-other-jobID, path), and the assertion
	// below would catch either failure mode.
	for i, id := range jobIDs {
		if id == "" {
			continue
		}
		s, ok := app.jobs.Get(id)
		if !ok {
			t.Errorf("goroutine %d: advertised job %s not found in registry", i, id)
			continue
		}
		if s.ResultPath != resultPaths[i] {
			t.Errorf("goroutine %d: job %s ResultPath = %q, want %q (channel handoff landed the wrong ID in the worker)",
				i, id, s.ResultPath, resultPaths[i])
		}
	}
}

// TestEnqueueExportWithResultChannelHandoffLandsStats mirrors
// the regression net for the stats-aware variant (enqueueExportWithResult).
// Same closure-capture race; same channel-handoff fix; same observable
// contract (every advertised job ID ends up with the per-call unique
// stats the worker returned).
func TestEnqueueExportWithResultChannelHandoffLandsStats(t *testing.T) {
	app := newStressApp(t)

	const concurrency = 16
	var wg sync.WaitGroup
	jobIDs := make([]string, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			app.enqueueExportWithResult("", "json_export", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
				return jobs.JobResult{Records: idx + 1}, nil
			}, "", rec)

			dixie := rec.Header().Get("X-DixieData-Redirect")
			if !strings.HasPrefix(dixie, "/jobs/") {
				t.Errorf("goroutine %d: missing X-DixieData-Redirect=/jobs/{id}, got %q", idx, dixie)
				return
			}
			jobIDs[idx] = strings.TrimPrefix(dixie, "/jobs/")
		}(i)
	}
	wg.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for {
		allDone := true
		for _, id := range jobIDs {
			if id == "" {
				continue
			}
			s, ok := app.jobs.Get(id)
			if !ok || (s.Status != jobs.StatusDone && s.Status != jobs.StatusError) {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("workers did not finish within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	for i, id := range jobIDs {
		if id == "" {
			continue
		}
		s, ok := app.jobs.Get(id)
		if !ok {
			t.Errorf("goroutine %d: advertised job %s not found in registry", i, id)
			continue
		}
		if s.Result.Records != i+1 {
			t.Errorf("goroutine %d: job %s Result.Records = %d, want %d (channel handoff landed the wrong ID in the worker)",
				i, id, s.Result.Records, i+1)
		}
	}
}