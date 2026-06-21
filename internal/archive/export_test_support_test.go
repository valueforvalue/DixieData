package archive

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/pkg/render"
)

// newTestExportServiceWithRegistry constructs an ExportService
// with the typst-backed Registry wired. After slice 7, every
// export goes through the Registry; the fpdf Service fallback
// is gone. This helper is the only sanctioned way for tests
// to construct an ExportService.
func newTestExportServiceWithRegistry(t *testing.T, d *db.DB, svc *SoldierService) *ExportService {
	t.Helper()
	binPath := findTypstBinaryInTest(t)
	templatesDir := findTemplatesDirInTest(t)
	t.Logf("test: typst=%s templates=%s", binPath, templatesDir)
	typst := render.NewTypstRenderer(binPath, filepath.Dir(templatesDir))
	reg := render.NewRegistry(typst, templatesDir)
	exportSvc := NewExportService(d, svc)
	exportSvc.SetRegistry(reg)
	return exportSvc
}

// findTypstBinaryInTest walks the repo looking for a typst binary.
// Mirrors pkg/render/smoke_test.go::findTypstBinary.
func findTypstBinaryInTest(t *testing.T) string {
	t.Helper()
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

// findTemplatesDirInTest walks the repo looking for the typst
// templates directory. Distinct from the Go html/template
// directory at internal/templates (which is unrelated). The
// typst templates live at <repo>/templates/ and contain
// soldier_landscape.typ, widow_portrait.typ, etc.
func findTemplatesDirInTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "templates")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			// Verify it's the typst templates dir by checking
			// for a known file. internal/templates contains
			// Go html/template files instead.
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
	t.Fatalf("no typst templates/ directory found above the working dir")
	return ""
}

// extractPDFText shells out to `pdftotext` to get the readable
// text from a PDF. Returns the empty string if pdftotext is
// missing or fails. Tests that rely on text-contains assertions
// should call this helper instead of scanning the raw PDF bytes
// (typst-compressed streams do not contain searchable text).
//
// Skips the test if pdftotext is unavailable so CI without it
// doesn't fail spuriously. Local dev boxes have poppler installed
// via the standard package manager; CI installs it in the
// test job.
func extractPDFText(t *testing.T, pdfPath string) string {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not on PATH; install poppler-utils (Linux) / poppler (macOS) / poppler (Windows) to enable PDF text-contains assertions")
	}
	cmd := exec.Command("pdftotext", pdfPath, "-")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	return string(out)
}

// extractDirectoryPDFText runs extractPDFText against every PDF
// in a directory and concatenates the result. The typst-backed
// bulk export writes one PDF per record into a directory named
// <outPath-stem>-record-pdfs/. Tests that want to assert on the
// aggregated output use this helper instead of scanning the
// directory manually.
func extractDirectoryPDFText(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %q: %v", dir, err)
	}
	var combined strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pdf") {
			continue
		}
		combined.WriteString(extractPDFText(t, filepath.Join(dir, entry.Name())))
		combined.WriteString("\n")
	}
	return combined.String()
}
