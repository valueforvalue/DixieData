// research_picker_handlers.go — Research & Review Person picker handlers
// (issue #378 slice 1 + slice 2 + slice 3). Six handlers:
//
//   - handleResearchPicker    GET  /research            — render the picker
//                                                       shell. Branches on
//                                                       ?partial=1 to return
//                                                       only the results
//                                                       panel fragment
//                                                       (slice 2 option B2).
//   - handleResearchSelect    POST /research/select     — record chosen
//                                                       Person in
//                                                       dd_person_ctx cookie
//                                                       + redirect to
//                                                       sub-page.
//   - handleResearchClear     POST /research/clear      — clear the cookie
//                                                       + redirect to picker.
//   - handleResearchRecent    GET  /research/recent     — return the
//                                                       recents-list fragment
//                                                       (slice 3 option C1).
//
// Slices' responsibilities:
//
//   Slice 1 ships the bare picker shell.
//   Slice 2 adds: ?next= forwarding through the picker (the picker forms
//     carry the request's NextAction into the hidden form field); the
//     partial-fragment branch on ?partial=1; pickerContextPresent helper
//     consumed by handleSoldierByID's sub-route guard.
//   Slice 3 adds: /research/recent fragment hydrates from
//     localStorage-supplied ids via JS.
//
// The picker-guard (redirect-through-picker when no
// dd_person_ctx cookie is set) lives at the top of
// handleSoldierByID's switch on parts[1] rather than as chi
// middleware. /soldiers/* is registered as a chi catch-all so chi
// can't route by sub-path intent; the single insert site captures
// the contract for all 6 foldout sub-paths.
//
// Kept in a dedicated file (research_picker_handlers.go) so the per-soldier
// research handlers in research_handlers.go stay grouped with their
// soldier-scoped URL surface.
package appshell

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/cookies"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// researchSubPathForAction maps the picker form's `next` field to the
// existing /soldiers/{id}/* sub-route. The map keys are the only values
// accepted by handleResearchSelect; anything else returns 400 to keep
// open-redirect payloads out (the picker test fixture supplies
// "../../etc/passwd" specifically to prove the gate fires).
func researchSubPathForAction(personID int64, action, geography string) string {
	switch action {
	case "camaraderie":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/camaraderie"
	case "timeline":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/timeline"
	case "research-log":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/research-log"
	case "conflict-ledger":
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/conflict-ledger"
	case "research-pack":
		// Slice 3: the picker sub-screen asks state vs county first.
		// We forward the choice to /research-pack/{state|county} so
		// the soldier-scoped sub-page knows which geography to look
		// up. Slice 2 routes to /state as a safe default.
		scope := strings.TrimSpace(geography)
		if scope != "state" && scope != "county" {
			scope = "state"
		}
		return "/soldiers/" + strconv.FormatInt(personID, 10) + "/research-pack/" + scope
	}
	return ""
}

// supportedPickerActions returns the list of sub-page action
// names the supplied soldier can support (issue #422 slice 2).
// The order is the display order in the Continue shortcut.
// Camaraderie is conditional on the soldier having a unit; the
// rest always work (Timeline / Research Log / Conflict Ledger
// handle empty data gracefully; Research Pack falls through to
// the state sub-screen which works because PensionState
// normalizes to N/A).
func supportedPickerActions(soldier models.Soldier) []string {
	var actions []string
	if records.HasUnitForCamaraderie(soldier) {
		actions = append(actions, "camaraderie")
	}
	actions = append(actions, "timeline", "research-log", "conflict-ledger", "research-pack")
	return actions
}

// isValidResearchAction reports whether the supplied action keyword
// resolves to a known sub-route. Used as the validation gate before
// writeExportRedirect.
func isValidResearchAction(action string) bool {
	switch action {
	case "camaraderie", "timeline", "research-log", "conflict-ledger",
		"research-pack":
		return true
	}
	return false
}

func (a *App) handleResearchPicker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	nextRaw := strings.TrimSpace(q.Get("next"))
	partial := q.Get("partial") == "1"

	// Issue #378 slice 2: forward ?next=... into the viewmodel so the
	// picker echo's it as a hidden field on every result form. The
	// allowlist is the only gate; an unknown ?next= falls back to the
	// default so we never echo a payload the picker would refuse at
	// submit time.
	next := "camaraderie"
	if nextRaw != "" && isValidResearchAction(nextRaw) {
		next = nextRaw
	}

	if partial {
		a.renderResearchSearchFragment(w, query, next)
		return
	}

	var current *viewmodel.PersonRecord
	var supportedActions []string
	hasCountyInBirth := false
	if a.personCtxKey != nil {
		if ctx, ok := cookies.ReadPersonCtx(r, a.personCtxKey); ok && ctx.PersonID > 0 {
			if soldier, err := a.soldiers.GetByID(ctx.PersonID); err == nil && soldier != nil {
				rec := viewmodel.PersonRecordFromModel(*soldier)
				current = &rec
				// Issue #422 slice 2: compute which sub-pages the
				// current person can support. The Continue
				// shortcut renders one button per supported
				// action; unsupported ones are hidden entirely.
				supportedActions = supportedPickerActions(*soldier)
				hasCountyInBirth = records.HasCountyInBirth(*soldier)
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
		CurrentPerson:    current,
		SearchQuery:      query,
		SearchResults:    results,
		NextAction:       next,
		SupportedActions: supportedActions,
		HasCountyInBirth: hasCountyInBirth,
	}
	presentation.ResearchPickerView(view).Render(r.Context(), w)
}

// renderResearchSearchFragment returns only the picker results panel
// (issue #378 slice 2 B2). The htmx-driven live search swap targets
// #panel.research.picker.results so a full-page render would
// double-render the rest of the picker chrome.
func (a *App) renderResearchSearchFragment(w http.ResponseWriter, query, next string) {
	var results []viewmodel.PersonRecord
	if query != "" {
		if rows, _, err := a.soldiers.SearchPage(query, 1, 10); err == nil {
			for i := range rows {
				results = append(results, viewmodel.PersonRecordFromModel(rows[i]))
			}
		}
	}
	view := viewmodel.ResearchPickerView{
		SearchQuery:   query,
		SearchResults: results,
		NextAction:    next,
	}
	presentation.ResearchPickerSearchResults(view).Render(requestContext(w), w)
}

// requestContext returns a non-nil context derived from the request
// when one is available, else context.Background(). Templ rendering
// requires a non-nil context for hx-* attribute processing; the
// test recorder doesn't carry a real Request, so we tolerate that
// path. This helper is currently a passthrough to context.Background()
// because the picker fragment doesn't depend on request-scoped data;
// future slice-3 work that needs request data should plumb a real
// context here.
func requestContext(_ http.ResponseWriter) context.Context {
	return context.Background()
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
	geography := strings.TrimSpace(r.FormValue("geography"))
	writeExportRedirect(w, researchSubPathForAction(personID, next, geography))
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

// pickerContextPresent reports whether the supplied request carries
// a dd_person_ctx cookie that this app can verify. Returns true
// when no cookie key is configured (no-context mode pass-through)
// or when the cookie is present and verifies.
//
// Issue #378 slice 2: handleSoldierByID checks this before the
// soldier-scoped sub-route branch fires so users without a Person
// in context are routed through the picker.
func (a *App) pickerContextPresent(r *http.Request) bool {
	if a.personCtxKey == nil {
		return true
	}
	_, ok := cookies.ReadPersonCtx(r, a.personCtxKey)
	return ok
}

// handleResearchRecent is the recents-list fragment endpoint
// (issue #378 slice 3, option C1). Registered at GET /research/recent
// in routes.go. Reads ?ids=... (comma-separated Person IDs from
// localStorage) and returns the recent-persons ul fragment so
// app.js can swap it in.
//
// The handler:
//   - parses comma-separated ids from ?ids=...
//   - caps the input at 10 to match the localStorage cap
//   - forwards ?next=... into the rendered hidden fields
//   - fetches the persons via a.soldiers.ByIDs
//   - silently drops unknown ids
//   - re-iterates in the request order so the rendered ul mirrors
//     the localStorage push-to-head order
//   - renders the empty-state paragraph when 0 valid ids
func (a *App) handleResearchRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	idsRaw := strings.TrimSpace(q.Get("ids"))
	nextRaw := strings.TrimSpace(q.Get("next"))

	next := "camaraderie"
	if nextRaw != "" && isValidResearchAction(nextRaw) {
		next = nextRaw
	}

	requested := make([]int64, 0, 10)
	if idsRaw != "" {
		for _, part := range strings.Split(idsRaw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				continue
			}
			requested = append(requested, id)
			if len(requested) >= 10 {
				break
			}
		}
	}

	view := viewmodel.ResearchPickerView{
		NextAction: next,
	}
	if len(requested) == 0 {
		presentation.ResearchPickerRecent(view).Render(r.Context(), w)
		return
	}

	rows, err := a.soldiers.ByIDs(requested)
	if err != nil {
		respondInternal(w, r, "Could not load recent persons.", err)
		return
	}
	byID := make(map[int64]viewmodel.PersonRecord, len(rows))
	for i := range rows {
		byID[rows[i].ID] = viewmodel.PersonRecordFromModel(rows[i])
	}
	for _, id := range requested {
		if rec, ok := byID[id]; ok {
			view.RecentPersons = append(view.RecentPersons, rec)
		}
	}
	presentation.ResearchPickerRecent(view).Render(r.Context(), w)
}
