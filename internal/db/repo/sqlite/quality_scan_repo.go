// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file ships the SQLite impl of QualityScanRepo (issue #625
// slice 9a). The seam extracts the 6 inline-SQL reads from
// internal/records/quality_scan.go (the data-quality scan +
// apply-to-review-queue paths) so the service no longer owns
// raw SQL.
//
// Parity contract: the SQL emitted here is verbatim-copied from
// the legacy inline paths. Same column order, same COALESCE
// wrappers, same ORDER BY / GROUP BY. The
// internal/records/quality_scan_repo_parity_test.go regression
// net pins byte-equivalent output across the seam.
package sqlite

import (
	"context"
	"database/sql"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
)

// QualityScanCandidateColumns is the SELECT column list used by
// QualityScanRepo.CandidatesForScan. Mirrors the legacy
// loadQualityScanCandidates SQL verbatim: 14 columns matching
// the records.QualityScanCandidate scan dest shape (id,
// display_id, entry_type, spouse_soldier_id, first_name,
// middle_name, last_name, birth_date, death_date, birth_info,
// buried_in, description, created_by_import_path, restored_at).
const QualityScanCandidateColumns = `id, display_id, entry_type, COALESCE(spouse_soldier_id, 0), COALESCE(first_name, ''), COALESCE(middle_name, ''), COALESCE(last_name, ''), COALESCE(birth_date, ''), COALESCE(death_date, ''), COALESCE(birth_info, ''), COALESCE(buried_in, ''), COALESCE(description, ''), COALESCE(created_by_import_path, ''), COALESCE(restored_at, '')`

// QualityScanEntryTypeColumns is the SELECT column list used by
// QualityScanRepo.EntryTypesByID. 2 columns: (id, entry_type).
const QualityScanEntryTypeColumns = `id, COALESCE(entry_type, 'soldier')`

// QualityScanAdvancedIssueColumns is the SELECT column list used by
// QualityScanRepo.AdvancedSourceRecordIssues. 8 columns matching
// the records.DataQualityIssue projection in the legacy
// loadAdvancedSourceRecordIssues SQL (the COUNT(r.id) aggregate
// is the "incomplete record" count the issue's Detail string
// surfaces).
const QualityScanAdvancedIssueColumns = `s.id, COALESCE(s.display_id, ''), COALESCE(s.first_name, ''), COALESCE(s.middle_name, ''), COALESCE(s.last_name, ''), COUNT(r.id), COALESCE(s.created_by_import_path, ''), COALESCE(s.restored_at, '')`

// QualityScanMarkupNoiseColumns is the SELECT column list used by
// QualityScanRepo.SourceRecordMarkupNoise. 11 columns matching
// the legacy loadSourceRecordMarkupNoiseIssues scan dest.
const QualityScanMarkupNoiseColumns = `s.id, COALESCE(s.display_id, ''), COALESCE(s.first_name, ''), COALESCE(s.middle_name, ''), COALESCE(s.last_name, ''), COALESCE(s.entry_type, 'soldier'), COALESCE(r.record_type, ''), COALESCE(r.app_id, ''), COALESCE(r.details, ''), COALESCE(s.created_by_import_path, ''), COALESCE(s.restored_at, '')`

// QualityScanEventZeroLinkColumns is the SELECT column list used by
// QualityScanRepo.EventZeroLinkIssues. 7 columns matching the
// legacy loadEventZeroLinkIssues scan dest (no COALESCE on
// display_id/kind/begin_date/end_date because Events always have
// those columns populated when they reach the scan; legacy
// behavior preserved verbatim).
const QualityScanEventZeroLinkColumns = `s.id, s.display_id, s.kind, s.begin_date, s.end_date, COALESCE(s.created_by_import_path, ''), COALESCE(s.restored_at, '')`

// QualityScanRepo is the SQLite-backed implementation of
// repo.QualityScanRepo. Constructed by NewQualityScanRepo;
// held by *SoldierService alongside the existing *db.DB +
// personRepo fields.
type QualityScanRepo struct {
	db *db.DB
}

// Compile-time check: QualityScanRepo satisfies
// repo.QualityScanRepo. Catches signature drift at compile time,
// not at the call site.
var _ repo.QualityScanRepo = (*QualityScanRepo)(nil)

// NewQualityScanRepo constructs a SQLite-backed QualityScanRepo
// that opens queries against the given *db.DB. The *db.DB is
// held by reference; closing the DB invalidates this repo.
func NewQualityScanRepo(d *db.DB) *QualityScanRepo {
	return &QualityScanRepo{db: d}
}

// CandidatesForScan returns every soldiers row projected into
// the columns the data-quality scan needs. The caller scans each
// row into a records.QualityScanCandidate using the column list
// in QualityScanCandidateColumns.
//
// The context parameter is accepted for future-proofing the
// seam (a future driver may honor ctx for cancellation); the
// SQLite impl ignores it today because the underlying
// `modernc.org/sqlite` driver does not honor ctx for Query.
func (r *QualityScanRepo) CandidatesForScan(ctx context.Context, q repo.Querier) (*sql.Rows, error) {
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx, `SELECT `+QualityScanCandidateColumns+` FROM soldiers`)
		if qErr != nil {
			return qErr
		}
		rows = qr
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

// EntryTypesByID returns (id, entry_type) pairs for every
// soldiers row. The caller builds the map[string]string (or
// map[int64]string) from the returned rows + applies the
// normalizeEntryType domain step.
//
// SQL mirrors the legacy loadEntryTypesByID query verbatim.
func (r *QualityScanRepo) EntryTypesByID(ctx context.Context, q repo.Querier) (*sql.Rows, error) {
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx, `SELECT `+QualityScanEntryTypeColumns+` FROM soldiers`)
		if qErr != nil {
			return qErr
		}
		rows = qr
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

// AdvancedSourceRecordIssues returns rows identifying soldiers
// that have at least one source record with empty record_type,
// app_id, AND details. The caller projects each row into a
// records.DataQualityIssue (Group="Source Records", Code=
// "source-record-empty").
//
// SQL mirrors the legacy loadAdvancedSourceRecordIssues query
// verbatim: same JOIN, same WHERE (TRIM wrappers around the
// three empty-string checks), same GROUP BY.
func (r *QualityScanRepo) AdvancedSourceRecordIssues(ctx context.Context, q repo.Querier) (*sql.Rows, error) {
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx, `SELECT `+QualityScanAdvancedIssueColumns+`
		FROM soldiers s
		JOIN records r ON r.person_record_id = s.id
		WHERE TRIM(COALESCE(r.record_type, '')) = ''
		  AND TRIM(COALESCE(r.app_id, '')) = ''
		  AND TRIM(COALESCE(r.details, '')) = ''
		GROUP BY s.id, s.display_id, s.first_name, s.middle_name, s.last_name, s.created_by_import_path, s.restored_at`)
		if qErr != nil {
			return qErr
		}
		rows = qr
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

// SourceRecordMarkupNoise returns rows where a source record's
// details column is non-empty. The caller invokes the markup
// classifier on each row's details value to decide whether to
// emit a DataQualityIssue (the classifier's empty-code return
// short-circuits to "no issue for this row").
//
// SQL mirrors the legacy loadSourceRecordMarkupNoiseIssues query
// verbatim: same JOIN, same WHERE (TRIM(details) != '' so
// whitespace-only details are skipped, the same gate the legacy
// path uses).
func (r *QualityScanRepo) SourceRecordMarkupNoise(ctx context.Context, q repo.Querier) (*sql.Rows, error) {
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx, `SELECT `+QualityScanMarkupNoiseColumns+`
		FROM records r
		JOIN soldiers s ON s.id = r.person_record_id
		WHERE TRIM(COALESCE(r.details, '')) != ''`)
		if qErr != nil {
			return qErr
		}
		rows = qr
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

// EventZeroLinkIssues returns rows identifying Event Records
// (soldiers.entry_type = ?) with zero event_person_links rows.
// Per v60 / issue #320 the Event Record lives in the soldiers
// table; this read is the structural-integrity check for
// orphaned Events.
//
// SQL mirrors the legacy loadEventZeroLinkIssues query verbatim:
// same LEFT JOIN, same WHERE (entry_type = 'event' AND epl.id IS
// NULL), same ORDER BY (updated_at DESC, id DESC).
func (r *QualityScanRepo) EventZeroLinkIssues(ctx context.Context, q repo.Querier) (*sql.Rows, error) {
	var rows *sql.Rows
	// models.EntryTypeEvent is the literal string "event" (see
	// internal/models/models.go). Hardcoding the literal here
	// keeps the repo package free of a models import — the
	// repo package is pure data access; domain literals
	// belong in the service.
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx, `SELECT `+QualityScanEventZeroLinkColumns+`
		 FROM soldiers s
		 LEFT JOIN event_person_links epl ON epl.event_id = s.id
		 WHERE s.entry_type = ? AND epl.id IS NULL
		 ORDER BY s.updated_at DESC, s.id DESC`, "event")
		if qErr != nil {
			return qErr
		}
		rows = qr
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

// ReviewStateForSoldier returns the needs_review flag +
// review_reason string for one soldier by id. The third return
// value is found: false when sql.ErrNoRows maps to "no such
// soldier" (the apply path increments result.NotFound). The
// caller uses the bool to distinguish "not found" from "real
// error", not by string-matching the error.
//
// SQL mirrors the legacy ApplyDataQualityFindingsToReviewQueue
// SELECT verbatim: same 2 columns, same COALESCE on review_reason.
func (r *QualityScanRepo) ReviewStateForSoldier(ctx context.Context, q repo.Querier, id int64) (needsReview bool, reason string, found bool, err error) {
	var nr sql.NullBool
	if err := db.WithBusyRetry(3, func() error {
		row := q.QueryRowContext(ctx, `SELECT needs_review, COALESCE(review_reason, '') FROM soldiers WHERE id = ?`, id)
		scanErr := row.Scan(&nr, &reason)
		if scanErr == sql.ErrNoRows {
			found = false
			return nil
		}
		if scanErr != nil {
			return scanErr
		}
		found = true
		if nr.Valid {
			needsReview = nr.Bool
		}
		return nil
	}); err != nil {
		return false, "", false, err
	}
	return needsReview, reason, found, nil
}

// SetReviewReason writes the merged review_reason back to the
// soldiers row. Returns rows-affected count (0 = no row matched;
// the service handles that the same way it handles the SELECT's
// found=false).
//
// SQL mirrors the legacy ApplyDataQualityFindingsToReviewQueue
// UPDATE verbatim: same SET clause, same WHERE.
func (r *QualityScanRepo) SetReviewReason(ctx context.Context, ex repo.Execer, id int64, reason string) (int64, error) {
	var affected int64
	if err := db.WithBusyRetry(3, func() error {
		res, execErr := ex.ExecContext(ctx, `UPDATE soldiers SET review_reason = ? WHERE id = ?`, reason, id)
		if execErr != nil {
			return execErr
		}
		rowsAffected, raErr := res.RowsAffected()
		if raErr != nil {
			return raErr
		}
		affected = rowsAffected
		return nil
	}); err != nil {
		return 0, err
	}
	return affected, nil
}