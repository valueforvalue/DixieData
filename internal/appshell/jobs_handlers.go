// jobs_handlers.go holds the /jobs/{id} status page handler, the polling
// fragment handler, and the POST /jobs/{id}/cancel endpoint. Extracted
// from app.go as part of the audit-fallout work tracked under issue
// #100. The handlers are read-only with respect to the registry; they
// never mutate job state directly.
package appshell

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
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
	case "artifact":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.streamJobArtifact(w, r, id)
	case "report":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.renderJobReport(w, r, id)
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
	if a.jobs == nil {
		http.NotFound(w, r)
		return
	}
	job, ok := a.jobs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("slot") == "1" {
		presentation.JobStatusSlotFragment(job).Render(r.Context(), w)
		return
	}
	if fragmentOnly {
		presentation.JobStatusFragment(job).Render(r.Context(), w)
		return
	}
	templates.JobStatusView(job).Render(r.Context(), w)
}

// renderJobReport serves /jobs/{id}/report. Renders the job's
// terminal-state summary card plus a structured report payload
// (timeline, artifact metadata, error log when present) on a
// printable layout so the user can save or share it without the
// live-polling scaffolding of the status view. Returns 404 when
// the job is unknown; falls through to a minimal "still running"
// report when the job is queued or running so a refresh during
// the export doesn't dead-end the user.
func (a *App) renderJobReport(w http.ResponseWriter, r *http.Request, id string) {
	if a.jobs == nil {
		http.NotFound(w, r)
		return
	}
	job, ok := a.jobs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	templates.JobReportView(job).Render(r.Context(), w)
}

// renderActiveJob serves /jobs/active: returns the slot variant of
// the most recent queued/running job, or 204 No Content when none.
// The layout progress slot polls this every 3s via htmx, so it
// surfaces whatever background task the user kicked off most recently
// regardless of the page they are on.
func (a *App) renderActiveJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.jobs == nil {
		// jobs registry not initialized — happens during very early
		// startup or in tests that don't wire the registry. Treat
		// as "no active job" so the layout progress slot stays
		// empty instead of crashing the request goroutine.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	job := a.jobs.MostRecentActive()
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	presentation.JobStatusSlotFragment(*job).Render(r.Context(), w)
}

func (a *App) cancelJob(w http.ResponseWriter, r *http.Request, id string) {
	if a.jobs == nil {
		http.NotFound(w, r)
		return
	}
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

// streamJobArtifact streams the saved file at job.ResultPath back to the
// browser with a Content-Disposition header so the file downloads or
// opens in the user's default app. The endpoint is only meaningful when
// the job is in the done state with a populated ResultPath; otherwise
// respond with 404 or 409 to make the failure mode clear.
func (a *App) streamJobArtifact(w http.ResponseWriter, r *http.Request, id string) {
	if a.jobs == nil {
		http.NotFound(w, r)
		return
	}
	job, ok := a.jobs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if job.ResultPath == "" {
		http.Error(w, "export has no artifact yet", http.StatusConflict)
		return
	}
	path := strings.TrimSpace(job.ResultPath)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "export artifact is missing on disk", http.StatusGone)
			return
		}
		respondInternal(w, r, "Could not open the export artifact.", err)
		return
	}
	w.Header().Set("Content-Length", formatContentLength(info.Size()))
	disposition, contentType := jobArtifactHeaders(path)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", disposition)
	http.ServeFile(w, r, path)
}

// jobArtifactHeaders returns the Content-Disposition and
// Content-Type the artifact stream should send based on the
// file's extension. Viewable types (PDF, images, HTML, text)
// are served inline so the user sees them render in the new
// tab the "Open" link opens; everything else (.ddbak, .ddshare,
// .zip, .json, .csv, .ics) downloads via Content-Disposition:
// attachment.
//
// Before this split, every artifact forced
// Content-Disposition: attachment, so opening a finished
// export's "Open" link in a new tab immediately triggered
// a download and left the user looking at a blank tab.
// See docs/agents/jobs-artifact-content-disposition-bug.md
// (the bug was reported by Jeremy on 2026-06-28 while
// testing a ddbak export; the same code path hit PDFs
// and JPGs and made the GUI feel broken).
func jobArtifactHeaders(path string) (disposition, contentType string) {
	ext := strings.ToLower(filepath.Ext(path))
	filename := filepath.Base(path)
	disposition = "attachment; filename=\"" + filename + "\""
	contentType = "application/octet-stream"
	if mime, ok := jobArtifactMimeByExt[ext]; ok {
		disposition = "inline; filename=\"" + filename + "\""
		contentType = mime
	}
	return disposition, contentType
}

// jobArtifactMimeByExt maps the file extensions the GUI's
// "Open" link sends inline to the MIME type the browser
// needs to render them. Add a new entry only when the
// extension is something the user is expected to view in
// the browser, not download.
var jobArtifactMimeByExt = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".html": "text/html; charset=utf-8",
	".htm":  "text/html; charset=utf-8",
	".txt":  "text/plain; charset=utf-8",
	".json": "application/json; charset=utf-8",
}

func formatContentLength(n int64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	return string(buf)
}