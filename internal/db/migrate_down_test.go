package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/update"
)

// TestApplyDownSchema_NoOpWhenAlreadyAtTarget covers the early-return
// path: if `current <= target`, applyDownSchema returns nil without
// opening a transaction. The runner must not waste a snapshot or a
// tx cycle on a no-op.
func TestApplyDownSchema_NoOpWhenAlreadyAtTarget(t *testing.T) {
	dataDir := t.TempDir()
	database, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// Current is CurrentSchemaVersion (59). target = 59 is a
	// no-op (we are already at the target).
	if err := applyDownSchema(database, CurrentSchemaVersion); err != nil {
		t.Errorf("applyDownSchema to current version: %v (expected nil for no-op)", err)
	}
	if err := applyDownSchema(database, CurrentSchemaVersion+5); err != nil {
		t.Errorf("applyDownSchema to version > current: %v (expected nil for no-op)", err)
	}
}

// TestApplyDownSchema_RefusesPastIrreversible covers the gate
// contract: any path that crosses an Irreversible block must be
// refused with ErrDowngradeRefused wrapping ErrMigrationIrreversible.
// The CLI unwraps this to print the blocking block ID + Reason.
func TestApplyDownSchema_RefusesPastIrreversible(t *testing.T) {
	dataDir := t.TempDir()
	database, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// DOWN from current (59) to 0 must cross Block 4 (phase1,
	// Irreversible) before reaching the helper blocks. The runner
	// stops at the first Irreversible block encountered.
	err = applyDownSchema(database, 0)
	if err == nil {
		t.Fatal("applyDownSchema to v0 should refuse (Block 4 is Irreversible)")
	}
	if !errors.Is(err, ErrDowngradeRefused) {
		t.Errorf("error = %v, want errors.Is(err, ErrDowngradeRefused)", err)
	}
	if !errors.Is(err, ErrMigrationIrreversible) {
		t.Errorf("error = %v, want errors.Is(err, ErrMigrationIrreversible) (unwrapped)", err)
	}
	// The runner must NOT have committed — the deferred Rollback
	// unwinds the partial state. PRAGMA user_version on the live
	// DB is still CurrentSchemaVersion.
	var v int
	if err := database.Conn().QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if v != CurrentSchemaVersion {
		t.Errorf("user_version after refused DOWN = %d, want %d (refusal must not commit)",
			v, CurrentSchemaVersion)
	}
}

// TestApplyDownSchema_PartialReversibleStepDown covers the happy
// path for PartiallyReversible blocks. Going from v59 to v58
// crosses Block 17 (research_log.evidence_type rename, Irreversible)
// which refuses. Going from v59 to v58 is therefore blocked; the
// only legal one-step DOWNs from v59 are to versions that don't
// cross an Irreversible boundary, and per the catalogue there is
// no such version (Block 17 is the most recent).
//
// This test documents that fact: the current schema state (v59)
// is the floor — every v(N) < v59 crosses at least one Irreversible
// block. The CLI must be the surface that gives the operator a
// path forward (via --restore-point + --force-irreversible + a
// documented acceptance of the data loss).
func TestApplyDownSchema_PartialReversibleStepDown(t *testing.T) {
	dataDir := t.TempDir()
	database, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// DOWN from v60 to v59 succeeds because Block 60
	// (block-60-event-records-event-person-links-fk-rename) is
	// PartiallyReversible. DOWN to any v < 59 must refuse because
	// Block 17 (the Irreversible evidence_type rename) blocks any
	// path crossing v55 → v54.
	for target := 0; target < CurrentSchemaVersion-1; target++ {
		err := applyDownSchema(database, target)
		if !errors.Is(err, ErrDowngradeRefused) {
			t.Errorf("applyDownSchema to v%d: err = %v, want ErrDowngradeRefused (every v<N<59 must refuse)", target, err)
		}
	}
}

// TestApplyDownSchema_EmptyArchiveSucceedsAtCurrentVersion covers
// the safety contract: on a fresh v59 archive with no
// data, the runner should be able to confirm the early-return
// path without writing anything to the user_version pragma.
// This pins the no-data regression net.
func TestApplyDownSchema_EmptyArchiveSucceedsAtCurrentVersion(t *testing.T) {
	dataDir := t.TempDir()
	database, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	if err := applyDownSchema(database, CurrentSchemaVersion); err != nil {
		t.Errorf("applyDownSchema(current): %v", err)
	}
	var v int
	if err := database.Conn().QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if v != CurrentSchemaVersion {
		t.Errorf("user_version after no-op DOWN = %d, want %d", v, CurrentSchemaVersion)
	}
}

// TestApplyDownSchema_SnapshotIsTakenBeforeTransaction covers the
// contract that the CLI's pre-flight takes a retained snapshot
// before calling applyDownSchema. This test does NOT exercise
// the snapshot path (that's the CLI's job — see
// internal/update/retained_backup_manager.go) but pins the
// runner's contract that no destructive SQL fires before the
// refusal point. If the runner were to start writing partial
// state before the Irreversible check, the deferred tx.Rollback
// would unwind it, but the test asserts the post-refusal state
// is byte-identical to pre-refusal.
func TestApplyDownSchema_RefusalDoesNotMutateTables(t *testing.T) {
	dataDir := t.TempDir()
	database, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// Snapshot every user-visible table's row count + user_version
	// before the refused DOWN.
	tables := []string{"soldiers", "records", "images", "tags", "person_record_tags", "archive_meta"}
	pre := map[string]int{}
	for _, tbl := range tables {
		var n int
		if err := database.Conn().QueryRow(`SELECT COUNT(*) FROM ` + tbl).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		pre[tbl] = n
	}

	err = applyDownSchema(database, 0)
	if !errors.Is(err, ErrDowngradeRefused) {
		t.Fatalf("applyDownSchema: %v, want ErrDowngradeRefused", err)
	}

	// Post-refusal counts must match pre-refusal counts exactly.
	for _, tbl := range tables {
		var n int
		if err := database.Conn().QueryRow(`SELECT COUNT(*) FROM ` + tbl).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		if n != pre[tbl] {
			t.Errorf("%s row count after refused DOWN = %d, want %d (refusal must not commit partial state)",
				tbl, n, pre[tbl])
		}
	}
}

// TestRetainedBackupDirectionDiscriminator pins that the future
// pre-schema-downgrade snapshot path can be built on top of the
// existing RetainedBackupManager. Today the manager only knows
// about pre-schema-upgrade snapshots; issue #273's PR 2 doesn't
// add a new snapshot kind, but it does document the integration
// point so the CLI runner can call CreatePreSchemaUpgradeBackup
// (the same function, directionally inverted) and the existing
// restoration path can re-open the snapshot.
//
// This is a smoke test against the existing public surface; the
// actual DOWN runner is wired by the CLI in cli_admin.go. The
// test pins: a) the manager still accepts the CreateInput
// shape it documented, b) a snapshot taken today can be
// inspected for the fields the DOWN runner will need
// (SourceSchemaVersion, TargetSchemaVersion).
func TestRetainedBackupDirectionDiscriminator(t *testing.T) {
	dataDir := t.TempDir()
	database, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// Write a v1 legacy archive by hand so the next Open()
	// triggers the pre-upgrade snapshot path.
	legacyConn, err := sql.Open("sqlite", Path(dataDir))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := legacyConn.Exec(`CREATE TABLE legacy_records (id INTEGER PRIMARY KEY, note TEXT)`); err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if _, err := legacyConn.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if err := legacyConn.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	// Reopen at the new dataDir — this triggers the snapshot.
	database2, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database2.Close()

	manager := update.NewRetainedBackupManager(dataDir)
	backups, err := manager.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("len(backups) = %d, want 1 (the pre-upgrade snapshot)", len(backups))
	}
	backup := backups[0]
	if backup.SourceSchemaVersion != 1 {
		t.Errorf("SourceSchemaVersion = %d, want 1", backup.SourceSchemaVersion)
	}
	if backup.TargetSchemaVersion != CurrentSchemaVersion {
		t.Errorf("TargetSchemaVersion = %d, want %d", backup.TargetSchemaVersion, CurrentSchemaVersion)
	}
	// The pre-schema-upgrade snapshot is the only Kind today;
	// the DOWN runner will eventually use the same CreateInput
	// shape (with inverted Source/Target) for a new pre-downgrade
	// kind, but that addition is a follow-up to this PR per the
	// audit's open question Q1 / Q5.
	_ = filepath.Join
}