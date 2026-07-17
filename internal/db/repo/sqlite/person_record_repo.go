// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// Slice 1 of issue #613 ships the SQLite impl of PersonRecordRepo.
// Future slices add SQLite impls for EventRepo, ArticleRepo,
// etc. (one file per repo interface).
//
// Parity contract: the SQL emitted here must produce identical
// rows to the legacy inline SQL in
// internal/records/soldier_service.go (the slice-1 parity test
// in internal/records/ asserts this). Specifically:
//
//   - List orders by last_name, first_name (the legacy behavior).
//   - List applies LIMIT ? OFFSET ? (the legacy behavior).
//   - List returns the total count separately (the legacy
//     behavior — used for the pagination footer).
//   - Both methods wrap the underlying Query/QueryRow in
//     db.WithBusyRetry(3, ...) (the legacy retry-on-busy
//     pattern used throughout soldier_service.go).
package sqlite

import (
	"context"
	"database/sql"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
)

// PersonRecordSelectColumns is the SELECT column list used by
// PersonRecordRepo.GetByID. Mirrors the legacy
// `soldierSelectColumns` constant in
// internal/records/soldier_service.go verbatim — the slice-1
// parity test asserts both produce identical rows.
//
// Slice 1 ships a leaner subset than the legacy list (drops the
// cross-table count subqueries used by `soldierListSelectColumns`
// since those are out of scope for slice 1). Future slices
// extend this list as GetByID starts pulling cross-table data.
const PersonRecordSelectColumns = `id, display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, show_prefix_before_name, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, biography, pdf_excerpt_override, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at, kind, begin_date, end_date, description, created_by_version, created_by_import_path, restored_at`

// PersonRecordListSelectColumns is the SELECT column list used by
// PersonRecordRepo.List. Mirrors the legacy
// `soldierListSelectColumns` shape in
// internal/records/soldier_service.go verbatim: it extends
// PersonRecordSelectColumns with three inline subqueries the
// browse page renders per row (linked spouse display_id, record
// count, image count). The slice-1 parity test asserts the
// service's scan helper still scans the full 51-dest shape.
const PersonRecordListSelectColumns = PersonRecordSelectColumns + `, COALESCE((SELECT display_id FROM soldiers linked WHERE linked.id = soldiers.spouse_soldier_id), ''), (SELECT COUNT(*) FROM records WHERE records.person_record_id = soldiers.id), (SELECT COUNT(*) FROM images WHERE images.person_record_id = soldiers.id)`

// PersonRecordRepo is the SQLite-backed implementation of
// repo.PersonRecordRepo. Constructed by NewPersonRecordRepo;
// held by *SoldierService alongside the legacy *db.DB field for
// the duration of the slice-by-slice migration.
type PersonRecordRepo struct {
	db *db.DB
}

// Compile-time check: PersonRecordRepo satisfies
// repo.PersonRecordRepo. Catches signature drift at compile
// time, not at the call site.
var _ repo.PersonRecordRepo = (*PersonRecordRepo)(nil)

// NewPersonRecordRepo constructs a SQLite-backed
// PersonRecordRepo that opens queries against the given *db.DB.
// The *db.DB is held by reference; closing the DB invalidates
// this repo.
func NewPersonRecordRepo(d *db.DB) *PersonRecordRepo {
	return &PersonRecordRepo{db: d}
}

// GetByID returns the Person Record row with the given
// primary-key ID. Returns sql.ErrNoRows if no row matches.
// The returned *sql.Row is scanned with the column list in
// PersonRecordSelectColumns.
//
// The context parameter is accepted for future-proofing the
// seam (a future driver may honor ctx for cancellation); the
// SQLite impl ignores it today because the underlying
// `modernc.org/sqlite` driver does not honor ctx for
// QueryRow.
func (r *PersonRecordRepo) GetByID(ctx context.Context, id int64) (*sql.Row, error) {
	conn := r.db.Conn()
	var row *sql.Row
	if err := db.WithBusyRetry(3, func() error {
		row = conn.QueryRowContext(ctx, `SELECT `+PersonRecordSelectColumns+` FROM soldiers WHERE id = ?`, id)
		return nil
	}); err != nil {
		return nil, err
	}
	return row, nil
}

// List returns paginated Person Records ordered by
// last_name, first_name, plus the total row count. The caller
// must Close the returned *sql.Rows.
//
// page is 1-indexed; pageSize is the cap. Slice 1 does not
// clamp page/pageSize to safe values (the legacy code also
// does not — that responsibility lives in the viewmodel layer
// which validates user input before calling Service.List).
func (r *PersonRecordRepo) List(ctx context.Context, page, pageSize int) (*sql.Rows, int, error) {
	conn := r.db.Conn()
	var total int
	if err := db.WithBusyRetry(3, func() error {
		return conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM soldiers`).Scan(&total)
	}); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		q, qErr := conn.QueryContext(ctx,
			`SELECT `+PersonRecordListSelectColumns+` FROM soldiers ORDER BY last_name, first_name LIMIT ? OFFSET ?`,
			pageSize, offset,
		)
		if qErr != nil {
			return qErr
		}
		rows = q
		return nil
	}); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}