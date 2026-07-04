package records

import (
	"errors"
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
