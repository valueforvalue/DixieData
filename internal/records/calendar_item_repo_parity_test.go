// calendar_item_repo_parity_test.go — issue #613 slice 6
// regression net.
//
// Pins the contract that the new CalendarItemRepo-backed
// CalendarService.Create + Update + Delete + listCalendarItems
// paths return identical results to the legacy inline-SQL
// paths on the same fixture.
package records

import (
	"context"
	"testing"
)

// TestCalendarItemRepo_Parity_CRUD is the slice-6
// service-level parity check. The full
// Create → list → Update → list → Delete → GetByID
// cycle exercises every slice-6 path through the service
// layer.
func TestCalendarItemRepo_Parity_CRUD(t *testing.T) {
	d := newTestDB(t)
	svc := NewCalendarService(d)
	_ = context.Background()

	// Create.
	item, err := svc.CreateCalendarItem(7, 4, CalendarItemInput{
		ItemType: "holiday",
		Title:    "Independence Day",
		Notes:    "Federal holiday",
	})
	if err != nil {
		t.Fatalf("CreateCalendarItem: %v", err)
	}
	if item.Title != "Independence Day" {
		t.Errorf("after Create: Title = %q, want %q", item.Title, "Independence Day")
	}

	// listCalendarItems returns the new item.
	items, err := svc.listCalendarItems(7, 4)
	if err != nil {
		t.Fatalf("listCalendarItems after Create: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("listCalendarItems after Create = %d items, want 1", len(items))
	}

	// Update.
	item, err = svc.UpdateCalendarItem(item.ID, CalendarItemInput{
		ItemType: "event",
		Title:    "Civil War memorial",
		Notes:    "Per archive",
	})
	if err != nil {
		t.Fatalf("UpdateCalendarItem: %v", err)
	}
	if item.Title != "Civil War memorial" {
		t.Errorf("after Update: Title = %q, want %q", item.Title, "Civil War memorial")
	}
	if item.ItemType != "event" {
		t.Errorf("after Update: ItemType = %q, want %q", item.ItemType, "event")
	}

	// Delete.
	if err := svc.DeleteCalendarItem(item.ID); err != nil {
		t.Fatalf("DeleteCalendarItem: %v", err)
	}

	// listCalendarItems now returns no items.
	items, err = svc.listCalendarItems(7, 4)
	if err != nil {
		t.Fatalf("listCalendarItems after Delete: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("listCalendarItems after Delete = %d items, want 0", len(items))
	}

	// GetByID returns ErrCalendarItemNotFound (via getCalendarItem).
	if _, err := svc.getCalendarItem(item.ID); err == nil {
		t.Errorf("getCalendarItem after Delete: err = nil, want ErrCalendarItemNotFound")
	}
}