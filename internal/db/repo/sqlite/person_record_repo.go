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
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
	"github.com/valueforvalue/DixieData/internal/models"
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

// PersonRecordInsertColumns is the column list used by
// PersonRecordRepo.Create. Mirrors the legacy `INSERT INTO
// soldiers` statement in
// internal/records/soldier_service.go::Create verbatim (45
// columns; includes `is_generated`, `created_at`,
// `created_by_version`, `created_by_import_path`).
const PersonRecordInsertColumns = `display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, show_prefix_before_name, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, biography, pdf_excerpt_override, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at, kind, begin_date, end_date, description, created_by_version, created_by_import_path`

// PersonRecordUpdateColumns is the column list used by
// PersonRecordRepo.Update. Mirrors the legacy `UPDATE soldiers
// SET` statement in internal/records/soldier_service.go::Update
// verbatim (42 columns; excludes `is_generated`, `created_at`,
// `created_by_version`, `created_by_import_path`; sets
// `updated_at`).
const PersonRecordUpdateColumns = `display_id=?, sync_id=?, entry_type=?, spouse_soldier_id=?, relationship_label=?, maiden_name=?, pension_id=?, application_id=?, prefix=?, show_prefix_before_name=?, first_name=?, middle_name=?, last_name=?, suffix=?, rank=?, rank_in=?, rank_out=?, unit=?, pension_state=?, confederate_home_status=?, confederate_home_name=?, death_year=?, death_month=?, death_day=?, birth_date=?, death_date=?, birth_info=?, buried_in=?, biography=?, pdf_excerpt_override=?, notes=?, needs_review=?, review_reason=?, added_by=?, last_edited_by=?, last_edited_fields=?, last_edited_at=?, updated_at=?, kind=?, begin_date=?, end_date=?, description=?`

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

// Create inserts a new Person Record row and returns the
// generated primary-key id via LastInsertId. The caller
// supplies a *sql.Tx so the insert can be composed with
// other writes (replaceRecords, audit log) in a single
// transaction; the repo does not manage the transaction
// boundary.
//
// Slice-2 parity contract: the column list + VALUES order
// match the legacy `INSERT INTO soldiers` statement in
// internal/records/soldier_service.go::Create verbatim. The
// legacy pre-insert normalization (Display ID, sync_id,
// audit timestamps, entry_type canonicalization) stays in
// the service layer.
func (r *PersonRecordRepo) Create(ctx context.Context, ex repo.Execer, s models.Soldier) (int64, error) {
	placeholders := strings.Repeat("?,", len(strings.Split(PersonRecordInsertColumns, ",")))
	placeholders = strings.TrimRight(placeholders, ",")

	query := `INSERT INTO soldiers (` + PersonRecordInsertColumns + `) VALUES (` + placeholders + `)`

	args := soldierInsertArgs(s)
	res, err := ex.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update modifies the Person Record row identified by s.ID
// and returns the rows-affected count. A return of 0 with no
// error means no row matched; the service layer is responsible
// for translating that to ErrSoldierNotFound.
//
// Slice-2 parity contract: the column list + arg order match
// the legacy `UPDATE soldiers SET` statement in
// internal/records/soldier_service.go::Update verbatim.
func (r *PersonRecordRepo) Update(ctx context.Context, ex repo.Execer, s models.Soldier) (int64, error) {
	query := `UPDATE soldiers SET ` + PersonRecordUpdateColumns + ` WHERE id=?`

	args := soldierUpdateArgs(s)
	args = append(args, s.ID)
	res, err := ex.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Delete removes the Person Record row identified by id and
// returns the rows-affected count. A return of 0 with no
// error means no row matched. Foreign-key cascades
// (records, images, tag joins) are handled by the SQLite
// schema's ON DELETE CASCADE rules.
func (r *PersonRecordRepo) Delete(ctx context.Context, ex repo.Execer, id int64) (int64, error) {
	res, err := ex.ExecContext(ctx, `DELETE FROM soldiers WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// soldierInsertArgs builds the args slice for the INSERT
// statement. Column order matches PersonRecordInsertColumns.
//
// SpouseSoldierID is rendered as NULL when 0 (the legacy
// nullableInt64 helper in soldier_service.go does the same).
func soldierInsertArgs(s models.Soldier) []interface{} {
	return []interface{}{
		s.DisplayID, s.SyncID, s.EntryType, nullableInt64(s.SpouseSoldierID),
		s.RelationshipLabel, s.MaidenName, s.IsGenerated, s.PensionID, s.ApplicationID,
		s.Prefix, s.ShowPrefixBeforeName, s.FirstName, s.MiddleName, s.LastName, s.Suffix,
		s.Rank, s.RankIn, s.RankOut, s.Unit, s.PensionState,
		s.ConfederateHomeStatus, s.ConfederateHomeName, s.DeathYear, s.DeathMonth,
		s.DeathDay, s.BirthDate, s.DeathDate, s.BirthInfo, s.BuriedIn,
		s.Biography, s.PDFExcerptOverride, s.Notes, s.NeedsReview, s.ReviewReason,
		s.AddedBy, s.LastEditedBy, s.LastEditedFields, s.LastEditedAt, s.CreatedAt,
		s.UpdatedAt, s.Kind, s.BeginDate, s.EndDate, s.Description,
		s.CreatedByVersion, s.CreatedByImportPath,
	}
}

// soldierUpdateArgs builds the args slice for the UPDATE
// statement. Column order matches PersonRecordUpdateColumns.
func soldierUpdateArgs(s models.Soldier) []interface{} {
	return []interface{}{
		s.DisplayID, s.SyncID, s.EntryType, nullableInt64(s.SpouseSoldierID),
		s.RelationshipLabel, s.MaidenName, s.PensionID, s.ApplicationID,
		s.Prefix, s.ShowPrefixBeforeName, s.FirstName, s.MiddleName, s.LastName, s.Suffix,
		s.Rank, s.RankIn, s.RankOut, s.Unit, s.PensionState,
		s.ConfederateHomeStatus, s.ConfederateHomeName, s.DeathYear, s.DeathMonth,
		s.DeathDay, s.BirthDate, s.DeathDate, s.BirthInfo, s.BuriedIn,
		s.Biography, s.PDFExcerptOverride, s.Notes, s.NeedsReview, s.ReviewReason,
		s.AddedBy, s.LastEditedBy, s.LastEditedFields, s.LastEditedAt, s.UpdatedAt,
		s.Kind, s.BeginDate, s.EndDate, s.Description,
	}
}

// nullableInt64 returns nil when the value is < 1 (the legacy
// convention — 0 means "no spouse", NULL is the storage
// representation). Mirrors the helper in
// internal/records/soldier_service.go.
func nullableInt64(value int64) interface{} {
	if value < 1 {
		return nil
	}
	return value
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