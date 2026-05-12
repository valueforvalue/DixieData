package main

import (
	"context"
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
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "embed"
	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
	"github.com/valueforvalue/DixieData/internal/templates"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed quotes.json
var embeddedQuotes []byte

type App struct {
	ctx         context.Context
	database    *db.DB
	soldiers    *services.SoldierService
	anniversary *services.AnniversaryService
	export      *services.ExportService
	backup      *services.BackupService
	google      *services.GoogleService
	quotes      []models.Quote
	mux         *http.ServeMux
	startupErr  error
	dataDir     string
}

const initializeDataConfirmationWord = "INITIALIZE"

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.dataDir = appdata.DefaultDir()
	var err error
	a.quotes, err = loadQuotes(embeddedQuotes)
	if err != nil {
		a.startupErr = fmt.Errorf("failed to load quotes: %w", err)
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}

	a.database, err = db.Open(a.dataDir)
	if err != nil {
		a.startupErr = fmt.Errorf("failed to open database: %w", err)
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}

	if err := a.reloadServices(); err != nil {
		a.startupErr = err
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}

	a.setupRoutes()
}

func (a *App) shutdown(ctx context.Context) {
	if a.database != nil {
		a.database.Close()
	}
}

func (a *App) setupRoutes() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", a.handleCalendar)
	mux.HandleFunc("/calendar", a.handleCalendar)
	mux.HandleFunc("/calendar/", a.handleCalendarMonth)
	mux.HandleFunc("/anniversary/", a.handleAnniversary)
	mux.HandleFunc("/soldiers", a.handleSoldiers)
	mux.HandleFunc("/soldiers/search", a.handleSearch)
	mux.HandleFunc("/soldiers/search/advanced", a.handleAdvancedSearch)
	mux.HandleFunc("/soldiers/new", a.handleNewSoldier)
	mux.HandleFunc("/soldiers/", a.handleSoldierByID)
	mux.HandleFunc("/export", a.handleExport)
	mux.HandleFunc("/settings", a.handleSettings)
	mux.HandleFunc("/settings/initialize", a.handleSettingsInitialize)
	mux.HandleFunc("/export/json", a.handleExportJSON)
	mux.HandleFunc("/export/csv", a.handleExportCSV)
	mux.HandleFunc("/export/ical", a.handleExportICalendar)
	mux.HandleFunc("/export/backup", a.handleExportBackup)
	mux.HandleFunc("/import/backup", a.handleImportBackup)
	mux.HandleFunc("/integrations/google/connect", a.handleGoogleConnect)
	mux.HandleFunc("/integrations/google/disconnect", a.handleGoogleDisconnect)
	mux.HandleFunc("/integrations/google/backup", a.handleGoogleBackup)
	mux.HandleFunc("/integrations/google/sheets/export", a.handleGoogleSheetsExport)
	mux.HandleFunc("/integrations/google/calendar/sync", a.handleGoogleCalendarSync)
	mux.HandleFunc("/integrations/google/calendar/unsync", a.handleGoogleCalendarUnsync)
	mux.HandleFunc("/images/screenshot", a.handleImageScreenshot)
	mux.HandleFunc("/images/rotate", a.handleImageRotate)
	mux.HandleFunc("/open-link", a.handleOpenLink)
	mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir(a.dataDir))))

	a.mux = mux
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.mux == nil {
		http.Error(w, "Application is still starting up.", http.StatusServiceUnavailable)
		return
	}
	if a.startupErr != nil {
		http.Error(w, a.startupErr.Error(), http.StatusInternalServerError)
		return
	}
	if override := requestMethodOverride(r); override != "" {
		r = r.Clone(r.Context())
		r.Method = override
	}
	a.mux.ServeHTTP(w, r)
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
	month := 1
	calendar, err := a.anniversary.GetMonthCalendar(month)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_, total, err := a.soldiers.List(1, 1)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	templates.Calendar(month, calendar, total, selectQuoteOfDay(a.quotes, time.Now())).Render(r.Context(), w)
}

func (a *App) handleCalendarMonth(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/calendar/"), "/")
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
	calendar, err := a.anniversary.GetMonthCalendar(month)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_, total, err := a.soldiers.List(1, 1)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	templates.Calendar(month, calendar, total, selectQuoteOfDay(a.quotes, time.Now())).Render(r.Context(), w)
}

func (a *App) handleAnniversary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
	soldiers, err := a.anniversary.GetByMonthDay(month, day)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	templates.AnniversaryPartial(soldiers, month, day).Render(r.Context(), w)
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
	templates.SoldierList(nil, page, 0, "").Render(r.Context(), w)
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
		templates.SearchResults(nil, search, page, 0, 50).Render(r.Context(), w)
		return
	}
	if strings.TrimSpace(q) == "" && search.Browse {
		soldiers, total, err := a.soldiers.List(page, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		templates.SearchResults(soldiers, search, page, total, 50).Render(r.Context(), w)
		return
	}
	soldiers, total, err := a.soldiers.SearchPage(q, page, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	templates.SearchResults(soldiers, search, page, total, 50).Render(r.Context(), w)
}

func (a *App) handleAdvancedSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	search := models.SoldierSearch{
		Mode:         "advanced",
		DisplayID:    r.URL.Query().Get("display_id"),
		FirstName:    r.URL.Query().Get("first_name"),
		MiddleName:   r.URL.Query().Get("middle_name"),
		LastName:     r.URL.Query().Get("last_name"),
		Rank:         r.URL.Query().Get("rank"),
		Unit:         r.URL.Query().Get("unit"),
		PensionState: r.URL.Query().Get("pension_state"),
		BuriedIn:     r.URL.Query().Get("buried_in"),
		DeathYear:    r.URL.Query().Get("death_year"),
		DeathMonth:   r.URL.Query().Get("death_month"),
		DeathDay:     r.URL.Query().Get("death_day"),
	}
	page := parsePage(r.URL.Query().Get("page"))
	if !hasAdvancedSearchInput(search) {
		templates.SearchResults(nil, search, page, 0, 50).Render(r.Context(), w)
		return
	}

	soldiers, total, err := a.soldiers.AdvancedSearch(search, page, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	templates.SearchResults(soldiers, search, page, total, 50).Render(r.Context(), w)
}

func hasAdvancedSearchInput(search models.SoldierSearch) bool {
	return strings.TrimSpace(search.DisplayID) != "" ||
		strings.TrimSpace(search.FirstName) != "" ||
		strings.TrimSpace(search.MiddleName) != "" ||
		strings.TrimSpace(search.LastName) != "" ||
		strings.TrimSpace(search.Rank) != "" ||
		strings.TrimSpace(search.Unit) != "" ||
		strings.TrimSpace(search.PensionState) != "" ||
		strings.TrimSpace(search.BuriedIn) != "" ||
		strings.TrimSpace(search.DeathYear) != "" ||
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
	templates.EntryForm(defaults, false).Render(r.Context(), w)
}

func (a *App) handleCreateSoldier(w http.ResponseWriter, r *http.Request) {
	s, err := parseSoldierForm(r, 0)
	if err != nil {
		defaults, defaultsErr := a.newSoldierDefaults()
		if defaultsErr != nil {
			http.Error(w, defaultsErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		templates.EntryFormWithError(defaults, false, err.Error()).Render(r.Context(), w)
		return
	}

	created, err := a.soldiers.Create(s)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templates.EntryFormWithError(s, false, err.Error()).Render(r.Context(), w)
		return
	}
	if err := a.saveUploadedImages(r, *created); err != nil {
		reloaded, reloadErr := a.soldiers.GetByID(created.ID)
		if reloadErr != nil {
			reloaded = created
		}
		w.WriteHeader(http.StatusBadRequest)
		templates.EntryFormWithError(*reloaded, true, err.Error()).Render(r.Context(), w)
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

	switch r.Method {
	case http.MethodGet:
		soldier, err := a.soldiers.GetByID(id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		templates.SoldierDetail(*soldier).Render(r.Context(), w)
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
	templates.EntryForm(*soldier, true).Render(r.Context(), w)
}

func (a *App) handleUpdateSoldier(w http.ResponseWriter, r *http.Request, id int64) {
	s, err := parseSoldierForm(r, id)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templates.EntryFormWithError(models.Soldier{ID: id}, true, err.Error()).Render(r.Context(), w)
		return
	}

	if err := a.soldiers.Update(s); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templates.EntryFormWithError(s, true, err.Error()).Render(r.Context(), w)
		return
	}
	if err := a.saveUploadedImages(r, s); err != nil {
		reloaded, reloadErr := a.soldiers.GetByID(id)
		if reloadErr != nil {
			reloaded = &s
		}
		w.WriteHeader(http.StatusBadRequest)
		templates.EntryFormWithError(*reloaded, true, err.Error()).Render(r.Context(), w)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/soldiers/%d", id), http.StatusSeeOther)
}

func (a *App) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := a.google.Status()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.ExportView(status).Render(r.Context(), w)
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	templates.SettingsView(initializeDataConfirmationWord).Render(r.Context(), w)
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

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.csv",
		Filters: []runtime.FileFilter{
			{DisplayName: "CSV", Pattern: "*.csv"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprintf(w, "Export cancelled.")
		return
	}
	if err := a.export.ExportCSV(path); err != nil {
		fmt.Fprintf(w, "Export failed: %v", err)
		return
	}
	fmt.Fprintf(w, "✓ Exported to %s", path)
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
	if err := a.export.ExportICalendar(path); err != nil {
		fmt.Fprintf(w, "iCalendar export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("iCalendar ready:", path))
}

func (a *App) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: backupArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "Backup archive", Pattern: "*.zip"},
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

func (a *App) handleImportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Backup archive", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Backup import cancelled.")
		return
	}

	if a.database != nil {
		a.database.Close()
		a.database = nil
	}

	manifest, err := a.backup.Import(path, a.dataDir)
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
	fmt.Fprintf(w, "Backup loaded: %d soldiers, %d records, %d images.", manifest.Soldiers, manifest.Records, manifest.Images)
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

func (a *App) handleGoogleCalendarSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	result, err := a.google.SyncCalendar(r.Context(), settings, soldiers)
	if err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "Google Calendar synced: %d created, %d updated, %d deleted, %d skipped.", result.Created, result.Updated, result.Deleted, result.Skipped)
}

func (a *App) handleGoogleCalendarUnsync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.UnsyncCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar unsync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "Google Calendar unsynced: %d event(s) removed.", result.Deleted)
}

func (a *App) handleSoldierPDF(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: soldierPDFName(*soldier),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "PDF export cancelled.")
		return
	}
	if err := a.export.ExportSoldierPDF(path, *soldier); err != nil {
		fmt.Fprintf(w, "PDF export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("PDF ready:", path))
}

func (a *App) handleCalendarPDF(w http.ResponseWriter, r *http.Request, monthValue string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: monthPDFName(month),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Monthly PDF export cancelled.")
		return
	}
	if err := a.export.ExportMonthlyAnniversaryPDF(path, month, calendar); err != nil {
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

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: imageArchiveName(*soldier),
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP archive", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Download cancelled.")
		return
	}
	if err := a.export.ExportImages(path, selected); err != nil {
		fmt.Fprintf(w, "Download failed: %v", err)
		return
	}
	fmt.Fprintf(w, "✓ Saved %d image(s) to %s", len(selected), path)
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

	deathYear, err := parseOptionalInt(r.FormValue("death_year"), "death_year")
	if err != nil {
		return models.Soldier{}, err
	}
	deathMonth, err := parseOptionalBoundedInt(r.FormValue("death_month"), "death_month", 0, 12)
	if err != nil {
		return models.Soldier{}, err
	}
	deathDay, err := parseOptionalBoundedInt(r.FormValue("death_day"), "death_day", 0, 31)
	if err != nil {
		return models.Soldier{}, err
	}

	return models.Soldier{
		ID:            id,
		DisplayID:     r.FormValue("display_id"),
		PensionID:     r.FormValue("pension_id"),
		ApplicationID: r.FormValue("application_id"),
		FirstName:     r.FormValue("first_name"),
		MiddleName:    r.FormValue("middle_name"),
		LastName:      r.FormValue("last_name"),
		Rank:          r.FormValue("rank_out"),
		RankIn:        r.FormValue("rank_in"),
		RankOut:       r.FormValue("rank_out"),
		Unit:          r.FormValue("unit"),
		PensionState:  r.FormValue("pension_state"),
		BirthInfo:     r.FormValue("birth_info"),
		BuriedIn:      r.FormValue("buried_in"),
		Notes:         r.FormValue("notes"),
		DeathYear:     deathYear,
		DeathMonth:    deathMonth,
		DeathDay:      deathDay,
		Records:       parseRecordInputs(r),
	}, nil
}

func (a *App) newSoldierDefaults() (models.Soldier, error) {
	displayID, err := a.database.NextDXDID()
	if err != nil {
		return models.Soldier{}, err
	}
	return models.Soldier{DisplayID: displayID}, nil
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

func parseOptionalBoundedInt(value, field string, min, max int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	return parseBoundedInt(trimmed, field, min, max)
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

func imageArchiveName(soldier models.Soldier) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = fmt.Sprintf("%s-%s", soldier.FirstName, soldier.LastName)
	}
	return sanitizedFileStem(base, "soldier-images") + "-images.zip"
}

func imageScreenshotName(fileName string) string {
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	return sanitizedFileStem(base, "archive-image") + "-screenshot.png"
}

func soldierPDFName(soldier models.Soldier) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return sanitizedFileStem(base, "soldier-record") + ".pdf"
}

func monthPDFName(month int) string {
	return sanitizedFileStem(fmt.Sprintf("%s report", monthNameValue(month)), "monthly-report") + ".pdf"
}

func backupArchiveName(now time.Time) string {
	return fmt.Sprintf("dixiedata-backup-%s.zip", now.Format("2006-01-02"))
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
	a.soldiers = services.NewSoldierService(a.database)
	a.anniversary = services.NewAnniversaryService(a.database)
	a.export = services.NewExportService(a.database, a.soldiers)
	a.backup = services.NewBackupService(a.soldiers)
	a.google = services.NewGoogleService(a.dataDir)
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

func selectQuoteOfDay(quotes []models.Quote, now time.Time) models.Quote {
	if len(quotes) == 0 {
		return models.Quote{}
	}
	index := (now.YearDay() - 1) % len(quotes)
	return quotes[index]
}

func parsePage(value string) int {
	page, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || page < 1 {
		return 1
	}
	return page
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
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, fileHeader.Filename); err != nil {
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
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, fileName); err != nil {
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
