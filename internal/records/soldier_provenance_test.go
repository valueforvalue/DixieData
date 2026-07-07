package records

// Issue #377 Slice 1: row provenance columns.
// The slice ships the schema columns (created_by_version +
// created_by_import_path) on soldiers + the read/write plumbing in
// SoldierService. The tracer-bullet regression net here proves:
//
// 1. The columns exist after a fresh install (inline schema) with
//    the right shape: TEXT NOT NULL DEFAULT ''.
// 2. Create stamps the columns when caller populates them.
// 3. Create leaves them as '' when caller leaves them empty
//    (backward compat with every existing caller).
// 4. Update does NOT rewrite created_by_version + created_by_import_path
//    (Decision 3: created_* frozen across Update).
// 5. Backfill assigns "unknown" to pre-v64 rows (via the same SQL
//    block-64 ships, exercised on a fresh DB where every row
//    starts at '').
//
// Slice 1 is schema + struct + read/write plumbing only. Slice 2
// (next session) wires every Create/Update call site to stamp the
// appropriate path string; Slice 3 surfaces the columns in the
// soldier_card footer + quality-scan results; Slice 4 stamps CLI
// subcommands.

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestProvenanceColumnsExist pins the schema: after a fresh install
// the inline CREATE TABLE for soldiers carries
// `created_by_version TEXT NOT NULL DEFAULT ''` and
// `created_by_import_path TEXT NOT NULL DEFAULT ''`. A regression
// that drops either column (or omits the default) would break the
// backfill and the read path in soldierScanDest.
func TestProvenanceColumnsExist(t *testing.T) {
	d := newTestDB(t)

	for _, col := range []string{"created_by_version", "created_by_import_path"} {
		row := d.Conn().QueryRow(
			`SELECT type, "notnull", dflt_value FROM pragma_table_info('soldiers') WHERE name = '` + col + `'`,
		)
		var typ string
		var notnull int
		var dflt string
		if err := row.Scan(&typ, &notnull, &dflt); err != nil {
			t.Fatalf("soldiers.%s pragma_table_info scan: %v", col, err)
		}
		if strings.ToLower(typ) != "text" {
			t.Errorf("soldiers.%s type = %q, want text", col, typ)
		}
		if notnull != 1 {
			t.Errorf("soldiers.%s NOT NULL = %d, want 1", col, notnull)
		}
		if dflt != "''" {
			t.Errorf("soldiers.%s DEFAULT = %q, want '' (SQLite renders empty-string defaults wrapped in quotes)", col, dflt)
		}
	}
}

// TestSoldierService_CreateStampsProvenance proves the Create path
// writes the caller-supplied provenance strings to the row. Slice 1
// does NOT auto-fill from versioninfo.AppVersion() — that's slice 2's
// job (handlers stamp per-path). Slice 1 only proves the column
// plumbing works when the caller populates the struct fields.
func TestSoldierService_CreateStampsProvenance(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName:           "James",
		LastName:            "Gillespie",
		CreatedByVersion:    "v1.2.64",
		CreatedByImportPath: "create_soldier",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.CreatedByVersion != "v1.2.64" {
		t.Errorf("CreatedByVersion = %q, want v1.2.64", created.CreatedByVersion)
	}
	if created.CreatedByImportPath != "create_soldier" {
		t.Errorf("CreatedByImportPath = %q, want create_soldier", created.CreatedByImportPath)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.CreatedByVersion != "v1.2.64" {
		t.Errorf("GetByID CreatedByVersion = %q, want v1.2.64 (column read path)", got.CreatedByVersion)
	}
	if got.CreatedByImportPath != "create_soldier" {
		t.Errorf("GetByID CreatedByImportPath = %q, want create_soldier (column read path)", got.CreatedByImportPath)
	}
}

// TestSoldierService_CreateDefaultsProvenanceToEmpty pins the
// backward-compat contract: existing callers that don't populate
// the new fields must continue to work unchanged. The struct
// defaults to "" so Create writes '' to both columns. Slice 2
// (handler stamping) changes the per-handler behaviour — every
// handler stamps before calling Create — but Slice 1 must not
// break callers that haven't been updated yet.
func TestSoldierService_CreateDefaultsProvenanceToEmpty(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Robert",
		LastName:  "Lee",
		// CreatedByVersion + CreatedByImportPath deliberately
		// not populated — slice 1 must not require them.
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.CreatedByVersion != "" {
		t.Errorf("CreatedByVersion = %q, want empty (caller didn't stamp; Slice 2 will auto-fill)", created.CreatedByVersion)
	}
	if created.CreatedByImportPath != "" {
		t.Errorf("CreatedByImportPath = %q, want empty (caller didn't stamp; Slice 2 will auto-fill)", created.CreatedByImportPath)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.CreatedByVersion != "" || got.CreatedByImportPath != "" {
		t.Errorf("GetByID provenance = (%q, %q), want ('', '')", got.CreatedByVersion, got.CreatedByImportPath)
	}
}

// TestSoldierService_UpdateFreezesCreatedProvenance pins Decision 3:
// "Created by" means *at the moment the row entered the table*.
// Updates don't re-stamp provenance. If the user wants to track
// re-imports, that's a separate column.
//
// The test stamps the row at Create, then Update with a different
// value in the struct fields (simulating a future caller bug that
// forgets Decision 3), and confirms the row's stored values are
// unchanged.
func TestSoldierService_UpdateFreezesCreatedProvenance(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName:           "Nathan",
		LastName:            "Bedford",
		CreatedByVersion:    "v1.2.64",
		CreatedByImportPath: "memorial_json_import",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate a future caller (or a buggy one) that populates
	// the struct fields on Update. The Update SQL must NOT
	// rewrite these columns — Decision 3 freezes them.
	updateTarget := *created
	updateTarget.CreatedByVersion = "v9.9.9-bogus"
	updateTarget.CreatedByImportPath = "bogus_path"
	updateTarget.Notes = "notes changed by update"
	if err := svc.Update(updateTarget); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.CreatedByVersion != "v1.2.64" {
		t.Errorf("Update rewrote CreatedByVersion: got %q, want v1.2.64 (Decision 3 — frozen across Update)", got.CreatedByVersion)
	}
	if got.CreatedByImportPath != "memorial_json_import" {
		t.Errorf("Update rewrote CreatedByImportPath: got %q, want memorial_json_import (Decision 3 — frozen across Update)", got.CreatedByImportPath)
	}
	// Sanity check: the Update DID apply the unrelated change,
	// so the test proves the freeze is targeted, not a no-op.
	if got.Notes != "notes changed by update" {
		t.Errorf("Update did not apply Notes change: got %q", got.Notes)
	}
}

// TestSoldierService_BackfillAssignsUnknownToEmpty pins the v64
// backfill behaviour: every pre-v64 row that landed with
// created_by_version = '' or created_by_import_path = '' (the
// DEFAULT, since the inline schema sets DEFAULT '') is rewritten
// to "unknown" by the same UPDATE block-64 ships.
//
// Simulates the upgrade path: insert a row on a fresh DB (the
// inline schema writes '' to both columns), then run the
// backfill UPDATEs, then confirm both columns are "unknown".
func TestSoldierService_BackfillAssignsUnknownToEmpty(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	// Create a row with no provenance stamp — the inline schema
	// writes '' to both columns because the caller didn't supply
	// them and the DEFAULT is ''.
	created, err := svc.Create(models.Soldier{
		FirstName: "Pierre",
		LastName:  "Beauregard",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Run the same backfill UPDATEs block-64 ships.
	if _, err := d.Conn().Exec(`UPDATE soldiers SET created_by_version = 'unknown' WHERE created_by_version = ''`); err != nil {
		t.Fatalf("backfill version: %v", err)
	}
	if _, err := d.Conn().Exec(`UPDATE soldiers SET created_by_import_path = 'unknown' WHERE created_by_import_path = ''`); err != nil {
		t.Fatalf("backfill path: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after backfill: %v", err)
	}
	if got.CreatedByVersion != "unknown" {
		t.Errorf("backfill CreatedByVersion = %q, want 'unknown'", got.CreatedByVersion)
	}
	if got.CreatedByImportPath != "unknown" {
		t.Errorf("backfill CreatedByImportPath = %q, want 'unknown'", got.CreatedByImportPath)
	}
}

// TestSoldierService_BackfillLeavesStampedRowsAlone proves the
// backfill is targeted: rows that already carry non-empty values
// are NOT rewritten. A regression that turned the backfill into
// `UPDATE soldiers SET ... = 'unknown'` without the WHERE clause
// would clobber stamped rows and break Decision 4 (stable path
// strings for analytics queries).
func TestSoldierService_BackfillLeavesStampedRowsAlone(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	// Create a row WITH provenance stamps so the backfill must
	// leave it alone.
	stamped, err := svc.Create(models.Soldier{
		FirstName:           "Stand",
		LastName:            "Watie",
		CreatedByVersion:    "v1.2.64",
		CreatedByImportPath: "restore_backup_archive",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Run the backfill (same SQL block-64 ships).
	if _, err := d.Conn().Exec(`UPDATE soldiers SET created_by_version = 'unknown' WHERE created_by_version = ''`); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got, err := svc.GetByID(stamped.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.CreatedByVersion != "v1.2.64" {
		t.Errorf("backfill clobbered stamped row: CreatedByVersion = %q, want v1.2.64", got.CreatedByVersion)
	}
	if got.CreatedByImportPath != "restore_backup_archive" {
		t.Errorf("backfill clobbered stamped row: CreatedByImportPath = %q, want restore_backup_archive", got.CreatedByImportPath)
	}
}