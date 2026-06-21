// exports_handlers.go holds the export HTTP handlers. Extracted from
// app.go as step 6 of the God-class reduction tracked in issue #42.
// Handlers stay on *App; routes registered in routes.go. The handleExport*
// methods are thin wrappers around a.export facade; handleExportDatabasePDF
// is the largest and builds a PrintSettings payload before dispatching to
// the archive service.
package appshell

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	runtime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/valueforvalue/DixieData/internal/archive"
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
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprintf(w, "Export cancelled.")
		return
	}
	if err := a.export.ExportJSON(path); err != nil {
		fmt.Fprintf(w, "Export failed: %v", err)
		return
	}
	fmt.Fprintf(w, "✓ Exported to %s", path)
}

func (a *App) handleExportInsightsPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	snapshot, err := a.analytics.Snapshot()
	if err != nil {
		fmt.Fprintf(w, "Analytics export failed: %v", err)
		return
	}
	options := parsePDFOptionsRequest(r, "P", false)
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
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
		fmt.Fprintf(w, "Analytics export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("Analytics report ready:", path))
}

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-export.xlsx",
		Filters: []runtime.FileFilter{
			{DisplayName: "Excel workbook", Pattern: "*.xlsx"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprintf(w, "Export cancelled.")
		return
	}
	if err := a.export.ExportExcel(path); err != nil {
		fmt.Fprintf(w, "Export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("Excel workbook ready:", path))
}

func (a *App) handleExportICalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "dixiedata-anniversaries.ics",
		Filters: []runtime.FileFilter{
			{DisplayName: "iCalendar", Pattern: "*.ics"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "iCalendar export cancelled.")
		return
	}
	preferences, err := a.google.ManagedEventPreferences()
	if err != nil {
		fmt.Fprintf(w, "iCalendar export failed: %v", err)
		return
	}
	if err := a.export.ExportICalendar(path, preferences); err != nil {
		fmt.Fprintf(w, "iCalendar export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("iCalendar ready:", path))
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
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP archive", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Static web archive export cancelled.")
		return
	}
	if err := a.export.ExportStaticArchive(path, a.dataDir); err != nil {
		fmt.Fprintf(w, "Static web archive export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup("Static web archive ready:", path))
}

func (a *App) handleExportDatabasePDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := parsePrintSettingsRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	message, err := a.ExportFullDatabasePDF(settings)
	if err != nil {
		fmt.Fprintf(w, "Printable PDF export failed: %v", err)
		return
	}
	fmt.Fprint(w, message)
}

func (a *App) ExportFullDatabasePDF(settings archive.PrintSettings) (string, error) {
	settings = settings.Normalize()
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
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
	selectedIDs, err := parseSelectedSoldierIDs(r.Form["selected_ids"])
	if err != nil {
		return archive.PrintSettings{}, err
	}
	settings := archive.PrintSettings{
		Scope:                        strings.TrimSpace(r.FormValue("scope")),
		Orientation:                  strings.TrimSpace(r.FormValue("orientation")),
		Template:                     strings.TrimSpace(r.FormValue("template")),
		PrinterFriendly:              r.FormValue("printer_friendly") != "",
		FullBiographyPage:            r.FormValue("full_biography_page") != "",
		SortBy:                       strings.TrimSpace(r.FormValue("sort_by")),
		GroupByUnit:                  r.FormValue("group_by_unit") != "",
		GroupByPensionState:          r.FormValue("group_by_pension_state") != "",
		GroupByConfederateHomeStatus: r.FormValue("group_by_confederate_home_status") != "",
		GroupByBuriedIn:              r.FormValue("group_by_buried_in") != "",
		FilterBuriedIn:               append([]string(nil), r.Form["filter_buried_in"]...),
		FilterEntryTypes:             append([]string(nil), r.Form["filter_entry_type"]...),
		FilterUnits:                  append([]string(nil), r.Form["filter_unit"]...),
		FilterPensionStates:          append([]string(nil), r.Form["filter_pension_state"]...),
		FilterConfederateHomeStatus:  append([]string(nil), r.Form["filter_confederate_home_status"]...),
		ExportAll:                    r.FormValue("export_all") != "",
		SelectedIDs:                  selectedIDs,
	}.Normalize()
	if settings.Scope == archive.PrintScopeSelected && len(settings.SelectedIDs) == 0 {
		return archive.PrintSettings{}, fmt.Errorf("select at least one record or choose a different export scope")
	}
	return settings, nil
}

func parsePDFOptionsRequest(r *http.Request, defaultOrientation string, defaultIncludeImages bool) archive.PDFOptions {
	options := archive.PDFOptions{
		Orientation:     strings.TrimSpace(r.FormValue("orientation")),
		PrinterFriendly: r.FormValue("printer_friendly") != "",
		IncludeImages:   parseBoolFormValueDefault(r.Form, "include_images", defaultIncludeImages),
	}
	return options.Normalize(defaultOrientation, defaultIncludeImages)
}

func parseBoolFormValueDefault(values url.Values, key string, fallback bool) bool {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	for _, value := range raw {
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "1", "true", "on", "yes":
			return true
		case "0", "false", "off", "no", "":
			return false
		}
	}
	return fallback
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

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: backupArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Backup export cancelled.")
		return
	}

	manifest, err := a.backup.Export(path, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Backup export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup(fmt.Sprintf("Backup ready (%d soldiers, %d images):", manifest.Soldiers, manifest.Images), path))
}

func (a *App) handleExportSharedArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: sharedArchiveName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Shared archive export cancelled.")
		return
	}

	manifest, err := a.backup.ExportShared(path, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Shared archive export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup(fmt.Sprintf("Shared archive ready (%d soldiers, %d images):", manifest.Soldiers, manifest.Images), path))
}

func (a *App) handleExportBugReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: archive.DiagnosticsBundleName(time.Now()),
		Filters: []runtime.FileFilter{
			{DisplayName: "Bug report bundle", Pattern: "*.zip"},
		},
	})
	if err != nil || path == "" {
		fmt.Fprint(w, "Bug report export cancelled.")
		return
	}

	manifest, err := a.diagnostics.Export(path, a.dataDir)
	if err != nil {
		fmt.Fprintf(w, "Bug report export failed: %v", err)
		return
	}
	fmt.Fprint(w, exportLinkMarkup(fmt.Sprintf("Bug report bundle ready (%d soldiers, %d images, %d scratch pads):", manifest.Soldiers, manifest.Images, manifest.Scratchpads), path))
}
