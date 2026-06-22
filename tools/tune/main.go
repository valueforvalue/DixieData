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

func run(args []string) error {
	if len(args) == 0 {
		return usage(nil)
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
		"list-templates": true, "list-records": true,
		"print-defaults": true, "help": true, "-h": true, "--help": true,
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
		*dataDir = filepath.Dir(*dbPath)
	}

	switch sub {
	case "render":
		return doRender(subArgs, *dbPath, *typstPath, *templatesDir, *dataDir)
	case "watch":
		return doWatch(subArgs, *dbPath, *typstPath, *templatesDir, *dataDir)
	case "diff":
		return doDiff(subArgs)
	case "list-templates":
		return doListTemplates(*typstPath, *templatesDir)
	case "list-records":
		return doListRecords(*dbPath, *dataDir)
	case "print-defaults":
		return doPrintDefaults(subArgs)
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
// soldier_landscape.typ inside.
func findTemplatesDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "templates")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(candidate, "soldier_landscape.typ")); err == nil {
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
	return "", fmt.Errorf("no templates/ directory with soldier_landscape.typ found; pass --templates")
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
	if rf.mode != "record" && rf.mode != "bulk" {
		return nil, fmt.Errorf("--mode must be record or bulk (got %q)", rf.mode)
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
		if err := r.RenderSingle(ctx, *soldier, opts, mustCreate(rf.out)); err != nil {
			return err
		}
		recordIDs = []int64{soldier.ID}

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
		f := mustCreate(rf.out)
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

// mustCreate creates the file at path and returns it as io.WriteCloser.
// Errors on create failure.
func mustCreate(path string) io.WriteCloser {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	return f
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
func doListRecords(dbPath, dataDir string) error {
	if dbPath == "" {
		return fmt.Errorf("--db is required (or DIXIEDATA_DB env)")
	}
	r, err := openRenderer(dbPath, dataDir, "", "")
	if err != nil {
		return err
	}
	defer r.Close()

	page := 1
	const pageSize = 50
	total := 0
	for {
		batch, count, err := r.List(page, pageSize)
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
		if page > 50 {
			break
		}
	}
	fmt.Fprintf(os.Stderr, "total: %d records\n", total)
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

// fileSize returns the size of the file at path.
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
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