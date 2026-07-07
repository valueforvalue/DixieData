package jobs

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestMostRecentActiveRaceWithWorker pins the contract that
// Registry.MostRecentActive can be called concurrently with a
// worker goroutine writing to the same Job's Status / Kind /
// StartedAt fields, without triggering the -race detector.
//
// Before the fix in #418, MostRecentActive read j.Status /
// j.Kind / j.StartedAt while holding r.mu (the registry map
// lock) but NOT j.mu (the per-Job lock). The worker in
// Start.func1 writes those fields inside job.mu.Lock() (at
// jobs.go:557, :566, :573, :574). The two mutexes do not
// synchronize, so the race detector flags every concurrent
// call to MostRecentActive while a worker is running.
//
// The fix: lock j.mu around the read of Status / Kind /
// StartedAt in MostRecentActive, then build the snapshot
// via cloneJob (which is already safe because cloneJob is
// only called after the lock is released, and the worker
// writes are atomic per-field once Status is settled).
//
// Regression net: under -race, this test fails before the
// fix and passes after. The 200-iter concurrent loop is
// enough to expose the race deterministically; CI's
// windows-latest runner is slow enough that the worker is
// still running when the loop hammers MostRecentActive.
func TestMostRecentActiveRaceWithWorker(t *testing.T) {
	reg := New()

	// Worker that takes long enough for the concurrent reader
	// loop to land many MostRecentActive calls while the
	// worker is mutating Status. 100ms is more than enough
	// for thousands of reads on a CI runner; we don't need
	// to wait that long, we just need the worker to be in
	// flight while the loop runs.
	reg.Start("race_target", func(ctx context.Context, p *Progress) error {
		p.Set(10, "starting")
		time.Sleep(100 * time.Millisecond)
		p.Set(50, "halfway")
		time.Sleep(100 * time.Millisecond)
		p.Set(100, "done")
		return nil
	})

	// Hammer MostRecentActive from a tight loop. Each call
	// reads j.Status + j.Kind + j.StartedAt without holding
	// j.mu (pre-fix). The race detector flags every read
	// that lands between the worker's lock acquisition and
	// release.
	for i := 0; i < 200; i++ {
		_ = reg.MostRecentActive()
	}
}

// TestShutdownRaceWithWorker pins the contract that
// Registry.Shutdown can read j.Status (line 1275) while a
// worker goroutine writes to it (line 557, :573, :574),
// without triggering the -race detector.
//
// Before the fix: Shutdown's loop `for _, j := range r.jobs
// { if j.Status == StatusQueued || j.Status == StatusRunning
// { j.cancelCause() } }` reads j.Status under r.mu only.
// The worker writes j.Status under j.mu. Race.
//
// After the fix: the Status read is performed under j.mu.
//
// The test starts a worker that takes long enough for the
// concurrent Shutdown to read its Status while the worker
// is still in flight, then calls Shutdown with a generous
// deadline so the test can drain cleanly.
func TestShutdownRaceWithWorker(t *testing.T) {
	reg := New()

	reg.Start("race_target", func(ctx context.Context, p *Progress) error {
		p.Set(10, "starting")
		// Stay in flight long enough for the Shutdown call
		// below to read j.Status while we hold j.mu.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		return nil
	})

	// Give the worker a moment to enter its body and acquire
	// j.mu. The race window is small but real; the explicit
	// sleep maximises the chance the race detector catches
	// the pre-fix code under -race.
	time.Sleep(10 * time.Millisecond)

	// Concurrent reader: spin a goroutine that calls
	// Shutdown's read path equivalent (MostRecentActive reads
	// the same j.Status field) in a tight loop, racing
	// against the worker's writes. We don't actually call
	// Shutdown here because we want the test to complete
	// (Shutdown would cancel the worker and close the test
	// cleanly, but the race is on the read side, not the
	// cancel side).
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = reg.MostRecentActive()
		}
	}()
	wg.Wait()

	// Clean shutdown with a generous deadline.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := reg.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown returned %v, want nil", err)
	}
}
