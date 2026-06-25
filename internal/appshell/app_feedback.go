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

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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

	w.Header().Set("X-DixieData-Close-Feedback", "true")
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
			fmt.Fprint(w, "No feedback has been saved yet.")
			return
		}
		respondInternal(w, r, "Could not read the feedback log.", err)
		return
	}

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: feedbackExportName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "Feedback log", Pattern: "*.jsonl"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Feedback log export cancelled.", nil)
		return
	}

	if err := copyFeedbackLog(sourcePath, path); err != nil {
		respondInternal(w, r, "Could not write the feedback log.", err)
		return
	}
	a.BrowserOpenURL( "file://"+filepath.ToSlash(path))
	setToastHeader(w, fmt.Sprintf("Feedback log saved to %s", path))
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
	defer file.Close()

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

func feedbackExportName(now time.Time) string {
	return fmt.Sprintf("DixieData-feedback-log-%s.jsonl", now.Format("20060102-150405"))
}

func urlQueryEscape(value string) string {
	replacer := strings.NewReplacer("%", "%25", " ", "%20", "+", "%2B", "&", "%26", "=", "%3D", "#", "%23", "?", "%3F")
	return replacer.Replace(value)
}
