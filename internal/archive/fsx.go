// Package-internal helper for issue #449 follow-up: deferred
// temp-directory cleanup hits the Windows unlinkat file-handle
// race when an *os.File (or a child process like pdfium.exe)
// is still being torn down by the OS at the moment the deferred
// RemoveAll fires. The unlinkat error
// "The process cannot access the file because it is being used
// by another process" leaves the parent dir behind.
//
// The fix is the same pattern testtemp.removeAllWithRetry uses:
// 6 attempts with exponential backoff, runtime.GC +
// runtime.Gosched between attempts to give the OS a settle
// window for file-handle release.
//
// Scope: package-internal because only the deferred tempdir
// cleanups across internal/archive use it. If a sibling
// package (pkg/render, internal/db) needs the same fix, lift
// the helper to internal/debug or a new internal/fsx package
// rather than re-export from testtemp (whose purpose is test
// boundaries, not production safety).
package archive

import (
	"errors"
	"os"
	"runtime"
	"time"
)

// removeAllWithRetry runs os.RemoveAll with retry-on-busy
// semantics to survive the Windows unlinkat file-handle race.
// On non-Windows platforms the retry is a no-op because the
// first RemoveAll succeeds; the cost is six RemoveAll calls
// on the happy path (negligible). Returns nil on success or
// ErrNotExist (idempotent). Returns the last retry's error on
// exhaustion. Callers in a defer should typically log + ignore.
func removeAllWithRetry(path string) error {
	const attempts = 6
	const baseDelay = 200 * time.Millisecond
	var lastErr error
	delay := baseDelay
	for i := 0; i < attempts; i++ {
		err := os.RemoveAll(path)
		if err == nil {
			return nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		lastErr = err
		if i < attempts-1 {
			runtime.GC()
			runtime.Gosched()
			time.Sleep(delay)
			delay *= 2
		}
	}
	return lastErr
}
