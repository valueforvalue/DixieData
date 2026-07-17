// person_record_repo_writes_test.go — issue #613 slice 2
// contract tests for the PersonRecordRepo write methods.
//
// Pins the contract that Create / Update / Delete execute
// the expected SQL + return the expected result codes against
// an in-memory SQLite. The transactional semantic (caller
// passes a *sql.Tx) is verified by wrapping each call in a
// tx inside the test.
//
// The slice-1 contract tests cover GetByID + List. This file
// covers the other three methods on the same interface.
package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestPersonRecordRepo_Create_ReturnsInsertID asserts Create
// inserts a row + returns the new id via LastInsertId.
func TestPersonRecordRepo_Create_ReturnsInsertID(t *testing.T) {
	d := newTestDB(t)
	r := NewPersonRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.Soldier{
		DisplayID: "P-0001",
		FirstName: "John",
		LastName:  "Doe",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id <= 0 {
		t.Errorf("Create returned id = %d, want positive int64", id)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Verify the row is now readable via the slice-1 GetByID path.
	row, err := r.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID after Create: %v", err)
	}
	dest := make([]any, len(strings.Split(PersonRecordSelectColumns, ",")))
	for i := range dest {
		dest[i] = new(sql.NullString)
	}
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan row: %v", err)
	}
	// Column 1 (1-indexed) is display_id.
	displayID, _ := dest[1].(*sql.NullString)
	if displayID == nil || displayID.String != "P-0001" {
		t.Errorf("display_id = %v, want %q", displayID, "P-0001")
	}
}

// TestPersonRecordRepo_Update_AffectsOneRow asserts Update
// modifies the row + returns rowsAffected=1.
func TestPersonRecordRepo_Update_AffectsOneRow(t *testing.T) {
	d := newTestDB(t)
	r := NewPersonRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.Soldier{
		DisplayID: "P-0002",
		FirstName: "Jane",
		LastName:  "Doe",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	affected, err := r.Update(context.Background(), tx, models.Soldier{
		ID:        id,
		DisplayID: "P-0002",
		FirstName: "Janet",
		LastName:  "Doe",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if affected != 1 {
		t.Errorf("Update rowsAffected = %d, want 1", affected)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	row, err := r.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID after Update: %v", err)
	}
	dest := make([]any, len(strings.Split(PersonRecordSelectColumns, ",")))
	for i := range dest {
		dest[i] = new(sql.NullString)
	}
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan row: %v", err)
	}
	// Column 13 (1-indexed) is first_name.
	firstName, _ := dest[12].(*sql.NullString)
	if firstName == nil || firstName.String != "Janet" {
		t.Errorf("first_name after update = %v, want %q", firstName, "Janet")
	}
}

// TestPersonRecordRepo_Update_NotFound asserts Update on a
// missing id returns rowsAffected=0 + no error. The service
// layer is responsible for translating this to
// ErrSoldierNotFound; the repo just reports what happened.
func TestPersonRecordRepo_Update_NotFound(t *testing.T) {
	d := newTestDB(t)
	r := NewPersonRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	affected, err := r.Update(context.Background(), tx, models.Soldier{
		ID:        9999,
		DisplayID: "P-9999",
		FirstName: "Ghost",
	})
	if err != nil {
		t.Fatalf("Update on missing id: %v", err)
	}
	if affected != 0 {
		t.Errorf("Update rowsAffected = %d, want 0", affected)
	}
}

// TestPersonRecordRepo_Delete_AffectsOneRow asserts Delete
// removes the row + returns rowsAffected=1.
func TestPersonRecordRepo_Delete_AffectsOneRow(t *testing.T) {
	d := newTestDB(t)
	r := NewPersonRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.Soldier{
		DisplayID: "P-0003",
		FirstName: "Delete",
		LastName:  "Me",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// New tx for the delete (the create tx is committed).
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
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit tx2: %v", err)
	}

	// GetByID should now return sql.ErrNoRows.
	row, err := r.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID after Delete: %v", err)
	}
	dest := make([]any, len(strings.Split(PersonRecordSelectColumns, ",")))
	for i := range dest {
		dest[i] = new(sql.NullString)
	}
	scanErr := row.Scan(dest...)
	if scanErr != sql.ErrNoRows {
		t.Errorf("Scan after delete: err = %v, want sql.ErrNoRows", scanErr)
	}
}

// TestPersonRecordRepo_Delete_NotFound asserts Delete on a
// missing id returns rowsAffected=0 + no error.
func TestPersonRecordRepo_Delete_NotFound(t *testing.T) {
	d := newTestDB(t)
	r := NewPersonRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	affected, err := r.Delete(context.Background(), tx, 9999)
	if err != nil {
		t.Fatalf("Delete on missing id: %v", err)
	}
	if affected != 0 {
		t.Errorf("Delete rowsAffected = %d, want 0", affected)
	}
}