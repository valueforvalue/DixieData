package records

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestEventService_CreateEvent (issue #320) verifies the basic
// create flow: empty Display ID gets an EVT-NNNNN minted; entry
// type is forced to "event" even if the caller passes something
// else; person-specific fields are cleared.
func TestEventService_CreateEvent(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{
		Kind:        "Battle",
		BeginDate:   "07/01/1863",
		EndDate:     "07/03/1863",
		Description: "Three days of fighting in southern Pennsylvania.",
		// Caller passes Soldier fields; service should clear them.
		FirstName: "should be cleared",
		LastName:  "should be cleared",
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if ev.DisplayID != "EVT-00001" {
		t.Errorf("DisplayID = %q, want EVT-00001", ev.DisplayID)
	}
	if ev.EntryType != models.EntryTypeEvent {
		t.Errorf("EntryType = %q, want %q", ev.EntryType, models.EntryTypeEvent)
	}
	if ev.Kind != "Battle" {
		t.Errorf("Kind = %q, want Battle", ev.Kind)
	}
	if ev.BeginDate != "07/01/1863" {
		t.Errorf("BeginDate = %q", ev.BeginDate)
	}
	if ev.EndDate != "07/03/1863" {
		t.Errorf("EndDate = %q", ev.EndDate)
	}
	if ev.FirstName != "" || ev.LastName != "" {
		t.Errorf("Person-specific fields not cleared: FirstName=%q LastName=%q", ev.FirstName, ev.LastName)
	}
	if ev.SpouseSoldierID != 0 {
		t.Errorf("SpouseSoldierID = %d, want 0", ev.SpouseSoldierID)
	}
}

// TestEventService_UpdateEvent preserves the immutable columns
// (Display ID, Sync ID, Entry Type) and the person-specific
// fields (cleared on update too).
func TestEventService_UpdateEvent(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{
		Kind: "Battle",
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	ev.Kind = "Engagement"
	ev.Description = "Updated."
	ev.FirstName = "should be cleared on update"
	if err := eventSvc.UpdateEvent(*ev); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	got, err := eventSvc.GetEventByID(ev.ID)
	if err != nil {
		t.Fatalf("GetEventByID: %v", err)
	}
	if got.Event.DisplayID != "EVT-00001" {
		t.Errorf("DisplayID changed: %q", got.Event.DisplayID)
	}
	if got.Event.EntryType != models.EntryTypeEvent {
		t.Errorf("EntryType changed: %q", got.Event.EntryType)
	}
	if got.Event.Kind != "Engagement" {
		t.Errorf("Kind = %q", got.Event.Kind)
	}
	if got.Event.Description != "Updated." {
		t.Errorf("Description = %q", got.Event.Description)
	}
	if got.Event.FirstName != "" {
		t.Errorf("FirstName not cleared on update: %q", got.Event.FirstName)
	}
}

// TestEventService_GetEventByDisplayID covers the case-insensitive
// display-id lookup path.
func TestEventService_GetEventByDisplayID(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	_, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	got, err := eventSvc.GetEventByDisplayID("evt-00001")
	if err != nil {
		t.Fatalf("GetEventByDisplayID: %v", err)
	}
	if got.Event.DisplayID != "EVT-00001" {
		t.Errorf("DisplayID = %q", got.Event.DisplayID)
	}
}

// TestEventService_UpdateEventOnSoldierRejects confirms that
// updating a non-Event Record via the event service returns an
// error (defense in depth; handlers should not invoke this).
func TestEventService_UpdateEventOnSoldierRejects(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	s, err := soldierSvc.Create(models.Soldier{
		FirstName: "Robert", LastName: "Lee",
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	s.Kind = "Battle"
	err = eventSvc.UpdateEvent(*s)
	if err == nil {
		t.Fatal("expected error updating a non-Event record via eventSvc")
	}
	if !strings.Contains(err.Error(), "not an Event") {
		t.Errorf("error message = %q, want 'not an Event'", err.Error())
	}
}

// TestEventService_AttachDetachPerson verifies the M-to-M link
// flow: attach creates a link, the duplicate attach returns
// ErrDuplicateLink, detach removes it, and the Person appears
// in ListForEvent + the Event appears in ListForPerson.
func TestEventService_AttachDetachPerson(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	p1, err := soldierSvc.Create(models.Soldier{FirstName: "Robert", LastName: "Lee"})
	if err != nil {
		t.Fatalf("Create person 1: %v", err)
	}
	p2, err := soldierSvc.Create(models.Soldier{FirstName: "Stonewall", LastName: "Jackson"})
	if err != nil {
		t.Fatalf("Create person 2: %v", err)
	}

	if _, err := eventSvc.AttachEventToPerson(ev.ID, p1.ID); err != nil {
		t.Fatalf("Attach 1: %v", err)
	}
	if _, err := eventSvc.AttachEventToPerson(ev.ID, p2.ID); err != nil {
		t.Fatalf("Attach 2: %v", err)
	}

	// Duplicate attach returns ErrDuplicateLink.
	_, err = eventSvc.AttachEventToPerson(ev.ID, p1.ID)
	if !errors.Is(err, ErrDuplicateLink) {
		t.Errorf("duplicate attach: err = %v, want ErrDuplicateLink", err)
	}

	// LinkCount
	if n, err := eventSvc.LinkCount(ev.ID); err != nil {
		t.Fatalf("LinkCount: %v", err)
	} else if n != 2 {
		t.Errorf("LinkCount = %d, want 2", n)
	}

	// ListForEvent returns both persons.
	linked, err := eventSvc.ListForEvent(ev.ID)
	if err != nil {
		t.Fatalf("ListForEvent: %v", err)
	}
	if len(linked) != 2 {
		t.Errorf("ListForEvent len = %d, want 2", len(linked))
	}

	// ListForPerson returns the event.
	forPerson, err := eventSvc.ListForPerson(p1.ID)
	if err != nil {
		t.Fatalf("ListForPerson: %v", err)
	}
	if len(forPerson) != 1 || forPerson[0].ID != ev.ID {
		t.Errorf("ListForPerson = %v, want 1 event with ID %d", forPerson, ev.ID)
	}

	// Detach one; LinkCount drops to 1.
	if err := eventSvc.DetachEventFromPerson(ev.ID, p1.ID); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if n, _ := eventSvc.LinkCount(ev.ID); n != 1 {
		t.Errorf("LinkCount after detach = %d, want 1", n)
	}

	// Detach is idempotent.
	if err := eventSvc.DetachEventFromPerson(ev.ID, p1.ID); err != nil {
		t.Errorf("Detach idempotent: %v", err)
	}
}

// TestEventService_AttachPersonToEventRejected covers defense in
// depth: trying to attach a non-Event as the event_id returns an
// error; trying to attach an Event as the person_id returns an
// error.
func TestEventService_AttachPersonToEventRejected(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	s, err := soldierSvc.Create(models.Soldier{FirstName: "R", LastName: "L"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	ev2, err := eventSvc.CreateEvent(models.Soldier{Kind: "Siege"})
	if err != nil {
		t.Fatalf("CreateEvent 2: %v", err)
	}

	// person_id cannot be an Event.
	if _, err := eventSvc.AttachEventToPerson(ev.ID, ev2.ID); err == nil {
		t.Error("expected error attaching Event as person_id")
	}
	// event_id cannot be a non-Event.
	if _, err := eventSvc.AttachEventToPerson(s.ID, s.ID); err == nil {
		t.Error("expected error attaching non-Event as event_id")
	}
}

// TestEventService_NextIDMints verifies the EVT- counter is
// independent of the DXD- counter.
func TestEventService_NextIDMints(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	// Pre-populate DXD- rows.
	for i := 0; i < 3; i++ {
		_, err := soldierSvc.Create(models.Soldier{FirstName: "X", LastName: "Y"})
		if err != nil {
			t.Fatalf("Create soldier %d: %v", i, err)
		}
	}

	ev, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if ev.DisplayID != "EVT-00001" {
		t.Errorf("DisplayID = %q, want EVT-00001 (DXD- rows should not affect EVT- counter)", ev.DisplayID)
	}
}

// TestEventService_ListEvents verifies the list + pagination
// surface, and that Events do not bleed into the Person Record
// list.
func TestEventService_ListEvents(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	for i := 0; i < 5; i++ {
		_, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
		if err != nil {
			t.Fatalf("CreateEvent %d: %v", i, err)
		}
	}
	for i := 0; i < 3; i++ {
		_, err := soldierSvc.Create(models.Soldier{FirstName: "P", LastName: "X"})
		if err != nil {
			t.Fatalf("Create soldier %d: %v", i, err)
		}
	}

	events, err := eventSvc.ListEvents(1, 10)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("ListEvents len = %d, want 5", len(events))
	}
	for _, e := range events {
		if e.EntryType != models.EntryTypeEvent {
			t.Errorf("non-Event in list: ID=%d EntryType=%q", e.ID, e.EntryType)
		}
	}

	// Page 1 with pageSize 2 returns 2 events; page 3 returns 1.
	page1, _ := eventSvc.ListEvents(1, 2)
	if len(page1) != 2 {
		t.Errorf("page 1 len = %d, want 2", len(page1))
	}
	page3, _ := eventSvc.ListEvents(3, 2)
	if len(page3) != 1 {
		t.Errorf("page 3 len = %d, want 1", len(page3))
	}
}

// TestEventService_UpdateEventPreservesAttachedSources is the
// regression test for issue #340 (the data-loss bug discovered
// 2026-07-04). v60 slot #329 reused the shared `records` table
// for per-Event sources, but SoldierService.Update calls
// replaceRecords which DELETEs every row where
// person_record_id = soldierID and re-inserts from
// soldier.Records. The Event edit form has no records field,
// so parseEventForm returned an Event with Records: nil, and
// every Event Update wiped every attached source. v61 moves
// Event sources to a dedicated event_sources table, which
// replaceRecords never touches.
//
// This test would FAIL on v60 (sources wiped: want 1, got 0)
// and PASS on v61+. See docs/agents/notes/v61-bug-repro.md.
func TestEventService_UpdateEventPreservesAttachedSources(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	_, err = eventSvc.AttachSourceToEvent(ev.ID, models.Record{
		RecordType: "Pension Application",
		AppID:      "APP-1880-7701",
		Details:    "Filed 1880, Co. B, 4th VA Infantry",
	})
	if err != nil {
		t.Fatalf("AttachSourceToEvent: %v", err)
	}

	// Sanity: source is attached immediately after create.
	if got := sourcesLen(t, eventSvc, ev.ID); got != 1 {
		t.Fatalf("post-attach sources = %d, want 1", got)
	}

	// The bug-trigger: any non-trivial Update used to wipe
	// sources via replaceRecords. With v61 the new table is
	// outside replaceRecords' DELETE scope.
	ev.Kind = "Engagement"
	ev.Description = "Updated description; sources must survive."
	if err := eventSvc.UpdateEvent(*ev); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	if got := sourcesLen(t, eventSvc, ev.ID); got != 1 {
		t.Errorf("sources wiped on Update: want 1, got %d (issue #340 regression)", got)
	}

	// Also verify the source is still functionally attached
	// (detach still works, count drops by 1).
	all, err := eventSvc.ListSourcesForEvent(ev.ID)
	if err != nil {
		t.Fatalf("ListSourcesForEvent: %v", err)
	}
	if len(all) != 1 || all[0].AppID != "APP-1880-7701" {
		t.Errorf("surviving source shape: want 1 source with AppID=APP-1880-7701, got %+v", all)
	}
}

// TestEventService_SourceRoundTripOnEventSourcesTable pins
// the new SQL contract: List returns rows in id order; Attach
// mints sync_id when empty; Detach refuses to drop a row
// belonging to a different Event.
func TestEventService_SourceRoundTripOnEventSourcesTable(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	id1, err := eventSvc.AttachSourceToEvent(ev.ID, models.Record{
		RecordType: "Pension",
		AppID:      "APP-1",
		Details:    "first",
	})
	if err != nil {
		t.Fatalf("AttachSourceToEvent 1: %v", err)
	}
	if id1 == 0 {
		t.Errorf("AttachSourceToEvent returned id=0; want positive")
	}

	// Attach with an explicit sync_id (simulates a distributed-
	// merge re-attach).
	explicitSyncID := "explicit-sync-id-test"
	id2, err := eventSvc.AttachSourceToEvent(ev.ID, models.Record{
		SyncID:     explicitSyncID,
		RecordType: "Roster",
		AppID:      "APP-2",
		Details:    "second",
	})
	if err != nil {
		t.Fatalf("AttachSourceToEvent 2: %v", err)
	}

	list, err := eventSvc.ListSourcesForEvent(ev.ID)
	if err != nil {
		t.Fatalf("ListSourcesForEvent: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
	if list[0].ID != id1 || list[1].ID != id2 {
		t.Errorf("list order = [%d, %d], want [%d, %d]", list[0].ID, list[1].ID, id1, id2)
	}
	if list[1].SyncID != explicitSyncID {
		t.Errorf("explicit sync_id not preserved: got %q, want %q", list[1].SyncID, explicitSyncID)
	}

	// Detach with wrong event id must be rejected.
	if err := eventSvc.DetachSourceFromEvent(ev.ID+1, id1); err == nil {
		t.Errorf("detach with wrong event id succeeded; want error")
	}
	if got := sourcesLen(t, eventSvc, ev.ID); got != 2 {
		t.Errorf("cross-event detach leaked: list len = %d, want 2", got)
	}

	// Real detach drops the row.
	if err := eventSvc.DetachSourceFromEvent(ev.ID, id1); err != nil {
		t.Fatalf("DetachSourceFromEvent: %v", err)
	}
	if got := sourcesLen(t, eventSvc, ev.ID); got != 1 {
		t.Errorf("post-detach list len = %d, want 1", got)
	}
}

func sourcesLen(t *testing.T, eventSvc *EventService, eventID int64) int {
	t.Helper()
	rows, err := eventSvc.ListSourcesForEvent(eventID)
	if err != nil {
		t.Fatalf("ListSourcesForEvent: %v", err)
	}
	return len(rows)
}

// TestEventService_GetEventByIDReturnsEventSourcesField pins the
// read-side projection for slice 4 of the v61 decomposition.
// GetEventByID must populate row.EventSources from the dedicated
// event_sources table so the view-model layer can render the
// per-Event Sources panel without a second SELECT.
func TestEventService_GetEventByIDReturnsEventSourcesField(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	_, err = eventSvc.AttachSourceToEvent(ev.ID, models.Record{
		RecordType: "Pension Application",
		AppID:      "APP-1880-7701",
		Details:    "Filed 1880",
	})
	if err != nil {
		t.Fatalf("AttachSourceToEvent: %v", err)
	}

	got, err := eventSvc.GetEventByID(ev.ID)
	if err != nil {
		t.Fatalf("GetEventByID: %v", err)
	}
	if len(got.Event.EventSources) != 1 {
		t.Fatalf("EventSources len = %d, want 1", len(got.Event.EventSources))
	}
	if got.Event.EventSources[0].AppID != "APP-1880-7701" {
		t.Errorf("EventSources[0].AppID = %q, want APP-1880-7701", got.Event.EventSources[0].AppID)
	}
	// Records must stay empty — the v60 records-table reuse is
	// removed; row.Records belongs to Person Records only.
	if len(got.Event.Records) != 0 {
		t.Errorf("Event.Records len = %d, want 0 (Event sources moved off records table)", len(got.Event.Records))
	}
}

// TestEventService_AttachSourcesToEvent pins the batch attach path
// the Event create + edit forms use to save inline Source Record
// rows in one transaction. Issue #357: the form must be able to
// save sources on the same write as the Event itself (mirrors the
// soldier entry form's Records[] pattern).
//
// RED today: AttachSourcesToEvent does not exist on EventService,
// so this test fails to compile. The compile failure IS the RED
// signal — once the method lands with the contract below, the
// test turns GREEN and pins the slice's acceptance criterion.
func TestEventService_AttachSourcesToEvent(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	sources := []models.Record{
		{RecordType: "Pension Application", AppID: "APP-1880-7701", Details: "Filed 1880, Co. B"},
		{RecordType: "Roster", AppID: "CO-B-4TH-VA", Details: "4th VA Infantry roster"},
		{RecordType: "", AppID: "", Details: ""}, // empty row — should be skipped
	}
	ids, err := eventSvc.AttachSourcesToEvent(ev.ID, sources)
	if err != nil {
		t.Fatalf("AttachSourcesToEvent: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("AttachSourcesToEvent returned %d ids, want 2 (empty row skipped)", len(ids))
	}

	got, err := eventSvc.ListSourcesForEvent(ev.ID)
	if err != nil {
		t.Fatalf("ListSourcesForEvent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("post-attach sources = %d, want 2", len(got))
	}
	if got[0].AppID != "APP-1880-7701" {
		t.Errorf("got[0].AppID = %q, want APP-1880-7701", got[0].AppID)
	}
	if got[1].RecordType != "Roster" {
		t.Errorf("got[1].RecordType = %q, want Roster", got[1].RecordType)
	}
}

// TestEventService_LookupPersonIDByDisplayID pins slice 2 of
// #361: the Event-side attach handler resolves a Person Record
// Display ID (e.g. 'SOL-00042') to its numeric ID so the
// editor's Add Linked Person form can post a Display ID string
// instead of forcing the user to know raw row IDs.
//
// Pre-slice, this method doesn't exist; the test fails to
// compile against the missing symbol, which is the right kind
// of RED (a missing public seam on the service). After the
// slice lands, the test exercises:
//   - happy path: existing Person Record by Display ID
//   - case-insensitive lookup (mirrors SoldierService.GetByDisplayID)
//   - whitespace trimming
//   - missing Display ID returns os.ErrNotExist (sentinel the
//     handler maps to HTTP 404 / 400)
//   - empty Display ID returns os.ErrNotExist (matches the
//     GetByDisplayID contract the helper delegates to)
func TestEventService_LookupPersonIDByDisplayID(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	// Seed two Person Records (SoldierService.Create mints
	// Display IDs like SOL-00001, SOL-00002).
	p1, err := soldierSvc.Create(models.Soldier{FirstName: "Robert", LastName: "Lee"})
	if err != nil {
		t.Fatalf("Create p1: %v", err)
	}
	p2, err := soldierSvc.Create(models.Soldier{FirstName: "Stonewall", LastName: "Jackson"})
	if err != nil {
		t.Fatalf("Create p2: %v", err)
	}

	cases := []struct {
		name      string
		displayID string
		wantID    int64
		wantErr   error
	}{
		{"exact-p1", p1.DisplayID, p1.ID, nil},
		{"exact-p2", p2.DisplayID, p2.ID, nil},
		{"lower-p1", strings.ToLower(p1.DisplayID), p1.ID, nil},
		{"upper-p1", strings.ToUpper(p1.DisplayID), p1.ID, nil},
		{"trimmed-p1", "  " + p1.DisplayID + "  ", p1.ID, nil},
		{"missing", "SOL-99999", 0, os.ErrNotExist},
		{"empty", "", 0, os.ErrNotExist},
		{"whitespace-only", "   ", 0, os.ErrNotExist},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotID, err := eventSvc.LookupPersonIDByDisplayID(tc.displayID)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("err = %v, want errors.Is(%v)", err, tc.wantErr)
				}
				if gotID != 0 {
					t.Errorf("gotID = %d on error, want 0", gotID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if gotID != tc.wantID {
				t.Errorf("gotID = %d, want %d", gotID, tc.wantID)
			}
		})
	}
}

// TestEventService_LookupPersonIDByDisplayID_NilService confirms
// the helper guards against a nil receiver (defense in depth —
// SoldierService.GetByDisplayID would panic on nil; the
// helper should return a clean error). Cheap test, catches a
// real footgun if Slice 2 ever swaps the seam.
func TestEventService_LookupPersonIDByDisplayID_NilService(t *testing.T) {
	var svc *EventService
	_, err := svc.LookupPersonIDByDisplayID("SOL-00001")
	if err == nil {
		t.Errorf("nil receiver should return an error, got nil")
	}
}

// TestEventService_LookupPersonIDByName (issue #373) pins the
// name-search fallback the Event editor's Add Linked Person
// form uses when the user's input is not a Display ID. The
// helper does a case-insensitive substring match against the
// concatenated first/middle/last/suffix name fields, sorts
// matches by display_id, and returns the FIRST row. Empty
// input returns os.ErrNotExist (same sentinel as
// LookupPersonIDByDisplayID) — the helper must NOT match
// every row when the user submits a blank field.
func TestEventService_LookupPersonIDByName(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	// Seed three Person Records. Display IDs are DXD-00001,
	// DXD-00002, DXD-00003 in insertion order, so alphabetical
	// order on display_id matches insertion order here.
	p1, err := soldierSvc.Create(models.Soldier{FirstName: "Robert", MiddleName: "E.", LastName: "Lee", Suffix: "Jr."})
	if err != nil {
		t.Fatalf("Create p1: %v", err)
	}
	p2, err := soldierSvc.Create(models.Soldier{FirstName: "Stonewall", LastName: "Jackson"})
	if err != nil {
		t.Fatalf("Create p2: %v", err)
	}
	p3, err := soldierSvc.Create(models.Soldier{FirstName: "James", MiddleName: "Robert", LastName: "Lee"})
	if err != nil {
		t.Fatalf("Create p3: %v", err)
	}

	cases := []struct {
		name    string
		input   string
		wantID  int64
		wantErr error
	}{
		// Single match (unique last name).
		{"single-last-name", "Jackson", p2.ID, nil},
		// Multiple matches on "Lee" — sorted by display_id, first wins.
		{"multi-last-name-first-wins", "Lee", p1.ID, nil},
		// Substring on first name (case-insensitive).
		{"first-name-lower", "stonewall", p2.ID, nil},
		{"first-name-upper", "STONEWALL", p2.ID, nil},
		// Substring spanning middle + last ("Robert Lee" matches p3;
		// "Lee" alone also matched p1 first).
		{"middle-last-substring", "Robert Lee", p3.ID, nil},
		// Empty string is rejected — must NOT match every row.
		{"empty", "", 0, os.ErrNotExist},
		{"whitespace-only", "   ", 0, os.ErrNotExist},
		// No match.
		{"no-match", "Nonexistent", 0, os.ErrNotExist},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotID, err := eventSvc.LookupPersonIDByName(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("err = %v, want errors.Is(%v)", err, tc.wantErr)
				}
				if gotID != 0 {
					t.Errorf("gotID = %d on error, want 0", gotID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if gotID != tc.wantID {
				t.Errorf("gotID = %d, want %d", gotID, tc.wantID)
			}
		})
	}
}

// TestEventService_LookupPersonIDByName_NilService confirms the
// nil-receiver guard from LookupPersonIDByDisplayID carries
// over to the name path (same defensive pattern; would panic
// on a nil SoldierService.db handle otherwise).
func TestEventService_LookupPersonIDByName_NilService(t *testing.T) {
	var svc *EventService
	_, err := svc.LookupPersonIDByName("Lee")
	if err == nil {
		t.Errorf("nil receiver should return an error, got nil")
	}
}
