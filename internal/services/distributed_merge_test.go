package services

import (
	"archive/zip"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	_ "modernc.org/sqlite"
)

func TestBackupService_ImportSQLiteBackupMigratesSchema(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "legacy-sqlite-backup.zip")
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
		t.Fatalf("Close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close backup: %v", err)
	}

	restoreDB := newTestDB(t)
	restoreSvc := NewSoldierService(restoreDB)
	backupSvc := NewBackupService(restoreDB, restoreSvc)
	restoreDir := t.TempDir()

	importedManifest, err := backupSvc.Import(backupPath, restoreDir)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if importedManifest.SchemaVersion != 1 {
		t.Fatalf("manifest schema version = %d", importedManifest.SchemaVersion)
	}

	reopened, err := openExistingTestDB(restoreDir)
	if err != nil {
		t.Fatalf("openExistingTestDB: %v", err)
	}
	defer reopened.Close()

	var userVersion int
	if err := reopened.Conn().QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if userVersion != buildinfo.SchemaVersion {
		t.Fatalf("user_version = %d, want %d", userVersion, buildinfo.SchemaVersion)
	}

	reopenedSvc := NewSoldierService(reopened)
	results, total, err := reopenedSvc.SearchPage("00001", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("restored search total=%d len=%d", total, len(results))
	}
	restored, err := reopenedSvc.GetByID(results[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if restored.DisplayID != "TDM65-DXD-00001" {
		t.Fatalf("DisplayID = %q", restored.DisplayID)
	}
	if restored.SyncID == "" || restored.DeathDate == "" || restored.UpdatedAt == "" {
		t.Fatalf("restored soldier missing migrated fields: %#v", restored)
	}
	if restored.BirthDate != "01/13/1842" {
		t.Fatalf("BirthDate = %q", restored.BirthDate)
	}
	if len(restored.Records) != 1 {
		t.Fatalf("records len = %d", len(restored.Records))
	}
	if restored.Records[0].SyncID == "" || restored.Records[0].SoldierSyncID != restored.SyncID {
		t.Fatalf("record identity mismatch: %#v soldier=%#v", restored.Records[0], restored)
	}
}

func TestDistributedMergeFormatSupportsDivergentAuthorDatabases(t *testing.T) {
	baseDir := t.TempDir()
	baseDB, err := db.Open(baseDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer baseDB.Close()
	if err := baseDB.SetSystemConfig("node_prefix", "BASE"); err != nil {
		t.Fatalf("SetSystemConfig base prefix: %v", err)
	}
	if err := baseDB.SetSystemConfig("node_id", "base-node"); err != nil {
		t.Fatalf("SetSystemConfig base node id: %v", err)
	}

	baseSvc := NewSoldierService(baseDB)
	baseSoldier, err := baseSvc.Create(models.Soldier{
		FirstName: "Base",
		LastName:  "Shared",
		Notes:     "base",
		Records:   []models.Record{{RecordType: "Roster", AppID: "APP-BASE-1", Details: "Shared roster"}},
	})
	if err != nil {
		t.Fatalf("Create base soldier: %v", err)
	}
	if err := baseSvc.AddImage(baseSoldier.ID, "base.png", `images\base\base.png`, "Base"); err != nil {
		t.Fatalf("AddImage base: %v", err)
	}

	authorADir := t.TempDir()
	authorBDir := t.TempDir()
	if err := baseDB.SnapshotTo(db.Path(authorADir)); err != nil {
		t.Fatalf("SnapshotTo author A: %v", err)
	}
	if err := baseDB.SnapshotTo(db.Path(authorBDir)); err != nil {
		t.Fatalf("SnapshotTo author B: %v", err)
	}

	authorADB, err := db.Open(authorADir)
	if err != nil {
		t.Fatalf("db.Open author A: %v", err)
	}
	defer authorADB.Close()
	if err := authorADB.SetSystemConfig("node_prefix", "ALPHA"); err != nil {
		t.Fatalf("SetSystemConfig author A prefix: %v", err)
	}
	if err := authorADB.SetSystemConfig("node_id", "author-a"); err != nil {
		t.Fatalf("SetSystemConfig author A node id: %v", err)
	}
	authorASvc := NewSoldierService(authorADB)

	authorBDB, err := db.Open(authorBDir)
	if err != nil {
		t.Fatalf("db.Open author B: %v", err)
	}
	defer authorBDB.Close()
	if err := authorBDB.SetSystemConfig("node_prefix", "BRAVO"); err != nil {
		t.Fatalf("SetSystemConfig author B prefix: %v", err)
	}
	if err := authorBDB.SetSystemConfig("node_id", "author-b"); err != nil {
		t.Fatalf("SetSystemConfig author B node id: %v", err)
	}
	authorBSvc := NewSoldierService(authorBDB)

	sharedA, err := authorASvc.GetByID(baseSoldier.ID)
	if err != nil {
		t.Fatalf("GetByID author A: %v", err)
	}
	sharedA.Notes = "edited by author A"
	sharedA.Records = append(sharedA.Records, models.Record{RecordType: "Letter", AppID: "APP-A-2", Details: "Author A note"})
	if err := authorASvc.Update(*sharedA); err != nil {
		t.Fatalf("Update author A: %v", err)
	}

	sharedB, err := authorBSvc.GetByID(baseSoldier.ID)
	if err != nil {
		t.Fatalf("GetByID author B: %v", err)
	}
	sharedB.BuriedIn = "Author B Cemetery"
	if err := authorBSvc.Update(*sharedB); err != nil {
		t.Fatalf("Update author B: %v", err)
	}
	if err := authorBSvc.AddImage(sharedB.ID, "b.png", `images\bravo\b.png`, "Bravo"); err != nil {
		t.Fatalf("AddImage author B: %v", err)
	}

	newAuthorA, err := authorASvc.Create(models.Soldier{FirstName: "Alpha", LastName: "Only"})
	if err != nil {
		t.Fatalf("Create author A generated soldier: %v", err)
	}
	newAuthorB, err := authorBSvc.Create(models.Soldier{FirstName: "Bravo", LastName: "Only"})
	if err != nil {
		t.Fatalf("Create author B generated soldier: %v", err)
	}

	sharedAFinal, err := authorASvc.GetByID(baseSoldier.ID)
	if err != nil {
		t.Fatalf("GetByID author A final: %v", err)
	}
	sharedBFinal, err := authorBSvc.GetByID(baseSoldier.ID)
	if err != nil {
		t.Fatalf("GetByID author B final: %v", err)
	}

	if sharedAFinal.SyncID == "" || sharedAFinal.SyncID != sharedBFinal.SyncID || sharedAFinal.SyncID != baseSoldier.SyncID {
		t.Fatalf("shared sync ids diverged: base=%q a=%q b=%q", baseSoldier.SyncID, sharedAFinal.SyncID, sharedBFinal.SyncID)
	}
	if len(sharedAFinal.Records) != 2 {
		t.Fatalf("author A records len = %d", len(sharedAFinal.Records))
	}
	for _, record := range sharedAFinal.Records {
		if record.SyncID == "" || record.SoldierSyncID != sharedAFinal.SyncID {
			t.Fatalf("author A record identity mismatch: %#v soldier=%#v", record, sharedAFinal)
		}
	}
	if len(sharedBFinal.Images) != 2 {
		t.Fatalf("author B images len = %d", len(sharedBFinal.Images))
	}
	for _, image := range sharedBFinal.Images {
		if image.SyncID == "" || image.SoldierSyncID != sharedBFinal.SyncID {
			t.Fatalf("author B image identity mismatch: %#v soldier=%#v", image, sharedBFinal)
		}
	}
	if newAuthorA.DisplayID != "ALPHA-DXD-00001" {
		t.Fatalf("author A generated display id = %q", newAuthorA.DisplayID)
	}
	if newAuthorB.DisplayID != "BRAVO-DXD-00001" {
		t.Fatalf("author B generated display id = %q", newAuthorB.DisplayID)
	}
	seen := map[string]struct{}{
		sharedAFinal.SyncID: {},
		newAuthorA.SyncID:   {},
		newAuthorB.SyncID:   {},
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct logical soldiers, got %#v", seen)
	}
}

func createLegacySchemaV1DB(t *testing.T, dbPath string) {
	t.Helper()
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer conn.Close()

	schemaV1 := `
CREATE TABLE soldiers (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    display_id   TEXT UNIQUE NOT NULL,
    is_generated BOOLEAN DEFAULT 0,
    pension_id   TEXT,
    application_id TEXT,
    first_name   TEXT,
    middle_name  TEXT,
    last_name    TEXT,
    rank         TEXT,
    rank_in      TEXT,
    rank_out     TEXT,
    unit         TEXT,
    pension_state TEXT,
    death_year   INTEGER,
    death_month  INTEGER,
    death_day    INTEGER,
    birth_info   TEXT,
    buried_in    TEXT,
    notes        TEXT,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE records (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    soldier_id   INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    record_type  TEXT,
    app_id       TEXT,
    details      TEXT
);
CREATE TABLE images (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    soldier_id   INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    file_name    TEXT,
    file_path    TEXT,
    caption      TEXT
);
CREATE INDEX idx_soldiers_death ON soldiers(death_month, death_day);
CREATE VIRTUAL TABLE soldiers_fts USING fts5(
    first_name, last_name, unit, soldier_rank,
    content=soldiers, content_rowid=id
);`
	if _, err := conn.Exec(schemaV1); err != nil {
		t.Fatalf("create schema v1: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO soldiers (display_id, is_generated, first_name, last_name, death_year, death_month, death_day, birth_info, notes, created_at) VALUES ('DXD-00001', 1, 'Legacy', 'Soldier', 1863, 5, 7, 'b. Jan. 13, 1842, Blount Co., AL, U.S.A.', 'legacy note', '2026-01-02 03:04:05')`); err != nil {
		t.Fatalf("insert soldier: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO records (soldier_id, record_type, app_id, details) VALUES (1, 'Roster', 'APP-1', 'Legacy record')`); err != nil {
		t.Fatalf("insert record: %v", err)
	}
	if _, err := conn.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
}
