// jobs_handlers.go holds the /jobs/{id} status page handler, the polling
// fragment handler, and the POST /jobs/{id}/cancel endpoint. Extracted
// from app.go as part of the audit-fallout work tracked under issue
// #100. The handlers are read-only with respect to the registry; they
// never mutate job state directly.
package appshell

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	case "stream":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.streamJobProgress(w, r, id)
	case "artifact":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.streamJobArtifact(w, r, id)
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

// streamJobProgress streams job snapshots as Server-Sent Events. The
// browser opens an EventSource on /jobs/{id}/stream; each Progress.Set
// or state transition fires a data: <json>\n\n frame. The stream
// terminates as soon as the job reaches a terminal state
// (done / error / cancelled / interrupted).
//
// Clients without EventSource support can keep using the existing
// /jobs/{id}/status polling endpoint; the two paths coexist.
func (a *App) streamJobProgress(w http.ResponseWriter, r *http.Request, id string) {
	job, ok := a.jobs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Push the current snapshot first so a slow worker that has not
	// called Progress.Set yet still produces a meaningful first event.
	writeJobEvent(w, job)
	flusher.Flush()

	if isTerminalJobStatus(job.Status) {
		return
	}

	sub := a.jobs.Subscribe(id)
	defer a.jobs.Unsubscribe(id, sub)

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case snap, ok := <-sub:
			if !ok {
				return
			}
			writeJobEvent(w, snap)
			flusher.Flush()
			if isTerminalJobStatus(snap.Status) {
				return
			}
		case <-keepalive.C:
			// Comment line keeps the connection warm through proxies
			// that close idle sockets.
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeJobEvent(w http.ResponseWriter, job jobs.Job) {
	payload, err := json.Marshal(job)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: progress\ndata: %s\n\n", payload)
}

func isTerminalJobStatus(status string) bool {
	switch status {
	case jobs.StatusDone, jobs.StatusError, jobs.StatusCancelled, jobs.StatusInterrupted:
		return true
	}
	return false
}

// streamJobArtifact streams the saved file at job.ResultPath back to the
// browser with a Content-Disposition header so the file downloads or
// opens in the user's default app. The endpoint is only meaningful when
// the job is in the done state with a populated ResultPath; otherwise
// respond with 404 or 409 to make the failure mode clear.
func (a *App) streamJobArtifact(w http.ResponseWriter, r *http.Request, id string) {
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
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", formatContentLength(info.Size()))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(path)+"\"")
	http.ServeFile(w, r, path)
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