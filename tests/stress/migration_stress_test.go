package stress

import (
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
)

func TestLegacyBombMigratesToCurrentSchema(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "legacy-bomb.ddbak")
	createLegacyBackupZip(t, backupPath)

	restoreDB, _ := newStressDB(t)
	restoreSvc := services.NewSoldierService(restoreDB)
	backupSvc := services.NewBackupService(restoreDB, restoreSvc)
	restoreDir := filepath.Join(t.TempDir(), ".dixiedata")

	if _, err := backupSvc.Import(backupPath, restoreDir); err != nil {
		t.Fatalf("Import legacy backup: %v", err)
	}
	restoreDB.Close()

	reopened, err := openExistingStressDB(restoreDir)
	if err != nil {
		t.Fatalf("openExistingStressDB: %v", err)
	}
	defer reopened.Close()

	names := columnNames(t, reopened.Conn(), "soldiers")
	if !containsString(names, "prefix") || !containsString(names, "suffix") {
		t.Fatalf("expected migrated schema to contain prefix/suffix columns, got %#v", names)
	}
	if containsString(names, "added_by") {
		t.Log("added_by column exists in migrated schema")
	} else {
		t.Log("architectural gap: soldiers.added_by is not present in the current schema, so legacy migration cannot preserve it")
	}
}

func TestModernBackupPreservesPrefixAndSuffixAcrossImport(t *testing.T) {
	sourceDB, soldierSvc, backupSvc, dataDir := newStressServices(t)
	defer sourceDB.Close()

	if _, err := soldierSvc.Create(models.Soldier{
		DisplayID: "MODERN-0001",
		Prefix:    "Col.",
		FirstName: "Modern",
		LastName:  "Proof",
		Suffix:    "III",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "modern.ddbak")
	if _, err := backupSvc.Export(backupPath, dataDir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	restoreDB, _ := newStressDB(t)
	restoreSvc := services.NewSoldierService(restoreDB)
	restoreBackupSvc := services.NewBackupService(restoreDB, restoreSvc)
	restoreDir := filepath.Join(t.TempDir(), ".dixiedata")

	if _, err := restoreBackupSvc.Import(backupPath, restoreDir); err != nil {
		t.Fatalf("Import: %v", err)
	}
	restoreDB.Close()

	reopened, err := openExistingStressDB(restoreDir)
	if err != nil {
		t.Fatalf("openExistingStressDB: %v", err)
	}
	defer reopened.Close()
	reopenedSvc := services.NewSoldierService(reopened)
	results, total, err := reopenedSvc.SearchPage("Modern", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("expected one restored modern record, got total=%d len=%d", total, len(results))
	}
	full, err := reopenedSvc.GetByID(results[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if full.Prefix != "Col." || full.Suffix != "III" {
		t.Fatalf("prefix/suffix were not preserved: %#v", full)
	}
}
