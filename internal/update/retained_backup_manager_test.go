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

// TestRetainedBackupManagerDowngradeSnapshot pins the DOWN path
// added in issue #273 PR 3. The downgrade snapshot must:
//   - have Kind = preSchemaDowngradeBackupKind (distinct from upgrade)
//   - have Direction = directionDowngrade
//   - write the snapshot file under the direction-aware filename
//     (dixiedata-pre-downgrade.db, not dixiedata-pre-upgrade.db)
//   - be listed alongside upgrade snapshots in List() (mixed
//     index is allowed; the direction is the discriminator)
//   - be restorable via RestoreDatabaseSnapshot (the path is
//     direction-agnostic)
func TestRetainedBackupManagerDowngradeSnapshot(t *testing.T) {
	dataDir := t.TempDir()
	manager := NewRetainedBackupManager(dataDir)

	record, err := manager.CreatePreSchemaDowngradeBackup(CreateRetainedBackupInput{
		SourceAppVersion:    "1.1.1",
		SourceSchemaVersion: 59,
		TargetAppVersion:    "1.1.1",
		TargetSchemaVersion: 58,
		BuildIdentity:       "test-build",
	}, func(outputPath string) error {
		return os.WriteFile(outputPath, []byte("downgrade-snapshot-bytes"), 0o644)
	})
	if err != nil {
		t.Fatalf("CreatePreSchemaDowngradeBackup: %v", err)
	}

	// Kind + Direction are set correctly.
	if record.Kind != preSchemaDowngradeBackupKind {
		t.Errorf("record.Kind = %q, want %q", record.Kind, preSchemaDowngradeBackupKind)
	}
	if record.Direction != directionDowngrade {
		t.Errorf("record.Direction = %q, want %q", record.Direction, directionDowngrade)
	}

	// Filename is direction-aware: dixiedata-pre-downgrade.db,
	// not dixiedata-pre-upgrade.db.
	wantPath := filepath.Join(dataDir, record.DatabaseSnapshotPath)
	if !strings.HasSuffix(wantPath, "dixiedata-pre-downgrade.db") {
		t.Errorf("snapshot path = %q, want suffix %q", wantPath, "dixiedata-pre-downgrade.db")
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("snapshot file not on disk: %v (path = %q)", err, wantPath)
	}
	// The upgrade-only filename must NOT be created for a downgrade
	// snapshot — that's how a mixed UP/DOWN record list is
	// unambiguous from the artifact alone.
	upgradePath := filepath.Join(filepath.Dir(wantPath), snapshotFileNameUpgrade)
	if _, err := os.Stat(upgradePath); !os.IsNotExist(err) {
		t.Errorf("upgrade filename %q should NOT exist for a downgrade snapshot (err = %v)",
			upgradePath, err)
	}

	// DirectionLabel() returns the explicit direction.
	if record.DirectionLabel() != directionDowngrade {
		t.Errorf("DirectionLabel() = %q, want %q", record.DirectionLabel(), directionDowngrade)
	}

	// Listed in the index; mixed UP/DOWN record list is allowed.
	listed, err := manager.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Kind != preSchemaDowngradeBackupKind {
		t.Errorf("listed = %#v, want one downgrade record", listed)
	}

	// RestoreDatabaseSnapshot works (path is direction-agnostic).
	restoredPath := filepath.Join(t.TempDir(), "restored.db")
	restored, err := manager.RestoreDatabaseSnapshot(record.ID, restoredPath)
	if err != nil {
		t.Fatalf("RestoreDatabaseSnapshot: %v", err)
	}
	if restored.Kind != preSchemaDowngradeBackupKind {
		t.Errorf("restored.Kind = %q, want %q", restored.Kind, preSchemaDowngradeBackupKind)
	}
	content, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "downgrade-snapshot-bytes" {
		t.Errorf("restored content = %q, want %q", content, "downgrade-snapshot-bytes")
	}
}

// TestRetainedBackupManagerMixedUpgradeAndDowngradeSnapshots pins
// that the same manager can hold both kinds of snapshot in a
// single index. The index sort is timestamp-only (newest first);
// the direction field is the operator's discriminator.
func TestRetainedBackupManagerMixedUpgradeAndDowngradeSnapshots(t *testing.T) {
	dataDir := t.TempDir()
	manager := NewRetainedBackupManager(dataDir)

	// First: an upgrade snapshot.
	upRecord, err := manager.CreatePreSchemaUpgradeBackup(CreateRetainedBackupInput{
		SourceSchemaVersion: 20,
		TargetSchemaVersion: 21,
	}, func(outputPath string) error {
		return os.WriteFile(outputPath, []byte("upgrade"), 0o644)
	})
	if err != nil {
		t.Fatalf("upgrade Create: %v", err)
	}
	if upRecord.Direction != directionUpgrade {
		t.Errorf("upgrade record Direction = %q, want %q", upRecord.Direction, directionUpgrade)
	}

	// Second: a downgrade snapshot. The manager must not reject
	// the mixed-direction index.
	downRecord, err := manager.CreatePreSchemaDowngradeBackup(CreateRetainedBackupInput{
		SourceSchemaVersion: 21,
		TargetSchemaVersion: 20,
	}, func(outputPath string) error {
		return os.WriteFile(outputPath, []byte("downgrade"), 0o644)
	})
	if err != nil {
		t.Fatalf("downgrade Create: %v", err)
	}
	if downRecord.Direction != directionDowngrade {
		t.Errorf("downgrade record Direction = %q, want %q", downRecord.Direction, directionDowngrade)
	}

	// Both present in the index.
	listed, err := manager.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2 (one upgrade, one downgrade)", len(listed))
	}
	kinds := map[string]bool{}
	for _, rec := range listed {
		kinds[rec.Kind] = true
	}
	if !kinds[preSchemaUpgradeBackupKind] || !kinds[preSchemaDowngradeBackupKind] {
		t.Errorf("listed kinds = %v, want both upgrade and downgrade present", kinds)
	}
}

// TestRetainedBackupRecordDirectionLabelDefaultsUpgrade covers
// backward compatibility: a record persisted before the Direction
// field was added (issue #273 PR 3) has Direction == "". The
// DirectionLabel() helper must return "upgrade" so listings /
// restore paths don't have to special-case the empty string.
func TestRetainedBackupRecordDirectionLabelDefaultsUpgrade(t *testing.T) {
	old := RetainedBackupRecord{ID: "legacy", Kind: preSchemaUpgradeBackupKind}
	if got := old.DirectionLabel(); got != directionUpgrade {
		t.Errorf("legacy record DirectionLabel() = %q, want %q (backward-compat default)", got, directionUpgrade)
	}
}

// TestCreatePreSchemaChangeBackupRejectsUnknownDirection pins the
// programmer-error guard: any direction string other than
// directionUpgrade / directionDowngrade is rejected. The check
// happens before any disk write, so a typo never produces a
// half-written record.
func TestCreatePreSchemaChangeBackupRejectsUnknownDirection(t *testing.T) {
	dataDir := t.TempDir()
	manager := NewRetainedBackupManager(dataDir)
	_, err := manager.CreatePreSchemaChangeBackup(CreateRetainedBackupInput{
		SourceSchemaVersion: 20,
		TargetSchemaVersion: 21,
	}, "sideways", func(outputPath string) error {
		return os.WriteFile(outputPath, []byte("x"), 0o644)
	})
	if err == nil {
		t.Fatal("expected error for unknown direction, got nil")
	}
	if !strings.Contains(err.Error(), "direction") {
		t.Errorf("error %q should mention 'direction'", err.Error())
	}
}
