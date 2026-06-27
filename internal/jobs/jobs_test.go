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