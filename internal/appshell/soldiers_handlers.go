// soldiers_handlers.go holds the core soldiers CRUD HTTP handlers: the
// soldiers list, search/browse, new + scrape, create, the by-ID router
// (which dispatches to edit/update sub-paths), and the /share redirect.
// Extracted from app.go as step 10 of the God-class reduction tracked
// in issue #42. Handlers stay on *App; routes registered in routes.go.
//
// The soldier-image CRUD (handleDownloadSoldierImages, handleImportSoldierImages,
// handleDeleteSoldierImages, handleSetPrimarySoldierImage), the soldier
// PDF/JPG export handlers (handleSoldierPDF, handleSoldierPDFNoImages,
// handleSoldierJPG), the parseSoldierForm + newSoldierDefaults + findAGrave
// helpers, and imageImportRedirectPath remain in app.go for now. They are
// soldier-domain but are interspersed with calendar helpers and form parsers.
// They will move to soldiers_handlers.go in a future cleanup commit.
package appshell

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/findagrave"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func (a *App) handleSoldiers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.handleCreateSoldier(w, r)
		return
	case http.MethodGet:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	suggestions, err := a.soldiers.FormSuggestions()
	if err != nil {
		respondInternal(w, r, "Could not load browse suggestions.", err)
		return
	}
	presentation.SoldierList(nil, page, 0, "", suggestions).Render(r.Context(), w)
}

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query().Get("q")
	page := parsePage(r.URL.Query().Get("page"))
	search := models.SoldierSearch{
		Mode:   "basic",
		Query:  q,
		Browse: r.URL.Query().Get("browse") == "1",
	}
	search = a.attachArchiveCounts(search)
	if strings.TrimSpace(q) == "" && !search.Browse {
		presentation.SearchResults(nil, search, page, 0, 50).Render(r.Context(), w)
		return
	}
	if strings.TrimSpace(q) == "" && search.Browse {
		soldiers, total, err := a.soldiers.List(page, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		presentation.SearchResults(soldiers, search, page, total, 50).Render(r.Context(), w)
		return
	}
	soldiers, total, err := a.soldiers.SearchPage(q, page, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	presentation.SearchResults(soldiers, search, page, total, 50).Render(r.Context(), w)
}

func (a *App) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request := parseBrowseRequest(r.URL.Query())
	suggestions, err := a.soldiers.FormSuggestions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	soldiers, total, normalized, err := a.soldiers.BrowsePage(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presentation.BrowseView(soldiers, normalized, total, suggestions).Render(r.Context(), w)
}

func (a *App) handleBrowseResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	soldiers, total, normalized, err := a.soldiers.BrowsePage(parseBrowseRequest(r.URL.Query()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presentation.BrowseResults(soldiers, normalized, total).Render(r.Context(), w)
}

func parseBrowseRequest(values url.Values) records.BrowseRequest {
	return records.BrowseRequest{
		Page:                  parsePage(values.Get("page")),
		PageSize:              parsePageSize(values.Get("page_size"), 100),
		Scope:                 values.Get("scope"),
		Sort:                  values.Get("sort"),
		EntryType:             values.Get("entry_type"),
		Unit:                  values.Get("unit"),
		BuriedIn:              values.Get("buried_in"),
		PensionState:          values.Get("pension_state"),
		ReviewStatus:          values.Get("review_status"),
		ConfederateHomeStatus: values.Get("confederate_home_status"),
	}
}

func parsePageSize(raw string, fallback int) int {
	if fallback < 1 {
		fallback = 100
	}
	size, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || size < 1 {
		return fallback
	}
	return size
}

func (a *App) handleRecentSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ids, err := parseCSVInt64s(r.URL.Query().Get("ids"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	soldiers, err := a.soldiers.RecentByIDs(ids, 10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recentSearch := a.attachArchiveCounts(models.SoldierSearch{Mode: "basic", Recent: true})
	presentation.SearchResults(soldiers, recentSearch, 1, len(soldiers), 10).Render(r.Context(), w)
}

func (a *App) handleAdvancedSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	search := models.SoldierSearch{
		Mode:                  "advanced",
		DisplayID:             r.URL.Query().Get("display_id"),
		EntryType:             r.URL.Query().Get("entry_type"),
		FirstName:             r.URL.Query().Get("first_name"),
		MiddleName:            r.URL.Query().Get("middle_name"),
		LastName:              r.URL.Query().Get("last_name"),
		MaidenName:            r.URL.Query().Get("maiden_name"),
		RelationshipLabel:     r.URL.Query().Get("relationship_label"),
		Rank:                  r.URL.Query().Get("rank"),
		RankIn:                r.URL.Query().Get("rank_in"),
		RankOut:               r.URL.Query().Get("rank_out"),
		Unit:                  r.URL.Query().Get("unit"),
		RecordType:            r.URL.Query().Get("record_type"),
		PensionState:          r.URL.Query().Get("pension_state"),
		ConfederateHomeStatus: r.URL.Query().Get("confederate_home_status"),
		ConfederateHomeName:   r.URL.Query().Get("confederate_home_name"),
		BuriedIn:              r.URL.Query().Get("buried_in"),
		ReviewStatus:          r.URL.Query().Get("review_status"),
		BirthDate:             r.URL.Query().Get("birth_date"),
		BirthYear:             r.URL.Query().Get("birth_year"),
		BirthYearTo:           r.URL.Query().Get("birth_year_to"),
		DeathDate:             r.URL.Query().Get("death_date"),
		DeathYear:             r.URL.Query().Get("death_year"),
		DeathYearTo:           r.URL.Query().Get("death_year_to"),
		DeathMonth:            r.URL.Query().Get("death_month"),
		DeathDay:              r.URL.Query().Get("death_day"),
	}
	page := parsePage(r.URL.Query().Get("page"))
	search = a.attachArchiveCounts(search)
	if !hasAdvancedSearchInput(search) {
		presentation.SearchResults(nil, search, page, 0, 50).Render(r.Context(), w)
		return
	}

	soldiers, total, err := a.soldiers.AdvancedSearch(search, page, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	presentation.SearchResults(soldiers, search, page, total, 50).Render(r.Context(), w)
}

func hasAdvancedSearchInput(search models.SoldierSearch) bool {
	return strings.TrimSpace(search.DisplayID) != "" ||
		strings.TrimSpace(search.EntryType) != "" ||
		strings.TrimSpace(search.FirstName) != "" ||
		strings.TrimSpace(search.MiddleName) != "" ||
		strings.TrimSpace(search.LastName) != "" ||
		strings.TrimSpace(search.MaidenName) != "" ||
		strings.TrimSpace(search.RelationshipLabel) != "" ||
		strings.TrimSpace(search.Rank) != "" ||
		strings.TrimSpace(search.RankIn) != "" ||
		strings.TrimSpace(search.RankOut) != "" ||
		strings.TrimSpace(search.Unit) != "" ||
		strings.TrimSpace(search.RecordType) != "" ||
		strings.TrimSpace(search.PensionState) != "" ||
		strings.TrimSpace(search.ConfederateHomeStatus) != "" ||
		strings.TrimSpace(search.ConfederateHomeName) != "" ||
		strings.TrimSpace(search.BuriedIn) != "" ||
		strings.TrimSpace(search.ReviewStatus) != "" ||
		strings.TrimSpace(search.BirthDate) != "" ||
		strings.TrimSpace(search.BirthYear) != "" ||
		strings.TrimSpace(search.BirthYearTo) != "" ||
		strings.TrimSpace(search.DeathDate) != "" ||
		strings.TrimSpace(search.DeathYear) != "" ||
		strings.TrimSpace(search.DeathYearTo) != "" ||
		strings.TrimSpace(search.DeathMonth) != "" ||
		strings.TrimSpace(search.DeathDay) != ""
}

func (a *App) handleNewSoldier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defaults, err := a.newSoldierDefaults()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.renderEntryForm(w, r, defaults, false, "", http.StatusOK)
}

func (a *App) handleScrapeFindAGrave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	defaults, err := a.newSoldierDefaults()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	scrape := models.FindAGraveScrapeState{Input: strings.TrimSpace(r.FormValue("findagrave_source"))}
	result, err := findagrave.ParseInput(r.Context(), scrape.Input)
	if err != nil {
		scrape.ErrorMessage = err.Error()
		a.renderEntryFormWithScrapeState(w, r, defaults, false, "", scrape, http.StatusBadRequest, true)
		return
	}

	scrape.SourceLabel = result.SourceLabel
	scrape.WarningLines = result.Warnings
	scrape.Spouses = result.Spouses
	scrape.ConfidenceScore = result.ConfidenceScore
	autofilled := applyFindAGraveAutofill(defaults, result)
	a.renderEntryFormWithScrapeState(w, r, autofilled, false, "", scrape, http.StatusOK, true)
}

func (a *App) handleCreateSoldier(w http.ResponseWriter, r *http.Request) {
	s, err := parseSoldierForm(r, 0)
	if err != nil {
		defaults, defaultsErr := a.newSoldierDefaults()
		if defaultsErr != nil {
			http.Error(w, defaultsErr.Error(), http.StatusInternalServerError)
			return
		}
		a.renderEntryForm(w, r, defaults, false, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := a.soldiers.Create(s)
	if err != nil {
		a.renderEntryForm(w, r, s, false, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.saveUploadedImages(r, *created); err != nil {
		reloaded, reloadErr := a.soldiers.GetByID(created.ID)
		if reloadErr != nil {
			reloaded = created
		}
		a.renderEntryForm(w, r, *reloaded, true, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/soldiers/%d", created.ID), http.StatusSeeOther)
}

func (a *App) handleSoldierByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/soldiers/")
	parts := strings.Split(path, "/")

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}

	if len(parts) > 1 && parts[1] == "edit" {
		a.handleEditSoldier(w, r, id)
		return
	}
	if len(parts) > 1 && parts[1] == "timeline" {
		a.handleServiceTimeline(w, r, id)
		return
	}
	if len(parts) > 1 && parts[1] == "research-log" {
		a.handleResearchLog(w, r, id, parts[1:])
		return
	}
	if len(parts) > 1 && parts[1] == "conflict-ledger" {
		a.handleConflictLedger(w, r, id)
		return
	}
	if len(parts) > 2 && parts[1] == "research-pack" {
		a.handleResearchPack(w, r, id, parts[2])
		return
	}
	if len(parts) > 1 && parts[1] == "camaraderie" {
		a.handleUnitCamaraderie(w, r, id)
		return
	}
	if len(parts) > 2 && parts[1] == "pdf" && parts[2] == "no-images" {
		a.handleSoldierPDFNoImages(w, r, id)
		return
	}
	if len(parts) > 1 && parts[1] == "jpg" {
		a.handleSoldierJPG(w, r, id)
		return
	}
	if len(parts) > 1 && parts[1] == "pdf" {
		a.handleSoldierPDF(w, r, id)
		return
	}
	if len(parts) > 2 && parts[1] == "images" && parts[2] == "download" {
		a.handleDownloadSoldierImages(w, r, id)
		return
	}
	if len(parts) > 2 && parts[1] == "images" && parts[2] == "import" {
		a.handleImportSoldierImages(w, r, id)
		return
	}
	if len(parts) > 2 && parts[1] == "images" && parts[2] == "delete" {
		a.handleDeleteSoldierImages(w, r, id)
		return
	}
	if len(parts) > 3 && parts[1] == "images" && parts[2] == "primary" {
		imageID, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			http.Error(w, "invalid image id", http.StatusBadRequest)
			return
		}
		a.handleSetPrimarySoldierImage(w, r, id, imageID)
		return
	}
	if len(parts) > 2 && parts[1] == "review" && parts[2] == "resolve" {
		a.handleResolveReviewStatus(w, r, id)
		return
	}
	if len(parts) > 2 && parts[1] == "review" && parts[2] == "flag" {
		a.handleFlagReviewStatus(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		soldier, err := a.soldiers.GetByID(id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		if err := a.attachDetailBackLink(soldier, strings.TrimSpace(r.URL.Query().Get("from"))); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		presentation.SoldierDetail(*soldier).Render(r.Context(), w)
	case http.MethodPut:
		a.handleUpdateSoldier(w, r, id)
	case http.MethodDelete:
		if err := a.soldiers.Delete(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		http.Redirect(w, r, "/soldiers", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (a *App) handleEditSoldier(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	a.renderEntryForm(w, r, *soldier, true, "", http.StatusOK)
}

func (a *App) handleUpdateSoldier(w http.ResponseWriter, r *http.Request, id int64) {
	s, err := parseSoldierForm(r, id)
	if err != nil {
		a.renderEntryForm(w, r, models.Soldier{ID: id}, true, err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.soldiers.Update(s); err != nil {
		a.renderEntryForm(w, r, s, true, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.saveUploadedImages(r, s); err != nil {
		reloaded, reloadErr := a.soldiers.GetByID(id)
		if reloadErr != nil {
			reloaded = &s
		}
		a.renderEntryForm(w, r, *reloaded, true, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/soldiers/%d", id), http.StatusSeeOther)
}

// attachArchiveCounts fills the IsArchiveEmpty and TotalRecordCount fields
// on a SoldierSearch so the search results template can render the first-

// run Setup card. If the count query fails, the search proceeds with the
// empty state populated as best-effort. Tracking: issue #98 from the
// 2026-06-24 audit.
func (a *App) attachArchiveCounts(search models.SoldierSearch) models.SoldierSearch {
	counts, err := a.soldiers.ArchiveCounts()
	if err != nil {
		return viewmodel.WithArchiveCounts(search, models.ArchiveCounts{})
	}
	return viewmodel.WithArchiveCounts(search, counts)
}
