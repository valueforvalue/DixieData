// Package testtemp provides a t.TempDir replacement that
// tolerates the Windows file-handle retention race that
// os.RemoveAll hits when t.TempDir's deferred cleanup fires
// while SQLite / zip / os.File handles are still being torn
// down by the OS.
//
// Why this exists (issue #449 slice 3):
//
// TestXxxPackage tests across internal/{archive,db,seed,update}
// consistently fail on Windows with `unlinkat <tempdir>/<file>:
// The process cannot access the file because it is being used
// by another process` during `t.TempDir()`'s Cleanup. Go's
// stdlib t.TempDir registers a t.Cleanup that runs
// os.RemoveAll on the tempdir; on Windows the SQLite WAL/SHM
// sidecars + zip reader handles + os.File handles survive the
// test function's defer chain long enough for RemoveAll to
// hit them mid-flight.
//
// The fix is to run RemoveAll with retry-on-Ebusy / on-Access-is-denied
// and to give Windows a brief settle window between
// retries. Each retry tears the tempdir down attempt-by-attempt;
// the test stays green if cleanup eventually succeeds.
//
// Scope:
//   - Drop-in replacement for t.TempDir. `tt := testtemp.Dir(t)`
//     returns a *Dir with `Path()` + `Release()`. The dir
//     auto-cleans on t.Cleanup if Release() was never called.
//   - Used in: slice-3 commit + new tests added after.
//   - Already-deployed tests can migrate incrementally; the
//     existing t.TempDir still works.

package testtemp

import (
	"errors"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Dir is a t.TempDir-style directory with Windows-tolerant
// cleanup. The dir's path is fixed at Dir(t) time; Release()
// runs os.RemoveAll with retry-on-busy + gives the OS a settle
// window between attempts.
//
// Always use t.Cleanup(func(){ _ = d.Release() }) OR call
// d.Release() yourself before t exits. The auto-cleanup only
// fires if Release was never called.
type Dir struct {
	t        *testing.T
	path     string
	released bool
	mu       sync.Mutex
}

// New creates a new temp directory under t.TempDir's parent
// (so nested Dir() calls nest under the test's umbrella, like
// t.TempDir does). Auto-cleanup is registered via t.Cleanup —
// call Release() explicitly to control when the heavy I/O
// fires (typically AFTER all deferred file/SQLite closes).
func New(t *testing.T) *Dir {
	t.Helper()
	// Anchor under the test's own TempDir so the umbrella
	// test-level cleanup walks the parent last. t.TempDir()
	// already returns a uniquely-named dir; we just nest one
	// deeper with a unique suffix to avoid collision with any
	// other code that might have inspected the parent path.
	d := &Dir{t: t}
	path, err := os.MkdirTemp(t.TempDir(), "testtemp-*")
	if err != nil {
		t.Fatalf("testtemp.New: %v", err)
	}
	d.path = path
	t.Cleanup(func() {
		_ = d.Release()
	})
	return d
}

// Path returns the absolute filesystem path of the dir.
func (d *Dir) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.path
}

// Release runs os.RemoveAll on the dir with retry-on-busy. Safe
// to call multiple times — the second call is a no-op. Auto-
// cleanup calls Release via t.Cleanup if Release was never
// invoked explicitly.
//
// On Windows: RemoveAll can fail with Ebusy / "The process
// cannot access the file because it is being used by another
// process" mid-test if a file handle is still being torn down.
// We retry with exponential backoff (6 attempts, 200/400/800/1600/3200ms)
// and call runtime.Gosched + runtime.GC between retries to give
// the OS a settle window. If the retry loop exhausts, the
// error is logged via the test's testing.T (when available)
// so the failure shows up in the JSONL log + a go test -v
// output, NOT silently swallowed.
func (d *Dir) Release() error {
	d.mu.Lock()
	if d.released {
		d.mu.Unlock()
		return nil
	}
	d.released = true
	path := d.path
	d.path = ""
	d.mu.Unlock()

	err := removeAllWithRetry(path)
	if err != nil && d.t != nil {
		d.t.Logf("testtemp.Release: RemoveAll %s failed after retries: %v (file may be locked; outer t.TempDir cleanup will fail too)", path, err)
	}
	return err
}

// removeAllWithRetry is the platform-tolerant os.RemoveAll.
// Exposed at package scope so future migration helpers
// (testtemp.Copy, testtemp.Rename, etc.) can reuse it.
func removeAllWithRetry(path string) error {
	const attempts = 6
	const baseDelay = 200 * time.Millisecond
	var lastErr error
	delay := baseDelay
	for i := 0; i < attempts; i++ {
		if err := os.RemoveAll(path); err == nil {
			return nil
		} else {
			// Skip ErrNotExist on subsequent attempts so an
			// already-cleaned dir doesn't masquerade as a
			// failure.
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			lastErr = err
		}
		if i < attempts-1 {
			// Give Windows a settle window. runtime.GC forces a
			// GC pass which flushes finalizers that may hold
			// the last file-handle references. runtime.Gosched
			// yields the goroutine so the OS can finish
			// releasing handles before the next retry.
			runtime.GC()
			runtime.Gosched()
			time.Sleep(delay)
			delay *= 2
		}
	}
	// Last attempt failed — log via the test's testing.T if we
	// still have a reference. Tests that care can check
	// Release()'s error explicitly; tests that don't will see
	// the failure surfaced via the existing t.TempDir auto-
	// cleanup path (which will ALSO fail, surfacing the same
	// error). We don't double-log; the t.Cleanup path stays
	// silent and the failure mode is the same.
	_ = lastErr
	return lastErr
}
