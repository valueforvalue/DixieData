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
	"fmt"
	"net/http"
	"strings"

	"github.com/valueforvalue/DixieData/internal/presentation"
)

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := a.updater.Settings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	mode := strings.TrimSpace(r.FormValue("quality_mode"))
	result, err := a.soldiers.RunDataQualityScan(mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	selected, err := parseSelectedSoldierIDs(r.Form["selected_ids"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(selected) == 0 {
		setToastHeaderWithType(w, "Select at least one finding first.", "warning")
		fmt.Fprint(w, "Select at least one finding first.")
		return
	}
	result, err := a.soldiers.ApplyDataQualityFindingsToReviewQueue(selected)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	relativePaths := make([]string, 0, len(r.Form["orphan_path"]))
	for _, value := range r.Form["orphan_path"] {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			relativePaths = append(relativePaths, trimmed)
		}
	}
	moved, trashRoot, err := a.images.MoveOrphansToTrash(a.dataDir, relativePaths)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setToastHeader(w, fmt.Sprintf("Moved %d orphaned image(s) into temp trash for 30-day retention.", moved))
	presentation.SettingsOrphanCleanupResult(moved, trashRoot).Render(r.Context(), w)
}

func (a *App) handleScanCompressibleImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	candidates, err := a.compress.DiscoverUncompressed(a.dataDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presentation.SettingsCompressibleImages(candidates).Render(r.Context(), w)
}

func (a *App) handleRunImageCompression(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	candidates, err := a.compress.DiscoverUncompressed(a.dataDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(candidates) == 0 {
		fmt.Fprint(w, "No images need compression.")
		return
	}
	relPaths := make([]string, len(candidates))
	for i, c := range candidates {
		relPaths[i] = c.RelativePath
	}
	report, _ := a.compress.CompressParallel(a.dataDir, relPaths, 4, nil)
	// CompressParallel always returns nil error; per-file failures are
	// surfaced via report.Errors. Matches handleCleanupImageOrphans shape.
	setToastHeader(w, fmt.Sprintf("Compressed %d image(s); saved %s (errors: %d).",
		report.Compressed, formatBytes(report.OriginalBytes-report.FinalBytes), len(report.Errors)))
	presentation.SettingsCompressionResult(report).Render(r.Context(), w)
}

// formatBytes converts a byte count to a human-readable string (e.g.
// 1.5 MB, 832 KB). Used by the compress-result toast.
func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func (a *App) handleSettingsInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(r.FormValue("confirmation_word")) != initializeDataConfirmationWord {
		fmt.Fprintf(w, "Initialization cancelled. Type %s to confirm.", initializeDataConfirmationWord)
		return
	}
	if err := a.initializeLocalData(); err != nil {
		fmt.Fprintf(w, "Initialization failed: %v", err)
		return
	}
	fmt.Fprint(w, "Local archive reset. A fresh database and folder tree were created.")
}
