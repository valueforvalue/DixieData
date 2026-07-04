package db

import (
	"fmt"
	"testing"
)

func TestNextDXDID_Format(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	id, err := d.NextDXDID()
	if err != nil {
		t.Fatalf("NextDXDID: %v", err)
	}
	if id != "DXD-00001" {
		t.Errorf("expected DXD-00001, got %s", id)
	}
}

func TestNextDXDID_Increment(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Insert some generated soldiers
	for i := 0; i < 5; i++ {
		displayID := fmt.Sprintf("DXD-%05d", i+1)
		_, err := d.conn.Exec(
			`INSERT INTO soldiers (display_id, is_generated) VALUES (?, 1)`,
			displayID,
		)
		if err != nil {
			t.Fatalf("insert soldier %d: %v", i+1, err)
		}
	}

	id, err := d.NextDXDID()
	if err != nil {
		t.Fatalf("NextDXDID: %v", err)
	}
	if id != "DXD-00006" {
		t.Errorf("expected DXD-00006, got %s", id)
	}
}

func TestNextDXDID_NonGeneratedIgnored(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Insert a non-generated soldier (pension ID) — should not affect count
	_, err = d.conn.Exec(
		`INSERT INTO soldiers (display_id, is_generated) VALUES ('PENSION-12345', 0)`,
	)
	if err != nil {
		t.Fatalf("insert pension soldier: %v", err)
	}

	id, err := d.NextDXDID()
	if err != nil {
		t.Fatalf("NextDXDID: %v", err)
	}
	if id != "DXD-00001" {
		t.Errorf("expected DXD-00001 (non-generated ignored), got %s", id)
	}
}

func TestNextDXDID_UsesExistingDXDIDsWithoutGeneratedFlag(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	_, err = d.conn.Exec(`INSERT INTO soldiers (display_id, is_generated) VALUES ('DXD-00007', 0)`)
	if err != nil {
		t.Fatalf("insert legacy dxd soldier: %v", err)
	}

	id, err := d.NextDXDID()
	if err != nil {
		t.Fatalf("NextDXDID: %v", err)
	}
	if id != "DXD-00008" {
		t.Fatalf("expected DXD-00008, got %s", id)
	}
}

func TestBuildUserNodePrefix(t *testing.T) {
	prefix, err := BuildUserNodePrefix("Samuel", "Thomas", "Carter", 1838)
	if err != nil {
		t.Fatalf("BuildUserNodePrefix: %v", err)
	}
	if prefix != "STC38" {
		t.Fatalf("prefix = %q", prefix)
	}
}

func TestIdentitySetupRequiredForFreshDatabase(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	required, err := d.IdentitySetupRequired()
	if err != nil {
		t.Fatalf("IdentitySetupRequired: %v", err)
	}
	if !required {
		t.Fatal("expected fresh database to require identity setup")
	}

	identity, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838)
	if err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	if identity.NodePrefix != "STC38" {
		t.Fatalf("node prefix = %q", identity.NodePrefix)
	}

	required, err = d.IdentitySetupRequired()
	if err != nil {
		t.Fatalf("IdentitySetupRequired after configure: %v", err)
	}
	if required {
		t.Fatal("expected configured database not to require identity setup")
	}
}

// TestNextEventID_Format (issue #320) verifies that NextEventID mints
// EVT-00001 on a fresh archive. Mirrors TestNextDXDID_Format.
func TestNextEventID_Format(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	id, err := d.NextEventID()
	if err != nil {
		t.Fatalf("NextEventID: %v", err)
	}
	if id != "EVT-00001" {
		t.Errorf("expected EVT-00001, got %s", id)
	}
}

// TestNextEventID_Increment verifies that NextEventID advances past
// pre-existing EVT-NNNNN rows.
func TestNextEventID_Increment(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Insert some pre-existing EVT- rows
	for i := 0; i < 5; i++ {
		displayID := fmt.Sprintf("EVT-%05d", i+1)
		_, err := d.conn.Exec(
			`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES (?, 1, 'event')`,
			displayID,
		)
		if err != nil {
			t.Fatalf("insert event %d: %v", i+1, err)
		}
	}

	id, err := d.NextEventID()
	if err != nil {
		t.Fatalf("NextEventID: %v", err)
	}
	if id != "EVT-00006" {
		t.Errorf("expected EVT-00006, got %s", id)
	}
}

// TestNextEventID_NamespaceIndependent verifies that DXD- rows do not
// affect the EVT- counter. Mirrors TestNextDXDID_NonGeneratedIgnored
// for cross-namespace isolation.
func TestNextEventID_NamespaceIndependent(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Insert Person Records in the DXD- namespace
	for i := 0; i < 3; i++ {
		displayID := fmt.Sprintf("DXD-%05d", i+1)
		_, err := d.conn.Exec(
			`INSERT INTO soldiers (display_id, is_generated, entry_type) VALUES (?, 1, 'soldier')`,
			displayID,
		)
		if err != nil {
			t.Fatalf("insert soldier %d: %v", i+1, err)
		}
	}

	id, err := d.NextEventID()
	if err != nil {
		t.Fatalf("NextEventID: %v", err)
	}
	if id != "EVT-00001" {
		t.Errorf("expected EVT-00001 (DXD- rows do not affect EVT- counter), got %s", id)
	}
}

// TestNextEventID_RoundTrip verifies the SanitizeID / CanonicalDisplayID
// helpers handle the EVT- namespace correctly.
func TestNextEventID_RoundTrip(t *testing.T) {
	// SanitizeID preserves the EVT- prefix
	if got := SanitizeID("EVT-00042", ""); got != "EVT-00042" {
		t.Errorf("SanitizeID(EVT-00042) = %q, want EVT-00042", got)
	}
	// CanonicalDisplayID parses EVT-00042 → ("EVT", 42, true)
	namespace, seq, ok := CanonicalDisplayID("EVT-00042")
	if !ok {
		t.Fatal("CanonicalDisplayID(EVT-00042) returned ok=false")
	}
	if namespace != "EVT" {
		t.Errorf("namespace = %q, want EVT", namespace)
	}
	if seq != 42 {
		t.Errorf("seq = %d, want 42", seq)
	}
}
