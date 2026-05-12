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
