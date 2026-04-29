package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectRootFromFindsNearestWailsConfig(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "build", "bin")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok := projectRootFrom(nested)
	if !ok {
		t.Fatal("expected project root to be found")
	}
	if got != root {
		t.Fatalf("root=%q want %q", got, root)
	}
}

func TestRecordImageDirUsesSanitizedDisplayID(t *testing.T) {
	absolute, relative := RecordImageDir(`C:\repo\.dixiedata`, `PENSION/42 A`)
	if absolute != `C:\repo\.dixiedata\images\PENSION-42-A` {
		t.Fatalf("absolute=%q", absolute)
	}
	if relative != `images\PENSION-42-A` {
		t.Fatalf("relative=%q", relative)
	}
}
