// Package repo declares the repository interfaces that decouple
// DixieData's domain services from the concrete database driver.
//
// Slice 9a of issue #625 adds the QualityScanRepo seam for the
// data-quality scan + apply-to-review-queue paths in
// internal/records/quality_scan.go.
//
// The data-quality scan is read-only (it produces []DataQualityIssue
// in memory). The apply-to-review-queue path performs a per-id
// SELECT + a conditional UPDATE. Both shapes cross the seam.
//
// Note: the issue body mentions RecordQualityIssue + ClearQualityIssues
// as future seam methods, but the current code has no quality_issues
// table — the scan is fully in-memory. Those methods are intentionally
// NOT exposed today; a future persistence layer (separate issue) can
// extend the interface.
package repo

import (
	"context"
	"database/sql"
)

// QualityScanRepo is the data-access seam for the data-quality
// scan + apply-to-review-queue paths in
// internal/records/quality_scan.go.
//
// Design contract (mirrors the rest of the repo package):
//
//   - Methods return *sql.Rows / *sql.Row (not domain structs) so
//     the scan-helper + domain-normalization step stays inside
//     the consuming service. This keeps domain packages
//     (pensionstate, confederatehomestatus, dates) out of the
//     repo package — the repo is pure data access.
//
//   - Column lists are exported from the impl package (in the
//     internal/db/repo/sqlite subpackage) as constants so the
//     scan helpers in the consuming service stay in sync with
//     what the repo's SELECT emits.
//
//   - Context-aware: every method takes context.Context. The
//     SQLite impl ignores it today (the underlying driver does
//     not honor ctx), but the signature future-proofs the seam
//     for drivers that do.
//
//   - The read methods take a Querier (not a *sql.DB) so the
//     caller can pass a *sql.Tx when the scan runs inside a
//     surrounding transaction. The apply-path write takes an
//     Execer for the same reason.
type QualityScanRepo interface {
	// CandidatesForScan returns every row in the soldiers
	// table projected into the columns the data-quality scan
	// needs. The caller scans each row into a
	// records.QualityScanCandidate (the slice-9a contract
	// pins the column order in the impl package's
	// QualityScanCandidateColumns constant).
	CandidatesForScan(ctx context.Context, q Querier) (*sql.Rows, error)

	// EntryTypesByID returns (id, entry_type) pairs for every
	// row in the soldiers table. The caller builds the map
	// from the returned rows (the service owns the
	// normalizeEntryType step).
	EntryTypesByID(ctx context.Context, q Querier) (*sql.Rows, error)

	// AdvancedSourceRecordIssues returns rows identifying
	// soldiers that have at least one source record with
	// empty record_type, app_id, AND details. The caller
	// projects each row into a records.DataQualityIssue.
	// Run only in DataQualityModeAdvanced (the
	// loadAdvancedSourceRecordIssues gate).
	AdvancedSourceRecordIssues(ctx context.Context, q Querier) (*sql.Rows, error)

	// SourceRecordMarkupNoise returns rows where a source
	// record's details column is non-empty (the candidate
	// set for the HTML / markup classifier). The caller
	// invokes classifyMarkupNoise on each row's details
	// value to decide whether to emit a DataQualityIssue.
	// Runs in BOTH scan modes (no mode gate; the issue
	// criterion is orthogonal to the empty-record check).
	SourceRecordMarkupNoise(ctx context.Context, q Querier) (*sql.Rows, error)

	// EventZeroLinkIssues returns rows identifying Event
	// Records (soldiers.entry_type = 'event') with zero
	// event_person_links rows. Per v60 / issue #320 the
	// Event Record lives in the soldiers table; this read
	// is the structural-integrity check for orphaned Events.
	// Runs in BOTH scan modes (the zero-link condition is
	// a structural issue, not a content-quality issue).
	EventZeroLinkIssues(ctx context.Context, q Querier) (*sql.Rows, error)

	// ReviewStateForSoldier returns the needs_review flag +
	// review_reason string for one soldier by id. The third
	// return value is found: false when sql.ErrNoRows maps
	// to "no such soldier" (the apply path increments
	// result.NotFound). Returning the error lets the caller
	// distinguish "not found" from "real error" via the
	// found bool, not by string-matching err.
	ReviewStateForSoldier(ctx context.Context, q Querier, id int64) (needsReview bool, reason string, found bool, err error)

	// SetReviewReason writes the merged review_reason back
	// to the soldiers row. The merge logic (combining
	// existing + new tag strings) stays in the service; the
	// repo only owns the SQL UPDATE. Returns rows-affected
	// count (0 = no row matched; the service handles that
	// the same way it handles the SELECT's found=false).
	SetReviewReason(ctx context.Context, ex Execer, id int64, reason string) (int64, error)
}