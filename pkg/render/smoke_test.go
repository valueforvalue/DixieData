package render

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestTypstRendererSmoke is the slice-1 smoke test. It loads the
// templates/hello.typ template, feeds it a tiny JSON payload, and
// asserts the output is a non-empty PDF. Proves the end-to-end
// pipeline: Go -> go-typst -> bundled binary -> Typst -> PDF.
func TestTypstRendererSmoke(t *testing.T) {
	binPath := findTypstBinary(t)
	templatesDir := findTemplatesDir(t)

	typst := NewTypstRenderer(binPath, filepath.Dir(templatesDir))
	tpl, err := typst.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(tpl) == 0 {
		t.Fatalf("no templates discovered in %s", templatesDir)
	}
	var hello *Template
	for i, candidate := range tpl {
		if candidate.Name == "hello" {
			hello = &tpl[i]
			break
		}
	}
	if hello == nil {
		names := make([]string, 0, len(tpl))
		for _, c := range tpl {
			names = append(names, c.Name)
		}
		t.Fatalf("hello template not found; discovered: %v", names)
	}

	data := map[string]any{
		"soldier": map[string]any{
			"display_id": "TEST-001",
		},
	}
	var buf bytes.Buffer
	if err := typst.Render(context.Background(), *hello, data, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("rendered PDF is empty")
	}
	if buf.Len() < 100 {
		t.Fatalf("rendered PDF is suspiciously small (%d bytes)", buf.Len())
	}
	// Sanity: the bytes should start with the PDF magic number.
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("output is not a PDF (first 8 bytes: %q)", buf.Bytes()[:8])
	}
}

// findTypstBinary walks the repo looking for a typst binary. In
// production this is <repo>/bin/typst-<platform>. In tests it could be
// anywhere depending on the developer machine.
func findTypstBinary(t *testing.T) string {
	t.Helper()
	// Walk up from cwd to find a bin/typst-* file.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		for _, name := range []string{"typst-windows.exe", "typst-macos", "typst-linux"} {
			candidate := filepath.Join(dir, "bin", name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("no typst binary found; expected bin/typst-{windows.exe,macos,linux} above the working dir")
	return ""
}

// findTemplatesDir returns the absolute path to the templates/
// directory in the repo root. Walks up from the test's working
// directory to find it.
func findTemplatesDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
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
	t.Fatalf("no templates/ directory found above the working dir")
	return ""
}
