package appshell

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/pkg/render"
)

// handleListExportTemplates returns the saved templates as a small
// JSON array used by the modal's Load dropdown. GET
// /export/templates.
func (a *App) handleListExportTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	templates, err := a.exportTemplates.List()
	if err != nil {
		respondInternal(w, r, "Could not load saved templates.", err)
		return
	}
	out := make([]exportTemplateSummary, 0, len(templates))
	for _, t := range templates {
		out = append(out, exportTemplateSummary{
			ID:          t.ID,
			Name:        t.Name,
			Scope:       t.Scope,
			LastUsedAt:  t.LastUsedAt,
			Orientation: t.Orientation,
		})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{"templates": out}); err != nil {
		// Body already partially written; nothing useful to do.
		return
	}
}

type exportTemplateSummary struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	LastUsedAt  any    `json:"last_used_at"`
	Orientation string `json:"orientation"`
}

// handleSaveExportTemplate reads the modal's form fields plus a
// template_name input and inserts a new row. POST /export/templates.
func (a *App) handleSaveExportTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the template form.", err)
		return
	}
	name := strings.TrimSpace(r.FormValue("template_name"))
	if name == "" {
		respondValidation(w, r, "Template name is required.", errors.New("missing template_name"))
		return
	}
	settings, err := parsePrintSettingsRequest(r)
	if err != nil {
		respondValidation(w, r, "Could not read the print settings.", err)
		return
	}
	template := records.ExportTemplate{
		Name:              name,
		Scope:             settings.Scope,
		Filters:           collectExportFilters(r),
		SortBy:            settings.SortBy,
		GroupBy:           collectExportGroupBy(settings),
		Orientation:       settings.Orientation,
		PrinterFriendly:   settings.PrinterFriendly,
		FullBiographyPage: settings.FullBiographyPage,
	}
	saved, err := a.exportTemplates.Create(template)
	if err != nil {
		if errors.Is(err, records.ErrExportTemplateNameTaken) {
			http.Error(w, "A template with that name already exists. Pick a different name.", http.StatusConflict)
			return
		}
		respondInternal(w, r, "Could not save the template.", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":   saved.ID,
		"name": saved.Name,
	}); err != nil {
		return
	}
}

// handleDeleteExportTemplate removes a saved template. DELETE
// /export/templates/{id}.
func (a *App) handleDeleteExportTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/export/templates/"), 10, 64)
	if err != nil {
		http.Error(w, "invalid template id", http.StatusBadRequest)
		return
	}
	if err := a.exportTemplates.Delete(id); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not delete template %d.", id), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleApplyExportTemplate returns the stored fields as JSON so
// the modal can populate its form. Also bumps last_used_at.
// POST /export/templates/{id}/apply.
func (a *App) handleApplyExportTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseExportTemplateIDFromPath(r.URL.Path, "/apply")
	if err != nil {
		http.Error(w, "invalid template id", http.StatusBadRequest)
		return
	}
	template, err := a.exportTemplates.Get(id)
	if err != nil {
		if errors.Is(err, records.ErrExportTemplateNotFound) {
			http.Error(w, "template not found", http.StatusNotFound)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not load template %d.", id), err)
		return
	}
	if err := a.exportTemplates.TouchLastUsed(id); err != nil {
		// Non-fatal — the template still loads.
		respondInternal(w, r, fmt.Sprintf("Could not update last_used_at for template %d.", id), err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(template); err != nil {
		return
	}
}

// collectExportFilters reads the multi-value filter_* form fields
// into the map shape ExportTemplate.Filters expects (family -> []string).
func collectExportFilters(r *http.Request) map[string][]string {
	families := []string{"buried_in", "entry_type", "unit", "pension_state", "confederate_home_status"}
	out := map[string][]string{}
	for _, family := range families {
		key := "filter_" + family
		values := r.Form[key]
		if len(values) == 0 {
			continue
		}
		cleaned := make([]string, 0, len(values))
		for _, v := range values {
			if v = strings.TrimSpace(v); v != "" {
				cleaned = append(cleaned, v)
			}
		}
		if len(cleaned) > 0 {
			out[family] = cleaned
		}
	}
	return out
}

// collectExportGroupBy returns the list of group_by_* settings
// that are enabled, in the canonical order the modal uses for
// rendering the JSON side.
func collectExportGroupBy(settings render.PrintSettings) []string {
	var out []string
	if settings.GroupByUnit {
		out = append(out, "unit")
	}
	if settings.GroupByPensionState {
		out = append(out, "pension_state")
	}
	if settings.GroupByConfederateHomeStatus {
		out = append(out, "confederate_home_status")
	}
	if settings.GroupByBuriedIn {
		out = append(out, "buried_in")
	}
	return out
}

// parseExportTemplateIDFromPath extracts the id from a URL like
// /export/templates/{id} or /export/templates/{id}/apply.
func parseExportTemplateIDFromPath(path, suffix string) (int64, error) {
	trimmed := strings.TrimPrefix(path, "/export/templates/")
	trimmed = strings.TrimSuffix(trimmed, suffix)
	return strconv.ParseInt(trimmed, 10, 64)
}