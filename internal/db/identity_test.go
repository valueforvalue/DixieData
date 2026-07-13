package db

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/testtemp"
)

func TestBackfillEntryAuditIdentity(t *testing.T) {
	d, err := Open(testtemp.New(t).Path())
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
	d, err := Open(testtemp.New(t).Path())
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

// TestConfigureUserIdentity_RefusesOverwriteOfCompleteIdentity
// (issue #495) pins the data-layer guard: once user_identity_complete
// is set, ConfigureUserIdentity returns ErrIdentityAlreadyComplete
// unless the caller opts in via IdentityForceOverwrite. The handler-
// level !a.setupRequired guard in handleInitialSetup is the primary
// gate; this is the defense-in-depth companion so a future code path
// (test helper, backup restore edge case, future API) cannot silently
// overwrite the node_prefix namespace and rename every existing
// soldier's display_id under the old prefix.
func TestConfigureUserIdentity_RefusesOverwriteOfCompleteIdentity(t *testing.T) {
	d, err := Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := d.ConfigureUserIdentity("Real", "Operator", "Identity", 1965); err != nil {
		t.Fatalf("first ConfigureUserIdentity: %v", err)
	}
	identityBefore, err := d.UserIdentity()
	if err != nil {
		t.Fatalf("UserIdentity before: %v", err)
	}

	// Second call WITHOUT IdentityForceOverwrite must fail and
	// leave the existing identity untouched.
	_, err = d.ConfigureUserIdentity("Test", "Harness", "User", 1900)
	if err == nil {
		t.Fatal("second ConfigureUserIdentity succeeded without IdentityForceOverwrite; want ErrIdentityAlreadyComplete")
	}
	if err != ErrIdentityAlreadyComplete {
		t.Errorf("error = %v, want ErrIdentityAlreadyComplete", err)
	}

	identityAfter, err := d.UserIdentity()
	if err != nil {
		t.Fatalf("UserIdentity after: %v", err)
	}
	if identityAfter.FirstName != identityBefore.FirstName ||
		identityAfter.NodePrefix != identityBefore.NodePrefix {
		t.Errorf("identity changed despite guard: before=%#v after=%#v", identityBefore, identityAfter)
	}
}

// TestConfigureUserIdentity_ForceOverwriteSucceeds (issue #495)
// pins the escape hatch: callers that legitimately need to write
// the identity a second time (backup restore on top of a freshly
// imported archive, gold-master fixtures, tests/stress helpers)
// pass IdentityForceOverwrite and the write goes through.
func TestConfigureUserIdentity_ForceOverwriteSucceeds(t *testing.T) {
	d, err := Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := d.ConfigureUserIdentity("Real", "Operator", "Identity", 1965); err != nil {
		t.Fatalf("first ConfigureUserIdentity: %v", err)
	}

	identity, err := d.ConfigureUserIdentity("Test", "Harness", "User", 1900, IdentityForceOverwrite())
	if err != nil {
		t.Fatalf("force-overwrite ConfigureUserIdentity: %v", err)
	}
	if identity.FirstName != "Test" || identity.NodePrefix != "THU00" {
		t.Errorf("force-overwrite identity = %#v, want FirstName=Test NodePrefix=THU00", identity)
	}
}
