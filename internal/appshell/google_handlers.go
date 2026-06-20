// google_handlers.go holds the Google integration HTTP handlers: OAuth
// connect/disconnect, Drive backup upload, Sheets export, and the managed +
// test calendar flows. Extracted from app.go as step 3 of the God-class
// reduction tracked in issue #42. Handlers stay on *App; routes registered
// in routes.go.
package appshell

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) handleGoogleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.google.Connect(r.Context()); err != nil {
		fmt.Fprintf(w, "Google connect failed: %v", err)
		return
	}
	fmt.Fprint(w, "Google account connected.")
}

func (a *App) handleGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.google.Disconnect(); err != nil {
		fmt.Fprintf(w, "Google disconnect failed: %v", err)
		return
	}
	fmt.Fprint(w, "Google account disconnected.")
}

func (a *App) handleGoogleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-drive-backup-*")
	if err != nil {
		fmt.Fprintf(w, "Google Drive upload failed: %v", err)
		return
	}
	defer os.RemoveAll(tempDir)

	backupPath := filepath.Join(tempDir, backupArchiveName(time.Now()))
	manifest, err := a.backup.Export(backupPath, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Google Drive upload failed: %v", err)
		return
	}
	uploaded, err := a.google.UploadBackup(r.Context(), backupPath)
	if err != nil {
		fmt.Fprintf(w, "Google Drive upload failed: %v", err)
		return
	}
	fmt.Fprint(w, externalLinkMarkup(
		fmt.Sprintf("Backup uploaded to Google Drive (%d soldiers, %d images):", manifest.Soldiers, manifest.Images),
		uploaded.WebViewLink,
		uploaded.Name,
	))
}

func (a *App) handleGoogleSheetsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-google-sheets-*")
	if err != nil {
		fmt.Fprintf(w, "Google Sheets export failed: %v", err)
		return
	}
	defer os.RemoveAll(tempDir)

	csvPath := filepath.Join(tempDir, "dixiedata-export.csv")
	if err := a.export.ExportCSV(csvPath); err != nil {
		fmt.Fprintf(w, "Google Sheets export failed: %v", err)
		return
	}

	uploaded, err := a.google.UploadCSVAsSheet(r.Context(), csvPath, "DixieData Export")
	if err != nil {
		fmt.Fprintf(w, "Google Sheets export failed: %v", err)
		return
	}
	fmt.Fprint(w, externalLinkMarkup(
		"Google Sheet ready:",
		uploaded.WebViewLink,
		uploaded.Name,
	))
}

func (a *App) handleGoogleCalendarUseManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	calendarID, created, err := a.google.UseManagedCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar setup failed: %v", err)
		return
	}
	if created {
		fmt.Fprintf(w, "Managed DixieData calendar created and selected (%s).", calendarID)
		return
	}
	fmt.Fprintf(w, "Managed DixieData calendar selected (%s).", calendarID)
}

func (a *App) handleGoogleCalendarPreferencesSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	preferences, err := parseCalendarEventPreferencesForm(r)
	if err != nil {
		fmt.Fprintf(w, "Calendar preference save failed: %v", err)
		return
	}
	saved, err := a.google.SaveManagedEventPreferences(preferences)
	if err != nil {
		fmt.Fprintf(w, "Calendar preference save failed: %v", err)
		return
	}
	fmt.Fprintf(w, "Calendar preferences saved. Title preset: %s. Start time: %s. Sync DixieData Calendar to apply changes globally.", saved.TitlePreset, saved.StartTime)
}

func (a *App) handleGoogleCalendarSyncManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, _, err := a.google.UseManagedCalendar(r.Context()); err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	settings, _, _, _, err := a.google.LoadEffectiveSettings()
	if err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	soldiers, err := a.listAllSoldiers()
	if err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	if strings.TrimSpace(r.FormValue("confirm_sync")) != "1" {
		preview, err := a.google.PreviewSyncCalendar(settings, soldiers)
		if err != nil {
			fmt.Fprintf(w, "Google Calendar sync preview failed: %v", err)
			return
		}
		fmt.Fprintf(w, "Dry run: %d create, %d update, %d delete, %d skip. <button type=\"button\" name=\"confirm_sync\" value=\"1\" class=\"secondary-button ml-2\" hx-post=\"/integrations/google/calendar/sync-managed\" hx-target=\"#google-status\" data-progress-label=\"Syncing DixieData Calendar...\" data-busy-group=\"google-calendar-actions\">Run Sync Now</button>", preview.Created, preview.Updated, preview.Deleted, preview.Skipped)
		return
	}
	result, err := a.google.SyncCalendar(r.Context(), settings, soldiers)
	if err != nil {
		fmt.Fprintf(w, "Google Calendar sync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "DixieData Calendar synced: %d created, %d updated, %d deleted, %d skipped.", result.Created, result.Updated, result.Deleted, result.Skipped)
}

func (a *App) handleGoogleCalendarUnsyncManaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.UnsyncCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar unsync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "DixieData Calendar unsynced: %d event(s) removed.", result.Deleted)
}

func (a *App) handleGoogleCalendarUseTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	calendarID, created, err := a.google.UseTestCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar test setup failed: %v", err)
		return
	}
	if created {
		fmt.Fprintf(w, "DixieData Test calendar created and selected (%s).", calendarID)
		return
	}
	fmt.Fprintf(w, "DixieData Test calendar selected (%s).", calendarID)
}

func (a *App) handleGoogleCalendarSyncTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.SyncTestCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar test sync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "DixieData Test sync complete: %d created, %d updated, %d deleted, %d skipped.", result.Created, result.Updated, result.Deleted, result.Skipped)
}

func (a *App) handleGoogleCalendarUnsyncTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.google.UnsyncTestCalendar(r.Context())
	if err != nil {
		fmt.Fprintf(w, "Google Calendar test unsync failed: %v", err)
		return
	}
	fmt.Fprintf(w, "DixieData Test unsynced: %d event(s) removed.", result.Deleted)
}
