package records

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/debug"
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
	registry EventRegistry
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
		`SELECT id, sync_id, record_type, app_id, details, sort_order
		 FROM event_sources
		 WHERE event_id = ?
		 ORDER BY sort_order, id`, eventID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "ListSourcesForEvent.rows")
	out := make([]models.Record, 0)
	for rows.Next() {
		var r models.Record
		if err := rows.Scan(&r.ID, &r.SyncID, &r.RecordType, &r.AppID, &r.Details, &r.SortOrder); err != nil {
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
	defer debug.DeferCloseLog(rows, "ListTagsForEvent.rows")
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
func (e *EventService) AttachSourceToEvent(eventID int64, source models.Record, sortOrder int64) (int64, error) {
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
	// Issue #368 slice 2: pass sort_order from the array index
	// so a re-save preserves the user's current display order.
	res, err := e.soldiers.db.Conn().Exec(
		`INSERT INTO event_sources (sync_id, event_id, event_sync_id, record_type, app_id, details, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		source.SyncID, eventID, source.PersonSyncID, source.RecordType, source.AppID, source.Details, sortOrder,
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

// AttachSourcesToEvent inserts a batch of source rows for the
// given Event. Issue #357 (v1 follow-up): the Event create +
// edit forms expose inline Source Record rows that submit
// alongside the main form (mirrors the soldier entry form's
// Records[] pattern). Empty rows (no record_type + no app_id +
// no details) are skipped so the user can leave the blank row
// in place without poisoning the data.
//
// Each insert mints its own SyncID; the dedicated event_sources
// table is outside replaceRecords' DELETE scope so the Event
// Update path does not collide (issue #340 / v61 fix).
func (e *EventService) AttachSourcesToEvent(eventID int64, sources []models.Record) ([]int64, error) {
	if eventID < 1 {
		return nil, fmt.Errorf("event id must be positive")
	}
	ids := make([]int64, 0, len(sources))
	for idx, src := range sources {
		if strings.TrimSpace(src.RecordType) == "" && strings.TrimSpace(src.AppID) == "" && strings.TrimSpace(src.Details) == "" {
			continue
		}
		id, err := e.AttachSourceToEvent(eventID, src, int64(idx))
		if err != nil {
			return ids, fmt.Errorf("attach source %q: %w", src.RecordType, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
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
// Records + its attached Source Records. Returns ErrNotFound
// if the row is missing or is not an Event. The Event's
// Sources are loaded into row.EventSources from the dedicated
// event_sources table (issue #340 / v61); row.Records stays
// empty because v60's records-table reuse was removed.
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
	sources, err := e.ListSourcesForEvent(id)
	if err != nil {
		return nil, err
	}
	row.EventSources = sources
	return &EventWithLinks{Event: *row, Links: links}, nil
}

// GetEventByDisplayID fetches an Event by its EVT-NNNNN Display
// ID + its linked Person Records + its attached Source Records.
// Returns ErrNotFound if no row matches or the matched row is
// not an Event. Sources are loaded from event_sources (issue
// #340 / v61) and stored on row.EventSources.
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
	sources, err := e.ListSourcesForEvent(row.ID)
	if err != nil {
		return nil, err
	}
	row.EventSources = sources
	return &EventWithLinks{Event: *row, Links: links}, nil
}

// LookupPersonIDByDisplayID resolves a Person Record Display ID
// (e.g. 'SOL-00042') to its numeric row ID. Used by the Event
// editor's Add Linked Person form (issue #361 slice 2), which
// posts a Display ID string from the user rather than forcing
// them to know raw row IDs. Delegates to SoldierService
// .GetByDisplayID (case-insensitive, whitespace-trimmed,
// returns os.ErrNotExist on missing/empty) so the lookup
// behavior matches the soldier-side attach pattern. A nil
// receiver returns an error rather than panicking — defense
// in depth in case the seam ever swaps to a nilable service.
func (e *EventService) LookupPersonIDByDisplayID(displayID string) (int64, error) {
	if e == nil || e.soldiers == nil {
		return 0, fmt.Errorf("event service not initialized")
	}
	row, err := e.soldiers.GetByDisplayID(displayID)
	if err != nil {
		return 0, err
	}
	return row.ID, nil

}
// LookupPersonIDByName resolves a Person Record by free-text
// name fragment (issue #373). Used by the Event editor's
// Add Linked Person form as the FALLBACK path when the
// user's input does not match an existing Display ID. The
// search is a case-insensitive substring match against the
// concatenated first + middle + last + suffix name fields
// (whitespace-normalized: trimmed, internal whitespace
// collapsed to single spaces so "Robert  E." and "Robert E."
// behave the same). Results are sorted by display_id and the
// FIRST row wins — that is the locked decision from issue
// #373, so the helper is deterministic and the handler can
// surface "no match" cleanly when the result set is empty.
//
// Empty input returns os.ErrNotExist without running a
// query — %""% would match every row and silently attach a
// random Person Record, which is exactly the bug slice 2 of
// #361 was supposed to prevent. The 400-vs-404 split happens
// in the handler (empty input is validation, no-match is
// not-found); the service just returns the sentinel.
//
// Returns os.ErrNotExist on missing/uninit receiver (same
// pattern as LookupPersonIDByDisplayID) — the handler's
// errors.Is(err, os.ErrNotExist) branch maps it to HTTP 404.
func (e *EventService) LookupPersonIDByName(nameFragment string) (int64, error) {
	if e == nil || e.soldiers == nil {
		return 0, fmt.Errorf("event service not initialized")
	}
	normalized := strings.Join(strings.Fields(nameFragment), " ")
	if normalized == "" {
		return 0, os.ErrNotExist
	}
	pattern := "%" + normalized + "%"
	conn := e.soldiers.db.Conn()
	row := conn.QueryRow(
		`SELECT id FROM soldiers
		 WHERE upper(coalesce(first_name,'') || ' ' || coalesce(middle_name,'') || ' ' || coalesce(last_name,'') || ' ' || coalesce(suffix,''))
		       LIKE upper(?)
		 ORDER BY display_id ASC
		 LIMIT 1`,
		pattern,
	)
	var id int64
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, os.ErrNotExist
		}
		return 0, err
	}
	return id, nil
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
	defer debug.DeferCloseLog(rows, "ListEvents.rows")
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
	defer debug.DeferCloseLog(rows, "ListForPerson.rows")
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
	defer debug.DeferCloseLog(rows, "ListForEvent.rows")
	return scanSoldiers(rows)
}

// Count returns the total number of Event Records in the Local
// Archive. Distinct from LinkCount (per-event link count) and
// from the per-month summary EventCount (per-calendar-day
// count). Powers the /inventory page + the Calendar header
// archive rollup (issue #491).
func (e *EventService) Count() (int, error) {
	var n int
	if err := e.soldiers.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM soldiers WHERE entry_type = 'event'`,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// EventKindCount is one bucket in the per-Event-kind rollup
// returned by KindRollup. Kind is the free-text event kind
// (Battle, Campaign, Death, Marriage, Hospital stay, etc. —
// no enum per issue #320). Count is the number of Event Records
// of that kind. Kinds with Count == 0 are not returned.
type EventKindCount struct {
	Kind  string
	Count int
}

// KindRollup returns the per-kind Event Record count for the
// /inventory page (issue #491). Empty/blank kinds are bucketed
// under "(unspecified)" so the rollup doesn't lose rows to a
// free-text typo or an import that didn't normalize. Ordered by
// count descending so the most common kind surfaces first.
func (e *EventService) KindRollup() ([]EventKindCount, error) {
	rows, err := e.soldiers.db.Conn().Query(`
		SELECT
			COALESCE(NULLIF(TRIM(kind), ''), '(unspecified)') AS kind,
			COUNT(*) AS n
		FROM soldiers
		WHERE LOWER(TRIM(entry_type)) = 'event'
		GROUP BY kind
		ORDER BY n DESC, kind ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventKindCount
	for rows.Next() {
		var item EventKindCount
		if err := rows.Scan(&item.Kind, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// LinkCount returns the number of person links for a given
// Event. Used by the Quality Scan event-zero-links check
// (an Event Record with zero links is a review-queue candidate).
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
	defer debug.DeferCloseLog(rows, "linksForEvent.rows")
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

// AddImage attaches an image row to an Event Record.
// (issue #320 child #332 close-out)
//
// Thin pass-through to SoldierService.AddImage. The handler-side
// facade guard at internal/appshell/app_facades.go:167 routes
// event image CRUD through EventService to satisfy the
// 'handlers MUST NOT call a.soldiers methods' law. The native
// dialog import gateway (App.importImagePaths at
// internal/appshell/app.go:2480) still writes through
// soldiers.AddImage internally because it serves both Person
// Records and Events from one shared path.
func (e *EventService) AddImage(eventID int64, fileName, relativePath, caption string) error {
	return e.soldiers.AddImage(eventID, fileName, relativePath, caption)
}

// RemoveImages deletes the image rows + tag-join rows for the
// given image IDs on an Event Record. (issue #320 child #332
// close-out.) Pass-through to SoldierService.DeleteImages;
// signatures match because both entry types share the images
// table keyed on person_record_id.
func (e *EventService) RemoveImages(eventID int64, imageIDs []int64) error {
	return e.soldiers.DeleteImages(eventID, imageIDs)
}

// EventRegistry is the issue #374 surface the EventService
// uses to pre-render an Event Record's PDF. Mirrors the
// ArticleRegistry shape (article_service.go:65-68) so the
// appshell wiring stays symmetric: a thin adapter bridges
// *render.Registry to this interface.
//
// The interface lives in internal/records (not pkg/render) to
// avoid an import cycle: pkg/render depends on internal/records,
// so internal/records cannot import pkg/render. Same reasoning
// is documented at ArticleRegistry above.
type EventRegistry interface {
	RenderEvent(ctx context.Context, recordType string, orientation string, data map[string]any, w io.Writer) error
}

// SetEventRegistry wires the typst-backed Registry into the
// EventService. Called once at startup by the appshell wiring
// (mirrors ArticleService.SetArticleRegistry); nil clears the
// wiring so RenderPDF returns an error instead of panicking.
func (e *EventService) SetEventRegistry(reg EventRegistry) {
	e.registry = reg
}

// RenderPDF pre-renders the Event Record's PDF body to bytes
// and returns them alongside a slugified filename. Mirrors
// ArticleService.RenderPDF so the handler can pre-render +
// open a SaveFileDialog + write synchronously, matching the
// article path (no job-enqueue overhead for the per-export
// click).
//
// orientation is "portrait" or "landscape"; both resolve to
// templates/event_<orientation>.typ via the Registry's
// templateForRecordType mapping. The linked slice is the
// slim per-Person projection for the "Linked Person Records"
// table; the pre-projection keeps the typst template DB-free.
//
// Errors:
//   - registry not configured: explicit error so the caller
//     can surface a misconfiguration rather than crash
//   - GetEventByID failure:    propagates as-is (the handler
//     already maps ErrEventNotFound to a 404)
//   - registry render failure: wraps the underlying error so
//     the handler can log it with context
func (e *EventService) RenderPDF(eventID int64, orientation string) (*PDFResult, error) {
	if e.registry == nil {
		return nil, fmt.Errorf("RenderPDF: registry not configured")
	}
	withLinks, err := e.GetEventByID(eventID)
	if err != nil {
		return nil, err
	}
	event := withLinks.Event
	linkedRows, err := e.ListForEvent(eventID)
	if err != nil {
		return nil, fmt.Errorf("RenderPDF list linked for %d: %w", eventID, err)
	}
	linkedDicts := make([]map[string]any, 0, len(linkedRows))
	for _, p := range linkedRows {
		name := strings.TrimSpace(p.FirstName + " " + p.MiddleName + " " + p.LastName)
		if name == "" {
			name = strings.TrimSpace(p.DisplayID)
		}
		range_ := ""
		if year := strings.TrimSpace(strings.SplitN(p.BirthDate, "/", 3)[2]); year != "" {
			range_ = year + " —"
		}
		if year := strings.TrimSpace(strings.SplitN(p.DeathDate, "/", 3)[2]); year != "" {
			if range_ != "" {
				range_ = strings.TrimSuffix(range_, " —") + " — " + year
			} else {
				range_ = "— " + year
			}
		}
		linkedDicts = append(linkedDicts, map[string]any{
			"display_id": strings.TrimSpace(p.DisplayID),
			"name":       name,
			"range":      range_,
		})
	}
	opts := eventRenderPDFOptions{
		Orientation:     normalizeOrientation(orientation),
		PrinterFriendly: true,
		IncludeImages:   false,
	}
	data := map[string]any{
		"soldier":  event,
		"linked":   linkedDicts,
		"options":  opts,
		"branding": map[string]string{},
	}
	var buf bytes.Buffer
	if err := e.registry.RenderEvent(context.Background(), "event", normalizeOrientation(orientation), data, &buf); err != nil {
		return nil, fmt.Errorf("RenderPDF %d: %w", eventID, err)
	}
	return &PDFResult{
		Bytes:    buf.Bytes(),
		Filename: slugifyEventFilename(event, orientation),
	}, nil
}

// eventRenderPDFOptions is the small subset of PDFOptions the
// event template's data.json needs. Local type to avoid
// importing pkg/render (cycle: pkg/render -> internal/records).
type eventRenderPDFOptions struct {
	Orientation     string `json:"orientation"`
	PrinterFriendly bool   `json:"printerFriendly"`
	IncludeImages   bool   `json:"includeImages"`
}

// slugifyEventFilename builds the suggested filename:
// "Event-EVT-NNNNN-<kind-slug>-<orientation>.pdf". Mirrors
// slugifyArticleFilename so the dialog default reads naturally
// for both Event and Article exports.
func slugifyEventFilename(event models.Soldier, orientation string) string {
	short := "landscape"
	if normalizeOrientation(orientation) == "P" {
		short = "portrait"
	}
	slug := slugify(strings.TrimSpace(event.Kind))
	if slug == "" {
		return fmt.Sprintf("Event-%s-%s.pdf", event.DisplayID, short)
	}
	return fmt.Sprintf("Event-%s-%s-%s.pdf", event.DisplayID, slug, short)
}

// MoveEventSource reorders an Event Source within its owning
// Event. Mirrors MoveRecordWithinPerson but writes to the
// event_sources table. The person_record_id check (ownerID ==
// eventID) uses the event_id column on event_sources.
//
// Issue #368 slice 2.
func (e *EventService) MoveEventSource(eventID, sourceID, position int64) error {
	if eventID < 1 || sourceID < 1 {
		return fmt.Errorf("event id and source id must be positive")
	}
	conn := e.soldiers.db.Conn()
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ownerID int64
	if err := tx.QueryRow(
		`SELECT event_id FROM event_sources WHERE id = ?`,
		sourceID,
	).Scan(&ownerID); err != nil {
		return err
	}
	if ownerID != eventID {
		return fmt.Errorf("source %d is not attached to event %d", sourceID, eventID)
	}

	var n int64
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM event_sources WHERE event_id = ?`,
		eventID,
	).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no event sources to reorder for event %d", eventID)
	}
	if position < 1 {
		position = 1
	}
	if position > n {
		position = n
	}

	var currentOrder int64
	if err := tx.QueryRow(
		`SELECT sort_order FROM event_sources WHERE id = ?`,
		sourceID,
	).Scan(&currentOrder); err != nil {
		return err
	}

	if currentOrder < position {
		if _, err := tx.Exec(
			`UPDATE event_sources SET sort_order = sort_order - 1 WHERE event_id = ? AND id <> ? AND sort_order > ? AND sort_order <= ?`,
			eventID, sourceID, currentOrder, position,
		); err != nil {
			return err
		}
	} else if currentOrder > position {
		if _, err := tx.Exec(
			`UPDATE event_sources SET sort_order = sort_order + 1 WHERE event_id = ? AND id <> ? AND sort_order >= ? AND sort_order < ?`,
			eventID, sourceID, position, currentOrder,
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(
		`UPDATE event_sources SET sort_order = ? WHERE id = ?`,
		position, sourceID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// LinkedEventTimelineMarker is the slim projection of an Event
// Record suitable for inclusion on a Person Record's Service
// Timeline (issue #320 slice #337). It carries only the fields
// the timeline builder reads so the query stays narrow and the
// builder can mint a ServiceTimelineEvent without an extra
// GetByID round trip per Event.
//
// Moved here from SoldierService as part of issue #343 finding #5 —
// the JOIN against event_person_links belongs on EventService
// (which already owns the Event-side schema), not on SoldierService
// (which otherwise doesn't touch the event_person_links table).
// SoldierService.ServiceTimeline consumes this projection via the
// back-reference set by SoldierService.SetEvents.
type LinkedEventTimelineMarker struct {
	Kind        string // Event kind (free-text: "Battle", "Hospital Stay", ...)
	BeginDate   string // canonical MM[/DD]/YYYY; falls back to EndDate
	EndDate     string // canonical MM[/DD]/YYYY; used only if BeginDate is empty
	Description string // long-form Event description; surfaced as the marker description
	DisplayID   string // EVT-NNNNN; surfaced as the marker source label
}

// LinkedEventsForTimeline returns the Event Records linked to
// the given person via event_person_links, projected onto
// LinkedEventTimelineMarker so ServiceTimeline can mint one
// Timeline Marker per Event without an extra GetByID round
// trip. The query is index-friendly: event_person_links has
// UNIQUE (event_id, person_id) so the join hits the existing
// index, and the WHERE clause filters by the indexed person_id
// side.
//
// Returns an empty slice (not nil) when no Events are linked.
// Returns an error only on query failure; per-row scan errors
// propagate. Dates are returned as the raw TEXT they were stored
// as on the soldiers row; ServiceTimeline parses them through
// dates.ParseCanonical.
func (e *EventService) LinkedEventsForTimeline(personID int64) ([]LinkedEventTimelineMarker, error) {
	if personID < 1 {
		return nil, fmt.Errorf("LinkedEventsForTimeline: person id must be positive")
	}
	rows, err := e.soldiers.db.Conn().Query(
		`SELECT s.kind, s.begin_date, s.end_date, s.description, s.display_id
		 FROM soldiers s
		 JOIN event_person_links epl ON epl.event_id = s.id
		 WHERE epl.person_id = ?`,
		personID,
	)
	if err != nil {
		return nil, fmt.Errorf("LinkedEventsForTimeline query: %w", err)
	}
	defer debug.DeferCloseLog(rows, "LinkedEventsForTimeline.rows")

	markers := make([]LinkedEventTimelineMarker, 0)
	for rows.Next() {
		var m LinkedEventTimelineMarker
		if err := rows.Scan(&m.Kind, &m.BeginDate, &m.EndDate, &m.Description, &m.DisplayID); err != nil {
			return nil, fmt.Errorf("LinkedEventsForTimeline scan: %w", err)
		}
		markers = append(markers, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("LinkedEventsForTimeline rows: %w", err)
	}
	return markers, nil
}
