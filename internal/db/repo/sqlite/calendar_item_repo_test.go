// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file pins the contract for CalendarItemRepo (issue #613
// slice 6). Calendar items live in the `calendar_items` table
// (per-month, per-day entries the /calendar page renders).
// Slice 6 covers 5 methods: the read paths (GetByID,
// ListForMonthDay) and the write paths (Create, Update,
// Delete).
package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// newCalendarTestDB opens an in-memory SQLite with the
// slice-6 calendar_items schema.
func newCalendarTestDB(t *testing.T) *db.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open in-memory: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	const schema = `
		CREATE TABLE calendar_items (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			item_type  TEXT NOT NULL DEFAULT '',
			month      INTEGER NOT NULL DEFAULT 0,
			day        INTEGER NOT NULL DEFAULT 0,
			title      TEXT NOT NULL DEFAULT '',
			notes      TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		);
	`
	if _, err := conn.Exec(schema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db.NewFromExisting(conn)
}

// TestCalendarItemRepo_Create_ReturnsInsertID asserts Create
// inserts a row + returns the generated id.
func TestCalendarItemRepo_Create_ReturnsInsertID(t *testing.T) {
	d := newCalendarTestDB(t)
	r := NewCalendarItemRepo(d)
	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.CalendarItem{
		ItemType: "holiday",
		Month:    7,
		Day:      4,
		Title:    "Independence Day",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id <= 0 {
		t.Errorf("Create returned id = %d, want positive", id)
	}
}

// TestCalendarItemRepo_GetByID_NotFound asserts the no-row
// case surfaces sql.ErrNoRows on Scan.
func TestCalendarItemRepo_GetByID_NotFound(t *testing.T) {
	d := newCalendarTestDB(t)
	r := NewCalendarItemRepo(d)
	row, err := r.GetByID(context.Background(), 9999)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if row == nil {
		t.Fatalf("GetByID: returned nil row")
	}
	dest := make([]any, len(strings.Split(CalendarItemSelectColumns, ",")))
	for i := range dest {
		dest[i] = new(sql.NullString)
	}
	if scanErr := row.Scan(dest...); scanErr != sql.ErrNoRows {
		t.Errorf("Scan err = %v, want sql.ErrNoRows", scanErr)
	}
}

// TestCalendarItemRepo_ListForMonthDay asserts the filtered
// list returns only rows for the given (month, day) pair.
func TestCalendarItemRepo_ListForMonthDay(t *testing.T) {
	d := newCalendarTestDB(t)
	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	r := NewCalendarItemRepo(d)

	// Seed: 2 items on July 4 + 1 on December 25.
	if _, err := r.Create(context.Background(), tx, models.CalendarItem{
		ItemType: "holiday", Month: 7, Day: 4, Title: "Independence Day",
	}); err != nil {
		t.Fatalf("Create July 4 a: %v", err)
	}
	if _, err := r.Create(context.Background(), tx, models.CalendarItem{
		ItemType: "event", Month: 7, Day: 4, Title: "Civil War memorial",
	}); err != nil {
		t.Fatalf("Create July 4 b: %v", err)
	}
	if _, err := r.Create(context.Background(), tx, models.CalendarItem{
		ItemType: "holiday", Month: 12, Day: 25, Title: "Christmas",
	}); err != nil {
		t.Fatalf("Create Dec 25: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rows, err := r.ListForMonthDay(context.Background(), conn, 7, 4)
	if err != nil {
		t.Fatalf("ListForMonthDay: %v", err)
	}
	if rows == nil {
		t.Fatalf("ListForMonthDay: returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 2 {
		t.Errorf("rows iterated = %d, want 2 (July 4 only)", count)
	}
}

// TestCalendarItemRepo_Update_AffectsOneRow asserts Update
// modifies the row + returns rowsAffected=1.
func TestCalendarItemRepo_Update_AffectsOneRow(t *testing.T) {
	d := newCalendarTestDB(t)
	r := NewCalendarItemRepo(d)
	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.CalendarItem{
		ItemType: "event", Month: 7, Day: 4, Title: "Original",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	affected, err := r.Update(context.Background(), tx, models.CalendarItem{
		ID: id, ItemType: "event", Month: 7, Day: 4, Title: "Updated",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if affected != 1 {
		t.Errorf("Update rowsAffected = %d, want 1", affected)
	}
}

// TestCalendarItemRepo_Update_NotFound asserts Update on a
// missing id returns rowsAffected=0.
func TestCalendarItemRepo_Update_NotFound(t *testing.T) {
	d := newCalendarTestDB(t)
	r := NewCalendarItemRepo(d)
	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	affected, err := r.Update(context.Background(), tx, models.CalendarItem{
		ID: 9999, ItemType: "event", Month: 7, Day: 4, Title: "Ghost",
	})
	if err != nil {
		t.Fatalf("Update on missing id: %v", err)
	}
	if affected != 0 {
		t.Errorf("Update rowsAffected = %d, want 0", affected)
	}
}

// TestCalendarItemRepo_Delete_AffectsOneRow asserts Delete
// removes the row + returns rowsAffected=1.
func TestCalendarItemRepo_Delete_AffectsOneRow(t *testing.T) {
	d := newCalendarTestDB(t)
	r := NewCalendarItemRepo(d)
	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.CalendarItem{
		ItemType: "event", Month: 7, Day: 4, Title: "Delete me",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin tx2: %v", err)
	}
	defer tx2.Rollback()
	affected, err := r.Delete(context.Background(), tx2, id)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if affected != 1 {
		t.Errorf("Delete rowsAffected = %d, want 1", affected)
	}
}