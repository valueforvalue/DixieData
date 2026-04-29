package main

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
	"github.com/valueforvalue/DixieData/internal/templates"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx         context.Context
	database    *db.DB
	soldiers    *services.SoldierService
	anniversary *services.AnniversaryService
	export      *services.ExportService
	mux         *http.ServeMux
	startupErr  error
	dataDir     string
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.dataDir = appdata.DefaultDir()

	var err error
	a.database, err = db.Open(a.dataDir)
	if err != nil {
		a.startupErr = fmt.Errorf("failed to open database: %w", err)
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}

	a.soldiers = services.NewSoldierService(a.database)
	a.anniversary = services.NewAnniversaryService(a.database)
	a.export = services.NewExportService(a.database, a.soldiers)

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
	mux.HandleFunc("/export/json", a.handleExportJSON)
	mux.HandleFunc("/export/csv", a.handleExportCSV)
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
	templates.Calendar(month, calendar).Render(r.Context(), w)
}

func (a *App) handleCalendarMonth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/calendar/"), "/")
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
	templates.Calendar(month, calendar).Render(r.Context(), w)
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
	templates.ExportView().Render(r.Context(), w)
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
		Notes:      r.FormValue("notes"),
		DeathYear:  deathYear,
		DeathMonth: deathMonth,
		DeathDay:   deathDay,
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
