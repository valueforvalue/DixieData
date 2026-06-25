// jobs_handlers.go holds the /jobs/{id} status page handler, the polling
// fragment handler, and the POST /jobs/{id}/cancel endpoint. Extracted
// from app.go as part of the audit-fallout work tracked under issue
// #100. The handlers are read-only with respect to the registry; they
// never mutate job state directly.
package appshell

import (
	"errors"
	"net/http"
	"strings"

	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/templates"
)

// handleJobStatus routes /jobs/{id}, /jobs/{id}/status, and
// /jobs/{id}/cancel.
func (a *App) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/jobs/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		a.renderJobStatus(w, r, id, false)
		return
	}
	switch parts[1] {
	case "status":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.renderJobStatus(w, r, id, true)
	case "cancel":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.cancelJob(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) renderJobStatus(w http.ResponseWriter, r *http.Request, id string, fragmentOnly bool) {
	job, ok := a.jobs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if fragmentOnly {
		presentation.JobStatusFragment(job).Render(r.Context(), w)
		return
	}
	templates.JobStatusView(job).Render(r.Context(), w)
}

func (a *App) cancelJob(w http.ResponseWriter, r *http.Request, id string) {
	switch err := a.jobs.Cancel(id); {
	case err == nil:
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
	case errors.Is(err, jobs.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, jobs.ErrAlreadyTerminal):
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
	default:
		respondInternal(w, r, "Could not cancel the export job.", err)
	}
}