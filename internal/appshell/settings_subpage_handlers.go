// settings_subpage_handlers.go -- issue #584.
//
// The 6 sub-page handlers that power /settings/<section>.
// Each handler renders the matching SettingsXxxView templ
// wrapped in the standard Layout. The split is purely
// templ-side: the existing POST handlers keep working
// unchanged (the form fields + URLs are identical).
//
// Locked per the issue:
//   - GET /settings/appearance -> theme, post-export surface, responsive layout mode
//   - GET /settings/updates -> source URL, check, apply, health, release notes banner
//   - GET /settings/maintenance -> image orphan scan + cleanup, data quality scan + apply
//   - GET /settings/data -> Initialize Local Archive (destructive)
//   - GET /settings/build -> build information (codename, branch, version, schema, commit/timestamp)
//   - GET /settings/diagnostics -> Support & Diagnostics + Debug Mode toggle

package appshell

import (
	"net/http"

	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
)

// handleSettingsAppearance serves GET /settings/appearance.
func (a *App) handleSettingsAppearance(w http.ResponseWriter, r *http.Request) {
	theme, exportSurface := a.loadAppearance()
	if err := presentation.SettingsAppearanceView(theme, exportSurface).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleSettingsUpdates serves GET /settings/updates.
func (a *App) handleSettingsUpdates(w http.ResponseWriter, r *http.Request) {
	settings, err := a.updater.Settings()
	if err != nil {
		respondInternal(w, r, "Could not load update settings.", err)
		return
	}
	if err := presentation.SettingsUpdatesView(settings).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleSettingsMaintenance serves GET /settings/maintenance.
func (a *App) handleSettingsMaintenance(w http.ResponseWriter, r *http.Request) {
	if err := presentation.SettingsMaintenanceView().Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleSettingsData serves GET /settings/data.
func (a *App) handleSettingsData(w http.ResponseWriter, r *http.Request) {
	if err := presentation.SettingsDataView(initializeDataConfirmationWord).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleSettingsBuild serves GET /settings/build. Renders the
// existing SettingsBuildPanel (issue #370 v1 About/Build
// panel) lifted into its own route. No new viewmodel needed;
// the build panel reads buildinfo directly.
func (a *App) handleSettingsBuild(w http.ResponseWriter, r *http.Request) {
	if err := presentation.SettingsBuildView().Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleSettingsDiagnostics serves GET /settings/diagnostics.
func (a *App) handleSettingsDiagnostics(w http.ResponseWriter, r *http.Request) {
	if err := presentation.SettingsDiagnosticsView(a.debugMode.Load()).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// loadAppearance centralizes the theme + export-surface
// resolution that the existing handleSettings inlines. The
// same fallback rules apply (Soft default for fresh installs
// per issue #494; ResolvedExportSurface default per #534).
func (a *App) loadAppearance() (theme, exportSurface string) {
	if v := a.theme.Load(); v != nil {
		if s, ok := v.(string); ok && s != "" {
			theme = s
		}
	}
	if theme == "" {
		theme = records.ThemeSoft
	}
	if v := a.exportSurface.Load(); v != nil {
		if s, ok := v.(string); ok && s != "" {
			exportSurface = s
		}
	}
	if exportSurface == "" {
		exportSurface = records.ResolvedExportSurface("")
	}
	return theme, exportSurface
}