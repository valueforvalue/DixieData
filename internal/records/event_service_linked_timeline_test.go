// Characterization tests for EventService.LinkedEventsForTimeline
// (issue #343 finding #5). Before the slice, this query lived
// on SoldierService (private method `linkedEventsForTimeline`)
// — duplicating the soldiers/event_person_links JOIN knowledge
// in a service that otherwise doesn't touch the Event-side
// schema. After the slice, it lives on EventService where the
// JOIN belongs, and the type is exported so the timeline
// builder on SoldierService can consume it without crossing
// package boundaries.
//
// The 3 tests below pin the move + the behavior:
//   1. TestEventService_LinkedEventsForTimelineReturnsMarkers
//      - creates a soldier + an event + a link, calls the
//        EventService method directly, asserts the marker
//        shape (kind / begin / end / description / display_id).
//   2. TestEventService_LinkedEventsForTimelineEmptyWhenNoLinks
//      - soldier with no linked events returns empty (not nil).
//   3. TestSoldierService_ServiceTimelineUsesEventServiceForLinkedMarkers
//      - the existing SoldierService.ServiceTimeline call
//        path still surfaces linked-event markers after the
//        back-reference is wired. Pins the integration
//        contract so a future refactor that breaks the wiring
//        goes red.
package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestEventService_LinkedEventsForTimelineReturnsMarkers(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)
	// Wire the back-reference so SoldierService can resolve
	// EventService for the linked-events query. The slice
	// adds this setter; the production wiring in app.go and
	// the existing tests will adopt it in a follow-up.
	svc.SetEvents(events)

	soldier, err := svc.Create(models.Soldier{
		DisplayID: "TLM-NEW-1",
		FirstName: "William",
		LastName:  "Walker",
		Unit:      "5th Alabama",
		BirthDate: "07/04/1840",
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	event, err := events.CreateEvent(models.Soldier{
		Kind:        "Battle of Antietam",
		BeginDate:   "09/17/1862",
		EndDate:     "09/17/1862",
		Description: "Single-day engagement.",
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if _, err := events.AttachEventToPerson(event.ID, soldier.ID); err != nil {
		t.Fatalf("AttachEventToPerson: %v", err)
	}

	markers, err := events.LinkedEventsForTimeline(soldier.ID)
	if err != nil {
		t.Fatalf("LinkedEventsForTimeline: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("LinkedEventsForTimeline len = %d, want 1; markers=%#v", len(markers), markers)
	}
	got := markers[0]
	if got.Kind != "Battle of Antietam" {
		t.Errorf("marker.Kind = %q, want Battle of Antietam", got.Kind)
	}
	if got.BeginDate != "09/17/1862" {
		t.Errorf("marker.BeginDate = %q, want 09/17/1862", got.BeginDate)
	}
	if got.Description != "Single-day engagement." {
		t.Errorf("marker.Description = %q, want Single-day engagement.", got.Description)
	}
	if got.DisplayID != event.DisplayID {
		t.Errorf("marker.DisplayID = %q, want %q", got.DisplayID, event.DisplayID)
	}
}

func TestEventService_LinkedEventsForTimelineEmptyWhenNoLinks(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)
	svc.SetEvents(events)

	soldier, err := svc.Create(models.Soldier{
		DisplayID: "TLM-NEW-2",
		FirstName: "Empty",
		LastName:  "Timeline",
		BirthDate: "01/01/1840",
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	markers, err := events.LinkedEventsForTimeline(soldier.ID)
	if err != nil {
		t.Fatalf("LinkedEventsForTimeline: %v", err)
	}
	if markers == nil {
		t.Errorf("LinkedEventsForTimeline returned nil; want empty slice")
	}
	if len(markers) != 0 {
		t.Errorf("LinkedEventsForTimeline len = %d, want 0; markers=%#v", len(markers), markers)
	}
}

// TestSoldierService_ServiceTimelineUsesEventServiceForLinkedMarkers
// is the integration pin: the existing SoldierService.ServiceTimeline
// must still surface linked-event markers after the query is
// delegated to EventService. This is the regression net for
// the back-reference wiring.
func TestSoldierService_ServiceTimelineUsesEventServiceForLinkedMarkers(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)
	svc.SetEvents(events)

	soldier, err := svc.Create(models.Soldier{
		DisplayID: "TLM-NEW-3",
		FirstName: "William",
		LastName:  "Walker",
		Unit:      "5th Alabama",
		BirthDate: "07/04/1840",
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	event, err := events.CreateEvent(models.Soldier{
		Kind:      "Battle of Gettysburg",
		BeginDate: "07/01/1863",
		EndDate:   "07/03/1863",
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if _, err := events.AttachEventToPerson(event.ID, soldier.ID); err != nil {
		t.Fatalf("AttachEventToPerson: %v", err)
	}

	timeline, err := svc.ServiceTimeline(soldier.ID)
	if err != nil {
		t.Fatalf("ServiceTimeline: %v", err)
	}
	var found bool
	for _, ev := range timeline.Events {
		if ev.Title == "Linked Event: Battle of Gettysburg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("timeline missing linked-event marker after EventService delegation; events=%#v", timeline.Events)
	}
}
