// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file ships the SQLite impl of EventRecordRepo (issue
// #613 slice 3). Parity contract: every method's SQL matches
// the legacy inline SQL in
// internal/records/event_service.go verbatim. The slice-3
// parity test (in internal/records/) asserts the same shape
// end-to-end.
package sqlite

import (
	"context"
	"database/sql"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
)

// EventRecordListColumns is the SELECT column list used by
// EventRecordRepo.ListEvents + ListForPerson. Matches the
// legacy `soldierSelectColumns` shape (47 cols, no cross-
// table subqueries) — the legacy ListEvents / ListForPerson
// inline SQL used `soldierSelectColumns`, NOT
// `soldierListSelectColumns`. The cross-table subqueries
// (spouse display_id, record count, image count) are
// soldier-browse-specific; Event Records don't render them.
const EventRecordListColumns = PersonRecordSelectColumns

// EventRecordRepo is the SQLite-backed implementation of
// repo.EventRecordRepo.
type EventRecordRepo struct {
	db *db.DB
}

// Compile-time check: EventRecordRepo satisfies
// repo.EventRecordRepo. Catches signature drift at compile
// time, not at the call site.
var _ repo.EventRecordRepo = (*EventRecordRepo)(nil)

// NewEventRecordRepo constructs a SQLite-backed
// EventRecordRepo that opens queries against the given
// *db.DB.
func NewEventRecordRepo(d *db.DB) *EventRecordRepo {
	return &EventRecordRepo{db: d}
}

// ListEvents returns paginated Event Records
// (entry_type='event') ordered by updated_at DESC, id DESC,
// plus the total row count. The caller must Close the
// returned *sql.Rows.
//
// Mirrors the legacy inline SQL in
// internal/records/event_service.go::ListEvents (page/pageSize
// clamping + ORDER BY updated_at DESC, id DESC + LIMIT/OFFSET).
func (r *EventRecordRepo) ListEvents(ctx context.Context, page, pageSize int) (*sql.Rows, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	conn := r.db.Conn()
	var total int
	if err := db.WithBusyRetry(3, func() error {
		return conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM soldiers WHERE entry_type = 'event'`).Scan(&total)
	}); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		q, qErr := conn.QueryContext(ctx,
			`SELECT `+EventRecordListColumns+` FROM soldiers WHERE entry_type = 'event' ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`,
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

// LinksForEvent returns the junction rows connecting the
// given Event to its linked Person Records, ordered by
// created_at, id. Mirrors the legacy inline SQL in
// internal/records/event_service.go::linksForEvent.
func (r *EventRecordRepo) LinksForEvent(ctx context.Context, q repo.Querier, eventID int64) (*sql.Rows, error) {
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx,
			`SELECT epl.id, epl.event_id, COALESCE(e.sync_id, ''), epl.person_id, COALESCE(p.sync_id, ''), COALESCE(p.display_id, ''), COALESCE(epl.created_at, '')
			 FROM event_person_links epl
			 JOIN soldiers e ON e.id = epl.event_id
			 JOIN soldiers p ON p.id = epl.person_id
			 WHERE epl.event_id = ?
			 ORDER BY p.display_id`,
			eventID,
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

// AttachEventToPerson inserts a junction row linking the
// given Event to the given Person. Returns the generated
// link id.
//
// The legacy code uses `db.WithBusyRetry` around the Exec;
// the slice-3 impl preserves the same retry pattern.
func (r *EventRecordRepo) AttachEventToPerson(ctx context.Context, ex repo.Execer, eventID, personID int64, syncID, eventSyncID, personSyncID string) (int64, error) {
	var id int64
	if err := db.WithBusyRetry(3, func() error {
		res, err := ex.ExecContext(ctx,
			`INSERT INTO event_person_links (event_id, person_id, sync_id, event_sync_id, person_sync_id) VALUES (?, ?, ?, ?, ?)`,
			eventID, personID, syncID, eventSyncID, personSyncID,
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	}); err != nil {
		return 0, err
	}
	return id, nil
}

// DetachEventFromPerson removes the junction row. Returns
// the rows-affected count (0 if no row matched).
func (r *EventRecordRepo) DetachEventFromPerson(ctx context.Context, ex repo.Execer, eventID, personID int64) (int64, error) {
	var n int64
	if err := db.WithBusyRetry(3, func() error {
		res, err := ex.ExecContext(ctx,
			`DELETE FROM event_person_links WHERE event_id = ? AND person_id = ?`,
			eventID, personID,
		)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ListForPerson returns the Event Records linked to the
// given Person, ordered by created_at, id. The caller must
// Close the returned *sql.Rows.
//
// Mirrors the legacy inline SQL in
// internal/records/event_service.go::ListForPerson.
func (r *EventRecordRepo) ListForPerson(ctx context.Context, q repo.Querier, personID int64) (*sql.Rows, error) {
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx,
			`SELECT `+EventRecordListColumns+`
			 FROM soldiers
			 WHERE soldiers.id IN (SELECT event_id FROM event_person_links WHERE person_id = ?)
			   AND soldiers.entry_type = 'event'
			 ORDER BY soldiers.updated_at DESC, soldiers.id DESC`,
			personID,
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