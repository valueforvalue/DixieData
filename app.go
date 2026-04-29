package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dataDir := filepath.Join(home, ".dixiedata")

	a.database, err = db.Open(dataDir)
	if err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
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
	mux.HandleFunc("/calendar/", a.handleCalendarMonth)
	mux.HandleFunc("/anniversary/", a.handleAnniversary)
	mux.HandleFunc("/soldiers", a.handleSoldiers)
	mux.HandleFunc("/soldiers/search", a.handleSearch)
	mux.HandleFunc("/soldiers/new", a.handleNewSoldier)
	mux.HandleFunc("/soldiers/", a.handleSoldierByID)
	mux.HandleFunc("/export", a.handleExport)
	mux.HandleFunc("/export/json", a.handleExportJSON)
	mux.HandleFunc("/export/csv", a.handleExportCSV)

	a.mux = mux
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

func (a *App) handleCalendar(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
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
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/calendar/"), "/")
	month := 1
	if len(parts) > 0 {
		if m, err := strconv.Atoi(parts[0]); err == nil && m >= 1 && m <= 12 {
			month = m
		}
	}
	calendar, err := a.anniversary.GetMonthCalendar(month)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	templates.Calendar(month, calendar).Render(r.Context(), w)
}

func (a *App) handleAnniversary(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/anniversary/"), "/")
	if len(parts) < 2 {
		http.Error(w, "bad request", 400)
		return
	}
	month, _ := strconv.Atoi(parts[0])
	day, _ := strconv.Atoi(parts[1])
	soldiers, err := a.anniversary.GetByMonthDay(month, day)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	templates.AnniversaryPartial(soldiers, month, day).Render(r.Context(), w)
}

func (a *App) handleSoldiers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a.handleCreateSoldier(w, r)
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
	templates.SoldierList(soldiers, page, total).Render(r.Context(), w)
}

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		templates.SearchResults([]models.Soldier{}).Render(r.Context(), w)
		return
	}
	soldiers, err := a.soldiers.Search(q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	templates.SearchResults(soldiers).Render(r.Context(), w)
}

func (a *App) handleNewSoldier(w http.ResponseWriter, r *http.Request) {
	templates.EntryForm(models.Soldier{}, false).Render(r.Context(), w)
}

func (a *App) handleCreateSoldier(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	s := models.Soldier{
		DisplayID: r.FormValue("display_id"),
		FirstName: r.FormValue("first_name"),
		LastName:  r.FormValue("last_name"),
		Rank:      r.FormValue("rank"),
		Unit:      r.FormValue("unit"),
		BirthInfo: r.FormValue("birth_info"),
		Notes:     r.FormValue("notes"),
	}
	s.DeathYear, _ = strconv.Atoi(r.FormValue("death_year"))
	s.DeathMonth, _ = strconv.Atoi(r.FormValue("death_month"))
	s.DeathDay, _ = strconv.Atoi(r.FormValue("death_day"))

	created, err := a.soldiers.Create(s)
	if err != nil {
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
	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	templates.EntryForm(*soldier, true).Render(r.Context(), w)
}

func (a *App) handleUpdateSoldier(w http.ResponseWriter, r *http.Request, id int64) {
	r.ParseForm()
	s := models.Soldier{
		ID:        id,
		DisplayID: r.FormValue("display_id"),
		FirstName: r.FormValue("first_name"),
		LastName:  r.FormValue("last_name"),
		Rank:      r.FormValue("rank"),
		Unit:      r.FormValue("unit"),
		BirthInfo: r.FormValue("birth_info"),
		Notes:     r.FormValue("notes"),
	}
	s.DeathYear, _ = strconv.Atoi(r.FormValue("death_year"))
	s.DeathMonth, _ = strconv.Atoi(r.FormValue("death_month"))
	s.DeathDay, _ = strconv.Atoi(r.FormValue("death_day"))

	if err := a.soldiers.Update(s); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/soldiers/%d", id), http.StatusSeeOther)
}

func (a *App) handleExport(w http.ResponseWriter, r *http.Request) {
	templates.ExportView().Render(r.Context(), w)
}

func (a *App) handleExportJSON(w http.ResponseWriter, r *http.Request) {
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
