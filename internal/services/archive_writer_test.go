package services

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteZipArchive_ReplacesExistingFileWithoutTempResidue(t *testing.T) {
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "archive.zip")
	if err := os.WriteFile(outputPath, []byte("old-data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := writeZipArchive(outputPath, func(zipWriter *zip.Writer) error {
		entry, err := zipWriter.Create("payload.txt")
		if err != nil {
			return err
		}
		_, err = entry.Write([]byte("new archive payload"))
		return err
	}); err != nil {
		t.Fatalf("writeZipArchive: %v", err)
	}

	reader, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()
	if len(reader.File) != 1 || reader.File[0].Name != "payload.txt" {
		t.Fatalf("unexpected zip entries: %#v", reader.File)
	}
	rc, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("Open payload.txt: %v", err)
	}
	defer rc.Close()
	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(content) != "new archive payload" {
		t.Fatalf("payload = %q", string(content))
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "archive.zip" {
		t.Fatalf("unexpected output directory contents: %#v", entries)
	}
}
