// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file pins the contract for AuditRecordRepo (issue #613
// slice 7). Schema mirrors the canonical
// duplicate_audit_findings table (left_record_id /
// right_record_id + status enum). Slice 7 covers 2 read
// methods; resolve methods stay in AuditService.
package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// newAuditTestDB opens an in-memory SQLite with the
// slice-7 audit schema (canonical shape).
func newAuditTestDB(t *testing.T) *db.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sql.Open in-memory: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	const schema = `
		CREATE TABLE duplicate_audit_findings (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			pair_key         TEXT UNIQUE NOT NULL,
			left_record_id   INTEGER NOT NULL DEFAULT 0,
			right_record_id  INTEGER NOT NULL DEFAULT 0,
			finding_type     TEXT NOT NULL DEFAULT '',
			reason           TEXT NOT NULL DEFAULT '',
			highlight_fields TEXT NOT NULL DEFAULT '',
			status           TEXT NOT NULL DEFAULT 'open',
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_detected_at DATETIME,
			resolved_at      DATETIME
		);
		-- soldiers table for the slice-8 JOIN-shaped read.
		-- Minimal column shape (id + display_id is all the
		-- legacy ListResolvedFindings LEFT JOIN needs).
		CREATE TABLE soldiers (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			display_id TEXT NOT NULL DEFAULT ''
		);
	`
	// Shared cache so multiple connections in the *sql.DB
	// pool see the same database.
	if _, err := conn.Exec(schema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db.NewFromExisting(conn)
}

// seedFinding inserts one open duplicate_audit_finding row
// and returns its id. left record id defaults to the
// given recordID; right record id defaults to recordID+1.
// To test "right side match" scenarios, the right id can
// be overridden.
func seedFinding(t *testing.T, d *db.DB, recordID int64) int64 {
	return seedFindingPair(t, d, recordID, recordID+1)
}

// seedFindingPair inserts one open duplicate_audit_finding
// row with explicit left/right record ids.
func seedFindingPair(t *testing.T, d *db.DB, leftID, rightID int64) int64 {
	t.Helper()
	pairKey := "pair-" + itoa(int(leftID)) + "-" + itoa(int(rightID))
	res, err := d.Conn().Exec(
		`INSERT INTO duplicate_audit_findings (pair_key, left_record_id, right_record_id, status) VALUES (?, ?, ?, 'open')`,
		pairKey, leftID, rightID,
	)
	if err != nil {
		t.Fatalf("seed finding: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// TestAuditRecordRepo_FindingsForRecordIDs_OnlyMatching
// asserts the OR-clause filter returns only findings where
// the given record id appears as left or right.
func TestAuditRecordRepo_FindingsForRecordIDs_OnlyMatching(t *testing.T) {
	d := newAuditTestDB(t)
	r := NewAuditRecordRepo(d)
	conn := d.Conn()

	// Seed: finding for record 42 (left=42) + finding for
	// record 99 (left=99, right=99+1=100).
	_ = seedFinding(t, d, 42)
	_ = seedFinding(t, d, 99)

	rows, err := r.FindingsForRecordIDs(context.Background(), conn, []int64{42}, "")
	if err != nil {
		t.Fatalf("FindingsForRecordIDs: %v", err)
	}
	if rows == nil {
		t.Fatalf("FindingsForRecordIDs: returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 1 {
		t.Errorf("rows iterated = %d, want 1 (record 42 only)", count)
	}
}

// TestAuditRecordRepo_FindingsForRecordIDs_MatchBothSides
// asserts a record that appears as the right side is also
// returned.
func TestAuditRecordRepo_FindingsForRecordIDs_MatchBothSides(t *testing.T) {
	d := newAuditTestDB(t)
	r := NewAuditRecordRepo(d)
	conn := d.Conn()

	// Seed: finding with left=42, right=99. A query for
	// [99] should match (right side).
	_ = seedFindingPair(t, d, 42, 99)

	rows, err := r.FindingsForRecordIDs(context.Background(), conn, []int64{99}, "")
	if err != nil {
		t.Fatalf("FindingsForRecordIDs: %v", err)
	}
	if rows == nil {
		t.Fatalf("FindingsForRecordIDs: returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 1 {
		t.Errorf("rows iterated = %d, want 1 (record 99 on right side)", count)
	}
}

// TestAuditRecordRepo_FindingsForRecordIDs_StatusFilter
// asserts the statusFilter argument scopes the result set
// to a single status value.
func TestAuditRecordRepo_FindingsForRecordIDs_StatusFilter(t *testing.T) {
	d := newAuditTestDB(t)
	r := NewAuditRecordRepo(d)
	conn := d.Conn()

	// Seed 2 findings: 1 open (default), 1 resolved.
	_ = seedFindingPair(t, d, 42, 43) // open
	_ = seedFindingPair(t, d, 44, 45) // resolved below
	if _, err := conn.Exec(
		`UPDATE duplicate_audit_findings SET status = 'resolved' WHERE left_record_id = 44`,
	); err != nil {
		t.Fatalf("mark resolved: %v", err)
	}

	// Verify the update took effect.
	var nOpen, nResolved int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM duplicate_audit_findings WHERE status = 'open'`).Scan(&nOpen); err != nil {
		t.Fatalf("verify open: %v", err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM duplicate_audit_findings WHERE status = 'resolved'`).Scan(&nResolved); err != nil {
		t.Fatalf("verify resolved: %v", err)
	}
	if nOpen != 1 || nResolved != 1 {
		t.Fatalf("after update: open=%d resolved=%d, want 1/1", nOpen, nResolved)
	}

	// Query for only the open record's id with the "open" filter.
	rows, err := r.FindingsForRecordIDs(context.Background(), conn, []int64{42, 43}, "open")
	if err != nil {
		t.Fatalf("FindingsForRecordIDs(open): %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id int64
		var status string
		// AuditRecordSelectColumns order: id, pair_key,
		// left_record_id, right_record_id, finding_type,
		// reason, highlight_fields, status, created_at,
		// last_detected_at, resolved_at. last_detected_at
		// is nullable in the schema — use NullString.
		if err := rows.Scan(
			&id, new(string), new(int64), new(int64),
			new(string), new(string), new(string),
			&status, new(string), new(sql.NullString), new(sql.NullString),
		); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if status != "open" {
			t.Errorf("open filter row %d has status %q, want %q", id, status, "open")
		}
		count++
	}
	if count != 1 {
		t.Errorf("open filter: rows iterated = %d, want 1", count)
	}

	// Query for the resolved record's id with the "resolved" filter.
	rows2, err := r.FindingsForRecordIDs(context.Background(), conn, []int64{44, 45}, "resolved")
	if err != nil {
		t.Fatalf("FindingsForRecordIDs(resolved): %v", err)
	}
	defer rows2.Close()
	count = 0
	for rows2.Next() {
		var id int64
		var status string
		if err := rows2.Scan(
			&id, new(string), new(int64), new(int64),
			new(string), new(string), new(string),
			&status, new(string), new(sql.NullString), new(sql.NullString),
		); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if status != "resolved" {
			t.Errorf("resolved filter row %d has status %q, want %q", id, status, "resolved")
		}
		count++
	}
	if count != 1 {
		t.Errorf("resolved filter: rows iterated = %d, want 1", count)
	}

	// Cross-check: querying for the open record's id with the
	// "resolved" filter should return 0 rows (the record is
	// open, not resolved).
	rows3, err := r.FindingsForRecordIDs(context.Background(), conn, []int64{42, 43}, "resolved")
	if err != nil {
		t.Fatalf("FindingsForRecordIDs(resolved-on-open): %v", err)
	}
	defer rows3.Close()
	count = 0
	for rows3.Next() {
		count++
	}
	if count != 0 {
		t.Errorf("resolved filter on open record: rows = %d, want 0", count)
	}
}

// TestAuditRecordRepo_ListResolvedFindings_Pagination
// asserts the paginated read returns only resolved findings
// + the correct total count.
func TestAuditRecordRepo_ListResolvedFindings_Pagination(t *testing.T) {
	d := newAuditTestDB(t)
	r := NewAuditRecordRepo(d)
	conn := d.Conn()

	// Seed 5 findings; mark 3 as resolved.
	for i := int64(0); i < 5; i++ {
		seedFinding(t, d, 100+i)
	}
	if _, err := conn.Exec(
		`UPDATE duplicate_audit_findings SET status = 'resolved', resolved_at = CURRENT_TIMESTAMP WHERE id <= 3`,
	); err != nil {
		t.Fatalf("mark resolved: %v", err)
	}

	rows, total, err := r.ListResolvedFindings(context.Background(), conn, 1, 10)
	if err != nil {
		t.Fatalf("ListResolvedFindings: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (resolved only)", total)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 3 {
		t.Errorf("page 1 row count = %d, want 3", count)
	}
}

// TestAuditRecordRepo_ListResolvedFindingsEnriched_JOINedDisplayIDs
// asserts the JOIN-shaped read returns the 8-col legacy
// shape with the LEFT JOINed display_id columns populated.
func TestAuditRecordRepo_ListResolvedFindingsEnriched_JOINedDisplayIDs(t *testing.T) {
	d := newAuditTestDB(t)
	r := NewAuditRecordRepo(d)
	conn := d.Conn()

	// Seed 2 soldiers (for the LEFT JOIN).
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id) VALUES (1, 'DXD-00001'), (2, 'DXD-00002')`,
	); err != nil {
		t.Fatalf("seed soldiers: %v", err)
	}

	// Seed 1 open + 1 resolved finding. Different pair
	// keys so the UNIQUE constraint doesn't collide.
	_ = seedFindingPair(t, d, 1, 2) // open
	resolvedID := seedFindingPair(t, d, 3, 4) // resolved below
	if resolvedID == 0 {
		t.Fatalf("seedFindingPair returned 0")
	}
	// Mark the second as resolved.
	if _, err := conn.Exec(
		`UPDATE duplicate_audit_findings SET status = 'resolved', resolved_at = '2026-07-17 12:00:00' WHERE id = ?`,
		resolvedID,
	); err != nil {
		t.Fatalf("mark resolved: %v", err)
	}

	// Add 2 more soldiers for the resolved finding's IDs.
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id) VALUES (3, 'DXD-00003'), (4, 'DXD-00004')`,
	); err != nil {
		t.Fatalf("seed extra soldiers: %v", err)
	}

	rows, total, err := r.ListResolvedFindingsEnriched(context.Background(), conn, 1, 10)
	if err != nil {
		t.Fatalf("ListResolvedFindingsEnriched: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1 (resolved only)", total)
	}
	defer rows.Close()

	// Iterate + scan the 8-col legacy shape.
	count := 0
	for rows.Next() {
		var (
			id                                                       int64
			leftRecordID, rightRecordID                              int64
			leftDisplayID, rightDisplayID                            string
			findingType, reason, resolvedAt                          string
		)
		if err := rows.Scan(
			&id, &leftRecordID, &rightRecordID,
			&leftDisplayID, &rightDisplayID,
			&findingType, &reason, &resolvedAt,
		); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if id != resolvedID {
			t.Errorf("row id = %d, want %d (the resolved one)", id, resolvedID)
		}
		if leftRecordID != 3 || rightRecordID != 4 {
			t.Errorf("LEFT/RIGHT record ids = (%d, %d), want (3, 4)", leftRecordID, rightRecordID)
		}
		if leftDisplayID != "DXD-00003" {
			t.Errorf("left display_id = %q, want %q", leftDisplayID, "DXD-00003")
		}
		if rightDisplayID != "DXD-00004" {
			t.Errorf("right display_id = %q, want %q", rightDisplayID, "DXD-00004")
		}
		if resolvedAt != "2026-07-17 12:00:00" {
			t.Errorf("resolved_at = %q, want %q", resolvedAt, "2026-07-17 12:00:00")
		}
		count++
	}
	if count != 1 {
		t.Errorf("rows iterated = %d, want 1", count)
	}
}