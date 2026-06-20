// Command dixiedata-tune is a developer tool for iterating on the
// Typst-based PDF export templates. It opens a DixieData SQLite in
// read-only mode, runs the existing fpdf exports to capture a
// baseline, and renders the same record through a Typst template so
// the researcher can see them side by side.
//
// Usage:
//
//	dixiedata-tune --db <path-to-dixiedata> render --template <name> --record <id> --out <pdf>
//	dixiedata-tune --db <path-to-dixiedata> capture-baseline
//	dixiedata-tune --db <path-to-dixiedata> compare --template <name> --record <id>
//	dixiedata-tune --db <path-to-dixiedata> list-templates
//	dixiedata-tune --db <path-to-dixiedata> list-records
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/pkg/dixiedata"
	"github.com/valueforvalue/DixieData/pkg/encode"
	"github.com/valueforvalue/DixieData/pkg/render"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	knownSubs := map[string]bool{
		"render": true, "list-templates": true, "list-records": true,
		"capture-baseline": true, "compare": true, "help": true, "-h": true, "--help": true,
	}
	if len(os.Args) < 2 {
		return usage()
	}
	// Find the subcommand positional in os.Args. Global flags
	// (-db, -typst, -templates) can appear before it.
	sub := ""
	subIdx := 1
	for i := 1; i < len(os.Args); i++ {
		if !strings.HasPrefix(os.Args[i], "-") {
			sub = os.Args[i]
			subIdx = i
			break
		}
		if !strings.Contains(os.Args[i], "=") && i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
			i++
		}
	}
	if sub == "" {
		return usage()
	}
	if !knownSubs[sub] {
		return fmt.Errorf("unknown subcommand %q", sub)
	}
	// Split the args: everything from position 1 up to (but not
	// including) the subcommand is global args. Everything from
	// subIdx+1 to the end is subcommand args.
	globalArgs := os.Args[1:subIdx]
	subArgs := os.Args[subIdx+1:]

	// Parse only the global --db flag. The subcommand gets its own
	// FlagSet and parses the rest.
	fsGlobal := flag.NewFlagSet("global", flag.ContinueOnError)
	dbFlag := fsGlobal.String("db", os.Getenv("DIXIEDATA_DB"), "Path to the DixieData SQLite file (or DIXIEDATA_DB env)")
	binFlag := fsGlobal.String("typst", defaultTypstBinary(), "Path to the typst binary")
	templateDir := fsGlobal.String("templates", "", "Path to the templates directory (default: <repo>/templates)")
	if err := fsGlobal.Parse(extractGlobalArgs(globalArgs)); err != nil {
		return err
	}
	args := subArgs

	if *templateDir == "" {
		*templateDir = findTemplatesDir()
	}

	if sub == "list-templates" {
		return doListTemplates(*binFlag, *templateDir)
	}

	if *dbFlag == "" {
		return fmt.Errorf("--db is required (or set DIXIEDATA_DB)")
	}

	archive, err := dixiedata.Open(*dbFlag)
	if err != nil {
		return err
	}
	defer archive.Close()

	ctx := context.Background()
	switch sub {
	case "render":
		return doRender(ctx, args, archive, *binFlag, *templateDir)
	case "list-records":
		return doListRecords(archive)
	case "capture-baseline":
		return doCaptureBaseline(archive)
	case "compare":
		return doCompare(ctx, args, archive, *binFlag, *templateDir)
	case "help", "-h", "--help":
		return usage()
	default:
		return fmt.Errorf("unknown subcommand %q", sub)
	}
}

// extractGlobalArgs returns the leading sequence of args that look
// like flags for the global FlagSet. Stops at the first positional
// arg (the subcommand) so subcommand flags aren't consumed by the
// global parser.
func extractGlobalArgs(args []string) []string {
	knownFlags := map[string]bool{
		"-db": true, "--db": true,
		"-typst": true, "--typst": true,
		"-templates": true, "--templates": true,
	}
	out := []string{}
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return out
		}
		if !knownFlags[a] {
			return out
		}
		out = append(out, a)
		i++
		if !strings.Contains(a, "=") && i < len(args) && !strings.HasPrefix(args[i], "-") {
			if strings.ContainsAny(args[i], "/\\") || strings.Contains(args[i], ".") {
				out = append(out, args[i])
				i++
			} else {
				return out
			}
		}
	}
	return out
}

// usage prints a short help message.
func usage() error {
	fmt.Fprintln(os.Stderr, "usage: dixiedata-tune [global flags] <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "global flags:")
	fmt.Fprintln(os.Stderr, "  --db PATH          path to the DixieData SQLite (or DIXIEDATA_DB env)")
	fmt.Fprintln(os.Stderr, "  --typst PATH       path to the typst binary (default: <repo>/bin/typst-*)")
	fmt.Fprintln(os.Stderr, "  --templates PATH   path to the templates directory (default: <repo>/templates)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "subcommands:")
	fmt.Fprintln(os.Stderr, "  render            render a template against a record")
	fmt.Fprintln(os.Stderr, "  list-templates    list discovered .typ templates")
	fmt.Fprintln(os.Stderr, "  list-records      list records in the DixieData SQLite")
	fmt.Fprintln(os.Stderr, "  capture-baseline  render every record through fpdf; save to baseline/")
	fmt.Fprintln(os.Stderr, "  compare           render the same record through fpdf and Typst; save both")
	return nil
}

// defaultTypstBinary returns the platform-specific Typst binary in
// <repo>/bin/. The tool is run from the repo root or any subdirectory;
// candidates list the most likely relative paths.
func defaultTypstBinary() string {
	candidates := []string{
		"bin/typst-windows.exe",
		"bin/typst-macos",
		"bin/typst-linux",
		"../bin/typst-windows.exe",
		"../bin/typst-macos",
		"../bin/typst-linux",
		"../../bin/typst-windows.exe",
		"../../bin/typst-macos",
		"../../bin/typst-linux",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "typst"
}

// findTemplatesDir walks up from the current working directory to
// find a templates/ directory.
func findTemplatesDir() string {
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "templates")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "templates"
}

// doRender renders a template against a single record. Routes to
// the FpdfRenderer for "fpdf:*" template names (the fpdf baseline
// path); routes to the TypstRenderer for everything else.
func doRender(ctx context.Context, args []string, archive *dixiedata.LocalArchive, binPath, templateDir string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	templateName := fs.String("template", "", "Template name (e.g. soldier_landscape, or fpdf:soldier for the baseline)")
	recordID := fs.Int64("record", 0, "Record ID to render")
	outPath := fs.String("out", "", "Output PDF path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *templateName == "" {
		return fmt.Errorf("--template is required")
	}
	if *recordID == 0 {
		return fmt.Errorf("--record is required")
	}
	if *outPath == "" {
		return fmt.Errorf("--out is required")
	}

	adapter := dixiedataToFpdfAdapter{archive}
	soldier, err := archive.GetByID(*recordID)
	if err != nil {
		return fmt.Errorf("get record: %w", err)
	}
	identity, err := archive.UserIdentity()
	if err != nil {
		return fmt.Errorf("get identity: %w", err)
	}
	data := map[string]any{
		"soldier":  *soldier,
		"options":  render.PDFOptions{Orientation: "L"},
		"branding": encode.BrandingFromIdentity(identity),
	}

	out, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	if strings.HasPrefix(*templateName, "fpdf:") {
		// fpdf baseline path. The fpdfRenderer writes through a temp
		// file because fpdf requires a real path; the file is then
		// copied to the writer.
		fpdfRenderer := render.NewFpdfRenderer(render.New(adapter, adapter))
		tpl := render.Template{Name: *templateName, Engine: "fpdf"}
		if err := fpdfRenderer.Render(ctx, tpl, data, out); err != nil {
			return fmt.Errorf("render: %w", err)
		}
	} else {
		// Typst path. Build the full TemplateData payload and run
		// through the TypstRenderer. Match the fpdf path's default
		// of landscape ("L") so the output page size matches.
		typstRenderer := render.NewTypstRenderer(binPath, filepath.Dir(templateDir))
		fullData := encode.NewTemplateDataForSoldier(*soldier, render.PDFOptions{Orientation: "L"}, encode.BrandingFromIdentity(identity))
		tpl := render.Template{Name: *templateName, Path: filepath.Join(templateDir, *templateName+".typ"), Engine: "typst"}
		payload := templateDataToMap(fullData)
		if err := typstRenderer.Render(ctx, tpl, payload, out); err != nil {
			return fmt.Errorf("render: %w", err)
		}
	}

	st, _ := out.Stat()
	fmt.Printf("wrote %s (%d bytes)\n", *outPath, st.Size())
	return nil
}

// doListTemplates prints the discovered .typ templates.
func doListTemplates(binPath, templateDir string) error {
	typstRenderer := render.NewTypstRenderer(binPath, filepath.Dir(templateDir))
	templates, err := typstRenderer.ListTemplates()
	if err != nil {
		return err
	}
	if len(templates) == 0 {
		fmt.Println("no templates found in", templateDir)
		return nil
	}
	for _, t := range templates {
		fmt.Printf("%s\t%s\t%s\n", t.Name, t.Engine, t.Description)
	}
	return nil
}

// doListRecords prints a paginated list of records.
func doListRecords(archive *dixiedata.LocalArchive) error {
	page := 1
	total := 0
	for {
		batch, count, err := archive.List(page, 50)
		if err != nil {
			return err
		}
		total = count
		if len(batch) == 0 {
			break
		}
		for _, s := range batch {
			fmt.Printf("%d\t%s\t%s\n", s.ID, s.DisplayID, nameOf(s))
		}
		page++
		if page > 50 {
			break
		}
	}
	fmt.Fprintf(os.Stderr, "total: %d records\n", total)
	return nil
}

// doCaptureBaseline renders every record through the fpdf service and
// saves the PDFs to baseline/soldier/<record-id>.pdf.
func doCaptureBaseline(archive *dixiedata.LocalArchive) error {
	identity, err := archive.UserIdentity()
	if err != nil {
		return fmt.Errorf("get identity: %w", err)
	}
	adapter := dixiedataToFpdfAdapter{archive}
	fpdfRenderer := render.NewFpdfRenderer(render.New(adapter, adapter))

	baselineDir := filepath.Join("baseline", "soldier")
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		return err
	}

	page := 1
	count := 0
	for {
		batch, total, err := archive.List(page, 50)
		if err != nil {
			return err
		}
		count = total
		if len(batch) == 0 {
			break
		}
		for _, s := range batch {
			enriched, err := archive.GetByID(s.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "skip %d: %v\n", s.ID, err)
				continue
			}
			outPath := filepath.Join(baselineDir, fmt.Sprintf("%d.pdf", enriched.ID))
			tpl := render.Template{Name: "fpdf:soldier", Engine: "fpdf"}
			out, err := os.Create(outPath)
			if err != nil {
				return err
			}
			if err := fpdfRenderer.Render(context.Background(), tpl, map[string]any{
				"soldier":  *enriched,
				"options":  render.PDFOptions{Orientation: "L"},
				"branding": encode.BrandingFromIdentity(identity),
			}, out); err != nil {
				out.Close()
				return fmt.Errorf("render %d: %w", enriched.ID, err)
			}
			out.Close()
			fmt.Printf("baseline %d -> %s\n", enriched.ID, outPath)
		}
		page++
		if page > 200 {
			break
		}
	}
	fmt.Fprintf(os.Stderr, "captured %d baselines to %s/\n", count, baselineDir)
	return nil
}

// doCompare renders the same record through both fpdf and Typst and
// saves both PDFs side-by-side.
func doCompare(ctx context.Context, args []string, archive *dixiedata.LocalArchive, binPath, templateDir string) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	templateName := fs.String("template", "", "Typst template name")
	recordID := fs.Int64("record", 0, "Record ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *templateName == "" {
		return fmt.Errorf("--template is required")
	}
	if *recordID == 0 {
		return fmt.Errorf("--record is required")
	}

	identity, err := archive.UserIdentity()
	if err != nil {
		return err
	}
	enriched, err := archive.GetByID(*recordID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll("compare", 0o755); err != nil {
		return err
	}

	adapter := dixiedataToFpdfAdapter{archive}
	fpdfRenderer := render.NewFpdfRenderer(render.New(adapter, adapter))
	fpdfPath := filepath.Join("compare", fmt.Sprintf("%d_fpdf.pdf", *recordID))
	fpdfOut, err := os.Create(fpdfPath)
	if err != nil {
		return err
	}
	if err := fpdfRenderer.Render(ctx, render.Template{Name: "fpdf:soldier", Engine: "fpdf"}, map[string]any{
		"soldier":  *enriched,
		"options":  render.PDFOptions{Orientation: "L"},
		"branding": encode.BrandingFromIdentity(identity),
	}, fpdfOut); err != nil {
		fpdfOut.Close()
		return fmt.Errorf("fpdf render: %w", err)
	}
	fpdfOut.Close()

	typstRenderer := render.NewTypstRenderer(binPath, filepath.Dir(templateDir))
	typstPath := filepath.Join("compare", fmt.Sprintf("%d_typst.pdf", *recordID))
	typstOut, err := os.Create(typstPath)
	if err != nil {
		return err
	}
	data := encode.NewTemplateDataForSoldier(*enriched, render.PDFOptions{}, encode.BrandingFromIdentity(identity))
	tpl := render.Template{Name: *templateName, Path: filepath.Join(templateDir, *templateName+".typ"), Engine: "typst"}
	payload := templateDataToMap(data)
	if err := typstRenderer.Render(ctx, tpl, payload, typstOut); err != nil {
		typstOut.Close()
		return fmt.Errorf("typst render: %w", err)
	}
	typstOut.Close()

	fmt.Printf("wrote %s and %s for record %d\n", fpdfPath, typstPath, *recordID)
	return nil
}

// dixiedataToFpdfAdapter satisfies the render.SoldierLister and
// render.UserIdentityStore interfaces so the LocalArchive can be
// passed directly to a *render.Service.
type dixiedataToFpdfAdapter struct {
	a *dixiedata.LocalArchive
}

func (d dixiedataToFpdfAdapter) List(page, pageSize int) ([]models.Soldier, int, error) {
	return d.a.List(page, pageSize)
}

func (d dixiedataToFpdfAdapter) GetByID(id int64) (*models.Soldier, error) {
	return d.a.GetByID(id)
}

func (d dixiedataToFpdfAdapter) UserIdentity() (models.UserIdentity, error) {
	return d.a.UserIdentity()
}

// templateDataToMap serializes a TemplateData to JSON and parses it
// back into a map[string]any so the render package can serialize it
// again as #sys.inputs.
func templateDataToMap(d encode.TemplateData) map[string]any {
	raw, err := d.Marshal()
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
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
