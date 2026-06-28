// cli_export.go — `dixiedata export ...` subcommands (Phase 4
// of cli-plan.md). Bypasses the native SaveFileDialog entirely:
// every command takes --out PATH so the destination is
// deterministic. This permanently dodges the
// wailsapp/wails#2807 WebView2 focus race for CLI users —
// there's no WebView2 in CLI mode, so MoveFocus can't fail.
//
// Dispatch is direct to the existing service methods on
// a.export / a.backup. No new business logic; just CLI plumbing.
//
//   dixiedata export pdf --soldier <id> --out <path>
//   dixiedata export pdf --month 2026-06 --out <path>
//   dixiedata export pdf --full --out <path> [--settings <json>]
//   dixiedata export jpg --soldier <id> --out <path>
//   dixiedata export json --out <path>
//   dixiedata export csv --out <path>
//   dixiedata export ical --out <path>
//   dixiedata export static-archive --out <dir>
//   dixiedata export backup --out <file>
//
// Common flags:
//   --out PATH               destination (required)
//   --orientation L|P        PDF orientation (soldier/calendar/full)
//   --no-images              exclude images (default: include)
//   --printer-friendly       optimise PDF for printing
//   --settings <json>       full PrintSettings for `export pdf --full`
package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/pkg/render"
)

// ExportKind identifies which sub-export the user wants.
type ExportKind int

const (
	ExportUnknown ExportKind = iota
	ExportPDF
	ExportJPG
	ExportJSON
	ExportCSV
	ExportICalendar
	ExportStaticArchive
	ExportBackup
)

// ExportMode further qualifies `export pdf` — which scope.
type ExportMode int

const (
	ExportModeUnknown ExportMode = iota
	ExportModeSingle     // --soldier <id>
	ExportModeMonth      // --month 2026-06
	ExportModeFull       // --full [--settings <json>]
)

// ExportOptions configures RunExport. The parser fills
// Kind/Mode/SoldierID/Month/OutPath/SettingsJSON; the runner
// fills Options/Orientation/etc. from those.
type ExportOptions struct {
	Kind         ExportKind
	Mode         ExportMode
	SoldierID    int64
	Month        int    // 1-12
	OutPath      string // required
	Orientation  string // empty = default ("L" for soldier, "P" for calendar)
	NoImages     bool
	PrinterFriendly bool
	SettingsJSON string // raw JSON for ExportModeFull; parsed lazily
	Writer       io.Writer
	App          *App
}

// RunExport dispatches to the right handler. Returns exit code
// (0 ok, 1 invalid args / not-found, 2 env error).
func RunExport(ctx context.Context, opts ExportOptions) (int, error) {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.OutPath == "" {
		return 3, fmt.Errorf("--out PATH is required")
	}
	app := opts.App
	if app == nil {
		return 2, fmt.Errorf("RunExport requires opts.App")
	}
	if app.export == nil && (opts.Kind == ExportPDF || opts.Kind == ExportJPG ||
		opts.Kind == ExportJSON || opts.Kind == ExportCSV ||
		opts.Kind == ExportICalendar || opts.Kind == ExportStaticArchive) {
		return 2, fmt.Errorf("export service not initialized; app.startup() must run first")
	}
	if app.backup == nil && opts.Kind == ExportBackup {
		return 2, fmt.Errorf("backup service not initialized")
	}

	switch opts.Kind {
	case ExportPDF:
		return runExportPDF(ctx, app, opts)
	case ExportJPG:
		return runExportJPG(ctx, app, opts)
	case ExportJSON:
		return runExportJSON(app, opts)
	case ExportCSV:
		return runExportCSV(app, opts)
	case ExportICalendar:
		return runExportICalendar(app, opts)
	case ExportStaticArchive:
		return runExportStaticArchive(app, opts)
	case ExportBackup:
		return runExportBackup(app, opts)
	default:
		return 3, fmt.Errorf("unknown export kind")
	}
}

// --- PDF (3 modes) ---

func runExportPDF(ctx context.Context, app *App, opts ExportOptions) (int, error) {
	switch opts.Mode {
	case ExportModeSingle:
		return runExportPDFSingle(app, opts)
	case ExportModeMonth:
		return runExportPDFMonth(app, opts)
	case ExportModeFull:
		return runExportPDFFull(app, opts)
	default:
		return 3, fmt.Errorf("export pdf requires --soldier <id>, --month YYYY-MM, or --full")
	}
}

func runExportPDFSingle(app *App, opts ExportOptions) (int, error) {
	soldier, err := app.soldiers.GetByID(opts.SoldierID)
	if err != nil {
		return 1, fmt.Errorf("soldier %d: %w", opts.SoldierID, err)
	}
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(app.dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	pdfOpts := render.PDFOptions{
		Orientation:     opts.Orientation,
		PrinterFriendly: opts.PrinterFriendly,
		IncludeImages:   !opts.NoImages,
	}.Normalize("L", true)

	if err := app.export.ExportSoldierPDF(opts.OutPath, *soldier, pdfOpts); err != nil {
		return 2, fmt.Errorf("export pdf: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	return 0, nil
}

func runExportPDFMonth(app *App, opts ExportOptions) (int, error) {
	calendar, err := app.anniversary.GetMonthCalendar(opts.Month)
	if err != nil {
		return 2, fmt.Errorf("load month %d: %w", opts.Month, err)
	}
	pdfOpts := render.PDFOptions{
		Orientation:     opts.Orientation,
		PrinterFriendly: opts.PrinterFriendly,
		IncludeImages:   !opts.NoImages,
	}.Normalize("P", false)

	if err := app.export.ExportMonthlyAnniversaryPDF(opts.OutPath, opts.Month, calendar, pdfOpts); err != nil {
		return 2, fmt.Errorf("export pdf --month: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	return 0, nil
}

func runExportPDFFull(app *App, opts ExportOptions) (int, error) {
	var settings render.PrintSettings
	if opts.SettingsJSON != "" {
		if err := json.Unmarshal([]byte(opts.SettingsJSON), &settings); err != nil {
			return 3, fmt.Errorf("--settings: invalid JSON: %w", err)
		}
	} else {
		// Defaults that match the GUI's "current archive" button.
		settings = render.PrintSettings{
			Orientation:      opts.Orientation,
			PrinterFriendly:  opts.PrinterFriendly,
			IncludeImages:    !opts.NoImages,
			SortBy:           "last_name",
			PrintableArchive: false,
		}
		if settings.Orientation == "" {
			settings.Orientation = "L"
		}
	}
	if err := app.export.ExportFullDatabasePDF(opts.OutPath, settings); err != nil {
		return 2, fmt.Errorf("export pdf --full: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	return 0, nil
}

// --- JPG ---

func runExportJPG(ctx context.Context, app *App, opts ExportOptions) (int, error) {
	if opts.SoldierID == 0 {
		return 3, fmt.Errorf("export jpg requires --soldier <id>")
	}
	soldier, err := app.soldiers.GetByID(opts.SoldierID)
	if err != nil {
		return 1, fmt.Errorf("soldier %d: %w", opts.SoldierID, err)
	}
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(app.dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	pdfOpts := render.PDFOptions{
		Orientation:     opts.Orientation,
		PrinterFriendly: opts.PrinterFriendly,
		IncludeImages:   !opts.NoImages,
	}.Normalize("L", true)

	cleanup, err := app.export.ExportSoldierJPG(opts.OutPath, *soldier, pdfOpts)
	if err != nil {
		return 2, fmt.Errorf("export jpg: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	for _, p := range cleanup {
		fmt.Fprintf(opts.Writer, "staged %s\n", p)
	}
	return 0, nil
}

// --- JSON / CSV / iCal ---

func runExportJSON(app *App, opts ExportOptions) (int, error) {
	if err := app.export.ExportJSON(opts.OutPath); err != nil {
		return 2, fmt.Errorf("export json: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	return 0, nil
}

func runExportCSV(app *App, opts ExportOptions) (int, error) {
	if err := app.export.ExportCSV(opts.OutPath); err != nil {
		return 2, fmt.Errorf("export csv: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	return 0, nil
}

func runExportICalendar(app *App, opts ExportOptions) (int, error) {
	prefs := models.DefaultCalendarEventPreferences()
	if err := app.export.ExportICalendar(opts.OutPath, prefs); err != nil {
		return 2, fmt.Errorf("export ical: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	return 0, nil
}

// --- Static archive ---

func runExportStaticArchive(app *App, opts ExportOptions) (int, error) {
	// ExportStaticArchive writes a ZIP at outputPath containing
	// index.html + archive_data.js + images/. The caller is
	// responsible for ensuring the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(opts.OutPath), 0o755); err != nil {
		return 2, fmt.Errorf("create parent dir for %s: %w", opts.OutPath, err)
	}
	if err := app.export.ExportStaticArchive(opts.OutPath, app.dataDir); err != nil {
		return 2, fmt.Errorf("export static-archive: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	return 0, nil
}

// --- Backup ---

func runExportBackup(app *App, opts ExportOptions) (int, error) {
	manifest, err := app.backup.Export(opts.OutPath, app.dataDir)
	if err != nil {
		return 2, fmt.Errorf("export backup: %w", err)
	}
	fmt.Fprintf(opts.Writer, "wrote %s\n", opts.OutPath)
	if manifest.Soldiers > 0 || manifest.Records > 0 || manifest.Images > 0 {
		fmt.Fprintf(opts.Writer, "manifest: soldiers=%d records=%d images=%d\n",
			manifest.Soldiers, manifest.Records, manifest.Images)
	}
	return 0, nil
}

// --- arg parsing ---

// ParseExportArgs inspects os.Args[1:] and returns the parsed
// ExportOptions. Returns ExportUnknown kind if args don't start
// with "export" + a recognised second verb.
//
// Expected shape:
//   export <kind> [--soldier N | --month M | --full] --out PATH [flags...]
func ParseExportArgs(args []string) (ExportOptions, error) {
	opts := ExportOptions{}
	if len(args) == 0 {
		return opts, nil
	}
	if args[0] != "export" {
		return opts, nil
	}
	if len(args) < 2 {
		return opts, fmt.Errorf("export requires a kind: pdf | jpg | json | csv | ical | static-archive | backup")
	}
	switch args[1] {
	case "pdf":
		opts.Kind = ExportPDF
	case "jpg":
		opts.Kind = ExportJPG
	case "json":
		opts.Kind = ExportJSON
	case "csv":
		opts.Kind = ExportCSV
	case "ical":
		opts.Kind = ExportICalendar
	case "static-archive":
		opts.Kind = ExportStaticArchive
	case "backup":
		opts.Kind = ExportBackup
	default:
		return opts, fmt.Errorf("unknown export kind %q (want pdf|jpg|json|csv|ical|static-archive|backup)", args[1])
	}

	// For PDF only, recognise mode flags. JPG requires --soldier.
	if opts.Kind == ExportPDF {
		// Walk once and pick the first mode flag we see.
		for _, a := range args[2:] {
			if strings.HasPrefix(a, "--soldier=") {
				if n, err := strconv.ParseInt(strings.TrimPrefix(a, "--soldier="), 10, 64); err == nil {
					opts.SoldierID = n
					opts.Mode = ExportModeSingle
				}
			} else if strings.HasPrefix(a, "--soldier") {
				// handled in second pass below
			} else if strings.HasPrefix(a, "--month=") {
				if m, err := parseMonthFlag(strings.TrimPrefix(a, "--month=")); err == nil {
					opts.Month = m
					opts.Mode = ExportModeMonth
				} else {
					return opts, fmt.Errorf("--month: %w", err)
				}
			} else if a == "--full" {
				opts.Mode = ExportModeFull
			}
		}
		// Second pass: --soldier N / --month N space form.
		for i := 2; i < len(args)-1; i++ {
			if args[i] == "--soldier" {
				if n, err := strconv.ParseInt(args[i+1], 10, 64); err == nil {
					opts.SoldierID = n
					if opts.Mode == ExportModeUnknown {
						opts.Mode = ExportModeSingle
					}
				}
			}
			if args[i] == "--month" {
				if m, err := parseMonthFlag(args[i+1]); err == nil {
					opts.Month = m
					if opts.Mode == ExportModeUnknown {
						opts.Mode = ExportModeMonth
					}
				}
			}
		}
	} else if opts.Kind == ExportJPG {
		for i := 2; i < len(args)-1; i++ {
			if args[i] == "--soldier" {
				if n, err := strconv.ParseInt(args[i+1], 10, 64); err == nil {
					opts.SoldierID = n
				}
			}
		}
		// also handle --soldier=N
		for _, a := range args[2:] {
			if strings.HasPrefix(a, "--soldier=") {
				if n, err := strconv.ParseInt(strings.TrimPrefix(a, "--soldier="), 10, 64); err == nil {
					opts.SoldierID = n
				}
			}
		}
	}

	// Common flags
	for i := 2; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--out" && i+1 < len(args):
			opts.OutPath = args[i+1]
		case strings.HasPrefix(a, "--out="):
			opts.OutPath = strings.TrimPrefix(a, "--out=")
		case strings.HasPrefix(a, "--orientation="):
			opts.Orientation = strings.TrimPrefix(a, "--orientation=")
		case a == "--orientation" && i+1 < len(args):
			opts.Orientation = args[i+1]
		case a == "--no-images":
			opts.NoImages = true
		case a == "--printer-friendly":
			opts.PrinterFriendly = true
		case strings.HasPrefix(a, "--settings="):
			opts.SettingsJSON = strings.TrimPrefix(a, "--settings=")
		case a == "--settings" && i+1 < len(args):
			opts.SettingsJSON = args[i+1]
		}
	}

	// Final sanity per kind.
	if opts.Kind == ExportPDF && opts.Mode == ExportModeUnknown {
		return opts, fmt.Errorf("export pdf requires --soldier <id>, --month YYYY-MM, or --full")
	}
	if opts.Kind == ExportJPG && opts.SoldierID == 0 {
		return opts, fmt.Errorf("export jpg requires --soldier <id>")
	}

	return opts, nil
}

// parseMonthFlag accepts "2026-06" or "6" (1-12).
func parseMonthFlag(s string) (int, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "-") {
		// YYYY-MM
		parts := strings.Split(s, "-")
		if len(parts) != 2 {
			return 0, fmt.Errorf("want YYYY-MM, got %q", s)
		}
		m, err := strconv.Atoi(parts[1])
		if err != nil || m < 1 || m > 12 {
			return 0, fmt.Errorf("month out of range: %q", s)
		}
		return m, nil
	}
	m, err := strconv.Atoi(s)
	if err != nil || m < 1 || m > 12 {
		return 0, fmt.Errorf("month out of range: %q", s)
	}
	return m, nil
}

// HasExportSubcommand returns true when the first arg is
// "export" and the second arg is a known export kind. main.go
// uses this to dispatch into RunExport before falling through.
func HasExportSubcommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	if args[0] != "export" {
		return false
	}
	switch args[1] {
	case "pdf", "jpg", "json", "csv", "ical", "static-archive", "backup":
		return true
	}
	return false
}

// StampExportForFilename returns a UTC timestamp string for use
// in default filenames (e.g. dixiedata-backup-20260628-153000.ddbak).
// Centralised so all default names use the same format.
func StampExportForFilename(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}