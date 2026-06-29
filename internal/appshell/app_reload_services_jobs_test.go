// Regression net for the ddbak-import job-id 404 bug.
//
// Symptom: handleImportBackup kicks off a background job, redirects the
// user to /jobs/{id}, and the page polls /jobs/{id}/status every 2s.
// After ImportWithLocalIdentity finishes the worker calls
// a.reopenDatabase() to swap in the freshly restored SQLite file.
// reopenDatabase runs reloadServices(), which used to replace a.jobs
// with a fresh empty Registry. The poll handler then called
// a.jobs.Get(id) on the new registry and returned 404, even though the
// import had succeeded.
//
// Fix: reloadServices preserves a.jobs when one is already wired. Only
// the very first call (NewApp() leaves a.jobs nil) allocates a fresh
// Registry, so tests that bypass startup still get a working registry.
//
// This test pins both code paths:
//   - The first reloadServices call on a fresh App allocates a jobs
//     registry (covers the test-bypass-startup path).
//   - The second reloadServices call (the one reopenDatabase makes
//     during .ddbak restore) preserves the existing registry AND its
//     in-flight job. After reload, Get(inFlightJobID) must return ok.

package appshell

import (
	"context"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/jobs"
)

// configureTestIdentity is defined in app_test.go (takes *App). Do NOT
// shadow it here — call it directly from each test.

// TestReloadServicesPreservesInFlightJobs pins the ddbak-import fix.
// Without the fix, a second reloadServices() call would replace a.jobs
// with a fresh empty Registry, dropping the in-flight job from the
// /jobs/{id} poll handler's reach. With the fix, the in-flight job
// survives the reload and the poll handler can still Get() it.
func TestReloadServicesPreservesInFlightJobs(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(dir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("first reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	if app.jobs == nil {
		t.Fatal("first reloadServices must allocate a.jobs when nil")
	}

	// Start a long-running job so the registry has a known ID we
	// can verify survives the reload. The job blocks on a channel
	// until the test releases it, simulating an import in progress.
	hold := make(chan struct{})
	jobID := app.jobs.Start("backup_import", func(_ context.Context, _ *jobs.Progress) error {
		<-hold
		return nil
	})
	if _, ok := app.jobs.Get(jobID); !ok {
		t.Fatalf("pre-reload: job %s missing from registry", jobID)
	}

	// Mimic reopenDatabase: a second reloadServices(). This is the
	// call that used to drop the in-flight job from the registry.
	if err := app.reloadServices(); err != nil {
		t.Fatalf("second reloadServices: %v", err)
	}

	// The fix's contract: a.jobs must NOT be replaced when one is
	// already wired. Pin it via pointer identity so a future
	// regression that swaps a.jobs for a new Registry trips here.
	original := app.jobs
	if err := app.reloadServices(); err != nil {
		t.Fatalf("third reloadServices: %v", err)
	}
	if app.jobs != original {
		t.Fatal("reloadServices replaced a.jobs; the ddbak-import fix requires it to preserve the existing Registry")
	}

	// The poll handler's contract: Get(jobID) must still return the
	// in-flight job after multiple reloads. Without the fix this
	// returns !ok because the second reload's fresh Registry never
	// saw the Start() call.
	_, ok := app.jobs.Get(jobID)
	if !ok {
		t.Fatalf("post-reload: job %s missing from registry — /jobs/{id}/status would return 404", jobID)
	}

	// Cleanly release the worker goroutine.
	close(hold)
}

// TestReloadServicesAllocatesFreshRegistryWhenNil covers the test path
// that bypasses startup. NewApp() leaves a.jobs nil; the first
// reloadServices() must allocate a working empty Registry so handler
// tests can exercise the /jobs/{id} routes without wiring the full
// lifecycle.
func TestReloadServicesAllocatesFreshRegistryWhenNil(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(dir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dir
	app.database = database

	if app.jobs != nil {
		t.Fatal("precondition: NewApp() must leave a.jobs nil so reloadServices can detect first-call vs preserve-existing")
	}

	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	if app.jobs == nil {
		t.Fatal("reloadServices must allocate a.jobs when NewApp left it nil")
	}

	// The freshly allocated Registry must be functional: Start a
	// job, Get it back, confirm ok.
	jobID := app.jobs.Start("test_kind", func(_ context.Context, _ *jobs.Progress) error { return nil })
	if _, ok := app.jobs.Get(jobID); !ok {
		t.Fatalf("fresh registry: job %s missing immediately after Start", jobID)
	}

	// Bound the test: the worker may take a moment to finish, but
	// it should complete fast. Don't hang the suite on a stuck
	// goroutine.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			if job, ok := app.jobs.Get(jobID); ok && job.Status == jobs.StatusDone {
				close(done)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not complete within 2s")
	}
}

// configureTestIdentity is defined in app_test.go (takes *App). Do NOT
// shadow it here — call it directly from each test.