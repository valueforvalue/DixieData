// Print-records fragment handler — serves the lazy-loaded body of
// the print-config modal. Called by htmx when the user opens the
// modal so the filter panel + record picker populate on demand
// instead of paying listAllSoldiers() cost on every /browse and
// /share GET (issue #234).
//
// The fragment is the same content the modal used to server-render
// in-place; splitting it out lets the page GET stay under 100ms
// even on 5k+ record archives. Modal stays interactive during the
// fetch — the JS in openPrintConfigModal disables the submit
// button until htmx:afterSwap fires on the placeholder.
package appshell

import (
	"net/http"

	"github.com/valueforvalue/DixieData/internal/debug/trace"
	"github.com/valueforvalue/DixieData/internal/templates/partials"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// handleSharePrintRecordsFragment returns the print-records
// fragment HTML for the print-config modal. GET only; the modal
// is opened by user click (not on page load) so a 405 for POST
// is intentional.
//
// On DB failure, the existing respondInternal helper writes an
// error message into the body so htmx swaps a visible error
// into [data-print-config-body] AND fires the X-DixieData-Toast
// header so the rest of the app surfaces the error in the toast
// queue.
func (a *App) handleSharePrintRecordsFragment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	soldiers, err := a.listAllSoldiers()
	if err != nil {
		respondInternal(w, r, "Could not load printable records.", err)
		return
	}
	options := viewmodel.ExportRecordOptionsFromModels(soldiers)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.PrintRecordsFragment(options).Render(r.Context(), w); err != nil {
		// Render failures after the header is set are awkward;
		// log + return so the client sees a partial response
		// rather than a confusing empty swap.
		trace.Log("print_records_fragment render failed", "err", err != nil)
	}
}