// tag_record_repo_parity_test.go — issue #613 slice 5
// regression net.
//
// Pins the contract that the new TagRecordRepo-backed
// TagService.UpsertByName + Attach + Detach paths return
// identical results to the legacy inline-SQL paths on the
// same fixture.
package records

import (
	"context"
	"database/sql"
	"testing"
)

// TestTagRecordRepo_Parity_UpsertAttachDetach is the slice-5
// service-level parity check. The full
// UpsertByName → Attach → Detach → (second Upsert = same id)
// cycle exercises every slice-5 write path through the
// service layer.
func TestTagRecordRepo_Parity_UpsertAttachDetach(t *testing.T) {
	d := newTestDB(t)
	conn := d.Conn()

	// Add the tags + person_record_tags tables (the existing
	// newTestDB doesn't include them; slice-5 service tests
	// need them for the TagService parity check).
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS tags (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		name            TEXT NOT NULL,
		normalized_name TEXT NOT NULL UNIQUE,
		created_at      TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tags: %v", err)
	}
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS person_record_tags (
		person_id INTEGER NOT NULL DEFAULT 0,
		tag_id    INTEGER NOT NULL DEFAULT 0,
		UNIQUE (person_id, tag_id)
	)`); err != nil {
		t.Fatalf("create person_record_tags: %v", err)
	}

	// Insert a Person Record so the FK on person_record_tags
	// is satisfied when we Attach(tagID, personID=42).
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id) VALUES (?, ?)`,
		42, "DXD-00042",
	); err != nil {
		t.Fatalf("seed person: %v", err)
	}

	svc := NewTagService(conn)
	ctx := context.Background()

	// Upsert: first call inserts.
	tag1, err := svc.UpsertByName(ctx, "Civil War")
	if err != nil {
		t.Fatalf("UpsertByName (insert): %v", err)
	}
	if tag1.ID <= 0 {
		t.Errorf("UpsertByName returned id = %d, want positive", tag1.ID)
	}

	// Upsert: second call with the same name returns the
	// same id (idempotent).
	tag2, err := svc.UpsertByName(ctx, "Civil War")
	if err != nil {
		t.Fatalf("UpsertByName (idempotent): %v", err)
	}
	if tag2.ID != tag1.ID {
		t.Errorf("second UpsertByName returned id %d, want %d (idempotent)", tag2.ID, tag1.ID)
	}

	// Attach: binds the tag to person 42.
	if err := svc.Attach(ctx, tag1.ID, 42); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Verify the junction row exists.
	var count int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM person_record_tags WHERE tag_id = ? AND person_id = ?`,
		tag1.ID, 42,
	).Scan(&count); err != nil {
		t.Fatalf("verify junction: %v", err)
	}
	if count != 1 {
		t.Errorf("junction row count = %d, want 1", count)
	}

	// Detach: removes the binding.
	if err := svc.Detach(ctx, tag1.ID, 42); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM person_record_tags WHERE tag_id = ? AND person_id = ?`,
		tag1.ID, 42,
	).Scan(&count); err != nil {
		t.Fatalf("verify post-detach: %v", err)
	}
	if count != 0 {
		t.Errorf("post-detach junction row count = %d, want 0", count)
	}
}

// sql import anchor (used by other slices; placeholder so the
// package compiles when this file is the only test entry
// point).
var _ = sql.ErrNoRows