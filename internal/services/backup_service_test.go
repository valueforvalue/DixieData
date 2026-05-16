package services

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func TestBackupService_ExportCreatesManifestAndImages(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	backupSvc := NewBackupService(d, soldierSvc)

	dataDir := t.TempDir()
	created, err := soldierSvc.Create(models.Soldier{
		DisplayID: "PENSION-1",
		FirstName: "Robert",
		LastName:  "Lee",
		Records:   []models.Record{{RecordType: "Roster", AppID: "APP-1", Details: "Roster details"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	imagePath := filepath.Join(dataDir, "images", "pension-1", "portrait.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := soldierSvc.AddImage(created.ID, "portrait.png", `images\pension-1\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "backup.zip")
	manifest, err := backupSvc.Export(outPath, dataDir)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if manifest.Soldiers != 1 || manifest.Records != 1 || manifest.Images != 1 {
		t.Fatalf("manifest = %#v", manifest)
	}

	reader, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	joined := strings.Join(names, "\n")
	for _, expected := range []string{"manifest.json", "data/dixiedata.db", "images/pension-1/portrait.png"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("backup missing %s", expected)
		}
	}

	var manifestFile *zip.File
	for _, file := range reader.File {
		if file.Name == "manifest.json" {
			manifestFile = file
			break
		}
	}
	if manifestFile == nil {
		t.Fatal("manifest.json missing")
	}
	rc, err := manifestFile.Open()
	if err != nil {
		t.Fatalf("Open manifest: %v", err)
	}
	defer rc.Close()
	var storedManifest BackupManifest
	if err := json.NewDecoder(rc).Decode(&storedManifest); err != nil {
		t.Fatalf("Decode manifest: %v", err)
	}
	if storedManifest.AppVersion != buildinfo.AppVersion || storedManifest.SchemaVersion != buildinfo.SchemaVersion || storedManifest.DataFormat != "sqlite" {
		t.Fatalf("unexpected stored manifest: %#v", storedManifest)
	}
}

func TestBackupService_ImportRestoresDataAndImages(t *testing.T) {
	sourceDB := newTestDB(t)
	sourceSvc := NewSoldierService(sourceDB)
	backupSvc := NewBackupService(sourceDB, sourceSvc)

	sourceDataDir := t.TempDir()
	created, err := sourceSvc.Create(models.Soldier{
		DisplayID: "PENSION-2",
		FirstName: "Thomas",
		LastName:  "Jackson",
		BuriedIn:  "Lexington",
		Records:   []models.Record{{RecordType: "Service Record", AppID: "APP-2", Details: "Service details"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sourceImagePath := filepath.Join(sourceDataDir, "images", "pension-2", "portrait.png")
	if err := os.MkdirAll(filepath.Dir(sourceImagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(sourceImagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := sourceSvc.AddImage(created.ID, "portrait.png", `images\pension-2\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "backup.zip")
	if _, err := backupSvc.Export(backupPath, sourceDataDir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	restoreDir := t.TempDir()
	manifest, err := backupSvc.Import(backupPath, restoreDir)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if manifest.Soldiers != 1 || manifest.Images != 1 {
		t.Fatalf("manifest = %#v", manifest)
	}

	restoreDB, err := openExistingTestDB(restoreDir)
	if err != nil {
		t.Fatalf("openExistingTestDB: %v", err)
	}
	defer restoreDB.Close()

	restoreSvc := NewSoldierService(restoreDB)
	results, total, err := restoreSvc.SearchPage("PENSION-2", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("restored search total=%d len=%d", total, len(results))
	}

	restored, err := restoreSvc.GetByID(results[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if restored.BuriedIn != "Lexington" || len(restored.Records) != 1 || len(restored.Images) != 1 {
		t.Fatalf("restored soldier = %#v", restored)
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "images", "pension-2", "portrait.png")); err != nil {
		t.Fatalf("restored image missing: %v", err)
	}
}

func TestBackupService_ImportAllowsExtraImageFilesInSQLiteBackup(t *testing.T) {
	sourceDB := newTestDB(t)
	sourceSvc := NewSoldierService(sourceDB)
	backupSvc := NewBackupService(sourceDB, sourceSvc)

	sourceDataDir := t.TempDir()
	created, err := sourceSvc.Create(models.Soldier{
		DisplayID: "PENSION-EXTRA",
		FirstName: "Extra",
		LastName:  "Image",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	liveImagePath := filepath.Join(sourceDataDir, "images", "pension-extra", "portrait.png")
	if err := os.MkdirAll(filepath.Dir(liveImagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll live image: %v", err)
	}
	if err := os.WriteFile(liveImagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile live image: %v", err)
	}
	if err := sourceSvc.AddImage(created.ID, "portrait.png", `images\pension-extra\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	orphanPath := filepath.Join(sourceDataDir, "images", "orphaned", "extra.png")
	if err := os.MkdirAll(filepath.Dir(orphanPath), 0o755); err != nil {
		t.Fatalf("MkdirAll orphan image: %v", err)
	}
	if err := os.WriteFile(orphanPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile orphan image: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "backup-extra-images.zip")
	if _, err := backupSvc.Export(backupPath, sourceDataDir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	restoreDir := t.TempDir()
	if _, err := backupSvc.Import(backupPath, restoreDir); err != nil {
		t.Fatalf("Import: %v", err)
	}

	if _, err := os.Stat(filepath.Join(restoreDir, "images", "pension-extra", "portrait.png")); err != nil {
		t.Fatalf("restored live image missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "images", "orphaned", "extra.png")); err != nil {
		t.Fatalf("restored orphan image missing: %v", err)
	}

	restoreDB, err := openExistingTestDB(restoreDir)
	if err != nil {
		t.Fatalf("openExistingTestDB: %v", err)
	}
	defer restoreDB.Close()
	restoreSvc := NewSoldierService(restoreDB)
	restored, total, err := restoreSvc.SearchPage("EXTRA", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(restored) != 1 {
		t.Fatalf("restored search total=%d len=%d", total, len(restored))
	}
	full, err := restoreSvc.GetByID(restored[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(full.Images) != 1 {
		t.Fatalf("restored DB image count = %d", len(full.Images))
	}
}

func openExistingTestDB(dataDir string) (*db.DB, error) {
	return db.Open(dataDir)
}

func TestBackupService_ImportLegacyJSONBackup(t *testing.T) {
	restoreDir := t.TempDir()
	backupPath := filepath.Join(t.TempDir(), "legacy.zip")

	file, err := os.Create(backupPath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	zipWriter := zip.NewWriter(file)

	manifest := BackupManifest{
		Format:    backupFormatName,
		Version:   1,
		CreatedAt: "2026-01-01T00:00:00Z",
		DataFile:  "data/soldiers.json",
		ImageRoot: "images/",
		Soldiers:  1,
		Records:   1,
		Images:    1,
	}
	if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	soldiers := []models.Soldier{{
		DisplayID: "PENSION-LEGACY",
		FirstName: "Legacy",
		LastName:  "Soldier",
		Records:   []models.Record{{RecordType: "Roster", AppID: "APP-1", Details: "Legacy details"}},
		Images:    []models.Image{{FileName: "portrait.png", FilePath: "images/pension-legacy/portrait.png", Caption: "Portrait"}},
	}}
	if err := writeBackupJSON(zipWriter, "data/soldiers.json", soldiers); err != nil {
		t.Fatalf("write soldiers: %v", err)
	}
	imageEntry, err := zipWriter.Create("images/pension-legacy/portrait.png")
	if err != nil {
		t.Fatalf("Create image entry: %v", err)
	}
	if _, err := imageEntry.Write(pngFixture()); err != nil {
		t.Fatalf("Write image entry: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("Close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close file: %v", err)
	}

	restoreDB := newTestDB(t)
	restoreSvc := NewSoldierService(restoreDB)
	backupSvc := NewBackupService(restoreDB, restoreSvc)

	manifestOut, err := backupSvc.Import(backupPath, restoreDir)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if manifestOut.Version != 1 {
		t.Fatalf("expected legacy manifest version 1, got %d", manifestOut.Version)
	}
	reopened, err := openExistingTestDB(restoreDir)
	if err != nil {
		t.Fatalf("openExistingTestDB: %v", err)
	}
	defer reopened.Close()
	reopenedSvc := NewSoldierService(reopened)
	results, total, err := reopenedSvc.SearchPage("PENSION-LEGACY", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("restored search total=%d len=%d", total, len(results))
	}
}
