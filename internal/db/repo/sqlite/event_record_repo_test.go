// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file pins the contract for EventRecordRepo (issue #613
// slice 3). Event Records live in the `soldiers` table with
// `entry_type = 'event'`; the `event_person_links` junction
// connects Events to Person Records.
//
// Slice 3 scope (per the approved plan): 5 methods covering
// the highest-coupling inline SQL in EventService:
//   - ListEvents: paginated Event read on soldiers
//   - LinksForEvent: junction read for one event
//   - AttachEventToPerson: junction insert
//   - DetachEventFromPerson: junction delete
//   - ListForPerson: reverse junction read
package sqlite

import (
	"context"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// TestEventRecordRepo_ListEvents asserts the paginated Event
// read returns only Event rows (entry_type='event') + the
// correct total count.
func TestEventRecordRepo_ListEvents(t *testing.T) {
	d := newTestDB(t)
	// Seed: 2 Event rows + 1 Soldier row (should be excluded).
	seedEvent(t, d, "EVT-0001", "Battle")
	seedEvent(t, d, "EVT-0002", "Campaign")
	seedSoldier(t, d, "P-0001", "John", "Doe")

	r := NewEventRecordRepo(d)
	rows, total, err := r.ListEvents(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 (Events only)", total)
	}
	if rows == nil {
		t.Fatalf("ListEvents: returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 2 {
		t.Errorf("rows iterated = %d, want 2", count)
	}
}

// TestEventRecordRepo_LinksForEvent asserts the junction read
// returns rows in (sort_order, id) order — the order matters
// for the Event detail page render.
func TestEventRecordRepo_LinksForEvent(t *testing.T) {
	d := newTestDB(t)
	eventID := seedEvent(t, d, "EVT-0001", "Battle")
	p1 := seedSoldier(t, d, "P-0001", "John", "Doe")
	p2 := seedSoldier(t, d, "P-0002", "Jane", "Doe")

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	r := NewEventRecordRepo(d)
	if _, err := r.AttachEventToPerson(context.Background(), tx, eventID, p1, "sync-1", "evt-sync", "p1-sync"); err != nil {
		t.Fatalf("AttachEventToPerson p1: %v", err)
	}
	if _, err := r.AttachEventToPerson(context.Background(), tx, eventID, p2, "sync-2", "evt-sync", "p2-sync"); err != nil {
		t.Fatalf("AttachEventToPerson p2: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rows, err := r.LinksForEvent(context.Background(), conn, eventID)
	if err != nil {
		t.Fatalf("LinksForEvent: %v", err)
	}
	if rows == nil {
		t.Fatalf("LinksForEvent: returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 2 {
		t.Errorf("rows iterated = %d, want 2", count)
	}
}

// TestEventRecordRepo_AttachEventToPerson_Duplicate asserts
// that attaching the same (event, person) pair twice is a
// no-op (the unique index prevents a duplicate INSERT). The
// repo surfaces this as a SQLite constraint error.
func TestEventRecordRepo_AttachEventToPerson_Duplicate(t *testing.T) {
	d := newTestDB(t)
	eventID := seedEvent(t, d, "EVT-0001", "Battle")
	personID := seedSoldier(t, d, "P-0001", "John", "Doe")

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	r := NewEventRecordRepo(d)
	if _, err := r.AttachEventToPerson(context.Background(), tx, eventID, personID, "sync-1", "evt-sync", "p-sync"); err != nil {
		t.Fatalf("first AttachEventToPerson: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Second attach should fail (UNIQUE(event_id, person_id) violation).
	tx2, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin tx2: %v", err)
	}
	defer tx2.Rollback()

	_, err = r.AttachEventToPerson(context.Background(), tx2, eventID, personID, "sync-2", "evt-sync", "p-sync")
	if err == nil {
		t.Errorf("second AttachEventToPerson: err = nil, want constraint violation")
	}
}

// TestEventRecordRepo_DetachEventToPerson asserts DELETE
// removes the junction row + returns rowsAffected=1.
func TestEventRecordRepo_DetachEventToPerson(t *testing.T) {
	d := newTestDB(t)
	eventID := seedEvent(t, d, "EVT-0001", "Battle")
	personID := seedSoldier(t, d, "P-0001", "John", "Doe")

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	r := NewEventRecordRepo(d)
	if _, err := r.AttachEventToPerson(context.Background(), tx, eventID, personID, "sync-1", "evt-sync", "p-sync"); err != nil {
		t.Fatalf("AttachEventToPerson: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin tx2: %v", err)
	}
	defer tx2.Rollback()

	affected, err := r.DetachEventFromPerson(context.Background(), tx2, eventID, personID)
	if err != nil {
		t.Fatalf("DetachEventFromPerson: %v", err)
	}
	if affected != 1 {
		t.Errorf("DetachEventFromPerson rowsAffected = %d, want 1", affected)
	}
}

// TestEventRecordRepo_DetachEventToPerson_NotLinked asserts
// DELETE on a non-existent link returns rowsAffected=0 with
// no error (matches the Update/Delete silent-no-op pattern
// from slice 2).
func TestEventRecordRepo_DetachEventToPerson_NotLinked(t *testing.T) {
	d := newTestDB(t)
	eventID := seedEvent(t, d, "EVT-0001", "Battle")
	personID := seedSoldier(t, d, "P-0001", "John", "Doe")

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	r := NewEventRecordRepo(d)
	affected, err := r.DetachEventFromPerson(context.Background(), tx, eventID, personID)
	if err != nil {
		t.Fatalf("DetachEventFromPerson on missing link: %v", err)
	}
	if affected != 0 {
		t.Errorf("DetachEventFromPerson rowsAffected = %d, want 0", affected)
	}
}

// TestEventRecordRepo_ListForPerson asserts the reverse
// junction read returns the Events linked to a Person.
func TestEventRecordRepo_ListForPerson(t *testing.T) {
	d := newTestDB(t)
	eventID1 := seedEvent(t, d, "EVT-0001", "Battle")
	eventID2 := seedEvent(t, d, "EVT-0002", "Campaign")
	personID := seedSoldier(t, d, "P-0001", "John", "Doe")

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	r := NewEventRecordRepo(d)
	if _, err := r.AttachEventToPerson(context.Background(), tx, eventID1, personID, "sync-1", "e1-sync", "p-sync"); err != nil {
		t.Fatalf("AttachEventToPerson e1: %v", err)
	}
	if _, err := r.AttachEventToPerson(context.Background(), tx, eventID2, personID, "sync-2", "e2-sync", "p-sync"); err != nil {
		t.Fatalf("AttachEventToPerson e2: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rows, err := r.ListForPerson(context.Background(), conn, personID)
	if err != nil {
		t.Fatalf("ListForPerson: %v", err)
	}
	if rows == nil {
		t.Fatalf("ListForPerson: returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 2 {
		t.Errorf("rows iterated = %d, want 2", count)
	}
}

// seedEvent inserts one Event row (entry_type='event') into
// the soldiers table and returns the generated id.
func seedEvent(t *testing.T, d *db.DB, displayID, kind string) int64 {
	t.Helper()
	res, err := d.Conn().Exec(
		`INSERT INTO soldiers (display_id, entry_type, kind) VALUES (?, 'event', ?)`,
		displayID, kind,
	)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}