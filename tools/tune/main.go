// Command dixiedata-tune is a developer tool for iterating on the
// Typst-based PDF export templates. It opens a DixieData SQLite and
// renders templates through the same code path the appshell uses
// (via pkg/exportbridge) so a PDF produced by tune is byte-identical
// to one produced by the appshell for the same inputs.
//
// Issue #69. Subcommands:
//
//	dixiedata-tune render         render one record or the bulk archive
//	dixiedata-tune watch          re-render on templates/*.typ change
//	dixiedata-tune diff           diff two existing PDFs
//	dixiedata-tune list-templates list discovered typst templates
//	dixiedata-tune list-records   list records in --db
//	dixiedata-tune print-defaults print the appshell's default flag set
//	dixiedata-tune doctor        preflight: typst + templates + db + snapshots
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/pkg/exportbridge"
	"github.com/valueforvalue/DixieData/pkg/render"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// Version is the tune binary's release tag (issue #515 slice D2).
// Bumped when the binary's CLI surface or the bridge facade it
// depends on changes in a user-visible way. The --version flag
// prints this alongside the typst binary version + the bridge
// module version so a developer can pin which toolchain produced
// a given PDF.
const Version = "1.0.0"

func run(args []string) error {
	if len(args) == 0 {
		return usage(nil)
	}

	// --version: short-circuit before any global flag parsing.
	// Works without --db (which is the whole point — users want
	// to know which binary they have without standing up an
	// archive first). Issue #515 slice D2.
	for _, a := range args {
		if a == "--version" || a == "-version" {
			return printVersion()
		}
	}

	globalFS := flag.NewFlagSet("global", flag.ContinueOnError)
	dbPath := globalFS.String("db", os.Getenv("DIXIEDATA_DB"), "path to DixieData data directory (the one containing dixiedata.db, or DIXIEDATA_DB env)")
	typstPath := globalFS.String("typst", "", "path to typst binary (default: <repo>/bin/typst-windows.exe)")
	templatesDir := globalFS.String("templates", "", "path to templates directory (default: <repo>/templates)")
	dataDir := globalFS.String("data-dir", "", "path to data directory for image resolution (default: --db)")

	// Find the subcommand: the first arg whose value matches a
	// known subcommand name. Global flags can appear in any order
	// before or after each other, but the subcommand name itself
	// is exact. This is more robust than the previous heuristic
	// that tried to skip "global flag values" because callers
	// pass values like /tmp/foo.db which look like positions.
	knownSubs := map[string]bool{
		"render": true, "watch": true, "diff": true,
		"anniversary": true, "insights": true,
		"list-templates": true, "list-records": true,
		"print-defaults": true, "doctor": true,
		"help": true, "-h": true, "--help": true,
	}
	subIdx := -1
	for i, a := range args {
		if knownSubs[a] {
			subIdx = i
			break
		}
	}
	if subIdx < 0 {
		if err := globalFS.Parse(args); err != nil && !errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usage(nil)
	}
	if err := globalFS.Parse(args[:subIdx]); err != nil {
		return err
	}
	subArgs := args[subIdx+1:]
	sub := args[subIdx]

	if strings.TrimSpace(*typstPath) == "" {
		abs, err := findTypstBinary()
		if err != nil {
			return err
		}
		*typstPath = abs
	}
	if strings.TrimSpace(*templatesDir) == "" {
		abs, err := findTemplatesDir()
		if err != nil {
			return err
		}
		*templatesDir = abs
	}
	if strings.TrimSpace(*dataDir) == "" && strings.TrimSpace(*dbPath) != "" {
		// --db is documented as the data directory (the one
		// containing dixiedata.db). Use it directly rather than
		// Dir(db) which would drop ".dixiedata" -> "." for
		// a relative path like ".dixiedata".
		*dataDir = *dbPath
	}

	switch sub {
	case "render":
		return doRender(subArgs, *dbPath, *typstPath, *templatesDir, *dataDir)
	case "watch":
		return doWatch(subArgs, *dbPath, *typstPath, *templatesDir, *dataDir)
	case "diff":
		return doDiff(subArgs)
	case "anniversary":
		return doAnniversary(subArgs, *dbPath, *typstPath, *templatesDir, *dataDir)
	case "insights":
		return doInsights(subArgs, *dbPath, *typstPath, *templatesDir, *dataDir)
	case "list-templates":
		return doListTemplates(*typstPath, *templatesDir)
	case "list-records":
		return doListRecords(*dbPath, *dataDir, subArgs)
	case "print-defaults":
		return doPrintDefaults(subArgs)
	case "doctor":
		return doDoctor(subArgs)
	case "help", "-h", "--help":
		return usage(nil)
	default:
		return fmt.Errorf("unknown subcommand %q (try --help)", sub)
	}
}

// findTypstBinary walks up from CWD looking for bin/typst-*.
// Returns an absolute path so exec.Command doesn't break from
// subdirectories.
func findTypstBinary() (string, error) {
	candidates := []string{
		"bin/typst-windows.exe",
		"bin/typst-macos",
		"bin/typst-linux",
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 6; i++ {
		for _, name := range candidates {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				abs, err := filepath.Abs(candidate)
				if err != nil {
					return "", err
				}
				return abs, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no typst binary found in any bin/ directory up to 6 levels; pass --typst")
}

// findTemplatesDir walks up from CWD looking for templates/ with
// any *_landscape.typ inside. Accepting the suffix rather than
// the literal soldier_landscape.typ means the same walker serves
// both soldier and event template families (issue #358).
func findTemplatesDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "templates")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			if hasLandscapeTemplate(candidate) {
				abs, err := filepath.Abs(candidate)
				if err != nil {
					return "", err
				}
				return abs, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no templates/ directory with a *_landscape.typ found; pass --templates")
}

// hasLandscapeTemplate reports whether dir contains any
// file matching the *_landscape.typ naming convention. Used by
// findTemplatesDir to accept soldier, event, or any future
// sub-discriminator template family.
func hasLandscapeTemplate(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, "_landscape.typ") {
			return true
		}
	}
	return false
}

// usage prints the help message.
func usage(extra error) error {
	if extra != nil {
		fmt.Fprintln(os.Stderr, "error:", extra)
	}
	fmt.Fprintln(os.Stderr, "usage: dixiedata-tune [global flags] <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "global flags:")
	fmt.Fprintln(os.Stderr, "  --db PATH          path to DixieData data directory (containing dixiedata.db)")
	fmt.Fprintln(os.Stderr, "  --typst PATH       path to typst binary")
	fmt.Fprintln(os.Stderr, "  --templates PATH   path to templates directory")
	fmt.Fprintln(os.Stderr, "  --data-dir PATH    data dir for image resolution (default: --db)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "subcommands:")
	fmt.Fprintln(os.Stderr, "  render             render a template against a record or the full archive")
	fmt.Fprintln(os.Stderr, "  watch              re-render on templates/*.typ change")
	fmt.Fprintln(os.Stderr, "  diff               diff two existing PDFs")
	fmt.Fprintln(os.Stderr, "  anniversary        render the monthly anniversary report (--month N)")
	fmt.Fprintln(os.Stderr, "  insights           render the archive summary / analytics report")
	fmt.Fprintln(os.Stderr, "  list-templates     list discovered typst templates")
	fmt.Fprintln(os.Stderr, "  list-records       list records in --db")
	fmt.Fprintln(os.Stderr, "  print-defaults     print the appshell's default flag set (bulk or record)")
	return nil
}

// renderResult is the JSON shape produced by --format json.
type renderResult struct {
	Template    string         `json:"template"`
	RecordIDs   []int64        `json:"record_ids"`
	RecordCount *int           `json:"record_count,omitempty"`
	OutputPath  string         `json:"output_path"`
	SizeBytes   int64          `json:"size_bytes"`
	DurationMS  int64          `json:"duration_ms"`
	Errors      []recordError  `json:"errors"`
}

type recordError struct {
	RecordID  int64  `json:"record_id"`
	DisplayID string `json:"display_id,omitempty"`
	Error     string `json:"error"`
}

type outputFormat string

const (
	formatHuman outputFormat = "human"
	formatJSON  outputFormat = "json"
)

// renderFlags carries the CLI flags shared by render and watch.
type renderFlags struct {
	template     string
	mode         string
	recordID     int64
	recordIDsRaw string
	orientation  string
	sortBy       string
	scope        string
	selectedIDs  string
	groupByUnit  bool
	groupByPS    bool
	groupByCHS   bool
	groupByBI    bool
	filterBI     string
	filterET     string
	filterUnit   string
	filterPS     string
	filterCHS    string
	printer      bool
	fullBio      bool
	out          string
	maxPages     int
	format       outputFormat
}

func parseRenderFlags(name string, args []string) (*renderFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	rf := &renderFlags{}
	fs.StringVar(&rf.template, "template", "", "template name (soldier_landscape, bulk_soldier, etc.)")
	fs.StringVar(&rf.mode, "mode", "bulk", "render mode: record (one soldier) or bulk (full archive)")
	fs.Int64Var(&rf.recordID, "record", 0, "single record ID for --mode record")
	fs.StringVar(&rf.recordIDsRaw, "record-ids", "", "comma-separated record IDs for --mode bulk (default: all)")
	fs.StringVar(&rf.orientation, "orientation", "L", "L (landscape) or P (portrait)")
	fs.StringVar(&rf.sortBy, "sort-by", "last_name", "last_name, birth_year, or death_year")
	fs.StringVar(&rf.scope, "scope", "all", "all, filtered, or selected")
	fs.StringVar(&rf.selectedIDs, "selected-ids", "", "comma-separated IDs for --scope selected")
	fs.BoolVar(&rf.groupByUnit, "group-by-unit", false, "group output by Unit")
	fs.BoolVar(&rf.groupByPS, "group-by-pension-state", false, "group by Pension State")
	fs.BoolVar(&rf.groupByCHS, "group-by-confederate-home-status", false, "group by Confederate Home Status")
	fs.BoolVar(&rf.groupByBI, "group-by-buried-in", false, "group by Burial Location")
	fs.StringVar(&rf.filterBI, "filter-buried-in", "", "comma-separated burial locations to include")
	fs.StringVar(&rf.filterET, "filter-entry-type", "", "comma-separated entry types (soldier, widow, wife, linked_person)")
	fs.StringVar(&rf.filterUnit, "filter-unit", "", "comma-separated units to include")
	fs.StringVar(&rf.filterPS, "filter-pension-state", "", "comma-separated pension states to include")
	fs.StringVar(&rf.filterCHS, "filter-confederate-home-status", "", "comma-separated Confederate Home statuses to include")
	fs.BoolVar(&rf.printer, "printer-friendly", false, "printer-friendly mode")
	fs.BoolVar(&rf.fullBio, "full-biography-page", false, "append full biography appendix")
	fs.StringVar(&rf.out, "out", "", "output PDF path (required)")
	fs.IntVar(&rf.maxPages, "max-pages-per-record", 2, "warn when a single record exceeds this many pages")
	fs.StringVar((*string)(&rf.format), "format", "human", "human or json")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if rf.template == "" {
		return nil, fmt.Errorf("--template is required")
	}
	if rf.out == "" {
		return nil, fmt.Errorf("--out is required")
	}
	if rf.format != formatHuman && rf.format != formatJSON {
		return nil, fmt.Errorf("--format must be human or json (got %q)", rf.format)
	}
	if rf.mode != "record" && rf.mode != "bulk" && rf.mode != "event" && rf.mode != "article" {
		return nil, fmt.Errorf("--mode must be record, bulk, event, or article (got %q)", rf.mode)
	}
	return rf, nil
}

// urlValuesFromFlags translates renderFlags into url.Values for the
// bridge's canonical parser.
func urlValuesFromFlags(rf *renderFlags) url.Values {
	v := url.Values{}
	if rf.scope != "" {
		v.Set("scope", rf.scope)
	}
	if rf.orientation != "" {
		v.Set("orientation", rf.orientation)
	}
	if rf.template != "" {
		v.Set("template", rf.template)
	}
	if rf.sortBy != "" {
		v.Set("sort_by", rf.sortBy)
	}
	if rf.groupByUnit {
		v.Set("group_by_unit", "1")
	}
	if rf.groupByPS {
		v.Set("group_by_pension_state", "1")
	}
	if rf.groupByCHS {
		v.Set("group_by_confederate_home_status", "1")
	}
	if rf.groupByBI {
		v.Set("group_by_buried_in", "1")
	}
	for _, s := range splitCSV(rf.filterBI) {
		v.Add("filter_buried_in", s)
	}
	for _, s := range splitCSV(rf.filterET) {
		v.Add("filter_entry_type", s)
	}
	for _, s := range splitCSV(rf.filterUnit) {
		v.Add("filter_unit", s)
	}
	for _, s := range splitCSV(rf.filterPS) {
		v.Add("filter_pension_state", s)
	}
	for _, s := range splitCSV(rf.filterCHS) {
		v.Add("filter_confederate_home_status", s)
	}
	if rf.printer {
		v.Set("printer_friendly", "1")
	}
	if rf.fullBio {
		v.Set("full_biography_page", "1")
	}
	for _, s := range splitCSV(rf.selectedIDs) {
		v.Add("selected_ids", s)
	}
	return v
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// doRender is the main entry point for `dixiedata-tune render`.
func doRender(args []string, dbPath, typstPath, templatesDir, dataDir string) error {
	rf, err := parseRenderFlags("render", args)
	if err != nil {
		return err
	}
	settings, err := exportbridge.PrintSettingsFromForm(urlValuesFromFlags(rf))
	if err != nil {
		return err
	}

	r, err := openRenderer(dbPath, dataDir, typstPath, templatesDir)
	if err != nil {
		return err
	}
	defer r.Close()

	// SVG output is inferred from --out extension. The tune tool
	// has historically only produced PDFs; adding native SVG
	// output by extension keeps callers (and tests) backwards-
	// compatible while opening the door to web-friendly previews.
	formatExt := strings.ToLower(filepath.Ext(rf.out))
	switch formatExt {
	case ".svg":
		r.SetOutputFormat("svg")
	case ".png":
		r.SetOutputFormat("png")
	default:
		r.SetOutputFormat("pdf")
	}

	// For multi-page SVG/PNG output we need access to pages 2..N
	// after the render returns; the renderer's TYPST_KEEP_WORKDIR
	// hook moves the workdir under our chosen directory instead
	// of cleaning it up.
	workdir := setupSvgWorkdir(formatExt)
	if workdir != "" && strings.TrimSpace(os.Getenv("TYPST_KEEP_WORKDIR")) == workdir {
		defer os.RemoveAll(workdir)
	}

	ctx := context.Background()
	start := time.Now()
	var (
		bytesOut  int64
		recErrors []recordError
		recordIDs []int64
		recordCnt *int
	)

	switch rf.mode {
	case "record":
		if rf.recordID == 0 {
			return fmt.Errorf("--record is required when --mode record")
		}
		soldier, err := r.GetByID(rf.recordID)
		if err != nil {
			return err
		}
		opts := render.PDFOptions{
			Orientation:     rf.orientation,
			PrinterFriendly: rf.printer,
			IncludeImages:   true,
		}
		out, err := createOutFile(rf.out)
		if err != nil {
			return fmt.Errorf("create %s: %w", rf.out, err)
		}
		if err := r.RenderSingle(ctx, *soldier, opts, out); err != nil {
			return err
		}
		recordIDs = []int64{soldier.ID}

	case "event":
		// Issue #358: tune needs to render Event Records via the
		// bridge's RenderEventSingle (which pre-projects linked
		// Person Records via EventService.ListForEvent). The bridge
		// already ships the method (added in issue #374, commit
		// predating #358); this is purely a CLI dispatch gap. The
		// record-type resolution for `event_landscape` lives in
		// exportService.recordTypeForSoldier (entry_type="event"),
		// so this case routes via the same event_<orientation>.typ
		// template the appshell's /events/{id}/pdf handler uses.
		if rf.recordID == 0 {
			return fmt.Errorf("--record is required when --mode event")
		}
		opts := render.PDFOptions{
			Orientation:     rf.orientation,
			PrinterFriendly: rf.printer,
			IncludeImages:   true,
		}
		out, err := createOutFile(rf.out)
		if err != nil {
			return fmt.Errorf("create %s: %w", rf.out, err)
		}
		if err := r.RenderEventSingle(ctx, rf.recordID, opts, out); err != nil {
			return err
		}
		recordIDs = []int64{rf.recordID}

	case "article":
		// Issue #430: Article Records (issue #321) have a
		// template family (article_landscape.typ /
		// article_portrait.typ) and a bridge method
		// (RenderArticleSingle, added when the articles slice
		// landed) but no tune dispatch case. Without this case
		// a user iterating on article_*.typ has no way to
		// render an article via tune. Mirrors the event case
		// shape: --record is the article id, --template picks
		// the template family, --orientation picks the layout.
		if rf.recordID == 0 {
			return fmt.Errorf("--record is required when --mode article")
		}
		opts := render.PDFOptions{
			Orientation:     rf.orientation,
			PrinterFriendly: rf.printer,
			IncludeImages:   false,
		}
		out, err := createOutFile(rf.out)
		if err != nil {
			return fmt.Errorf("create %s: %w", rf.out, err)
		}
		if err := r.RenderArticleSingle(ctx, rf.recordID, opts, out); err != nil {
			return err
		}
		recordIDs = []int64{rf.recordID}

	case "bulk":
		ids := splitCSV(rf.recordIDsRaw)
		if len(ids) > 0 {
			settings.SelectedIDs = nil
			for _, s := range ids {
				n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
				if err != nil {
					return fmt.Errorf("invalid --record-ids value %q: %w", s, err)
				}
				settings.SelectedIDs = append(settings.SelectedIDs, n)
			}
			settings.Scope = render.PrintScopeSelected
		}
		// Issue #68: the bulk path no longer force-clears the
		// per-record Template field. PrintSettings.BulkTemplate
		// is the authoritative override; the bridge no longer
		// rewrites the field. If --template was passed for a bulk
		// render, route it to BulkTemplate so the Registry's
		// bulk-guard sees it.
		if rf.mode == "bulk" {
			settings.BulkTemplate = rf.template
			settings.SingleRecordTemplate = ""
		} else {
			settings.SingleRecordTemplate = rf.template
			settings.BulkTemplate = ""
		}
		f, err := createOutFile(rf.out)
		if err != nil {
			return fmt.Errorf("create %s: %w", rf.out, err)
		}
		errs, err := r.RenderBulk(ctx, settings, f)
		f.Close()
		if err != nil {
			os.Remove(rf.out)
			return err
		}
		for _, e := range errs {
			recErrors = append(recErrors, recordError{
				RecordID:  e.RecordID,
				DisplayID: e.DisplayID,
				Error:     e.Error,
			})
		}
		if len(settings.SelectedIDs) > 0 {
			recordIDs = settings.SelectedIDs
		} else {
			all, _, err := r.List(1, 1<<31-1)
			if err == nil {
				n := len(all)
				recordCnt = &n
			}
		}
	}

	// Multi-page SVG/PNG: copy pages 2..N from the renderer's
	// preserved workdir next to --out. The first page is already
	// in --out (streamed by Render). Page 1 in the workdir is
	// the same content; we only copy pages 2..N as siblings.
	if formatExt == ".svg" || formatExt == ".png" {
		copied, err := copyExtraPages(workdir, rf.out, formatExt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
		fmt.Fprintf(os.Stderr, "%d extra pages copied to %s-N.<ext>\n", copied, strings.TrimSuffix(rf.out, formatExt))
	}

	dur := time.Since(start)
	bytesOut, _ = fileSize(rf.out)

	if rf.format == formatJSON {
		result := renderResult{
			Template:    rf.template,
			RecordIDs:   recordIDs,
			RecordCount: recordCnt,
			OutputPath:  rf.out,
			SizeBytes:   bytesOut,
			DurationMS:  dur.Milliseconds(),
			Errors:      recErrors,
		}
		return writeJSON(os.Stdout, result)
	}

	fmt.Printf("wrote %s (%d bytes) in %dms\n", rf.out, bytesOut, dur.Milliseconds())
	if pages, ok := pdfPageCount(rf.out); ok && pages > rf.maxPages {
		fmt.Fprintf(os.Stderr, "warning: %s is %d pages (--max-pages-per-record=%d); consider shortening content\n",
			rf.out, pages, rf.maxPages)
	}
	for _, e := range recErrors {
		fmt.Fprintf(os.Stderr, "record %d (%s): %s\n", e.RecordID, e.DisplayID, e.Error)
	}
	return nil
}

// createOutFile creates the file at path and returns it as
// io.WriteCloser. Returns the error from os.Create so callers
// can surface a clean `error: ...` line instead of panicking.
// Renamed from `mustCreate` (issue #516 slice B3) so the name
// matches the semantics.
func createOutFile(path string) (io.WriteCloser, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// doAnniversary renders the monthly anniversary report for one
// month. The anniversary template reads `data["month"]` and
// `data["calendar"]` from the AnniversaryService. Required flag:
// --month N (1-12).
func doAnniversary(args []string, dbPath, typstPath, templatesDir, dataDir string) error {
	fs := flag.NewFlagSet("anniversary", flag.ContinueOnError)
	month := fs.Int("month", 0, "month to render (1-12)")
	out := fs.String("out", "", "output PDF path")
	orientation := fs.String("orientation", "P", "page orientation (L or P)")
	printer := fs.Bool("printer-friendly", false, "suppress the 'Made with DixieData' page footer")
	format := fs.String("format", "human", "output format: human or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *month < 1 || *month > 12 {
		return fmt.Errorf("--month must be 1-12 (got %d)", *month)
	}
	if *out == "" {
		return errors.New("--out is required")
	}
	if strings.TrimSpace(dbPath) == "" {
		return errors.New("--db is required")
	}
	r, err := openRenderer(dbPath, dataDir, typstPath, templatesDir)
	if err != nil {
		return err
	}
	defer r.Close()

	// SVG output is inferred from --out extension. See doRender for
	// the same hook; this keeps the tune subcommands aligned.
	formatExt := strings.ToLower(filepath.Ext(*out))
	switch formatExt {
	case ".svg":
		r.SetOutputFormat("svg")
	default:
		r.SetOutputFormat("pdf")
	}

	workdir := setupSvgWorkdir(formatExt)
	if workdir != "" && strings.TrimSpace(os.Getenv("TYPST_KEEP_WORKDIR")) == workdir {
		defer os.RemoveAll(workdir)
	}

	start := time.Now()
	opts := render.PDFOptions{
		Orientation:     *orientation,
		PrinterFriendly: *printer,
	}
	outFile, err := createOutFile(*out)
	if err != nil {
		return fmt.Errorf("create %s: %w", *out, err)
	}
	if err := r.RenderAnniversary(context.Background(), *month, opts, outFile); err != nil {
		os.Remove(*out)
		return err
	}
	if workdir != "" {
		copied, _ := copyExtraPages(workdir, *out, formatExt)
		fmt.Fprintf(os.Stderr, "%d extra pages copied alongside %s\n", copied, *out)
	}
	dur := time.Since(start)
	size, _ := fileSize(*out)
	if *format == "json" {
		return writeJSON(os.Stdout, map[string]any{
			"subcommand":  "anniversary",
			"month":       *month,
			"output_path": *out,
			"size_bytes":  size,
			"duration_ms": dur.Milliseconds(),
		})
	}
	fmt.Printf("wrote %s (%d bytes) in %dms\n", *out, size, dur.Milliseconds())
	return nil
}

// doInsights renders the archive summary / analytics report.
// Uses templates/analytics_summary.typ via the Registry.
func doInsights(args []string, dbPath, typstPath, templatesDir, dataDir string) error {
	fs := flag.NewFlagSet("insights", flag.ContinueOnError)
	out := fs.String("out", "", "output PDF path")
	orientation := fs.String("orientation", "P", "page orientation (L or P)")
	printer := fs.Bool("printer-friendly", false, "suppress the 'Made with DixieData' page footer")
	format := fs.String("format", "human", "output format: human or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("--out is required")
	}
	if strings.TrimSpace(dbPath) == "" {
		return errors.New("--db is required")
	}
	r, err := openRenderer(dbPath, dataDir, typstPath, templatesDir)
	if err != nil {
		return err
	}
	defer r.Close()

	switch strings.ToLower(filepath.Ext(*out)) {
	case ".svg":
		r.SetOutputFormat("svg")
	default:
		r.SetOutputFormat("pdf")
	}

	formatExt := strings.ToLower(filepath.Ext(*out))
	workdir := setupSvgWorkdir(formatExt)
	if workdir != "" && strings.TrimSpace(os.Getenv("TYPST_KEEP_WORKDIR")) == workdir {
		defer os.RemoveAll(workdir)
	}

	start := time.Now()
	opts := render.PDFOptions{
		Orientation:     *orientation,
		PrinterFriendly: *printer,
	}
	outFile, err := createOutFile(*out)
	if err != nil {
		return fmt.Errorf("create %s: %w", *out, err)
	}
	if err := r.RenderInsights(context.Background(), opts, outFile); err != nil {
		os.Remove(*out)
		return err
	}
	if workdir != "" {
		copied, _ := copyExtraPages(workdir, *out, formatExt)
		fmt.Fprintf(os.Stderr, "%d extra pages copied alongside %s\n", copied, *out)
	}
	dur := time.Since(start)
	size, _ := fileSize(*out)
	if *format == "json" {
		return writeJSON(os.Stdout, map[string]any{
			"subcommand":  "insights",
			"output_path": *out,
			"size_bytes":  size,
			"duration_ms": dur.Milliseconds(),
		})
	}
	fmt.Printf("wrote %s (%d bytes) in %dms\n", *out, size, dur.Milliseconds())
	return nil
}

// doWatch re-renders on templates/*.typ mtime change.
func doWatch(args []string, dbPath, typstPath, templatesDir, dataDir string) error {
	rf, err := parseRenderFlags("render", args)
	if err != nil {
		return err
	}

	// Default --record-ids to first 5 records when caller didn't
	// specify. Bulk + no filter would otherwise re-render every
	// record on every keystroke.
	if rf.mode == "bulk" && rf.recordIDsRaw == "" {
		rf.recordIDsRaw = "1,2,3,4,5"
	}

	if err := doRender(args, dbPath, typstPath, templatesDir, dataDir); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "watching %s for changes (Ctrl-C to stop)\n", templatesDir)

	lastMtime := map[string]time.Time{}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for range tick.C {
		changed := false
		entries, err := os.ReadDir(templatesDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "watch error: %v\n", err)
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".typ") {
				continue
			}
			path := filepath.Join(templatesDir, e.Name())
			info, err := e.Info()
			if err != nil {
				continue
			}
			mtime := info.ModTime()
			if prev, ok := lastMtime[path]; !ok || !prev.Equal(mtime) {
				lastMtime[path] = mtime
				if ok {
					changed = true
				}
			}
		}
		if changed {
			if err := doRender(args, dbPath, typstPath, templatesDir, dataDir); err != nil {
				fmt.Fprintf(os.Stderr, "re-render failed: %v\n", err)
			}
		}
	}
	return nil
}

// doDiff compares two existing PDFs by text extraction and page count.
func doDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	before := fs.String("before", "", "path to the 'before' PDF (required)")
	after := fs.String("after", "", "path to the 'after' PDF (required)")
	var format string
	fs.StringVar(&format, "format", "human", "human or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *before == "" || *after == "" {
		return fmt.Errorf("--before and --after are required")
	}

	beforeText, beforePages, err := extractPDF(*before)
	if err != nil {
		return fmt.Errorf("extract before: %w", err)
	}
	afterText, afterPages, err := extractPDF(*after)
	if err != nil {
		return fmt.Errorf("extract after: %w", err)
	}

	diff := diffText(beforeText, afterText)

	if format == string(formatJSON) {
		return writeJSON(os.Stdout, map[string]any{
			"before_path":        *before,
			"after_path":         *after,
			"before_pages":       beforePages,
			"after_pages":        afterPages,
			"page_count_delta":   afterPages - beforePages,
			"text_lines_added":   diff.added,
			"text_lines_removed": diff.removed,
		})
	}

	fmt.Printf("before: %s (%d pages)\n", *before, beforePages)
	fmt.Printf("after:  %s (%d pages, %+d)\n", *after, afterPages, afterPages-beforePages)
	fmt.Printf("text: +%d -%d lines\n", diff.added, diff.removed)
	if len(diff.sample) > 0 {
		fmt.Println("first differences:")
		for _, d := range diff.sample {
			fmt.Printf("  %s\n", d)
		}
	}
	return nil
}

type textDiff struct {
	added   int
	removed int
	sample  []string
}

func diffText(a, b string) textDiff {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	aSet := map[string]bool{}
	for _, l := range aLines {
		aSet[l] = true
	}
	bSet := map[string]bool{}
	for _, l := range bLines {
		bSet[l] = true
	}
	var d textDiff
	for l := range bSet {
		if !aSet[l] {
			d.added++
			if len(d.sample) < 5 {
				d.sample = append(d.sample, "+ "+truncate(l, 120))
			}
		}
	}
	for l := range aSet {
		if !bSet[l] {
			d.removed++
			if len(d.sample) < 5 {
				d.sample = append(d.sample, "- "+truncate(l, 120))
			}
		}
	}
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// doListTemplates lists discovered typst templates.
func doListTemplates(typstPath, templatesDir string) error {
	typst := render.NewTypstRenderer(typstPath, filepath.Dir(templatesDir))
	templates, err := typst.ListTemplates()
	if err != nil {
		return err
	}
	if len(templates) == 0 {
		fmt.Printf("no templates found in %s\n", templatesDir)
		return nil
	}
	sort.Slice(templates, func(i, j int) bool { return templates[i].Name < templates[j].Name })
	for _, t := range templates {
		fmt.Printf("%s\t%s\t%s\n", t.Name, t.Engine, t.Description)
	}
	return nil
}

// doListRecords lists records in --db.
func doListRecords(dbPath, dataDir string, args []string) error {
	if dbPath == "" {
		return fmt.Errorf("--db is required (or DIXIEDATA_DB env)")
	}
	fs := flag.NewFlagSet("list-records", flag.ContinueOnError)
	kind := fs.String("kind", "soldier", "record kind: soldier, article, or event (issue #518 adds event)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := openRenderer(dbPath, dataDir, "", "")
	if err != nil {
		return err
	}
	defer r.Close()

	switch *kind {
	case "soldier", "soldiers":
		return listSoldiers(r)
	case "article", "articles":
		return listArticles(r)
	case "event", "events":
		return listEvents(r)
	default:
		return fmt.Errorf("--kind must be soldier, article, or event, got %q", *kind)
	}
}

// listSoldiers prints one tab-separated line per Person Record:
// id, display_id, name. Uses BulkRenderer.ListPeople (which
// filters entry_type to soldier/wife/widow/linked_person) so
// events and articles that share the soldiers table don't leak
// in (issue #518 slice C2). Paginated to the actual end so
// archives >2500 records render the full list (slice C3).
func listSoldiers(r *exportbridge.BulkRenderer) error {
	page := 1
	const pageSize = 50
	total := 0
	for {
		batch, count, err := r.ListPeople(page, pageSize)
		if err != nil {
			return err
		}
		total = count
		for _, s := range batch {
			fmt.Printf("%d\t%s\t%s\n", s.ID, s.DisplayID, nameOf(s))
		}
		if len(batch) < pageSize {
			break
		}
		page++
	}
	fmt.Fprintf(os.Stderr, "total: %d records\n", total)
	return nil
}

// listEvents prints one tab-separated line per Event Record
// (entry_type='event' in the soldiers table): id, display_id,
// kind. Used by issue #518 slice C1 so a user iterating on
// event_*.typ templates can find an event id without writing SQL.
func listEvents(r *exportbridge.BulkRenderer) error {
	page := 1
	const pageSize = 50
	total := 0
	for {
		batch, count, err := r.ListEvents(page, pageSize)
		if err != nil {
			return err
		}
		total = count
		for _, e := range batch {
			fmt.Printf("%d\t%s\t%s\n", e.ID, e.DisplayID, e.Kind)
		}
		if len(batch) < pageSize {
			break
		}
		page++
	}
	fmt.Fprintf(os.Stderr, "total: %d events\n", total)
	return nil
}

// listArticles prints one tab-separated line per Article:
// id, display_id, title. Used by issue #430 so a user iterating
// on article_*.typ can find an article id without writing SQL.
func listArticles(r *exportbridge.BulkRenderer) error {
	page := 1
	const pageSize = 50
	total := 0
	for {
		batch, count, err := r.ListArticles(page, pageSize)
		if err != nil {
			return err
		}
		total = count
		for _, a := range batch {
			title := strings.TrimSpace(a.Title)
			if title == "" {
				title = "(untitled)"
			}
			fmt.Printf("%d\t%s\t%s\n", a.ID, a.DisplayID, title)
		}
		if len(batch) < pageSize {
			break
		}
		page++
	}
	fmt.Fprintf(os.Stderr, "total: %d articles\n", total)
	return nil
}

// nameOf returns a printable name for a soldier record.
func nameOf(s models.Soldier) string {
	first := strings.TrimSpace(s.FirstName)
	last := strings.TrimSpace(s.LastName)
	if last != "" {
		if first != "" {
			return last + ", " + first
		}
		return last
	}
	if first != "" {
		return first
	}
	return strings.TrimSpace(s.DisplayID)
}

// checkResult is one row of the doctor output. Defined at
// package scope (not local to doDoctor) so countFailed can take
// the slice as a parameter.
type checkResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// doDoctor is the preflight gate (issue #515 slice D3). Runs
// five checks in sequence and prints pass/fail per check:
//
//  1. typst binary present + version (findTypstBinary + --version probe)
//  2. templates dir resolves + contains at least one *_landscape.typ
//  3. --db opens + has at least one record (or an explicit empty archive)
//  4. seed-data fixture present at .scratch/tune-fixture/ (for snapshot tests)
//  5. snapshot suites green (go test -short ./internal/exportcontract/ ./tools/tune/...)
//
// Exits 1 if any check fails. JSON output via --format json for
// CI / audit scripts (DIXIEDATA_TUNE_FORMAT=json still works for
// parity with the other subcommands).
func doDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	format := fs.String("format", "human", "human or json")
	quick := fs.Bool("quick", false, "skip the snapshot test invocation (file-presence only)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var results []checkResult
	allPassed := true
	add := func(name string, passed bool, msg string) {
		if !passed {
			allPassed = false
		}
		results = append(results, checkResult{name, passed, msg})
	}

	// 1. typst binary + version.
	if abs, err := findTypstBinary(); err != nil {
		add("typst binary", false, err.Error())
	} else if v, ok := probeTypstVersion(abs); !ok {
		add("typst binary", false, fmt.Sprintf("%s found but --version probe failed", abs))
	} else {
		add("typst binary", true, fmt.Sprintf("%s (%s)", abs, v))
	}

	// 2. templates dir.
	if tdir, err := findTemplatesDir(); err != nil {
		add("templates dir", false, err.Error())
	} else {
		entries, _ := os.ReadDir(tdir)
		landscapes := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), "_landscape.typ") {
				landscapes++
			}
		}
		if landscapes == 0 {
			add("templates dir", false, fmt.Sprintf("%s has no *_landscape.typ", tdir))
		} else {
			add("templates dir", true, fmt.Sprintf("%s (%d landscape templates)", tdir, landscapes))
		}
	}

	// 3. --db opens + has records. Skip silently if --db not set
	//    (doctor is useful pre-archive too).
	if dbPath := strings.TrimSpace(os.Getenv("DIXIEDATA_DB")); dbPath != "" || flagWasSet("db") {
		_ = dbPath // handled by caller via flag lookup below
	}
	// Re-derive dbPath from the same walker the global flags used:
	// for simplicity, check $DIXIEDATA_DB directly.
	if dbEnv := strings.TrimSpace(os.Getenv("DIXIEDATA_DB")); dbEnv != "" {
		if _, err := os.Stat(filepath.Join(dbEnv, "dixiedata.db")); err != nil {
			add("--db archive", false, fmt.Sprintf("no dixiedata.db at %s", filepath.Join(dbEnv, "dixiedata.db")))
		} else {
			add("--db archive", true, dbEnv)
		}
	} else {
		add("--db archive", true, "no DIXIEDATA_DB set; skip (set it if you want a record-count check)")
	}

	// 4. seed-data fixture.
	if fixture, ok := findSeedFixtureHint(); ok {
		add("seed fixture", true, fixture)
	} else {
		add("seed fixture", false, "no .scratch/tune-fixture/ found; run `make debug` to create")
	}

	// 5. snapshot suites green (or quick mode: file-presence only).
	if *quick {
		missing := missingSnapshotFiles()
		if len(missing) > 0 {
			add("snapshots present", false, fmt.Sprintf("%d missing snapshot files", len(missing)))
		} else {
			add("snapshots present", true, "all snapshot files present (run without --quick to verify green)")
		}
	} else {
		msg, ok := probeSnapshotsGreen()
		if ok {
			add("snapshots green", true, msg)
		} else {
			add("snapshots green", false, msg)
		}
	}

	payload := map[string]any{
		"checks":  results,
		"passed":  allPassed,
	}
	if *format == "json" {
		if !allPassed {
			// Still print JSON but the caller exits 1 via the
			// returned error so CI can distinguish pass/fail.
			_ = writeJSON(os.Stdout, payload)
			return fmt.Errorf("doctor: %d check(s) failed", countFailed(results))
		}
		return writeJSON(os.Stdout, payload)
	}

	for _, r := range results {
		mark := "ok  "
		if !r.Passed {
			mark = "FAIL"
		}
		fmt.Printf("[%s] %-22s %s\n", mark, r.Name, r.Message)
	}
	fmt.Println()
	if allPassed {
		fmt.Println("doctor: all checks passed")
		return nil
	}
	return fmt.Errorf("doctor: %d check(s) failed", countFailed(results))
}

// flagWasSet is a thin wrapper that uses the os.Args slice to
// detect whether a global flag was passed by the user. We can't
// easily introspect flag.FlagSet from here; this is a heuristic
// that scans the raw argv for the flag name. Used only for the
// doctor subcommand's optional --db archive check.
func flagWasSet(name string) bool {
	prefix := "--" + name + "="
	for _, a := range os.Args[1:] {
		if a == "--"+name || strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

// findSeedFixtureHint returns the seed-fixture path when one
// exists (either the canonical .scratch/tune-fixture/ in the
// repo root or the TUNE_FIXTURE env override). Best-effort
// walker — does not error on miss; returns ok=false.
func findSeedFixtureHint() (string, bool) {
	if env := strings.TrimSpace(os.Getenv("TUNE_FIXTURE")); env != "" {
		if _, err := os.Stat(filepath.Join(env, "dixiedata.db")); err == nil {
			return env, true
		}
		return env, false
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, ".scratch", "tune-fixture", "dixiedata.db")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, ".scratch", "tune-fixture"), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// missingSnapshotFiles returns the names of snapshot files that
// should exist but don't. Quick-mode sanity check.
func missingSnapshotFiles() []string {
	required := []string{
		"internal/exportcontract/testdata/snapshots/soldier-landscape.pdf",
		"internal/exportcontract/testdata/snapshots-cli/soldier-landscape.pdf",
		"tools/tune/testdata/soldier1-landscape.pdf",
	}
	var missing []string
	dir, err := os.Getwd()
	if err != nil {
		return required
	}
	for i := 0; i < 6; i++ {
		for _, rel := range required {
			if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
				missing = append(missing, rel)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		break
	}
	return missing
}

// probeSnapshotsGreen shells out to `go test -short -count=1` on
// the snapshot suites with a 300-second timeout. tools/tune is
// a separate Go module (its own go.mod), so we must invoke
// `go test` once from the repo root (for internal/exportcontract)
// and once from tools/tune/ (for the tune package). Returns the
// trimmed combined output as the message + ok=true on success.
// ok=false on failure or timeout.
func probeSnapshotsGreen() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "repo root not found", false
		}
		dir = parent
	}
	var msgs []string
	// Suite 1: internal/exportcontract (root module).
	ctx1, cancel1 := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel1()
	cmd1 := exec.CommandContext(ctx1, "go", "test", "-short", "-count=1",
		"./internal/exportcontract/")
	cmd1.Dir = dir
	out1, err1 := cmd1.CombinedOutput()
	if err1 != nil {
		return strings.TrimSpace(string(out1)), false
	}
	msgs = append(msgs, "internal/exportcontract ok")
	// Suite 2: tools/tune (separate module, run from its own dir).
	tuneDir := filepath.Join(dir, "tools", "tune")
	if _, err := os.Stat(filepath.Join(tuneDir, "go.mod")); err == nil {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel2()
		cmd2 := exec.CommandContext(ctx2, "go", "test", "-short", "-count=1", "./...")
		cmd2.Dir = tuneDir
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return strings.TrimSpace(string(out2)), false
		}
		msgs = append(msgs, "tools/tune ok")
	}
	return strings.Join(msgs, "; "), true
}

// countFailed returns the number of check results that did not pass.
func countFailed(results []checkResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}

// doPrintDefaults prints the appshell's default flag set.
func doPrintDefaults(args []string) error {
	fs := flag.NewFlagSet("print-defaults", flag.ContinueOnError)
	mode := fs.String("mode", "bulk", "bulk or record")
	var format string
	fs.StringVar(&format, "format", "human", "human or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	template := "bulk_soldier"
	if *mode == "record" {
		template = "soldier_landscape"
	}
	rf := &renderFlags{
		template:    template,
		mode:        *mode,
		orientation: "L",
		sortBy:      "last_name",
		scope:       "all",
	}
	settings, err := exportbridge.PrintSettingsFromForm(urlValuesFromFlags(rf))
	if err != nil {
		return err
	}

	if format == string(formatJSON) {
		return writeJSON(os.Stdout, map[string]any{
			"mode":           *mode,
			"template":       template,
			"orientation":    "L",
			"print_settings": settings,
		})
	}

	fmt.Printf("# Defaults for dixiedata-tune render --mode %s\n", *mode)
	fmt.Printf("# (copy-paste these flags after `render --mode %s`)\n", *mode)
	fmt.Printf("--template %s\n", template)
	fmt.Printf("--orientation L\n")
	fmt.Printf("--sort-by last_name\n")
	fmt.Printf("--scope all\n")
	return nil
}

// openRenderer wires up a BulkRenderer with the typst registry.
func openRenderer(dbPath, dataDir, typstPath, templatesDir string) (*exportbridge.BulkRenderer, error) {
	if dbPath == "" {
		return nil, errors.New("--db is required")
	}
	// Strict-db guard (issue #516 slice B2): refuse to open a
	// missing db rather than letting db.Open MkdirAll + create
	// a fresh empty db at the given path. The previous behavior
	// silently produced a phantom `.dixiedata/dixiedata.db`
	// anywhere in the tree that happened to not have one when
	// the user ran `--db .dixiedata` from a non-repo-root CWD,
	// then returned `total: 0 records` with no warning. Resolve
	// the db file path the way db.Open would and bail with a
	// clear error if it doesn't exist. --db-create opts in to
	// the legacy behavior for callers that genuinely want to
	// bootstrap an empty archive.
	if err := requireExistingDB(dbPath); err != nil {
		return nil, err
	}
	r, err := exportbridge.NewBulkRenderer(dbPath, dataDir)
	if err != nil {
		return nil, fmt.Errorf("new renderer: %w", err)
	}
	if typstPath != "" && templatesDir != "" {
		typst := render.NewTypstRenderer(typstPath, filepath.Dir(templatesDir))
		reg := render.NewRegistry(typst, templatesDir)
		r.SetRegistry(reg)
	}
	return r, nil
}

// requireExistingDB checks whether dbPath points at an existing
// DixieData database. dbPath may be either the data directory
// (the one containing dixiedata.db — the documented --db shape)
// or the database file directly. Returns a clear error if the
// resolved database file does not exist so the caller fails
// fast instead of letting db.Open MkdirAll + create a phantom
// empty db (issue #516 slice B2). Opt out via
// DIXIEDATA_TUNE_DB_CREATE=1 to restore the legacy
// auto-create-empty-db behavior for callers that need it
// (e.g. seed-data bootstrap flows).
func requireExistingDB(dbPath string) error {
	if os.Getenv("DIXIEDATA_TUNE_DB_CREATE") == "1" {
		return nil
	}
	resolved := dbPath
	if info, err := os.Stat(dbPath); err == nil && info.IsDir() {
		resolved = filepath.Join(dbPath, "dixiedata.db")
	}
	if _, err := os.Stat(resolved); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no dixiedata.db found at %s (did you mean to pass --db .dixiedata? set DIXIEDATA_TUNE_DB_CREATE=1 to auto-create an empty db)", resolved)
		}
		return fmt.Errorf("stat db %s: %w", resolved, err)
	}
	return nil
}

// fileSize returns the size of the file at path.
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// printVersion prints the tune binary version, the typst binary
// version (best-effort; falls back to 'unknown' if --typst hasn't
// been resolved or the binary isn't in PATH), and the bridge
// module version. Set DIXIEDATA_TUNE_JSON=1 for JSON output for
// CI / audit scripts (issue #515 slice D2).
func printVersion() error {
	typstVersion := "unknown"
	if typst := strings.TrimSpace(os.Getenv("DIXIEDATA_TUNE_TYPST")); typst != "" {
		if v, ok := probeTypstVersion(typst); ok {
			typstVersion = v
		}
	}
	// Default typst binary walker: try the same path findTypstBinary
	// would resolve so --version works without explicit --typst.
	if typstVersion == "unknown" {
		if abs, err := findTypstBinary(); err == nil {
			if v, ok := probeTypstVersion(abs); ok {
				typstVersion = v
			}
		}
	}
	payload := map[string]string{
		"tune":   Version,
		"typst":  typstVersion,
		"bridge": exportbridge.Version,
	}
	if os.Getenv("DIXIEDATA_TUNE_JSON") == "1" {
		return writeJSON(os.Stdout, payload)
	}
	fmt.Printf("dixiedata-tune %s\n", Version)
	fmt.Printf("  typst:  %s\n", typstVersion)
	fmt.Printf("  bridge: %s\n", exportbridge.Version)
	return nil
}

// probeTypstVersion shells out to the given typst binary with
// --version and returns the trimmed stdout. Returns ("", false)
// when the binary is missing, fails to start, or prints
// something unexpected.
func probeTypstVersion(binPath string) (string, bool) {
	out, err := exec.Command(binPath, "--version").Output()
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", false
	}
	return v, true
}

// pdfPageCount returns the page count of a PDF using pdfinfo.
// Returns (0, false) when pdfinfo is unavailable or fails.
func pdfPageCount(path string) (int, bool) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return 0, false
	}
	out, err := exec.Command("pdfinfo", path).Output()
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				n, err := strconv.Atoi(fields[1])
				if err == nil {
					return n, true
				}
			}
		}
	}
	return 0, false
}

// extractPDF returns the text content and page count of a PDF.
// Uses pdftotext if available.
func extractPDF(path string) (string, int, error) {
	if _, err := exec.LookPath("pdftotext"); err == nil {
		out, err := exec.Command("pdftotext", path, "-").Output()
		if err != nil {
			return "", 0, err
		}
		pages, _ := pdfPageCount(path)
		return string(out), pages, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		return "", 0, fmt.Errorf("not a PDF: %s", path)
	}
	return string(data), 0, nil
}

// writeJSON marshals v as indented JSON to w.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// setupSvgWorkdir prepares the renderer's TYPST_KEEP_WORKDIR hook
// for SVG/PNG output so the caller can access pages 2..N after
// the render returns. Returns the empty string when the output
// is PDF (no workdir needed) or the caller already supplied a
// keep-workdir via env.
//
// Limitation (issue #516 slice B4): the renderer reads
// TYPST_KEEP_WORKDIR from process env at fork time
// (pkg/render/renderers.go:132), so this helper MUST set the
// env var to communicate the keep directory to the renderer.
// There is currently no per-call API on the renderer; the env
// mutation is the contract. Implications:
//
//   - The env mutation is process-global. tune is a single-shot
//     CLI today so this is harmless, but a future batch-mode or
//     library-use path must either accept the global state or
//     wait for a renderer API that takes the keep dir as an
//     argument (deferred — not worth the API churn yet).
//   - The mutation is scoped: only set when SVG/PNG output is
//     requested AND the caller has not already set the var. A
//     caller-supplied value is respected verbatim (the if-existing
//     branch below).
//   - Return value is the source of truth for cleanup; copyExtraPages
//     consumes it. We do NOT rely on the env var's value being
//     equal to our return — a caller-supplied env var may point
//     elsewhere, and copyExtraPages walks whatever path we return.
//
// If the renderer grows a SetKeepWorkdir(path string) method
// (or equivalent), switch this helper to use it and drop the
// os.Setenv side-effect.
func setupSvgWorkdir(formatExt string) string {
	if formatExt != ".svg" && formatExt != ".png" {
		return ""
	}
	if existing := strings.TrimSpace(os.Getenv("TYPST_KEEP_WORKDIR")); existing != "" {
		return existing
	}
	workdir, err := os.MkdirTemp("", "dixiedata-tune-svg-")
	if err != nil {
		return ""
	}
	os.Setenv("TYPST_KEEP_WORKDIR", workdir)
	return workdir
}

// copyExtraPages walks the renderer-preserved workdir for SVG/PNG
// outputs and copies pages 2..N next to the user-requested --out.
// typst emits files named out-1.<ext>, out-2.<ext>, ... when the
// output path uses the {p} page-template. Page 1 is already at
// --out (streamed by Render) so we skip it; the rest land as
// {--out-stem}-{p}.{ext} siblings so callers can preview or
// distribute them individually.
//
// Returns the number of extra pages copied. Empty workdir or a
// missing workdir is not an error: it just means the render was
// single-page and --out is the only artifact.
func copyExtraPages(workdir, outPath, ext string) (int, error) {
	if workdir == "" {
		return 0, nil
	}
	outerEntries, err := os.ReadDir(workdir)
	if err != nil {
		return 0, nil // workdir missing: nothing to do
	}
	// Find the renamed workdir (random basename from MkdirTemp)
	// the renderer uses. TYPST_KEEP_WORKDIR is the parent; the
	// inner basename is generated by the renderer, not us.
	var inner string
	for _, e := range outerEntries {
		if e.IsDir() {
			inner = filepath.Join(workdir, e.Name())
			break
		}
	}
	if inner == "" {
		return 0, nil
	}
	// Now iterate the inner workdir: typst writes out-{p}.<ext>
	// files there when the output path uses the {p} template.
	innerEntries, err := os.ReadDir(inner)
	if err != nil {
		return 0, nil
	}
	stem := strings.TrimSuffix(outPath, ext)
	prefix := "out-"
	suffix := ext
	copied := 0
	for _, e := range innerEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		pageStr := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
		// Skip page 1 (rendered as "out-1.ext"; the same content
		// is already streamed to --out by Render).
		if pageStr == "1" {
			continue
		}
		src := filepath.Join(inner, name)
		dst := fmt.Sprintf("%s-%s%s", stem, pageStr, suffix)
		data, err := os.ReadFile(src)
		if err != nil {
			return copied, fmt.Errorf("read %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return copied, fmt.Errorf("write %s: %w", dst, err)
		}
		copied++
	}
	return copied, nil
}