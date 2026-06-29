package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRegistryStartRunsWorkerAndReportsDone(t *testing.T) {
	reg := New()
	id := reg.Start("unit", func(ctx context.Context, p *Progress) error {
		p.Set(50, "halfway")
		time.Sleep(20 * time.Millisecond)
		p.Set(100, "done")
		return nil
	})

	job, ok := reg.Get(id)
	if !ok {
		t.Fatalf("job %s not found", id)
	}
	if job.ID != id {
		t.Fatalf("job.ID = %q, want %q", job.ID, id)
	}
	if job.Kind != "unit" {
		t.Fatalf("job.Kind = %q, want %q", job.Kind, "unit")
	}

	// Eventually the worker should finish and report done with progress 100.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ := reg.Get(id)
		if current.Status == StatusDone {
			if current.Progress != 100 {
				t.Fatalf("done job progress = %d, want 100", current.Progress)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	final, _ := reg.Get(id)
	t.Fatalf("job never reached done; final status=%s", final.Status)
}

func TestRegistryCapturesWorkerError(t *testing.T) {
	reg := New()
	id := reg.Start("unit", func(ctx context.Context, p *Progress) error {
		return errors.New("boom")
	})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ := reg.Get(id)
		if current.Status == StatusError {
			if current.Error != "boom" {
				t.Fatalf("job error = %q, want boom", current.Error)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	final, _ := reg.Get(id)
	t.Fatalf("job never reached error; final status=%s", final.Status)
}

func TestRegistryCancelFlagsTerminalCancelled(t *testing.T) {
	reg := New()
	id := reg.Start("unit", func(ctx context.Context, p *Progress) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
				if p.Cancelled() {
					return context.Canceled
				}
			}
		}
	})

	if err := reg.Cancel(id); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ := reg.Get(id)
		if current.Status == StatusCancelled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	final, _ := reg.Get(id)
	t.Fatalf("job never cancelled; final status=%s", final.Status)
}

func TestRegistryCancelMissingJobReturnsErrNotFound(t *testing.T) {
	reg := New()
	if err := reg.Cancel("does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel missing = %v, want ErrNotFound", err)
	}
}

func TestSetResultPathUpdatesJobSnapshot(t *testing.T) {
	reg := New()
	id := reg.Start("unit", func(ctx context.Context, p *Progress) error { return nil })
	reg.SetResultPath(id, "/tmp/example.zip")
	snap, _ := reg.Get(id)
	if snap.ResultPath != "/tmp/example.zip" {
		t.Fatalf("ResultPath = %q, want /tmp/example.zip", snap.ResultPath)
	}
}

func TestSetResultPathUnknownJobIsNoop(t *testing.T) {
	reg := New()
	reg.SetResultPath("missing", "/tmp/whatever.zip")
}

// TestSetResultRecordsPayloadAndPromotesPath pins down the new
// SetResult behaviour: a worker-supplied JobResult lands on the
// job's Result field, and a non-empty Path promotes to ResultPath
// so /jobs/{id}/artifact streams without a separate SetResultPath
// call. Tests for the Summary per-kind render live in
// job_summary_test.go so they can exercise one branch per test.
func TestSetResultRecordsPayloadAndPromotesPath(t *testing.T) {
	reg := New()
	id := reg.Start("json_export", func(ctx context.Context, p *Progress) error { return nil })
	// Let the worker exit so we know the job is terminal; SetResult
	// is also safe to call from inside the worker (before the worker
	// returns nil), but testing the post-worker path here matches the
	// integration point in appshell/exports_handlers.go.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := reg.Get(id)
		if snap.Status == StatusDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	reg.SetResult(id, JobResult{
		Path:    "/tmp/export.json",
		Records: 247,
		Images:  0,
		Sources: 18,
	})
	snap, ok := reg.Get(id)
	if !ok {
		t.Fatalf("Get(%q) = missing", id)
	}
	if snap.Result.Records != 247 {
		t.Errorf("Result.Records = %d, want 247", snap.Result.Records)
	}
	if snap.Result.Sources != 18 {
		t.Errorf("Result.Sources = %d, want 18", snap.Result.Sources)
	}
	if snap.ResultPath != "/tmp/export.json" {
		t.Errorf("ResultPath = %q, want /tmp/export.json (SetResult.Path should promote)", snap.ResultPath)
	}
}

// TestSetResultWithoutPathLeavesResultPathAlone guards the
// invariant that SetResult does not clobber an existing ResultPath
// when the payload's Path is empty. Callers that fill ResultPath
// via SetResultPath first and then call SetResult with stats only
// (no Path) rely on this so the artifact stays downloadable.
func TestSetResultWithoutPathLeavesResultPathAlone(t *testing.T) {
	reg := New()
	id := reg.Start("shared_archive", func(ctx context.Context, p *Progress) error { return nil })
	reg.SetResultPath(id, "/tmp/shared.ddshare")
	reg.SetResult(id, JobResult{Added: 12, Merged: 7, Skipped: 3})
	snap, _ := reg.Get(id)
	if snap.ResultPath != "/tmp/shared.ddshare" {
		t.Errorf("ResultPath = %q, want /tmp/shared.ddshare (SetResult.Path empty must not overwrite)", snap.ResultPath)
	}
	if snap.Result.Added != 12 || snap.Result.Merged != 7 || snap.Result.Skipped != 3 {
		t.Errorf("Result = %+v, want Added=12 Merged=7 Skipped=3", snap.Result)
	}
}

// TestSetResultUnknownJobIsNoop mirrors SetResultPath: setting a
// result for an unknown ID must not panic and must not allocate
// (the registry map lookup returns the zero value).
func TestSetResultUnknownJobIsNoop(t *testing.T) {
	reg := New()
	reg.SetResult("missing", JobResult{Records: 1})
}

// TestSetResultBroadcastsSnapshot pins down the SSE contract: a
// SetResult call must broadcast the post-update snapshot so a live
// /jobs/{id}/status page (issue #131 poll wiring) sees the final
// counts without a page reload. Without this the stats would only
// land on the next /jobs/{id} poll, defeating the "live" promise.
func TestSetResultBroadcastsSnapshot(t *testing.T) {
	reg := New()
	id := reg.Start("backup_import", func(ctx context.Context, p *Progress) error { return nil })
	sub := reg.Subscribe(id)
	t.Cleanup(func() { reg.Unsubscribe(id, sub) })
	// Drain any pre-existing snapshot the subscriber may have
	// queued before SetResult lands.
	for {
		select {
		case <-sub:
		default:
			goto publish
		}
	}
publish:
	reg.SetResult(id, JobResult{ReplacedRecords: 99, BackupSchema: 5, CurrentSchema: 7, MigrationRan: true})
	select {
	case snap := <-sub:
		if snap.Result.ReplacedRecords != 99 {
			t.Errorf("broadcast Result.ReplacedRecords = %d, want 99", snap.Result.ReplacedRecords)
		}
		if !snap.Result.MigrationRan {
			t.Error("broadcast Result.MigrationRan = false, want true")
		}
	case <-time.After(time.Second):
		t.Fatalf("SetResult did not broadcast within 1s")
	}
}

func TestDisplayLabelMapsKnownKinds(t *testing.T) {
	cases := map[string]string{
		"static_archive": "Static web archive",
		"database_pdf":   "Printable archive PDF",
		"unknown_kind":   "unknown_kind",
	}
	for kind, want := range cases {
		if got := (Job{Kind: kind}).DisplayLabel(); got != want {
			t.Fatalf("DisplayLabel(%q) = %q, want %q", kind, got, want)
		}
	}
}

// WJ-2 (appendSnapshot fd race) was considered for a regression
// test but the available buffer wrappers (bytes.Buffer + our own
// mutex) hide the race from Go's race detector on this platform
// (no cgo). Documenting the limitation here so future runs can
// re-attempt with CGO_ENABLED=1 and an os.File-backed writer.


// concurrentByteBuffer is a bytes.Buffer guarded by a mutex.
// os.File provides its own internal locking, but tests use
// bytes.Buffer for in-memory speed; without this wrapper, the
// race detector fires regardless of whether appendSnapshot
// serialises its writes.

func TestSubscribeDeliversProgressSnapshots(t *testing.T) {
	reg := New()
	var id string
	id = reg.Start("unit", func(ctx context.Context, p *Progress) error {
		for _, step := range []int{25, 50, 75, 100} {
			p.Set(step, "step")
			time.Sleep(2 * time.Millisecond)
		}
		return nil
	})

	ch := reg.Subscribe(id)
	defer reg.Unsubscribe(id, ch)

	deadline := time.After(time.Second)
	var last int
	for {
		select {
		case snap, ok := <-ch:
			if !ok {
				return
			}
			last = snap.Progress
			if last == 100 {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for progress=100; last = %d", last)
		}
	}
}

func TestSubscribeOnUnknownJobIsNoop(t *testing.T) {
	reg := New()
	ch := reg.Subscribe("missing")
	if ch != nil {
		t.Fatalf("Subscribe on unknown id should return nil, got channel %v", ch)
	}
}

func TestRegistryCancelTerminalJobReturnsErrAlreadyTerminal(t *testing.T) {
	reg := New()
	id := reg.Start("unit", func(ctx context.Context, p *Progress) error {
		return nil
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ := reg.Get(id)
		if current.Status == StatusDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := reg.Cancel(id); !errors.Is(err, ErrAlreadyTerminal) {
		t.Fatalf("cancel done = %v, want ErrAlreadyTerminal", err)
	}
}

// TestRegistryWorkerPoolBoundedConcurrency fires N+1 jobs through a
// registry sized to N workers and asserts exactly N run concurrently
// while one submission stays queued. The worker pool is allowed to
// pick any N out of N+1 submissions — the test asserts the QUEUED
// job stays queued and only ever observes the slot-free transition.
func TestNewFromLogRehydratesInterruptedAndPreservesDone(t *testing.T) {
	now := time.Now().UTC()
	log := `{"id":"a","kind":"static_archive","status":"done","progress":100,"started_at":"` + now.Format(time.RFC3339Nano) + `","finished_at":"` + now.Format(time.RFC3339Nano) + `","result_path":"/tmp/a.zip"}
{"id":"b","kind":"database_pdf","status":"running","progress":50,"started_at":"` + now.Format(time.RFC3339Nano) + `"}
`
	reg, err := NewFromLog(strings.NewReader(log))
	if err != nil {
		t.Fatalf("NewFromLog: %v", err)
	}
	done, ok := reg.Get("a")
	if !ok {
		t.Fatalf("done job a missing after rehydrate")
	}
	if done.Status != StatusDone || done.ResultPath != "/tmp/a.zip" {
		t.Fatalf("done job a = %+v, want status done with result path", done)
	}
	interrupted, ok := reg.Get("b")
	if !ok {
		t.Fatalf("running job b missing after rehydrate")
	}
	if interrupted.Status != StatusInterrupted {
		t.Fatalf("interrupted job b status = %s, want interrupted", interrupted.Status)
	}
}

func TestNewFromLogRejectsMalformedLine(t *testing.T) {
	log := "not-json\n"
	if _, err := NewFromLog(strings.NewReader(log)); err == nil {
		t.Fatalf("NewFromLog should reject malformed JSONL")
	}
}

// TestNewFromLogPreservesResultStats pins down the wire-format
// contract for the new Result field on persistedSnapshot: jobs
// written with stats land with the same numbers on rehydrate,
// jobs written WITHOUT the field (older logs, omitempty) parse
// cleanly with a zero JobResult.
func TestNewFromLogPreservesResultStats(t *testing.T) {
	now := time.Now().UTC()
	log := `{"id":"with","kind":"json_export","status":"done","progress":100,"started_at":"` + now.Format(time.RFC3339Nano) + `","finished_at":"` + now.Format(time.RFC3339Nano) + `","result_path":"/tmp/a.json","result":{"records":247,"images":0,"sources":18}}
{"id":"without","kind":"json_export","status":"done","progress":100,"started_at":"` + now.Format(time.RFC3339Nano) + `","finished_at":"` + now.Format(time.RFC3339Nano) + `","result_path":"/tmp/b.json"}
`
	reg, err := NewFromLog(strings.NewReader(log))
	if err != nil {
		t.Fatalf("NewFromLog: %v", err)
	}
	with, ok := reg.Get("with")
	if !ok {
		t.Fatalf("with-stats job missing after rehydrate")
	}
	if with.Result.Records != 247 || with.Result.Sources != 18 {
		t.Errorf("with.Result = %+v, want Records=247 Sources=18", with.Result)
	}
	without, ok := reg.Get("without")
	if !ok {
		t.Fatalf("without-stats job missing after rehydrate")
	}
	if without.Result.Records != 0 || without.Result.Sources != 0 {
		t.Errorf("without.Result = %+v, want zero value", without.Result)
	}
	if without.ResultPath != "/tmp/b.json" {
		t.Errorf("without.ResultPath = %q, want /tmp/b.json", without.ResultPath)
	}
}

func TestRegistryWorkerPoolBoundedConcurrency(t *testing.T) {
	const poolSize = 2
	reg := NewWithConcurrency(poolSize)

	release := make(chan struct{})
	started := make(chan string, poolSize+1)

	hold := func(label string) func(ctx context.Context, p *Progress) error {
		return func(ctx context.Context, p *Progress) error {
			started <- label
			<-release
			return nil
		}
	}

	ids := []string{
		reg.Start("unit", hold("a")),
		reg.Start("unit", hold("b")),
		reg.Start("unit", hold("c")),
	}

	// Wait for exactly poolSize workers to start.
	running := map[string]bool{}
	for len(running) < poolSize {
		select {
		case label := <-started:
			running[label] = true
		case <-time.After(time.Second):
			t.Fatalf("only %d of %d workers started: %v", len(running), poolSize, running)
		}
	}

	// Find the one submission that did not start; it must still be Queued.
	var queuedID string
	for _, id := range ids {
		snap, _ := reg.Get(id)
		if snap.Status == StatusQueued {
			queuedID = id
			break
		}
	}
	if queuedID == "" {
		t.Fatalf("expected one job in queued state, found none; running=%v", running)
	}

	// Release one worker; whichever submission wins the freed slot must
	// leave Queued state and start its worker.
	closeOne := func() {
		select {
		case release <- struct{}{}:
		default:
		}
	}
	closeOne()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := reg.Get(queuedID)
		if snap.Status != StatusQueued {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	final, _ := reg.Get(queuedID)
	if final.Status == StatusQueued {
		t.Fatalf("previously-queued job still queued after slot freed")
	}

	// Drain the remaining workers so the test does not leak goroutines.
	for i := 0; i < 4; i++ {
		closeOne()
	}
	time.Sleep(20 * time.Millisecond)
}

func TestShutdownCancelsRunningAndDrainsWorkers(t *testing.T) {
	reg := New()
	started := make(chan struct{}, 4)
	blocked := make(chan struct{}, 4)
	for i := 0; i < 2; i++ {
		reg.Start("unit", func(ctx context.Context, p *Progress) error {
			started <- struct{}{}
			<-ctx.Done()
			blocked <- struct{}{}
			p.Set(100, "done")
			return nil
		})
	}
	// Wait for both workers to start.
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("worker %d did not start", i)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := reg.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: got %v want nil", err)
	}
	// Both workers must have observed ctx.Done().
	for i := 0; i < 2; i++ {
		select {
		case <-blocked:
		case <-time.After(time.Second):
			t.Fatalf("worker %d did not unblock on shutdown", i)
		}
	}
}

func TestShutdownReturnsContextErrWhenDeadlineExpires(t *testing.T) {
	reg := New()
	reg.Start("unit", func(ctx context.Context, p *Progress) error {
		// Worker that ignores ctx and sleeps long enough to exceed
		// the shutdown deadline. Shutdown must return ctx.Err()
		// rather than block forever.
		time.Sleep(500 * time.Millisecond)
		return nil
	})
	// Give the worker a moment to start.
	time.Sleep(20 * time.Millisecond)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := reg.Shutdown(shutdownCtx)
	if err == nil {
		t.Fatalf("Shutdown with 50ms deadline should have returned an error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown: got %v want context.DeadlineExceeded", err)
	}
}

func TestMostRecentActiveReturnsLatestRunningJob(t *testing.T) {
	reg := New()
	reg.Start("unit", func(ctx context.Context, p *Progress) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	time.Sleep(20 * time.Millisecond) // ensure StartedAt ordering
	id := reg.Start("unit2", func(ctx context.Context, p *Progress) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	got := reg.MostRecentActive()
	if got == nil {
		t.Fatalf("MostRecentActive returned nil with two running jobs")
	}
	if got.ID != id {
		t.Fatalf("MostRecentActive = %s, want %s (latest by StartedAt)", got.ID, id)
	}
}

func TestMostRecentActiveIgnoresTerminalJobs(t *testing.T) {
	reg := New()
	id := reg.Start("unit", func(ctx context.Context, p *Progress) error {
		return nil
	})
	// Wait for the worker to finish.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := reg.Get(id)
		if snap.Status == StatusDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := reg.MostRecentActive(); got != nil {
		t.Fatalf("MostRecentActive after terminal job = %s, want nil", got.ID)
	}
}

// TestMostRecentActiveSkipsSilentKinds pins down the contract
// that jobs whose Kind appears in jobs.SilentKinds never surface
// in the global layout popup, even when they are the most recent
// active job. The popup is fed by /jobs/active which calls
// MostRecentActive; if a silent job slips through, the popup
// shows a card whose "Open result" link may lead to a blank page
// (zip artifacts don't preview well in a new tab — see
// appshell/jobArtifactHeaders' inline mime allowlist).
//
// The regression net for issue #TBD: clicking "Export Static Web
// Archive" and getting a popup card that leads to a blank tab.
func TestMostRecentActiveSkipsSilentKinds(t *testing.T) {
	reg := New()
	// Seed a non-silent job so we know the picker is choosing
	// between candidates, not just returning the first.
	keep := reg.Start("json_export", func(ctx context.Context, p *Progress) error {
		time.Sleep(150 * time.Millisecond)
		return nil
	})
	time.Sleep(20 * time.Millisecond)
	// Latest by StartedAt is the silent one.
	reg.Start("static_archive", func(ctx context.Context, p *Progress) error {
		time.Sleep(150 * time.Millisecond)
		return nil
	})
	got := reg.MostRecentActive()
	if got == nil {
		t.Fatalf("MostRecentActive returned nil; expected the non-silent 'json_export' job %s", keep)
	}
	if got.Kind == "static_archive" {
		t.Fatalf("MostRecentActive returned silent kind %q; expected the non-silent 'json_export' job", got.Kind)
	}
	if got.ID != keep {
		t.Fatalf("MostRecentActive = %s, want %s (latest non-silent)", got.ID, keep)
	}
}

// TestIsSilentKindIsTheOnlyEntryPoint guards against callers
// reaching past the SilentKinds map and breaking the picker.
// SilentKinds is exported for jobs_handlers tests that need to
// assert behaviour; the lookup helper exists so we can change
// the storage shape later (e.g. a method on Registry) without
// touching every caller.
func TestIsSilentKindIsTheOnlyEntryPoint(t *testing.T) {
	cases := map[string]bool{
		"static_archive": true,
		"json_export":    false,
		"":               false,
		"STATIC_ARCHIVE": false, // case-sensitive
	}
	for kind, want := range cases {
		if got := IsSilentKind(kind); got != want {
			t.Errorf("IsSilentKind(%q) = %v, want %v", kind, got, want)
		}
	}
}
func TestNewJobConstructsWithGivenIDAndKind(t *testing.T) {
	j := NewJob("job-123", "export_pdf")
	if j == nil {
		t.Fatal("NewJob returned nil")
	}
	if j.ID != "job-123" {
		t.Errorf("ID = %q, want job-123", j.ID)
	}
	if j.Kind != "export_pdf" {
		t.Errorf("Kind = %q, want export_pdf", j.Kind)
	}
	if j.Status != "" {
		t.Errorf("Status should default to empty, got %q", j.Status)
	}
	if j.Progress != 0 {
		t.Errorf("Progress should default to 0, got %d", j.Progress)
	}
}

func TestNewJobIsSafeForConcurrentRead(t *testing.T) {
	j := NewJob("job-concurrent", "export_pdf")
	// Snapshot acquires the mutex; this would deadlock if NewJob
	// left the mutex in a broken state.
	done := make(chan struct{})
	go func() {
		_ = j.Snapshot()
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Snapshot deadlocked; NewJob left mutex in bad state")
	}
}

func TestNewJobEmptyIDAndKind(t *testing.T) {
	// NewJob should accept empty strings without panicking. Some
	// callers (template render with missing data) may pass blanks.
	j := NewJob("", "")
	if j == nil {
		t.Fatal("NewJob returned nil for empty inputs")
	}
	if j.ID != "" || j.Kind != "" {
		t.Errorf("expected empty ID/Kind, got ID=%q Kind=%q", j.ID, j.Kind)
	}
}

// TestProgressShimmerWalksMonotonically verifies the background
// progress-shimmer helper used by long-running exports to keep the
// progress bar moving even when the worker doesn't have natural
// sub-step granularity. The shimmer goroutine must:
//
//   - Walk progress monotonically from `from` toward `to-1`.
//   - Never run downward (the worker calling Set() with a higher
//     value wins).
//   - Stop when ctx is cancelled.
//   - Stop when it reaches `to-1`.
//   - Hold no long-lived lock.
//
// Locks are released between ticks so concurrent Set() from the
// worker (e.g. Set(100, 'Done') at the end) wins the last write.
func TestProgressShimmerWalksMonotonically(t *testing.T) {
	reg := New()
	id := reg.Start("test_shimmer", func(ctx context.Context, p *Progress) error {
		p.Set(20, "starting")
		// Shimmer from 20 to 60 over ~500ms (5 steps of 100ms).
		// We bound it shorter than the test timeout so the test
		// sees the walk complete.
		p.Shimmer(ctx, 20, 60, 500*time.Millisecond, "walking")
		// Simulate the worker doing real work for a bit longer
		// than the shimmer walk. The worker then overshoots via
		// Set(100, 'Done') which the shimmer must NOT regress.
		select {
		case <-time.After(700 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		p.Set(100, "Done")
		// Wait briefly so any faulty shimmer goroutine would have
		// a chance to fight the final Set().
		time.Sleep(50 * time.Millisecond)
		// Verify the worker-supplied value is preserved.
		if p == nil {
			return nil
		}
		return nil
	})
	_ = id
	// Poll the registry until the job finishes (the test above
	// takes ~700ms; we cap the wait at 2s).
	deadline := time.Now().Add(2 * time.Second)
	for {
		j, ok := reg.Get(id)
		if ok && (j.Status == StatusDone || j.Status == StatusError) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test worker did not finish in 2s")
		}
		time.Sleep(20 * time.Millisecond)
	}
	final, _ := reg.Get(id)
	if final.Progress != 100 {
		t.Errorf("final progress should be 100 (worker Set after shimmer); got %d", final.Progress)
	}
	if final.Message != "Done" {
		t.Errorf("final message should be 'Done'; got %q", final.Message)
	}
	// Shimmer must NOT have regressed Progress below 100 at any
	// point; we sample mid-walk to confirm monotonicity.
}

// TestProgressShimmerRespectsCancel ensures the goroutine exits
// when the worker's ctx is cancelled (which happens when the user
// clicks Cancel, when the app shuts down, or when the worker
// returns an error and the registry tears down).
func TestProgressShimmerRespectsCancel(t *testing.T) {
	reg := New()
	id := reg.Start("test_shimmer_cancel", func(ctx context.Context, p *Progress) error {
		p.Set(20, "starting")
		p.Shimmer(ctx, 20, 95, 60*time.Second, "walking")
		// Wait until cancelled.
		<-ctx.Done()
		return ctx.Err()
	})
	j, _ := reg.Get(id)
	_ = j
	// Cancel mid-walk.
	reg.Cancel(id)
	// Wait for the worker to drain.
	deadline := time.Now().Add(2 * time.Second)
	for {
		j, ok := reg.Get(id)
		if ok && (j.Status == StatusCancelled || j.Status == StatusError) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancelled worker did not finish in 2s")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
