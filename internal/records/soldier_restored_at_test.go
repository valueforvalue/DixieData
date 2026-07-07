package records

// Issue #423 Slice 1: restored_at column on soldiers.
//
// Slice 1 ships the schema column + the struct field + the read
// path. Slice 2 (next session) wires the in-place update flow's
// restoreSnapshotBackup to stamp every pre-existing row's
// restored_at after the SQLite file swap. Slice 3 surfaces the
// column in the soldier_card footer + data-quality scan results.
//
// Decision: restored_at is NULLABLE (not NOT NULL DEFAULT '') so:
//   1. The migration backfills without breaking older rows
//      (column simply doesn't exist on pre-v65 soldiers; ALTER
//      TABLE ADD COLUMN without a default is the natural fit).
//   2. A fresh-install row that has never been carried over a
//      restore point carries NULL (not "unknown") — the
//      presence/absence of the value is itself the signal.
//   3. Go field is `string` (empty == NULL) with `json:"-"` —
//      static archive output is unchanged for never-restored rows
//      (the field is hidden from JSON; the v64 fields with
//      omitempty stay that way).
//
// The tracer-bullet regression net here proves:
//   1. The column exists after a fresh install (inline schema)
//      with the right shape: TEXT NULL (not NOT NULL).
//   2. GetByID returns empty string for a never-restored row.
//   3. Update does NOT rewrite restored_at (frozen-across-Update
//      policy — same as created_by_* per #377 Decision 3).
//   4. A round-trip with a populated value preserves the value.

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestRestoredAtColumnShape pins the schema: after a fresh
// install the inline CREATE TABLE for soldiers carries
// `restored_at TEXT` with NOT NULL = 0 (nullable) and no
// DEFAULT (so fresh inserts leave the column NULL). A
// regression that adds NOT NULL + a non-empty default would
// break the presence/absence semantic.
func TestRestoredAtColumnShape(t *testing.T) {
	d := newTestDB(t)

	row := d.Conn().QueryRow(
		`SELECT type, "notnull", dflt_value FROM pragma_table_info('soldiers') WHERE name = 'restored_at'`,
	)
	var typ string
	var notnull int
	var dflt *string
	if err := row.Scan(&typ, &notnull, &dflt); err != nil {
		t.Fatalf("soldiers.restored_at pragma_table_info scan: %v", err)
	}
	if strings.ToLower(typ) != "text" {
		t.Errorf("soldiers.restored_at type = %q, want text", typ)
	}
	if notnull != 0 {
		t.Errorf("soldiers.restored_at NOT NULL = %d, want 0 (nullable)", notnull)
	}
	if dflt != nil {
		t.Errorf("soldiers.restored_at DEFAULT = %v, want nil (no default → fresh inserts leave NULL)", dflt)
	}
}

// TestRestoredAtEmptyForNeverRestoredRow proves that a row
// written via SoldierService.Create comes back with an empty
// RestoredAt string (the Go-side representation of SQL NULL).
// This is the "row has never been carried over a restore point"
// signal.
func TestRestoredAtEmptyForNeverRestoredRow(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Never",
		LastName:  "Restored",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.RestoredAt != "" {
		t.Errorf("RestoredAt = %q, want empty (NULL) for a never-restored row", got.RestoredAt)
	}
}

// TestRestoredAtPreservedAcrossUpdate proves the frozen-
// across-Update policy: when a row already has a restored_at
// value (populated by restoreSnapshotBackup's bulk UPDATE),
// a subsequent Update call does NOT clear it. The column is
// a historical fact — same invariant as created_by_*
// per #377 Decision 3.
func TestRestoredAtPreservedAcrossUpdate(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Update",
		LastName:  "Target",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate a row that was carried over a restore point
	// (slice 2 will wire this in restoreSnapshotBackup).
	if _, err := d.Conn().Exec(
		`UPDATE soldiers SET restored_at = ? WHERE id = ?`,
		"2026-07-08T12:00:00Z", created.ID,
	); err != nil {
		t.Fatalf("simulate restore: %v", err)
	}

	// Update through the service — must NOT touch restored_at.
	updated, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	updated.Notes = "Researcher notes"
	if err := svc.Update(*updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after Update: %v", err)
	}
	if after.RestoredAt != "2026-07-08T12:00:00Z" {
		t.Errorf("RestoredAt after Update = %q, want %q (frozen-across-Update)",
			after.RestoredAt, "2026-07-08T12:00:00Z")
	}
	if after.Notes != "Researcher notes" {
		t.Errorf("Notes = %q, want %q (Update still writes other fields)",
			after.Notes, "Researcher notes")
	}
}
