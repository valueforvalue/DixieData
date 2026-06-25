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
	if err := a.export.ExportJSON(path); err != nil {
		respondInternal(w, r, "Could not write the JSON export.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("JSON saved to %s", path))
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
	if err := a.export.ExportAnalyticsSummaryPDF(path, snapshot, options); err != nil {
		respondInternal(w, r, "Could not write the insights PDF.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Analytics PDF saved to %s", path))
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
	if err := a.export.ExportExcel(path); err != nil {
		respondInternal(w, r, "Could not write the Excel workbook.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Excel workbook saved to %s", path))
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
	if err := a.export.ExportICalendar(path, preferences); err != nil {
		respondInternal(w, r, "Could not write the iCalendar file.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("iCalendar saved to %s", path))
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
	if r.URL.Query().Get("async") == "1" {
		a.enqueueStaticArchive(r.Context(), path, w)
		return
	}
	if err := a.export.ExportStaticArchive(path, a.dataDir); err != nil {
		respondInternal(w, r, "Could not write the static web archive.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Static web archive saved to %s", path))
}

// enqueueStaticArchive kicks off a background Static Archive export and
// responds with a 302 to the /jobs/{id} status page. Workers use the
// registry's cooperative cancellation to honour user clicks.
func (a *App) enqueueStaticArchive(parent context.Context, path string, w http.ResponseWriter) {
	var jobID string
	jobID = a.jobs.Start("static_archive", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, "Gathering images")
		err := a.export.ExportStaticArchive(path, a.dataDir)
		if err == nil {
			a.jobs.SetResultPath(jobID, path)
		}
		p.Set(100, "Done")
		return err
	})
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}

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
	settings = settings.Normalize()
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: printableArchivePDFName(settings),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Printable PDF export cancelled.", nil)
		return
	}
	if r.URL.Query().Get("async") == "1" {
		a.enqueueDatabasePDF(r.Context(), path, settings, w)
		return
	}
	if err := a.export.ExportFullDatabasePDF(path, settings); err != nil {
		respondInternal(w, r, "Could not write the printable PDF.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Printable PDF saved to %s", path))
}

// enqueueDatabasePDF kicks off a background Printable Archive PDF export
// and responds with a 302 to the /jobs/{id} status page.
func (a *App) enqueueDatabasePDF(parent context.Context, path string, settings archive.PrintSettings, w http.ResponseWriter) {
	var jobID string
	jobID = a.jobs.Start("database_pdf", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, "Building archive")
		err := a.export.ExportFullDatabasePDF(path, settings)
		if err == nil {
			a.jobs.SetResultPath(jobID, path)
		}
		p.Set(100, "Done")
		return err
	})
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}

func (a *App) ExportFullDatabasePDF(settings archive.PrintSettings) (string, error) {
	settings = settings.Normalize()
	path, err := a.SaveFileDialog( runtime.SaveDialogOptions{
		DefaultFilename: printableArchivePDFName(settings),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		return "Printable PDF export cancelled.", nil
	}
	if err := a.export.ExportFullDatabasePDF(path, settings); err != nil {
		return "", err
	}
	return exportLinkMarkup("Printable PDF ready:", path), nil
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

	manifest, err := a.backup.Export(path, a.dataDir)
	if err != nil {
		respondInternal(w, r, "Could not write the backup archive.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Backup saved to %s (%d soldiers, %d images)", path, manifest.Soldiers, manifest.Images))
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

	manifest, err := a.backup.ExportShared(path, a.dataDir)
	if err != nil {
		respondInternal(w, r, "Could not write the shared archive.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Shared archive saved to %s (%d soldiers, %d images)", path, manifest.Soldiers, manifest.Images))
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

	manifest, err := a.diagnostics.Export(path, a.dataDir)
	if err != nil {
		respondInternal(w, r, "Could not write the bug report bundle.", err)
		return
	}
	setToastHeader(w, fmt.Sprintf("Bug report bundle saved to %s (%d soldiers, %d images, %d scratch pads)", path, manifest.Soldiers, manifest.Images, manifest.Scratchpads))
}
