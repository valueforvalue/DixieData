package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/supportuploader"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// formsparkDefaultEndpointForTest lets the test suite redirect
// the production Formspark endpoint to a local httptest server
// without exposing the override to production callers. The
// production handler reads this var when non-empty and falls
// back to supportuploader.DefaultFormsparkEndpoint otherwise.
// Issue #566 slice 2.
var formsparkDefaultEndpointForTest string

// formsparkEndpoint returns the Formspark endpoint the handler
// should POST to. Priority: test override > configured
// cfg.Services.FeedbackEndpoint (issue #660 audit gap) > the
// built-in default. The configured value is read once at App
// construction via the formsparkConfiguredEndpoint global below
// so the hot path stays a constant-time lookup.
func formsparkEndpoint() string {
	if formsparkDefaultEndpointForTest != "" {
		return formsparkDefaultEndpointForTest
	}
	if formsparkConfiguredEndpoint != "" {
		return formsparkConfiguredEndpoint
	}
	return supportuploader.DefaultFormsparkEndpoint
}

// formsparkConfiguredEndpoint is set by reloadServices from
// cfg.Services.FeedbackEndpoint. Empty in production builds that
// haven't gone through reloadServices; the fallback chain above
// covers that path.
var formsparkConfiguredEndpoint string

// feedbackSendTimeoutS is set by reloadServices from
// cfg.Timing.FeedbackSendTimeoutS (issue #660). The handler
// reads it at the start of the upload context to size the
// per-attempt deadline; the upload package's own timeout
// (cfg.Timing.FeedbackUploadTimeoutS) governs the inner
// POST. Zero means "use the built-in 35s default" so old
// configs without the field still work.
var feedbackSendTimeoutS int

type feedbackEntry struct {
	SubmittedAt   string `json:"submitted_at"`
	PagePath      string `json:"page_path,omitempty"`
	Category      string `json:"category,omitempty"`
	ContactName   string `json:"contact_name,omitempty"`
	ContactEmail  string `json:"contact_email,omitempty"`
	Message       string `json:"message"`
	AppVersion    string `json:"app_version"`
	BuildIdentity string `json:"build_identity"`
	SchemaVersion int    `json:"schema_version"`
}

func (a *App) handleSoldierByDisplayID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	displayID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/soldiers/display/"))
	if displayID == "" {
		http.NotFound(w, r)
		return
	}

	record, err := a.soldiers.GetByDisplayID(displayID)
	if err == nil && record != nil {
		http.Redirect(w, r, fmt.Sprintf("/soldiers/%d", record.ID), http.StatusSeeOther)
		return
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		respondInternal(w, r, fmt.Sprintf("Could not look up Display ID %s.", displayID), err)
		return
	}

	http.Redirect(w, r, "/soldiers/search?q="+urlQueryEscape(displayID), http.StatusSeeOther)
}

func (a *App) handleFeedbackSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the feedback form.", err)
		return
	}

	entry := feedbackEntry{
		SubmittedAt:   time.Now().Format(time.RFC3339),
		PagePath:      strings.TrimSpace(r.FormValue("page_path")),
		Category:      strings.TrimSpace(r.FormValue("category")),
		ContactName:   strings.TrimSpace(r.FormValue("contact_name")),
		ContactEmail:  strings.TrimSpace(r.FormValue("contact_email")),
		Message:       strings.TrimSpace(r.FormValue("message")),
		AppVersion:    buildinfo.AppVersion,
		BuildIdentity: buildinfo.BuildIdentity(),
		SchemaVersion: buildinfo.SchemaVersion,
	}
	if entry.Message == "" {
		setToastHeaderWithType(w, "Feedback message is required.", "error")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Please enter feedback before submitting.")
		return
	}

	if _, err := appendFeedbackEntry(a.dataDir, entry); err != nil {
		setToastHeaderWithType(w, "Feedback could not be saved.", "error")
		respondInternal(w, r, "Could not save feedback to the local log.", err)
		return
	}

	// Issue #566: the action field disambiguates the existing
	// "Save" path (action=save, default) from the "Send to
	// support" path (action=send). The local JSONL is ALWAYS
	// written first (above) so the user has a local copy
	// regardless of upload success. When action=send, also POST
	// the entry to the DixieData-owned Formspark endpoint via
	// the supportuploader package; the toast carries a
	// confirmation on success or a user-visible failure
	// message. The endpoint is a package-level constant
	// (DefaultFormsparkEndpoint) — per-user override is
	// intentionally not supported in this slice.
	action := strings.TrimSpace(r.FormValue("action"))
	w.Header().Set("X-DixieData-Close-Feedback", "true")
	if action == "send" {
		timeout := time.Duration(feedbackSendTimeoutS) * time.Second
		if timeout == 0 {
			timeout = 35 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if uploadErr := supportuploader.UploadFeedbackFormspark(ctx, entry, formsparkEndpoint()); uploadErr != nil {
			log := debug.FromContext(r.Context())
			log.Warn("feedback upload to support failed", "error", uploadErr.Error())
			setToastHeaderWithType(w, fmt.Sprintf("Feedback saved locally; upload failed: %s", uploadErr.Error()), "error")
			fmt.Fprint(w, "Feedback saved locally; upload failed.")
			return
		}
		setToastHeader(w, "Feedback sent to DixieData support. A copy is in the local log.")
		fmt.Fprint(w, "Feedback sent to DixieData support.")
		return
	}
	setToastHeader(w, "Feedback saved to the local log.")
	fmt.Fprint(w, "Thanks. Your feedback was saved to the local log and can be exported from Share.")
}

func (a *App) handleExportFeedbackLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sourcePath := appdata.FeedbackLogPath(a.dataDir)
	if _, err := os.Stat(sourcePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Issue #137: surface the empty-state as a toast so the
			// dispatcher's X-DixieData-Toast path lands it on the page
			// instead of dropping a bare-body response on the floor.
			setInfoToastHeader(w, "No feedback has been saved yet — nothing to export.")
			return
		}
		respondInternal(w, r, "Could not read the feedback log.", err)
		return
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: feedbackExportName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "Feedback log", Pattern: "*.jsonl"},
		},
	}
	dupKey := guardedSaveFileDialogKey("feedback_log", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}

	// Issue #137: route the copy through enqueueExport so the UX
	// matches every other export on /share — native save dialog →
	// redirect to /jobs/{id} → progress card → final summary. The
	// pre-existing direct-response path silently dropped the toast
	// for non-redirect synthetic submits (dispatchDixieDataForm
	// saved the toast to sessionStorage but never re-rendered it).
	a.enqueueExport(dupKey, "feedback_log", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(10, "Reading feedback log")
		return copyFeedbackLog(sourcePath, path)
	}, path, w)
}

func appendFeedbackEntry(dataDir string, entry feedbackEntry) (string, error) {
	path := appdata.FeedbackLogPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer debug.DeferCloseLog(file, "appendFeedbackEntry.file")

	payload, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		return "", err
	}
	return path, nil
}

func copyFeedbackLog(sourcePath, destinationPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(destinationPath, data, 0o644)
}

// defaultFeedbackRetentionDays is the default prune window. Override
// via SetFeedbackRetentionDays from config.json at startup.
var defaultFeedbackRetentionDays = 365

// SetFeedbackRetentionDays overrides the feedback log retention
// window from config.json.
func SetFeedbackRetentionDays(days int) {
	if days > 0 {
		defaultFeedbackRetentionDays = days
	}
}

// pruneFeedbackLogOnStartup rewrites the feedback JSONL so it only
// contains entries newer than the retention window. Best-effort: a
// missing or unreadable log is fine, a corrupt log is left alone
// rather than silently dropped, and a successful prune is silent on
// the happy path.
func pruneFeedbackLogOnStartup(dataDir string) {
	if dataDir == "" {
		return
	}
	path := appdata.FeedbackLogPath(dataDir)
	pruneFeedbackLogAtPath(path, defaultFeedbackRetentionDays)
}

// pruneFeedbackLogAtPath is the testable form of the startup prune.
// retentionDays=0 means 'keep everything'. retentionDays<0 keeps
// nothing (matches the empty-keep edge case for tests).
func pruneFeedbackLogAtPath(path string, retentionDays int) {
	if retentionDays == 0 {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	kept := make([][]byte, 0, 16)
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var entry feedbackEntry
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
			// Skip the malformed line. We never silently delete
			// something we could not parse.
			continue
		}
		if retentionDays < 0 {
			continue
		}
		ts, err := time.Parse(time.RFC3339, entry.SubmittedAt)
		if err != nil {
			kept = append(kept, []byte(trimmed))
			continue
		}
		if ts.After(cutoff) {
			kept = append(kept, []byte(trimmed))
		}
	}
	out := strings.Join(byteSlicesToStrings(kept), "\n")
	if out != "" {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "feedback: prune %s failed: %v\n", path, err)
	}
}

func byteSlicesToStrings(in [][]byte) []string {
	out := make([]string, len(in))
	for i, b := range in {
		out[i] = string(b)
	}
	return out
}

func feedbackExportName(now time.Time) string {
	return fmt.Sprintf("DixieData-feedback-log-%s.jsonl", now.Format("20060102-150405"))
}

func urlQueryEscape(value string) string {
	replacer := strings.NewReplacer("%", "%25", " ", "%20", "+", "%2B", "&", "%26", "=", "%3D", "#", "%23", "?", "%3F")
	return replacer.Replace(value)
}
