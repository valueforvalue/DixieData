// update_progress.go -- thread-safe progress state for the
// in-place update apply handler (issue #661).
//
// The apply handler spawns PrepareLatestWithProgress in a
// goroutine and stores live progress here. The
// /settings/updates/progress GET endpoint reads the latest
// state on every poll (500ms cadence from the JS dispatcher)
// so the user sees a live progress bar during the download
// + verify + extract + restore-point pipeline.
//
// Lives next to app_update.go (the apply handler) so the
// whole "in-place update with progress" surface is in one
// directory.
package appshell

import (
	"sync"

	"github.com/valueforvalue/DixieData/internal/update"
)

// updateProgressState is the per-process progress state the
// apply goroutine writes to and the progress endpoint reads
// from. The mutex guards the embedded UpdateProgress struct
// only — the *updateProgressState pointer itself is set once
// at App construction and never reassigned, so the pointer is
// safe to share across goroutines without a separate mutex.
type updateProgressState struct {
	mu sync.RWMutex
	p  update.UpdateProgress
}

// newUpdateProgressState returns a zero-value progress state
// with Phase=PhaseIdle so the UI can render an empty
// progress bar before the apply handler fires.
func newUpdateProgressState() *updateProgressState {
	return &updateProgressState{
		p: update.UpdateProgress{Phase: update.PhaseIdle},
	}
}

// set replaces the current progress state under the write lock.
func (s *updateProgressState) set(p update.UpdateProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p = p
}

// get returns a snapshot of the current progress state under
// the read lock. The returned value is a copy so the caller
// can render it without holding the lock.
func (s *updateProgressState) get() update.UpdateProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.p
}