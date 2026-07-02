// jobs_handlers.go holds the /jobs/{id} status page handler, the polling
// fragment handler, and the POST /jobs/{id}/cancel endpoint. Extracted
// from app.go as part of the audit-fallout work tracked under issue
// #100. The handlers are read-only with respect to the registry; they
// never mutate job state directly.
package appshell

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/valueforvalue/DixieData/internal/debug"
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
	case "open":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.openJobArtifact(w, r, id)
	case "log":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.streamJobLog(w, r, id)
	case "confirm":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.confirmJob(w, r, id)
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
	// Manual jobs (e.g. Memorial import) live in StatusQueued until
	// the user confirms. The registry's Cancel would mark them
	// StatusCancelled without ever running the worker; we use
	// the manual-job entry's cancel callback which does the same
	// thing plus forgets the entry so a subsequent /confirm call
	// returns ErrNotFound.
	err := a.cancelManualJob(id)
	if errors.Is(err, jobs.ErrNotFound) {
		// Not a manual job; fall through to the registry.
		err = a.jobs.Cancel(id)
	}
	switch {
	case err == nil:
		a.forgetManualJob(id)
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
	case errors.Is(err, jobs.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, jobs.ErrAlreadyTerminal):
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
	default:
		respondInternal(w, r, "Could not cancel the export job.", err)
	}
}

// confirmJob handles POST /jobs/{id}/confirm: releases the manual
// trigger for a confirm-before-run job (Memorial import today),
// flipping it from StatusQueued to StatusRunning. The page is the
// same /jobs/{id} status page; the user clicks a Confirm button
// instead of waiting for an external flow.
func (a *App) confirmJob(w http.ResponseWriter, r *http.Request, id string) {
	if a.jobs == nil {
		http.NotFound(w, r)
		return
	}
	job, ok := a.jobs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if job.Status != jobs.StatusQueued {
		// Defensive: already released (running / done / cancelled).
		// The /jobs/{id} page renders the actual state, so we just
		// bounce back with a friendly toast.
		setToastHeaderWithType(w, "Job already started.", "info")
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
		return
	}
	err := a.releaseManualJob(id)
	switch {
	case err == nil:
		a.forgetManualJob(id)
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
	case errors.Is(err, jobs.ErrNotFound):
		// Not a manual job — the /confirm endpoint is bound to
		// confirm-before-run kinds only. Send the user to the
		// status page; the page itself won't show a Confirm
		// button (toggled on a flag we set elsewhere) and will
		// explain what's happening.
		setToastHeaderWithType(w, "This job does not need confirmation.", "info")
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
	default:
		respondInternal(w, r, "Could not confirm the export job.", err)
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

// openJobArtifact handles POST /jobs/{id}/open: opens the saved
// artifact in the OS-default application using runtime.BrowserOpenURL.
//
// In the Wails desktop binary the file:// URL is dispatched to the
// host shell, which opens the file in the default application (e.g.
// Notepad for .json, Excel for .xlsx, Adobe Reader for .pdf). The
// /jobs/{id} status page no longer renders an inline "Open {label}"
// link pointing at the artifact endpoint, because the inline
// disposition produced a blank tab for application/json and most
// other types. The dedicated /jobs/{id}/open endpoint gives the user
// a single "Open file" affordance with one consistent code path.
//
// In the web-mode binary (dixiedata-web.exe), BrowserOpenURL returns
// errWailsFrontendUnavailable. The handler maps that to a
// clear info toast so the button doesn't fail silently; it also
// writes 303 + Location back to /jobs/{id} so the page reloads
// with the toast. Without a browser-to-OS bridge, web-mode users
// fall back to the "Copy path" affordance.
func (a *App) openJobArtifact(w http.ResponseWriter, r *http.Request, id string) {
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
		setToastHeaderWithType(w, "Export finished but the artifact path was not recorded.", "error")
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
		return
	}
	path := strings.TrimSpace(job.ResultPath)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			setToastHeaderWithType(w, "Export artifact is missing on disk.", "error")
			http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
			return
		}
		respondInternal(w, r, "Could not open the export artifact.", err)
		return
	}
	_ = info
	fileURL := "file:///" + strings.TrimLeft(filepath.ToSlash(path), "/")
	if err := a.BrowserOpenURL(fileURL); err != nil {
		// web-mode or any other environment without a Wails frontend
		// context. Surface a clear info toast rather than failing with
		// a generic 500; the user is on /jobs/{id} and the page reloads
		// to display the toast.
		// Keep as slog.Debug (not trace.Log) — narrative message + path/err
		// attrs make this a candidate for the always-on INFO+ log when an
		// operator investigates a web-mode support ticket. Migrating to
		// trace.Log would hide it behind DIXIEDATA_DEBUG=1 for no good
		// reason. See docs/adr/0006-slog-vs-trace-decision-tree.md.
		debug.FromContext(r.Context()).Debug("openJobArtifact: BrowserOpenURL unavailable in this runtime", "path", path, "err", err)
		setToastHeaderWithType(w, "Open file is only available in the desktop app. Use Copy Path to get the file location.", "info")
		http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
		return
	}
	// Wails actually opened the file. Hand the user back to the
	// status page with a success toast.
	setToastHeaderWithType(w, "Opening "+filepath.Base(path), "success")
	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

// streamJobLog streams the optional companion log file at
// job.Result.LogPath back to the browser with a
// Content-Disposition: attachment header so the file downloads.
// Used by jobSummaryCard's "Download log" button when LogPath is
// set (memorial import error logs).
//
// Path-traversal protection: the backend writes memorial logs to
// os.TempDir() via os.CreateTemp in writeMemorialImportErrorLog.
// We resolve the absolute path and verify it lives inside
// os.TempDir(); anything else returns 403. Combined with the
// job-lookup gate (only valid job IDs reach this handler), the
// user cannot pivot this endpoint into reading arbitrary files
// by manipulating LogPath.
func (a *App) streamJobLog(w http.ResponseWriter, r *http.Request, id string) {
	if a.jobs == nil {
		http.NotFound(w, r)
		return
	}
	job, ok := a.jobs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	logPath := strings.TrimSpace(job.Result.LogPath)
	if logPath == "" {
		http.Error(w, "job has no companion log", http.StatusNotFound)
		return
	}
	absPath, err := filepath.Abs(logPath)
	if err != nil {
		respondInternal(w, r, "Could not resolve the log path.", err)
		return
	}
	tempDir, err := filepath.Abs(os.TempDir())
	if err != nil {
		respondInternal(w, r, "Could not resolve the temp directory.", err)
		return
	}
	// Containment check: the resolved path must live inside
	// os.TempDir() (where the backend writes memorial logs). Use
	// filepath.Rel + prefix check rather than HasPrefix to avoid
	// /tmp vs /tmp2 style collisions.
	rel, err := filepath.Rel(tempDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		http.Error(w, "log path is outside the allowed directory", http.StatusForbidden)
		return
	}
	info, statErr := os.Stat(absPath)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			http.Error(w, "log file is missing on disk", http.StatusGone)
			return
		}
		respondInternal(w, r, "Could not open the log file.", statErr)
		return
	}
	w.Header().Set("Content-Length", formatContentLength(info.Size()))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(absPath)))
	http.ServeFile(w, r, absPath)
}

// jobArtifactHeaders returns the Content-Disposition and
// Content-Type the artifact stream should send based on the
// file's extension. Viewable types (PDF, images, HTML, text,
// JSON) are served inline so the user sees them render in the new
// tab the "Open" link opens; everything else (.ddbak, .ddshare,
// .zip, .csv, .xlsx, .ics) downloads via Content-Disposition:
// attachment.
//
// The /jobs/{id} status page no longer links to /artifact for
// inline rendering (the "Open result" button now calls
// /jobs/{id}/open which uses runtime.BrowserOpenURL in the
// Wails desktop; in web-mode the copy-path button is the only
// affordance). The endpoint is preserved for any tool or future
// feature that needs to fetch the bytes.
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

// jobArtifactMimeByExt maps the file extensions the
// artifact stream renders inline (vs. forcing attachment).
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