package archive

// TestWriteJPGFormatSidecar is a unit test for the sidecar
// writer that doesn't need PDFium. It exercises the
// file-write + JSON-marshal path directly so the sidecar
// shape is locked even when the test env has no PDFium.
// The full integration test (TestExportSoldierJPGWritesFormatSidecar)
// runs when DIXIEDATA_PDFIUM_DLL is set.
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

func TestWriteJPGFormatSidecarUnit(t *testing.T) {
	dir := testtemp.New(t).Path()
	jpgPath := filepath.Join(dir, "page1.jpg")
	if err := os.WriteFile(jpgPath, []byte("fake jpg bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile jpg: %v", err)
	}
	if err := writeJPGFormatSidecar(jpgPath, buildinfo.JPGFormatVersion, "soldier"); err != nil {
		t.Fatalf("writeJPGFormatSidecar: %v", err)
	}
	sidecarPath := strings.TrimSuffix(jpgPath, filepath.Ext(jpgPath)) + ".meta.json"
	body, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("ReadFile sidecar: %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := payload["format_version"]; got != buildinfo.JPGFormatVersion {
		t.Errorf("format_version = %q, want %q", got, buildinfo.JPGFormatVersion)
	}
	if got := payload["content_kind"]; got != "soldier" {
		t.Errorf("content_kind = %q, want soldier", got)
	}
	if payload["app_version"] != buildinfo.AppVersion {
		t.Errorf("app_version = %q, want %q", payload["app_version"], buildinfo.AppVersion)
	}
}
