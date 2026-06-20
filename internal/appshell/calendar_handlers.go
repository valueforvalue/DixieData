// calendar_handlers.go holds the calendar and anniversary HTTP handlers:
// the top-level /calendar and /calendar/ routes, the /anniversary/ route,
// the initial-setup route, the version probe, and the calendar grid +
// per-month PDF dispatch. Extracted from app.go as step 4 of the God-class
// reduction tracked in issue #42. Handlers stay on *App; routes registered
// in routes.go. handleCalendarPDF (a per-month PDF export) is registered
// in routes.go as /calendar/.../pdf and lives in calendar_pdf_handlers.go
// (it was already in app.go before this refactor and is moved separately).
package appshell

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/presentation"
)


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
