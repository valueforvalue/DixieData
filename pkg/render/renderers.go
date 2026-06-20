package render

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dadido3/go-typst"
	"github.com/valueforvalue/DixieData/internal/models"
)

// FpdfRenderer implements Renderer using the existing fpdf-based
// service. It exists as a wrapper so the new Registry can dispatch
// uniformly; the actual fpdf work happens in (*Service).Export*PDF.
type FpdfRenderer struct {
	service *Service
}

// NewFpdfRenderer wraps an existing Service as a Renderer.
func NewFpdfRenderer(s *Service) *FpdfRenderer {
	return &FpdfRenderer{service: s}
}

// Name returns the engine name.
func (f *FpdfRenderer) Name() string { return "fpdf" }

// ListTemplates returns a single synthetic "fpdf:soldier" template
// that the registry uses to mean "render with fpdf, no template file".
// The fpdf path does not read .typ files.
func (f *FpdfRenderer) ListTemplates() ([]Template, error) {
	return []Template{
		{Name: "fpdf:soldier", Engine: "fpdf", Description: "fpdf fallback (legacy)"},
		{Name: "fpdf:spouse", Engine: "fpdf", Description: "fpdf fallback (legacy)"},
		{Name: "fpdf:widow", Engine: "fpdf", Description: "fpdf fallback (legacy)"},
	}, nil
}

// Render dispatches to the fpdf service. The template's name encodes
// the record type; data is ignored (the fpdf service reads from the
// model directly via the soldier service). The output is a PDF.
func (f *FpdfRenderer) Render(ctx context.Context, tpl Template, data map[string]any, w io.Writer) error {
	// The fpdf path needs a real file path because fpdf writes to disk.
	// Write to a temp file then copy to w.
	tmp, err := os.CreateTemp("", "dixiedata-fpdf-*.pdf")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	recordType := recordTypeFromTemplateName(tpl.Name)
	soldier, _ := data["soldier"].(models.Soldier)
	options, _ := data["options"].(PDFOptions)
	settings, _ := data["settings"].(PrintSettings)
	snapshot, _ := data["snapshot"].(AnalyticsSnapshot)
	month, _ := data["month"].(int)
	calendarAny, _ := data["calendar"].(map[int][]models.Soldier)

	var renderErr error
	switch recordType {
	case "soldier":
		renderErr = f.service.ExportSoldierPDF(tmpPath, soldier, options)
	case "soldier-no-images":
		renderErr = f.service.ExportSoldierPDFWithoutImages(tmpPath, soldier)
	case "anniversary":
		renderErr = f.service.ExportMonthlyAnniversaryPDF(tmpPath, month, calendarAny, options)
	case "database":
		renderErr = f.service.ExportFullDatabasePDF(tmpPath, settings)
	case "analytics":
		renderErr = f.service.ExportAnalyticsSummaryPDF(tmpPath, snapshot, options)
	default:
		os.Remove(tmpPath)
		return fmt.Errorf("fpdf renderer: unknown template %q", tpl.Name)
	}
	if renderErr != nil {
		os.Remove(tmpPath)
		return renderErr
	}
	defer os.Remove(tmpPath)

	// Copy the temp file to the writer.
	f2, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer f2.Close()
	_, err = io.Copy(w, f2)
	return err
}

// recordTypeFromTemplateName extracts the record-type portion of a
// template name like "fpdf:soldier" or "fpdf:database". Returns "" if
// the name doesn't fit the convention.
func recordTypeFromTemplateName(name string) string {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "fpdf:") {
		return name
	}
	return strings.TrimPrefix(name, "fpdf:")
}

// TypstRenderer implements Renderer by compiling .typ templates with
// the bundled Typst binary. Currently a skeleton -- the data flow is
// wired but real templates ship in slice 2+.
type TypstRenderer struct {
	binPath  string
	rootDir  string
	fontDirs []string
}

// NewTypstRenderer constructs a TypstRenderer that shells out to the
// Typst binary at binPath. The rootDir is the working directory for
// `typst compile`; the template files are resolved relative to it.
func NewTypstRenderer(binPath, rootDir string) *TypstRenderer {
	return &TypstRenderer{
		binPath:  binPath,
		rootDir:  rootDir,
		fontDirs: nil,
	}
}

// Name returns the engine name.
func (t *TypstRenderer) Name() string { return "typst" }

// ListTemplates returns every .typ file in the renderer root.
func (t *TypstRenderer) ListTemplates() ([]Template, error) {
	templatesDir := filepath.Join(t.rootDir, "templates")
	return DiscoverTemplates(templatesDir)
}

// Render compiles a .typ template with the given data. The data is
// serialized as JSON and exposed to the template via #let data =
// json("data.json"). The output is written to w as a PDF.
func (t *TypstRenderer) Render(ctx context.Context, tpl Template, data map[string]any, w io.Writer) error {
	// Build a temporary working directory: write the template as
	// main.typ, write data.json, run `typst compile data.json main.typ -`.
	workDir, err := os.MkdirTemp("", "dixiedata-typst-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	mainPath := filepath.Join(workDir, "main.typ")
	if err := copyFile(tpl.Path, mainPath); err != nil {
		return fmt.Errorf("copy template: %w", err)
	}

	dataPath := filepath.Join(workDir, "data.json")
	if err := writeJSONFile(dataPath, data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	caller := typst.CLI{ExecutablePath: t.binPath}
	if err := caller.Compile(
		openFile(mainPath),
		w,
		&typst.OptionsCompile{
			Root:   workDir,
			Format: typst.OutputFormatPDF,
		},
	); err != nil {
		return fmt.Errorf("typst compile: %w", err)
	}
	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// writeJSONFile serializes v as indented JSON to path.
func writeJSONFile(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// openFile returns an io.Reader over the file at path. Used as the
// source argument to go-typst's Compile.
func openFile(path string) io.Reader {
	f, err := os.Open(path)
	if err != nil {
		// go-typst expects a non-nil reader; return an empty one on
		// error so the caller sees a clean error.
		return bytes.NewReader(nil)
	}
	return f
}
