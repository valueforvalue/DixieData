package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCompressImages_Build(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "compress-images.exe")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
}

func TestCompressImages_MissingDataDir_Errors(t *testing.T) {
	// Clear env so resolveDataDir sees no fallback.
	t.Setenv("DIXIEDATA_DATA_DIR", "")
	if err := run([]string{"--dry-run"}); err == nil {
		t.Fatalf("expected error when --data-dir is missing and DIXIEDATA_DATA_DIR is unset")
	}
}
