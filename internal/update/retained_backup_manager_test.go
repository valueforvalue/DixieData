package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetainedBackupManagerCreateAndRestoreDatabaseSnapshot(t *testing.T) {
	dataDir := t.TempDir()
	manager := NewRetainedBackupManager(dataDir)
	manager.now = func() time.Time {
		return time.Date(2026, time.May, 23, 6, 28, 8, 0, time.UTC)
	}

	record, err := manager.CreatePreSchemaUpgradeBackup(CreateRetainedBackupInput{
		SourceAppVersion:    "1.1.20",
		SourceSchemaVersion: 20,
		TargetAppVersion:    "1.1.21",
		TargetSchemaVersion: 21,
		BuildIdentity:       "commit abc123 · 2026-05-23T06:28:08Z",
	}, func(outputPath string) error {
		return os.WriteFile(outputPath, []byte("pre-upgrade-db"), 0o644)
	})
	if err != nil {
		t.Fatalf("CreatePreSchemaUpgradeBackup: %v", err)
	}

	if record.Kind != preSchemaUpgradeBackupKind {
		t.Fatalf("record.Kind = %q", record.Kind)
	}
	if record.SourceSchemaVersion != 20 || record.TargetSchemaVersion != 21 {
		t.Fatalf("record = %#v", record)
	}
	if !strings.HasPrefix(record.DatabaseSnapshotPath, "updates/backups/schema-upgrade-20260523-062808-v20-to-v21/") {
		t.Fatalf("record.DatabaseSnapshotPath = %q", record.DatabaseSnapshotPath)
	}

	indexBytes, err := os.ReadFile(filepath.Join(dataDir, "updates", "backups", "index.json"))
	if err != nil {
		t.Fatalf("ReadFile(index.json): %v", err)
	}
	if !strings.Contains(string(indexBytes), `"database_snapshot_path": "updates/backups/schema-upgrade-20260523-062808-v20-to-v21/dixiedata-pre-upgrade.db"`) {
		t.Fatalf("index.json = %s", indexBytes)
	}

	listed, err := manager.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != record.ID {
		t.Fatalf("listed = %#v", listed)
	}

	restorePath := filepath.Join(t.TempDir(), "restored.db")
	restored, err := manager.RestoreDatabaseSnapshot(record.ID, restorePath)
	if err != nil {
		t.Fatalf("RestoreDatabaseSnapshot: %v", err)
	}
	if restored.ID != record.ID {
		t.Fatalf("restored = %#v", restored)
	}
	content, err := os.ReadFile(restorePath)
	if err != nil {
		t.Fatalf("ReadFile(restored.db): %v", err)
	}
	if string(content) != "pre-upgrade-db" {
		t.Fatalf("restored content = %q", content)
	}
}

func TestRetainedBackupManagerPrunesOlderBackupsByCount(t *testing.T) {
	dataDir := t.TempDir()
	manager := NewRetainedBackupManager(dataDir)
	manager.policy.MaxBackups = 2

	times := []time.Time{
		time.Date(2026, time.May, 23, 6, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 23, 7, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 23, 8, 0, 0, 0, time.UTC),
	}
	for i, currentTime := range times {
		manager.now = func() time.Time { return currentTime }
		_, err := manager.CreatePreSchemaUpgradeBackup(CreateRetainedBackupInput{
			SourceAppVersion:    "1.1.20",
			SourceSchemaVersion: 20,
			TargetAppVersion:    "1.1.21",
			TargetSchemaVersion: 21,
		}, func(outputPath string) error {
			return os.WriteFile(outputPath, []byte{byte('A' + i)}, 0o644)
		})
		if err != nil {
			t.Fatalf("CreatePreSchemaUpgradeBackup(%d): %v", i, err)
		}
	}

	listed, err := manager.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, listed = %#v", len(listed), listed)
	}
	if strings.Contains(listed[0].ID, "060000") || strings.Contains(listed[1].ID, "060000") {
		t.Fatalf("oldest backup should have been pruned: %#v", listed)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "updates", "backups", "schema-upgrade-20260523-060000-v20-to-v21")); !os.IsNotExist(err) {
		t.Fatalf("oldest backup directory should be removed, err = %v", err)
	}
}
