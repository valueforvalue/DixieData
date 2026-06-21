// Command dixiedata-tune is a developer tool for iterating on the
// Typst-based PDF export templates. It opens a DixieData SQLite in
// read-only mode and renders a single record (or every record) through
// a Typst template so the researcher can see the output, then
// iterate on the .typ file in their editor and re-run.
//
// Slice 7 of the Typst migration removed the fpdf path. The tune
// tool's exports now mirror the appshell's exports exactly: every
// render goes through pkg/render.TypstRenderer, the same renderer
// the production appshell uses. There is no separate fpdf baseline.
//
// Usage:
//
//	dixiedata-tune --db <path> render --template <name> --record <id> --out <pdf>
//	dixiedata-tune --db <path> list-templates
//	dixiedata-tune --db <path> list-records
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
		"help": true, "-h": true, "--help": true,
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
		return doRender(ctx, args, archive, *binFlag, *templateDir, filepath.Dir(*dbFlag))
	case "list-records":
		return doListRecords(archive)
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
	return nil
}

// defaultTypstBinary returns the platform-specific Typst binary in
// <repo>/bin/. DixieData is a Windows-only app; the release
// archive ships only typst-windows.exe. The macOS and Linux
// names are kept as fallbacks so this code still locates a
// binary if a developer happens to be running it on a
// non-Windows host for testing, but release builds do not
// bundle them.
//
// The tool is run from the repo root or any subdirectory;
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
// find a templates/ directory that contains a known typst
// template (soldier_landscape.typ). Distinct from the Go
// html/template directory at internal/templates.
func findTemplatesDir() string {
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "templates")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(candidate, "soldier_landscape.typ")); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "templates"
}

// doRender renders a Typst template against a single record. The
// output mirrors what the production appshell produces for the same
// record, so iterating on a .typ file in this tool produces a
// faithful preview of the export.
func doRender(ctx context.Context, args []string, archive *dixiedata.LocalArchive, binPath, templateDir, dataDir string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	templateName := fs.String("template", "", "Template name (e.g. soldier_landscape)")
	recordID := fs.Int64("record", 0, "Record ID to render")
	outPath := fs.String("out", "", "Output PDF path")
	orientation := fs.String("orientation", "L", "Page orientation: L (landscape) or P (portrait)")
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

	soldier, err := archive.GetByID(*recordID)
	if err != nil {
		return fmt.Errorf("get record: %w", err)
	}
	// The appshell resolves each image's FilePath against the data
	// dir before calling the renderer. The tune tool needs to do the
	// same so the renderer's image-staging step can find the file
	// on disk. The data dir is the parent directory of the SQLite
	// passed via --db.
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	identity, err := archive.UserIdentity()
	if err != nil {
		return fmt.Errorf("get identity: %w", err)
	}

	options := render.PDFOptions{Orientation: *orientation, IncludeImages: true}
	branding := encode.BrandingFromIdentity(identity)
	fullData := encode.NewTemplateDataForSoldier(*soldier, options, branding)
	payload := templateDataToMap(fullData)

	out, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	typstRenderer := render.NewTypstRenderer(binPath, filepath.Dir(templateDir))
	tpl := render.Template{
		Name: *templateName,
		Path: filepath.Join(templateDir, *templateName+".typ"),
		Engine: "typst",
	}
	if err := typstRenderer.Render(ctx, tpl, payload, out); err != nil {
		return fmt.Errorf("render: %w", err)
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
