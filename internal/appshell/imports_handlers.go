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
	"time"

	runtime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/debug"
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
			// Option C: dispatchDixieDataForm reads X-DixieData-Redirect.
			writeExportRedirect(w, "/jobs/"+jobID)
			return
		}
		respondError(w, r, KindUnavailable, "A backup restore is already in progress; please wait for it to finish.", nil)
		return
	}

	// Dialog-guard per CONTEXT.md "Laws (non-negotiable)" and
	// docs/agents/dialog-guard.md: a rapid double-click on the
	// "Restore from backup" button can land two OpenFileDialog
	// calls on the Wails UI thread; WebView2 loses focus and
	// crashes with Chrome_WidgetWin_0. Error = 1412. Reject the
	// second request before it reaches the native dialog.
	// The key includes the file pattern so a legitimate retry
	// against a different file path can still be admitted.
	dialogOpts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
			{DisplayName: "Legacy backup archive", Pattern: "*.zip"},
		},
	}
	dupKey := guardedOpenFileDialogKey("backup_import", dialogOpts)
	admitted, entry := a.enterInFlight(dupKey)
	if !admitted {
		debug.FromContext(r.Context()).Debug("handleImportBackup duplicate request rejected")
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	defer a.leaveInFlight(dupKey, entry)

	path, err := a.OpenFileDialog(dialogOpts)
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
		// The ImportWithLocalIdentity call is the slow phase (tens
		// of seconds for a 500MB .ddbak). Without Shimmer the bar
		// sits at 20% the entire time, then jumps to 85%
		// (database reopen) and 100% (done). With Shimmer the bar
		// walks smoothly through the IO phase; Set(85) below wins
		// the last write, then the registry's Set(100) wins the
		// terminal tick.
		p.Shimmer(ctx, 20, 85, 60*time.Second, "Restoring records…")
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
	// Option C: dispatchDixieDataForm reads X-DixieData-Redirect.
	writeExportRedirect(w, "/jobs/"+jobID)
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
		// Shared-archive import is bounded by file IO and merge
		// review compare-pass duration. Without Shimmer the bar
		// sits at 20% across the import, then jumps to 95 or 100
		// at the end. With Shimmer the bar walks smoothly through
		// the merge phase.
		p.Shimmer(ctx, 20, 95, 45*time.Second, "Merging records…")
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
	setInfoToastHeader(w, "Shared archive import started…")
	writeExportRedirect(w, "/jobs/"+jobID)
}

// handleImportMemorialJSON is the Memorial JSON import entry point:
// the user picks a .json file, the handler preflights (cheap
// header-only parse), and enqueues a jobs.Registry.StartManual
// job that sits in StatusQueued + Progress=0 until the user
// clicks Confirm on /jobs/{id}. Memorial imports are
// irreversible (no Merge Review, no undo), so the worker does
// not fire until the user explicitly opts in.
// /jobs/{id}/confirm calls releaseManualJob which flips the
// queued job to StatusRunning and runs ImportMemorialArchive.
func (a *App) handleImportMemorialJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Memorial archive JSON", Pattern: "*.json"},
		},
	}
	dupKey := guardedOpenFileDialogKey("memorial_import", opts)
	path, admitted, ok := a.guardedOpenFileDialog(dupKey, opts)
	if !admitted {
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	if !ok {
		respondError(w, r, KindValidation, "Memorial JSON import cancelled.", nil)
		return
	}
	// Preflight: parse the archive header so we can show the user
	// exactly what the import will do. Cheap (count-only); the
	// real work happens after the worker is released.
	preview, err := a.soldiers.PreviewMemorialArchive(path)
	if err != nil {
		respondInternal(w, r, "Memorial JSON preflight failed.", err)
		return
	}
	// Seed the queued snapshot with the preflight summary so
	// /jobs/{id} shows the headline before the user confirms.
	summary := fmt.Sprintf(
		"Awaiting confirmation: will create %d, skip %d, fail %d (of %d rows in %s).",
		preview.WouldCreate, preview.WouldSkip, preview.WouldFail,
		preview.TotalRows, filepath.Base(path),
	)

	// The closure captures `id` by reference, but `id` is not in
	// scope until after StartManual returns. Pass it via a tiny
	// indirection so the deferred SetResult / forgetManualJob
	// calls can read it.
	var id string
	id, release, cancel := a.jobs.StartManual("memorial_import", summary, func(ctx context.Context, p *jobs.Progress) error {
		defer a.forgetManualJob(id)
		p.Set(20, "Reading Memorial archive")
		import_summary, err := a.soldiers.ImportMemorialArchive(path)
		if err != nil {
			return err
		}
		p.Set(80, "Writing error log")
		logPath, logErr := writeMemorialImportErrorLog(import_summary)
		if logErr != nil {
			return logErr
		}
		p.Set(100, fmt.Sprintf("Memorial import complete: %d created, %d skipped, %d failed. Log: %s", import_summary.Created, import_summary.Skipped, import_summary.Failed, logPath))
		a.jobs.SetResult(id, jobs.JobResult{
			Added:   import_summary.Created,
			Skipped: import_summary.Skipped,
			Failed:  import_summary.Failed,
			LogPath: logPath,
		})
		return nil
	})
	_ = cancel
	a.rememberManualJob(id, release, cancel)

	setInfoToastHeader(w, "Memorial JSON import queued. Confirm on the status page to proceed.")
	writeExportRedirect(w, "/jobs/"+id)
}


