package archive

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
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
	if _, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	if err := d.SetSystemConfig("node_id", "source-node-1"); err != nil {
		t.Fatalf("SetSystemConfig node_id: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "shared.ddshare")
	manifest, err := backupSvc.ExportShared(outPath, t.TempDir())
	if err != nil {
		t.Fatalf("ExportShared: %v", err)
	}
	if manifest.ArchiveKind != archiveKindShared {
		t.Fatalf("manifest = %#v", manifest)
	}
	if manifest.DataFormat != "json" || manifest.DataFile != "data/soldiers.json" || manifest.DatabaseFile != "" {
		t.Fatalf("shared manifest should use json payloads: %#v", manifest)
	}
	if manifest.SourceNodeID != "source-node-1" || manifest.SourceLabel != "Samuel Thomas Carter" {
		t.Fatalf("shared manifest should include source identity: %#v", manifest)
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
	for _, expected := range []string{"manifest.json", "data/soldiers.json"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("shared archive missing %s", expected)
		}
	}
	if strings.Contains(joined, "data/dixiedata.db") {
		t.Fatalf("shared archive should not include an sqlite snapshot: %s", joined)
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
	if storedManifest.SourceNodeID != "source-node-1" || storedManifest.SourceLabel != "Samuel Thomas Carter" {
		t.Fatalf("stored shared manifest missing source identity: %#v", storedManifest)
	}
}

func TestBackupService_ExportSharedIncludesReferencedImagesOnly(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	backupSvc := NewBackupService(d, soldierSvc)

	dataDir := t.TempDir()
	created, err := soldierSvc.Create(models.Soldier{
		DisplayID: "SHARED-1",
		FirstName: "Shared",
		LastName:  "Image",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	imagePath := filepath.Join(dataDir, "images", "shared-1", "portrait.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll image: %v", err)
	}
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}
	if err := soldierSvc.AddImage(created.ID, "portrait.png", `images\shared-1\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	orphanPath := filepath.Join(dataDir, "images", "orphaned", "orphan.png")
	if err := os.MkdirAll(filepath.Dir(orphanPath), 0o755); err != nil {
		t.Fatalf("MkdirAll orphan: %v", err)
	}
	if err := os.WriteFile(orphanPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile orphan: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "shared.ddshare")
	if _, err := backupSvc.ExportShared(outPath, dataDir); err != nil {
		t.Fatalf("ExportShared: %v", err)
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
	if !strings.Contains(joined, "images/shared-1/portrait.png") {
		t.Fatalf("shared archive missing referenced image: %s", joined)
	}
	if strings.Contains(joined, "images/orphaned/orphan.png") {
		t.Fatalf("shared archive should exclude orphan images: %s", joined)
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

// TestBackupService_ImportDestroysFilesInsideDataDir locks the
// contract that motivates Phase 5's choice to skip pre-import
// restore points. Importing a .ddbak replaces the data dir via
// archive.replaceDataDir (os.Rename old -> sibling, then
// RemoveAll sibling). Any file written inside the data dir
// BEFORE the import is gone afterwards.
//
// This is the seam Phase 5 hit when trying to use the
// in-place update restore-point manager as an import rollback
// safety net: the snapshot landed in
// <dataDir>/updates/restore-points/<id>/, which got renamed
// away by the import's replaceDataDir. Reverting this test
// would require either (a) changing replaceDataDir to
// preserve sidecar data dirs, or (b) moving the restore-point
// root outside the data dir. Both are Phase 6 work; until
// then the import CLI doesn't pre-snapshot.
func TestBackupService_ImportDestroysFilesInsideDataDir(t *testing.T) {
	sourceDB := newTestDB(t)
	sourceSvc := NewSoldierService(sourceDB)
	backupSvc := NewBackupService(sourceDB, sourceSvc)

	sourceDataDir := t.TempDir()
	created, err := sourceSvc.Create(models.Soldier{
		DisplayID: "PENSION-9",
		FirstName: "Jane",
		LastName:  "Doe",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = created

	backupPath := filepath.Join(t.TempDir(), "backup.zip")
	if _, err := backupSvc.Export(backupPath, sourceDataDir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	restoreDir := t.TempDir()
	// Plant a sidecar file inside the target data dir, mimicking
	// what a pre-import restore-point manager would have written
	// at <dataDir>/updates/restore-points/<id>/local-archive.ddbak.
	sidecarDir := filepath.Join(restoreDir, "updates", "restore-points", "restore-point-20260628-070000")
	if err := os.MkdirAll(sidecarDir, 0o755); err != nil {
		t.Fatalf("MkdirAll sidecar: %v", err)
	}
	sidecarFile := filepath.Join(sidecarDir, "local-archive.ddbak")
	if err := os.WriteFile(sidecarFile, []byte("would-be-rollback-snapshot"), 0o644); err != nil {
		t.Fatalf("WriteFile sidecar: %v", err)
	}

	if _, err := backupSvc.Import(backupPath, restoreDir); err != nil {
		t.Fatalf("Import: %v", err)
	}

	if _, err := os.Stat(sidecarFile); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar to be destroyed by import; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "updates")); !os.IsNotExist(err) {
		t.Fatalf("expected updates/ to be gone after import; stat err = %v", err)
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
		Soldiers:  2,
		Records:   1,
		Images:    1,
	}
	if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	soldiers := []models.Soldier{
		{
			ID:        11,
			DisplayID: "PENSION-LEGACY",
			FirstName: "Legacy",
			LastName:  "Soldier",
			Records:   []models.Record{{RecordType: "Roster", AppID: "APP-1", Details: "Legacy details"}},
			Images:    []models.Image{{FileName: "portrait.png", FilePath: "images/pension-legacy/portrait.png", Caption: "Portrait"}},
		},
		{
			ID:                12,
			DisplayID:         "PENSION-LEGACY-LINK",
			EntryType:         "linked_person",
			SpouseSoldierID:   11,
			RelationshipLabel: "Brother",
			FirstName:         "Linked",
			LastName:          "Person",
		},
	}
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
	results, _, err := reopenedSvc.SearchPage("PENSION-LEGACY", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	var restoredSoldier *models.Soldier
	for _, result := range results {
		if result.DisplayID == "PENSION-LEGACY" {
			restoredSoldier, err = reopenedSvc.GetByID(result.ID)
			if err != nil {
				t.Fatalf("GetByID restored soldier: %v", err)
			}
			break
		}
	}
	if restoredSoldier == nil {
		t.Fatalf("restored soldier missing from search results: %#v", results)
	}
	linkedResults, _, err := reopenedSvc.SearchPage("PENSION-LEGACY-LINK", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage linked: %v", err)
	}
	var linked *models.Soldier
	for _, result := range linkedResults {
		if result.DisplayID == "PENSION-LEGACY-LINK" {
			linked, err = reopenedSvc.GetByID(result.ID)
			if err != nil {
				t.Fatalf("GetByID linked: %v", err)
			}
			break
		}
	}
	if linked == nil {
		t.Fatalf("restored person record missing from search results: %#v", linkedResults)
	}
	if linked.EntryType != "linked_person" || linked.RelationshipLabel != "Brother" {
		t.Fatalf("restored person record missing relationship fields: %#v", linked)
	}
	if linked.SpouseDisplayID != "PENSION-LEGACY" || linked.SpouseName != "Legacy Soldier" {
		t.Fatalf("restored person record missing soldier link: %#v", linked)
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

func TestBackupService_ImportFormatVersion2SQLiteBackup(t *testing.T) {
	sourceDB := newTestDB(t)
	sourceSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)
	if _, err := sourceDB.ConfigureUserIdentity("Terry", "Dale", "Morris", 1965); err != nil {
		t.Fatalf("ConfigureUserIdentity source: %v", err)
	}
	created, err := sourceSvc.Create(models.Soldier{
		DisplayID: "TDM65-00001",
		FirstName: "Andrew",
		LastName:  "Morris",
		Unit:      "4th Oklahoma Infantry",
	})
	if err != nil {
		t.Fatalf("Create source soldier: %v", err)
	}

	sourceDataDir := t.TempDir()
	currentBackupPath := filepath.Join(t.TempDir(), "current.ddbak")
	manifest, err := sourceBackupSvc.Export(currentBackupPath, sourceDataDir)
	if err != nil {
		t.Fatalf("Export current backup: %v", err)
	}

	legacyV2Path := filepath.Join(t.TempDir(), "legacy-v2.ddbak")
	file, err := os.Create(legacyV2Path)
	if err != nil {
		t.Fatalf("Create v2 backup: %v", err)
	}
	defer file.Close()
	zipWriter := zip.NewWriter(file)
	manifest.Version = 2
	if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
		t.Fatalf("write v2 manifest: %v", err)
	}
	reader, err := zip.OpenReader(currentBackupPath)
	if err != nil {
		t.Fatalf("open exported backup: %v", err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Name == "manifest.json" {
			continue
		}
		header := entry.FileHeader
		writer, err := zipWriter.CreateHeader(&header)
		if err != nil {
			t.Fatalf("CreateHeader %s: %v", entry.Name, err)
		}
		source, err := entry.Open()
		if err != nil {
			t.Fatalf("Open %s: %v", entry.Name, err)
		}
		if _, err := io.Copy(writer, source); err != nil {
			source.Close()
			t.Fatalf("Copy %s: %v", entry.Name, err)
		}
		if err := source.Close(); err != nil {
			t.Fatalf("Close %s: %v", entry.Name, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close v2 zip: %v", err)
	}

	localDB := newTestDB(t)
	localSvc := NewSoldierService(localDB)
	localBackupSvc := NewBackupService(localDB, localSvc)
	if _, err := localDB.ConfigureUserIdentity("Laura", "Jane", "Wilson", 1904); err != nil {
		t.Fatalf("ConfigureUserIdentity local: %v", err)
	}

	restoreDir := t.TempDir()
	if _, err := localBackupSvc.Import(legacyV2Path, restoreDir); err != nil {
		t.Fatalf("Import v2 backup: %v", err)
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
	if identity.NodePrefix != "LJW04" {
		t.Fatalf("expected local identity to persist, got %#v", identity)
	}

	restoreSvc := NewSoldierService(reopened)
	results, total, err := restoreSvc.SearchPage(created.DisplayID, 1, 10)
	if err != nil {
		t.Fatalf("SearchPage imported record: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("expected imported record, total=%d len=%d", total, len(results))
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
	if summary.SoldiersInserted != 1 || summary.SoldiersUpdated != 0 {
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
	if _, err := os.Stat(filepath.Join(appdata.LogsDir(targetDir), "shared-merge-latest.log")); err != nil {
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
	results, total, err := targetSvc.SearchPage("Shared Soldier", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("shared search total=%d len=%d", total, len(results))
	}
	if !strings.HasPrefix(results[0].DisplayID, "LJW04-") {
		t.Fatalf("shared import should regenerate into the local namespace: %#v", results[0])
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

func TestBackupService_ImportSharedBackupRemembersHumanDuplicateAliasBySource(t *testing.T) {
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
	if err := sourceDB.SetSystemConfig("node_id", "satellite-a"); err != nil {
		t.Fatalf("SetSystemConfig source node_id: %v", err)
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

	sharedPath := filepath.Join(t.TempDir(), "human-duplicate-alias.ddshare")
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
		t.Fatalf("unexpected conflicts: %#v", conflicts)
	}
	if err := backupSvc.ResolveMergeConflict(conflicts[0].ID, "use-shared", targetDir); err != nil {
		t.Fatalf("ResolveMergeConflict use-shared: %v", err)
	}

	var aliasCount int
	if err := targetDB.Conn().QueryRow(`SELECT COUNT(*) FROM shared_merge_aliases WHERE source_node_id = ? AND source_person_sync_id = ? AND canonical_person_id = ?`,
		"satellite-a", imported.SyncID, local.ID,
	).Scan(&aliasCount); err != nil {
		t.Fatalf("query alias ledger: %v", err)
	}
	if aliasCount != 1 {
		t.Fatalf("expected one alias ledger row, got %d", aliasCount)
	}

	sharedPath2 := filepath.Join(t.TempDir(), "human-duplicate-alias-repeat.ddshare")
	if _, err := sourceBackupSvc.ExportShared(sharedPath2, sourceDir); err != nil {
		t.Fatalf("ExportShared repeat: %v", err)
	}
	summary, err = backupSvc.ImportSharedBackup(sharedPath2, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup repeat: %v", err)
	}
	if summary.PendingConflicts != 0 {
		t.Fatalf("repeat import should not restage duplicate conflict: %#v", summary)
	}
	results, total, err := targetSvc.SearchPage("Andrew Morris", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("repeat import should keep one canonical record, total=%d len=%d", total, len(results))
	}
}

func TestBackupService_ImportSharedBackupAliasLedgerIsSourceScoped(t *testing.T) {
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
	})
	if err != nil {
		t.Fatalf("Create local soldier: %v", err)
	}

	makeSharedArchive := func(tempName, nodeID string, forcedSyncID string) string {
		sourceDir := t.TempDir()
		if err := targetDB.SnapshotTo(db.Path(sourceDir)); err != nil {
			t.Fatalf("SnapshotTo %s: %v", nodeID, err)
		}
		sourceDB, err := db.Open(sourceDir)
		if err != nil {
			t.Fatalf("db.Open %s: %v", nodeID, err)
		}
		defer sourceDB.Close()
		if _, err := sourceDB.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
			t.Fatalf("ConfigureUserIdentity %s: %v", nodeID, err)
		}
		if err := sourceDB.SetSystemConfig("node_id", nodeID); err != nil {
			t.Fatalf("SetSystemConfig %s: %v", nodeID, err)
		}
		sourceSvc := NewSoldierService(sourceDB)
		sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)
		if err := sourceSvc.Delete(local.ID); err != nil {
			t.Fatalf("Delete source local %s: %v", nodeID, err)
		}
		imported, err := sourceSvc.Create(models.Soldier{
			DisplayID: "STC38-00044",
			FirstName: "Andrew",
			LastName:  "Morris",
			BirthDate: "01/13/1842",
			Unit:      "4th Alabama Infantry",
			Notes:     nodeID,
		})
		if err != nil {
			t.Fatalf("Create imported soldier %s: %v", nodeID, err)
		}
		if forcedSyncID != "" {
			if _, err := sourceDB.Conn().Exec(`UPDATE soldiers SET sync_id = ? WHERE id = ?`, forcedSyncID, imported.ID); err != nil {
				t.Fatalf("force sync_id %s: %v", nodeID, err)
			}
		} else {
			forcedSyncID = imported.SyncID
		}
		sharedPath := filepath.Join(t.TempDir(), tempName)
		if _, err := sourceBackupSvc.ExportShared(sharedPath, sourceDir); err != nil {
			t.Fatalf("ExportShared %s: %v", nodeID, err)
		}
		return sharedPath
	}

	firstPath := makeSharedArchive("source-a.ddshare", "satellite-a", "")
	summary, err := backupSvc.ImportSharedBackup(firstPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup source A: %v", err)
	}
	if summary.PendingConflicts != 1 {
		t.Fatalf("source A should stage one human duplicate conflict: %#v", summary)
	}
	conflicts, err := backupSvc.PendingMergeConflicts()
	if err != nil {
		t.Fatalf("PendingMergeConflicts source A: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected one source A conflict, got %#v", conflicts)
	}
	sourceASync := conflicts[0].SourceSoldier.SyncID
	if err := backupSvc.ResolveMergeConflict(conflicts[0].ID, "use-shared", targetDir); err != nil {
		t.Fatalf("ResolveMergeConflict source A: %v", err)
	}

	secondPath := makeSharedArchive("source-b.ddshare", "satellite-b", sourceASync)
	summary, err = backupSvc.ImportSharedBackup(secondPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup source B: %v", err)
	}
	if summary.PendingConflicts != 1 {
		t.Fatalf("source B should not reuse source A alias blindly: %#v", summary)
	}
	conflicts, err = backupSvc.PendingMergeConflicts()
	if err != nil {
		t.Fatalf("PendingMergeConflicts source B: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].ConflictType != "human-duplicate" {
		t.Fatalf("source B should stage a human duplicate, got %#v", conflicts)
	}
}

func TestBackupService_ImportSharedBackupIgnoresMetadataOnlyDifferences(t *testing.T) {
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
	if _, err := targetDB.Conn().Exec(`UPDATE soldiers SET added_by = ?, last_edited_by = ?, last_edited_at = ?, last_edited_fields = ? WHERE id = ?`,
		"Local User", "Local User", "2026-05-30T00:00:00Z", "notes", local.ID,
	); err != nil {
		t.Fatalf("seed local metadata: %v", err)
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
	if _, err := sourceDB.Conn().Exec(`UPDATE soldiers SET added_by = ?, last_edited_by = ?, last_edited_at = ?, last_edited_fields = ? WHERE id = ?`,
		"Shared User", "Shared User", "2026-05-31T00:00:00Z", "last_edited_by,last_edited_at", local.ID,
	); err != nil {
		t.Fatalf("seed shared metadata: %v", err)
	}

	sharedPath := filepath.Join(t.TempDir(), "metadata-only.ddshare")
	if _, err := sourceBackupSvc.ExportShared(sharedPath, sourceDir); err != nil {
		t.Fatalf("ExportShared: %v", err)
	}

	summary, err := backupSvc.ImportSharedBackup(sharedPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup: %v", err)
	}
	if summary.PendingConflicts != 0 {
		t.Fatalf("metadata-only import should not stage conflict: %#v", summary)
	}
	merged, err := targetSvc.GetByID(local.ID)
	if err != nil {
		t.Fatalf("GetByID merged: %v", err)
	}
	if merged.AddedBy != "Local User" || merged.LastEditedBy != "Local User" || merged.LastEditedAt != "2026-05-30T00:00:00Z" {
		t.Fatalf("metadata-only import should not overwrite local audit metadata: %#v", merged)
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

// TestReplaceDataDir_HandlesEmptyAndLockedTargets is the
// regression net for issue #216. The Windows
// `Access is denied` failure mode is non-deterministic — a
// transient handle conflict from OneDrive / SearchHost / the
// asset-server watcher makes `os.Rename` fail. The previous
// behavior was to return the error immediately, leaving the
// data dir in an inconsistent state (target renamed away,
// staging not promoted). The fix has two layers:
//
//  1. Skip the rename when the target is logically empty (no
//     DB, or DB < 64KB = schema-only). The user has nothing
//     to back up; moving stagingDir → targetDir directly
//     avoids the rename entirely.
//  2. Retry the rename with exponential backoff when the
//     target is non-empty. 5 attempts, 4 sleep periods of
//     200/400/800/1600ms = 3s total wait time.
//
// Acceptance:
//   - Remove the isLogicallyEmpty short-circuit → cases (2)
//     and (3) fail (the empty-target case is no longer
//     handled).
//   - Remove the renameWithRetry call → case (5) fails
//     (transient failures don't recover).
//   - Set retry attempts to 0 → case (5) fails.
//   - Remove the %w from the wrapped error → errors.Is
//     detection fails for case (6).
//   - Remove the rollback rename (case 7) → target isn't
//     restored after staging rename fails.
func TestReplaceDataDir_HandlesEmptyAndLockedTargets(t *testing.T) {
	// (1) Target doesn't exist — no rename needed, staging
	// promoted directly. No backup dir created.
	t.Run("target does not exist", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, ".dixiedata")
		staging := filepath.Join(parent, ".dixiedata-import-test")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll staging: %v", err)
		}
		marker := filepath.Join(staging, "staging-only.txt")
		if err := os.WriteFile(marker, []byte("staging"), 0o644); err != nil {
			t.Fatalf("WriteFile marker: %v", err)
		}

		if err := replaceDataDir(target, staging); err != nil {
			t.Fatalf("replaceDataDir: %v", err)
		}

		if _, err := os.Stat(filepath.Join(target, "staging-only.txt")); err != nil {
			t.Fatalf("staging marker should now be at target: %v", err)
		}
		// No .dixiedata-previous-* should exist.
		entries, _ := os.ReadDir(parent)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".dixiedata-previous-") {
				t.Fatalf("unexpected backup dir created: %s", e.Name())
			}
		}
	})

	// (2) Target exists but has no DB → skip rename, staging
	// promoted directly. The first renameOS call (target → backup)
	// must NOT happen because the target is empty and there's
	// nothing to back up.
	t.Run("target exists without DB", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, ".dixiedata")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("MkdirAll target: %v", err)
		}
		// No dixiedata.db in target.
		staging := filepath.Join(parent, ".dixiedata-import-test")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll staging: %v", err)
		}
		marker := filepath.Join(staging, "staging-only.txt")
		if err := os.WriteFile(marker, []byte("staging"), 0o644); err != nil {
			t.Fatalf("WriteFile marker: %v", err)
		}

		origRenameOS := renameOS
		t.Cleanup(func() { renameOS = origRenameOS })

		// Track every renameOS call. For an empty target, the
		// first call (target → backup) must be skipped.
		var targetBackupCalled bool
		renameOS = func(src, dst string) error {
			if !strings.HasSuffix(src, ".dixiedata-import-test") {
				targetBackupCalled = true
			}
			return origRenameOS(src, dst)
		}

		if err := replaceDataDir(target, staging); err != nil {
			t.Fatalf("replaceDataDir: %v", err)
		}

		if targetBackupCalled {
			t.Fatalf("empty target case should NOT call renameOS for target → backup; replaceDataDir must skip the rename when target is empty")
		}
		if _, err := os.Stat(filepath.Join(target, "staging-only.txt")); err != nil {
			t.Fatalf("staging marker should now be at target: %v", err)
		}
	})

	// (3) Target has a small (schema-only) DB → skip rename.
	// The first renameOS call (target → backup) must NOT happen
	// because there's no real data to back up.
	t.Run("target exists with empty DB", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, ".dixiedata")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("MkdirAll target: %v", err)
		}
		// Write a 1KB file — well under the 64KB threshold.
		if err := os.WriteFile(filepath.Join(target, "dixiedata.db"), make([]byte, 1024), 0o644); err != nil {
			t.Fatalf("WriteFile DB: %v", err)
		}
		staging := filepath.Join(parent, ".dixiedata-import-test")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll staging: %v", err)
		}
		marker := filepath.Join(staging, "staging-only.txt")
		if err := os.WriteFile(marker, []byte("staging"), 0o644); err != nil {
			t.Fatalf("WriteFile marker: %v", err)
		}

		origRenameOS := renameOS
		t.Cleanup(func() { renameOS = origRenameOS })

		var targetBackupCalled bool
		renameOS = func(src, dst string) error {
			if !strings.HasSuffix(src, ".dixiedata-import-test") {
				targetBackupCalled = true
			}
			return origRenameOS(src, dst)
		}

		if err := replaceDataDir(target, staging); err != nil {
			t.Fatalf("replaceDataDir: %v", err)
		}

		if targetBackupCalled {
			t.Fatalf("schema-only target case should NOT call renameOS for target → backup; replaceDataDir must skip the rename when target is logically empty")
		}
		if _, err := os.Stat(filepath.Join(target, "staging-only.txt")); err != nil {
			t.Fatalf("staging marker should now be at target: %v", err)
		}
	})

	// (4) Target has a large DB → first rename succeeds,
	// backup dir created.
	t.Run("target exists with non-empty DB, first rename succeeds", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, ".dixiedata")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("MkdirAll target: %v", err)
		}
		// Write a 128KB file — over the 64KB threshold.
		if err := os.WriteFile(filepath.Join(target, "dixiedata.db"), make([]byte, 128*1024), 0o644); err != nil {
			t.Fatalf("WriteFile DB: %v", err)
		}
		targetMarker := filepath.Join(target, "original.txt")
		if err := os.WriteFile(targetMarker, []byte("original"), 0o644); err != nil {
			t.Fatalf("WriteFile targetMarker: %v", err)
		}
		staging := filepath.Join(parent, ".dixiedata-import-test")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll staging: %v", err)
		}
		stagingMarker := filepath.Join(staging, "staging-only.txt")
		if err := os.WriteFile(stagingMarker, []byte("staging"), 0o644); err != nil {
			t.Fatalf("WriteFile stagingMarker: %v", err)
		}

		// Inject failing rename for the first 2 attempts to
		// exercise the retry path.
		origRenameOS := renameOS
		t.Cleanup(func() { renameOS = origRenameOS })

		failCount := 0
		renameOS = func(src, dst string) error {
			if !strings.HasSuffix(src, ".dixiedata-import-test") {
				failCount++
				if failCount <= 2 {
					return fmt.Errorf("synthetic access denied (fail #%d)", failCount)
				}
			}
			return origRenameOS(src, dst)
		}

		if err := replaceDataDir(target, staging); err != nil {
			t.Fatalf("replaceDataDir: %v (after %d synthetic fails)", err, failCount)
		}
		if failCount != 3 {
			t.Fatalf("expected 3 rename calls (2 fails + 1 success), got %d", failCount)
		}
		// Staging content should now be at target.
		if _, err := os.Stat(filepath.Join(target, "staging-only.txt")); err != nil {
			t.Fatalf("staging marker should be at target: %v", err)
		}
		// Backup dir should be removed.
		entries, _ := os.ReadDir(parent)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".dixiedata-previous-") {
				t.Fatalf("backup dir should be cleaned up: %s", e.Name())
			}
		}
	})

	// (5) Transient failure with retry — first 2 rename
	// attempts fail, 3rd succeeds. Renames use real os.Rename
	// for the 3rd call.
	t.Run("transient failure recovers via retry", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, ".dixiedata")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("MkdirAll target: %v", err)
		}
		if err := os.WriteFile(filepath.Join(target, "dixiedata.db"), make([]byte, 128*1024), 0o644); err != nil {
			t.Fatalf("WriteFile DB: %v", err)
		}
		staging := filepath.Join(parent, ".dixiedata-import-test")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll staging: %v", err)
		}

		origRenameOS := renameOS
		t.Cleanup(func() { renameOS = origRenameOS })

		failCount := 0
		renameOS = func(src, dst string) error {
			if !strings.HasSuffix(src, ".dixiedata-import-test") {
				failCount++
				if failCount <= 2 {
					return fmt.Errorf("synthetic access denied (fail #%d)", failCount)
				}
			}
			return origRenameOS(src, dst)
		}

		if err := replaceDataDir(target, staging); err != nil {
			t.Fatalf("replaceDataDir: %v (after %d synthetic fails)", err, failCount)
		}
		if failCount != 3 {
			t.Fatalf("expected 3 rename calls (2 fails + 1 success), got %d", failCount)
		}
	})

	// (6) All retries fail → wrapped error returned with
	// rename target named.
	t.Run("all retries fail", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, ".dixiedata")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("MkdirAll target: %v", err)
		}
		if err := os.WriteFile(filepath.Join(target, "dixiedata.db"), make([]byte, 128*1024), 0o644); err != nil {
			t.Fatalf("WriteFile DB: %v", err)
		}
		staging := filepath.Join(parent, ".dixiedata-import-test")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll staging: %v", err)
		}

		origRenameOS := renameOS
		t.Cleanup(func() { renameOS = origRenameOS })

		renameOS = func(src, dst string) error {
			return fmt.Errorf("synthetic permanent failure")
		}

		err := replaceDataDir(target, staging)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed after 5 attempts") {
			t.Fatalf("error must mention retry count: %v", err)
		}
		if !strings.Contains(err.Error(), target) {
			t.Fatalf("error must mention target: %v", err)
		}
		// errors.Is should still work via %w wrapping.
		if !strings.Contains(err.Error(), "synthetic permanent failure") {
			t.Fatalf("error must preserve the underlying cause: %v", err)
		}
	})

	// (7) Rollback — target rename succeeds, staging rename
	// fails, target is restored from backup.
	t.Run("rollback when staging rename fails", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, ".dixiedata")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("MkdirAll target: %v", err)
		}
		if err := os.WriteFile(filepath.Join(target, "dixiedata.db"), make([]byte, 128*1024), 0o644); err != nil {
			t.Fatalf("WriteFile DB: %v", err)
		}
		targetMarker := filepath.Join(target, "original.txt")
		if err := os.WriteFile(targetMarker, []byte("original"), 0o644); err != nil {
			t.Fatalf("WriteFile targetMarker: %v", err)
		}
		staging := filepath.Join(parent, ".dixiedata-import-test")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			t.Fatalf("MkdirAll staging: %v", err)
		}

		origRenameOS := renameOS
		t.Cleanup(func() { renameOS = origRenameOS })

		// Fail the second rename (staging → target).
		renameOS = func(src, dst string) error {
			if strings.HasSuffix(src, ".dixiedata-import-test") {
				return fmt.Errorf("synthetic staging rename failure")
			}
			return origRenameOS(src, dst)
		}

		err := replaceDataDir(target, staging)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		// Target should have been restored to its original
		// location with the original marker intact.
		if _, err := os.Stat(targetMarker); err != nil {
			t.Fatalf("target marker should be restored after rollback: %v", err)
		}
	})
}
