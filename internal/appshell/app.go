package appshell

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"
	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/findagrave"
	"github.com/valueforvalue/DixieData/internal/integrations"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/scratchpad"
	"github.com/valueforvalue/DixieData/internal/update"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed quotes.json
var embeddedQuotes []byte

type App struct {
	ctx                     context.Context
	database                *db.DB
	soldiers                personRecordsFacade
	anniversary             anniversaryFacade
	calendar                calendarFacade
	analytics               analyticsFacade
	audit                   reviewFacade
	images                  imageFacade
	export                  exportFacade
	backup                  backupFacade
	diagnostics             diagnosticsFacade
	google                  integrationFacade
	updater                 updaterFacade
	restorePoints           *update.RestorePointManager
	quotes                  []models.Quote
	mux                     *http.ServeMux
	startupErr              error
	setupRequired           bool
	pendingLaunchStateClear bool
	pendingRecovery         *update.RestorePointRecord
	recoveryFailure         string
	dataDir                 string
	scratchpads             scratchpadOpener
	frontendAssets          fs.FS
	previewMu               sync.Mutex
	memorialPreview         map[string]string
}

func shouldAttemptPostUpdateHealthClear(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet || r.URL == nil {
		return false
	}
	return isPostUpdateHealthTrustPath(r.URL.Path)
}

func isPostUpdateHealthTrustPath(path string) bool {
	switch {
	case path == "/", path == "/calendar":
		return true
	case strings.HasPrefix(path, "/browse"):
		return true
	case strings.HasPrefix(path, "/settings"):
		return true
	case strings.HasPrefix(path, "/insights"):
		return true
	case strings.HasPrefix(path, "/soldiers"):
		return true
	case strings.HasPrefix(path, "/review-queue"):
		return true
	case strings.HasPrefix(path, "/research-collections"):
		return true
	case strings.HasPrefix(path, "/export"):
		return true
	case strings.HasPrefix(path, "/compare"):
		return true
	default:
		return false
	}
}

func (a *App) clearPendingLaunchState() error {
	if !a.pendingLaunchStateClear {
		return nil
	}
	if a.restorePoints == nil {
		return fmt.Errorf("restore point manager unavailable")
	}
	if err := a.restorePoints.ClearLaunchState(); err != nil {
		return fmt.Errorf("failed to clear restore point launch state: %w", err)
	}
	a.pendingLaunchStateClear = false
	return nil
}

func (a *App) handleUpdateBootstrapHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.pendingLaunchStateClear {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if a.restorePoints == nil {
		http.Error(w, "restore point manager unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := a.clearPendingLaunchState(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type scratchpadOpener interface {
	Open(displayID, seed string) error
}

const initializeDataConfirmationWord = "INITIALIZE"

func renderStartupPlaceholder(w http.ResponseWriter, r *http.Request) {
	target := "/calendar"
	if r != nil && r.URL != nil {
		if requestPath := strings.TrimSpace(r.URL.RequestURI()); requestPath != "" && requestPath != "/" {
			target = requestPath
		}
	}
	retryTarget := startupPlaceholderRetryTarget(target)
	targetJS, err := json.Marshal(retryTarget)
	if err != nil {
		targetJS = []byte(`"/calendar?_dd_boot=1"`)
	}
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Refresh", fmt.Sprintf("1; url=%s", retryTarget))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="1;url=%s">
<meta http-equiv="cache-control" content="no-cache, no-store, must-revalidate">
<meta http-equiv="pragma" content="no-cache">
<meta http-equiv="expires" content="0">
<title>Loading DixieData...</title>
</head>
<body hx-get="%s" hx-trigger="load delay:700ms" hx-target="body" hx-swap="outerHTML" class="min-h-screen" style="background: linear-gradient(180deg, #d7d2c9 0%%, #c9c2b5 42%%, #b9b1a3 100%%);">
<div class="flex min-h-screen items-center justify-center px-6">
  <div class="rounded-3xl border border-[#8d7440] bg-[rgba(36,48,61,0.92)] px-8 py-6 shadow-[0_18px_34px_rgba(21,29,38,0.2)]">
    <p class="mb-2 text-sm uppercase tracking-[0.24em] text-[#cfb77a]">Local Archive</p>
    <p class="text-2xl font-semibold text-[#f2ede1]">Loading DixieData...</p>
    <p class="mt-2 text-sm text-[#d8cfbc]">The local archive is still starting up. This screen will refresh automatically.</p>
  </div>
</div>
<script>
window.setTimeout(function() {
  window.location.replace(%s);
}, 700);
</script>
</body>
</html>`, html.EscapeString(retryTarget), html.EscapeString(retryTarget), string(targetJS))
}

func startupPlaceholderRetryTarget(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return "/calendar?_dd_boot=1"
	}
	query := parsed.Query()
	query.Set("_dd_boot", "1")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func setupRequestAllowed(path string) bool {
	switch {
	case path == "/setup":
		return true
	case path == "/version":
		return true
	case path == "/app.js":
		return true
	case path == "/app.css":
		return true
	case strings.HasPrefix(path, "/wailsjs/"):
		return true
	default:
		return false
	}
}

func recoveryRequestAllowed(path string) bool {
	switch {
	case path == "/recovery":
		return true
	case path == "/version":
		return true
	case path == "/app.js":
		return true
	case path == "/app.css":
		return true
	default:
		return false
	}
}

func requestMethodOverride(r *http.Request) string {
	if r == nil || r.Method != http.MethodPost {
		return ""
	}
	if override := normalizedMethodOverride(r.Header.Get("X-HTTP-Method-Override")); override != "" {
		return override
	}
	if err := parseRequestFormForOverride(r); err == nil {
		if override := normalizedMethodOverride(r.FormValue("_method")); override != "" {
			return override
		}
	}
	return ""
}

func parseRequestFormForOverride(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return r.ParseMultipartForm(64 << 20)
	}
	return r.ParseForm()
}

func normalizedMethodOverride(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case http.MethodPut:
		return http.MethodPut
	case http.MethodDelete:
		return http.MethodDelete
	case http.MethodPatch:
		return http.MethodPatch
	default:
		return ""
	}
}

func (a *App) handleCalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/calendar" {
		http.NotFound(w, r)
		return
	}
	month := int(time.Now().Month())
	summary, err := a.calendar.GetMonthSummary(month)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	counts, err := a.soldiers.ArchiveCounts()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	presentation.Calendar(month, summary, counts, selectQuoteForArchive(a.quotes, counts.TotalSoldiers)).Render(r.Context(), w)
}

func (a *App) handleInitialSetup(w http.ResponseWriter, r *http.Request) {
	if !a.setupRequired {
		http.Redirect(w, r, "/calendar", http.StatusSeeOther)
		return
	}
	switch r.Method {
	case http.MethodGet:
		presentation.InitialSetupView(models.InitialSetupForm{}).Render(r.Context(), w)
	case http.MethodPost:
		form, birthYear, err := parseInitialSetupForm(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			form.ErrorMessage = err.Error()
			presentation.InitialSetupView(form).Render(r.Context(), w)
			return
		}
		_, err = a.database.ConfigureUserIdentity(form.FirstName, form.MiddleName, form.LastName, birthYear)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			form.ErrorMessage = err.Error()
			presentation.InitialSetupView(form).Render(r.Context(), w)
			return
		}
		a.setupRequired = false
		if err := a.reloadServices(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/calendar", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"app":"%s","version":"%s","schema_version":%d,"build_identity":%q}`+"\n",
		buildinfo.AppName,
		buildinfo.AppVersion,
		buildinfo.SchemaVersion,
		buildinfo.BuildIdentity(),
	)
}

func (a *App) handleCalendarMonth(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/calendar/"), "/")
	if len(parts) > 1 && parts[1] == "grid" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.handleCalendarGrid(w, r, parts[0])
		return
	}
	if len(parts) > 2 && parts[1] == "report" && parts[2] == "pdf" {
		a.handleCalendarPDF(w, r, parts[0])
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	month, err := parseBoundedInt(parts[0], "month", 1, 12)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	summary, err := a.calendar.GetMonthSummary(month)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	counts, err := a.soldiers.ArchiveCounts()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	presentation.Calendar(month, summary, counts, selectQuoteForArchive(a.quotes, counts.TotalSoldiers)).Render(r.Context(), w)
}

func (a *App) handleCalendarGrid(w http.ResponseWriter, r *http.Request, monthValue string) {
	month, err := parseBoundedInt(monthValue, "month", 1, 12)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	summary, err := a.calendar.GetMonthSummary(month)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presentation.CalendarGrid(month, summary).Render(r.Context(), w)
}

func (a *App) handleAnniversary(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/anniversary/"), "/")
	if len(parts) < 2 {
		http.Error(w, "bad request", 400)
		return
	}
	month, err := parseBoundedInt(parts[0], "month", 1, 12)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	day, err := parseBoundedInt(parts[1], "day", 0, 31)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		editID, err := parseOptionalInt64(r.URL.Query().Get("edit"), "edit")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.renderCalendarDayDetail(w, r, month, day, editID, "", "", "", "", "", "", http.StatusOK)
		return
	}
	if len(parts) == 3 && parts[2] == "items" && r.Method == http.MethodPost {
		a.handleCreateCalendarItem(w, r, month, day)
		return
	}
	if len(parts) == 4 && parts[2] == "items" {
		itemID, err := parseOptionalInt64(parts[3], "item_id")
		if err != nil || itemID <= 0 {
			http.Error(w, "invalid item_id", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			a.handleUpdateCalendarItem(w, r, month, day, itemID)
			return
		case http.MethodDelete:
			a.handleDeleteCalendarItem(w, r, month, day, itemID)
			return
		}
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	presentation.SearchResults(soldiers, models.SoldierSearch{Mode: "basic", Recent: true}, 1, len(soldiers), 10).Render(r.Context(), w)
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

func (a *App) handleUnitCamaraderie(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	graph, err := a.soldiers.UnitCamaraderieGraph(id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	presentation.UnitCamaraderieView(*graph).Render(r.Context(), w)
}

func (a *App) handleServiceTimeline(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	timeline, err := a.soldiers.ServiceTimeline(id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	presentation.ServiceTimelineView(*timeline).Render(r.Context(), w)
}

func (a *App) handleResearchLog(w http.ResponseWriter, r *http.Request, id int64, parts []string) {
	if len(parts) == 1 && r.Method == http.MethodGet {
		log, err := a.soldiers.ResearchLog(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		presentation.ResearchLogView(*log).Render(r.Context(), w)
		return
	}
	if len(parts) == 2 && parts[1] == "tasks" && r.Method == http.MethodPost {
		a.handleResearchTaskCreate(w, r, id)
		return
	}
	if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "resolve" && r.Method == http.MethodPost {
		taskID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			http.Error(w, "invalid research task id", http.StatusBadRequest)
			return
		}
		a.handleResearchTaskResolve(w, r, id, taskID)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (a *App) handleResearchTaskCreate(w http.ResponseWriter, r *http.Request, id int64) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	evidenceType := strings.TrimSpace(r.FormValue("evidence_type"))
	if err := a.soldiers.AddResearchTask(id, title, notes, evidenceType); err != nil {
		setToastHeaderWithType(w, "Research task could not be saved.", "error")
		fmt.Fprintf(w, "Research task could not be saved: %v", err)
		return
	}
	setToastHeader(w, "Success: research task added.")
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d/research-log", id))
	fmt.Fprint(w, "Research task saved.")
}

func (a *App) handleResearchTaskResolve(w http.ResponseWriter, r *http.Request, id, taskID int64) {
	if err := a.soldiers.ResolveResearchTask(id, taskID); err != nil {
		setToastHeaderWithType(w, "Research task could not be resolved.", "error")
		fmt.Fprintf(w, "Research task could not be resolved: %v", err)
		return
	}
	setToastHeader(w, "Success: research task resolved.")
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d/research-log", id))
	fmt.Fprint(w, "Research task resolved.")
}

func (a *App) handleConflictLedger(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ledger, err := a.backup.ConflictLedger(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	presentation.MergeReviewLedgerView(*ledger).Render(r.Context(), w)
}

func (a *App) handleResearchPack(w http.ResponseWriter, r *http.Request, id int64, scope string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pack, err := a.soldiers.ResearchPackForPersonRecord(id, scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	presentation.ResearchPackView(*pack).Render(r.Context(), w)
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

func (a *App) handleLegacyExportRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/share", http.StatusSeeOther)
}

func (a *App) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := a.google.Status()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	conflicts, err := a.backup.PendingMergeConflicts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	exportRecords, err := a.listAllSoldiers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	drift, err := a.google.CalendarDriftStatus(exportRecords)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status.LastSyncedAt = drift.LastSyncedAt
	status.DriftAdded = drift.Added
	status.DriftUpdated = drift.Updated
	status.DriftRemoved = drift.Removed
	status.OutOfSync = drift.OutOfSync
	presentation.ShareView(status, conflicts, exportRecords).Render(r.Context(), w)
}

func (a *App) handleResearchCollections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		fromID, err := parseOptionalInt64(r.URL.Query().Get("from"), "from")
		if err != nil {
			http.Error(w, "invalid from id", http.StatusBadRequest)
			return
		}
		hub, err := a.soldiers.ResearchCollectionsHub(fromID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		presentation.ResearchCollectionsHubView(*hub).Render(r.Context(), w)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}
		if err := a.soldiers.CreateResearchCollection(r.FormValue("name"), r.FormValue("description")); err != nil {
			setToastHeaderWithType(w, "Collection could not be created.", "error")
			fmt.Fprintf(w, "Collection could not be created: %v", err)
			return
		}
		redirectTo := "/research-collections"
		if fromID, err := parseOptionalInt64(r.FormValue("from"), "from"); err == nil && fromID > 0 {
			redirectTo = fmt.Sprintf("/research-collections?from=%d", fromID)
		}
		setToastHeader(w, "Success: research collection created.")
		w.Header().Set("X-DixieData-Redirect", redirectTo)
		fmt.Fprint(w, "Collection created.")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleResearchCollectionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/research-collections/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	collectionID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid collection id", http.StatusBadRequest)
		return
	}
	if len(parts) == 2 && parts[1] == "add" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}
		soldierID, err := parseOptionalInt64(r.FormValue("soldier_id"), "soldier_id")
		if err != nil || soldierID < 1 {
			http.Error(w, "invalid soldier id", http.StatusBadRequest)
			return
		}
		if err := a.soldiers.AddPersonRecordToResearchCollection(collectionID, soldierID); err != nil {
			setToastHeaderWithType(w, "Record could not be added to the collection.", "error")
			fmt.Fprintf(w, "Record could not be added to the collection: %v", err)
			return
		}
		redirectTo := fmt.Sprintf("/research-collections/%d", collectionID)
		if fromID, err := parseOptionalInt64(r.FormValue("from"), "from"); err == nil && fromID > 0 {
			redirectTo = fmt.Sprintf("/research-collections/%d?from=%d", collectionID, fromID)
		}
		setToastHeader(w, "Success: record added to collection.")
		w.Header().Set("X-DixieData-Redirect", redirectTo)
		fmt.Fprint(w, "Record added to collection.")
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		fromID, err := parseOptionalInt64(r.URL.Query().Get("from"), "from")
		if err != nil {
			http.Error(w, "invalid from id", http.StatusBadRequest)
			return
		}
		detail, err := a.soldiers.ResearchCollectionDetail(collectionID, fromID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		presentation.ResearchCollectionDetailView(*detail).Render(r.Context(), w)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (a *App) handleReviewQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page := parsePage(r.URL.Query().Get("page"))
	soldiers, total, err := a.soldiers.ReviewQueue(page, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	soldierIDs := make([]int64, 0, len(soldiers))
	for _, soldier := range soldiers {
		soldierIDs = append(soldierIDs, soldier.ID)
	}
	findings, err := a.audit.FindingsForPersonRecords(soldierIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presentation.ReviewQueueView(soldiers, findings, page, total, 50).Render(r.Context(), w)
}

func (a *App) handleReviewQueueBulk(w http.ResponseWriter, r *http.Request) {
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
		setToastHeaderWithType(w, "Select at least one review record first.", "warning")
		fmt.Fprint(w, "Select at least one review record first.")
		return
	}
	action := strings.ToLower(strings.TrimSpace(r.FormValue("bulk_action")))
	switch action {
	case "ignore":
		for _, soldierID := range selected {
			if err := a.soldiers.MarkReviewResolved(soldierID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := a.audit.ResolveFindingsForPersonRecord(soldierID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		setToastHeader(w, fmt.Sprintf("Resolved %d review queue item(s).", len(selected)))
	case "delete":
		for _, soldierID := range selected {
			if err := a.audit.ResolveFindingsForPersonRecord(soldierID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := a.soldiers.Delete(soldierID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		setToastHeaderWithType(w, fmt.Sprintf("Deleted %d review queue record(s).", len(selected)), "success")
	default:
		http.Error(w, "invalid bulk action", http.StatusBadRequest)
		return
	}
	w.Header().Set("X-DixieData-Redirect", "/review-queue")
	fmt.Fprint(w, "Review queue updated.")
}

func (a *App) handleInsights(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot, err := a.analytics.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presentation.InsightsView(snapshot).Render(r.Context(), w)
}

func (a *App) handleInsightsDrilldown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	value := strings.TrimSpace(r.URL.Query().Get("value"))
	page := parsePage(r.URL.Query().Get("page"))

	title, description, search, useGroupedSpouseQuery, err := insightDrilldownConfig(scope, value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var (
		soldiers []models.Soldier
		total    int
	)
	if useGroupedSpouseQuery {
		soldiers, total, err = a.soldiers.ListByEntryTypes([]string{"wife", "widow"}, page, 50)
	} else {
		soldiers, total, err = a.soldiers.AdvancedSearch(search, page, 50)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	presentation.InsightsDrilldownView(title, description, soldiers, search, page, total, 50, scope, value).Render(r.Context(), w)
}

func insightDrilldownConfig(scope, value string) (string, string, models.SoldierSearch, bool, error) {
	search := models.SoldierSearch{Mode: "advanced"}
	switch scope {
	case "entry_type":
		switch strings.ToLower(value) {
		case "soldier":
			search.EntryType = "soldier"
			return "Soldier Records", "Records included in the Insights soldier total.", search, false, nil
		case "linked_person":
			search.EntryType = "linked_person"
			return "Person Records", "Records included in the Insights person-record total.", search, false, nil
		case "spouse":
			return "Spouse Records", "Wife and widow records included in the Insights spouse total.", search, true, nil
		}
	case "buried_in":
		search.BuriedIn = value
		return "Burial Drilldown", fmt.Sprintf("Records buried in %s.", value), search, false, nil
	case "confederate_home_status":
		search.ConfederateHomeStatus = value
		return "Confederate Home Status", fmt.Sprintf("Records with Confederate Home status set to %s.", value), search, false, nil
	case "confederate_home_name":
		search.ConfederateHomeName = value
		return "Confederate Home Name", fmt.Sprintf("Records tied to %s.", value), search, false, nil
	case "pension_state":
		search.PensionState = value
		return "Pension State", fmt.Sprintf("Records with pension state %s.", value), search, false, nil
	case "unit":
		search.Unit = value
		return "Unit Drilldown", fmt.Sprintf("Records tied to %s.", value), search, false, nil
	case "birth_decade":
		decade, err := insightDecadeValue(value)
		if err != nil {
			return "", "", models.SoldierSearch{}, false, err
		}
		search.BirthYear = fmt.Sprintf("%d", decade)
		search.BirthYearTo = fmt.Sprintf("%d", decade+9)
		return "Birth Decade Drilldown", fmt.Sprintf("Records with birth years in the %ds.", decade), search, false, nil
	case "death_decade":
		decade, err := insightDecadeValue(value)
		if err != nil {
			return "", "", models.SoldierSearch{}, false, err
		}
		search.DeathYear = fmt.Sprintf("%d", decade)
		search.DeathYearTo = fmt.Sprintf("%d", decade+9)
		return "Death Decade Drilldown", fmt.Sprintf("Records with death years in the %ds.", decade), search, false, nil
	case "review_status":
		search.ReviewStatus = value
		return "Review Queue Drilldown", "Records currently flagged for review from the duplicate audit and research queue.", search, false, nil
	}
	return "", "", models.SoldierSearch{}, false, fmt.Errorf("unknown insight drilldown")
}

func insightDecadeValue(value string) (int, error) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "s"))
	if len(trimmed) != 4 {
		return 0, fmt.Errorf("invalid decade")
	}
	decade, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid decade")
	}
	return decade, nil
}

func (a *App) handleRunDuplicateAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.audit.RunDuplicateAudit()
	if err != nil {
		setToastHeaderWithType(w, "Duplicate audit failed.", "error")
		fmt.Fprintf(w, "Duplicate audit failed: %v", err)
		return
	}
	message := fmt.Sprintf("Success: scanned %d records and found %d candidate duplicate pairs (%d suppressed by prior resolutions).", result.ScannedRecords, result.FindingsDiscovered, result.FindingsSuppressed)
	setToastHeader(w, message)
	fmt.Fprintf(w, `<div class="rounded-2xl border border-[rgba(141,116,64,0.35)] bg-white/70 px-4 py-3 text-sm text-slate-600">Scanned <strong>%d</strong> records. <strong>%d</strong> candidate duplicate pairs remain open in the Review Queue.</div>`, result.ScannedRecords, result.OpenFindings)
}

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

func (a *App) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprintf(w, "Export cancelled.")
		return
	}
	if err := a.export.ExportJSON(path); err != nil {
		fmt.Fprintf(w, "Export failed: %v", err)
		return
	}
	fmt.Fprintf(w, "✓ Exported to %s", path)
}

func (a *App) handleExportInsightsPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	snapshot, err := a.analytics.Snapshot()
	if err != nil {
		fmt.Fprintf(w, "Analytics export failed: %v", err)
		return
	}
	options := parsePDFOptionsRequest(r, "P", false)
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: pdfReportName("dixiedata-archive-insights", options, false),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Analytics export cancelled.")
		return
	}
	if err := a.export.ExportAnalyticsSummaryPDF(path, snapshot, options); err != nil {
		fmt.Fprintf(w, "Analytics export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("Analytics report ready:", path))
}

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.xlsx",
		Filters: []runtime.FileFilter{
			{DisplayName: "Excel workbook", Pattern: "*.xlsx"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprintf(w, "Export cancelled.")
		return
	}
	if err := a.export.ExportExcel(path); err != nil {
		fmt.Fprintf(w, "Export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("Excel workbook ready:", path))
}

func (a *App) handleExportICalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-anniversaries.ics",
		Filters: []runtime.FileFilter{
			{DisplayName: "iCalendar", Pattern: "*.ics"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "iCalendar export cancelled.")
		return
	}
	preferences, err := a.google.ManagedEventPreferences()
	if err != nil {
		fmt.Fprintf(w, "iCalendar export failed: %v", err)
		return
	}
	if err := a.export.ExportICalendar(path, preferences); err != nil {
		fmt.Fprintf(w, "iCalendar export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("iCalendar ready:", path))
}

func (a *App) handleExportStaticArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defaultName := "DixieData_Archive.zip"
	if suggested, err := a.export.StaticArchiveFileName(time.Now()); err == nil {
		defaultName = suggested
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP archive", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Static web archive export cancelled.")
		return
	}
	if err := a.export.ExportStaticArchive(path, a.dataDir); err != nil {
		fmt.Fprintf(w, "Static web archive export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("Static web archive ready:", path))
}

func (a *App) handleExportDatabasePDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := parsePrintSettingsRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	message, err := a.ExportFullDatabasePDF(settings)
	if err != nil {
		fmt.Fprintf(w, "Printable PDF export failed: %v", err)
		return
	}
	fmt.Fprint(w, message)
}

func (a *App) ExportFullDatabasePDF(settings archive.PrintSettings) (string, error) {
	settings = settings.Normalize()
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: printableArchivePDFName(settings),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		return "Printable PDF export cancelled.", nil
	}
	if err := a.export.ExportFullDatabasePDF(path, settings); err != nil {
		return "", err
	}
	return exportLinkMarkup("Printable PDF ready:", path), nil
}

func parsePrintSettingsRequest(r *http.Request) (archive.PrintSettings, error) {
	if err := r.ParseForm(); err != nil {
		return archive.PrintSettings{}, fmt.Errorf("failed to parse print settings")
	}
	selectedIDs, err := parseSelectedSoldierIDs(r.Form["selected_ids"])
	if err != nil {
		return archive.PrintSettings{}, err
	}
	settings := archive.PrintSettings{
		Scope:                        strings.TrimSpace(r.FormValue("scope")),
		Orientation:                  strings.TrimSpace(r.FormValue("orientation")),
		PrinterFriendly:              r.FormValue("printer_friendly") != "",
		FullBiographyPage:            r.FormValue("full_biography_page") != "",
		SortBy:                       strings.TrimSpace(r.FormValue("sort_by")),
		GroupByUnit:                  r.FormValue("group_by_unit") != "",
		GroupByPensionState:          r.FormValue("group_by_pension_state") != "",
		GroupByConfederateHomeStatus: r.FormValue("group_by_confederate_home_status") != "",
		GroupByBuriedIn:              r.FormValue("group_by_buried_in") != "",
		FilterBuriedIn:               append([]string(nil), r.Form["filter_buried_in"]...),
		FilterEntryTypes:             append([]string(nil), r.Form["filter_entry_type"]...),
		FilterUnits:                  append([]string(nil), r.Form["filter_unit"]...),
		FilterPensionStates:          append([]string(nil), r.Form["filter_pension_state"]...),
		FilterConfederateHomeStatus:  append([]string(nil), r.Form["filter_confederate_home_status"]...),
		ExportAll:                    r.FormValue("export_all") != "",
		SelectedIDs:                  selectedIDs,
	}.Normalize()
	if settings.Scope == archive.PrintScopeSelected && len(settings.SelectedIDs) == 0 {
		return archive.PrintSettings{}, fmt.Errorf("select at least one record or choose a different export scope")
	}
	return settings, nil
}

func parsePDFOptionsRequest(r *http.Request, defaultOrientation string, defaultIncludeImages bool) archive.PDFOptions {
	options := archive.PDFOptions{
		Orientation:     strings.TrimSpace(r.FormValue("orientation")),
		PrinterFriendly: r.FormValue("printer_friendly") != "",
		IncludeImages:   parseBoolFormValueDefault(r.Form, "include_images", defaultIncludeImages),
	}
	return options.Normalize(defaultOrientation, defaultIncludeImages)
}

func parseBoolFormValueDefault(values url.Values, key string, fallback bool) bool {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	for _, value := range raw {
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "1", "true", "on", "yes":
			return true
		case "0", "false", "off", "no", "":
			return false
		}
	}
	return fallback
}

func setToastHeader(w http.ResponseWriter, message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	w.Header().Set("X-DixieData-Toast", message)
	w.Header().Set("X-DixieData-Toast-Type", "success")
}

func setToastHeaderWithType(w http.ResponseWriter, message, kind string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	if strings.TrimSpace(kind) == "" {
		kind = "success"
	}
	w.Header().Set("X-DixieData-Toast", message)
	w.Header().Set("X-DixieData-Toast-Type", kind)
}

func (a *App) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: backupArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Backup export cancelled.")
		return
	}

	manifest, err := a.backup.Export(path, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Backup export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup(fmt.Sprintf("Backup ready (%d soldiers, %d images):", manifest.Soldiers, manifest.Images), path))
}

func (a *App) handleExportSharedArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: sharedArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Shared archive export cancelled.")
		return
	}

	manifest, err := a.backup.ExportShared(path, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Shared archive export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup(fmt.Sprintf("Shared archive ready (%d soldiers, %d images):", manifest.Soldiers, manifest.Images), path))
}

func (a *App) handleExportBugReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: archive.DiagnosticsBundleName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "Bug report bundle", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Bug report export cancelled.")
		return
	}

	manifest, err := a.diagnostics.Export(path, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Bug report export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup(fmt.Sprintf("Bug report bundle ready (%d soldiers, %d images, %d scratch pads):", manifest.Soldiers, manifest.Images, manifest.Scratchpads), path))
}

func (a *App) handleImportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
			{DisplayName: "Legacy backup archive", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Backup import cancelled.")
		return
	}

	var localIdentity models.UserIdentity
	preserveLocalIdentity := false
	if a.database != nil {
		complete, err := a.database.SystemConfig("user_identity_complete")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(complete) == "1" {
			localIdentity, err = a.database.UserIdentity()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			preserveLocalIdentity = true
		}
	}

	if a.database != nil {
		a.database.Close()
		a.database = nil
	}

	manifest, err := a.backup.ImportWithLocalIdentity(path, a.dataDir, localIdentity, preserveLocalIdentity)
	if err != nil {
		if reopenErr := a.reopenDatabase(); reopenErr != nil {
			http.Error(w, fmt.Sprintf("backup import failed: %v (and reopen failed: %v)", err, reopenErr), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Backup import failed: %v", err)
		return
	}
	if err := a.reopenDatabase(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setToastHeader(w, fmt.Sprintf("Success: %d records imported from backup.", manifest.Soldiers))
	fmt.Fprintf(w, "Backup loaded: %d soldiers, %d records, %d images.", manifest.Soldiers, manifest.Records, manifest.Images)
}

func (a *App) handleImportSharedArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Shared backup import cancelled.")
		return
	}

	summary, err := a.backup.ImportSharedBackup(path, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Shared backup import failed: %v", err)
		return
	}
	if summary.PendingConflicts > 0 {
		w.Header().Set("X-DixieData-Redirect", "/export")
	}
	setToastHeader(w, fmt.Sprintf("Success: %d records imported, %d flagged for review.", summary.SoldiersInserted+summary.SoldiersUpdated, summary.PendingConflicts))
	fmt.Fprintf(w, "Shared backup merged: %d soldiers added, %d updated; %d records added, %d updated; %d images added, %d updated; %d conflicts staged for review. Merge log: %s",
		summary.SoldiersInserted, summary.SoldiersUpdated,
		summary.RecordsInserted, summary.RecordsUpdated,
		summary.ImagesInserted, summary.ImagesUpdated,
		summary.PendingConflicts,
		summary.LogPath,
	)
}

func (a *App) handlePreviewMemorialJSONImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Memorial archive JSON", Pattern: "*.json"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Memorial JSON import preview cancelled.")
		return
	}
	preview, err := a.soldiers.PreviewMemorialArchive(path)
	if err != nil {
		fmt.Fprintf(w, "Memorial JSON preview failed: %v", err)
		return
	}
	token, err := a.rememberMemorialPreview(path)
	if err != nil {
		fmt.Fprintf(w, "Memorial JSON preview failed: %v", err)
		return
	}
	fmt.Fprint(w, memorialImportPreviewMarkup(preview, token))
}

func (a *App) handleConfirmMemorialJSONImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(r.FormValue("preview_token"))
	path, ok := a.consumeMemorialPreview(token)
	if !ok {
		http.Error(w, "import preview expired. Run preview again.", http.StatusBadRequest)
		return
	}
	summary, err := a.soldiers.ImportMemorialArchive(path)
	if err != nil {
		fmt.Fprintf(w, "Memorial JSON import failed: %v", err)
		return
	}
	logPath, logErr := writeMemorialImportErrorLog(summary)
	if logErr != nil {
		fmt.Fprintf(w, "Memorial JSON import failed while writing error log: %v", logErr)
		return
	}
	setToastHeader(w, fmt.Sprintf("Memorial import complete: %d created, %d skipped, %d failed.", summary.Created, summary.Skipped, summary.Failed))
	fmt.Fprint(w, memorialImportSummaryMarkup(summary, logPath))
}

func (a *App) rememberMemorialPreview(path string) (string, error) {
	token, err := db.NewSyncID()
	if err != nil {
		return "", err
	}
	a.previewMu.Lock()
	if a.memorialPreview == nil {
		a.memorialPreview = make(map[string]string)
	}
	a.memorialPreview[token] = strings.TrimSpace(path)
	a.previewMu.Unlock()
	return token, nil
}

func (a *App) consumeMemorialPreview(token string) (string, bool) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", false
	}
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	if a.memorialPreview == nil {
		return "", false
	}
	path, ok := a.memorialPreview[trimmed]
	if ok {
		delete(a.memorialPreview, trimmed)
	}
	return path, ok
}

func (a *App) handleMergeReviewConflict(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/merge-review/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	conflictID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || conflictID < 1 {
		http.Error(w, "invalid merge review id", http.StatusBadRequest)
		return
	}
	var decision string
	switch parts[1] {
	case "keep-local":
		decision = "keep-local"
	case "keep-both":
		decision = "keep-both"
	case "keep-shared":
		decision = "keep-shared"
	case "use-shared":
		decision = "use-shared"
	default:
		http.NotFound(w, r)
		return
	}
	if err := a.backup.ResolveMergeConflict(conflictID, decision, a.dataDir); err != nil {
		fmt.Fprintf(w, "Merge review update failed: %v", err)
		return
	}
	w.Header().Set("X-DixieData-Redirect", "/export")
	setToastHeader(w, "Success: merge review updated.")
	fmt.Fprint(w, "Merge review updated.")
}

func (a *App) handleResolveReviewStatus(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.audit.ResolveFindingsForPersonRecord(id); err != nil {
		fmt.Fprintf(w, "Review queue update failed: %v", err)
		return
	}
	if err := a.soldiers.MarkReviewResolved(id); err != nil {
		fmt.Fprintf(w, "Review queue update failed: %v", err)
		return
	}
	setToastHeader(w, "Success: review item resolved.")
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("context")), "queue") {
		fmt.Fprint(w, "")
		return
	}
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d", id))
	fmt.Fprint(w, "Review status cleared.")
}

func (a *App) handleFlagReviewStatus(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(r.FormValue("review_reason"))
	if reason == "" {
		setToastHeaderWithType(w, "Add a review note before sending this record to the queue.", "warning")
		fmt.Fprint(w, "Add a review note before sending this record to the queue.")
		return
	}
	if err := a.soldiers.SetReviewStatus(id, true, reason); err != nil {
		setToastHeaderWithType(w, "Review queue update failed.", "error")
		fmt.Fprintf(w, "Review queue update failed: %v", err)
		return
	}
	setToastHeader(w, "Success: record added to the review queue.")
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d", id))
	fmt.Fprint(w, "Review status updated.")
}

func (a *App) handleReviewQueueCompare(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/review-queue/compare/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	findingID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid comparison id", http.StatusBadRequest)
		return
	}
	if len(parts) > 1 && parts[1] == "resolve" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := a.audit.ResolveFinding(findingID); err != nil {
			fmt.Fprintf(w, "Duplicate audit resolution failed: %v", err)
			return
		}
		setToastHeader(w, "Success: duplicate pair resolved.")
		w.Header().Set("X-DixieData-Redirect", "/review-queue")
		fmt.Fprint(w, "Duplicate pair resolved.")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	comparison, err := a.audit.Comparison(findingID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	presentation.ReviewQueueCompareView(*comparison).Render(r.Context(), w)
}

func (a *App) handleCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id1, id2, err := compareIDsFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	comparison, err := a.soldiers.ManualComparison(id1, id2)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "record not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if fromID, err := parseOptionalInt64(r.URL.Query().Get("from"), "from"); err == nil && fromID > 0 {
		comparison.BackHref = fmt.Sprintf("/soldiers/%d", fromID)
		comparison.BackLabel = "Back to Person Record"
	}
	presentation.ReviewQueueCompareView(*comparison).Render(r.Context(), w)
}

func (a *App) handleGoogleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.google.Connect(r.Context()); err != nil {
		fmt.Fprintf(w, "Google connect failed: %v", err)
		return
	}
	fmt.Fprint(w, "Google account connected.")
}

func (a *App) handleGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.google.Disconnect(); err != nil {
		fmt.Fprintf(w, "Google disconnect failed: %v", err)
		return
	}
	fmt.Fprint(w, "Google account disconnected.")
}

func (a *App) handleGoogleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-drive-backup-*")
	if err != nil {
		fmt.Fprintf(w, "Google Drive upload failed: %v", err)
		return
	}
	defer os.RemoveAll(tempDir)

	backupPath := filepath.Join(tempDir, backupArchiveName(time.Now()))
	manifest, err := a.backup.Export(backupPath, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Google Drive upload failed: %v", err)
		return
	}
	uploaded, err := a.google.UploadBackup(r.Context(), backupPath)
	if err != nil {
		fmt.Fprintf(w, "Google Drive upload failed: %v", err)
		return
	}
	fmt.Fprint(w, externalLinkMarkup(
		fmt.Sprintf("Backup uploaded to Google Drive (%d soldiers, %d images):", manifest.Soldiers, manifest.Images),
		uploaded.WebViewLink,
		uploaded.Name,
	))
}

func (a *App) handleGoogleSheetsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-google-sheets-*")
	if err != nil {
		fmt.Fprintf(w, "Google Sheets export failed: %v", err)
		return
	}
	defer os.RemoveAll(tempDir)

	csvPath := filepath.Join(tempDir, "dixiedata-export.csv")
	if err := a.export.ExportCSV(csvPath); err != nil {
		fmt.Fprintf(w, "Google Sheets export failed: %v", err)
		return
	}

	uploaded, err := a.google.UploadCSVAsSheet(r.Context(), csvPath, "DixieData Export")
	if err != nil {
		fmt.Fprintf(w, "Google Sheets export failed: %v", err)
		return
	}
	fmt.Fprint(w, externalLinkMarkup(
		"Google Sheet ready:",
		uploaded.WebViewLink,
		uploaded.Name,
	))
}

func (a *App) handleGoogleCalendarUseManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	calendarID, created, err := a.google.UseManagedCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar setup failed: %v", err)
		return
	}
	if created {
		fmt.Fprintf(w, "Managed DixieData calendar created and selected (%s).", calendarID)
		return
	}
	fmt.Fprintf(w, "Managed DixieData calendar selected (%s).", calendarID)
}

func (a *App) handleGoogleCalendarPreferencesSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	preferences, err := parseCalendarEventPreferencesForm(r)
	if err != nil {
		fmt.Fprintf(w, "Calendar preference save failed: %v", err)
		return
	}
	saved, err := a.google.SaveManagedEventPreferences(preferences)
	if err != nil {
		fmt.Fprintf(w, "Calendar preference save failed: %v", err)
		return
	}
	fmt.Fprintf(w, "Calendar preferences saved. Title preset: %s. Start time: %s. Sync DixieData Calendar to apply changes globally.", saved.TitlePreset, saved.StartTime)
}

func (a *App) handleGoogleCalendarSyncManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, _, err := a.google.UseManagedCalendar(r.Context()); err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	settings, _, _, _, err := a.google.LoadEffectiveSettings()
	if err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	soldiers, err := a.listAllSoldiers()
	if err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	if strings.TrimSpace(r.FormValue("confirm_sync")) != "1" {
		preview, err := a.google.PreviewSyncCalendar(settings, soldiers)
		if err != nil {
			fmt.Fprintf(w, "Google Calendar sync preview failed: %v", err)
			return
		}
		fmt.Fprintf(w, "Dry run: %d create, %d update, %d delete, %d skip. <button type=\"button\" name=\"confirm_sync\" value=\"1\" class=\"secondary-button ml-2\" hx-post=\"/integrations/google/calendar/sync-managed\" hx-target=\"#google-status\" data-progress-label=\"Syncing DixieData Calendar...\" data-busy-group=\"google-calendar-actions\">Run Sync Now</button>", preview.Created, preview.Updated, preview.Deleted, preview.Skipped)
		return
	}
	result, err := a.google.SyncCalendar(r.Context(), settings, soldiers)
	if err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "DixieData Calendar synced: %d created, %d updated, %d deleted, %d skipped.", result.Created, result.Updated, result.Deleted, result.Skipped)
}

func (a *App) handleGoogleCalendarUnsyncManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.UnsyncCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar unsync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "DixieData Calendar unsynced: %d event(s) removed.", result.Deleted)
}

func (a *App) handleGoogleCalendarUseTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	calendarID, created, err := a.google.UseTestCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar test setup failed: %v", err)
		return
	}
	if created {
		fmt.Fprintf(w, "DixieData Test calendar created and selected (%s).", calendarID)
		return
	}
	fmt.Fprintf(w, "DixieData Test calendar selected (%s).", calendarID)
}

func (a *App) handleGoogleCalendarSyncTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.SyncTestCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar test sync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "DixieData Test sync complete: %d created, %d updated, %d deleted, %d skipped.", result.Created, result.Updated, result.Deleted, result.Skipped)
}

func (a *App) handleGoogleCalendarUnsyncTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.UnsyncTestCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar test unsync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "DixieData Test unsynced: %d event(s) removed.", result.Deleted)
}

func (a *App) handleSoldierPDF(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(a.dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	options := parsePDFOptionsRequest(r, "L", true)

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: soldierPDFName(*soldier, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "PDF export cancelled.")
		return
	}
	if err := a.export.ExportSoldierPDF(path, *soldier, options); err != nil {
		fmt.Fprintf(w, "PDF export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("PDF ready:", path))
}

func (a *App) handleSoldierPDFNoImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: soldierPDFNameNoImages(*soldier),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "PDF export cancelled.")
		return
	}
	if err := a.export.ExportSoldierPDFWithoutImages(path, *soldier); err != nil {
		fmt.Fprintf(w, "PDF export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("PDF without images ready:", path))
}

func (a *App) handleSoldierJPG(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(a.dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	options := parsePDFOptionsRequest(r, "L", true)

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: soldierJPGName(*soldier, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "JPEG image", Pattern: "*.jpg"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "JPG export cancelled.")
		return
	}

	paths, err := a.export.ExportSoldierJPG(path, *soldier, options)
	if err != nil {
		fmt.Fprintf(w, "JPG export failed: %v", err)
		return
	}

	label := "JPG ready:"
	if len(paths) > 1 {
		label = fmt.Sprintf("JPG ready (%d pages; first page shown):", len(paths))
	}
	fmt.Fprint(w, exportLinkMarkup(label, paths[0]))
}

func (a *App) handleCalendarPDF(w http.ResponseWriter, r *http.Request, monthValue string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	month, err := parseBoundedInt(monthValue, "month", 1, 12)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	calendar, err := a.anniversary.GetMonthCalendar(month)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	options := parsePDFOptionsRequest(r, "P", false)

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: monthPDFName(month, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Monthly PDF export cancelled.")
		return
	}
	if err := a.export.ExportMonthlyAnniversaryPDF(path, month, calendar, options); err != nil {
		fmt.Fprintf(w, "Monthly PDF export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("Monthly PDF ready:", path))
}

func (a *App) handleImageScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ImageData string `json:"imageData"`
		FileName  string `json:"fileName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid screenshot payload", http.StatusBadRequest)
		return
	}

	imageData := strings.TrimSpace(payload.ImageData)
	if !strings.HasPrefix(imageData, "data:image/png;base64,") {
		http.Error(w, "invalid screenshot image", http.StatusBadRequest)
		return
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(imageData, "data:image/png;base64,"))
	if err != nil {
		http.Error(w, "invalid screenshot image", http.StatusBadRequest)
		return
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: imageScreenshotName(payload.FileName),
		Filters: []runtime.FileFilter{
			{DisplayName: "PNG image", Pattern: "*.png"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Screenshot cancelled.")
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintf(w, "Screenshot failed: %v", err)
		return
	}
	fmt.Fprintf(w, "✓ Saved screenshot to %s", path)
}

type imageRotateRequest struct {
	ImageID   int64  `json:"imageId"`
	Direction string `json:"direction"`
}

func (a *App) handleImageRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req imageRotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid rotate request", http.StatusBadRequest)
		return
	}
	if req.ImageID < 1 {
		http.Error(w, "invalid image id", http.StatusBadRequest)
		return
	}

	imageRecord, err := a.soldiers.GetImageByID(req.ImageID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	imagePath := filepath.Join(a.dataDir, filepath.FromSlash(imageRecord.FilePath))
	switch strings.ToLower(strings.TrimSpace(req.Direction)) {
	case "cw":
		err = rotateImageFile(imagePath, true)
	case "ccw":
		err = rotateImageFile(imagePath, false)
	default:
		http.Error(w, "invalid rotate direction", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Fprint(w, "Image rotated.")
}

func rotateImageFile(path string, clockwise bool) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open image file: %w", err)
	}
	img, format, err := image.Decode(source)
	source.Close()
	if err != nil {
		return fmt.Errorf("decode image file: %w", err)
	}

	rotated := rotateImage90(img, clockwise)
	tempPath := path + ".rotate"
	output, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create rotated image file: %w", err)
	}

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err = jpeg.Encode(output, rotated, &jpeg.Options{Quality: 95})
	case "png":
		err = png.Encode(output, rotated)
	case "gif":
		err = gif.Encode(output, rotated, nil)
	default:
		err = fmt.Errorf("unsupported image format for rotation: %s", format)
	}
	closeErr := output.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	if err := os.Remove(path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace rotated image file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace rotated image file: %w", err)
	}
	return nil
}

func rotateImage90(src image.Image, clockwise bool) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, height, width))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if clockwise {
				dst.Set(height-1-y, x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			} else {
				dst.Set(y, width-1-x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
	}
	return dst
}

func (a *App) handleDownloadSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if len(soldier.Images) == 0 {
		fmt.Fprint(w, "No images are attached to this record.")
		return
	}

	selected, err := selectedSoldierImages(*soldier, r.Form["image_ids"], a.dataDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(selected) == 0 {
		fmt.Fprint(w, "Select at least one image to download.")
		return
	}

	parentDir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose where to copy the record images",
	})
	if err != nil || parentDir == "" {
		fmt.Fprint(w, "Download cancelled.")
		return
	}
	destinationDir := filepath.Join(parentDir, imageExportFolderName(*soldier))
	if err := a.export.ExportImages(destinationDir, selected); err != nil {
		fmt.Fprintf(w, "Download failed: %v", err)
		return
	}
	fmt.Fprintf(w, "✓ Copied %d image(s) to %s", len(selected), destinationDir)
}

func (a *App) handleImportSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Image files", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp;*.svg"},
		},
	})
	if err != nil || len(paths) == 0 {
		fmt.Fprint(w, "Image import cancelled.")
		return
	}

	imported, importErr := a.importImagePaths(*soldier, paths)
	if importErr != nil {
		if imported > 0 {
			fmt.Fprintf(w, "Imported %d image(s), but some files failed: %v", imported, importErr)
			return
		}
		fmt.Fprintf(w, "Image import failed: %v", importErr)
		return
	}

	w.Header().Set("X-DixieData-Redirect", imageImportRedirectPath(id, r.URL.Query().Get("return")))
	fmt.Fprintf(w, "Imported %d image(s).", imported)
}

func imageImportRedirectPath(id int64, returnTarget string) string {
	switch strings.ToLower(strings.TrimSpace(returnTarget)) {
	case "edit":
		return fmt.Sprintf("/soldiers/%d/edit", id)
	default:
		return fmt.Sprintf("/soldiers/%d", id)
	}
}

func (a *App) handleDeleteSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	selected, err := selectedSoldierImages(*soldier, r.Form["image_ids"], a.dataDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(selected) == 0 {
		fmt.Fprint(w, "Select at least one image to delete.")
		return
	}

	for _, image := range selected {
		if err := os.Remove(image.FilePath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(w, "Delete failed: %v", err)
			return
		}
	}

	imageIDs := make([]int64, 0, len(selected))
	for _, image := range selected {
		imageIDs = append(imageIDs, image.ID)
	}
	if err := a.soldiers.DeleteImages(id, imageIDs); err != nil {
		fmt.Fprintf(w, "Delete failed: %v", err)
		return
	}

	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d", id))
	fmt.Fprintf(w, "Deleted %d image(s).", len(selected))
}

func (a *App) handleSetPrimarySoldierImage(w http.ResponseWriter, r *http.Request, id, imageID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := a.soldiers.GetByID(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := a.soldiers.SetPrimaryImage(id, imageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "image not found", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, "Primary image update failed: %v", err)
		return
	}
	setToastHeader(w, "Primary image updated.")
	fmt.Fprint(w, "Primary image updated.")
}

func parseCalendarEventPreferencesForm(r *http.Request) (models.CalendarEventPreferences, error) {
	if err := r.ParseForm(); err != nil {
		return models.CalendarEventPreferences{}, fmt.Errorf("failed to parse form: %w", err)
	}
	preferences := models.CalendarEventPreferences{
		TitlePreset:         strings.TrimSpace(r.FormValue("title_preset")),
		StartTime:           strings.TrimSpace(r.FormValue("start_time")),
		ReminderPrimary:     strings.TrimSpace(r.FormValue("reminder_primary")),
		ReminderSecondary:   strings.TrimSpace(r.FormValue("reminder_secondary")),
		IncludeRecordID:     r.FormValue("include_record_id") == "1",
		IncludeUnit:         r.FormValue("include_unit") == "1",
		IncludeBuriedIn:     r.FormValue("include_buried_in") == "1",
		IncludeOriginalDate: r.FormValue("include_original_date") == "1",
	}
	if _, _, ok := models.CalendarTimeComponents(preferences.StartTime); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("start time must be between 05:00 and 23:00 in 15-minute increments")
	}
	if _, ok := models.CalendarReminderMinutes(preferences.ReminderPrimary); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("invalid primary reminder option")
	}
	if _, ok := models.CalendarReminderMinutes(preferences.ReminderSecondary); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("invalid secondary reminder option")
	}
	if strings.TrimSpace(preferences.ReminderPrimary) != "none" && preferences.ReminderPrimary == preferences.ReminderSecondary {
		return models.CalendarEventPreferences{}, fmt.Errorf("reminder selections must be different")
	}
	if !preferences.IncludeRecordID && !preferences.IncludeUnit && !preferences.IncludeBuriedIn && !preferences.IncludeOriginalDate {
		return models.CalendarEventPreferences{}, fmt.Errorf("select at least one description field")
	}
	return preferences, nil
}

func parseSoldierForm(r *http.Request, id int64) (models.Soldier, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return models.Soldier{}, fmt.Errorf("failed to parse multipart form: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return models.Soldier{}, fmt.Errorf("failed to parse form: %w", err)
		}
	}

	birthDate, err := parseOptionalCanonicalDate(r.FormValue("birth_date"), "birth_date")
	if err != nil {
		return models.Soldier{}, err
	}
	deathDate, err := parseOptionalCanonicalDate(r.FormValue("death_date"), "death_date")
	if err != nil {
		return models.Soldier{}, err
	}
	spouseSoldierID, err := parseOptionalInt64(r.FormValue("spouse_soldier_id"), "spouse_soldier_id")
	if err != nil {
		return models.Soldier{}, err
	}

	needsReview := r.FormValue("existing_needs_review") == "1"
	reviewReason := r.FormValue("existing_review_reason")
	if findAGraveNeedsReview(r) {
		needsReview = true
		reviewReason = findAGraveReviewReason(r)
	}

	return models.Soldier{
		ID:                    id,
		DisplayID:             r.FormValue("display_id"),
		EntryType:             r.FormValue("entry_type"),
		SpouseSoldierID:       spouseSoldierID,
		RelationshipLabel:     r.FormValue("relationship_label"),
		MaidenName:            r.FormValue("maiden_name"),
		PensionID:             r.FormValue("pension_id"),
		ApplicationID:         r.FormValue("application_id"),
		Prefix:                r.FormValue("prefix"),
		ShowPrefixBeforeName:  r.FormValue("show_prefix_before_name") == "1",
		FirstName:             r.FormValue("first_name"),
		MiddleName:            r.FormValue("middle_name"),
		LastName:              r.FormValue("last_name"),
		Suffix:                r.FormValue("suffix"),
		Rank:                  r.FormValue("rank_out"),
		RankIn:                r.FormValue("rank_in"),
		RankOut:               r.FormValue("rank_out"),
		Unit:                  r.FormValue("unit"),
		PensionState:          r.FormValue("pension_state"),
		ConfederateHomeStatus: r.FormValue("confederate_home_status"),
		ConfederateHomeName:   r.FormValue("confederate_home_name"),
		BirthDate:             birthDate,
		DeathDate:             deathDate,
		BirthInfo:             r.FormValue("birth_info"),
		BuriedIn:              r.FormValue("buried_in"),
		Biography:             r.FormValue("biography"),
		PDFExcerptOverride:    r.FormValue("pdf_excerpt_override"),
		Notes:                 r.FormValue("notes"),
		NeedsReview:           needsReview,
		ReviewReason:          reviewReason,
		Records:               parseRecordInputs(r),
	}, nil
}

func findAGraveNeedsReview(r *http.Request) bool {
	score, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("scrape_confidence_score")))
	return strings.TrimSpace(r.FormValue("scrape_source_label")) != "" && score > 0 && score < 70
}

func findAGraveReviewReason(r *http.Request) string {
	if !findAGraveNeedsReview(r) {
		return ""
	}
	score, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("scrape_confidence_score")))
	return fmt.Sprintf("Low-confidence Find a Grave scrape (%d/100). Verify memorial details before clearing review.", score)
}

func (a *App) newSoldierDefaults() (models.Soldier, error) {
	displayID, err := a.database.NextDXDID()
	if err != nil {
		return models.Soldier{}, err
	}
	return models.Soldier{DisplayID: displayID, PensionState: pensionstate.NotApplicable, ConfederateHomeStatus: confederatehomestatus.NotApplicable, ShowPrefixBeforeName: false}, nil
}

func parseOptionalInt(value, field string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func parseOptionalInt64(value, field string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func (a *App) handleCreateCalendarItem(w http.ResponseWriter, r *http.Request, month, day int) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	input := records.CalendarItemInput{
		ItemType: r.FormValue("item_type"),
		Title:    r.FormValue("title"),
		Notes:    r.FormValue("notes"),
	}
	item, err := a.calendar.CreateCalendarItem(month, day, input)
	if err != nil {
		if calendarValidationError(err) {
			a.renderCalendarDayDetail(w, r, month, day, 0, input.ItemType, input.Title, input.Notes, err.Error(), "", "", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", fmt.Sprintf("%s saved.", calendarItemTypeLabel(item.ItemType)), http.StatusOK)
}

func (a *App) handleUpdateCalendarItem(w http.ResponseWriter, r *http.Request, month, day int, itemID int64) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	input := records.CalendarItemInput{
		ItemType: r.FormValue("item_type"),
		Title:    r.FormValue("title"),
		Notes:    r.FormValue("notes"),
	}
	item, err := a.calendar.UpdateCalendarItem(itemID, input)
	if err != nil {
		switch {
		case errors.Is(err, records.ErrCalendarItemNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		case calendarValidationError(err):
			a.renderCalendarDayDetail(w, r, month, day, itemID, input.ItemType, input.Title, input.Notes, err.Error(), "", "", http.StatusBadRequest)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", fmt.Sprintf("%s updated.", calendarItemTypeLabel(item.ItemType)), http.StatusOK)
}

func (a *App) handleDeleteCalendarItem(w http.ResponseWriter, r *http.Request, month, day int, itemID int64) {
	if err := a.calendar.DeleteCalendarItem(itemID); err != nil {
		switch {
		case errors.Is(err, records.ErrCalendarItemNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		case calendarValidationError(err):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", "Calendar item deleted.", http.StatusOK)
}

func (a *App) renderCalendarDayDetail(w http.ResponseWriter, r *http.Request, month, day int, editingID int64, itemType, title, notes, errorMessage, statusKind, statusMessage string, statusCode int) {
	detail, err := a.calendar.GetDay(month, day)
	if err != nil {
		if calendarValidationError(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if editingID > 0 && strings.TrimSpace(itemType) == "" && strings.TrimSpace(title) == "" && strings.TrimSpace(notes) == "" {
		item, ok := findCalendarItem(detail.Items, editingID)
		if !ok {
			http.Error(w, records.ErrCalendarItemNotFound.Error(), http.StatusNotFound)
			return
		}
		itemType = item.ItemType
		title = item.Title
		notes = item.Notes
	}
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	presentation.CalendarDayDetail(detail, editingID, itemType, title, notes, errorMessage, statusKind, statusMessage).Render(r.Context(), w)
}

func findCalendarItem(items []models.CalendarItem, itemID int64) (models.CalendarItem, bool) {
	for _, item := range items {
		if item.ID == itemID {
			return item, true
		}
	}
	return models.CalendarItem{}, false
}

func calendarValidationError(err error) bool {
	var validationErr *records.CalendarValidationError
	return errors.As(err, &validationErr)
}

func calendarItemTypeLabel(itemType string) string {
	switch itemType {
	case models.CalendarItemTypeHoliday:
		return "Holiday"
	default:
		return "Event"
	}
}

func parseOptionalBoundedInt(value, field string, min, max int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	return parseBoundedInt(trimmed, field, min, max)
}

func (a *App) renderEntryForm(w http.ResponseWriter, r *http.Request, soldier models.Soldier, isEdit bool, errorMessage string, statusCode int) {
	a.renderEntryFormWithScrapeState(w, r, soldier, isEdit, errorMessage, models.FindAGraveScrapeState{}, statusCode, false)
}

func (a *App) renderEntryFormWithScrapeState(w http.ResponseWriter, r *http.Request, soldier models.Soldier, isEdit bool, errorMessage string, scrape models.FindAGraveScrapeState, statusCode int, fragmentOnly bool) {
	candidates, err := a.soldiers.MarriageCandidates()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	suggestions, err := a.soldiers.FormSuggestions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(statusCode)
	if fragmentOnly {
		presentation.EntryFormFragment(soldier, candidates, suggestions, scrape, isEdit, errorMessage).Render(r.Context(), w)
		return
	}
	if errorMessage != "" {
		presentation.EntryFormWithError(soldier, candidates, suggestions, scrape, isEdit, errorMessage).Render(r.Context(), w)
		return
	}
	presentation.EntryForm(soldier, candidates, suggestions, scrape, isEdit).Render(r.Context(), w)
}

func applyFindAGraveAutofill(base models.Soldier, result findagrave.Result) models.Soldier {
	base.FirstName = result.FirstName
	base.MiddleName = result.MiddleName
	base.LastName = result.LastName
	base.BirthDate = result.BirthDate
	base.BirthInfo = result.BirthInfo
	base.DeathDate = result.DeathDate
	base.BuriedIn = result.BuriedIn
	if strings.TrimSpace(result.MemorialID) != "" || strings.TrimSpace(result.MemorialURL) != "" {
		details := strings.TrimSpace(result.MemorialURL)
		if details == "" {
			details = "Find a Grave memorial"
		}
		base.Records = []models.Record{{
			RecordType: "Find a Grave",
			AppID:      strings.TrimSpace(result.MemorialID),
			Details:    details,
		}}
	}
	return base
}

func parseOptionalCanonicalDate(value, field string) (string, error) {
	normalized, err := dates.NormalizeCanonical(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s", field)
	}
	return normalized, nil
}

func parseLegacySearchComponent(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func parseBoundedInt(value, field string, min, max int) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func selectedSoldierImages(soldier models.Soldier, selectedIDs []string, dataDir string) ([]models.Image, error) {
	selectedSet := make(map[int64]struct{}, len(selectedIDs))
	for _, value := range selectedIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid image selection")
		}
		selectedSet[id] = struct{}{}
	}

	var selected []models.Image
	for _, image := range soldier.Images {
		if _, ok := selectedSet[image.ID]; !ok {
			continue
		}
		image.FilePath = filepath.Join(dataDir, filepath.FromSlash(image.FilePath))
		selected = append(selected, image)
	}
	return selected, nil
}

func imageExportFolderName(soldier models.Soldier) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = fmt.Sprintf("%s-%s", soldier.FirstName, soldier.LastName)
	}
	return sanitizedFileStem(base, "soldier-images") + "_Images"
}

func imageScreenshotName(fileName string) string {
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	return sanitizedFileStem(base, "archive-image") + "-screenshot.png"
}

func soldierPDFName(soldier models.Soldier, options archive.PDFOptions) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return pdfReportName(base, options, !options.IncludeImages)
}

func soldierJPGName(soldier models.Soldier, options archive.PDFOptions) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return jpgReportName(base, options, !options.IncludeImages)
}

func soldierPDFNameNoImages(soldier models.Soldier) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return pdfReportName(base, archive.PDFOptions{Orientation: "L", IncludeImages: false}, true)
}

func monthPDFName(month int, options archive.PDFOptions) string {
	return pdfReportName(fmt.Sprintf("%s report", monthNameValue(month)), options, false)
}

func printableArchivePDFName(settings archive.PrintSettings) string {
	name := pdfReportName("dixiedata-printable-archive", archive.PDFOptions{
		Orientation:     settings.Orientation,
		PrinterFriendly: settings.PrinterFriendly,
	}, false)
	if !settings.FullBiographyPage {
		return name
	}
	return strings.TrimSuffix(name, ".pdf") + "-full-biography.pdf"
}

func pdfReportName(base string, options archive.PDFOptions, noImages bool) string {
	stem := sanitizedFileStem(base, "pdf-report")
	suffix := pdfOptionFilenameSuffix(options, noImages)
	if suffix != "" {
		stem += "-" + suffix
	}
	return stem + ".pdf"
}

func jpgReportName(base string, options archive.PDFOptions, noImages bool) string {
	stem := sanitizedFileStem(base, "jpg-report")
	suffix := pdfOptionFilenameSuffix(options, noImages)
	if suffix != "" {
		stem += "-" + suffix
	}
	return stem + ".jpg"
}

func pdfOptionFilenameSuffix(options archive.PDFOptions, noImages bool) string {
	options = options.Normalize("P", true)
	parts := make([]string, 0, 3)
	if options.PrinterFriendly {
		parts = append(parts, "printer-friendly")
	}
	if options.Orientation == "L" {
		parts = append(parts, "landscape")
	} else {
		parts = append(parts, "portrait")
	}
	if noImages {
		parts = append(parts, "no-images")
	}
	return strings.Join(parts, "-")
}

func backupArchiveName(now time.Time) string {
	return fmt.Sprintf("dixiedata-backup-%s.ddbak", now.Format("2006-01-02"))
}

func sharedArchiveName(now time.Time) string {
	return fmt.Sprintf("dixiedata-shared-%s.ddshare", now.Format("2006-01-02"))
}

func sanitizedFileStem(value, fallback string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		case r == ' ':
			return '-'
		default:
			return '-'
		}
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		return fallback
	}
	return value
}

func monthNameValue(month int) string {
	months := []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	if month < 1 || month > 12 {
		return "Unknown"
	}
	return months[month]
}

func parseRecordInputs(r *http.Request) []models.Record {
	recordTypes := r.Form["record_type"]
	appIDs := r.Form["record_app_id"]
	details := r.Form["record_details"]

	count := len(recordTypes)
	if len(appIDs) > count {
		count = len(appIDs)
	}
	if len(details) > count {
		count = len(details)
	}

	records := make([]models.Record, 0, count)
	for i := 0; i < count; i++ {
		record := models.Record{}
		if i < len(recordTypes) {
			record.RecordType = recordTypes[i]
		}
		if i < len(appIDs) {
			record.AppID = appIDs[i]
		}
		if i < len(details) {
			record.Details = details[i]
		}
		records = append(records, record)
	}
	return records
}

func memorialImportPreviewMarkup(preview records.MemorialImportPreview, token string) string {
	confirm := fmt.Sprintf(
		`<form hx-post="/import/memorial-json/confirm" hx-target="#share-status" class="mt-4"><input type="hidden" name="preview_token" value="%s"/><button class="primary-button" type="submit">Confirm Import</button></form>`,
		html.EscapeString(strings.TrimSpace(token)),
	)
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">
<div class="font-semibold text-[#22303d]">Memorial JSON preview ready</div>
<div class="mt-2">File: <code>%s</code></div>
<div class="mt-1">Rows: %d · Would create: %d · Would skip: %d · Would fail: %d</div>
%s%s
</div>`,
		html.EscapeString(strings.TrimSpace(preview.FilePath)),
		preview.TotalRows,
		preview.WouldCreate,
		preview.WouldSkip,
		preview.WouldFail,
		memorialImportIssuesList(preview.Issues, ""),
		confirm,
	)
}

func memorialImportSummaryMarkup(summary records.MemorialImportSummary, logPath string) string {
	reportLink := `<a href="/browse?scope=last_import&sort=created_desc" class="pill-link">Open Browse Last Import</a>`
	logLine := ""
	trimmedLog := strings.TrimSpace(logPath)
	if trimmedLog != "" {
		logLine = fmt.Sprintf(`<div class="mt-2 text-xs text-slate-500">Full error log: <code>%s</code></div>`, html.EscapeString(trimmedLog))
	}
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">
<div class="font-semibold text-[#22303d]">Memorial JSON import complete</div>
<div class="mt-2">Rows: %d · Created: %d · Skipped: %d · Failed: %d</div>
<div class="mt-2">%s</div>
%s%s
</div>`,
		summary.TotalRows,
		summary.Created,
		summary.Skipped,
		summary.Failed,
		reportLink,
		memorialImportIssuesList(summary.Issues, "first 20 errors"),
		logLine,
	)
}

func memorialImportIssuesList(issues []records.MemorialImportIssue, label string) string {
	if len(issues) == 0 {
		return ""
	}
	limit := 20
	if len(issues) < limit {
		limit = len(issues)
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		issue := issues[i]
		memorialID := strings.TrimSpace(issue.MemorialID)
		if memorialID == "" {
			memorialID = "unknown memorial_id"
		}
		name := strings.TrimSpace(issue.Name)
		if name == "" {
			name = "unnamed"
		}
		lines = append(lines, fmt.Sprintf(`<li>Row %d (%s / %s): %s</li>`,
			issue.Row,
			html.EscapeString(memorialID),
			html.EscapeString(name),
			html.EscapeString(issue.Error),
		))
	}
	prefix := "Issues"
	if strings.TrimSpace(label) != "" {
		prefix = strings.TrimSpace(label)
	}
	return fmt.Sprintf(`<div class="mt-3 rounded-2xl border border-amber-700/40 bg-amber-50/80 px-3 py-2 text-xs text-amber-950"><div class="font-semibold">%s</div><ul class="mt-2 list-disc space-y-1 pl-5">%s</ul></div>`,
		html.EscapeString(prefix),
		strings.Join(lines, ""),
	)
}

func writeMemorialImportErrorLog(summary records.MemorialImportSummary) (string, error) {
	if len(summary.Issues) == 0 {
		return "", nil
	}
	file, err := os.CreateTemp("", "dixiedata-memorial-import-*.log")
	if err != nil {
		return "", err
	}
	defer file.Close()
	for _, issue := range summary.Issues {
		_, err := fmt.Fprintf(file, "row=%d memorial_id=%q name=%q error=%q\n", issue.Row, issue.MemorialID, issue.Name, issue.Error)
		if err != nil {
			return "", err
		}
	}
	return file.Name(), nil
}

func exportLinkMarkup(label, path string) string {
	fileURL := "file:///" + strings.TrimPrefix(filepath.ToSlash(path), "/")
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">%s <a href="%s" data-open-external="true" class="pill-link" target="_blank" rel="noreferrer">%s</a></div>`,
		html.EscapeString(label),
		html.EscapeString(fileURL),
		html.EscapeString(path),
	)
}

func externalLinkMarkup(label, href, text string) string {
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">%s <a href="%s" data-open-external="true" class="pill-link" target="_blank" rel="noreferrer">%s</a></div>`,
		html.EscapeString(label),
		html.EscapeString(href),
		html.EscapeString(text),
	)
}

func (a *App) handleOpenLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	target, err := normalizeChromeOpenTarget(r.FormValue("target"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := openLinkTarget(target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleScratchpadOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	if a.scratchpads == nil {
		http.Error(w, "scratch pad service unavailable", http.StatusServiceUnavailable)
		return
	}
	displayID := strings.TrimSpace(r.FormValue("display_id"))
	if displayID == "" {
		http.Error(w, "A Display ID is required before opening the scratch pad.", http.StatusBadRequest)
		return
	}
	seed := r.FormValue("scratchpad_seed")
	if err := a.scratchpads.Open(displayID, seed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "Scratch pad ready for %s.", displayID)
}

func (a *App) handleMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	relative := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/media/"))
	relative = strings.TrimLeft(relative, `/\`)
	if relative == "" {
		http.NotFound(w, r)
		return
	}

	baseDir := filepath.Clean(a.dataDir)
	resolved := filepath.Join(baseDir, filepath.FromSlash(relative))
	withinBase, err := filepath.Rel(baseDir, resolved)
	if err != nil || strings.HasPrefix(withinBase, "..") {
		http.NotFound(w, r)
		return
	}

	info, err := os.Stat(resolved)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, resolved)
		return
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 480 320" role="img" aria-label="Image Missing">
<rect width="480" height="320" rx="28" fill="#f6f1e4"/>
<rect x="16" y="16" width="448" height="288" rx="22" fill="#fff" stroke="#8d7440" stroke-width="4" stroke-dasharray="12 8"/>
<path d="M96 224l56-72 52 48 44-56 88 80" fill="none" stroke="#324253" stroke-width="16" stroke-linecap="round" stroke-linejoin="round"/>
<circle cx="164" cy="116" r="24" fill="#c5ab68"/>
<text x="240" y="264" text-anchor="middle" font-family="Arial, sans-serif" font-size="28" font-weight="700" fill="#22303d">Image Missing</text>
<text x="240" y="292" text-anchor="middle" font-family="Arial, sans-serif" font-size="15" fill="#324253">%s</text>
</svg>`, html.EscapeString(filepath.Base(relative)))
}

func normalizeChromeOpenTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("missing link target")
	}
	if strings.HasPrefix(strings.ToLower(target), "http://") || strings.HasPrefix(strings.ToLower(target), "https://") || strings.HasPrefix(strings.ToLower(target), "file:///") {
		return target, nil
	}
	if filepath.IsAbs(target) {
		return "file:///" + strings.TrimPrefix(filepath.ToSlash(target), "/"), nil
	}
	parsed, err := url.Parse(target)
	if err == nil && parsed.Scheme != "" {
		return target, nil
	}
	return "", fmt.Errorf("unsupported link target: %s", target)
}

func openLinkInChrome(target string) error {
	chromePath, err := findChromeExecutable()
	if err != nil {
		return err
	}
	return exec.Command(chromePath, "--new-tab", target).Start()
}

func openLinkTarget(target string) error {
	if isFileOpenTarget(target) {
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	}
	return openLinkInChrome(target)
}

func isFileOpenTarget(target string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "file:///")
}

func findChromeExecutable() (string, error) {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("chrome.exe"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("chrome"); err == nil {
		return path, nil
	}
	return "", errors.New("Google Chrome was not found")
}

func (a *App) reloadServices() error {
	soldierSvc := records.NewSoldierService(a.database)
	a.soldiers = soldierSvc
	a.anniversary = records.NewAnniversaryService(a.database)
	a.calendar = records.NewCalendarService(a.database)
	a.analytics = records.NewAnalyticsService(a.database)
	a.audit = records.NewAuditService(a.database)
	a.images = archive.NewImageService(a.database)
	a.export = archive.NewExportService(a.database, soldierSvc)
	a.backup = archive.NewBackupService(a.database, soldierSvc)
	a.diagnostics = archive.NewDiagnosticsService(a.database, soldierSvc)
	a.google = integrations.NewGoogleService(a.dataDir)
	a.updater = update.NewService(a.database, a.dataDir, func(outputPath string) error {
		_, err := a.backup.Export(outputPath, a.dataDir)
		return err
	})
	a.scratchpads = scratchpad.NewLauncher(a.dataDir, a.database)
	if a.database != nil {
		if err := a.images.EnsureShardedStorage(a.dataDir); err != nil {
			return err
		}
		if err := a.images.PurgeExpiredTrash(a.dataDir); err != nil {
			return err
		}
		required, err := a.database.IdentitySetupRequired()
		if err != nil {
			return err
		}
		a.setupRequired = required
		if !required {
			needsBackfill, err := a.database.EntryAuditIdentityBackfillNeeded()
			if err != nil {
				return err
			}
			if needsBackfill {
				if err := a.database.BackfillEntryAuditIdentity(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (a *App) activatePendingRecovery(restorePointID string, cause error) error {
	if a.database != nil {
		a.database.Close()
		a.database = nil
	}
	record, err := a.restorePoints.Get(restorePointID)
	if err != nil {
		return fmt.Errorf("load restore point %q: %w", restorePointID, err)
	}
	a.pendingRecovery = &record
	if cause != nil {
		a.recoveryFailure = cause.Error()
	}
	return nil
}

func (a *App) initializeLocalData() error {
	if filepath.Base(a.dataDir) != ".dixiedata" {
		return fmt.Errorf("refusing to initialize unexpected data directory: %s", a.dataDir)
	}
	if a.database != nil {
		a.database.Close()
		a.database = nil
	}
	if err := os.RemoveAll(a.dataDir); err != nil {
		return err
	}
	return a.reopenDatabase()
}

func (a *App) reopenDatabase() error {
	database, err := db.Open(a.dataDir)
	if err != nil {
		return err
	}
	a.database = database
	return a.reloadServices()
}

func loadQuotes(data []byte) ([]models.Quote, error) {
	var payload struct {
		Quotes []models.Quote `json:"civil_war_quotes"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Quotes == nil {
		payload.Quotes = []models.Quote{}
	}
	return payload.Quotes, nil
}

func (a *App) listAllSoldiers() ([]models.Soldier, error) {
	var soldiers []models.Soldier
	page := 1
	for {
		batch, _, err := a.soldiers.List(page, 500)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		soldiers = append(soldiers, batch...)
		if len(batch) < 500 {
			break
		}
		page++
	}
	return soldiers, nil
}

func selectQuoteForArchive(quotes []models.Quote, totalSoldiers int) models.Quote {
	if len(quotes) == 0 {
		return models.Quote{}
	}
	if totalSoldiers < 0 {
		totalSoldiers = 0
	}
	index := (totalSoldiers / 3) % len(quotes)
	return quotes[index]
}

func parseInitialSetupForm(r *http.Request) (models.InitialSetupForm, int, error) {
	if err := r.ParseForm(); err != nil {
		return models.InitialSetupForm{}, 0, fmt.Errorf("failed to parse setup form")
	}
	form := models.InitialSetupForm{
		FirstName:  strings.TrimSpace(r.FormValue("first_name")),
		MiddleName: strings.TrimSpace(r.FormValue("middle_name")),
		LastName:   strings.TrimSpace(r.FormValue("last_name")),
		BirthYear:  strings.TrimSpace(r.FormValue("birth_year")),
	}
	birthYear, err := parseBoundedInt(form.BirthYear, "birth_year", 1000, 9999)
	if err != nil {
		return form, 0, err
	}
	prefix, err := db.BuildUserNodePrefix(form.FirstName, form.MiddleName, form.LastName, birthYear)
	if err != nil {
		return form, 0, err
	}
	form.PrefixPreview = prefix
	return form, birthYear, nil
}

func parsePage(value string) int {
	page, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func parseCSVInt64s(value string) ([]int64, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	results := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id < 1 {
			return nil, fmt.Errorf("invalid ids")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		results = append(results, id)
	}
	return results, nil
}

func parseSelectedSoldierIDs(values []string) ([]int64, error) {
	selected := make([]int64, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		id, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil || id < 1 {
			return nil, fmt.Errorf("invalid review queue selection")
		}
		selected = append(selected, id)
	}
	return selected, nil
}

func compareIDsFromRequest(r *http.Request) (int64, int64, error) {
	id1, err1 := parseOptionalInt64(strings.TrimSpace(r.URL.Query().Get("id1")), "id1")
	id2, err2 := parseOptionalInt64(strings.TrimSpace(r.URL.Query().Get("id2")), "id2")
	if err1 == nil && err2 == nil && id1 > 0 && id2 > 0 {
		if id1 == id2 {
			return 0, 0, fmt.Errorf("choose two different records to compare")
		}
		return id1, id2, nil
	}
	values := r.URL.Query()["compare_ids"]
	if len(values) != 2 {
		return 0, 0, fmt.Errorf("choose exactly two records to compare")
	}
	selected, err := parseSelectedSoldierIDs(values)
	if err != nil || len(selected) != 2 {
		return 0, 0, fmt.Errorf("choose exactly two records to compare")
	}
	if selected[0] == selected[1] {
		return 0, 0, fmt.Errorf("choose two different records to compare")
	}
	return selected[0], selected[1], nil
}

func (a *App) attachDetailBackLink(soldier *models.Soldier, fromValue string) error {
	if soldier == nil || fromValue == "" {
		return nil
	}
	fromID, err := strconv.ParseInt(fromValue, 10, 64)
	if err != nil || fromID < 1 || fromID == soldier.ID {
		return nil
	}
	source, err := a.soldiers.GetByID(fromID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	soldier.BackLinkURL = fmt.Sprintf("/soldiers/%d", fromID)
	soldier.BackLinkLabel = "Back to " + linkedRecordLabel(*source)
	return nil
}

func linkedRecordLabel(s models.Soldier) string {
	switch strings.TrimSpace(strings.ToLower(s.EntryType)) {
	case "wife":
		return "Wife Record"
	case "widow":
		return "Widow Record"
	default:
		return "Soldier Record"
	}
}

func (a *App) saveUploadedImages(r *http.Request, soldier models.Soldier) error {
	if r.MultipartForm == nil || len(r.MultipartForm.File["images"]) == 0 {
		return nil
	}

	recordDir, relativeDir := appdata.RecordImageDir(a.dataDir, soldier.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return fmt.Errorf("create image directory: %w", err)
	}
	namePrefix := filepath.Base(relativeDir)
	nextSequence, err := nextStoredImageSequence(recordDir, namePrefix)
	if err != nil {
		return fmt.Errorf("prepare image filenames: %w", err)
	}

	var issues []string
	for _, fileHeader := range r.MultipartForm.File["images"] {
		if fileHeader == nil || fileHeader.Filename == "" {
			continue
		}
		if !isAllowedImageFile(fileHeader.Filename) {
			issues = append(issues, fmt.Sprintf("unsupported image file: %s", fileHeader.Filename))
			continue
		}

		storedName := standardizedImageFileName(namePrefix, nextSequence, fileHeader.Filename)
		absolutePath := filepath.Join(recordDir, storedName)
		relativePath := filepath.Join(relativeDir, storedName)

		if err := saveUploadedFile(fileHeader, absolutePath); err != nil {
			issues = append(issues, err.Error())
			continue
		}
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, ""); err != nil {
			_ = os.Remove(absolutePath)
			issues = append(issues, err.Error())
			continue
		}
		nextSequence++
	}

	if len(issues) > 0 {
		return errors.New(strings.Join(issues, "; "))
	}
	return nil
}

func (a *App) importImagePaths(soldier models.Soldier, paths []string) (int, error) {
	recordDir, relativeDir := appdata.RecordImageDir(a.dataDir, soldier.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return 0, fmt.Errorf("create image directory: %w", err)
	}
	namePrefix := filepath.Base(relativeDir)
	nextSequence, err := nextStoredImageSequence(recordDir, namePrefix)
	if err != nil {
		return 0, fmt.Errorf("prepare image filenames: %w", err)
	}

	imported := 0
	var issues []string
	for _, sourcePath := range paths {
		sourcePath = strings.TrimSpace(sourcePath)
		if sourcePath == "" {
			continue
		}
		fileName := filepath.Base(sourcePath)
		if !isAllowedImageFile(fileName) {
			issues = append(issues, fmt.Sprintf("unsupported image file: %s", fileName))
			continue
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			issues = append(issues, fmt.Sprintf("read image file %s: %v", fileName, err))
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			issues = append(issues, fmt.Sprintf("image file %s is empty", fileName))
			continue
		}

		storedName := standardizedImageFileName(namePrefix, nextSequence, fileName)
		absolutePath := filepath.Join(recordDir, storedName)
		relativePath := filepath.Join(relativeDir, storedName)

		if err := copyImageFile(sourcePath, absolutePath); err != nil {
			issues = append(issues, err.Error())
			continue
		}
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, ""); err != nil {
			_ = os.Remove(absolutePath)
			issues = append(issues, err.Error())
			continue
		}
		imported++
		nextSequence++
	}

	if len(issues) > 0 {
		return imported, errors.New(strings.Join(issues, "; "))
	}
	return imported, nil
}

func saveUploadedFile(fileHeader *multipart.FileHeader, destination string) error {
	src, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("open upload %s: %w", fileHeader.Filename, err)
	}
	defer src.Close()

	dst, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create image file %s: %w", destination, err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("write image file %s: %w", destination, err)
	}
	if written == 0 {
		dst.Close()
		_ = os.Remove(destination)
		return fmt.Errorf("image file %s is empty", fileHeader.Filename)
	}
	return nil
}

func copyImageFile(sourcePath, destination string) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open image file %s: %w", filepath.Base(sourcePath), err)
	}
	defer src.Close()

	dst, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create image file %s: %w", destination, err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("write image file %s: %w", destination, err)
	}
	if written == 0 {
		dst.Close()
		_ = os.Remove(destination)
		return fmt.Errorf("image file %s is empty", filepath.Base(sourcePath))
	}
	return nil
}

func standardizedImageFileName(prefix string, sequence int, originalName string) string {
	return fmt.Sprintf("%s-img-%03d%s", strings.TrimSpace(prefix), sequence, normalizedImageExtension(originalName))
}

func nextStoredImageSequence(recordDir, prefix string) (int, error) {
	entries, err := os.ReadDir(recordDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 1, nil
		}
		return 0, err
	}

	maxSequence := 0
	patternPrefix := prefix + "-img-"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, patternPrefix) {
			continue
		}
		base := strings.TrimSuffix(name, filepath.Ext(name))
		sequenceText := strings.TrimPrefix(base, patternPrefix)
		sequence, err := strconv.Atoi(sequenceText)
		if err != nil {
			continue
		}
		if sequence > maxSequence {
			maxSequence = sequence
		}
	}
	return maxSequence + 1, nil
}

func normalizedImageExtension(name string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".jpg":
		return ".jpg"
	case ".jpeg":
		return ".jpeg"
	case ".png":
		return ".png"
	case ".gif":
		return ".gif"
	case ".webp":
		return ".webp"
	case ".bmp":
		return ".bmp"
	case ".svg":
		return ".svg"
	default:
		return ".img"
	}
}

func isAllowedImageFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	default:
		return false
	}
}
