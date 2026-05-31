package records

import (
	"errors"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestCalendarServiceGetMonthSummaryAndDay(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	calendarSvc := NewCalendarService(d)

	_, _ = soldierSvc.Create(models.Soldier{FirstName: "A", LastName: "One", DeathMonth: 5, DeathDay: 12})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "B", LastName: "Two", DeathMonth: 5, DeathDay: 12})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "C", LastName: "Three", DeathMonth: 5, DeathDay: 0})

	if _, err := calendarSvc.CreateCalendarItem(5, 12, CalendarItemInput{ItemType: models.CalendarItemTypeEvent, Title: "Genealogy Night"}); err != nil {
		t.Fatalf("CreateCalendarItem event: %v", err)
	}
	if _, err := calendarSvc.CreateCalendarItem(5, 12, CalendarItemInput{ItemType: models.CalendarItemTypeHoliday, Title: "Decoration Day"}); err != nil {
		t.Fatalf("CreateCalendarItem holiday: %v", err)
	}

	summary, err := calendarSvc.GetMonthSummary(5)
	if err != nil {
		t.Fatalf("GetMonthSummary: %v", err)
	}
	if got := summary[12]; got.AnniversaryCount != 2 || got.EventCount != 1 || got.HolidayCount != 1 {
		t.Fatalf("summary[12] = %+v, want anniversaries=2 events=1 holidays=1", got)
	}
	if _, ok := summary[0]; ok {
		t.Fatalf("summary should not expose day 0")
	}

	day, err := calendarSvc.GetDay(5, 12)
	if err != nil {
		t.Fatalf("GetDay: %v", err)
	}
	if len(day.Anniversaries) != 2 {
		t.Fatalf("got %d anniversaries, want 2", len(day.Anniversaries))
	}
	if len(day.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(day.Items))
	}
	if day.Items[0].ItemType != models.CalendarItemTypeHoliday {
		t.Fatalf("first item type = %q, want holiday first", day.Items[0].ItemType)
	}
}

func TestCalendarServiceCreateUpdateDelete(t *testing.T) {
	d := newTestDB(t)
	calendarSvc := NewCalendarService(d)

	if _, err := calendarSvc.CreateCalendarItem(6, 0, CalendarItemInput{ItemType: models.CalendarItemTypeHoliday, Title: "Invalid"}); err == nil {
		t.Fatal("CreateCalendarItem accepted day 0")
	}

	item, err := calendarSvc.CreateCalendarItem(6, 14, CalendarItemInput{
		ItemType: models.CalendarItemTypeHoliday,
		Title:    "Flag Day",
		Notes:    "Original note",
	})
	if err != nil {
		t.Fatalf("CreateCalendarItem: %v", err)
	}

	updated, err := calendarSvc.UpdateCalendarItem(item.ID, CalendarItemInput{
		ItemType: models.CalendarItemTypeEvent,
		Title:    "Flag Day Program",
		Notes:    "Updated note",
	})
	if err != nil {
		t.Fatalf("UpdateCalendarItem: %v", err)
	}
	if updated.ItemType != models.CalendarItemTypeEvent || updated.Title != "Flag Day Program" || updated.Notes != "Updated note" {
		t.Fatalf("updated item = %+v", updated)
	}

	if err := calendarSvc.DeleteCalendarItem(item.ID); err != nil {
		t.Fatalf("DeleteCalendarItem: %v", err)
	}
	if err := calendarSvc.DeleteCalendarItem(item.ID); !errors.Is(err, ErrCalendarItemNotFound) {
		t.Fatalf("second delete err = %v, want ErrCalendarItemNotFound", err)
	}
}
