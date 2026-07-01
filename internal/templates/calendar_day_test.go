package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestCalendarDayDetailShowsAnniversariesBeforeCustomItemsAndCompactControls(t *testing.T) {
	var buf bytes.Buffer
	err := CalendarDayDetail(viewmodel.CalendarDayDetail{
		Month: 5,
		Day:   12,
		Form: viewmodel.CalendarItemForm{
			ItemType: "event",
		},
		Items: []viewmodel.CalendarItem{
			{ID: 1, ItemType: "holiday", Title: "Decoration Day", Notes: "Town observance"},
		},
		Anniversaries: []viewmodel.PersonRecord{
			{DisplayID: "DXD-00001", FirstName: "James", LastName: "Carter"},
		},
		AllowCustomItems: true,
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	anniversaryIndex := strings.Index(content, "Anniversaries")
	eventsIndex := strings.Index(content, "Events &amp; Holidays")
	if anniversaryIndex == -1 || eventsIndex == -1 || anniversaryIndex > eventsIndex {
		t.Fatalf("anniversaries should render before custom items")
	}
	for _, needle := range []string{"Decoration Day", "Town observance", "Edit", "Delete", "Add Event or Holiday", "Expanded", "Compact", "data-calendar-anniversary-density"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("calendar day detail missing %s", needle)
		}
	}
}

func TestCalendarDayDetailOpensActionMenuWhileEditing(t *testing.T) {
	var buf bytes.Buffer
	err := CalendarDayDetail(viewmodel.CalendarDayDetail{
		Month: 5,
		Day:   12,
		Form: viewmodel.CalendarItemForm{
			EditingID: 7,
			ItemType:  "event",
			Title:     "Camp reunion",
		},
		AllowCustomItems: true,
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{"<details class=\"relative\" open>", "Edit Calendar Item", "Cancel Edit", "Save Changes"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("calendar day edit popout missing %s", needle)
		}
	}
}

// TestCalendarDayAnniversaryCompactRowHasShareQueueButton (issue #191)
// asserts each anniversary row exposes a [+] Queue button that
// hooks into the share-queue-add click handler installed by
// frontend/app.js. Keeps the pattern consistent with the
// Browse row button.
func TestCalendarDayAnniversaryCompactRowHasShareQueueButton(t *testing.T) {
	var buf bytes.Buffer
	err := CalendarAnniversaryCompactRow(viewmodel.PersonRecord{
		ID:        42,
		DisplayID: "DXD-CAL-42",
		FirstName: "James",
		LastName:  "Carter",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, needle := range []string{
		`data-share-queue-add="42"`,
		"+ Queue",
		"Add DXD-CAL-42 to the Share Queue",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("calendar anniversary compact row missing %s; got %s", needle, content)
		}
	}
}
