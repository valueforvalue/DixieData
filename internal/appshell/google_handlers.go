// google_handlers.go holds the Google integration HTTP handlers: OAuth
// connect/disconnect, Drive backup upload, Sheets export, and the managed +
// test calendar flows. Extracted from app.go as step 3 of the God-class
// reduction tracked in issue #42. Handlers stay on *App; routes registered
// in routes.go.
package appshell

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

func (a *App) handleGoogleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.google.Connect(r.Context()); err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google connect failed: %v", err), "error")
		return
	}
	setToastHeader(w, "Google account connected.")
}

func (a *App) handleGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.google.Disconnect(); err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google disconnect failed: %v", err), "error")
		return
	}
	setToastHeader(w, "Google account disconnected.")
}

func (a *App) handleGoogleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-drive-backup-*")
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Drive upload failed: %v", err), "error")
		return
	}
	backupPath := filepath.Join(tempDir, backupArchiveName(time.Now()))

	var jobID string
	jobID = a.jobs.Start("google_drive_backup", func(ctx context.Context, p *jobs.Progress) error {
		// tempDir cleanup MUST run after the worker, not via defer
		// in the request goroutine which returns immediately.
		defer os.RemoveAll(tempDir)

		p.Set(10, "Building backup archive")
		manifest, err := a.backup.Export(backupPath, a.dataDir)
		if err != nil {
			return err
		}
		p.Set(60, "Uploading to Google Drive")
		uploaded, err := a.google.UploadBackup(ctx, backupPath)
		if err != nil {
			return err
		}
		p.Set(100, fmt.Sprintf("Uploaded %d soldiers, %d images.", manifest.Soldiers, manifest.Images))
		_ = uploaded
		_ = jobID
		return nil
	})
	setInfoToastHeader(w, "Google Drive upload started\u2026")
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}

func (a *App) handleGoogleSheetsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-google-sheets-*")
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Sheets export failed: %v", err), "error")
		return
	}
	csvPath := filepath.Join(tempDir, "dixiedata-export.csv")

	var jobID string
	jobID = a.jobs.Start("google_sheets_export", func(ctx context.Context, p *jobs.Progress) error {
		defer os.RemoveAll(tempDir)

		p.Set(10, "Building CSV")
		if err := a.export.ExportCSV(csvPath); err != nil {
			return err
		}
		p.Set(70, "Uploading to Google Sheets")
		uploaded, err := a.google.UploadCSVAsSheet(ctx, csvPath, "DixieData Export")
		if err != nil {
			return err
		}
		p.Set(100, "Google Sheet ready.")
		_ = uploaded
		_ = jobID
		return nil
	})
	setInfoToastHeader(w, "Google Sheets export started\u2026")
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}

func (a *App) handleGoogleCalendarUseManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	calendarID, created, err := a.google.UseManagedCalendar(r.Context())
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar setup failed: %v", err), "error")
		return
	}
	if created {
		setToastHeader(w, fmt.Sprintf("Managed DixieData calendar created and selected (%s).", calendarID))
		return
	}
	setToastHeader(w, fmt.Sprintf("Managed DixieData calendar selected (%s).", calendarID))
}

func (a *App) handleGoogleCalendarPreferencesSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	preferences, err := parseCalendarEventPreferencesForm(r)
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Calendar preference save failed: %v", err), "error")
		return
	}
	saved, err := a.google.SaveManagedEventPreferences(preferences)
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Calendar preference save failed: %v", err), "error")
		return
	}
	setToastHeader(w, fmt.Sprintf("Calendar preferences saved. Title preset: %s. Start time: %s. Sync DixieData Calendar to apply changes globally.", saved.TitlePreset, saved.StartTime))
}

func (a *App) handleGoogleCalendarSyncManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, _, err := a.google.UseManagedCalendar(r.Context()); err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar sync failed: %v", err), "error")
		return
	}
	settings, _, _, _, err := a.google.LoadEffectiveSettings()
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar sync failed: %v", err), "error")
		return
	}
	soldiers, err := a.listAllSoldiers()
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar sync failed: %v", err), "error")
		return
	}
	if strings.TrimSpace(r.FormValue("confirm_sync")) != "1" {
		preview, err := a.google.PreviewSyncCalendar(settings, soldiers)
		if err != nil {
			respondInternal(w, r, "Google Calendar sync preview failed.", err)
			return
		}
		fmt.Fprintf(w, "Dry run: %d create, %d update, %d delete, %d skip. <button type=\"button\" name=\"confirm_sync\" value=\"1\" class=\"secondary-button ml-2\" hx-post=\"/integrations/google/calendar/sync-managed\" hx-target=\"#google-status\" data-progress-label=\"Syncing DixieData Calendar...\" data-busy-group=\"google-calendar-actions\">Run Sync Now</button>", preview.Created, preview.Updated, preview.Deleted, preview.Skipped)
		return
	}
	result, err := a.google.SyncCalendar(r.Context(), settings, soldiers)
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar sync failed: %v", err), "error")
		return
	}
	setToastHeader(w, fmt.Sprintf("DixieData Calendar synced: %d created, %d updated, %d deleted, %d skipped.", result.Created, result.Updated, result.Deleted, result.Skipped))
}

func (a *App) handleGoogleCalendarUnsyncManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.UnsyncCalendar(r.Context())
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar unsync failed: %v", err), "error")
		return
	}
	setToastHeader(w, fmt.Sprintf("DixieData Calendar unsynced: %d event(s) removed.", result.Deleted))
}

func (a *App) handleGoogleCalendarUseTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	calendarID, created, err := a.google.UseTestCalendar(r.Context())
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar test setup failed: %v", err), "error")
		return
	}
	if created {
		setToastHeader(w, fmt.Sprintf("DixieData Test calendar created and selected (%s).", calendarID))
		return
	}
	setToastHeader(w, fmt.Sprintf("DixieData Test calendar selected (%s).", calendarID))
}

func (a *App) handleGoogleCalendarSyncTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.SyncTestCalendar(r.Context())
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar test sync failed: %v", err), "error")
		return
	}
	setToastHeader(w, fmt.Sprintf("DixieData Test sync complete: %d created, %d updated, %d deleted, %d skipped.", result.Created, result.Updated, result.Deleted, result.Skipped))
}

func (a *App) handleGoogleCalendarUnsyncTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.UnsyncTestCalendar(r.Context())
	if err != nil {
		setToastHeaderWithType(w, fmt.Sprintf("Google Calendar test unsync failed: %v", err), "error")
		return
	}
	setToastHeader(w, fmt.Sprintf("DixieData Test unsynced: %d event(s) removed.", result.Deleted))
}
