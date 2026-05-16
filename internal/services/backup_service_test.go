package services

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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
	if _, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}

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
	if storedManifest.NodePrefix != "STC38" || storedManifest.OwnerName != "Samuel Thomas Carter" {
		t.Fatalf("unexpected origin metadata: %#v", storedManifest)
	}
	if storedManifest.ArchiveKind != archiveKindBackup {
		t.Fatalf("expected backup archive kind, got %#v", storedManifest)
	}
}

func TestBackupService_ExportSharedCreatesSharedManifest(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	backupSvc := NewBackupService(d, soldierSvc)

	outPath := filepath.Join(t.TempDir(), "shared.ddshare")
	manifest, err := backupSvc.ExportShared(outPath, t.TempDir())
	if err != nil {
		t.Fatalf("ExportShared: %v", err)
	}
	if manifest.ArchiveKind != archiveKindShared {
		t.Fatalf("manifest = %#v", manifest)
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

func TestBackupService_ImportPreservesLocalIdentityForCurrentSQLiteBackup(t *testing.T) {
	sourceDir := t.TempDir()
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	sourceSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)
	if _, err := sourceDB.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity source: %v", err)
	}
	if err := sourceDB.SetSystemConfig("node_prefix", "TDM65"); err != nil {
		t.Fatalf("SetSystemConfig source node_prefix: %v", err)
	}
	created, err := sourceSvc.Create(models.Soldier{FirstName: "Source", LastName: "Soldier"})
	if err != nil {
		t.Fatalf("Create source soldier: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "current.ddbak")
	if _, err := sourceBackupSvc.Export(backupPath, sourceDir); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if err := sourceDB.Close(); err != nil {
		t.Fatalf("Close sourceDB: %v", err)
	}

	localDB := newTestDB(t)
	localSvc := NewSoldierService(localDB)
	localBackupSvc := NewBackupService(localDB, localSvc)
	if _, err := localDB.ConfigureUserIdentity("Laura", "Jane", "Wilson", 1904); err != nil {
		t.Fatalf("ConfigureUserIdentity local: %v", err)
	}

	restoreDir := t.TempDir()
	if _, err := localBackupSvc.Import(backupPath, restoreDir); err != nil {
		t.Fatalf("Import: %v", err)
	}

	reopened, err := openExistingTestDB(restoreDir)
	if err != nil {
		t.Fatalf("openExistingTestDB: %v", err)
	}
	defer reopened.Close()

	identity, err := reopened.UserIdentity()
	if err != nil {
		t.Fatalf("UserIdentity: %v", err)
	}
	if identity.NodePrefix != "LJW04" || identity.FirstName != "Laura" || identity.LastName != "Wilson" {
		t.Fatalf("restored identity = %#v", identity)
	}
	nextID, err := reopened.NextDXDID()
	if err != nil {
		t.Fatalf("NextDXDID: %v", err)
	}
	if nextID != "LJW04-00002" {
		t.Fatalf("next ID = %q", nextID)
	}
	restoreSvc := NewSoldierService(reopened)
	results, total, err := restoreSvc.SearchPage(created.DisplayID, 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("restored search total=%d len=%d", total, len(results))
	}
}

func TestBackupService_ImportLegacySQLiteKeepsHistoricalRecordsButUsesLocalIdentity(t *testing.T) {
	localDB := newTestDB(t)
	localSvc := NewSoldierService(localDB)
	localBackupSvc := NewBackupService(localDB, localSvc)
	if _, err := localDB.ConfigureUserIdentity("Laura", "Jane", "Wilson", 1904); err != nil {
		t.Fatalf("ConfigureUserIdentity local: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "legacy-sqlite.ddbak")
	legacyDBPath := filepath.Join(t.TempDir(), db.FileName)
	createLegacySchemaV1DB(t, legacyDBPath)

	file, err := os.Create(backupPath)
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	zipWriter := zip.NewWriter(file)
	manifest := BackupManifest{
		Format:        backupFormatName,
		Version:       buildinfo.BackupFormatVersion,
		ArchiveKind:   archiveKindBackup,
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: 1,
		CreatedAt:     "2026-05-15T18:41:06-05:00",
		DataFormat:    "sqlite",
		DatabaseFile:  "data/dixiedata.db",
		ImageRoot:     "images/",
		Soldiers:      1,
		Records:       1,
		Images:        0,
	}
	if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := addBackupFile(zipWriter, manifest.DatabaseFile, legacyDBPath); err != nil {
		t.Fatalf("add database: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	restoreDir := t.TempDir()
	if _, err := localBackupSvc.Import(backupPath, restoreDir); err != nil {
		t.Fatalf("Import: %v", err)
	}

	reopened, err := openExistingTestDB(restoreDir)
	if err != nil {
		t.Fatalf("openExistingTestDB: %v", err)
	}
	defer reopened.Close()

	identity, err := reopened.UserIdentity()
	if err != nil {
		t.Fatalf("UserIdentity: %v", err)
	}
	if identity.NodePrefix != "LJW04" || identity.FirstName != "Laura" || identity.LastName != "Wilson" {
		t.Fatalf("local identity should be restored after legacy import, got %#v", identity)
	}
	nextID, err := reopened.NextDXDID()
	if err != nil {
		t.Fatalf("NextDXDID: %v", err)
	}
	if nextID != "LJW04-00002" {
		t.Fatalf("next ID = %q", nextID)
	}

	restoreSvc := NewSoldierService(reopened)
	results, total, err := restoreSvc.SearchPage("DXD-00001", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage legacy record: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("expected imported legacy record to keep its historical display ID, total=%d len=%d", total, len(results))
	}
	firstCreated, err := restoreSvc.Create(models.Soldier{FirstName: "First", LastName: "Added"})
	if err != nil {
		t.Fatalf("Create first added soldier: %v", err)
	}
	if firstCreated.DisplayID != "LJW04-00002" {
		t.Fatalf("first created display ID = %q", firstCreated.DisplayID)
	}
	secondCreated, err := restoreSvc.Create(models.Soldier{FirstName: "Second", LastName: "Added"})
	if err != nil {
		t.Fatalf("Create second added soldier: %v", err)
	}
	if secondCreated.DisplayID != "LJW04-00003" {
		t.Fatalf("second created display ID = %q", secondCreated.DisplayID)
	}
}

func TestBackupService_ImportSharedBackupMergesContents(t *testing.T) {
	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	targetSvc := NewSoldierService(targetDB)
	backupSvc := NewBackupService(targetDB, targetSvc)

	shared, err := targetSvc.Create(models.Soldier{
		FirstName: "Base",
		LastName:  "Shared",
		Notes:     "local",
	})
	if err != nil {
		t.Fatalf("Create target soldier: %v", err)
	}

	sourceDir := t.TempDir()
	if err := targetDB.SnapshotTo(db.Path(sourceDir)); err != nil {
		t.Fatalf("SnapshotTo source: %v", err)
	}
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	sourceSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)

	sharedSource, err := sourceSvc.GetByID(shared.ID)
	if err != nil {
		t.Fatalf("GetByID source shared: %v", err)
	}
	if err := sourceSvc.Update(*sharedSource); err != nil {
		t.Fatalf("Update source shared: %v", err)
	}

	imported, err := sourceSvc.Create(models.Soldier{
		FirstName: "Imported",
		LastName:  "Only",
	})
	if err != nil {
		t.Fatalf("Create imported soldier: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "shared-backup.zip")
	if _, err := sourceBackupSvc.ExportShared(backupPath, sourceDir); err != nil {
		t.Fatalf("Export shared backup: %v", err)
	}

	summary, err := backupSvc.ImportSharedBackup(backupPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup: %v", err)
	}
	if summary.SoldiersInserted != 1 || summary.SoldiersUpdated != 1 {
		t.Fatalf("unexpected merge summary: %#v", summary)
	}
	if summary.PendingConflicts != 0 {
		t.Fatalf("expected no pending conflicts, got %#v", summary)
	}
	if summary.LogPath == "" {
		t.Fatalf("expected merge log path in summary: %#v", summary)
	}
	if _, err := os.Stat(summary.LogPath); err != nil {
		t.Fatalf("merge log missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "logs", "shared-merge-latest.log")); err != nil {
		t.Fatalf("latest merge log missing: %v", err)
	}

	results, total, err := targetSvc.SearchPage("Imported", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage imported: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("imported search total=%d len=%d", total, len(results))
	}
	if results[0].SyncID != imported.SyncID {
		t.Fatalf("imported sync mismatch: got %q want %q", results[0].SyncID, imported.SyncID)
	}

	merged, err := targetSvc.GetByID(shared.ID)
	if err != nil {
		t.Fatalf("GetByID merged shared: %v", err)
	}
	if merged.Notes != "local" || merged.BuriedIn != "" {
		t.Fatalf("shared soldier not updated: %#v", merged)
	}
	if len(merged.Records) != 0 {
		t.Fatalf("shared records not preserved: %#v", merged.Records)
	}
	if len(merged.Images) != 0 {
		t.Fatalf("shared images not preserved: %#v", merged.Images)
	}
}

func TestBackupService_ImportSharedBackupKeepsLocalIdentity(t *testing.T) {
	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	if _, err := targetDB.ConfigureUserIdentity("Laura", "Jane", "Wilson", 1904); err != nil {
		t.Fatalf("ConfigureUserIdentity target: %v", err)
	}
	targetSvc := NewSoldierService(targetDB)
	targetBackupSvc := NewBackupService(targetDB, targetSvc)

	sourceDir := t.TempDir()
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	if _, err := sourceDB.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity source: %v", err)
	}
	sourceSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)
	if _, err := sourceSvc.Create(models.Soldier{FirstName: "Shared", LastName: "Soldier"}); err != nil {
		t.Fatalf("Create source soldier: %v", err)
	}
	sharedPath := filepath.Join(t.TempDir(), "shared.ddshare")
	if _, err := sourceBackupSvc.ExportShared(sharedPath, sourceDir); err != nil {
		t.Fatalf("ExportShared: %v", err)
	}

	if _, err := targetBackupSvc.ImportSharedBackup(sharedPath, targetDir); err != nil {
		t.Fatalf("ImportSharedBackup: %v", err)
	}

	identity, err := targetDB.UserIdentity()
	if err != nil {
		t.Fatalf("UserIdentity: %v", err)
	}
	if identity.NodePrefix != "LJW04" || identity.FirstName != "Laura" || identity.LastName != "Wilson" {
		t.Fatalf("target identity = %#v", identity)
	}
	nextID, err := targetDB.NextDXDID()
	if err != nil {
		t.Fatalf("NextDXDID: %v", err)
	}
	if nextID != "LJW04-00002" {
		t.Fatalf("next ID = %q", nextID)
	}
}

func TestBackupService_ImportSharedBackupStagesConflictAndResolvesShared(t *testing.T) {
	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	targetSvc := NewSoldierService(targetDB)
	backupSvc := NewBackupService(targetDB, targetSvc)
	if _, err := targetDB.ConfigureUserIdentity("Laura", "Jane", "Wilson", 1904); err != nil {
		t.Fatalf("ConfigureUserIdentity target: %v", err)
	}

	shared, err := targetSvc.Create(models.Soldier{
		FirstName: "Base",
		LastName:  "Shared",
		Notes:     "local",
	})
	if err != nil {
		t.Fatalf("Create target soldier: %v", err)
	}

	sourceDir := t.TempDir()
	if err := targetDB.SnapshotTo(db.Path(sourceDir)); err != nil {
		t.Fatalf("SnapshotTo source: %v", err)
	}
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	sourceSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)
	if _, err := sourceDB.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity source: %v", err)
	}
	if _, err := sourceDB.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity source: %v", err)
	}

	sharedSource, err := sourceSvc.GetByID(shared.ID)
	if err != nil {
		t.Fatalf("GetByID source shared: %v", err)
	}
	sharedSource.Notes = "remote"
	sharedSource.BuriedIn = "Merged Cemetery"
	sharedSource.Records = append(sharedSource.Records, models.Record{
		RecordType: "Roster",
		AppID:      "APP-SHARED",
		Details:    "Merged record",
	})
	if err := sourceSvc.Update(*sharedSource); err != nil {
		t.Fatalf("Update source shared: %v", err)
	}
	imagePath := filepath.Join(sourceDir, "images", "shared", "merged.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll source image: %v", err)
	}
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile source image: %v", err)
	}
	if err := sourceSvc.AddImage(sharedSource.ID, "merged.png", `images\shared\merged.png`, "Merged"); err != nil {
		t.Fatalf("AddImage source shared: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "shared-conflict.ddshare")
	if _, err := sourceBackupSvc.ExportShared(backupPath, sourceDir); err != nil {
		t.Fatalf("Export shared backup: %v", err)
	}

	summary, err := backupSvc.ImportSharedBackup(backupPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup: %v", err)
	}
	if summary.PendingConflicts != 1 || summary.SoldiersInserted != 0 || summary.SoldiersUpdated != 0 {
		t.Fatalf("unexpected staged conflict summary: %#v", summary)
	}

	conflicts, err := backupSvc.PendingMergeConflicts()
	if err != nil {
		t.Fatalf("PendingMergeConflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 pending conflict, got %d", len(conflicts))
	}
	if conflicts[0].LocalSoldier == nil || conflicts[0].LocalSoldier.Notes != "local" || conflicts[0].SourceSoldier.Notes != "remote" {
		t.Fatalf("unexpected conflict payload: %#v", conflicts[0])
	}

	unchanged, err := targetSvc.GetByID(shared.ID)
	if err != nil {
		t.Fatalf("GetByID unchanged shared: %v", err)
	}
	if unchanged.Notes != "local" || len(unchanged.Records) != 0 || len(unchanged.Images) != 0 {
		t.Fatalf("conflicted soldier changed before review: %#v", unchanged)
	}

	if err := backupSvc.ResolveMergeConflict(conflicts[0].ID, "use-shared", targetDir); err != nil {
		t.Fatalf("ResolveMergeConflict: %v", err)
	}
	merged, err := targetSvc.GetByID(shared.ID)
	if err != nil {
		t.Fatalf("GetByID merged shared: %v", err)
	}
	if merged.Notes != "remote" || merged.BuriedIn != "Merged Cemetery" {
		t.Fatalf("shared soldier not updated after review: %#v", merged)
	}
	if len(merged.Records) != 1 || merged.Records[0].AppID != "APP-SHARED" {
		t.Fatalf("shared records not merged after review: %#v", merged.Records)
	}
	if len(merged.Images) != 1 || merged.Images[0].Caption != "Merged" {
		t.Fatalf("shared images not merged after review: %#v", merged.Images)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "images", "shared", "merged.png")); err != nil {
		t.Fatalf("merged image missing after review: %v", err)
	}

	pending, err := backupSvc.PendingMergeConflicts()
	if err != nil {
		t.Fatalf("PendingMergeConflicts after resolve: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending conflicts after resolve, got %d", len(pending))
	}
}

func TestBackupService_ResolveDisplayIDCollisionKeepBoth(t *testing.T) {
	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	if _, err := targetDB.ConfigureUserIdentity("John", "Charles", "Morgan", 1887); err != nil {
		t.Fatalf("ConfigureUserIdentity target: %v", err)
	}
	targetSvc := NewSoldierService(targetDB)
	backupSvc := NewBackupService(targetDB, targetSvc)

	local, err := targetSvc.Create(models.Soldier{
		DisplayID: "DXD-00001",
		FirstName: "Thomas",
		LastName:  "Lewis",
		Unit:      "12th Georgia Infantry",
	})
	if err != nil {
		t.Fatalf("Create local soldier: %v", err)
	}
	if _, err := targetDB.Conn().Exec(`UPDATE soldiers SET added_by = ?, last_edited_by = ? WHERE id = ?`, "L. Wilson", "L. Wilson", local.ID); err != nil {
		t.Fatalf("seed local attribution: %v", err)
	}

	sourceDir := t.TempDir()
	if err := targetDB.SnapshotTo(db.Path(sourceDir)); err != nil {
		t.Fatalf("SnapshotTo source: %v", err)
	}
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	if _, err := sourceDB.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity source: %v", err)
	}
	sourceSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)

	if err := sourceSvc.Delete(local.ID); err != nil {
		t.Fatalf("Delete source local: %v", err)
	}
	imported, err := sourceSvc.Create(models.Soldier{
		DisplayID: "DXD-00001",
		FirstName: "Andrew",
		LastName:  "Morris",
		Unit:      "4th Alabama Infantry",
	})
	if err != nil {
		t.Fatalf("Create imported soldier: %v", err)
	}
	if _, err := sourceDB.Conn().Exec(`UPDATE soldiers SET added_by = ?, last_edited_by = ? WHERE id = ?`, "S. Carter", "S. Carter", imported.ID); err != nil {
		t.Fatalf("seed shared attribution: %v", err)
	}

	sharedPath := filepath.Join(t.TempDir(), "collision.ddshare")
	if _, err := sourceBackupSvc.ExportShared(sharedPath, sourceDir); err != nil {
		t.Fatalf("ExportShared: %v", err)
	}

	summary, err := backupSvc.ImportSharedBackup(sharedPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup: %v", err)
	}
	if summary.PendingConflicts > 0 {
		conflicts, err := backupSvc.PendingMergeConflicts()
		if err != nil {
			t.Fatalf("PendingMergeConflicts: %v", err)
		}
		if len(conflicts) != 1 || conflicts[0].ConflictType != "display-id-collision" {
			t.Fatalf("unexpected conflicts: %#v", conflicts)
		}
		if err := backupSvc.ResolveMergeConflict(conflicts[0].ID, "keep-both", targetDir); err != nil {
			t.Fatalf("ResolveMergeConflict keep-both: %v", err)
		}
	}

	results, total, err := targetSvc.List(1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(results) != 2 {
		t.Fatalf("expected two records after keep-both, total=%d len=%d", total, len(results))
	}

	var keptLocal, keptShared *models.Soldier
	for _, result := range results {
		full, err := targetSvc.GetByID(result.ID)
		if err != nil {
			t.Fatalf("GetByID result %d: %v", result.ID, err)
		}
		switch full.SyncID {
		case local.SyncID:
			keptLocal = full
		case imported.SyncID:
			keptShared = full
		}
	}
	if keptLocal == nil || keptShared == nil {
		t.Fatalf("missing expected local/shared records after keep-both")
	}
	if keptLocal.DisplayID != "DXD-00001" {
		t.Fatalf("local display ID changed unexpectedly: %#v", keptLocal)
	}
	if keptLocal.AddedBy != "L. Wilson" && keptLocal.AddedBy != "LJW04" {
		t.Fatalf("local attribution changed unexpectedly: %#v", keptLocal)
	}
	if keptShared.DisplayID == "DXD-00001" {
		t.Fatalf("shared record did not receive a unique display ID: %#v", keptShared)
	}
	if !regexp.MustCompile(`^JCM87-\d{5}$`).MatchString(keptShared.DisplayID) {
		t.Fatalf("shared record did not receive a fresh generated display ID: %#v", keptShared)
	}
	if strings.Contains(keptShared.DisplayID, "-DXD-") {
		t.Fatalf("shared record kept a wrapped legacy ID instead of a clean local ID: %#v", keptShared)
	}
	if keptShared.FirstName != "Andrew" || keptShared.Unit != "4th Alabama Infantry" {
		t.Fatalf("shared record not preserved after keep-both: %#v", keptShared)
	}
	if keptShared.AddedBy != "S. Carter" || keptShared.LastEditedBy != "S. Carter" {
		t.Fatalf("shared attribution not preserved after keep-both: %#v", keptShared)
	}
	if !keptLocal.NeedsReview || !keptShared.NeedsReview {
		t.Fatalf("keep-both should flag both records for review: local=%#v shared=%#v", keptLocal, keptShared)
	}
	if strings.TrimSpace(keptLocal.ReviewReason) == "" || strings.TrimSpace(keptShared.ReviewReason) == "" {
		t.Fatalf("keep-both should attach review reasons: local=%#v shared=%#v", keptLocal, keptShared)
	}
	counts, err := targetSvc.ArchiveCounts()
	if err != nil {
		t.Fatalf("ArchiveCounts: %v", err)
	}
	if counts.TotalSoldiers != 2 || counts.TotalWivesWidows != 0 {
		t.Fatalf("keep-both should preserve archive counts: %#v", counts)
	}
}

func TestBackupService_ImportSharedBackupStagesHumanDuplicateConflict(t *testing.T) {
	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	if _, err := targetDB.ConfigureUserIdentity("John", "Charles", "Morgan", 1887); err != nil {
		t.Fatalf("ConfigureUserIdentity target: %v", err)
	}
	targetSvc := NewSoldierService(targetDB)
	backupSvc := NewBackupService(targetDB, targetSvc)

	local, err := targetSvc.Create(models.Soldier{
		DisplayID: "JCM87-00001",
		FirstName: "Andrew",
		LastName:  "Morris",
		BirthDate: "01/13/1842",
		Unit:      "4th Alabama Infantry",
		Notes:     "local",
	})
	if err != nil {
		t.Fatalf("Create local soldier: %v", err)
	}

	sourceDir := t.TempDir()
	if err := targetDB.SnapshotTo(db.Path(sourceDir)); err != nil {
		t.Fatalf("SnapshotTo source: %v", err)
	}
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	if _, err := sourceDB.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity source: %v", err)
	}
	sourceSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)

	if err := sourceSvc.Delete(local.ID); err != nil {
		t.Fatalf("Delete source local: %v", err)
	}
	imported, err := sourceSvc.Create(models.Soldier{
		DisplayID: "STC38-00044",
		FirstName: "Andrew",
		LastName:  "Morris",
		BirthDate: "01/13/1842",
		Unit:      "4th Alabama Infantry",
		Notes:     "shared",
	})
	if err != nil {
		t.Fatalf("Create imported soldier: %v", err)
	}
	if _, err := sourceDB.Conn().Exec(`UPDATE soldiers SET added_by = ?, last_edited_by = ? WHERE id = ?`, "S. Carter", "S. Carter", imported.ID); err != nil {
		t.Fatalf("seed shared attribution: %v", err)
	}

	sharedPath := filepath.Join(t.TempDir(), "human-duplicate.ddshare")
	if _, err := sourceBackupSvc.ExportShared(sharedPath, sourceDir); err != nil {
		t.Fatalf("ExportShared: %v", err)
	}

	summary, err := backupSvc.ImportSharedBackup(sharedPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup: %v", err)
	}
	if summary.PendingConflicts != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	conflicts, err := backupSvc.PendingMergeConflicts()
	if err != nil {
		t.Fatalf("PendingMergeConflicts: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].ConflictType != "human-duplicate" {
		t.Fatalf("unexpected human duplicate conflicts: %#v", conflicts)
	}
	if err := backupSvc.ResolveMergeConflict(conflicts[0].ID, "use-shared", targetDir); err != nil {
		t.Fatalf("ResolveMergeConflict use-shared: %v", err)
	}

	results, total, err := targetSvc.SearchPage("Andrew Morris", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("expected one merged human record, total=%d len=%d", total, len(results))
	}
	merged, err := targetSvc.GetByID(results[0].ID)
	if err != nil {
		t.Fatalf("GetByID merged: %v", err)
	}
	if merged.DisplayID != "JCM87-00001" {
		t.Fatalf("human duplicate should keep the local display ID, got %#v", merged)
	}
	if merged.Notes != "shared" {
		t.Fatalf("human duplicate should apply shared content, got %#v", merged)
	}
	if merged.AddedBy != local.AddedBy {
		t.Fatalf("human duplicate should preserve local created-by attribution, got %#v", merged)
	}
}

func TestBackupService_ConflictLedger(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	backupSvc := NewBackupService(d, soldierSvc)

	created, err := soldierSvc.Create(models.Soldier{
		DisplayID: "LED-0001",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "1st Texas Infantry",
		PensionID: "P-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	localJSON, err := marshalMergeReviewSnapshot(mergeReviewSnapshot{
		Soldier: models.Soldier{
			DisplayID: "LED-0001",
			FirstName: "Andrew",
			LastName:  "Cole",
			Unit:      "1st Texas Infantry",
			PensionID: "P-1",
		},
	})
	if err != nil {
		t.Fatalf("marshal local: %v", err)
	}
	sourceJSON, err := marshalMergeReviewSnapshot(mergeReviewSnapshot{
		Soldier: models.Soldier{
			DisplayID: "SRC-0001",
			FirstName: "Andrew",
			LastName:  "Cole",
			Unit:      "2nd Texas Infantry",
			PensionID: "P-9",
		},
	})
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}

	if _, err := d.Conn().Exec(`
		INSERT INTO merge_review_sessions (id, archive_path, source_root, status, updated_at)
		VALUES ('session-ledger', 'ledger.ddshare', 'C:\\source', 'open', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := d.Conn().Exec(`
		INSERT INTO merge_review_conflicts (session_id, conflict_type, reason, soldier_sync_id, local_soldier_id, local_display_id, source_display_id, local_data, source_data, resolution, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, "session-ledger", "soldier-update", "Shared archive changed unit and pension ID.", created.SyncID, created.ID, created.DisplayID, "SRC-0001", localJSON, sourceJSON, "keep-local"); err != nil {
		t.Fatalf("insert conflict: %v", err)
	}

	ledger, err := backupSvc.ConflictLedger(created.ID)
	if err != nil {
		t.Fatalf("ConflictLedger: %v", err)
	}
	if ledger.ResolvedCount != 1 || len(ledger.Entries) != 1 {
		t.Fatalf("unexpected ledger counts: %#v", ledger)
	}
	entry := ledger.Entries[0]
	if entry.SourceDisplayID != "SRC-0001" || entry.LocalSnapshot.DisplayID != "LED-0001" || entry.SourceSnapshot.PensionID != "P-9" {
		t.Fatalf("unexpected ledger entry snapshots: %#v", entry)
	}
	if !strings.Contains(strings.Join(entry.DifferenceFields, ","), "unit") || !strings.Contains(strings.Join(entry.DifferenceFields, ","), "pension ID") {
		t.Fatalf("expected differing fields in ledger entry: %#v", entry.DifferenceFields)
	}
}

func TestBackupService_ImportSharedBackupMigratesLegacySQLite(t *testing.T) {
	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	targetSvc := NewSoldierService(targetDB)
	backupSvc := NewBackupService(targetDB, targetSvc)

	backupPath := filepath.Join(t.TempDir(), "legacy-shared-backup.zip")
	legacyDBPath := filepath.Join(t.TempDir(), db.FileName)
	createLegacySchemaV1DB(t, legacyDBPath)

	file, err := os.Create(backupPath)
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	zipWriter := zip.NewWriter(file)
	manifest := BackupManifest{
		Format:        backupFormatName,
		Version:       buildinfo.BackupFormatVersion,
		ArchiveKind:   archiveKindShared,
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: 1,
		CreatedAt:     "2026-05-15T18:41:06-05:00",
		DataFormat:    "sqlite",
		DatabaseFile:  "data/dixiedata.db",
		ImageRoot:     "images/",
		Soldiers:      1,
		Records:       1,
		Images:        0,
	}
	if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := addBackupFile(zipWriter, manifest.DatabaseFile, legacyDBPath); err != nil {
		t.Fatalf("add database: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	summary, err := backupSvc.ImportSharedBackup(backupPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup: %v", err)
	}
	if summary.SoldiersInserted != 1 {
		t.Fatalf("unexpected legacy summary: %#v", summary)
	}

	results, total, err := targetSvc.SearchPage("00001", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("legacy search total=%d len=%d", total, len(results))
	}
	restored, err := targetSvc.GetByID(results[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if restored.DisplayID != "DXD-00001" || restored.BirthDate != "01/13/1842" || restored.SyncID == "" {
		t.Fatalf("legacy shared import missing migrated fields: %#v", restored)
	}
}

func TestBackupService_ImportSharedBackupRejectsBackupArchive(t *testing.T) {
	sourceDir := t.TempDir()
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	sourceSvc := NewSoldierService(sourceDB)
	backupSvc := NewBackupService(sourceDB, sourceSvc)

	if _, err := sourceSvc.Create(models.Soldier{FirstName: "Robert", LastName: "Lee"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "backup.ddbak")
	if _, err := backupSvc.Export(backupPath, sourceDir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	targetSvc := NewSoldierService(targetDB)
	targetBackupSvc := NewBackupService(targetDB, targetSvc)

	if _, err := targetBackupSvc.ImportSharedBackup(backupPath, targetDir); err == nil || !strings.Contains(err.Error(), "shared archive") {
		t.Fatalf("expected shared archive rejection, got %v", err)
	}
}

func TestBackupService_ImportSharedBackupRejectsMissingSQLiteImage(t *testing.T) {
	sourceDir := t.TempDir()
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	sourceSvc := NewSoldierService(sourceDB)
	backupSvc := NewBackupService(sourceDB, sourceSvc)

	soldier, err := sourceSvc.Create(models.Soldier{DisplayID: "TDM65-MISSING-IMAGE", FirstName: "Missing", LastName: "Image"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sourceSvc.AddImage(soldier.ID, "portrait.png", `images\missing-image\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	sharedPath := filepath.Join(t.TempDir(), "missing-image.ddshare")
	if _, err := backupSvc.ExportShared(sharedPath, sourceDir); err != nil {
		t.Fatalf("ExportShared: %v", err)
	}

	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	targetSvc := NewSoldierService(targetDB)
	targetBackupSvc := NewBackupService(targetDB, targetSvc)

	if _, err := targetBackupSvc.ImportSharedBackup(sharedPath, targetDir); err == nil || !strings.Contains(err.Error(), `missing image file images\missing-image\portrait.png`) {
		t.Fatalf("expected missing-image rejection, got %v", err)
	}
}

func TestBackupService_ImportRejectsSharedArchive(t *testing.T) {
	sourceDir := t.TempDir()
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	sourceSvc := NewSoldierService(sourceDB)
	backupSvc := NewBackupService(sourceDB, sourceSvc)

	if _, err := sourceSvc.Create(models.Soldier{FirstName: "Stonewall", LastName: "Jackson"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	sharedPath := filepath.Join(t.TempDir(), "shared.ddshare")
	if _, err := backupSvc.ExportShared(sharedPath, sourceDir); err != nil {
		t.Fatalf("ExportShared: %v", err)
	}

	restoreDir := t.TempDir()
	if _, err := backupSvc.Import(sharedPath, restoreDir); err == nil || !strings.Contains(err.Error(), "backup archive") {
		t.Fatalf("expected backup archive rejection, got %v", err)
	}
}
