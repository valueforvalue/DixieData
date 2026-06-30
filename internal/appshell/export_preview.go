package appshell

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/pkg/render"
)

// handleExportPreview returns a small HTML fragment describing what
// the current print-config modal selection would generate: total
// record count, first 5 display IDs + names, active sort label,
// active group-by labels, and a "scope" note. Used by the live
// preview panel added to the print-config modal in issue #179.
//
// Response shape: a single <div> with classes matching the rest of
// the modal's preview region. The frontend inserts it into
// [data-print-config-preview] via the existing inline-render path
// in dispatchDixieDataForm.
//
// Reuses exportbridge.PrintSettingsFromForm + the existing
// listAllSoldiers + render.FilterPrintableSoldiers +
// render.SortPrintableSoldiers pipeline so the preview resolves
// identically to the actual PDF generation. Cost is bounded:
// listAllSoldiers loads N rows once; the in-memory filter and
// sort are O(N log N). For the 1000-record stress test in
// TestHandleBrowseResponseUnderThreshold the preview fetch is
// dominated by the same list query.
func (a *App) handleExportPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := parsePrintSettingsRequest(r)
	if err != nil {
		// An empty form (or scope=selected with no IDs) yields a
		// settings error. Render a friendly preview instead of
		// bubbling 400 — the modal caller may still be filling
		// fields in.
		writeExportPreviewFragment(w, exportPreviewView{
			Count:    0,
			Sort:     previewSortLabel(settings.SortBy),
			GroupBy:  previewGroupLabels(settings),
			Note:     "Select at least one record, or pick a different scope, to see a preview.",
			HasError: true,
		})
		return
	}

	allSoldiers, err := a.listAllSoldiers()
	if err != nil {
		respondInternal(w, r, "Could not load person records for preview.", err)
		return
	}

	matched := filterAndSortPreview(allSoldiers, settings)

	view := exportPreviewView{
		Count:   len(matched),
		Sort:    previewSortLabel(settings.SortBy),
		GroupBy: previewGroupLabels(settings),
		First:   previewFirstRecords(matched),
	}
	switch settings.Scope {
	case render.PrintScopeAll:
		view.Note = fmt.Sprintf("Scope: All records. Showing first %d.", len(view.First))
	case render.PrintScopeFiltered:
		view.Note = fmt.Sprintf("Scope: Filtered records. Showing first %d of %d matched.", len(view.First), view.Count)
	case render.PrintScopeSelected:
		view.Note = fmt.Sprintf("Scope: Manually selected (%d IDs). Showing first %d.", len(settings.SelectedIDs), len(view.First))
	default:
		view.Note = "Scope: unknown."
	}
	writeExportPreviewFragment(w, view)
}

type exportPreviewView struct {
	Count    int
	Sort     string
	GroupBy  []string
	First    []previewRecordLine
	Note     string
	HasError bool
}

type previewRecordLine struct {
	DisplayID string
	Name      string
}

func filterAndSortPreview(all []models.Soldier, settings render.PrintSettings) []models.Soldier {
	settings = settings.Normalize()
	switch settings.Scope {
	case render.PrintScopeSelected:
		if len(settings.SelectedIDs) == 0 {
			return nil
		}
		idSet := make(map[int64]struct{}, len(settings.SelectedIDs))
		for _, id := range settings.SelectedIDs {
			idSet[id] = struct{}{}
		}
		filtered := make([]models.Soldier, 0, len(settings.SelectedIDs))
		for _, s := range all {
			if _, ok := idSet[s.ID]; ok {
				filtered = append(filtered, s)
			}
		}
		render.SortPrintableSoldiers(filtered, settings)
		return filtered
	case render.PrintScopeFiltered:
		filtered := render.FilterPrintableSoldiers(all, settings)
		render.SortPrintableSoldiers(filtered, settings)
		return filtered
	default:
		render.SortPrintableSoldiers(all, settings)
		return all
	}
}

func previewSortLabel(sortBy string) string {
	switch strings.TrimSpace(sortBy) {
	case "birth_year":
		return "Chronological by Birth Year"
	case "death_year":
		return "Chronological by Death Year"
	default:
		return "Alphabetical by Last Name"
	}
}

func previewGroupLabels(settings render.PrintSettings) []string {
	settings = settings.Normalize()
	var labels []string
	if settings.GroupByUnit {
		labels = append(labels, "Unit")
	}
	if settings.GroupByPensionState {
		labels = append(labels, "Pension State")
	}
	if settings.GroupByConfederateHomeStatus {
		labels = append(labels, "Confederate Home Status")
	}
	if settings.GroupByBuriedIn {
		labels = append(labels, "Burial Location")
	}
	if len(labels) == 0 {
		return []string{"No grouping"}
	}
	return labels
}

func previewFirstRecords(soldiers []models.Soldier) []previewRecordLine {
	const cap = 5
	if len(soldiers) > cap {
		soldiers = soldiers[:cap]
	}
	lines := make([]previewRecordLine, 0, len(soldiers))
	for _, s := range soldiers {
		name := strings.TrimSpace(strings.Join([]string{
			strings.TrimSpace(s.FirstName),
			strings.TrimSpace(s.LastName),
		}, " "))
		if name == "" {
			name = "Unnamed Record"
		}
		lines = append(lines, previewRecordLine{
			DisplayID: strings.TrimSpace(s.DisplayID),
			Name:      name,
		})
	}
	return lines
}

func writeExportPreviewFragment(w http.ResponseWriter, view exportPreviewView) {
	var b strings.Builder
	b.WriteString(`<div class="space-y-2 text-sm text-slate-700"`)
	if view.HasError {
		b.WriteString(` data-print-config-preview-error="true"`)
	}
	b.WriteString(`>`)
	b.WriteString(`<p class="font-semibold text-[#22303d]">`)
	fmt.Fprintf(&b, "%d record", view.Count)
	if view.Count != 1 {
		b.WriteByte('s')
	}
	b.WriteString(` match this configuration.</p>`)
	if view.Note != "" {
		fmt.Fprintf(&b, `<p class="text-xs text-slate-500">%s</p>`, html.EscapeString(view.Note))
	}
	fmt.Fprintf(&b, `<p class="text-xs text-slate-500">Sort: %s</p>`, html.EscapeString(view.Sort))
	fmt.Fprintf(&b, `<p class="text-xs text-slate-500">Group By: %s</p>`, html.EscapeString(strings.Join(view.GroupBy, ", ")))
	if len(view.First) > 0 {
		b.WriteString(`<ul class="ml-5 list-disc space-y-1 text-xs text-slate-700">`)
		for _, line := range view.First {
			fmt.Fprintf(&b, `<li><strong>%s</strong> — %s</li>`,
				html.EscapeString(line.DisplayID),
				html.EscapeString(line.Name),
			)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</div>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}