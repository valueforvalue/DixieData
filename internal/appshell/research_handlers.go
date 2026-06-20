// research_handlers.go holds the per-soldier research HTTP handlers:
// unit camaraderie, service timeline, research log, research task create +
// resolve, conflict ledger, and research pack. Extracted from app.go
// as step 9 of the God-class reduction tracked in issue #42. Handlers
// stay on *App; routes registered in routes.go. The handleResearchLog
// function dispatches to handleResearchTaskCreate and handleResearchTaskResolve
// based on URL parts.
package appshell

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/presentation"
)

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
