package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestCalendarShowsSplitArchiveCounts(t *testing.T) {
	var buf bytes.Buffer
	err := Calendar(5, map[int]viewmodel.CalendarDaySummary{}, viewmodel.ArchiveCounts{
		SoldierCount:      12,
		SpouseRecordCount: 5,
	}, viewmodel.Quote{
		Author: "Test Author",
		Text:   "Test quote",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		">12<",
		">5<",
		"Soldiers",
		"Spouse Records",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("calendar header missing %s", needle)
		}
	}
}

func TestCalendarShowsWeekdayHeaders(t *testing.T) {
	var buf bytes.Buffer
	err := Calendar(5, map[int]viewmodel.CalendarDaySummary{}, viewmodel.ArchiveCounts{}, viewmodel.Quote{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, weekday := range []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"} {
		if !strings.Contains(content, ">"+weekday+"<") {
			t.Fatalf("calendar missing weekday header %s", weekday)
		}
	}
}

func TestCalendarShowsTypedDayMarkers(t *testing.T) {
	var buf bytes.Buffer
	err := Calendar(5, map[int]viewmodel.CalendarDaySummary{
		12: {
			AnniversaryCount: 3,
			EventCount:       1,
			HolidayCount:     2,
		},
	}, viewmodel.ArchiveCounts{}, viewmodel.Quote{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{"Anniversaries", "Events", "Holidays", ">3<", ">1<", ">2<", "bg-[#c5ab68]", "bg-[#7cb3e2]", "bg-[#d98989]"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("calendar missing marker fragment %s", needle)
		}
	}
}

func TestCalendarHighlightsCurrentDay(t *testing.T) {
	originalNow := calendarNow
	calendarNow = func() time.Time {
		return time.Date(2026, time.May, 31, 12, 0, 0, 0, time.UTC)
	}
	defer func() {
		calendarNow = originalNow
	}()

	var buf bytes.Buffer
	err := Calendar(5, map[int]viewmodel.CalendarDaySummary{}, viewmodel.ArchiveCounts{}, viewmodel.Quote{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{"Today", "ring-[#1f5b3b]", "font-semibold text-[#1f5b3b]"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("calendar missing today highlight fragment %s", needle)
		}
	}
}
