// exports_handlers.go holds the export HTTP handlers. Extracted from
// app.go as step 6 of the God-class reduction tracked in issue #42.
// Handlers stay on *App; routes registered in routes.go. The handleExport*
// methods are thin wrappers around a.export facade; handleExportDatabasePDF
// is the largest and builds a PrintSettings payload before dispatching to
// the archive service.
package appshell

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	runtime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/pkg/exportbridge"
)


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
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Export cancelled.", nil)
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
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: pdfReportName("dixiedata-archive-insights", options, false),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Analytics export cancelled.")
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
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.xlsx",
		Filters: []runtime.FileFilter{
			{DisplayName: "Excel workbook", Pattern: "*.xlsx"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Export cancelled.", nil)
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
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-anniversaries.ics",
		Filters: []runtime.FileFilter{
			{DisplayName: "iCalendar", Pattern: "*.ics"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "iCalendar export cancelled.", nil)
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
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP archive", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Static web archive export cancelled.", nil)
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
func (a *App) exportFullDatabasePDFPath(settings archive.PrintSettings) (string, error) {
	settings = settings.Normalize()
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

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: backupArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Backup export cancelled.", nil)
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

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: sharedArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Shared archive export cancelled.", nil)
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

	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: archive.DiagnosticsBundleName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "Bug report bundle", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Bug report export cancelled.", nil)
		return
	}

	a.enqueueExport("bug_report", func(p *jobs.Progress) error {
		p.Set(10, "Collecting diagnostics")
		_, err := a.diagnostics.Export(path, a.dataDir)
		return err
	}, path, w)
}
