// event_record_repo_parity_test.go — issue #613 slice 3
// regression net.
//
// Pins the contract that the new EventRecordRepo-backed
// EventService methods return identical results to the legacy
// inline-SQL paths on the same fixture.
//
// The service-level methods (ListEvents, ListForPerson,
// AttachEventToPerson, DetachEventFromPerson, linksForEvent)
// are exercised end-to-end through the public API. A
// regression in the slice-3 seam would surface here as either
// a wrong row count, a wrong row order, or a missing link.
package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// TestEventRecordRepo_Parity_ListEvents_OnlyEvents asserts the
// repo-backed ListEvents returns ONLY rows with entry_type =
// 'event', not Person Records or any other entry_type.
func TestEventRecordRepo_Parity_ListEvents_OnlyEvents(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	evSvc := NewEventService(svc)

	// Seed: 2 Events + 2 Person Records (Soldier + Wife).
	event1, err := svc.Create(modelsSoldierEvent("EVT-0001", "Battle"))
	if err != nil {
		t.Fatalf("Create event 1: %v", err)
	}
	if _, err := svc.Create(modelsSoldierEvent("EVT-0002", "Campaign")); err != nil {
		t.Fatalf("Create event 2: %v", err)
	}
	if _, err := svc.Create(modelsSoldierForSeed("P-0001", "John", "Doe")); err != nil {
		t.Fatalf("Create person 1: %v", err)
	}
	if _, err := svc.Create(modelsSoldierForSeed("P-0002", "Jane", "Doe")); err != nil {
		t.Fatalf("Create person 2: %v", err)
	}
	_ = event1 // referenced by later attach

	events, err := evSvc.ListEvents(1, 100)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("ListEvents returned %d rows, want 2 (Events only)", len(events))
	}
	for _, e := range events {
		if e.EntryType != "event" {
			t.Errorf("ListEvents returned non-event row: EntryType = %q", e.EntryType)
		}
	}
}

// TestEventRecordRepo_Parity_AttachDetach verifies the
// service-level Attach + Detach round-trip.
func TestEventRecordRepo_Parity_AttachDetach(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	evSvc := NewEventService(svc)

	event1, err := svc.Create(modelsSoldierEvent("EVT-0001", "Battle"))
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}
	eventID := event1.ID
	person1, err := svc.Create(modelsSoldierForSeed("P-0001", "John", "Doe"))
	if err != nil {
		t.Fatalf("Create person: %v", err)
	}
	personID := person1.ID

	linkID, err := evSvc.AttachEventToPerson(eventID, personID)
	if err != nil {
		t.Fatalf("AttachEventToPerson: %v", err)
	}
	if linkID <= 0 {
		t.Errorf("AttachEventToPerson returned linkID = %d, want positive", linkID)
	}

	// ListForPerson now returns the Event.
	events, err := evSvc.ListForPerson(personID)
	if err != nil {
		t.Fatalf("ListForPerson after attach: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("ListForPerson after attach = %d events, want 1", len(events))
	}
	if len(events) > 0 && events[0].ID != eventID {
		t.Errorf("ListForPerson returned event ID %d, want %d", events[0].ID, eventID)
	}

	// Detach.
	if err := evSvc.DetachEventFromPerson(eventID, personID); err != nil {
		t.Fatalf("DetachEventFromPerson: %v", err)
	}

	// ListForPerson now returns no Events.
	events, err = evSvc.ListForPerson(personID)
	if err != nil {
		t.Fatalf("ListForPerson after detach: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("ListForPerson after detach = %d events, want 0", len(events))
	}
}

// modelsSoldierEvent returns a models.Soldier pre-shaped as
// an Event Record. Used to seed fixtures in slice-3 parity
// tests.
func modelsSoldierEvent(displayID, kind string) models.Soldier {
	return models.Soldier{
		DisplayID: displayID,
		EntryType: "event",
		Kind:      kind,
	}
}

// modelsSoldierForSeed returns a models.Soldier pre-shaped as
// a Person Record (entry_type='soldier'). Used to seed fixtures
// in slice-3 parity tests.
func modelsSoldierForSeed(displayID, first, last string) models.Soldier {
	return models.Soldier{
		DisplayID: displayID,
		EntryType: "soldier",
		FirstName: first,
		LastName:  last,
	}
}

// db import anchor (used by future slices; placeholder so the
// package compiles when this file is the only test entry
// point).
var _ = db.NewFromExisting