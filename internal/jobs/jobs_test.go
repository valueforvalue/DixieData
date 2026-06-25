package jobs

import (
	"context"
	"errors"
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
// registry sized to N workers and asserts the (N+1)th stays queued
// until a slot frees. Each worker blocks on a release channel so the
// test fully controls when workers make progress.
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

	id1 := reg.Start("unit", hold("a"))
	id2 := reg.Start("unit", hold("b"))
	id3 := reg.Start("unit", hold("c"))

	// Wait for the first two workers to start.
	got := map[string]bool{}
	for len(got) < poolSize {
		select {
		case label := <-started:
			got[label] = true
		case <-time.After(time.Second):
			t.Fatalf("only %d of %d workers started: %v", len(got), poolSize, got)
		}
	}

	// The third job must stay queued while the pool is full.
	third, _ := reg.Get(id3)
	if third.Status != StatusQueued {
		t.Fatalf("third job status = %s, want queued", third.Status)
	}

	// Release one worker; the third should start.
	closeOne := func() {
		select {
		case release <- struct{}{}:
		default:
		}
	}
	closeOne()
	select {
	case label := <-started:
		if label != "c" {
			t.Fatalf("third worker started as %q, want c", label)
		}
	case <-time.After(time.Second):
		t.Fatalf("third worker never started after a slot freed")
	}

	// Drain the remaining two.
	closeOne()
	closeOne()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		allDone := true
		for _, id := range []string{id1, id2, id3} {
			snap, _ := reg.Get(id)
			if snap.Status != StatusDone {
				allDone = false
				break
			}
		}
		if allDone {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("workers never reached done after release")
}