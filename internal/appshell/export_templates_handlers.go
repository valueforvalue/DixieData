package appshell

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/templates/partials"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
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
		SelectedIDs:       settings.SelectedIDs,
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

// handleUpdateExportTemplate (issue #186) replaces the mutable
// fields on a saved template. Accepts PATCH or POST (templates
// work with the standard Option C dispatcher and HTML forms do
// not natively issue PATCH). On 200 returns {id, name} JSON. On
// 404 (missing) or 409 (name collision) surfaces the issue code
// via the existing respond-error shape so the modal's
// templates-status slot can render an inline message.
func (a *App) handleUpdateExportTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseExportTemplateIDFromPath(r.URL.Path, "")
	if err != nil {
		http.Error(w, "invalid template id", http.StatusBadRequest)
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
	updated, err := a.exportTemplates.Update(id, records.ExportTemplate{
		Name:              name,
		Scope:             settings.Scope,
		Filters:           collectExportFilters(r),
		SortBy:            settings.SortBy,
		GroupBy:           collectExportGroupBy(settings),
		Orientation:       settings.Orientation,
		SelectedIDs:       settings.SelectedIDs,
		PrinterFriendly:   settings.PrinterFriendly,
		FullBiographyPage: settings.FullBiographyPage,
	})
	if err != nil {
		switch {
		case errors.Is(err, records.ErrExportTemplateNotFound):
			respondNotFound(w, r, fmt.Sprintf("Template %d not found.", id), err)
		case errors.Is(err, records.ErrExportTemplateNameTaken):
			http.Error(w, "A template with that name already exists. Pick a different name.", http.StatusConflict)
		default:
			respondInternal(w, r, fmt.Sprintf("Could not update template %d.", id), err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":   updated.ID,
		"name": updated.Name,
	}); err != nil {
		return
	}
}

// handleApplyExportTemplate returns the stored fields as JSON so
// the modal can populate its form, wrapped in a response that also
// lists any "stale" filter values or selected IDs that no longer
// exist in the current archive (issue #181). Bumps last_used_at.
//
// Response shape: {"template": {...ExportTemplate fields...},
// "warnings": ["Filter Unit no longer matches any record: ..."]}
//
// The frontend reads data.template and applies the fields to the
// form, then pops a toast per warning (batched).
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
	warnings := a.computeExportTemplateStale(template)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"template": template,
		"warnings": warnings,
	}); err != nil {
		return
	}
}

// computeExportTemplateStale cross-checks a loaded template's
// filter values and selected IDs against the current archive and
// returns one warning per stale entry. Filters out unknown-value
// sentinels (the "__unknown__" marker the modal uses for "show
// records with this field blank") — those are still meaningful
// even if no current record uses them.
func (a *App) computeExportTemplateStale(template records.ExportTemplate) []string {
	familyLabels := map[string]string{
		"buried_in":                "Burial Location",
		"entry_type":               "Entry Type",
		"unit":                     "Unit",
		"pension_state":            "Pension State",
		"confederate_home_status":  "Confederate Home Status",
	}
	current, err := a.listAllSoldiers()
	if err != nil {
		// Without the current archive we can't decide what's stale.
		// Return no warnings rather than misleading the user.
		return nil
	}
	exportRecords := viewmodel.ExportRecordOptionsFromModels(current)
	warnings := []string{}
	for family, values := range template.Filters {
		have := map[string]bool{}
		for _, v := range partials.ExportUniqueFilterValues(exportRecords, exportFieldAccessor(family)) {
			if v == "" {
				continue
			}
			have[strings.ToLower(v)] = true
		}
		for _, v := range values {
			if v == "" || strings.TrimSpace(v) == partials.ExportFilterUnknownValue {
				continue
			}
			if !have[strings.ToLower(strings.TrimSpace(v))] {
				warnings = append(warnings, fmt.Sprintf(
					"Filter %s no longer matches any record: %q.",
					familyLabels[family], v,
				))
			}
		}
	}
	if template.Scope == render.PrintScopeSelected {
		idSet := map[int64]bool{}
		for _, s := range current {
			idSet[s.ID] = true
		}
		for _, id := range template.SelectedIDs {
			if !idSet[id] {
				warnings = append(warnings, fmt.Sprintf(
					"Selected ID %d no longer exists in the archive.", id,
				))
			}
		}
	}
	return warnings
}

// exportFieldAccessor returns the field-extraction function for the
// given filter family, suitable for exportUniqueFilterValues.
func exportFieldAccessor(family string) func(viewmodel.ExportRecordOption) string {
	switch family {
	case "buried_in":
		return func(r viewmodel.ExportRecordOption) string { return r.BuriedIn }
	case "entry_type":
		return func(r viewmodel.ExportRecordOption) string { return r.EntryType }
	case "unit":
		return func(r viewmodel.ExportRecordOption) string { return r.Unit }
	case "pension_state":
		return func(r viewmodel.ExportRecordOption) string { return r.PensionState }
	case "confederate_home_status":
		return func(r viewmodel.ExportRecordOption) string { return r.ConfederateHomeStatus }
	default:
		return func(r viewmodel.ExportRecordOption) string { return "" }
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