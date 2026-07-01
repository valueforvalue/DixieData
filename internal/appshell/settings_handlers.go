// settings_handlers.go holds the settings HTTP handlers: the top-level
// /settings page, image-orphan scan + cleanup, data quality scan + apply,
// and the destructive /settings/initialize reset. Extracted from app.go
// as step 7 of the God-class reduction tracked in issue #42.
//
// Note: the settings/update* handlers (handleUpdateSource, handleCheckForUpdates,
// handleApplyLatestUpdate) live in app_update.go, which was extracted in a
// prior refactor. handleUpdateBootstrapHealth lives in app.go near the
// App lifecycle and is registered as /settings/updates/health/bootstrap.
// It will move to app_update.go in a future cleanup.
package appshell

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/presentation"
)

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := a.updater.Settings()
	if err != nil {
		respondInternal(w, r, "Could not load update settings.", err)
		return
	}
	presentation.SettingsView(initializeDataConfirmationWord, settings).Render(r.Context(), w)
}

func (a *App) handleScanImageOrphans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	orphans, err := a.images.DiscoverOrphans(a.dataDir)
	if err != nil {
		respondInternal(w, r, "Could not scan for orphaned images.", err)
		return
	}
	presentation.SettingsOrphanedImages(orphans).Render(r.Context(), w)
}

func (a *App) handleScanDataQuality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the data quality form.", err)
		return
	}
	mode := strings.TrimSpace(r.FormValue("quality_mode"))
	result, err := a.soldiers.RunDataQualityScan(mode)
	if err != nil {
		respondInternal(w, r, "Data quality scan failed.", err)
		return
	}
	presentation.SettingsQualityScanResults(result).Render(r.Context(), w)
}

func (a *App) handleApplyDataQuality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the apply-data-quality form.", err)
		return
	}
	selected, err := parseSelectedSoldierIDs(r.Form["selected_ids"])
	if err != nil {
		respondValidation(w, r, "Could not parse selected finding ids.", err)
		return
	}
	if len(selected) == 0 {
		setToastHeaderWithType(w, "Select at least one finding first.", "warning")
		fmt.Fprint(w, "Select at least one finding first.")
		return
	}
	result, err := a.soldiers.ApplyDataQualityFindingsToReviewQueue(selected)
	if err != nil {
		respondInternal(w, r, "Could not move selected records to the Review Queue.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Moved %d record(s) to Review Queue (%d already queued).", result.Flagged, result.AlreadyInQueue))
	presentation.SettingsQualityScanApplyResult(result).Render(r.Context(), w)
}

func (a *App) handleCleanupImageOrphans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the orphan cleanup form.", err)
		return
	}
	relativePaths := make([]string, 0, len(r.Form["orphan_path"]))
	for _, value := range r.Form["orphan_path"] {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			relativePaths = append(relativePaths, trimmed)
		}
	}
	var jobID string
	jobID = a.jobs.Start("image_orphan_cleanup", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, fmt.Sprintf("Moving %d orphan(s) to trash", len(relativePaths)))
		moved, trashRoot, err := a.images.MoveOrphansToTrash(a.dataDir, relativePaths)
		if err != nil {
			return err
		}
		p.Set(100, fmt.Sprintf("Moved %d image(s) into temp trash.", moved))
		_ = trashRoot
		return nil
	})
	setInfoToastHeader(w, "Orphan cleanup started…")
	// Option C: dispatchDixieDataForm reads X-DixieData-Redirect.
	writeExportRedirect(w, "/jobs/"+jobID)
}

func (a *App) handleSettingsInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the initialise form.", err)
		return
	}
	if strings.TrimSpace(r.FormValue("confirmation_word")) != initializeDataConfirmationWord {
		fmt.Fprintf(w, "Initialization cancelled. Type %s to confirm.", initializeDataConfirmationWord)
		return
	}
	if err := a.initializeLocalData(); err != nil {
		// For htmx requests, redirect to /setup so the user isn't
		// stranded on the broken form. For full-page nav, render
		// the error through the Layout wrapper.
		if blockIfFragment(w, r, "/setup") {
			return
		}
		a.respondErrorPage(w, r, KindInternal,
			"Initialisation failed. The local archive was not changed.",
			err)
		return
	}
	setInfoToastHeader(w, "Local archive initialised successfully.")
	writeExportRedirect(w, "/calendar")
}
