// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file ships the SQLite impl of ArticleRecordRepo (issue
// #613 slice 4). Parity contract: every method's SQL matches
// the legacy inline SQL in
// internal/records/article_service.go verbatim.
package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
	"github.com/valueforvalue/DixieData/internal/models"
)

// ArticleRecordSelectColumns is the SELECT column list used by
// ArticleRecordRepo.GetByID + List. Mirrors the legacy inline
// SQL column list in internal/records/article_service.go (11
// columns: id, sync_id, display_id, title, subtitle, body_md,
// body_html, created_at, updated_at, snapshot_of_id,
// is_snapshot).
const ArticleRecordSelectColumns = `id, sync_id, display_id, title, subtitle, body_md, body_html, created_at, updated_at, snapshot_of_id, is_snapshot`

// ArticleRecordInsertColumns is the column list used by
// ArticleRecordRepo.Create. Mirrors the legacy INSERT in
// article_service.go::Create (10 columns; id is auto-generated
// so excluded).
const ArticleRecordInsertColumns = `sync_id, display_id, title, subtitle, body_md, body_html, created_at, updated_at, snapshot_of_id, is_snapshot`

// ArticleRecordUpdateColumns is the SET-clause column list
// used by ArticleRecordRepo.Update. Mirrors the legacy UPDATE
// in article_service.go::Update verbatim (5 columns: title,
// subtitle, body_md, body_html, updated_at). The slice-4
// parity test asserts the service's pre-UPDATE normalization
// (title trim, body render) runs against the slice-4 repo
// delegation.
const ArticleRecordUpdateColumns = `title=?, subtitle=?, body_md=?, body_html=?, updated_at=?`

// ArticleRecordRepo is the SQLite-backed implementation of
// repo.ArticleRecordRepo.
type ArticleRecordRepo struct {
	db *db.DB
}

// Compile-time check: ArticleRecordRepo satisfies
// repo.ArticleRecordRepo.
var _ repo.ArticleRecordRepo = (*ArticleRecordRepo)(nil)

// NewArticleRecordRepo constructs a SQLite-backed
// ArticleRecordRepo that opens queries against the given
// *db.DB.
func NewArticleRecordRepo(d *db.DB) *ArticleRecordRepo {
	return &ArticleRecordRepo{db: d}
}

// Create inserts a new Article row and returns the generated
// primary-key id.
func (r *ArticleRecordRepo) Create(ctx context.Context, ex repo.Execer, a models.Article) (int64, error) {
	placeholders := strings.Repeat("?,", len(strings.Split(ArticleRecordInsertColumns, ",")))
	placeholders = strings.TrimRight(placeholders, ",")

	query := `INSERT INTO articles (` + ArticleRecordInsertColumns + `) VALUES (` + placeholders + `)`
	args := articleInsertArgs(a)

	res, err := ex.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetByID returns the Article row with the given primary-key
// ID. Returns sql.ErrNoRows if no row matches (surfaced on
// Scan).
func (r *ArticleRecordRepo) GetByID(ctx context.Context, id int64) (*sql.Row, error) {
	conn := r.db.Conn()
	var row *sql.Row
	if err := db.WithBusyRetry(3, func() error {
		row = conn.QueryRowContext(ctx, `SELECT `+ArticleRecordSelectColumns+` FROM articles WHERE id = ?`, id)
		return nil
	}); err != nil {
		return nil, err
	}
	return row, nil
}

// List returns paginated LIVE Articles (is_snapshot = 0)
// ordered by updated_at DESC, id DESC, plus the total live
// row count. The caller must Close the returned *sql.Rows.
//
// The legacy article_service.go::List applied `is_snapshot =
// 0` to both the COUNT and the SELECT; the slice-4 repo
// preserves that filter so the returned total matches the
// page slice exactly (a snapshot row can't shift the
// pagination math).
func (r *ArticleRecordRepo) List(ctx context.Context, page, pageSize int) (*sql.Rows, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	conn := r.db.Conn()
	var total int
	if err := db.WithBusyRetry(3, func() error {
		return conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM articles WHERE is_snapshot = 0`).Scan(&total)
	}); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		q, qErr := conn.QueryContext(ctx,
			`SELECT `+ArticleRecordSelectColumns+` FROM articles WHERE is_snapshot = 0 ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`,
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

// Update modifies the Article row identified by a.ID and
// returns the rows-affected count. The legacy
// `is_snapshot = 0` filter is preserved (snapshots are
// read-only by design; see slice-2 issue #321 locked
// decision #11).
func (r *ArticleRecordRepo) Update(ctx context.Context, ex repo.Execer, a models.Article) (int64, error) {
	query := `UPDATE articles SET ` + ArticleRecordUpdateColumns + ` WHERE id=? AND is_snapshot = 0`
	args := articleUpdateArgs(a)
	args = append(args, a.ID)
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

// Delete removes the Article row identified by id and returns
// the rows-affected count. The legacy Delete filters on
// `is_snapshot = 0` so snapshots are preserved by the
// schema's ON DELETE CASCADE rules.
func (r *ArticleRecordRepo) Delete(ctx context.Context, ex repo.Execer, id int64) (int64, error) {
	res, err := ex.ExecContext(ctx, `DELETE FROM articles WHERE id = ? AND is_snapshot = 0`, id)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// articleInsertArgs builds the args slice for the INSERT
// statement. Column order matches ArticleRecordInsertColumns.
func articleInsertArgs(a models.Article) []interface{} {
	return []interface{}{
		a.SyncID, a.DisplayID, a.Title, a.Subtitle, a.BodyMD, a.BodyHTML,
		a.CreatedAt, a.UpdatedAt, a.SnapshotOfID, a.IsSnapshot,
	}
}

// articleUpdateArgs builds the args slice for the UPDATE
// statement. Column order matches ArticleRecordUpdateColumns
// (5 columns: title, subtitle, body_md, body_html, updated_at).
// The repo's Update deliberately doesn't write sync_id,
// display_id, snapshot_of_id, or is_snapshot on Update —
// those are set at Create time and never mutate. The legacy
// inline SQL preserved the same constraint.
func articleUpdateArgs(a models.Article) []interface{} {
	return []interface{}{
		a.Title, a.Subtitle, a.BodyMD, a.BodyHTML, a.UpdatedAt,
	}
}