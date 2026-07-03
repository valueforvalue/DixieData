package appshell

import (
	"net/http"

	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
)

// Issue #284: dedicated handlers for the /share/exports,
// /share/imports, /share/sync subpages. These three
// subpages replace the in-page #export-section /
// #import-section / Google Integration card that lived
// inline on /share pre-#284. The /share landing is
// updated to render a sub-overview (Slice 2). The foldout
// menu items update to navigate to these routes
// (Slice 2).
//
// Each handler loads only the data its subpage needs.
// The original handleShare loaded everything for all
// three sections; with the split, /share/exports no
// longer needs Google status, /share/imports needs
// nothing handler-side (it's a button launchpad), and
// /share/sync needs the full Google status + drift
// counts. This is the smaller, more honest shape.

// handleShareExports serves GET /share/exports.
// Data: exportRecords (for the PrintConfigModal partial
// under the section) + shareIncludeTagsEnabled (for
// the include-tags checkbox).
func (a *App) handleShareExports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	exportRecords, err := a.listAllSoldiers()
	if err != nil {
		respondInternal(w, r, "Could not enumerate person records for export.", err)
		return
	}
	shareIncludeTags := a.archiveMeta.IncludeTags(r.Context(), records.ArchiveKindShared)
	presentation.ShareExportsView(exportRecords, shareIncludeTags).Render(r.Context(), w)
}

// handleShareImports serves GET /share/imports. Static
// launchpad — no handler-side data needed. All three
// import buttons fire data-action submits to the
// existing /import/* routes.
func (a *App) handleShareImports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	presentation.ShareImportsView().Render(r.Context(), w)
}

// handleShareSync serves GET /share/sync. Data: full
// Google status (Connected, calendar IDs, last sync,
// shared client state) + drift counts (from
// CalendarDriftStatus, called against a fresh list of
// export records to match the pre-split behaviour).
func (a *App) handleShareSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := a.google.Status()
	if err != nil {
		respondInternal(w, r, "Could not load Google integration status.", err)
		return
	}
	exportRecords, err := a.listAllSoldiers()
	if err != nil {
		respondInternal(w, r, "Could not enumerate person records for sync drift.", err)
		return
	}
	drift, err := a.google.CalendarDriftStatus(exportRecords)
	if err != nil {
		respondInternal(w, r, "Could not compute Google Calendar drift.", err)
		return
	}
	status.LastSyncedAt = drift.LastSyncedAt
	status.DriftAdded = drift.Added
	status.DriftUpdated = drift.Updated
	status.DriftRemoved = drift.Removed
	status.OutOfSync = drift.OutOfSync
	presentation.ShareSyncView(status).Render(r.Context(), w)
}
