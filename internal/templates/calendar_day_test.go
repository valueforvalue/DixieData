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
