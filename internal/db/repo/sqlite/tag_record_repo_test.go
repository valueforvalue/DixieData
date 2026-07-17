// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file pins the contract for TagRecordRepo (issue #613
// slice 5). Tags live in two tables: `tags` (the canonical
// tag list) + `person_record_tags` (the per-Person-Record
// many-to-many junction). Slice 5 covers 4 methods: the
// most-common write (UpsertByName), the two attach/detach
// methods on the junction, and the list read.
package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// newTagTestDB opens an in-memory SQLite with the slice-5
// tag schema (tags + person_record_tags). The test schema
// mirrors the canonical columns; the service-level tests
// exercise the slice-5 repo delegation.
func newTagTestDB(t *testing.T) *db.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open in-memory: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	const schema = `
		CREATE TABLE tags (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			name            TEXT NOT NULL,
			normalized_name TEXT NOT NULL UNIQUE
		);
		CREATE TABLE person_record_tags (
			person_id INTEGER NOT NULL DEFAULT 0,
			tag_id    INTEGER NOT NULL DEFAULT 0,
			UNIQUE (person_id, tag_id)
		);
	`
	if _, err := conn.Exec(schema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db.NewFromExisting(conn)
}

// TestTagRecordRepo_UpsertByName_Insert asserts UpsertByName
// inserts a new tag + returns the generated id.
func TestTagRecordRepo_UpsertByName_Insert(t *testing.T) {
	d := newTagTestDB(t)
	r := NewTagRecordRepo(d)
	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.UpsertByName(context.Background(), tx, "Civil War")
	if err != nil {
		t.Fatalf("UpsertByName: %v", err)
	}
	if id <= 0 {
		t.Errorf("UpsertByName returned id = %d, want positive", id)
	}
}

// TestTagRecordRepo_UpsertByName_Idempotent asserts
// UpsertByName on an existing tag returns the existing id
// (INSERT OR IGNORE via the unique index on normalized_name).
func TestTagRecordRepo_UpsertByName_Idempotent(t *testing.T) {
	d := newTagTestDB(t)
	r := NewTagRecordRepo(d)
	conn := d.Conn()

	// First upsert: tx1 commits.
	tx1, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin tx1: %v", err)
	}
	id1, err := r.UpsertByName(context.Background(), tx1, "Civil War")
	if err != nil {
		t.Fatalf("first UpsertByName: %v", err)
	}
	if err := tx1.Commit(); err != nil {
		t.Fatalf("Commit tx1: %v", err)
	}

	// Second upsert: same name, expect same id.
	tx2, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin tx2: %v", err)
	}
	defer tx2.Rollback()
	id2, err := r.UpsertByName(context.Background(), tx2, "Civil War")
	if err != nil {
		t.Fatalf("second UpsertByName: %v", err)
	}
	if id1 != id2 {
		t.Errorf("UpsertByName returned id %d on second call, want %d (idempotent)", id2, id1)
	}
}

// TestTagRecordRepo_Attach_InsertsJunction asserts Attach
// inserts the person_record_tags row + is idempotent.
func TestTagRecordRepo_Attach_InsertsJunction(t *testing.T) {
	d := newTagTestDB(t)
	r := NewTagRecordRepo(d)
	conn := d.Conn()

	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	tagID, err := r.UpsertByName(context.Background(), tx, "FindAGrave")
	if err != nil {
		t.Fatalf("UpsertByName: %v", err)
	}

	if err := r.Attach(context.Background(), tx, tagID, 42); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Second attach on the same pair is a no-op (UNIQUE +
	// INSERT OR IGNORE).
	if err := r.Attach(context.Background(), tx, tagID, 42); err != nil {
		t.Errorf("second Attach: %v (expected no-op)", err)
	}
}

// TestTagRecordRepo_Detach_RemovesJunction asserts Detach
// removes the junction row + returns rowsAffected=1.
func TestTagRecordRepo_Detach_RemovesJunction(t *testing.T) {
	d := newTagTestDB(t)
	r := NewTagRecordRepo(d)
	conn := d.Conn()

	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	tagID, _ := r.UpsertByName(context.Background(), tx, "FindAGrave")
	if err := r.Attach(context.Background(), tx, tagID, 42); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin tx2: %v", err)
	}
	defer tx2.Rollback()
	affected, err := r.Detach(context.Background(), tx2, tagID, 42)
	if err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if affected != 1 {
		t.Errorf("Detach rowsAffected = %d, want 1", affected)
	}
}

// TestTagRecordRepo_Detach_NotAttached asserts Detach on a
// missing junction row returns rowsAffected=0 with no error.
func TestTagRecordRepo_Detach_NotAttached(t *testing.T) {
	d := newTagTestDB(t)
	r := NewTagRecordRepo(d)
	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	tagID, _ := r.UpsertByName(context.Background(), tx, "FindAGrave")
	affected, err := r.Detach(context.Background(), tx, tagID, 42)
	if err != nil {
		t.Fatalf("Detach on missing: %v", err)
	}
	if affected != 0 {
		t.Errorf("Detach rowsAffected = %d, want 0", affected)
	}
}