// quality_scan_repo_parity_test.go — issue #625 slice 9a
// regression net.
//
// Pins the contract that the new repository-backed
// RunDataQualityScan + ApplyDataQualityFindingsToReviewQueue
// paths return identical results to the legacy inline-SQL
// paths on the same fixture.
//
// Strategy: run the suite of existing quality_scan_test.go
// tests (TestRunDataQualityScan_* + TestClassifyMarkupNoise
// + TestIsPersonBearingEntryType) — these tests were written
// against the legacy inline-SQL path and were passing before
// the seam was introduced. The fact that they STILL pass with
// the new seam-delegating implementation is itself the parity
// assertion.
//
// This file adds 2 NEW slice-9a-specific tests that pin
// invariants the existing suite doesn't fully cover:
//
//   - The seam delegation path through RunDataQualityScan
//     emits the same DataQualityIssue.Count + Group/Code
//     breakdown as the legacy path. (Regression net for a
//     future column-list drift in the repo constants.)
//
//   - The seam delegation path through
//     ApplyDataQualityFindingsToReviewQueue produces the same
//     {Selected, Flagged, AlreadyInQueue, NotFound} counter
//     shape as the legacy path. (Regression net for a future
//     edge-case in the ReviewStateForSoldier / SetReviewReason
//     pair — e.g. found=false on a row that should exist, or
//     affected=0 on a row the legacy UPDATE matched.)
package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestQualityScanRepo_Parity_FullScanReturnsExpectedIssueCounts
// seeds a fixture with rows that exercise every code path the
// scan touches (soldier, widow, event, soldier with markup in
// source-record details, soldier with empty source record)
// and asserts the seam-delegated scan emits the expected issue
// counts per (Group, Code) pair.
//
// The expected counts were pinned by running the legacy
// (pre-seam) RunDataQualityScan against this same fixture on
// commit 2d0eb53 and snapshotting the per-(Group,Code) counts.
// The seam must produce identical counts; a future column-list
// drift in QualityScan*Columns constants (e.g. dropping
// restored_at) would surface here as a mismatched count for
// the markup-noise path.
func TestQualityScanRepo_Parity_FullScanReturnsExpectedIssueCounts(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)

	// Soldier with full name + an empty source record (advanced
	// mode gate fires source-record-empty).
	s1, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Carter"})
	if err != nil {
		t.Fatalf("Create s1: %v", err)
	}
	if _, err := d.Conn().Exec(
		`INSERT INTO records (person_record_id, record_type, app_id, details) VALUES (?, '', '', '')`,
		s1.ID,
	); err != nil {
		t.Fatalf("seed empty record: %v", err)
	}

	// Soldier with markup in source-record details (markup-noise
	// gate fires for both modes).
	s2, err := svc.Create(models.Soldier{FirstName: "Sam", LastName: "Walker"})
	if err != nil {
		t.Fatalf("Create s2: %v", err)
	}
	if _, err := d.Conn().Exec(
		`INSERT INTO records (person_record_id, record_type, app_id, details) VALUES (?, 'pension', '12345', '<p>HTML markup</p>')`,
		s2.ID,
	); err != nil {
		t.Fatalf("seed markup record: %v", err)
	}

	// Orphan event record (event-zero-links gate fires in both modes).
	if _, err := events.CreateEvent(models.Soldier{
		EntryType: models.EntryTypeEvent,
		Kind:      "Battle",
		BeginDate: "07/01/1862",
		EndDate:   "07/03/1862",
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	// Run the advanced-mode scan (exercises every read path).
	result, err := svc.RunDataQualityScan("advanced")
	if err != nil {
		t.Fatalf("RunDataQualityScan: %v", err)
	}

	// Tally per (Group, Code) so we can assert the breakdown
	// rather than the raw issue list (the raw list is order-
	// sensitive + ties to specific soldier ids that may
	// shift with seed-data changes; the (Group, Code) tally
	// is the durable invariant).
	counts := map[[2]string]int{}
	for _, issue := range result.Issues {
		counts[[2]string{issue.Group, issue.Code}]++
	}

	// Expected counts (pinned against the legacy pre-seam path
	// on commit 2d0eb53):
	//
	//   source-record-empty: s1 has 1 empty record → 1 issue.
	//     (s2's record is populated with record_type='pension',
	//     so it does NOT contribute.)
	//   mixed-content-script / raw-html-tags: s2 has 1 markup
	//     record. The classifier picks the most-severe code;
	//     raw HTML like `<p>...</p>` fires 'raw-html-tags'
	//     (severity 'medium'). 1 issue.
	//   event-zero-links: 1 orphan event → 1 issue.
	//   spouse-link-missing: s2 has no spouse_soldier_id set;
	//     the relationship-integrity check fires (the legacy
	//     path emits this same issue, so the seam must too).
	want := map[[2]string]int{
		{"Source Records", "source-record-empty"}:     1,
		{"Field Content", "raw-html-tags"}:            1,
		{"Event Integrity", "event-zero-links"}:       1,
		{"Relationship Integrity", "spouse-link-missing"}: 1,
	}
	for key, wantCount := range want {
		if got := counts[key]; got != wantCount {
			t.Errorf("issue count for %v = %d, want %d (seam parity drift)", key, got, wantCount)
		}
	}
	if len(counts) != len(want) {
		t.Errorf("scan emitted %d distinct (Group,Code) pairs, want %d. Full tally: %v", len(counts), len(want), counts)
	}
}

// TestQualityScanRepo_Parity_ApplyFindingsCounterShape asserts
// the ApplyDataQualityFindingsToReviewQueue path returns the
// same counter shape the legacy path returned:
//
//   - Selected = len(deduped ids) — the pre-call count.
//   - Flagged = number of ids that were NOT already in the
//     review queue (needs_review=0) — the new flags.
//   - AlreadyInQueue = number of ids that were already flagged
//     (needs_review=1).
//   - NotFound = number of ids that don't exist in soldiers.
//
// The seam-delegating path uses ReviewStateForSoldier (SELECT)
// + SetReviewReason (UPDATE) instead of the legacy inline SQL.
// A regression in either method (e.g. found=true for a
// non-existent id, affected=0 for a real UPDATE) surfaces here
// as a mismatched counter.
func TestQualityScanRepo_Parity_ApplyFindingsCounterShape(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	// Seed 4 soldiers:
	//   - 2 will be flagged fresh (needs_review=0 → flag).
	//   - 1 will be skipped (needs_review=1 → already in queue).
	//   - 1 will be NotFound (non-existent id).
	fresh1, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Carter"})
	if err != nil {
		t.Fatalf("Create fresh1: %v", err)
	}
	fresh2, err := svc.Create(models.Soldier{FirstName: "Jane", LastName: "Roe"})
	if err != nil {
		t.Fatalf("Create fresh2: %v", err)
	}
	already, err := svc.Create(models.Soldier{FirstName: "Sam", LastName: "Walker"})
	if err != nil {
		t.Fatalf("Create already: %v", err)
	}
	if _, err := d.Conn().Exec(`UPDATE soldiers SET needs_review = 1, review_reason = 'pre-existing tag' WHERE id = ?`, already.ID); err != nil {
		t.Fatalf("set already: %v", err)
	}

	missing := int64(999999)

	result, err := svc.ApplyDataQualityFindingsToReviewQueue([]int64{fresh1.ID, fresh2.ID, already.ID, missing})
	if err != nil {
		t.Fatalf("ApplyDataQualityFindingsToReviewQueue: %v", err)
	}
	if result.Selected != 4 {
		t.Errorf("Selected = %d, want 4", result.Selected)
	}
	if result.Flagged != 2 {
		t.Errorf("Flagged = %d, want 2 (fresh1 + fresh2)", result.Flagged)
	}
	if result.AlreadyInQueue != 1 {
		t.Errorf("AlreadyInQueue = %d, want 1 (already)", result.AlreadyInQueue)
	}
	if result.NotFound != 1 {
		t.Errorf("NotFound = %d, want 1 (missing)", result.NotFound)
	}

	// Verify the post-apply DB state: fresh1 + fresh2 should
	// now have needs_review=1 (the SetReviewStatus path that
	// fires when needsReview=false; this is NOT the seam's
	// SetReviewReason, which only fires when needsReview=true).
	for _, id := range []int64{fresh1.ID, fresh2.ID} { // fresh1.ID + fresh2.ID are int64 fields on *models.Soldier
		var nr int
		if err := d.Conn().QueryRow(`SELECT needs_review FROM soldiers WHERE id = ?`, id).Scan(&nr); err != nil {
			t.Fatalf("read needs_review for %d: %v", id, err)
		}
		if nr != 1 {
			t.Errorf("soldier %d needs_review = %d, want 1 (post-apply)", id, nr)
		}
	}
	// The 'already' row's review_reason gets the merge marker
	// appended (the legacy path does the same: the merge
	// function appends the marker when the existing reason
	// doesn't already contain it; the ApplyResult code then
	// writes the merged string back via SetReviewReason).
	var reason string
	if err := d.Conn().QueryRow(`SELECT review_reason FROM soldiers WHERE id = ?`, already.ID).Scan(&reason); err != nil {
		t.Fatalf("read review_reason for already: %v", err)
	}
	if reason == "pre-existing tag" {
		t.Errorf("already.ID review_reason was not merged: still %q (SetReviewReason should have appended the marker)", reason)
	}
	if reason == "" {
		t.Errorf("already.ID review_reason is empty after Apply; seam SetReviewReason failed to write")
	}
}

// TestQualityScanRepo_Parity_ApplyFindingsMergesNewReason
// asserts the seam's SetReviewReason fires the merge write
// when the existing review_reason differs from the merge
// output (the "needsReview && reason changed" branch). The
// pre-existing review_reason gets the merge marker appended.
//
// This pins the seam's UPDATE path end-to-end (including the
// touchAuditFields call that follows SetReviewReason in the
// service): if SetReviewReason returns affected=0 on a row
// that should match, this test catches it.
func TestQualityScanRepo_Parity_ApplyFindingsMergesNewReason(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	// Seed a soldier already in the queue with a DIFFERENT
	// reason string (forces the merge-write branch).
	soldier, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Carter"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := soldier.ID
	if _, err := d.Conn().Exec(`UPDATE soldiers SET needs_review = 1, review_reason = 'existing-tag' WHERE id = ?`, id); err != nil {
		t.Fatalf("seed already-in-queue: %v", err)
	}

	_, err = svc.ApplyDataQualityFindingsToReviewQueue([]int64{id})
	if err != nil {
		t.Fatalf("ApplyDataQualityFindingsToReviewQueue: %v", err)
	}

	// Read back the merged reason. The merge step concatenates
	// the existing reason with the new marker (see
	// mergeQualityReviewReason in quality_scan.go).
	var gotReason string
	if err := d.Conn().QueryRow(`SELECT review_reason FROM soldiers WHERE id = ?`, id).Scan(&gotReason); err != nil {
		t.Fatalf("read back review_reason: %v", err)
	}
	if gotReason == "existing-tag" {
		t.Errorf("review_reason was not merged: still %q after Apply", gotReason)
	}
	if gotReason == "" {
		t.Errorf("review_reason is empty after Apply; seam SetReviewReason failed to write")
	}
	// Touch-audit-fields also fires; spot-check that
	// last_edited_at is no longer empty.
	var lastEditedAt string
	if err := d.Conn().QueryRow(`SELECT last_edited_at FROM soldiers WHERE id = ?`, id).Scan(&lastEditedAt); err != nil {
		t.Fatalf("read last_edited_at: %v", err)
	}
	if lastEditedAt == "" {
		t.Errorf("last_edited_at is empty after Apply; touchAuditFields did not fire")
	}
}