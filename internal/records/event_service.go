package records

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// EventLink is the per-row payload of the event_person_links junction
// (issue #320). One Event can link to N Person Records; one Person
// Record can be linked to N Events. The row carries the link ID plus
// the per-row sync_id mirror (matches the records.person_sync_id
// pattern) so distributed merge can reconcile linked rows across
// two nodes that have the same Event but different row IDs.
type EventLink struct {
	ID            int64
	EventID       int64
	EventSyncID   string
	PersonID      int64
	PersonSyncID  string
	PersonDisplay string
	CreatedAt     string
}

// EventWithLinks is the read-side projection of an Event Record
// plus its linked Person Records. Returned by GetEventByID and
// GetEventByDisplayID.
type EventWithLinks struct {
	Event models.Soldier
	Links []EventLink
}

// EventService is the slice-2 service for Event Records (issue #320).
// Mirrors the soldierService shape: every Event Record is a
// `soldiers` row with `entry_type = 'event'`, so the read/write
// surface is layered on top of the existing Person Record
// infrastructure. The separate service exists so handlers can
// route to a focused API surface (Event-only methods) without
// dragging the full Person Record service into the call site.
//
// The service does NOT own its own DB connection; it borrows
// SoldierService.db via a constructor. This keeps the v59→v60
// transactional discipline intact: every Event write goes
// through the same tx pool as Person Record writes.
type EventService struct {
	soldiers *SoldierService
}

// NewEventService constructs an EventService that borrows the
// given SoldierService's database handle. Returns a focused
// API surface; the underlying writes still flow through
// SoldierService.Create / Update / Delete (which the v60
// normalizeSoldierEntry bypass for entry_type='event' makes
// safe).
func NewEventService(soldiers *SoldierService) *EventService {
	return &EventService{soldiers: soldiers}
}

// ListSourcesForEvent returns the Source Records attached to the
// Event. Source Records live in the dedicated event_sources
// table (issue #340 / v61). v60 reused the shared records
// table, but SoldierService.Update's replaceRecords REPLACE-only
// semantics silently destroyed every attached source on every
// Event Edit. The v61 fix moves Event sources to their own
// table so the Update path leaves them alone.
func (e *EventService) ListSourcesForEvent(eventID int64) ([]models.Record, error) {
	rows, err := e.soldiers.db.Conn().Query(
		`SELECT id, sync_id, record_type, app_id, details
		 FROM event_sources
		 WHERE event_id = ?
		 ORDER BY id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Record, 0)
	for rows.Next() {
		var r models.Record
		if err := rows.Scan(&r.ID, &r.SyncID, &r.RecordType, &r.AppID, &r.Details); err != nil {
			return nil, err
		}
		r.PersonRecordID = eventID
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListTagsForEvent returns the Tags attached to the Event
// (person_record_tags row join). Re-uses the same table as
// Person Record tags since v60 renamed the FK to
// person_record_id; Events are soldiers rows.
func (e *EventService) ListTagsForEvent(eventID int64) ([]Tag, error) {
	rows, err := e.soldiers.db.Conn().Query(
		`SELECT t.id, t.name, t.normalized_name, t.created_at
		 FROM tags t
		 JOIN person_record_tags pt ON pt.tag_id = t.id
		 WHERE pt.person_id = ?
		 ORDER BY t.name COLLATE NOCASE`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.NormalizedName, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AddTagToEvent inserts a person_record_tags row for the Event.
// INSERT OR IGNORE so duplicate-add is a no-op.
func (e *EventService) AddTagToEvent(eventID, tagID int64) error {
	if eventID < 1 || tagID < 1 {
		return fmt.Errorf("event and tag ids must be positive")
	}
	_, err := e.soldiers.db.Conn().Exec(
		`INSERT OR IGNORE INTO person_record_tags (person_id, tag_id) VALUES (?, ?)`,
		eventID, tagID)
	return err
}

// DetachTagFromEvent removes the person_record_tags row.
func (e *EventService) DetachTagFromEvent(eventID, tagID int64) error {
	if eventID < 1 || tagID < 1 {
		return fmt.Errorf("event and tag ids must be positive")
	}
	_, err := e.soldiers.db.Conn().Exec(
		`DELETE FROM person_record_tags WHERE person_id = ? AND tag_id = ?`,
		eventID, tagID)
	return err
}

// AttachSourceToEvent inserts a row into event_sources for the
// given Event. The PersonRecordID / PersonSyncID fields on the
// supplied source are ignored (events use event_id /
// event_sync_id); the handler layer only carries record_type,
// app_id, and details. SyncID is minted if absent (distributed-
// merge ready).
func (e *EventService) AttachSourceToEvent(eventID int64, source models.Record) (int64, error) {
	if eventID < 1 {
		return 0, fmt.Errorf("event id must be positive")
	}
	if strings.TrimSpace(source.SyncID) == "" {
		syncID, err := db.NewSyncID()
		if err != nil {
			return 0, err
		}
		source.SyncID = syncID
	}
	if source.PersonSyncID == "" {
		eventRow, err := e.soldiers.GetByID(eventID)
		if err != nil {
			return 0, err
		}
		source.PersonSyncID = eventRow.SyncID
	}
	res, err := e.soldiers.db.Conn().Exec(
		`INSERT INTO event_sources (sync_id, event_id, event_sync_id, record_type, app_id, details) VALUES (?, ?, ?, ?, ?, ?)`,
		source.SyncID, eventID, source.PersonSyncID, source.RecordType, source.AppID, source.Details,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DetachSourceFromEvent removes a single source row by its
// primary key. Verifies the row belongs to the given Event so a
// malicious sourceId cannot drop an unrelated row.
func (e *EventService) DetachSourceFromEvent(eventID, sourceID int64) error {
	res, err := e.soldiers.db.Conn().Exec(
		`DELETE FROM event_sources WHERE id = ? AND event_id = ?`,
		sourceID, eventID,
	)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("source %d not attached to event %d", sourceID, eventID)
	}
	return nil
}

// CreateEvent mints a new EVT-NNNNN Display ID (via
// (*DB).NextEventID), populates the per-subtype columns
// (kind/begin_date/end_date/description), and persists the row
// via SoldierService.Create. The Create path's
// normalizeSoldierEntry already short-circuits for entry_type =
// 'event' (the spouse-soldier-id check is bypassed; the maiden
// name is cleared).
func (e *EventService) CreateEvent(event models.Soldier) (*models.Soldier, error) {
	event.EntryType = models.EntryTypeEvent
	event.Kind = strings.TrimSpace(event.Kind)
	event.BeginDate = strings.TrimSpace(event.BeginDate)
	event.EndDate = strings.TrimSpace(event.EndDate)
	event.Description = strings.TrimSpace(event.Description)
	event.PDFExcerptOverride = strings.TrimSpace(event.PDFExcerptOverride)
	// Person-specific fields stay blank for an Event.
	event.FirstName = ""
	event.MiddleName = ""
	event.LastName = ""
	event.Suffix = ""
	event.Prefix = ""
	event.Rank = ""
	event.Unit = ""
	event.BuriedIn = ""
	event.Biography = ""

	if event.DisplayID == "" {
		id, err := e.soldiers.db.NextEventID()
		if err != nil {
			return nil, err
		}
		event.DisplayID = id
	}
	return e.soldiers.Create(event)
}

// UpdateEvent updates an existing Event Record. The event is
// re-fetched before the write so the update path always operates
// on the current row (avoids the stale-write race when a
// Service Timeline rebuild lands between read and write).
func (e *EventService) UpdateEvent(event models.Soldier) error {
	existing, err := e.soldiers.GetByID(event.ID)
	if err != nil {
		return err
	}
	if existing.EntryType != models.EntryTypeEvent {
		return fmt.Errorf("person record %d is %q, not an Event", event.ID, existing.EntryType)
	}
	// Preserve the immutable columns on update.
	event.EntryType = models.EntryTypeEvent
	event.DisplayID = existing.DisplayID
	event.SyncID = existing.SyncID
	event.SpouseSoldierID = 0
	event.FirstName = ""
	event.MiddleName = ""
	event.LastName = ""
	return e.soldiers.Update(event)
}

// DeleteEvent removes an Event Record. Re-fetches the row
// first to defend against a stale-write race. The cascade
// on event_person_links is handled by the SQLite schema's
// ON DELETE CASCADE on the FK (issue #320 v60 schema).
func (e *EventService) DeleteEvent(id int64) error {
	existing, err := e.soldiers.GetByID(id)
	if err != nil {
		return err
	}
	if existing.EntryType != models.EntryTypeEvent {
		return fmt.Errorf("person record %d is %q, not an Event", id, existing.EntryType)
	}
	return e.soldiers.Delete(id)
}

// GetEventByID fetches an Event Record + its linked Person
// Records. Returns ErrNotFound if the row is missing or is
// not an Event.
func (e *EventService) GetEventByID(id int64) (*EventWithLinks, error) {
	row, err := e.soldiers.GetByID(id)
	if err != nil {
		return nil, err
	}
	if row.EntryType != models.EntryTypeEvent {
		return nil, fmt.Errorf("person record %d is %q, not an Event", id, row.EntryType)
	}
	links, err := e.linksForEvent(id)
	if err != nil {
		return nil, err
	}
	return &EventWithLinks{Event: *row, Links: links}, nil
}

// GetEventByDisplayID fetches an Event by its EVT-NNNNN Display
// ID. Returns ErrNotFound if no row matches or the matched row
// is not an Event.
func (e *EventService) GetEventByDisplayID(displayID string) (*EventWithLinks, error) {
	row, err := e.soldiers.GetByDisplayID(displayID)
	if err != nil {
		return nil, err
	}
	if row.EntryType != models.EntryTypeEvent {
		return nil, fmt.Errorf("display id %q is %q, not an Event", displayID, row.EntryType)
	}
	links, err := e.linksForEvent(row.ID)
	if err != nil {
		return nil, err
	}
	return &EventWithLinks{Event: *row, Links: links}, nil
}

// ListEvents returns a page of Event Records sorted by updated_at
// DESC. Excludes the linked-Person-Records subquery for
// efficiency; callers that need the link set per event should
// call GetEventByID for the visible rows.
func (e *EventService) ListEvents(page, pageSize int) ([]models.Soldier, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	offset := (page - 1) * pageSize
	conn := e.soldiers.db.Conn()

	rows, err := conn.Query(
		`SELECT `+soldierSelectColumns+` FROM soldiers WHERE entry_type = ? ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`,
		models.EntryTypeEvent, pageSize, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSoldiers(rows)
}

// AttachEventToPerson creates an event_person_links row. The
// event_id and person_id must both reference existing soldiers
// rows; the FK is enforced by the schema. Returns the link ID
// and a sentinel ErrDuplicateLink if the pair already exists.
func (e *EventService) AttachEventToPerson(eventID, personID int64) (int64, error) {
	if eventID < 1 || personID < 1 {
		return 0, fmt.Errorf("event id and person id must be positive")
	}
	// Verify the event is actually an Event Record (defense in
	// depth; the FK allows any soldiers.id on either side).
	var eventType, personType string
	if err := e.soldiers.db.Conn().QueryRow(`SELECT entry_type FROM soldiers WHERE id = ?`, eventID).Scan(&eventType); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("event record %d not found", eventID)
		}
		return 0, err
	}
	if err := e.soldiers.db.Conn().QueryRow(`SELECT entry_type FROM soldiers WHERE id = ?`, personID).Scan(&personType); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("person record %d not found", personID)
		}
		return 0, err
	}
	if eventType != models.EntryTypeEvent {
		return 0, fmt.Errorf("record %d is %q, not an Event", eventID, eventType)
	}
	if personType == models.EntryTypeEvent {
		return 0, fmt.Errorf("record %d is also an Event; link must be Event↔Person", personID)
	}

	tx, err := e.soldiers.db.Conn().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Mint a sync_id for the link row (matches the records.sync_id
	// discipline).
	syncID, err := db.NewSyncID()
	if err != nil {
		return 0, err
	}
	res, err := tx.Exec(
		`INSERT INTO event_person_links (event_id, person_id, sync_id, event_sync_id, person_sync_id) VALUES (?, ?, ?, (SELECT sync_id FROM soldiers WHERE id = ?), (SELECT sync_id FROM soldiers WHERE id = ?))`,
		eventID, personID, syncID, eventID, personID,
	)
	if err != nil {
		// SQLite UNIQUE constraint violation.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, ErrDuplicateLink
		}
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// DetachEventFromPerson removes the event_person_links row. No
// error if the row does not exist (idempotent).
func (e *EventService) DetachEventFromPerson(eventID, personID int64) error {
	_, err := e.soldiers.db.Conn().Exec(
		`DELETE FROM event_person_links WHERE event_id = ? AND person_id = ?`,
		eventID, personID,
	)
	return err
}

// ListForPerson returns the Events linked to the given Person
// Record (the Events tab on the Person Record detail page).
func (e *EventService) ListForPerson(personID int64) ([]models.Soldier, error) {
	rows, err := e.soldiers.db.Conn().Query(
		`SELECT `+soldierSelectColumns+` FROM soldiers
		 WHERE id IN (SELECT event_id FROM event_person_links WHERE person_id = ?)
		 ORDER BY updated_at DESC, id DESC`,
		personID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSoldiers(rows)
}

// ListForEvent returns the Person Records linked to the given
// Event (the Linked Person Records section on the Event detail
// page). Returns the linked Person Records in display-ID order.
func (e *EventService) ListForEvent(eventID int64) ([]models.Soldier, error) {
	rows, err := e.soldiers.db.Conn().Query(
		`SELECT `+soldierSelectColumns+` FROM soldiers
		 WHERE id IN (SELECT person_id FROM event_person_links WHERE event_id = ?)
		 ORDER BY last_name, first_name, display_id`,
		eventID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSoldiers(rows)
}

// LinkCount returns the number of links for a given Event.
// Used by the Quality Scan event-zero-links check (an Event
// Record with zero links is a review-queue candidate).
func (e *EventService) LinkCount(eventID int64) (int, error) {
	var n int
	if err := e.soldiers.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM event_person_links WHERE event_id = ?`,
		eventID,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// linksForEvent returns the per-link EventLink rows for the
// given Event, joined with the linked Person's display_id for
// the UI.
func (e *EventService) linksForEvent(eventID int64) ([]EventLink, error) {
	rows, err := e.soldiers.db.Conn().Query(
		`SELECT epl.id, epl.event_id, COALESCE(e.sync_id, ''), epl.person_id, COALESCE(p.sync_id, ''), COALESCE(p.display_id, ''), COALESCE(epl.created_at, '')
		 FROM event_person_links epl
		 JOIN soldiers e ON e.id = epl.event_id
		 JOIN soldiers p ON p.id = epl.person_id
		 WHERE epl.event_id = ?
		 ORDER BY p.display_id`,
		eventID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []EventLink
	for rows.Next() {
		var link EventLink
		if err := rows.Scan(&link.ID, &link.EventID, &link.EventSyncID, &link.PersonID, &link.PersonSyncID, &link.PersonDisplay, &link.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}


// ErrDuplicateLink is returned by AttachEventToPerson when the
// (event_id, person_id) pair already exists in event_person_links.
var ErrDuplicateLink = errors.New("event-person link already exists")
