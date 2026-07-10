// research_handlers.go holds the per-soldier research HTTP handlers:
// service timeline, research log, and research task create + resolve.
// Extracted from app.go as step 9 of the God-class reduction tracked
// in issue #42. Handlers stay on *App; routes registered in routes.go.
// The handleResearchLog function dispatches to handleResearchTaskCreate
// and handleResearchTaskResolve based on URL parts.
//
// Issue #455 slice 3: handleUnitCamaraderie + handleResearchPack
// deleted. Their data lives on Insights (Camaraderie's scope=unit
// drilldown + Research Pack's Top Units / Top Cemeteries panels)
// and the use cases route there from the soldier_card tile and the
// R&R foldout.
//
// Issue #455 slice 4: handleConflictLedger deleted. The
// per-Soldier merge conflict ledger view now lives on the Review
// Queue Resolved tab (issue #378 follow-up deferred — the merge
// review_conflicts table stays for the pending + resolved audit
// views; the per-Person sub-page is dropped).
//
// Issue #422 slice 1: empty-state pages replace 500 errors for
// soldiers with missing data. Error dispatch normalized:
// sql.ErrNoRows → 404, validation errors → 400, everything else
// → 500.
package appshell

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/presentation"
)

func (a *App) handleServiceTimeline(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	timeline, err := a.soldiers.ServiceTimeline(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			respondNotFound(w, r, fmt.Sprintf("Service timeline for person record %d is unavailable.", id), err)
			return
		}
		respondInternal(w, r, "Could not build the service timeline.", err)
		return
	}
	// Issue #384 / Slice 4: wrap Render.
	if err := presentation.ServiceTimelineView(*timeline).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the service timeline.", err)
	}
}

func (a *App) handleResearchLog(w http.ResponseWriter, r *http.Request, id int64, parts []string) {
	if len(parts) == 1 && r.Method == http.MethodGet {
		log, err := a.soldiers.ResearchLog(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
				return
			}
			respondInternal(w, r, fmt.Sprintf("Could not build research log for record %d.", id), err)
			return
		}
		// Issue #384 / Slice 4: wrap Render.
		if err := presentation.ResearchLogView(*log).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, fmt.Sprintf("Could not render the research log for person record %d.", id), err)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "tasks" && r.Method == http.MethodPost {
		a.handleResearchTaskCreate(w, r, id)
		return
	}
	if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "resolve" && r.Method == http.MethodPost {
		taskID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			respondValidation(w, r, "Invalid research task id.", err)
			return
		}
		a.handleResearchTaskResolve(w, r, id, taskID)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (a *App) handleResearchTaskCreate(w http.ResponseWriter, r *http.Request, id int64) {
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the research task form.", err)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	evidenceType := strings.TrimSpace(r.FormValue("evidence_type"))
	// Issue #422: validate title before calling the service so a
	// blank form returns 400 (validation), not 500.
	if title == "" {
		respondValidation(w, r, "Research task title is required.", nil)
		return
	}
	if err := a.soldiers.AddResearchTask(id, title, notes, evidenceType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
			return
		}
		setToastHeaderWithType(w, "Research task could not be saved.", "error")
		respondInternal(w, r, fmt.Sprintf("Could not save research task for record %d.", id), err)
		return
	}
	setToastHeader(w, "Success: research task added.")
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d/research-log", id))
	fmt.Fprint(w, "Research task saved.")
}

func (a *App) handleResearchTaskResolve(w http.ResponseWriter, r *http.Request, id, taskID int64) {
	if err := a.soldiers.ResolveResearchTask(id, taskID); err != nil {
		// Issue #422: the service returns "research task not found"
		// when the task doesn't exist. Map to 404 instead of 500.
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			respondNotFound(w, r, fmt.Sprintf("Research task %d not found for person record %d.", taskID, id), err)
			return
		}
		setToastHeaderWithType(w, "Research task could not be resolved.", "error")
		respondInternal(w, r, fmt.Sprintf("Could not resolve research task %d for record %d.", taskID, id), err)
		return
	}
	setToastHeader(w, "Success: research task resolved.")
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d/research-log", id))
	fmt.Fprint(w, "Research task resolved.")
}

func (a *App) handleConflictLedger(w http.ResponseWriter, r *http.Request, id int64) {
	// Issue #455 slice 4: per-Soldier conflict-ledger sub-page
	// removed; same data surfaces on the Review Queue Resolved
	// tab. The route registration in routes.go is gone and the
	// dispatch branch in soldiers_handlers.go is removed; this
	// stub remains so any stale URL returns 303→/review-queue
	// instead of 404 (data lives, just on a different surface).
	// Replaced in follow-up work with the Resolved tab global
	// listing.
	target := "/review-queue?tab=resolved"
	w.Header().Set("Location", target)
	w.Header().Set("X-DixieData-Redirect", target)
	w.WriteHeader(http.StatusSeeOther)
}
