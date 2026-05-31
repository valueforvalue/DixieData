package db

import "testing"

func TestBackfillEntryAuditIdentity(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, sync_id, first_name, last_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"DXD-00001", "sync-1", "Legacy", "Record", "2026-01-01 00:00:00", "2026-01-02 00:00:00"); err != nil {
		t.Fatalf("insert soldier: %v", err)
	}
	if _, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	if err := d.BackfillEntryAuditIdentity(); err != nil {
		t.Fatalf("BackfillEntryAuditIdentity: %v", err)
	}

	var addedBy, lastEditedBy, lastEditedAt string
	if err := d.Conn().QueryRow(`SELECT added_by, last_edited_by, last_edited_at FROM soldiers WHERE display_id = ?`, "DXD-00001").Scan(&addedBy, &lastEditedBy, &lastEditedAt); err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if addedBy != "S. Carter" || lastEditedBy != "S. Carter" {
		t.Fatalf("unexpected audit attribution: added_by=%q last_edited_by=%q", addedBy, lastEditedBy)
	}
	if lastEditedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("last_edited_at = %q", lastEditedAt)
	}
}

func TestEntryAuditIdentityBackfillNeeded(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, sync_id, first_name, last_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"DXD-00001", "sync-1", "Legacy", "Record", "2026-01-01 00:00:00", "2026-01-02 00:00:00"); err != nil {
		t.Fatalf("insert soldier: %v", err)
	}

	needed, err := d.EntryAuditIdentityBackfillNeeded()
	if err != nil {
		t.Fatalf("EntryAuditIdentityBackfillNeeded before backfill: %v", err)
	}
	if !needed {
		t.Fatalf("expected backfill to be needed before audit identity is populated")
	}

	if _, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	if err := d.BackfillEntryAuditIdentity(); err != nil {
		t.Fatalf("BackfillEntryAuditIdentity: %v", err)
	}

	needed, err = d.EntryAuditIdentityBackfillNeeded()
	if err != nil {
		t.Fatalf("EntryAuditIdentityBackfillNeeded after backfill: %v", err)
	}
	if needed {
		t.Fatalf("expected backfill to be unnecessary after audit identity is populated")
	}
}
