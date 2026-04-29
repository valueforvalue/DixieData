package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"os"
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
	mux.HandleFunc("/integrations/google/calendar/sync", a.handleGoogleCalendarSync)
	mux.HandleFunc("/integrations/google/calendar/unsync", a.handleGoogleCalendarUnsync)
	mux.HandleFunc("/images/screenshot", a.handleImageScreenshot)
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
	a.mux.ServeHTTP(w, r)
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
	soldiers, total, err := a.soldiers.List(page, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	templates.SoldierList(soldiers, page, total, "").Render(r.Context(), w)
}

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query().Get("q")
	page := parsePage(r.URL.Query().Get("page"))
	search := models.SoldierSearch{Mode: "basic", Query: q}
	if strings.TrimSpace(q) == "" {
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
		Mode:       "advanced",
		DisplayID:  r.URL.Query().Get("display_id"),
		FirstName:  r.URL.Query().Get("first_name"),
		LastName:   r.URL.Query().Get("last_name"),
		Rank:       r.URL.Query().Get("rank"),
		Unit:       r.URL.Query().Get("unit"),
		BuriedIn:   r.URL.Query().Get("buried_in"),
		DeathYear:  r.URL.Query().Get("death_year"),
		DeathMonth: r.URL.Query().Get("death_month"),
		DeathDay:   r.URL.Query().Get("death_day"),
	}
	page := parsePage(r.URL.Query().Get("page"))

	soldiers, total, err := a.soldiers.AdvancedSearch(search, page, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	templates.SearchResults(soldiers, search, page, total, 50).Render(r.Context(), w)
}

func (a *App) handleNewSoldier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	templates.EntryForm(models.Soldier{}, false).Render(r.Context(), w)
}

func (a *App) handleCreateSoldier(w http.ResponseWriter, r *http.Request) {
	s, err := parseSoldierForm(r, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := a.soldiers.Create(s)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := a.saveUploadedImages(r, *created); err != nil {
		http.Error(w, err.Error(), 500)
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.soldiers.Update(s); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := a.saveUploadedImages(r, s); err != nil {
		http.Error(w, err.Error(), 500)
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
		ID:         id,
		DisplayID:  r.FormValue("display_id"),
		FirstName:  r.FormValue("first_name"),
		LastName:   r.FormValue("last_name"),
		Rank:       r.FormValue("rank"),
		Unit:       r.FormValue("unit"),
		BirthInfo:  r.FormValue("birth_info"),
		BuriedIn:   r.FormValue("buried_in"),
		Notes:      r.FormValue("notes"),
		DeathYear:  deathYear,
		DeathMonth: deathMonth,
		DeathDay:   deathDay,
		Records:    parseRecordInputs(r),
	}, nil
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
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">%s <a href="%s" class="pill-link" target="_blank" rel="noreferrer">%s</a></div>`,
		html.EscapeString(label),
		html.EscapeString(fileURL),
		html.EscapeString(path),
	)
}

func externalLinkMarkup(label, href, text string) string {
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">%s <a href="%s" class="pill-link" target="_blank" rel="noreferrer">%s</a></div>`,
		html.EscapeString(label),
		html.EscapeString(href),
		html.EscapeString(text),
	)
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

	for index, fileHeader := range r.MultipartForm.File["images"] {
		if fileHeader == nil || fileHeader.Filename == "" {
			continue
		}
		if !isAllowedImageFile(fileHeader.Filename) {
			return fmt.Errorf("unsupported image file: %s", fileHeader.Filename)
		}

		storedName := fmt.Sprintf("%d-%02d-%s", time.Now().UnixNano(), index+1, sanitizeUploadFileName(fileHeader.Filename))
		absolutePath := filepath.Join(recordDir, storedName)
		relativePath := filepath.Join(relativeDir, storedName)

		if err := saveUploadedFile(fileHeader, absolutePath); err != nil {
			return err
		}
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, fileHeader.Filename); err != nil {
			return err
		}
	}

	return nil
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

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("write image file %s: %w", destination, err)
	}
	return nil
}

func sanitizeUploadFileName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == "" {
		return "upload-image"
	}

	var builder strings.Builder
	for _, r := range base {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}

	safe := strings.Trim(builder.String(), "-")
	if safe == "" {
		return "upload-image"
	}
	return safe
}

func isAllowedImageFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	default:
		return false
	}
}
