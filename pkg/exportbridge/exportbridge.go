// Package exportbridge wraps DixieData's PDF export pipeline behind
// a thin facade so the appshell and external tools (notably
// tools/tune) can drive the same code path without each one
// re-implementing the wiring. Issue #69.
//
// The bridge does NOT add new export behaviour; it is a thin
// wrapper over internal/archive.ExportService plus the appshell's
// parsePrintSettingsRequest helper. Every export the bridge can
// produce is a PDF the appshell can produce for the same input.
package exportbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/pkg/render"
)

// BulkRenderer is the entry point used by both the appshell and
// external tools. Construct one via NewBulkRenderer and drive the
// render methods.
type BulkRenderer struct {
	export      *archive.ExportService
	soldier     *archive.SoldierService
	anniversary *archive.AnniversaryService
	analytics   *archive.AnalyticsService
	dataDir     string
	dbPath      string
}

// NewBulkRenderer constructs a BulkRenderer for the DixieData
// archive at dbPath. dataDir is the parent directory of dbPath
// (where image files live); the appshell passes a.dataDir, tools
// pass filepath.Dir(dbPath).
//
// SetRegistry must be called once before RenderBulk / RenderSingle
// will produce a PDF; the appshell does this at startup, tools do
// it via pkg/render.NewRegistry.
func NewBulkRenderer(dbPath, dataDir string) (*BulkRenderer, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	soldierSvc := archive.NewSoldierService(database)
	anniversarySvc := archive.NewAnniversaryService(database)
	analyticsSvc := archive.NewAnalyticsService(database)
	exportSvc := archive.NewExportService(database, soldierSvc)
	// Resolve the data dir to an absolute path. A relative
	// data dir would cascade into the per-image ResolvedPath
	// strings (image staging reads those to copy source files
	// into the workdir) and a relative ResolvedPath fails the
	// os.Stat in pkg/render.stageOneImage — images silently
	// don't render.
	absDataDir, absErr := filepath.Abs(dataDir)
	if absErr != nil {
		return nil, fmt.Errorf("resolve data dir: %w", absErr)
	}
	exportSvc.SetDataDir(absDataDir)
	return &BulkRenderer{
		export:      exportSvc,
		soldier:     soldierSvc,
		anniversary: anniversarySvc,
		analytics:   analyticsSvc,
		dataDir:     absDataDir,
		dbPath:      dbPath,
	}, nil
}

// GetByID returns a single soldier by ID. Mirrors
// internal/records.SoldierService.GetByID. Useful for
// --mode record renders where the caller has an ID but not a
// fully-populated models.Soldier.
func (b *BulkRenderer) GetByID(id int64) (*models.Soldier, error) {
	return b.soldier.GetByID(id)
}

// List returns a page of soldiers. Mirrors
// internal/records.SoldierService.List.
func (b *BulkRenderer) List(page, pageSize int) ([]models.Soldier, int, error) {
	return b.soldier.List(page, pageSize)
}

// SetRegistry wires the typst-backed Registry into the underlying
// export service. The appshell calls this at startup; tools call
// it after constructing the renderer. After SetRegistry returns
// the renderer is ready to produce PDFs.
func (b *BulkRenderer) SetRegistry(reg *render.Registry) {
	b.export.SetRegistry(reg)
}

// SetOutputFormat switches subsequent RenderSingle / RenderBulk
// calls between PDF (default) and native SVG output. The format
// applies to every typst-backed export until changed again.
func (b *BulkRenderer) SetOutputFormat(format string) {
	b.export.SetOutputFormat(format)
}

// Registry returns the underlying typst Registry, or nil when no
// Registry has been wired via SetRegistry. Useful for callers that
// need to flip output formats at runtime.
func (b *BulkRenderer) Registry() *render.Registry {
	return b.export.Registry()
}

// DBPath returns the SQLite path the renderer was opened with.
// Used by tests to discover the path-based data dir.
func (b *BulkRenderer) DBPath() string { return b.dbPath }

// DataDir returns the on-disk root used to resolve image paths.
func (b *BulkRenderer) DataDir() string { return b.dataDir }

// RenderBulk renders the bulk printable archive to out. It mirrors
// internal/archive.ExportService.ExportFullDatabasePDF exactly:
// one typst invocation that loops over a sorted array of records.
//
// Returns ([]RecordError, error). On full success, errors is empty
// and err is nil. On failure, err is the first fatal error (which
// halts the bulk render); errors is the per-record list when err
// is non-nil but the bulk render produced partial output. The
// single-export failure mode today halts on first error; per-
// record error accumulation is reserved for a future slice.
func (b *BulkRenderer) RenderBulk(ctx context.Context, settings render.PrintSettings, out io.Writer) ([]RecordError, error) {
	settings = settings.Normalize()
	// Issue #68: the bulk path no longer force-clears the
	// per-record Template field. PrintSettings.BulkTemplate is
	// the authoritative override; the Registry's bulk guard
	// rejects a per-record template assignment. The default
	// (BulkTemplate unset) falls through to bulk_soldier.typ.

	if path := filePathFromWriter(out); path != "" {
		err := b.export.ExportFullDatabasePDF(path, settings)
		return nil, err
	}

	tmp, err := os.CreateTemp("", "dixiedata-exportbridge-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := b.export.ExportFullDatabasePDF(tmp.Name(), settings); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil, err
	}
	_, err = out.Write(data)
	return nil, err
}

// RenderSingle renders one soldier's PDF to out. Mirrors
// archive.ExportService.ExportSoldierPDF.
func (b *BulkRenderer) RenderSingle(ctx context.Context, soldier models.Soldier, opts render.PDFOptions, out io.Writer) error {
	opts = opts.Normalize("L", true)
	// Resolve image paths against dataDir if the caller has not
	// already populated ResolvedPath. Mirrors the appshell's
	// handleSoldierPDF pre-render loop. The dataDir has been
	// resolved to an absolute path in NewBulkRenderer, so the
	// result here is also absolute and the image-staging step
	// (which does an os.Stat) can find the source files.
	for i := range soldier.Images {
		if strings.TrimSpace(soldier.Images[i].ResolvedPath) == "" &&
			strings.TrimSpace(soldier.Images[i].FilePath) != "" {
			soldier.Images[i].ResolvedPath = filepath.Join(
				b.dataDir,
				filepath.FromSlash(soldier.Images[i].FilePath),
			)
		}
	}

	if path := filePathFromWriter(out); path != "" {
		return b.export.ExportSoldierPDF(path, soldier, opts)
	}
	tmp, err := os.CreateTemp("", "dixiedata-exportbridge-*.pdf")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := b.export.ExportSoldierPDF(tmp.Name(), soldier, opts); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

// RenderAnniversary renders the monthly anniversary report for the
// given month (1-12). Mirrors the appshell's
// handleMonthPDF path. Uses templates/anniversary.typ via the
// Registry. The AnniversaryService builds the calendar from the
// live archive.
func (b *BulkRenderer) RenderAnniversary(ctx context.Context, month int, opts render.PDFOptions, out io.Writer) error {
	opts = opts.Normalize("P", true)
	calendar, err := b.anniversary.GetMonthCalendar(month)
	if err != nil {
		return fmt.Errorf("build anniversary calendar: %w", err)
	}
	if path := filePathFromWriter(out); path != "" {
		return b.export.ExportMonthlyAnniversaryPDF(path, month, calendar, opts)
	}
	tmp, err := os.CreateTemp("", "dixiedata-exportbridge-anniversary-*.pdf")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := b.export.ExportMonthlyAnniversaryPDF(tmp.Name(), month, calendar, opts); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

// RenderInsights renders the archive summary report. Mirrors
// handleExportInsightsPDF. Uses templates/analytics_summary.typ
// via the Registry.
func (b *BulkRenderer) RenderInsights(ctx context.Context, opts render.PDFOptions, out io.Writer) error {
	opts = opts.Normalize("P", false)
	snapshot, err := b.analytics.Snapshot()
	if err != nil {
		return fmt.Errorf("analytics snapshot: %w", err)
	}
	if path := filePathFromWriter(out); path != "" {
		return b.export.ExportAnalyticsSummaryPDF(path, snapshot, opts)
	}
	tmp, err := os.CreateTemp("", "dixiedata-exportbridge-insights-*.pdf")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := b.export.ExportAnalyticsSummaryPDF(tmp.Name(), snapshot, opts); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

// ResolveTemplate exposes pkg/render.Registry.Resolve for the
// diagnostic case ("which template would the bulk path pick for
// recordType X given settings Y?").
func (b *BulkRenderer) ResolveTemplate(settings render.PrintSettings, recordType string) (render.Template, error) {
	return b.export.ResolveTemplate(settings, recordType)
}

// StageImages exposes the image-staging step for the diagnostic
// case ("would the typst renderer find these images on disk?").
func (b *BulkRenderer) StageImages(workDir string, data map[string]any) error {
	return b.export.StageImages(workDir, data)
}

// Close releases the underlying SQLite handle.
func (b *BulkRenderer) Close() error {
	return b.export.Close()
}

// RecordError describes a per-record failure encountered during a
// bulk render. Today the bulk render halts on first error, so
// RecordError is reserved for future partial-render support.
type RecordError struct {
	RecordID  int64  `json:"record_id"`
	DisplayID string `json:"display_id,omitempty"`
	Error     string `json:"error"`
}

// PrintSettingsFromForm builds a PrintSettings from URL form values.
// Moved verbatim from internal/appshell/exports_handlers.go so the
// appshell and tools/tune parse identically.
//
// If scope == "selected" with no selected_ids, this returns an
// error -- matches the appshell's existing contract.
func PrintSettingsFromForm(values url.Values) (render.PrintSettings, error) {
	selectedIDs, err := parseSelectedIDs(values["selected_ids"])
	if err != nil {
		return render.PrintSettings{}, err
	}
	settings := render.PrintSettings{
		Scope:                         strings.TrimSpace(values.Get("scope")),
		Orientation:                   strings.TrimSpace(values.Get("orientation")),
		SingleRecordTemplate:          strings.TrimSpace(values.Get("template")),
		BulkTemplate:                  strings.TrimSpace(values.Get("bulk_template")),
		PrinterFriendly:               values.Get("printer_friendly") != "",
		FullBiographyPage:             values.Get("full_biography_page") != "",
		SortBy:                        strings.TrimSpace(values.Get("sort_by")),
		GroupByUnit:                   values.Get("group_by_unit") != "",
		GroupByPensionState:           values.Get("group_by_pension_state") != "",
		GroupByConfederateHomeStatus:  values.Get("group_by_confederate_home_status") != "",
		GroupByBuriedIn:               values.Get("group_by_buried_in") != "",
		FilterBuriedIn:                append([]string(nil), values["filter_buried_in"]...),
		FilterEntryTypes:              append([]string(nil), values["filter_entry_type"]...),
		FilterUnits:                   append([]string(nil), values["filter_unit"]...),
		FilterPensionStates:           append([]string(nil), values["filter_pension_state"]...),
		FilterConfederateHomeStatuses: append([]string(nil), values["filter_confederate_home_status"]...),
		ExportAll:                     values.Get("export_all") != "",
		SelectedIDs:                   selectedIDs,
	}.Normalize()
	if settings.Scope == render.PrintScopeSelected && len(settings.SelectedIDs) == 0 {
		return render.PrintSettings{}, errors.New("select at least one record or choose a different export scope")
	}
	return settings, nil
}

// PDFOptionsFromForm builds PDFOptions from URL form values.
// Moved verbatim from internal/appshell/exports_handlers.go.
func PDFOptionsFromForm(values url.Values, defaultOrientation string, defaultIncludeImages bool) render.PDFOptions {
	options := render.PDFOptions{
		Orientation:     strings.TrimSpace(values.Get("orientation")),
		PrinterFriendly: values.Get("printer_friendly") != "",
		IncludeImages:   parseBoolFormValueDefault(values, "include_images", defaultIncludeImages),
	}
	return options.Normalize(defaultOrientation, defaultIncludeImages)
}

// parseSelectedIDs extracts int64 IDs from a list of strings.
// Mirrors the appshell's parseSelectedSoldierIDs helper.
func parseSelectedIDs(values []string) ([]int64, error) {
	out := make([]int64, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid soldier id %q: %w", v, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// parseBoolFormValueDefault returns the form value's boolean
// interpretation, falling back to default when absent.
func parseBoolFormValueDefault(values url.Values, key string, fallback bool) bool {
	raw, ok := values[key]
	if !ok || len(raw) == 0 {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(raw[0])) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off", "":
		return false
	}
	return fallback
}

// filePathFromWriter extracts a filesystem path from a writer
// when it is backed by an *os.File. Returns empty string when the
// writer is in-memory (e.g. bytes.Buffer); callers should fall back
// to a temp file in that case.
func filePathFromWriter(w io.Writer) string {
	if f, ok := w.(*os.File); ok {
		return f.Name()
	}
	return ""
}