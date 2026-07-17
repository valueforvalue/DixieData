// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file ships the SQLite impl of CalendarItemRepo (issue
// #613 slice 6). Parity contract: every method's SQL matches
// the legacy inline SQL in
// internal/records/calendar_service.go verbatim.
package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
	"github.com/valueforvalue/DixieData/internal/models"
)

// CalendarItemSelectColumns is the SELECT column list used by
// CalendarItemRepo.GetByID + ListForMonthDay (8 columns: id,
// item_type, month, day, title, notes, created_at,
// updated_at). Matches the legacy inline SQL in
// internal/records/calendar_service.go verbatim.
const CalendarItemSelectColumns = `id, item_type, month, day, title, notes, created_at, updated_at`

// CalendarItemInsertColumns is the column list used by
// CalendarItemRepo.Create (6 columns: item_type, month, day,
// title, notes, updated_at). created_at relies on the
// schema's DEFAULT CURRENT_TIMESTAMP — the legacy Create
// path doesn't write it explicitly. Matches the legacy
// inline SQL verbatim.
const CalendarItemInsertColumns = `item_type, month, day, title, notes, updated_at`

// CalendarItemUpdateColumns is the SET-clause column list
// used by CalendarItemRepo.Update (4 columns: item_type,
// title, notes, updated_at). The legacy Update path doesn't
// rewrite month/day — only the user-supplied fields. Matches
// the legacy inline SQL verbatim.
const CalendarItemUpdateColumns = `item_type=?, title=?, notes=?, updated_at=?`

// CalendarItemRepo is the SQLite-backed implementation of
// repo.CalendarItemRepo.
type CalendarItemRepo struct {
	db *db.DB
}

var _ repo.CalendarItemRepo = (*CalendarItemRepo)(nil)

// NewCalendarItemRepo constructs a SQLite-backed
// CalendarItemRepo.
func NewCalendarItemRepo(d *db.DB) *CalendarItemRepo {
	return &CalendarItemRepo{db: d}
}

// Create inserts a new CalendarItem and returns the generated
// primary-key id.
func (r *CalendarItemRepo) Create(ctx context.Context, ex repo.Execer, item models.CalendarItem) (int64, error) {
	placeholders := strings.Repeat("?,", len(strings.Split(CalendarItemInsertColumns, ",")))
	placeholders = strings.TrimRight(placeholders, ",")
	query := `INSERT INTO calendar_items (` + CalendarItemInsertColumns + `) VALUES (` + placeholders + `)`
	res, err := ex.ExecContext(ctx, query, calendarInsertArgs(item)...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetByID returns the CalendarItem row with the given
// primary-key ID.
func (r *CalendarItemRepo) GetByID(ctx context.Context, id int64) (*sql.Row, error) {
	conn := r.db.Conn()
	var row *sql.Row
	if err := db.WithBusyRetry(3, func() error {
		row = conn.QueryRowContext(ctx, `SELECT `+CalendarItemSelectColumns+` FROM calendar_items WHERE id = ?`, id)
		return nil
	}); err != nil {
		return nil, err
	}
	return row, nil
}

// ListForMonthDay returns every CalendarItem for the given
// (month, day) pair, ordered by item_type (holiday < event <
// other) + title + id.
func (r *CalendarItemRepo) ListForMonthDay(ctx context.Context, q repo.Querier, month, day int) (*sql.Rows, error) {
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx,
			`SELECT `+CalendarItemSelectColumns+` FROM calendar_items WHERE month = ? AND day = ? ORDER BY CASE item_type WHEN 'holiday' THEN 0 WHEN 'event' THEN 1 ELSE 2 END, LOWER(title), id`,
			month, day,
		)
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

// Update modifies the CalendarItem identified by item.ID and
// returns the rows-affected count.
func (r *CalendarItemRepo) Update(ctx context.Context, ex repo.Execer, item models.CalendarItem) (int64, error) {
	query := `UPDATE calendar_items SET ` + CalendarItemUpdateColumns + ` WHERE id=?`
	args := calendarUpdateArgs(item)
	args = append(args, item.ID)
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

// Delete removes the CalendarItem identified by id and
// returns the rows-affected count.
func (r *CalendarItemRepo) Delete(ctx context.Context, ex repo.Execer, id int64) (int64, error) {
	res, err := ex.ExecContext(ctx, `DELETE FROM calendar_items WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// calendarInsertArgs builds the args slice for the INSERT.
// Column order matches CalendarItemInsertColumns (6 cols:
// item_type, month, day, title, notes, updated_at).
func calendarInsertArgs(item models.CalendarItem) []interface{} {
	return []interface{}{
		item.ItemType, item.Month, item.Day, item.Title, item.Notes, item.UpdatedAt,
	}
}

// calendarUpdateArgs builds the args slice for the UPDATE.
// Column order matches CalendarItemUpdateColumns (4 cols:
// item_type, title, notes, updated_at).
func calendarUpdateArgs(item models.CalendarItem) []interface{} {
	return []interface{}{
		item.ItemType, item.Title, item.Notes, item.UpdatedAt,
	}
}