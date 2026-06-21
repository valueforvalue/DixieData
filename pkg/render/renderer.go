package render

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Renderer is the interface that TypstRenderer implements. The
// render.Registry dispatches PrintSettings to the right template.
//
// Render writes a single PDF to out. The template name selects a
// `.typ` file from <program-dir>/templates/. Data is the
// JSON-marshalable payload that the template reads.
type Renderer interface {
	Name() string
	// ListTemplates returns the templates this renderer can serve.
	// The first return value is the template name; the second is the
	// template's metadata block (record types, orientation, etc).
	ListTemplates() ([]Template, error)
	// Render compiles the template with the given data and writes the
	// output to w.
	Render(ctx context.Context, tpl Template, data map[string]any, w io.Writer) error
}

// Template is a discovered `.typ` file with its metadata block parsed.
type Template struct {
	Name        string
	Path        string
	RecordTypes []string
	Orientation string
	ExportTypes []string
	Description string
	Engine      string // "typst"
}

// Registry is the dispatcher. After slice 7, the Registry only
// dispatches to the TypstRenderer; the fpdf fallback is gone.
type Registry struct {
	typst       *TypstRenderer
	templateDir string
}

// NewRegistry constructs a Registry with the typst renderer wired up.
func NewRegistry(typst *TypstRenderer, templateDir string) *Registry {
	return &Registry{typst: typst, templateDir: templateDir}
}

// Resolve returns the template that matches the given PrintSettings, or
// an error if no template matches.
//
// If PrintSettings.Template is set and a .typ file with that name
// exists, return it. Otherwise, return the default for (recordType,
// orientation). If no typst template matches, return an error.
func (r *Registry) Resolve(ps PrintSettings, recordType string) (Template, error) {
	if r.templateDir == "" {
		return Template{}, fmt.Errorf("template directory not configured")
	}
	if name := strings.TrimSpace(ps.Template); name != "" {
		path := filepath.Join(r.templateDir, name+".typ")
		if _, err := os.Stat(path); err == nil {
			return Template{Name: name, Path: path, Engine: "typst"}, nil
		}
	}
	// Default mapping: the audit-derived record-subtype and orientation.
	// Each subtype gets a template named <subtype>_<orientation>. If
	// the file does not exist, return an error rather than falling
	// back to a missing renderer.
	defaultName := defaultTemplateName(recordType, ps.Orientation)
	if defaultName != "" {
		path := filepath.Join(r.templateDir, defaultName+".typ")
		if _, err := os.Stat(path); err == nil {
			return Template{Name: defaultName, Path: path, Engine: "typst"}, nil
		}
	}
	return Template{}, fmt.Errorf("no typst template matches recordType=%q orientation=%q", recordType, ps.Orientation)
}

func defaultTemplateName(recordType, orientation string) string {
	recordType = strings.ToLower(strings.TrimSpace(recordType))
	orientation = strings.ToLower(strings.TrimSpace(orientation))
	if recordType == "" {
		recordType = "soldier"
	}
	// Bulk templates are orientation-agnostic; a single template
	// loops over the sorted array and emits each record with the
	// orientation the caller asked for. The metadata block in
	// templates/bulk_soldier.typ uses orientation: any.
	if recordType == "bulk" {
		return "bulk_soldier"
	}
	if orientation == "p" || orientation == "portrait" {
		return recordType + "_portrait"
	}
	return recordType + "_landscape"
}

// Render is the public entry point used by the DixieData export entry
// points. It resolves the template and dispatches to the typst
// renderer.
func (r *Registry) Render(ctx context.Context, ps PrintSettings, recordType string, data map[string]any, w io.Writer) error {
	tpl, err := r.Resolve(ps, recordType)
	if err != nil {
		return err
	}
	if tpl.Engine != "typst" {
		return fmt.Errorf("unknown template engine %q", tpl.Engine)
	}
	return r.typst.Render(ctx, tpl, data, w)
}

// templateMetadataPattern matches the metadata block at the top of a
// .typ file. The block is a series of Typst comments with the shape
//
//   // metadata:
//   //   name: foo
//   //   record_types: [soldier, spouse]
//   //   orientation: landscape
//   //   export_types: [record_card]
//   //   description: One-liner
//
// Block is delimited by `// metadata:` and the first non-`//` line.
var templateMetadataPattern = regexp.MustCompile(`(?m)^//\s*metadata:\s*$`)

// parseTemplateMetadata extracts the metadata block from a .typ file's
// header. Returns a zero-value Template if the file has no metadata
// block.
func parseTemplateMetadata(path string, content string) Template {
	tpl := Template{
		Path:   path,
		Engine: "typst",
	}

	loc := templateMetadataPattern.FindStringIndex(content)
	if loc == nil {
		return tpl
	}
	// Take everything from the metadata line onwards until the first
	// non-comment line or end of file.
	header := content[loc[0]:]
	lines := strings.Split(header, "\n")
	if len(lines) == 0 {
		return tpl
	}
	// First line is "// metadata:". Iterate the rest.
	fields := 0
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		// Strip the leading "//" and split on the first colon.
		body := strings.TrimPrefix(trimmed, "//")
		body = strings.TrimSpace(body)
		idx := strings.Index(body, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(body[:idx])
		value := strings.TrimSpace(body[idx+1:])
		switch key {
		case "name":
			tpl.Name = value
		case "record_types":
			tpl.RecordTypes = splitBracketList(value)
		case "orientation":
			tpl.Orientation = value
		case "export_types":
			tpl.ExportTypes = splitBracketList(value)
		case "description":
			tpl.Description = value
		}
		fields++
		if fields >= 5 {
			break
		}
	}
	if tpl.Name == "" {
		// Fall back to the file's basename.
		tpl.Name = strings.TrimSuffix(filepath.Base(path), ".typ")
	}
	return tpl
}

func splitBracketList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// DiscoverTemplates reads every `.typ` file in the template directory and
// returns its parsed metadata block.
func DiscoverTemplates(templateDir string) ([]Template, error) {
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		return nil, fmt.Errorf("read template directory: %w", err)
	}
	var out []Template
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".typ") {
			continue
		}
		path := filepath.Join(templateDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out = append(out, parseTemplateMetadata(path, string(content)))
	}
	return out, nil
}
