// Package repo declares the repository interfaces that decouple
// DixieData's domain services (internal/records/, internal/articles/,
// etc.) from the concrete database driver.
//
// Slice 3 of issue #613 adds the EventRecordRepo seam.
//
// Event Records live in the `soldiers` table with
// `entry_type = 'event'`; the `event_person_links` junction
// connects Events to Person Records. EventRecordRepo covers
// both: event-shaped reads on the soldiers table + the
// junction-table operations.
//
// Design contract (same as PersonRecordRepo):
//
//   - Repos own SQL. Return *sql.Rows for collections;
//     return int64 for write-result counts.
//   - Context-aware: every method takes context.Context.
//   - Concrete impls live under subpackages
//     (internal/db/repo/sqlite/ for SQLite-backed).
package repo

import (
	"context"
	"database/sql"
)

// EventRecordRepo is the data-access seam for Event Records
// (soldiers.entry_type = 'event') + the event_person_links
// junction.
//
// Slice 3 covers the 5 highest-coupling inline SQL paths in
// internal/records/event_service.go. Future slices add
// LinkCount + Count + any other event-shaped reads.
type EventRecordRepo interface {
	// ListEvents returns a paginated slice of Event Records
	// (entry_type='event') ordered by kind, begin_date, id,
	// plus the total count. Returns *sql.Rows that the caller
	// must Close.
	ListEvents(ctx context.Context, page, pageSize int) (*sql.Rows, int, error)

	// LinksForEvent returns the junction rows connecting the
	// given Event to its linked Person Records, ordered by
	// created_at, id. Returns *sql.Rows the caller must Close.
	LinksForEvent(ctx context.Context, q Querier, eventID int64) (*sql.Rows, error)

	// AttachEventToPerson inserts a junction row linking the
	// given Event to the given Person. Returns the generated
	// link id. The caller supplies an Execer (typically
	// *sql.Tx for atomicity with other writes).
	AttachEventToPerson(ctx context.Context, ex Execer, eventID, personID int64, syncID, eventSyncID, personSyncID string) (int64, error)

	// DetachEventFromPerson removes the junction row. Returns
	// the rows-affected count (0 if no row matched).
	DetachEventFromPerson(ctx context.Context, ex Execer, eventID, personID int64) (int64, error)

	// ListForPerson returns the Event Records linked to the
	// given Person, ordered by created_at, id. Returns
	// *sql.Rows the caller must Close.
	ListForPerson(ctx context.Context, q Querier, personID int64) (*sql.Rows, error)
}