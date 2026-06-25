package appshell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindTemplatesDirPicksTypstNotInternalTemplates is a regression
// test for the bug where the appshell's findTemplatesDir returned
// internal/templates (the Go html/template dir) instead of templates
// (the Typst dir). Both directories exist in the repo, but only the
// Typst dir contains soldier_landscape.typ.
func TestFindTemplatesDirPicksTypstNotInternalTemplates(t *testing.T) {
	app := NewApp()
	dir, err := app.findTemplatesDir()
	if err != nil {
		t.Fatalf("findTemplatesDir: %v", err)
	}
	t.Logf("findTemplatesDir -> %s", dir)
	// Reject internal/templates (Go html dir) explicitly.
	if strings.HasSuffix(filepath.Clean(dir), string(filepath.Separator)+"internal"+string(filepath.Separator)+"templates") {
		t.Fatalf("findTemplatesDir returned Go html template dir: %s", dir)
	}
	// Verify the sentinel Typst file is present.
	if _, err := os.Stat(filepath.Join(dir, "soldier_landscape.typ")); err != nil {
		t.Fatalf("templates dir missing soldier_landscape.typ: %v", err)
	}
}
