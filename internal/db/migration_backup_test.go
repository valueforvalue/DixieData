package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/update"
	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

func TestOpenCreatesRetainedPreMigrationBackup(t *testing.T) {
	dataDir := t.TempDir()
	legacyConn, err := sql.Open("sqlite", Path(dataDir))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := legacyConn.Exec(`CREATE TABLE legacy_records (id INTEGER PRIMARY KEY, note TEXT)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := legacyConn.Exec(`INSERT INTO legacy_records (note) VALUES ('before-upgrade')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if _, err := legacyConn.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set legacy user_version: %v", err)
	}
	if err := legacyConn.Close(); err != nil {
		t.Fatalf("Close legacy connection: %v", err)
	}

	database, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	manager := update.NewRetainedBackupManager(dataDir)
	backups, err := manager.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("len(backups) = %d, backups = %#v", len(backups), backups)
	}
	backup := backups[0]
	if backup.SourceSchemaVersion != 1 || backup.TargetSchemaVersion != CurrentSchemaVersion {
		t.Fatalf("backup = %#v", backup)
	}
	if backup.SourceAppVersion != versioninfo.AppVersionForSchema(1) || backup.TargetAppVersion != GetAppVersion() {
		t.Fatalf("backup app versions = %#v", backup)
	}

	backupConn, err := sql.Open("sqlite", filepath.Join(dataDir, filepath.FromSlash(backup.DatabaseSnapshotPath)))
	if err != nil {
		t.Fatalf("sql.Open backup: %v", err)
	}
	defer backupConn.Close()

	var backupSchemaVersion int
	if err := backupConn.QueryRow(`PRAGMA user_version`).Scan(&backupSchemaVersion); err != nil {
		t.Fatalf("read backup user_version: %v", err)
	}
	if backupSchemaVersion != 1 {
		t.Fatalf("backup user_version = %d", backupSchemaVersion)
	}

	var migratedSchemaVersion int
	if err := database.Conn().QueryRow(`PRAGMA user_version`).Scan(&migratedSchemaVersion); err != nil {
		t.Fatalf("read migrated user_version: %v", err)
	}
	if migratedSchemaVersion != CurrentSchemaVersion {
		t.Fatalf("migrated user_version = %d", migratedSchemaVersion)
	}

	metadataPath := filepath.Join(dataDir, filepath.FromSlash(backup.MetadataPath))
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("Stat(metadata.json): %v", err)
	}
}
