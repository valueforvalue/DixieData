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

	rows, err := r.FindingsForRecordIDs(context.Background(), conn, []int64{42})
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

	rows, err := r.FindingsForRecordIDs(context.Background(), conn, []int64{99})
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