// share_queue_handlers.go holds the HTTP handlers for the Share Queue
// feature (issue #182). The Share Queue is the in-memory list of
// Person Records a researcher has staged for inclusion in a Shared
// Archive (.ddshare) before exporting; see CONTEXT.md for the term
// and the [Unreleased] CHANGELOG bullet in commit 1.
//
// Routes registered in routes.go:
//
//   GET  /share/queue/modal    → handleShareQueueModal
//   POST /share/queue/preview  → handleShareQueuePreview
//   POST /share/queue/clear    → handleShareQueueClear
//
// The actual export branch lives inside handleExportSharedArchive
// in exports_handlers.go (the same handler, the ?subset=1 branch);
// the three handlers here are the in-app envelope that lets the
// user build, preview, and clear the queue.
//
// Commit 4 (this commit) wires the routes + regression tests. The
// UI fragments and the modal body land in commit 5; this file's
// handlers return a small 501 surface with a clear "not yet wired"
// marker so the route table is correct and the audit manifest
// (audit/discover_export_buttons.mjs) treats them as known
// non-export endpoints from the moment the routes are registered.
package appshell

import (
	"net/http"
)

// handleShareQueueModal renders the Share Queue modal HTML fragment
// for the showOverlayModal slot. The full implementation lands in
// commit 5; this stub returns 501 with a clear toast header so
// callers see a useful response and the route is registered.
func (a *App) handleShareQueueModal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("X-DixieData-Toast", "Share Queue modal is not yet wired (issue #182, commit 5).")
	w.WriteHeader(http.StatusNotImplemented)
}

// handleShareQueuePreview returns a small HTML fragment with
// Person Records / Source Records / Images counts for the supplied
// selected_ids repeating form field. The full implementation lands
// in commit 5.
func (a *App) handleShareQueuePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("X-DixieData-Toast", "Share Queue preview is not yet wired (issue #182, commit 5).")
	w.WriteHeader(http.StatusNotImplemented)
}

// handleShareQueueClear clears the Share Queue on the server side.
// The client-side localStorage entry clears via the modal's "Clear"
// button (no server round-trip); this handler is reserved for a
// future "force clear" admin path. The full implementation lands in
// commit 5.
func (a *App) handleShareQueueClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("X-DixieData-Toast", "Share Queue clear is not yet wired (issue #182, commit 5).")
	w.WriteHeader(http.StatusNotImplemented)
}
