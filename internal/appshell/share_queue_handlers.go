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
package appshell

import (
	"fmt"
	"net/http"

	"github.com/valueforvalue/DixieData/internal/templates/partials"
)

// handleShareQueueModal renders the Share Build modal HTML fragment
// for the showOverlayModal slot. The modal body is a server-rendered
// shell; the queue list and live-preview counts are populated
// client-side from localStorage + the /share/queue/preview endpoint.
func (a *App) handleShareQueueModal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := partials.ShareQueueModal()
	if err := component.Render(r.Context(), w); err != nil {
		// Headers may already be sent (templ streamed partial
		// output). Surface the error server-side; the client
		// will see a truncated modal.
		http.Error(w, "share queue modal render failed", http.StatusInternalServerError)
	}
}

// handleShareQueuePreview returns a small HTML fragment with the
// Person Records count for the supplied selected_ids repeating
// form field. The client calls this on modal open and on every
// add/remove. The response replaces the [data-share-queue-preview]
// pane inside the modal.
//
// For v1, only the Person Records count is computed (the input
// length). Source Records and Images counts are stubbed at zero;
// a follow-up issue can wire per-record enrichment (mirroring the
// manifest-count fix in commit 4) when the live-preview is used
// for production exports. Matches the printable-PDF preview
// shape from issue #179.
func (a *App) handleShareQueuePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.Header().Set("X-DixieData-Toast", "Could not read the Share Queue preview request.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ids, err := parseSelectedSoldierIDs(r.Form["selected_ids"])
	if err != nil {
		w.Header().Set("X-DixieData-Toast", "Invalid Share Queue selection.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	// Single HTML fragment that replaces the entire
	// [data-share-queue-preview] container. Client swaps via
	// htmx or a manual innerHTML write; the wrapper keeps the
	// data-* hooks intact for subsequent reads.
	fmt.Fprintf(w, `<div data-share-queue-preview class="grid grid-cols-3 gap-3 text-center text-xs text-slate-600">`+
		`<div><div data-share-queue-preview-records class="text-lg font-semibold text-[#22303d]">%d</div><div>Person Records</div></div>`+
		`<div><div data-share-queue-preview-sources class="text-lg font-semibold text-[#22303d]">0</div><div>Source Records</div></div>`+
		`<div><div data-share-queue-preview-images class="text-lg font-semibold text-[#22303d]">0</div><div>Images</div></div>`+
		`</div>`, len(ids))
}

// handleShareQueueClear is reserved for a future "force clear" admin
// path. For v1 the client-side localStorage entry clears via the
// modal's "Clear" button (no server round-trip); this handler
// returns 200 with an X-DixieData-Toast acknowledging the call so
// any future server-side queue state can hook in here without a
// route change.
func (a *App) handleShareQueueClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("X-DixieData-Toast", "Share Queue cleared.")
	w.WriteHeader(http.StatusOK)
}
