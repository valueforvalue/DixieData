package db

import (
	"fmt"
	"testing"
)

func TestNextCSAID_Format(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	id, err := d.NextCSAID()
	if err != nil {
		t.Fatalf("NextCSAID: %v", err)
	}
	if id != "CSA-000001" {
		t.Errorf("expected CSA-000001, got %s", id)
	}
}

func TestNextCSAID_Increment(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Insert some generated soldiers
	for i := 0; i < 5; i++ {
		displayID := fmt.Sprintf("CSA-%06d", i+1)
		_, err := d.conn.Exec(
			`INSERT INTO soldiers (display_id, is_generated) VALUES (?, 1)`,
			displayID,
		)
		if err != nil {
			t.Fatalf("insert soldier %d: %v", i+1, err)
		}
	}

	id, err := d.NextCSAID()
	if err != nil {
		t.Fatalf("NextCSAID: %v", err)
	}
	if id != "CSA-000006" {
		t.Errorf("expected CSA-000006, got %s", id)
	}
}

func TestNextCSAID_NonGeneratedIgnored(t *testing.T) {
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

	id, err := d.NextCSAID()
	if err != nil {
		t.Fatalf("NextCSAID: %v", err)
	}
	if id != "CSA-000001" {
		t.Errorf("expected CSA-000001 (non-generated ignored), got %s", id)
	}
}
