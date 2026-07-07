// research_picker_handlers.go — Research & Review Person picker handlers
// (issue #378 slice 1). Three handlers:
//
//   - handleResearchPicker    GET  /research        — render the picker shell
//   - handleResearchSelect    POST /research/select — record chosen Person in
//                                                     dd_person_ctx cookie +
//                                                     redirect to sub-page
//   - handleResearchClear     POST /research/clear  — clear the cookie +
//                                                     redirect to picker
//
// Slice 1 ships the bare picker shell: search input, "Continue" shortcut
// when a cookie is set, recents list (empty in slice 1; slice 3 lifts
// persistence to localStorage), results region (empty in slice 1; slice 2
// swaps in live htmx-driven results).
//
// The three handlers share a small allowlist of "next" actions (camaraderie,
// timeline, research-log, conflict-ledger, research-pack/state|county)
// that map to existing /soldiers/{id}/* sub-routes. The allowlist is
// the gate that protects against open-redirect payloads (issue #378
// "Decisions to confirm Q1"): an attacker-supplied next value can never
// reach handleResearchSelect's redirect target unless it appears in the
// allowlist, which keeps the X-DixieData-Redirect path safe.
//
// Kept in a dedicated file (research_picker_handlers.go) so the per-soldier
// research handlers in research_handlers.go stay grouped with their
// soldier-scoped URL surface.
package appshell

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/cookies"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// researchSubPathForAction maps the picker form's `next` field to the
// existing /soldiers/{id}/* sub-route. The map keys are the only values
// accepted by handleResearchSelect; anything else returns 400 to keep
// open-redirect payloads out (the picker test fixture supplies
// "../../etc/passwd" specifically to prove the gate fires).
func researchSubPathForAction(personID int64, action string) string {
	switch action {
	case "camaraderie":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/camaraderie"
	case "timeline":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/timeline"
	case "research-log":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/research-log"
	case "conflict-ledger":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/conflict-ledger"
	case "research-pack-state":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/research-pack/state"
	case "research-pack-county":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/research-pack/county"
	}
	return ""
}

// isValidResearchAction reports whether the supplied action keyword
// resolves to a known sub-route. Used as the validation gate before
// writeExportRedirect.
func isValidResearchAction(action string) bool {
	switch action {
	case "camaraderie", "timeline", "research-log", "conflict-ledger",
		"research-pack-state", "research-pack-county":
		return true
	}
	return false
}

func (a *App) handleResearchPicker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var current *viewmodel.PersonRecord
	if a.personCtxKey != nil {
		if ctx, ok := cookies.ReadPersonCtx(r, a.personCtxKey); ok && ctx.PersonID > 0 {
			if soldier, err := a.soldiers.GetByID(ctx.PersonID); err == nil && soldier != nil {
				rec := viewmodel.PersonRecordFromModel(*soldier)
				current = &rec
			}
		}
	}

	var results []viewmodel.PersonRecord
	if query != "" {
		if rows, _, err := a.soldiers.SearchPage(query, 1, 10); err == nil {
			for i := range rows {
				results = append(results, viewmodel.PersonRecordFromModel(rows[i]))
			}
		}
	}

	view := viewmodel.ResearchPickerView{
		CurrentPerson: current,
		SearchQuery:   query,
		SearchResults: results,
	}
	presentation.ResearchPickerView(view).Render(r.Context(), w)
}

func (a *App) handleResearchSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the picker form.", err)
		return
	}
	personID, err := parseOptionalInt64(r.FormValue("person_id"), "person_id")
	if err != nil || personID <= 0 {
		respondValidation(w, r, "Please pick a Person Record.", err)
		return
	}
	next := strings.TrimSpace(r.FormValue("next"))
	if !isValidResearchAction(next) {
		respondValidation(w, r, "Unknown research destination.", nil)
		return
	}

	if a.personCtxKey != nil {
		if err := cookies.WritePersonCtxFromRequest(w, a.personCtxKey, r, personID); err != nil {
			respondInternal(w, r, "Could not record the Person selection.", err)
			return
		}
	}
	setToastHeader(w, "Person selection recorded.")
	writeExportRedirect(w, researchSubPathForAction(personID, next))
}

func (a *App) handleResearchClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cookies.ClearPersonCtx(w)
	setToastHeader(w, "Person selection cleared.")
	writeExportRedirect(w, "/research")
}