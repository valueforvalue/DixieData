// exports_handlers.go holds the export HTTP handlers. Extracted from
// app.go as step 6 of the God-class reduction tracked in issue #42.
// Handlers stay on *App; routes registered in routes.go. The handleExport*
// methods are thin wrappers around a.export facade; handleExportDatabasePDF
// is the largest and builds a PrintSettings payload before dispatching to
// the archive service.
package appshell

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	runtime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/pkg/exportbridge"
)

// errExportInFlight is returned by exportFullDatabasePDFPath when a
// SaveFileDialog is already up for the same destination. The HTTP
// handler maps it to a 429; the Wails binding maps it to a friendly
// toast via the same respondError path.
var errExportInFlight = errors.New("export already in progress; please wait for the save dialog")

// guardedSaveFileDialogKey returns the dedup key guardedSaveFileDialog
// would use for the given kind + options. Callers that need the key
// (so they can thread it into enqueueExport and update the entry's
// JobID) compute it once with this helper before calling the dialog
// wrapper.
func guardedSaveFileDialogKey(kind string, opts runtime.SaveDialogOptions) string {
	return fmt.Sprintf("export|%s|%s|%v", kind, opts.DefaultFilename, opts.Filters)
}

// guardedSaveFileDialog wraps a.SaveFileDialog with the duplicate-request
// guard used by handleCalendarPDF. Without it, a double-click (or any
// back-to-back POST that reaches the handler before the user picks a
// file) queues a SECOND native dialog on the Wails main window message
// loop while the first is still up. Both block on the UI thread, and
// the WebView2 frontend crashes with `Chrome_WidgetWin_0. Error = 1412`
// (wailsapp/wails#2807). The guard returns a 429 with a friendly toast
// so the second click is rejected without ever calling SaveFileDialog.
//
// The guard key is the export kind + a fingerprint of the dialog
// options. Two requests with the same options collapse to one
// dialog; two requests with different options (e.g. exporting JSON
// then immediately exporting CSV) are independent and both proceed.
//
// CRITICAL: the slot is released AFTER SaveFileDialog returns, not
// before. Releasing early (before the dialog returns) would let a
// concurrent goroutine race through the LoadOrStore check and queue
// a second native dialog while the first is still up — exactly the
// scenario the guard is meant to prevent. The release happens in
// `defer` so it covers both the success and the cancel/error paths.
//
// Every export handler that calls a.SaveFileDialog must route through
// this helper. The calendar PDF handler carries an equivalent guard
// inline; that handler predates this helper.
// saveOutcome reports why a guardedSaveFileDialog call did not yield
// a destination path. The handler needs to distinguish three outcomes:
//
//   - SaveOutcomeOK              → caller proceeds to enqueueExport.
//   - SaveOutcomeDuplicated      → a second click raced the dialog;
//                                   respondDuplicateInFlight hands the
//                                   user the in-progress /jobs/{id}
//                                   page.
//   - SaveOutcomeDialogAborted   → the native dialog was cancelled, or
//                                   returned a frontend-unavailable
//                                   error. Different from "duplicate":
//                                   the user dismissed it, nothing is
//                                   running. Returned as a toast on the
//                                   current page so the modal/share-page
//                                   isn't replaced with an error body.
//
// The dedup branch still uses (*App).inFlight (the Wails v2.12.0 UI-
// thread crash from two native dialogs landing simultaneously is still
// real even though the dual-JS-handler race is gone after Option C).
// What changed in Option C + this fix is the response shape for the
// non-dedup paths.
type saveOutcome int

const (
	SaveOutcomeOK saveOutcome = iota
	SaveOutcomeDuplicated
	SaveOutcomeDialogAborted
)

func (a *App) guardedSaveFileDialog(dupKey string, opts runtime.SaveDialogOptions) (string, saveOutcome) {
	if dupKey == "" {
		dupKey = guardedSaveFileDialogKey("export", opts)
	}
	admitted, entry := a.enterInFlight(dupKey)
	if !admitted {
		return "", SaveOutcomeDuplicated
	}
	defer a.leaveInFlight(dupKey, entry)
	path, err := a.SaveFileDialog(opts)
	if err != nil {
		// Native dialog is unavailable (web-mode with no override
		// wired) or the OS picker returned an error (write-protected
		// path, etc). Treat as dialog-aborted so the user gets a
		// toast rather than the misleading "duplicate in flight"
		// redirect.
		return "", SaveOutcomeDialogAborted
	}
	if path == "" {
		// User clicked Cancel in the OS picker. Same response shape
		// as above: stay on the current page with a toast.
		return "", SaveOutcomeDialogAborted
	}
	return path, SaveOutcomeOK
}

// guardedOpenFileDialogKey returns the dedup key guardedOpenFileDialog
// would use for the given kind + options. Mirrors
// guardedSaveFileDialogKey so callers can compute the key once,
// thread it into enqueueExport, and use the same key for both the
// dedup check and the JobID redirect (issue #130 pattern).
func guardedOpenFileDialogKey(kind string, opts runtime.OpenDialogOptions) string {
	return fmt.Sprintf("open|%s|%s|%v", kind, opts.DefaultFilename, opts.Filters)
}

// guardedOpenFileDialog wraps a.OpenFileDialog with the same
// in-flight guard used by guardedSaveFileDialog. Native
// OpenFileDialog calls land on the same UI thread as Save
// dialogs; a double-click during a .ddbak import or shared
// archive preview would otherwise queue a second dialog and
// crash WebView2 with Chrome_WidgetWin_0. Error = 1412.
//
// The guard key is the import kind + a fingerprint of the
// dialog options. Two requests with the same options collapse
// to one dialog; two requests with different kinds (shared
// archive vs memorial JSON) are independent and both proceed.
func (a *App) guardedOpenFileDialog(dupKey string, opts runtime.OpenDialogOptions) (string, bool, bool) {
	if dupKey == "" {
		dupKey = guardedOpenFileDialogKey("open", opts)
	}
	admitted, entry := a.enterInFlight(dupKey)
	if !admitted {
		return "", false, false
	}
	defer a.leaveInFlight(dupKey, entry)
	path, err := a.OpenFileDialog(opts)
	if err != nil || path == "" {
		return "", true, false
	}
	return path, true, true
}

// guardedOpenDirectoryDialogKey mirrors guardedOpenFileDialogKey
// for folder-picker dialogs. The Title field is the only
// stable fingerprint for directory pickers (no DefaultFilename
// or Filters), so the key includes it instead.
func guardedOpenDirectoryDialogKey(kind string, opts runtime.OpenDialogOptions) string {
	return fmt.Sprintf("opendir|%s|%s", kind, opts.Title)
}

// guardedOpenDirectoryDialog wraps a.OpenDirectoryDialog with
// the in-flight guard. Same crash class as the save + file
// pickers; same dialog guard law applies.
func (a *App) guardedOpenDirectoryDialog(dupKey string, opts runtime.OpenDialogOptions) (string, bool, bool) {
	if dupKey == "" {
		dupKey = guardedOpenDirectoryDialogKey("opendir", opts)
	}
	admitted, entry := a.enterInFlight(dupKey)
	if !admitted {
		return "", false, false
	}
	defer a.leaveInFlight(dupKey, entry)
	path, err := a.OpenDirectoryDialog(opts)
	if err != nil || path == "" {
		return "", true, false
	}
	return path, true, true
}

// guardedOpenMultipleFilesDialogKey mirrors guardedOpenFileDialogKey
// for multi-select file pickers.
func guardedOpenMultipleFilesDialogKey(kind string, opts runtime.OpenDialogOptions) string {
	return fmt.Sprintf("openmulti|%s|%s|%v", kind, opts.DefaultFilename, opts.Filters)
}

// guardedOpenMultipleFilesDialog wraps a.OpenMultipleFilesDialog
// with the in-flight guard. Multi-select pickers land on the
// same UI thread as the single-select pickers; same crash
// class; same dialog guard law applies.
func (a *App) guardedOpenMultipleFilesDialog(dupKey string, opts runtime.OpenDialogOptions) ([]string, bool, bool) {
	if dupKey == "" {
		dupKey = guardedOpenMultipleFilesDialogKey("openmulti", opts)
	}
	admitted, entry := a.enterInFlight(dupKey)
	if !admitted {
		return nil, false, false
	}
	defer a.leaveInFlight(dupKey, entry)
	paths, err := a.OpenMultipleFilesDialog(opts)
	if err != nil || len(paths) == 0 {
		return nil, true, false
	}
	return paths, true, true
}


// --- handleLegacyExportRedirect ---
func (a *App) handleLegacyExportRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/share", http.StatusSeeOther)
}


// --- main export handlers (JSON, InsightsPDF, CSV, iCal, StaticArchive, DatabasePDF, Backup, SharedArchive, BugReport) ---

// enqueueExportOpt configures the response shape written by
// enqueueExport / enqueueExportWithResult. The zero value is the
// new Option C contract: 200 OK + X-DixieData-Redirect, read by
// dispatchDixieDataForm in frontend/app.js.
type enqueueExportOpt struct {
	// NativeRedirect keeps the legacy 303 + Location contract for
	// callers reached by a plain <form method="post"> (the browser
	// follows 303 natively; the custom dispatcher doesn't intercept).
	// Only handleExportStaticArchive needs this carve-out — every
	// share-page button now goes through the dispatcher, which
	// understands the new contract.
	NativeRedirect bool
}

// enqueueExport starts a background export job and writes a redirect
// to /jobs/{id}. Used by every export handler that wraps long-running
// work. Mirrors the shape of enqueueStaticArchive / enqueueDatabasePDF
// (which predate this helper). When dupKey is non-empty the new JobID
// is recorded against the in-flight entry so a duplicate request that
// arrives after the job has been queued can be redirected to the same
// status page (issue #130).
//
// Response shape:
//   - default: 200 OK + X-DixieData-Redirect: /jobs/{id} (Option C contract)
//   - if opts.NativeRedirect: 303 See Other + Location: /jobs/{id} (legacy)
//
// The X-DixieData-Redirect branch is read by dispatchDixieDataForm in
// frontend/app.js, which calls window.location.assign() to navigate.
// The 303 branch is read by the browser natively — used only by
// handleExportStaticArchive, which is reached by a plain <form
// method="post"> without hx-post.
func (a *App) enqueueExport(dupKey, kind string, work func(ctx context.Context, p *jobs.Progress) error, path string, w http.ResponseWriter, opts ...enqueueExportOpt) {
	var jobID string
	jobID = a.jobs.Start(kind, func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, "Preparing")
		err := work(ctx, p)
		if err == nil && path != "" {
			a.jobs.SetResultPath(jobID, path)
			p.Set(100, "Done")
		}
		return err
	})
	if dupKey != "" {
		if actual, loaded := a.inFlight.Load(dupKey); loaded {
			if entry, ok := actual.(*inFlightEntry); ok {
				entry.JobID = jobID
			}
		}
	}
	writeExportRedirect(w, "/jobs/"+jobID, opts...)
}

// enqueueExportWithResult is the stats-aware counterpart of
// enqueueExport. The work callback returns a jobs.JobResult
// (records / images / sources for structured exports) that is
// recorded against the job before completion, so /jobs/{id}
// renders per-kind stats on the terminal summary card (see
// jobs.Summary and the appendExportStats helper in jobs.go).
//
// Existing workers that have no stats to record (soldier_pdf,
// soldier_jpg, monthly_pdf, insights_pdf, bug_report,
// image_import) continue to use enqueueExport unchanged. New
// workers that compute per-record counts use this helper with
// the matching ExportXxxWithStats service method.
func (a *App) enqueueExportWithResult(dupKey, kind string, work func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error), path string, w http.ResponseWriter, opts ...enqueueExportOpt) {
	var jobID string
	jobID = a.jobs.Start(kind, func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, "Preparing")
		result, err := work(ctx, p)
		if err == nil {
			if path != "" {
				result.Path = path
			}
			a.jobs.SetResult(jobID, result)
			p.Set(100, "Done")
		}
		return err
	})
	if dupKey != "" {
		if actual, loaded := a.inFlight.Load(dupKey); loaded {
			if entry, ok := actual.(*inFlightEntry); ok {
				entry.JobID = jobID
			}
		}
	}
	writeExportRedirect(w, "/jobs/"+jobID, opts...)
}

// writeExportRedirect writes the response that takes the user to the
// post-then-navigate destination. Default shape: 200 OK +
// X-DixieData-Redirect, read by dispatchDixieDataForm. With
// NativeRedirect: 303 + Location, followed natively by the browser
// (used for the static archive's plain-<form> carve-out).
//
// HX-Redirect is no longer written. htmx is not the dispatcher in this
// codebase; the custom JS reads X-DixieData-Redirect. See
// internal/appshell/redirect_headers_test.go (TestPostThenNavigateUsesDixieRedirect)
// for the source-scan regression net.
func writeExportRedirect(w http.ResponseWriter, target string, opts ...enqueueExportOpt) {
	native := false
	if len(opts) > 0 {
		native = opts[0].NativeRedirect
	}
	if native {
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusSeeOther)
		return
	}
	w.Header().Set("X-DixieData-Redirect", target)
	w.WriteHeader(http.StatusOK)
}

func (a *App) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	opts := runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	}
	dupKey := guardedSaveFileDialogKey("json_export", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}
	a.enqueueExportWithResult(dupKey, "json_export", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
		p.Set(20, "Writing JSON")
		// Without a shimmer the bar would jump 20 → 100 the moment
		// the encode pass completes. With it the bar walks to 95
		// across the work, then Set(100, Done) wins the last write.
		p.Shimmer(ctx, 20, 95, 30*time.Second, "Encoding JSON…")
		records, images, sources, err := a.export.ExportJSONWithStats(path)
		return jobs.JobResult{Records: records, Images: images, Sources: sources}, err
	}, path, w)
}

func (a *App) handleExportInsightsPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the export form.", err)
		return
	}
	snapshot, err := a.analytics.Snapshot()
	if err != nil {
		respondInternal(w, r, "Could not build the insights snapshot.", err)
		return
	}
	options := parsePDFOptionsRequest(r, "P", false)
	opts := runtime.SaveDialogOptions{
		DefaultFilename: pdfReportName("dixiedata-archive-insights", options, false),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF", Pattern: "*.pdf"},
		},
	}
	dupKey := guardedSaveFileDialogKey("insights_pdf", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}
	a.enqueueExport(dupKey, "insights_pdf", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, "Rendering analytics PDF")
		return a.export.ExportAnalyticsSummaryPDF(path, snapshot, options)
	}, path, w)
}

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	opts := runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.xlsx",
		Filters: []runtime.FileFilter{
			{DisplayName: "Excel workbook", Pattern: "*.xlsx"},
		},
	}
	dupKey := guardedSaveFileDialogKey("excel_export", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}
	a.enqueueExportWithResult(dupKey, "excel_export", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
		p.Set(20, "Building workbook")
		// Walk 20 → 90 across the workbook build (multi-sheet, can
		// take seconds for 100+ records).
		p.Shimmer(ctx, 20, 90, 30*time.Second, "Writing workbook…")
		records, images, sources, err := a.export.ExportExcelWithStats(path)
		return jobs.JobResult{Records: records, Images: images, Sources: sources}, err
	}, path, w)
}

func (a *App) handleExportICalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	opts := runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-anniversaries.ics",
		Filters: []runtime.FileFilter{
			{DisplayName: "iCalendar", Pattern: "*.ics"},
		},
	}
	dupKey := guardedSaveFileDialogKey("icalendar_export", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}
	preferences, err := a.google.ManagedEventPreferences()
	if err != nil {
		respondInternal(w, r, "Could not load Google Calendar preferences.", err)
		return
	}
	a.enqueueExportWithResult(dupKey, "icalendar_export", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
		p.Set(20, "Building iCalendar file")
		records, images, sources, err := a.export.ExportICalendarWithStats(path, preferences)
		return jobs.JobResult{Records: records, Images: images, Sources: sources}, err
	}, path, w)
}

func (a *App) handleExportStaticArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defaultName := "DixieData_Archive.zip"
	if suggested, err := a.export.StaticArchiveFileName(time.Now()); err == nil {
		defaultName = suggested
	}
	opts := runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP archive", Pattern: "*.zip"},
		},
	}
	dupKey := guardedSaveFileDialogKey("static_archive", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}
	// Always enqueue — static archive export is heavy and the user
	// benefits from the persistent progress slot regardless of which
	// page they navigate to while it runs.
	// NativeRedirect: true — handleExportStaticArchive is reached by
	// a plain <form method="post"> on share.templ (no hx-post), so
	// the browser follows the 303 + Location natively. The custom
	// dispatcher is not in the path; we can't rely on
	// X-DixieData-Redirect being read.
	a.enqueueExport(dupKey, "static_archive", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, "Gathering images")
		return a.export.ExportStaticArchive(path, a.dataDir)
	}, path, w, enqueueExportOpt{NativeRedirect: true})
}

// enqueueStaticArchive was removed in phase 4 of the feedback /
// progress redesign: the unified enqueueExport helper handles all
// background exports and the static-archive export always uses it
// now (no more synchronous fallback path).

func (a *App) handleExportDatabasePDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := parsePrintSettingsRequest(r)
	if err != nil {
		respondValidation(w, r, "Print settings could not be read.", err)
		return
	}
	path, dupKey, err := a.exportFullDatabasePDFPath(settings)
	if errors.Is(err, errExportInFlight) {
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Printable PDF export cancelled.", err)
		return
	}
	// Always enqueue — printable PDF is heavy.
	a.enqueueExportWithResult(dupKey, "database_pdf", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
		p.Set(5, "Building archive")
		// Walk 5 → 90 across the PDF render (which is slow for large
		// archives) so the user sees progress move; the worker calls
		// Set(100, 'Done') via enqueueExportWithResult after this
		// returns, which wins the last write.
		p.Shimmer(ctx, 5, 90, 60*time.Second, "Compiling printable archive…")
		records, images, sources, err := a.export.ExportFullDatabasePDFWithStats(path, settings)
		return jobs.JobResult{Records: records, Images: images, Sources: sources}, err
	}, path, w)
}

func (a *App) ExportFullDatabasePDF(settings archive.PrintSettings) (string, error) {
	path, dupKey, err := a.exportFullDatabasePDFPath(settings)
	if errors.Is(err, errExportInFlight) {
		// A duplicate request hit the guard. If a job is already in
		// flight under the same key, surface a status link so the
		// user can monitor it instead of being told to wait for a
		// dialog that has already been dismissed.
		if jobID := a.inFlightJobID(dupKey); jobID != "" {
			return fmt.Sprintf("Printable PDF export already in progress. <a href=\"/jobs/%s\">View status</a>.", jobID), nil
		}
		return "Printable PDF export already in progress; please wait for the save dialog.", nil
	}
	if err != nil {
		return "", err
	}
	if path == "" {
		return "Printable PDF export cancelled.", nil
	}
	if err := a.export.ExportFullDatabasePDF(path, settings); err != nil {
		return "", err
	}
	return exportLinkMarkup("Printable PDF ready:", path), nil
}

// exportFullDatabasePDFPath normalizes settings and prompts for a
// destination via the SaveFileDialog. Returns the user-chosen path,
// the dupKey used to guard the dialog (so the caller can thread it
// into enqueueExport and update the in-flight JobID), and an error.
// When the user cancels the dialog the path is empty and err is
// nil. Used by both handleExportDatabasePDF (HTTP) and
// ExportFullDatabasePDF (Wails binding) so the SaveFileDialog block
// stays in one place.
//
// Like the other SaveFileDialog call sites, this carries a
// sync.Map-based in-flight guard against the WebView2 focus race
// described in handleCalendarPDF. Without it, a double-click on
// the share-page "Printable PDF" button or two parallel calls from
// the Wails binding queue a second native dialog while the first
// is still up and crash the WebView2 control.
func (a *App) exportFullDatabasePDFPath(settings archive.PrintSettings) (string, string, error) {
	settings = settings.Normalize()
	dupKey := fmt.Sprintf("db-pdf|%s", printableArchivePDFName(settings))
	admitted, entry := a.enterInFlight(dupKey)
	if !admitted {
		debug.FromContext(context.Background()).Debug("exportFullDatabasePDFPath duplicate request rejected")
		return "", dupKey, errExportInFlight
	}
	defer a.leaveInFlight(dupKey, entry)
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: printableArchivePDFName(settings),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		return "", dupKey, err
	}
	return path, dupKey, nil
}

func parsePrintSettingsRequest(r *http.Request) (archive.PrintSettings, error) {
	if err := r.ParseForm(); err != nil {
		return archive.PrintSettings{}, fmt.Errorf("failed to parse print settings")
	}
	// Issue #69: route through pkg/exportbridge so the appshell
	// and tools/tune parse identically. The bridge's
	// PrintSettingsFromForm is the canonical parser; this thin
	// wrapper exists to preserve the http.Request signature and
	// surface a friendly error message.
	return exportbridge.PrintSettingsFromForm(r.Form)
}

func parsePDFOptionsRequest(r *http.Request, defaultOrientation string, defaultIncludeImages bool) archive.PDFOptions {
	if err := r.ParseForm(); err != nil {
		// Fall through with empty form values; PDFOptionsFromForm
		// will fall back to its defaults.
	}
	return exportbridge.PDFOptionsFromForm(r.Form, defaultOrientation, defaultIncludeImages)
}

func setToastHeader(w http.ResponseWriter, message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	w.Header().Set("X-DixieData-Toast", sanitiseToastForHeader(message))
	w.Header().Set("X-DixieData-Toast-Type", "success")
}

func setToastHeaderWithType(w http.ResponseWriter, message, kind string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	if strings.TrimSpace(kind) == "" {
		kind = "success"
	}
	w.Header().Set("X-DixieData-Toast", sanitiseToastForHeader(message))
	w.Header().Set("X-DixieData-Toast-Type", kind)
}

// toastHeaderASCIIReplacements maps Unicode punctuation to its ASCII
// twin for use inside the X-DixieData-Toast HTTP response header.
//
// Chromium / WebView2 decodes HTTP/1.x response headers as
// Windows-1252 (per WHATWG Fetch), not UTF-8. Bytes above 0x7F get
// reinterpreted as separate Windows-1252 codepoints, producing
// visible mojibake like "Shared archive import startedâ¦" when the
// source contains a real U+2026 HORIZONTAL ELLIPSIS rune. The Go
// stdlib writes header bytes raw (RFC 7230 §3.2.4 allows
// ISO-8859-1), so the corruption happens on the browser side and
// cannot be fixed server-side without changing the wire format.
//
// The table deliberately covers only punctuation that has a clean
// ASCII twin. Stripping every byte above 0x7F would silently mangle
// future toasts that quote user input (e.g. "Saved record for
// José") — user data passes through unchanged on purpose. See
// docs/adr/0005-toast-header-ascii-safe.md for the trade-off.
var toastHeaderASCIIReplacements = map[rune]string{
	'\u2026': "...", // … HORIZONTAL ELLIPSIS
	'\u2014': "--",  // — EM DASH
	'\u2013': "-",   // – EN DASH
	'\u2018': "'",   // ‘ LEFT SINGLE QUOTATION MARK
	'\u2019': "'",   // ’ RIGHT SINGLE QUOTATION MARK
	'\u201C': `"`,   // “ LEFT DOUBLE QUOTATION MARK
	'\u201D': `"`,   // ” RIGHT DOUBLE QUOTATION MARK
	'\u00A0': " ",   //   NO-BREAK SPACE
	'\u2192': "->",  // → RIGHTWARDS ARROW
	'\u2713': "OK",  // ✓ CHECK MARK
	'\u00B7': "*",   // · MIDDLE DOT
	'\u00A7': "",    // § SECTION SIGN (omit)
}

// sanitiseToastForHeader rewrites every Unicode punctuation rune in
// message to its ASCII twin. Bytes that are not in the replacement
// table pass through unchanged so future toasts that quote user
// input (accented names, non-Latin scripts) survive the Windows-1252
// decoding unmodified.
func sanitiseToastForHeader(message string) string {
	if message == "" {
		return message
	}
	var b strings.Builder
	b.Grow(len(message))
	for _, r := range message {
		if replacement, ok := toastHeaderASCIIReplacements[r]; ok {
			b.WriteString(replacement)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// setInfoToastHeader writes an "in-progress" toast header. The
// frontend uses kind="info" to mark the toast as auto-dismissable
// (same behaviour as success), but the wording in the toast
// heading ("Heads up" instead of "Success") accurately reflects
// that the action is still running rather than complete. Used by
// every handler that enqueues a background job — image import,
// shared archive import, memorial JSON import, Google Drive /
// Sheets exports, duplicate audit, bulk reviews, orphan cleanup.
// Issue #132.
func setInfoToastHeader(w http.ResponseWriter, message string) {
	setToastHeaderWithType(w, message, "info")
}

func (a *App) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: backupArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
		},
	}
	dupKey := guardedSaveFileDialogKey("backup_archive", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}

	a.enqueueExportWithResult(dupKey, "backup_archive", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
		p.Set(10, "Gathering archive data")
		manifest, err := a.backup.Export(path, a.dataDir)
		if err == nil {
			p.Set(80, "Compressing backup")
			// The compression pass is single-shot (no natural
			// sub-steps). Shimmer 80 → 95 so the bar continues to
			// move during the actual zip write; the registry's
			// Set(100, 'Done') wins the last write.
			p.Shimmer(ctx, 80, 95, 20*time.Second, "Compressing…")
		}
		return jobs.JobResult{
			Records: manifest.Soldiers,
			Images:  manifest.Images,
			Sources: manifest.Records,
		}, err
	}, path, w)
}

func (a *App) handleExportSharedArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: sharedArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	}
	dupKey := guardedSaveFileDialogKey("shared_archive", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}

	a.enqueueExportWithResult(dupKey, "shared_archive", func(ctx context.Context, p *jobs.Progress) (jobs.JobResult, error) {
		p.Set(10, "Building shared archive")
		// Issue #183: read the archive_meta.include_tags toggle
		// at run-time so the user-controlled PATCH handler on
		// /share/export-options changes the next export's payload
		// without restarting the app.
		includeTags := false
		if a.archiveMeta != nil {
			includeTags = a.archiveMeta.IncludeTags(ctx, records.ArchiveKindShared)
		}
		// Shimmer to keep the bar visible during the merge-review
		// compare pass + zip write. enqueueExportWithResult's
		// Set(100, 'Done') wins the last write.
		p.Shimmer(ctx, 10, 95, 45*time.Second, "Staging shared archive…")
		manifest, err := a.backup.ExportSharedWithTags(path, a.dataDir, includeTags)
		return jobs.JobResult{
			Records: manifest.Soldiers,
			Images:  manifest.Images,
			Sources: manifest.Records,
		}, err
	}, path, w)
}

func (a *App) handleExportBugReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: archive.DiagnosticsBundleName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "Bug report bundle", Pattern: "*.zip"},
		},
	}
	dupKey := guardedSaveFileDialogKey("bug_report", opts)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Export cancelled.", nil)
		return
	}

	a.enqueueExport(dupKey, "bug_report", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(10, "Collecting diagnostics")
		_, err := a.diagnostics.Export(path, a.dataDir)
		return err
	}, path, w)
}
