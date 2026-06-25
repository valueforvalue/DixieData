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