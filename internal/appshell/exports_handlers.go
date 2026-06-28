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
	"github.com/valueforvalue/DixieData/pkg/exportbridge"
)

// errExportInFlight is returned by exportFullDatabasePDFPath when a
// SaveFileDialog is already up for the same destination. The HTTP
// handler maps it to a 429; the Wails binding maps it to a friendly
// toast via the same respondError path.
var errExportInFlight = errors.New("export already in progress; please wait for the save dialog")

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
func (a *App) guardedSaveFileDialog(kind string, opts runtime.SaveDialogOptions) (string, bool) {
	dupKey := fmt.Sprintf("export|%s|%s|%v", kind, opts.DefaultFilename, opts.Filters)
	if _, loaded := a.inFlight.LoadOrStore(dupKey, struct{}{}); loaded {
		return "", false
	}
	defer a.inFlight.Delete(dupKey)
	path, err := a.SaveFileDialog(opts)
	if err != nil || path == "" {
		return "", false
	}
	return path, true
}


// --- handleLegacyExportRedirect ---
func (a *App) handleLegacyExportRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/share", http.StatusSeeOther)
}


// --- main export handlers (JSON, InsightsPDF, CSV, iCal, StaticArchive, DatabasePDF, Backup, SharedArchive, BugReport) ---

// enqueueExport starts a background export job and writes a 303
// redirect to /jobs/{id}. Used by every export handler that wraps
// long-running work. Mirrors the shape of enqueueStaticArchive /
// enqueueDatabasePDF (which predate this helper).
func (a *App) enqueueExport(kind string, work func(p *jobs.Progress) error, path string, w http.ResponseWriter) {
	var jobID string
	jobID = a.jobs.Start(kind, func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, "Preparing")
		err := work(p)
		if err == nil && path != "" {
			a.jobs.SetResultPath(jobID, path)
			p.Set(100, "Done")
		}
		return err
	})
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}

func (a *App) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, ok := a.guardedSaveFileDialog("json_export", runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
	if !ok {
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}
	a.enqueueExport("json_export", func(p *jobs.Progress) error {
		p.Set(20, "Writing JSON")
		return a.export.ExportJSON(path)
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
	path, ok := a.guardedSaveFileDialog("insights_pdf", runtime.SaveDialogOptions{
		DefaultFilename: pdfReportName("dixiedata-archive-insights", options, false),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF", Pattern: "*.pdf"},
		},
	})
	if !ok {
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}
	a.enqueueExport("insights_pdf", func(p *jobs.Progress) error {
		p.Set(20, "Rendering analytics PDF")
		return a.export.ExportAnalyticsSummaryPDF(path, snapshot, options)
	}, path, w)
}

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, ok := a.guardedSaveFileDialog("excel_export", runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.xlsx",
		Filters: []runtime.FileFilter{
			{DisplayName: "Excel workbook", Pattern: "*.xlsx"},
		},
	})
	if !ok {
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}
	a.enqueueExport("excel_export", func(p *jobs.Progress) error {
		p.Set(20, "Building workbook")
		return a.export.ExportExcel(path)
	}, path, w)
}

func (a *App) handleExportICalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, ok := a.guardedSaveFileDialog("icalendar_export", runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-anniversaries.ics",
		Filters: []runtime.FileFilter{
			{DisplayName: "iCalendar", Pattern: "*.ics"},
		},
	})
	if !ok {
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}
	preferences, err := a.google.ManagedEventPreferences()
	if err != nil {
		respondInternal(w, r, "Could not load Google Calendar preferences.", err)
		return
	}
	a.enqueueExport("icalendar_export", func(p *jobs.Progress) error {
		p.Set(20, "Building iCalendar file")
		return a.export.ExportICalendar(path, preferences)
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
	path, ok := a.guardedSaveFileDialog("static_archive", runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP archive", Pattern: "*.zip"},
		},
	})
	if !ok {
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}
	// Always enqueue — static archive export is heavy and the user
	// benefits from the persistent progress slot regardless of which
	// page they navigate to while it runs.
	a.enqueueExport("static_archive", func(p *jobs.Progress) error {
		p.Set(5, "Gathering images")
		return a.export.ExportStaticArchive(path, a.dataDir)
	}, path, w)
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
	path, err := a.exportFullDatabasePDFPath(settings)
	if errors.Is(err, errExportInFlight) {
		respondError(w, r, KindUnavailable, "Printable PDF export already in progress; please wait for the save dialog.", err)
		return
	}
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Printable PDF export cancelled.", err)
		return
	}
	// Always enqueue — printable PDF is heavy.
	a.enqueueExport("database_pdf", func(p *jobs.Progress) error {
		p.Set(5, "Building archive")
		return a.export.ExportFullDatabasePDF(path, settings)
	}, path, w)
}

func (a *App) ExportFullDatabasePDF(settings archive.PrintSettings) (string, error) {
	path, err := a.exportFullDatabasePDFPath(settings)
	if errors.Is(err, errExportInFlight) {
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
// destination via the SaveFileDialog. Returns ("", nil) when the
// user cancels the dialog. Used by both handleExportDatabasePDF
// (HTTP) and ExportFullDatabasePDF (Wails binding) so the
// SaveFileDialog block stays in one place.
//
// Like the other SaveFileDialog call sites, this carries a
// sync.Map-based in-flight guard against the WebView2 focus race
// described in handleCalendarPDF. Without it, a double-click on
// the share-page "Printable PDF" button or two parallel calls from
// the Wails binding queue a second native dialog while the first
// is still up and crash the WebView2 control.
func (a *App) exportFullDatabasePDFPath(settings archive.PrintSettings) (string, error) {
	settings = settings.Normalize()
	dupKey := fmt.Sprintf("db-pdf|%s", printableArchivePDFName(settings))
	if _, loaded := a.inFlight.LoadOrStore(dupKey, struct{}{}); loaded {
		debug.FromContext(context.Background()).Debug("exportFullDatabasePDFPath duplicate request rejected")
		return "", errExportInFlight
	}
	defer a.inFlight.Delete(dupKey)
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: printableArchivePDFName(settings),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
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
	w.Header().Set("X-DixieData-Toast", message)
	w.Header().Set("X-DixieData-Toast-Type", "success")
}

func setToastHeaderWithType(w http.ResponseWriter, message, kind string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	if strings.TrimSpace(kind) == "" {
		kind = "success"
	}
	w.Header().Set("X-DixieData-Toast", message)
	w.Header().Set("X-DixieData-Toast-Type", kind)
}

func (a *App) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, ok := a.guardedSaveFileDialog("backup_archive", runtime.SaveDialogOptions{
		DefaultFilename: backupArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
		},
	})
	if !ok {
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}

	a.enqueueExport("backup_archive", func(p *jobs.Progress) error {
		p.Set(10, "Gathering archive data")
		_, err := a.backup.Export(path, a.dataDir)
		if err == nil {
			p.Set(80, "Compressing backup")
		}
		return err
	}, path, w)
}

func (a *App) handleExportSharedArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, ok := a.guardedSaveFileDialog("shared_archive", runtime.SaveDialogOptions{
		DefaultFilename: sharedArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	})
	if !ok {
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}

	a.enqueueExport("shared_archive", func(p *jobs.Progress) error {
		p.Set(10, "Building shared archive")
		_, err := a.backup.ExportShared(path, a.dataDir)
		return err
	}, path, w)
}

func (a *App) handleExportBugReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, ok := a.guardedSaveFileDialog("bug_report", runtime.SaveDialogOptions{
		DefaultFilename: archive.DiagnosticsBundleName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "Bug report bundle", Pattern: "*.zip"},
		},
	})
	if !ok {
		respondError(w, r, KindUnavailable, "Export already in progress; please wait for the save dialog.", nil)
		return
	}

	a.enqueueExport("bug_report", func(p *jobs.Progress) error {
		p.Set(10, "Collecting diagnostics")
		_, err := a.diagnostics.Export(path, a.dataDir)
		return err
	}, path, w)
}
