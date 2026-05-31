package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func TestRestoreBackupArchiveRestoresLocalArchiveSnapshot(t *testing.T) {
	sourceDB := newTestDB(t)
	sourceSvc := NewSoldierService(sourceDB)
	backupSvc := NewBackupService(sourceDB, sourceSvc)
	if _, err := sourceDB.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}

	sourceDir := t.TempDir()
	created, err := sourceSvc.Create(models.Soldier{
		DisplayID: "DXD-00054",
		FirstName: "James",
		LastName:  "Myers",
		Records:   []models.Record{{RecordType: "Roster", AppID: "A1", Details: "Roster details"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	imagePath := filepath.Join(sourceDir, "images", "dxd-00054", "portrait.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := sourceSvc.AddImage(created.ID, "portrait.png", `images\dxd-00054\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "restore-point.ddbak")
	if _, err := backupSvc.Export(backupPath, sourceDir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	restoreDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(restoreDir, "images", "stale"), 0o755); err != nil {
		t.Fatalf("MkdirAll(stale): %v", err)
	}
	if err := os.WriteFile(filepath.Join(restoreDir, "images", "stale", "old.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale): %v", err)
	}

	if _, err := RestoreBackupArchive(backupPath, restoreDir); err != nil {
		t.Fatalf("RestoreBackupArchive: %v", err)
	}

	restoredDB, err := db.Open(restoreDir)
	if err != nil {
		t.Fatalf("db.Open(restored): %v", err)
	}
	defer restoredDB.Close()
	restoredSvc := NewSoldierService(restoredDB)
	restored, err := restoredSvc.GetByDisplayID("DXD-00054")
	if err != nil {
		t.Fatalf("GetByDisplayID: %v", err)
	}
	if restored.FirstName != "James" || len(restored.Images) != 1 || len(restored.Records) != 1 {
		t.Fatalf("restored = %#v", restored)
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "images", "dxd-00054", "portrait.png")); err != nil {
		t.Fatalf("restored image missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "images", "stale")); !os.IsNotExist(err) {
		t.Fatalf("stale image directory should be replaced, err = %v", err)
	}
}
