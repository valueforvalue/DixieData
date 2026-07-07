package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

// === Issue #377 slice 2 RED tests ===
//
// Slice 2 changes the policy from "caller must populate" to
// "service defaults version + import path when caller passes
// empty". This unblocks every Create call site that today
// passes empty strings (the slice-1 backward-compat behavior)
// — without a code change at every call site, every new
// Soldier row gets the running binary's version + a generic
// sentinel for the import path.

func TestSoldierService_CreateDefaultsProvenanceWhenCallerPassesEmpty(t *testing.T) {
	// Issue #377 slice 2: Create defaults created_by_version to
	// versioninfo.AppVersion() and created_by_import_path to
	// "create_soldier" when the caller leaves both empty. The
	// sentinel matters because every existing call site today
	// (the slice-1 net asserts they all leave it empty) starts
	// stamping rows the moment this slice lands, without
	// changes to app.go / cli_*.go / etc.
	d := newTestDB(t)
	svc := NewSoldierService(d)
	s, err := svc.Create(models.Soldier{
		FirstName: "Default",
		LastName:  "Provenance",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.CreatedByVersion != versioninfo.AppVersion() {
		t.Errorf("CreatedByVersion = %q; want %q (current app version)", s.CreatedByVersion, versioninfo.AppVersion())
	}
	if s.CreatedByImportPath != "create_soldier" {
		t.Errorf("CreatedByImportPath = %q; want %q", s.CreatedByImportPath, "create_soldier")
	}
}

func TestSoldierService_CreatePreservesCallerSuppliedProvenance(t *testing.T) {
	// The default behavior must NOT clobber a caller-supplied
	// non-empty value. Slice 2's whole point is to give call
	// sites a way to stamp a more-specific import path (e.g.
	// "memorial_json_import", "cli_export") — the defaults
	// only kick in when the caller leaves the field empty.
	d := newTestDB(t)
	svc := NewSoldierService(d)
	s, err := svc.Create(models.Soldier{
		FirstName:           "Custom",
		LastName:            "Stamped",
		CreatedByVersion:    "v1.2.99-test",
		CreatedByImportPath: "memorial_json_import",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.CreatedByVersion != "v1.2.99-test" {
		t.Errorf("CreatedByVersion = %q; want caller-supplied value", s.CreatedByVersion)
	}
	if s.CreatedByImportPath != "memorial_json_import" {
		t.Errorf("CreatedByImportPath = %q; want caller-supplied value", s.CreatedByImportPath)
	}
}

func TestSoldierService_UpdateDoesNotChangeProvenanceOnProvenanceFields(t *testing.T) {
	// Decision 3: created_* frozen on Update. Slice 2's
	// default-stamping is Create-time only; Update must not
	// rewrite the fields even when the caller passes an
	// "updated" value (the existing slice-1 test already
	// proves this; this test pins the policy after slice 2
	// adds the Create-time default).
	d := newTestDB(t)
	svc := NewSoldierService(d)
	created, err := svc.Create(models.Soldier{
		FirstName: "Frozen",
		LastName:  "Provenance",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	originalVersion := created.CreatedByVersion
	originalPath := created.CreatedByImportPath

	// Mutate an unrelated field + try to mutate provenance.
	created.Notes = "changed notes"
	created.CreatedByVersion = "v9.9.9-fake"
	created.CreatedByImportPath = "should_not_persist"
	if err := svc.Update(*created); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := svc.GetByID(created.ID)
	if got.CreatedByVersion != originalVersion {
		t.Errorf("Update rewrote CreatedByVersion: %q -> %q", originalVersion, got.CreatedByVersion)
	}
	if got.CreatedByImportPath != originalPath {
		t.Errorf("Update rewrote CreatedByImportPath: %q -> %q", originalPath, got.CreatedByImportPath)
	}
}
