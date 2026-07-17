// Package sqlite provides the SQLite-backed implementation of the
// repository interfaces declared in internal/db/repo.
//
// This file pins the contract for PersonRecordRepo (issue #613
// slice 1). Tests run against an in-memory SQLite database seeded
// with the canonical `soldiers` schema, exercising every repo
// method the consumer (SoldierService) calls in slice 1.
package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
	_ "modernc.org/sqlite"
)

// newTestDB opens an in-memory SQLite database, applies the
// schema.sql body inline (the slice-1 minimal schema: just the
// `soldiers` table), and returns a ready *db.DB.
//
// We do NOT use the full db.Open() because that would apply every
// schema migration + open a real file + run busy_timeout pragmas.
// The in-memory path is hermetic; tests must not touch disk.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open in-memory: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	// Minimal soldiers schema for slice-1 tests. Mirrors the
	// relevant columns from internal/db/schema.go (the slice-1
	// column list matches soldierSelectColumns). Future
	// columns land in slice 2+; this schema is the smallest
	// shape that satisfies PersonRecordRepo.GetByID/List.
	const schema = `
		CREATE TABLE soldiers (
			id INTEGER PRIMARY KEY,
			display_id TEXT NOT NULL,
			sync_id TEXT NOT NULL DEFAULT '',
			entry_type TEXT NOT NULL DEFAULT 'soldier',
			spouse_soldier_id INTEGER,
			relationship_label TEXT NOT NULL DEFAULT '',
			maiden_name TEXT NOT NULL DEFAULT '',
			is_generated INTEGER NOT NULL DEFAULT 0,
			pension_id TEXT NOT NULL DEFAULT '',
			application_id TEXT NOT NULL DEFAULT '',
			prefix TEXT NOT NULL DEFAULT '',
			show_prefix_before_name INTEGER NOT NULL DEFAULT 0,
			first_name TEXT NOT NULL DEFAULT '',
			middle_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			suffix TEXT NOT NULL DEFAULT '',
			rank TEXT NOT NULL DEFAULT '',
			rank_in TEXT NOT NULL DEFAULT '',
			rank_out TEXT NOT NULL DEFAULT '',
			unit TEXT NOT NULL DEFAULT '',
			pension_state TEXT NOT NULL DEFAULT '',
			confederate_home_status TEXT NOT NULL DEFAULT 'N/A',
			confederate_home_name TEXT NOT NULL DEFAULT '',
			death_year INTEGER,
			death_month INTEGER,
			death_day INTEGER,
			birth_date TEXT NOT NULL DEFAULT '',
			death_date TEXT NOT NULL DEFAULT '',
			birth_info TEXT NOT NULL DEFAULT '',
			buried_in TEXT NOT NULL DEFAULT '',
			biography TEXT NOT NULL DEFAULT '',
			pdf_excerpt_override TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			needs_review INTEGER NOT NULL DEFAULT 0,
			review_reason TEXT NOT NULL DEFAULT '',
			added_by TEXT NOT NULL DEFAULT '',
			last_edited_by TEXT NOT NULL DEFAULT '',
			last_edited_fields TEXT NOT NULL DEFAULT '',
			last_edited_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			begin_date TEXT NOT NULL DEFAULT '',
			end_date TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_by_version INTEGER NOT NULL DEFAULT 0,
			created_by_import_path TEXT NOT NULL DEFAULT '',
			restored_at TEXT NOT NULL DEFAULT ''
		);
		-- Cross-table subqueries in PersonRecordListSelectColumns
		-- reference these tables. They are empty in slice-1
		-- tests; the contract is "the SQL is valid + the
		-- scan dest shape is honored".
		CREATE TABLE records (
			id INTEGER PRIMARY KEY,
			person_record_id INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE images (
			id INTEGER PRIMARY KEY,
			person_record_id INTEGER NOT NULL DEFAULT 0
		);
	`
	if _, err := conn.Exec(schema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	return db.NewFromExisting(conn)
}

// seedSoldier inserts one canonical soldier row and returns the
// generated id. Used by GetByID/List tests.
func seedSoldier(t *testing.T, d *db.DB, displayID, first, last string) int64 {
	t.Helper()
	res, err := d.Conn().Exec(
		`INSERT INTO soldiers (display_id, first_name, last_name) VALUES (?, ?, ?)`,
		displayID, first, last,
	)
	if err != nil {
		t.Fatalf("seed soldier: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// TestPersonRecordRepo_GetByID asserts the repo returns a row
// that scans into a models.Soldier with the expected fields.
//
// RED→GREEN progression: this test is the first contract check.
// It pins the column list + scan shape + Scan() error handling.
func TestPersonRecordRepo_GetByID(t *testing.T) {
	d := newTestDB(t)
	id := seedSoldier(t, d, "P-0001", "John", "Doe")

	r := NewPersonRecordRepo(d)
	row, err := r.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID(%d): unexpected err = %v", id, err)
	}
	if row == nil {
		t.Fatalf("GetByID(%d): returned nil row", id)
	}

	var (
		gotID          int64
		gotDisplayID   string
		gotFirstName   string
		gotLastName    string
		gotEntryType   string
		gotHomeStatus  string
	)
	if err := row.Scan(
		&gotID, &gotDisplayID, new(string), &gotEntryType, new(sql.NullInt64),
		new(string), new(string), new(int64), new(string), new(string),
		new(string), new(int64), &gotFirstName, new(string), &gotLastName, new(string),
		new(string), new(string), new(string), new(string), new(string),
		&gotHomeStatus, new(string), new(sql.NullInt64), new(sql.NullInt64), new(sql.NullInt64),
		new(string), new(string), new(string), new(string), new(string),
		new(string), new(string), new(string), new(string), new(string),
		new(string), new(string), new(string), new(string), new(string),
		new(string), new(string), new(string), new(string), new(int64),
		new(string), new(string),
	); err != nil {
		t.Fatalf("scan row: %v", err)
	}
	if gotID != id {
		t.Errorf("id = %d, want %d", gotID, id)
	}
	if gotDisplayID != "P-0001" {
		t.Errorf("display_id = %q, want %q", gotDisplayID, "P-0001")
	}
	if gotFirstName != "John" {
		t.Errorf("first_name = %q, want %q", gotFirstName, "John")
	}
	if gotLastName != "Doe" {
		t.Errorf("last_name = %q, want %q", gotLastName, "Doe")
	}
	if gotEntryType != "soldier" {
		t.Errorf("entry_type = %q, want %q (default)", gotEntryType, "soldier")
	}
	if gotHomeStatus != "N/A" {
		t.Errorf("confederate_home_status = %q, want %q (default)", gotHomeStatus, "N/A")
	}
}

// TestPersonRecordRepo_GetByID_NotFound asserts an unknown id
// surfaces sql.ErrNoRows when the caller scans the row. The
// caller (SoldierService.GetByID) maps this to
// ErrSoldierNotFound; the repo just returns the row + lets the
// caller's Scan() surface the error.
//
// QueryRow's behavior: ErrNoRows is deferred until Scan() is
// called. So the test scans into a discard dest and asserts the
// error returned by Scan.
func TestPersonRecordRepo_GetByID_NotFound(t *testing.T) {
	d := newTestDB(t)
	r := NewPersonRecordRepo(d)
	row, err := r.GetByID(context.Background(), 9999)
	if err != nil {
		t.Fatalf("GetByID: unexpected err = %v", err)
	}
	if row == nil {
		t.Fatalf("GetByID: returned nil row")
	}
	// Scan with enough discard destinations to cover all
	// columns; only the err matters here.
	dest := make([]any, len(strings.Split(PersonRecordSelectColumns, ",")))
	for i := range dest {
		dest[i] = new(sql.NullString)
	}
	scanErr := row.Scan(dest...)
	if scanErr != sql.ErrNoRows {
		t.Errorf("Scan err = %v, want sql.ErrNoRows", scanErr)
	}
}

// TestPersonRecordRepo_List_PageOne asserts List returns the
// paginated soldiers + the correct total count.
func TestPersonRecordRepo_List_PageOne(t *testing.T) {
	d := newTestDB(t)
	ids := make([]int64, 0, 25)
	for i := 0; i < 25; i++ {
		ids = append(ids, seedSoldier(t, d,
			// Order matters: List sorts by last_name, first_name.
			// Seed so each row has a unique sort key.
			"P-"+itoa(i+1),
			"First"+itoa(i),
			"Last"+itoa(i),
		))
	}

	r := NewPersonRecordRepo(d)
	rows, total, err := r.List(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("List: unexpected err = %v", err)
	}
	if total != 25 {
		t.Errorf("total = %d, want 25", total)
	}
	if rows == nil {
		t.Fatalf("List: returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if count != 10 {
		t.Errorf("page 1 row count = %d, want 10", count)
	}
}

// TestPersonRecordRepo_List_PageThree asserts the OFFSET works.
// Page 3 of 25 rows @ 10/page = rows 21-25 = 5 rows.
func TestPersonRecordRepo_List_PageThree(t *testing.T) {
	d := newTestDB(t)
	for i := 0; i < 25; i++ {
		seedSoldier(t, d, "P-"+itoa(i+1), "First"+itoa(i), "Last"+itoa(i))
	}

	r := NewPersonRecordRepo(d)
	rows, total, err := r.List(context.Background(), 3, 10)
	if err != nil {
		t.Fatalf("List: unexpected err = %v", err)
	}
	if total != 25 {
		t.Errorf("total = %d, want 25", total)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 5 {
		t.Errorf("page 3 row count = %d, want 5", count)
	}
}

// TestPersonRecordRepo_List_Empty asserts an empty table returns
// total=0 and a non-nil rows handle that yields zero rows.
func TestPersonRecordRepo_List_Empty(t *testing.T) {
	d := newTestDB(t)
	r := NewPersonRecordRepo(d)
	rows, total, err := r.List(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("List: unexpected err = %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if rows == nil {
		t.Fatalf("List: returned nil rows on empty table")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if count != 0 {
		t.Errorf("empty table row count = %d, want 0", count)
	}
}

// itoa is a tiny int-to-string helper for test fixture building.
// Inlined here rather than importing strconv so the test reads
// at one glance.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// Touch repo package import (compile-time anchor; ensures the
// repo package itself is part of the dependency graph so go vet
// + go build don't drop it).
var _ repo.PersonRecordRepo = (*PersonRecordRepo)(nil)

// filepath import anchor (used by future slices; placeholder so
// this test file is self-contained when other slices add helpers
// that need temp-dir support).
var _ = filepath.Join