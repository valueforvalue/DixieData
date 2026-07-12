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

	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
)

// resolvedBootTheme returns the theme name that should be stamped
// on the very first paint of the Wails WebView2 — before App.Startup()
// has necessarily stored a.theme. It mirrors the nil-safe + disk-backfill
// pattern used by renderStartupPlaceholder (app.go) so the static
// frontend/index.html shell (served by the Wails asset server, which
// bypasses ServeHTTP for the first paint of "/") can request the
// persisted theme via the /boot-theme.js script and apply it to
// <html data-theme> before the body paints.
//
// Issue #483 follow-up: the #474/#481 fixes only themed the
// server-rendered Layout pages + the pre-mux placeholder. But the
// Wails asset server serves frontend/index.html as a STATIC asset
// for "/" — Go never sees that request, so Layout (which sets
// <html data-theme> server-side) never runs for the first paint.
// The shell's <html> has no data-theme, and the shell's
// <body hx-get="/calendar" hx-trigger="load"> htmx swap discards the
// response's <html data-theme> (innerHTML swap into <body> doesn't
// touch the <html> element). So High Contrast / Soft users saw the
// Default theme on the home screen until a full-page navigation
// re-rendered the whole document through Layout. The fix: a blocking
// <script src="/boot-theme.js"> in index.html <head> runs before the
// body paints and sets the attribute from the same source Layout
// would use. This helper resolves that source: a.theme atomic first
// (steady state), then local_settings.json on disk (cold-start race
// before Startup stores the atomic), then records.ThemeDefault.
func resolvedBootTheme(a *App) string {
	var theme string
	if a != nil {
		if v := a.theme.Load(); v != nil {
			if s, ok := v.(string); ok {
				theme = s
			}
		}
	}
	if theme == "" && a != nil && a.dataDir != "" {
		if settings, err := records.LoadLocalSettings(a.dataDir); err == nil {
			theme = settings.ResolvedTheme()
		}
	}
	if theme == "" {
		theme = records.ThemeDefault
	}
	return theme
}

// handleBootThemeScript serves a tiny JS snippet that stamps the
// persisted theme on document.documentElement before the static
// index.html shell paints. See resolvedBootTheme for the full
// rationale. The response is no-store so a theme change in /settings
// is reflected on the next shell load (the shell only loads on a
// full navigation to "/", so the cost is one tiny fetch per landing).
func (a *App) handleBootThemeScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	fmt.Fprintf(w, "document.documentElement.setAttribute('data-theme',%q);", resolvedBootTheme(a))
}

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
	// Issue #474: pass the current resolved theme so the Appearance
	// card can render the right radio as checked. The same value is
	// served via the per-request context (WithLayoutTheme) so the
	// first paint of <html data-theme="..."> already matches.
	var currentTheme string
	if v := a.theme.Load(); v != nil {
		if s, ok := v.(string); ok {
			currentTheme = s
		}
	}
	if currentTheme == "" {
		currentTheme = records.ThemeDefault
	}
	// Issue #384 / Slice 7: wrap Render.
	if err := presentation.SettingsView(initializeDataConfirmationWord, settings, currentTheme).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the settings page.", err)
	}
}

// handleSettingsTheme is the POST handler for the Appearance form
// on the Settings page. It reads the picked theme, validates it
// against the known set, persists via SaveLocalSettings, and
// updates the in-memory App.theme store so the next request renders
// <html data-theme="..."> with the new value. The form is submitted
// via the JS dispatcher (data-dixie-submit="true") so the response
// uses the standard X-DixieData-Redirect back to /settings to
// re-render the page with the new selection.
//
// Issue #474 — the theme system. Slice 1 only validates + persists;
// the palette work is slice 2.
func (a *App) handleSettingsTheme(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the theme form.", err)
		return
	}
	picked := strings.TrimSpace(r.FormValue("theme"))
	switch picked {
	case records.ThemeDefault, records.ThemeHighContrast, records.ThemeSoft:
		// ok
	default:
		respondValidation(w, r, "Pick one of: Default, High Contrast, Soft.", nil)
		return
	}
	settings, err := records.LoadLocalSettings(a.dataDir)
	if err != nil {
		respondInternal(w, r, "Could not load local settings.", err)
		return
	}
	settings.Theme = picked
	if err := records.SaveLocalSettings(a.dataDir, settings); err != nil {
		respondInternal(w, r, "Could not save local settings.", err)
		return
	}
	a.theme.Store(picked)
	log := debug.FromContext(r.Context())
	log.Info("theme changed via settings", "theme", picked)
	setToastHeader(w, fmt.Sprintf("Theme set to %s.", themeDisplayName(picked)))
	// The form is dispatched via the JS dispatcher; respond with the
	// standard X-DixieData-Redirect so the browser navigates back to
	// /settings and the new theme is reflected on the next paint.
	writeExportRedirect(w, "/settings")
}

// themeDisplayName maps the persisted theme value to the user-facing
// label used in toast messages. Keep in sync with the radio options
// in SettingsAppearancePanel.
func themeDisplayName(value string) string {
	switch value {
	case records.ThemeHighContrast:
		return "High Contrast"
	case records.ThemeSoft:
		return "Soft"
	default:
		return "Default"
	}
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
	// Issue #384 / Slice 7: wrap Render.
	if err := presentation.SettingsOrphanedImages(orphans).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the orphaned-images results.", err)
	}
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
	// Issue #384 / Slice 7: wrap Render.
	if err := presentation.SettingsQualityScanResults(result).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the data-quality scan results.", err)
	}
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
	// Issue #384 / Slice 7: wrap Render.
	if err := presentation.SettingsQualityScanApplyResult(result).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the apply-data-quality results.", err)
	}
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
