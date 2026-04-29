package services

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func TestBackupService_ExportCreatesManifestAndImages(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	backupSvc := NewBackupService(soldierSvc)

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
	for _, expected := range []string{"manifest.json", "data/soldiers.json", "images/pension-1/portrait.png"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("backup missing %s", expected)
		}
	}
}

func TestBackupService_ImportRestoresDataAndImages(t *testing.T) {
	sourceDB := newTestDB(t)
	sourceSvc := NewSoldierService(sourceDB)
	backupSvc := NewBackupService(sourceSvc)

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

func openExistingTestDB(dataDir string) (*db.DB, error) {
	return db.Open(dataDir)
}
