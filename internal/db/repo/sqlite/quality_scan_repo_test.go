// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file pins the contract for QualityScanRepo (issue #625
// slice 9a). The seam extracts the 6 inline-SQL reads from
// internal/records/quality_scan.go. The test schema mirrors the
// columns the legacy SQL touches (soldiers + records +
// event_person_links); the tests assert row counts + column
// shape against seeded fixtures.
//
// Out of scope here: the end-to-end service parity tests live
// in internal/records/quality_scan_repo_parity_test.go (they
// call RunDataQualityScan + ApplyDataQualityFindingsToReviewQueue
// and compare against expected behavior; this file pins the
// repo contract in isolation).
package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// newQualityScanTestDB opens an in-memory SQLite with the
// slice-9a schema (soldiers + records + event_person_links).
// Column shapes mirror the legacy SQL the seam extracts:
//   - soldiers: every column QualityScanCandidateColumns /
//     QualityScanAdvancedIssueColumns / QualityScanMarkupNoiseColumns /
//     QualityScanEventZeroLinkColumns / ReviewStateForSoldier touch.
//   - records: (id, person_record_id, record_type, app_id,
//     details) — the columns the advanced + markup-noise reads join on.
//   - event_person_links: (id, event_id, person_id) — the column
//     the EventZeroLinkIssues LEFT JOIN checks.
func newQualityScanTestDB(t *testing.T) *db.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sql.Open in-memory: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	const schema = `
		CREATE TABLE soldiers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			display_id TEXT NOT NULL DEFAULT '',
			sync_id TEXT NOT NULL DEFAULT '',
			entry_type TEXT NOT NULL DEFAULT 'soldier',
			spouse_soldier_id INTEGER,
			first_name TEXT NOT NULL DEFAULT '',
			middle_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			birth_date TEXT NOT NULL DEFAULT '',
			death_date TEXT NOT NULL DEFAULT '',
			birth_info TEXT NOT NULL DEFAULT '',
			buried_in TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			needs_review INTEGER NOT NULL DEFAULT 0,
			review_reason TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			begin_date TEXT NOT NULL DEFAULT '',
			end_date TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			created_by_import_path TEXT NOT NULL DEFAULT '',
			restored_at TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			person_record_id INTEGER NOT NULL DEFAULT 0,
			record_type TEXT NOT NULL DEFAULT '',
			app_id TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE event_person_links (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id INTEGER NOT NULL DEFAULT 0,
			person_id INTEGER NOT NULL DEFAULT 0
		);
	`
	if _, err := conn.Exec(schema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db.NewFromExisting(conn)
}

// seedQualityScanSoldier inserts one soldiers row with the
// given display_id + first/last name. Returns the generated id.
// Used by CandidatesForScan / EntryTypesByID /
// AdvancedSourceRecordIssues tests.
func seedQualityScanSoldier(t *testing.T, d *db.DB, displayID, first, last string) int64 {
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

// seedQualityScanEvent inserts one soldiers row with
// entry_type='event' + the given kind/begin/end dates. Returns
// the generated id. Used by EventZeroLinkIssues tests.
func seedQualityScanEvent(t *testing.T, d *db.DB, displayID, kind, beginDate, endDate string) int64 {
	t.Helper()
	res, err := d.Conn().Exec(
		`INSERT INTO soldiers (display_id, entry_type, kind, begin_date, end_date, updated_at) VALUES (?, 'event', ?, ?, ?, CURRENT_TIMESTAMP)`,
		displayID, kind, beginDate, endDate,
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

// seedQualityScanRecord inserts one records row with the given
// person_record_id + record_type + app_id + details. Used by
// AdvancedSourceRecordIssues + SourceRecordMarkupNoise tests.
func seedQualityScanRecord(t *testing.T, d *db.DB, personID int64, recordType, appID, details string) int64 {
	t.Helper()
	res, err := d.Conn().Exec(
		`INSERT INTO records (person_record_id, record_type, app_id, details) VALUES (?, ?, ?, ?)`,
		personID, recordType, appID, details,
	)
	if err != nil {
		t.Fatalf("seed record: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// seedQualityScanEventLink inserts one event_person_links row.
// Used by EventZeroLinkIssues tests to verify the LEFT JOIN
// filter (an event WITH a link must NOT appear in the
// zero-link result set).
func seedQualityScanEventLink(t *testing.T, d *db.DB, eventID, personID int64) {
	t.Helper()
	if _, err := d.Conn().Exec(
		`INSERT INTO event_person_links (event_id, person_id) VALUES (?, ?)`,
		eventID, personID,
	); err != nil {
		t.Fatalf("seed event link: %v", err)
	}
}

// TestQualityScanRepo_CandidatesForScan_AllRows asserts the
// repo returns every soldiers row in the table (no implicit
// filter). The scan driver in the service builds a candidate
// slice by iterating rows.Next(); the slice length must match
// the seed count.
func TestQualityScanRepo_CandidatesForScan_AllRows(t *testing.T) {
	d := newQualityScanTestDB(t)
	_ = seedQualityScanSoldier(t, d, "P-0001", "John", "Doe")
	_ = seedQualityScanSoldier(t, d, "P-0002", "Jane", "Roe")
	_ = seedQualityScanSoldier(t, d, "P-0003", "Sam", "Carter")

	r := NewQualityScanRepo(d)
	rows, err := r.CandidatesForScan(context.Background(), d.Conn())
	if err != nil {
		t.Fatalf("CandidatesForScan: %v", err)
	}
	if rows == nil {
		t.Fatalf("CandidatesForScan: returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 3 {
		t.Errorf("CandidatesForScan returned %d rows, want 3", count)
	}
}

// TestQualityScanRepo_CandidatesForScan_ScansAllColumns
// asserts the column list (14 columns matching the
// records.QualityScanCandidate shape) scans cleanly into 14
// destinations. A column-list drift in the repo constant would
// surface here as "sql: expected N destination arguments, got M".
func TestQualityScanRepo_CandidatesForScan_ScansAllColumns(t *testing.T) {
	d := newQualityScanTestDB(t)
	id := seedQualityScanSoldier(t, d, "P-0001", "John", "Doe")

	r := NewQualityScanRepo(d)
	rows, err := r.CandidatesForScan(context.Background(), d.Conn())
	if err != nil {
		t.Fatalf("CandidatesForScan: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("CandidatesForScan: no rows")
	}
	var (
		gotID             int64
		gotDisplayID      string
		gotEntryType      string
		gotSpouseID       int64
		gotFirst          string
		gotMiddle         string
		gotLast           string
		gotBirth          string
		gotDeath          string
		gotBirthInfo      string
		gotBuriedIn       string
		gotDescription    string
		gotImportPath     string
		gotRestoredAt     string
	)
	if err := rows.Scan(
		&gotID, &gotDisplayID, &gotEntryType, &gotSpouseID,
		&gotFirst, &gotMiddle, &gotLast,
		&gotBirth, &gotDeath, &gotBirthInfo, &gotBuriedIn,
		&gotDescription,
		&gotImportPath, &gotRestoredAt,
	); err != nil {
		t.Fatalf("Scan 14 columns: %v", err)
	}
	if gotID != id {
		t.Errorf("id = %d, want %d", gotID, id)
	}
	if gotDisplayID != "P-0001" {
		t.Errorf("display_id = %q, want P-0001", gotDisplayID)
	}
	if gotEntryType != "soldier" {
		t.Errorf("entry_type = %q, want soldier", gotEntryType)
	}
	if gotFirst != "John" || gotLast != "Doe" {
		t.Errorf("name = %q %q, want John Doe", gotFirst, gotLast)
	}
}

// TestQualityScanRepo_EntryTypesByID asserts the entry-type
// projection returns (id, entry_type) for every row. The
// service builds a map[int64]string from these rows and runs
// normalizeEntryType on each value (the normalize step stays
// in the service — the repo only returns the raw column).
func TestQualityScanRepo_EntryTypesByID(t *testing.T) {
	d := newQualityScanTestDB(t)
	id1 := seedQualityScanSoldier(t, d, "P-0001", "John", "Doe")
	id2 := seedQualityScanEvent(t, d, "EVT-0001", "Battle", "07/01/1862", "07/03/1862")

	r := NewQualityScanRepo(d)
	rows, err := r.EntryTypesByID(context.Background(), d.Conn())
	if err != nil {
		t.Fatalf("EntryTypesByID: %v", err)
	}
	defer rows.Close()

	types := map[int64]string{}
	for rows.Next() {
		var id int64
		var et string
		if err := rows.Scan(&id, &et); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		types[id] = et
	}
	if types[id1] != "soldier" {
		t.Errorf("entry_type for %d = %q, want soldier", id1, types[id1])
	}
	if types[id2] != "event" {
		t.Errorf("entry_type for %d = %q, want event", id2, types[id2])
	}
	if len(types) != 2 {
		t.Errorf("EntryTypesByID returned %d rows, want 2", len(types))
	}
}

// TestQualityScanRepo_AdvancedSourceRecordIssues asserts the
// advanced read returns one row per soldier that has at least
// one record with empty record_type + app_id + details (the
// COUNT(r.id) in the legacy SQL aggregates the empty record
// count per soldier).
func TestQualityScanRepo_AdvancedSourceRecordIssues(t *testing.T) {
	d := newQualityScanTestDB(t)
	id1 := seedQualityScanSoldier(t, d, "P-0001", "John", "Doe")
	id2 := seedQualityScanSoldier(t, d, "P-0002", "Jane", "Roe")
	// id1 gets 2 empty records; id2 gets 1 empty + 1 populated.
	_ = seedQualityScanRecord(t, d, id1, "", "", "")
	_ = seedQualityScanRecord(t, d, id1, "", "", "")
	_ = seedQualityScanRecord(t, d, id2, "", "", "")
	_ = seedQualityScanRecord(t, d, id2, "pension", "12345", "has content")

	r := NewQualityScanRepo(d)
	rows, err := r.AdvancedSourceRecordIssues(context.Background(), d.Conn())
	if err != nil {
		t.Fatalf("AdvancedSourceRecordIssues: %v", err)
	}
	defer rows.Close()

	counts := map[int64]int{}
	for rows.Next() {
		var (
			soldierID int64
			display   string
			first     string
			middle    string
			last      string
			count     int
			importP   string
			restored  string
		)
		if err := rows.Scan(&soldierID, &display, &first, &middle, &last, &count, &importP, &restored); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		counts[soldierID] = count
	}
	if counts[id1] != 2 {
		t.Errorf("soldier %d empty-record count = %d, want 2", id1, counts[id1])
	}
	if counts[id2] != 1 {
		t.Errorf("soldier %d empty-record count = %d, want 1 (only the empty record counts)", id2, counts[id2])
	}
}

// TestQualityScanRepo_SourceRecordMarkupNoise asserts the
// markup-noise read returns every record with a non-empty
// details column (the service's classifier decides whether
// to fire a DataQualityIssue). Whitespace-only details are
// excluded by the WHERE TRIM(...) != '' gate.
func TestQualityScanRepo_SourceRecordMarkupNoise(t *testing.T) {
	d := newQualityScanTestDB(t)
	id := seedQualityScanSoldier(t, d, "P-0001", "John", "Doe")
	_ = seedQualityScanRecord(t, d, id, "pension", "12345", "<p>HTML markup</p>")
	_ = seedQualityScanRecord(t, d, id, "pension", "12346", "plain text")
	_ = seedQualityScanRecord(t, d, id, "pension", "12347", "   ") // whitespace-only — excluded

	r := NewQualityScanRepo(d)
	rows, err := r.SourceRecordMarkupNoise(context.Background(), d.Conn())
	if err != nil {
		t.Fatalf("SourceRecordMarkupNoise: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 2 {
		t.Errorf("SourceRecordMarkupNoise returned %d rows, want 2 (whitespace-only excluded)", count)
	}
}

// TestQualityScanRepo_EventZeroLinkIssues asserts the
// zero-link read returns only Events (entry_type='event') that
// have ZERO event_person_links rows. An event WITH a link
// must be excluded.
func TestQualityScanRepo_EventZeroLinkIssues(t *testing.T) {
	d := newQualityScanTestDB(t)
	linkedEvent := seedQualityScanEvent(t, d, "EVT-0001", "Battle", "07/01/1862", "07/03/1862")
	orphanEvent1 := seedQualityScanEvent(t, d, "EVT-0002", "Campaign", "08/01/1862", "")
	orphanEvent2 := seedQualityScanEvent(t, d, "EVT-0003", "Skirmish", "", "")
	// Attach the first event to a person — must NOT appear in
	// the zero-link result.
	personID := seedQualityScanSoldier(t, d, "P-0001", "John", "Doe")
	seedQualityScanEventLink(t, d, linkedEvent, personID)

	r := NewQualityScanRepo(d)
	rows, err := r.EventZeroLinkIssues(context.Background(), d.Conn())
	if err != nil {
		t.Fatalf("EventZeroLinkIssues: %v", err)
	}
	defer rows.Close()

	gotIDs := map[int64]bool{}
	for rows.Next() {
		var (
			eventID  int64
			display  string
			kind     string
			begin    string
			end      string
			importP  string
			restored string
		)
		if err := rows.Scan(&eventID, &display, &kind, &begin, &end, &importP, &restored); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		gotIDs[eventID] = true
	}
	if gotIDs[linkedEvent] {
		t.Errorf("event %d has a link; must NOT appear in zero-link result", linkedEvent)
	}
	if !gotIDs[orphanEvent1] {
		t.Errorf("orphan event %d missing from zero-link result", orphanEvent1)
	}
	if !gotIDs[orphanEvent2] {
		t.Errorf("orphan event %d missing from zero-link result", orphanEvent2)
	}
	if len(gotIDs) != 2 {
		t.Errorf("EventZeroLinkIssues returned %d events, want 2", len(gotIDs))
	}
}

// TestQualityScanRepo_ReviewStateForSoldier_Found asserts the
// SELECT returns needs_review + reason for an existing row.
func TestQualityScanRepo_ReviewStateForSoldier_Found(t *testing.T) {
	d := newQualityScanTestDB(t)
	id := seedQualityScanSoldier(t, d, "P-0001", "John", "Doe")
	if _, err := d.Conn().Exec(`UPDATE soldiers SET needs_review = 1, review_reason = 'data-quality' WHERE id = ?`, id); err != nil {
		t.Fatalf("set review state: %v", err)
	}

	r := NewQualityScanRepo(d)
	needsReview, reason, found, err := r.ReviewStateForSoldier(context.Background(), d.Conn(), id)
	if err != nil {
		t.Fatalf("ReviewStateForSoldier: %v", err)
	}
	if !found {
		t.Errorf("ReviewStateForSoldier: found = false, want true")
	}
	if !needsReview {
		t.Errorf("needs_review = false, want true")
	}
	if reason != "data-quality" {
		t.Errorf("review_reason = %q, want data-quality", reason)
	}
}

// TestQualityScanRepo_ReviewStateForSoldier_NotFound asserts
// the SELECT returns found=false for a non-existent id, with
// err == nil (the caller distinguishes via the bool, not by
// string-matching the error).
func TestQualityScanRepo_ReviewStateForSoldier_NotFound(t *testing.T) {
	d := newQualityScanTestDB(t)
	r := NewQualityScanRepo(d)
	_, _, found, err := r.ReviewStateForSoldier(context.Background(), d.Conn(), 9999)
	if err != nil {
		t.Fatalf("ReviewStateForSoldier: unexpected err = %v", err)
	}
	if found {
		t.Errorf("ReviewStateForSoldier: found = true for non-existent id, want false")
	}
}

// TestQualityScanRepo_SetReviewReason asserts the UPDATE
// writes review_reason + returns the rows-affected count.
func TestQualityScanRepo_SetReviewReason(t *testing.T) {
	d := newQualityScanTestDB(t)
	id := seedQualityScanSoldier(t, d, "P-0001", "John", "Doe")

	r := NewQualityScanRepo(d)
	affected, err := r.SetReviewReason(context.Background(), d.Conn(), id, "merged-tag")
	if err != nil {
		t.Fatalf("SetReviewReason: %v", err)
	}
	if affected != 1 {
		t.Errorf("SetReviewReason affected = %d, want 1", affected)
	}

	var gotReason string
	if err := d.Conn().QueryRow(`SELECT review_reason FROM soldiers WHERE id = ?`, id).Scan(&gotReason); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gotReason != "merged-tag" {
		t.Errorf("review_reason after SetReviewReason = %q, want merged-tag", gotReason)
	}
}

// TestQualityScanRepo_SetReviewReason_NoMatch asserts the
// UPDATE returns 0 for a non-existent id (not an error). The
// service treats 0 the same as the SELECT's found=false.
func TestQualityScanRepo_SetReviewReason_NoMatch(t *testing.T) {
	d := newQualityScanTestDB(t)
	r := NewQualityScanRepo(d)
	affected, err := r.SetReviewReason(context.Background(), d.Conn(), 9999, "no-such-row")
	if err != nil {
		t.Fatalf("SetReviewReason: unexpected err = %v", err)
	}
	if affected != 0 {
		t.Errorf("SetReviewReason affected = %d, want 0", affected)
	}
}

// TestQualityScanRepo_ContextPropagated is intentionally
// omitted. Each per-method test above already passes
// context.Background() and pins the ctx parameter on the
// method signature; a sweeping "all methods accept ctx"
// test adds coverage without adding signal. See the slice-7
// audit_record_repo_test.go for the same pattern (no
// aggregate ctx test).