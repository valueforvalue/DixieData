// imports_handlers.go holds the backup and shared-archive import HTTP
// handlers plus the memorial-JSON preview helpers. Extracted from app.go
// as step 5 of the God-class reduction tracked in issue #42. Handlers stay
// on *App; routes registered in routes.go.
package appshell

import (
	"fmt"
	"net/http"
	"strings"

	runtime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func (a *App) handleImportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := a.OpenFileDialog( runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
			{DisplayName: "Legacy backup archive", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Backup import cancelled.", nil)
		return
	}

	var localIdentity models.UserIdentity
	preserveLocalIdentity := false
	if a.database != nil {
		complete, err := a.database.SystemConfig("user_identity_complete")
		if err != nil {
			respondInternal(w, r, "Could not read system configuration.", err)
			return
		}
		if strings.TrimSpace(complete) == "1" {
			localIdentity, err = a.database.UserIdentity()
			if err != nil {
				respondInternal(w, r, "Could not load the local identity.", err)
				return
			}
			preserveLocalIdentity = true
		}
	}

	if a.database != nil {
		a.database.Close()
		a.database = nil
	}

	manifest, err := a.backup.ImportWithLocalIdentity(path, a.dataDir, localIdentity, preserveLocalIdentity)
	if err != nil {
		if reopenErr := a.reopenDatabase(); reopenErr != nil {
			respondInternal(w, r, "Backup import failed and the database could not be reopened. Restart DixieData to recover.", fmt.Errorf("import: %w; reopen: %w", err, reopenErr))
			return
		}
		respondInternal(w, r, "Backup import failed.", err)
		return
	}
	if err := a.reopenDatabase(); err != nil {
		respondInternal(w, r, "Backup imported but the database could not be reopened.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Success: %d records imported from backup.", manifest.Soldiers))
	fmt.Fprintf(w, "Backup loaded: %d soldiers, %d records, %d images.", manifest.Soldiers, manifest.Records, manifest.Images)
}

func (a *App) handleImportSharedArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := a.OpenFileDialog( runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Shared archive import cancelled.", nil)
		return
	}

	summary, err := a.backup.ImportSharedBackup(path, a.dataDir)
	if err != nil {
		respondInternal(w, r, "Shared archive import failed.", err)
		return
	}
	if summary.PendingConflicts > 0 {
		w.Header().Set("X-DixieData-Redirect", "/export")
	}
	setToastHeader(w, fmt.Sprintf("Success: %d records imported, %d flagged for review.", summary.SoldiersInserted+summary.SoldiersUpdated, summary.PendingConflicts))
	fmt.Fprintf(w, "Shared backup merged: %d soldiers added, %d updated; %d records added, %d updated; %d images added, %d updated; %d conflicts staged for review. Merge log: %s",
		summary.SoldiersInserted, summary.SoldiersUpdated,
		summary.RecordsInserted, summary.RecordsUpdated,
		summary.ImagesInserted, summary.ImagesUpdated,
		summary.PendingConflicts,
		summary.LogPath,
	)
}

func (a *App) handlePreviewMemorialJSONImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := a.OpenFileDialog( runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Memorial archive JSON", Pattern: "*.json"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Memorial JSON import preview cancelled.", nil)
		return
	}
	preview, err := a.soldiers.PreviewMemorialArchive(path)
	if err != nil {
		respondInternal(w, r, "Memorial JSON preview failed.", err)
		return
	}
	token, err := a.rememberMemorialPreview(path)
	if err != nil {
		respondInternal(w, r, "Memorial JSON preview failed.", err)
		return
	}
	fmt.Fprint(w, memorialImportPreviewMarkup(preview, token))
}

func (a *App) handleConfirmMemorialJSONImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the import confirmation form.", err)
		return
	}
	token := strings.TrimSpace(r.FormValue("preview_token"))
	path, ok := a.consumeMemorialPreview(token)
	if !ok {
		respondValidation(w, r, "Import preview expired. Run preview again.", nil)
		return
	}
	summary, err := a.soldiers.ImportMemorialArchive(path)
	if err != nil {
		respondInternal(w, r, "Memorial JSON import failed.", err)
		return
	}
	logPath, logErr := writeMemorialImportErrorLog(summary)
	if logErr != nil {
		respondInternal(w, r, "Memorial JSON import completed but the error log could not be written.", logErr)
		return
	}
	setToastHeader(w, fmt.Sprintf("Memorial import complete: %d created, %d skipped, %d failed.", summary.Created, summary.Skipped, summary.Failed))
	fmt.Fprint(w, memorialImportSummaryMarkup(summary, logPath))
}

func (a *App) rememberMemorialPreview(path string) (string, error) {
	token, err := db.NewSyncID()
	if err != nil {
		return "", err
	}
	a.previewMu.Lock()
	if a.memorialPreview == nil {
		a.memorialPreview = make(map[string]string)
	}
	a.memorialPreview[token] = strings.TrimSpace(path)
	a.previewMu.Unlock()
	return token, nil
}

func (a *App) consumeMemorialPreview(token string) (string, bool) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", false
	}
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	if a.memorialPreview == nil {
		return "", false
	}
	path, ok := a.memorialPreview[trimmed]
	if ok {
		delete(a.memorialPreview, trimmed)
	}
	return path, ok
}

