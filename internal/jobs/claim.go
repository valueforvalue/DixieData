// claim.go — issue #615 slice 1.
//
// TryClaim provides a lightweight in-flight dedup primitive for the
// jobs.Registry. Each handler that needs "one at a time" semantics
// (export dialog, backup restore, manual-job confirmation) calls
// release, ok := registry.TryClaim(key). When ok is false, another
// caller already holds the claim and the handler can redirect or
// return a friendly error. The release function is idempotent.
//
// Claims replace the 4 redundant in-flight mechanisms that existed
// before this slice:
//   1. a.inFlight sync.Map (dialog dedup)
//   2. importInFlight atomic.Bool (backup restore guard)
//   3. manualJobs sync.Map (manual job callbacks)
//   4. errExportInFlight sentinel error
//
// The grep probe after all slices land: sync.Map|inFlight|atomic.Value
// in appshell/ returns ≤2 (only dialog-guard internals + claim internals).

package jobs

import (
	"sync/atomic"
)

// TryClaim attempts to acquire the named claim. Returns (release, true)
// if this caller is the active owner. Returns (nil, false) if another
// caller already holds the claim.
//
// The release function is idempotent — safe to call multiple times.
// Typical usage:
//
//	release, ok := reg.TryClaim("export:pdf:soldier-42")
//	if !ok {
//	    // Another export is in flight; redirect or show toast.
//	    return
//	}
//	defer release()
//	// ... open dialog, run work ...
func (r *Registry) TryClaim(key string) (release func(), ok bool) {
	var released atomic.Bool
	_, loaded := r.claims.LoadOrStore(key, struct{}{})
	if loaded {
		return nil, false
	}
	release = func() {
		if released.CompareAndSwap(false, true) {
			r.claims.Delete(key)
		}
	}
	return release, true
}
