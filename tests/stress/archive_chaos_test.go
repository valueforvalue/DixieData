package stress

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
)

func TestCorruptBackupImportRejectsPoisonArchives(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test: run via `make stress` or `go test ./tests/stress/...`")
	}
	database, _, backupSvc, _ := newStressServices(t)
	defer database.Close()

	t.Run("truncated zip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "truncated.ddbak")
		if err := os.WriteFile(path, []byte("not-a-zip"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := backupSvc.Import(path, filepath.Join(t.TempDir(), "restore")); err == nil {
			t.Fatal("expected truncated zip import to fail")
		}
	})

	t.Run("invalid manifest json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid-manifest.ddbak")
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		zipWriter := zip.NewWriter(file)
		writer, err := zipWriter.Create("manifest.json")
		if err != nil {
			t.Fatalf("Create manifest: %v", err)
		}
		if _, err := writer.Write([]byte("{ definitely-not-json")); err != nil {
			t.Fatalf("Write manifest: %v", err)
		}
		if err := zipWriter.Close(); err != nil {
			t.Fatalf("Close zip: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("Close file: %v", err)
		}
		if _, err := backupSvc.Import(path, filepath.Join(t.TempDir(), "restore")); err == nil {
			t.Fatal("expected invalid manifest import to fail")
		}
	})

	t.Run("mismatched counts", func(t *testing.T) {
		validBackup, _ := createValidCurrentBackup(t)
		data, err := os.ReadFile(validBackup)
		if err != nil {
			t.Fatalf("ReadFile valid backup: %v", err)
		}

		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		manifest := archive.BackupManifest{}
		files := map[string][]byte{}
		for _, file := range reader.File {
			rc, err := file.Open()
			if err != nil {
				t.Fatalf("Open zip entry: %v", err)
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("ReadAll zip entry: %v", err)
			}
			if file.Name == "manifest.json" {
				if err := json.Unmarshal(content, &manifest); err != nil {
					t.Fatalf("Unmarshal manifest: %v", err)
				}
				continue
			}
			files[file.Name] = content
		}
		manifest.Soldiers++
		poisonPath := filepath.Join(t.TempDir(), "mismatched.ddbak")
		if err := writePoisonArchive(poisonPath, manifest, files); err != nil {
			t.Fatalf("writePoisonArchive: %v", err)
		}
		if _, err := backupSvc.Import(poisonPath, filepath.Join(t.TempDir(), "restore")); err == nil {
			t.Fatal("expected mismatched backup counts to fail validation")
		}
	})
}

func TestCorruptSharedImportRejectsPoisonArchives(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test: run via `make stress` or `go test ./tests/stress/...`")
	}
	database, _, backupSvc, dataDir := newStressServices(t)
	defer database.Close()

	t.Run("wrong archive kind", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "wrong-kind.ddshare")
		manifest := archive.BackupManifest{
			Format:        "dixiedata-backup",
			Version:       buildinfo.BackupFormatVersion,
			ArchiveKind:   "backup",
			AppVersion:    buildinfo.AppVersion,
			SchemaVersion: buildinfo.SchemaVersion,
			CreatedAt:     "2026-05-16T00:00:00Z",
			DataFormat:    "sqlite",
			DatabaseFile:  "data/dixiedata.db",
			ImageRoot:     "images/",
		}
		if err := writePoisonArchive(path, manifest, map[string][]byte{
			manifest.DatabaseFile: []byte("not sqlite"),
		}); err != nil {
			t.Fatalf("writePoisonArchive: %v", err)
		}
		if _, err := backupSvc.ImportSharedBackup(path, dataDir); err == nil {
			t.Fatal("expected wrong archive kind to fail")
		}
	})

	t.Run("schema from future", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "future.ddshare")
		manifest := archive.BackupManifest{
			Format:        "dixiedata-backup",
			Version:       buildinfo.BackupFormatVersion,
			ArchiveKind:   "shared",
			AppVersion:    buildinfo.AppVersion,
			SchemaVersion: buildinfo.SchemaVersion + 100,
			CreatedAt:     "2026-05-16T00:00:00Z",
			DataFormat:    "sqlite",
			DatabaseFile:  "data/dixiedata.db",
			ImageRoot:     "images/",
		}
		if err := writePoisonArchive(path, manifest, map[string][]byte{
			manifest.DatabaseFile: []byte("not sqlite"),
		}); err != nil {
			t.Fatalf("writePoisonArchive: %v", err)
		}
		if _, err := backupSvc.ImportSharedBackup(path, dataDir); err == nil {
			t.Fatal("expected future schema shared archive to fail")
		}
	})
}
