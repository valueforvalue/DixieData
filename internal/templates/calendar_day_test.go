package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestCalendarDayDetailShowsCustomItemsBeforeAnniversaries(t *testing.T) {
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
	eventsIndex := strings.Index(content, "Events &amp; Holidays")
	anniversaryIndex := strings.Index(content, "Anniversaries")
	if eventsIndex == -1 || anniversaryIndex == -1 || eventsIndex > anniversaryIndex {
		t.Fatalf("custom items section should render before anniversaries")
	}
	for _, needle := range []string{"Decoration Day", "Town observance", "Edit", "Delete", "Add Calendar Item"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("calendar day detail missing %s", needle)
		}
	}
}
