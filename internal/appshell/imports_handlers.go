// imports_handlers.go holds the backup and shared-archive import HTTP
// handlers plus the memorial-JSON preview helpers. Extracted from app.go
// as step 5 of the God-class reduction tracked in issue #42. Handlers stay
// on *App; routes registered in routes.go.
package appshell

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	runtime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/models"
)

func (a *App) handleImportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Only one .ddbak restore can run at a time because the
	// import replaces the SQLite file the rest of the app is
	// reading. Refuse a second concurrent request with a 503
	// + toast that points at the existing /jobs/{id} so the user
	// can monitor the in-flight restore.
	if a.importInFlight.Load() {
		if jobID := a.importInFlightJobID(); jobID != "" {
			http.Redirect(w, r, "/jobs/"+jobID, http.StatusSeeOther)
			return
		}
		respondError(w, r, KindUnavailable, "A backup restore is already in progress; please wait for it to finish.", nil)
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

	// Capture the local identity BEFORE we close the DB — the
	// worker doesn't have access to a.database once the import
	// starts, and the identity preservation rule needs to read
	// it now.
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

	// Run the restore inside a background job so the user sees a
	// /jobs/{id} status page with real progress instead of being
	// blocked on the HTTP goroutine for 10+ seconds on a 500 MB
	// archive (issue #133). The worker closes the DB, replaces the
	// data dir, and reopens the DB + reloads services when done.
	var jobID string
	jobID = a.jobs.Start("backup_import", func(ctx context.Context, p *jobs.Progress) error {
		a.importInFlight.Store(true)
		a.importInFlightJobIDSet(jobID)
		defer a.importInFlight.Store(false)
		defer a.importInFlightJobIDClear()
		p.Set(5, "Closing database")
		if a.database != nil {
			a.database.Close()
			a.database = nil
		}
		p.Set(20, fmt.Sprintf("Restoring %s", filepath.Base(path)))
		manifest, err := a.backup.ImportWithLocalIdentity(path, a.dataDir, localIdentity, preserveLocalIdentity)
		if err != nil {
			if reopenErr := a.reopenDatabase(); reopenErr != nil {
				return fmt.Errorf("import failed (%w) and database could not be reopened (%v); restart DixieData to recover", err, reopenErr)
			}
			return fmt.Errorf("import failed: %w", err)
		}
		p.Set(85, "Reopening database")
		if err := a.reopenDatabase(); err != nil {
			return fmt.Errorf("backup imported but database could not be reopened: %w", err)
		}
		p.Set(100, fmt.Sprintf("Imported %d soldiers, %d records, %d images.", manifest.Soldiers, manifest.Records, manifest.Images))
		// Surface the replace-semantics stats on the /jobs/{id}
		// summary card: how many records/images the backup
		// overwrote and whether the schema migration ran. Backup
		// restore is a full replace (not a merge) so the summary
		// wording switches accordingly.
		a.jobs.SetResult(jobID, jobs.JobResult{
			ReplacedRecords: manifest.Soldiers,
			ReplacedImages:  manifest.Images,
			BackupSchema:    manifest.SchemaVersion,
			CurrentSchema:   buildinfo.SchemaVersion,
			MigrationRan:    manifest.SchemaVersion != buildinfo.SchemaVersion,
		})
		return nil
	})

	setInfoToastHeader(w, fmt.Sprintf("Restoring backup: %s", filepath.Base(path)))
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}

func (a *App) handleImportSharedArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	}
	dupKey := guardedOpenFileDialogKey("shared_archive", opts)
	path, admitted, ok := a.guardedOpenFileDialog(dupKey, opts)
	if !admitted {
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	if !ok {
		respondError(w, r, KindValidation, "Shared archive import cancelled.", nil)
		return
	}

	// Run the import inside a background job so the user sees real
	// progress. We pre-compute the summary outside the worker (the
	// worker only touches jobs state) because ImportSharedBackup is
	// a single blocking call and we want a single progress tick.
	var jobID string
	jobID = a.jobs.Start("shared_import", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, "Merging shared archive")
		summary, err := a.backup.ImportSharedBackup(path, a.dataDir)
		if err != nil {
			return err
		}
		if summary.PendingConflicts > 0 {
			p.Set(95, fmt.Sprintf("Staged %d conflicts for review", summary.PendingConflicts))
		}
		p.Set(100, fmt.Sprintf("Imported %d soldiers, %d pending conflicts.", summary.SoldiersInserted+summary.SoldiersUpdated, summary.PendingConflicts))
		// Surface the merge-review headline on the /jobs/{id}
		// summary card: how many records were added / merged /
		// skipped, how many conflicts were staged for review, and
		// how many images / source records came in. When
		// PendingConflicts > 0 the summary line reminds the user
		// to open Merge Review; when 0 the import is fully
		// resolved.
		//
		// Note: SharedImportSummary currently does not return a
		// source-records count. SourcesImported stays 0; if the
		// service ever surfaces that field, plumb it through here.
		a.jobs.SetResult(jobID, jobs.JobResult{
			Added:          summary.SoldiersInserted,
			Merged:         summary.SoldiersUpdated,
			Skipped:        0, // service does not surface Skipped today
			Conflicts:      summary.PendingConflicts,
			ImagesImported: summary.ImagesInserted + summary.ImagesUpdated,
		})
		_ = jobID
		return nil
	})
	setInfoToastHeader(w, "Shared archive import started\u2026")
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}

func (a *App) handlePreviewMemorialJSONImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Memorial archive JSON", Pattern: "*.json"},
		},
	}
	dupKey := guardedOpenFileDialogKey("memorial_preview", opts)
	path, admitted, ok := a.guardedOpenFileDialog(dupKey, opts)
	if !admitted {
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	if !ok {
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

	// Capture the path before the request returns and enqueue the
	// actual import as a background job. The /jobs/{id} page will
	// render when the worker finishes. The summary + error log are
	// written to the job's ResultPath-equivalent so they survive
	// the request lifecycle; in practice we use the existing
	// memorialImportSummaryMarkup against the summary captured
	// inside the worker.
	var jobID string
	jobID = a.jobs.Start("memorial_import", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, "Reading Memorial archive")
		summary, err := a.soldiers.ImportMemorialArchive(path)
		if err != nil {
			return err
		}
		p.Set(80, "Writing error log")
		logPath, logErr := writeMemorialImportErrorLog(summary)
		if logErr != nil {
			return logErr
		}
		p.Set(100, fmt.Sprintf("Memorial import complete: %d created, %d skipped, %d failed. Log: %s", summary.Created, summary.Skipped, summary.Failed, logPath))
		// Surface the memorial-import headline on the /jobs/{id}
		// summary card: how many rows were added / skipped /
		// failed. Memorial JSON is additive (no Merge Review) so
		// the conflicts line stays hidden. The optional error log
		// becomes a secondary download action via Result.LogPath.
		a.jobs.SetResult(jobID, jobs.JobResult{
			Added:   summary.Created,
			Skipped: summary.Skipped,
			Failed:  summary.Failed,
			LogPath: logPath,
		})
		_ = jobID
		return nil
	})
	setInfoToastHeader(w, "Memorial JSON import started\u2026")
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
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

