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

func TestCalendarCompactsExportControlsIntoFoldout(t *testing.T) {
	var buf bytes.Buffer
	err := Calendar(5, map[int]viewmodel.CalendarDaySummary{}, viewmodel.ArchiveCounts{}, viewmodel.Quote{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{"Export Month", "PDF / Print", "Monthly Calendar Export", "Printer-friendly", "xl:flex-row", "xl:justify-end", "absolute left-0", "xl:left-auto xl:right-0"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("calendar missing compact export control fragment %s", needle)
		}
	}
}

func TestCalendarGridDaysAlignWithCurrentYearWeekday(t *testing.T) {
	originalNow := calendarNow
	calendarNow = func() time.Time {
		return time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	}
	defer func() {
		calendarNow = originalNow
	}()

	days := calendarGridDays(6)
	if len(days) != 35 {
		t.Fatalf("len(days) = %d, want 35", len(days))
	}
	if days[0] != 0 || days[1] != 1 {
		t.Fatalf("first week = %v, want Sunday padding before Monday the 1st", days[:7])
	}
}

func TestCalendarGridDaysUseActualMonthLength(t *testing.T) {
	originalNow := calendarNow
	calendarNow = func() time.Time {
		return time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	}
	defer func() {
		calendarNow = originalNow
	}()

	if got := calendarDaysInMonth(4); got != 30 {
		t.Fatalf("calendarDaysInMonth(4) = %d, want 30", got)
	}

	days := calendarGridDays(4)
	for _, day := range days {
		if day == 31 {
			t.Fatalf("calendarGridDays(4) unexpectedly included day 31: %v", days)
		}
	}
}

// TestCalendarEmptyStateSwapsDetailsPane (#213) — when TotalRecords == 0
// the EmptyStateCard must occupy the right (390px) column, not shove the
// CalendarGrid into it. Layout is verified by checking that the rendered
// DOM order is CalendarGrid first (column 1 of the 2-col grid) and the
// welcome panel renders in the right column, with no details-pane
// element present (it would be orphaned in row 2 col 1 of a broken grid).
func TestCalendarEmptyStateSwapsDetailsPane(t *testing.T) {
	var buf bytes.Buffer
	err := Calendar(5, map[int]viewmodel.CalendarDaySummary{}, viewmodel.ArchiveCounts{
		SoldierCount:      0,
		SpouseRecordCount: 0,
		PersonRecordCount: 0,
	}, viewmodel.Quote{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()

	for _, needle := range []string{"Welcome to DixieData", "calendar-layout"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("empty-state calendar missing %s", needle)
		}
	}
	if strings.Contains(content, `id="details-pane"`) {
		t.Fatalf("empty-state calendar must not render details-pane (would orphan into row 2 col 1)")
	}

	// CalendarGrid must render BEFORE the EmptyStateCard in DOM order
	// so CSS Grid places it in column 1 (1fr), not column 2 (390px).
	calendarIdx := strings.Index(content, `id="calendar-grid-panel"`)
	welcomeIdx := strings.Index(content, "Welcome to DixieData")
	if calendarIdx < 0 || welcomeIdx < 0 {
		t.Fatalf("missing calendar grid panel (%d) or welcome panel (%d)", calendarIdx, welcomeIdx)
	}
	if calendarIdx >= welcomeIdx {
		t.Fatalf("calendar grid (idx %d) must precede welcome panel (idx %d) so CSS Grid places it in column 1", calendarIdx, welcomeIdx)
	}
}

// TestCalendarPopulatedRendersDetailsPane (#213) — when TotalRecords > 0
// the details-pane renders in column 2 and the welcome panel is hidden.
// This is the normal post-load layout and must not regress when the
// empty-state branch swaps the column-2 child.
func TestCalendarPopulatedRendersDetailsPane(t *testing.T) {
	var buf bytes.Buffer
	err := Calendar(5, map[int]viewmodel.CalendarDaySummary{}, viewmodel.ArchiveCounts{
		SoldierCount:      12,
		SpouseRecordCount: 5,
		PersonRecordCount: 1,
	}, viewmodel.Quote{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()

	if !strings.Contains(content, `id="details-pane"`) {
		t.Fatalf("populated calendar must render details-pane in column 2")
	}
	if strings.Contains(content, "Welcome to DixieData") {
		t.Fatalf("populated calendar must not render EmptyStateCard welcome panel")
	}

	// CalendarGrid must still render first (column 1) before details-pane.
	calendarIdx := strings.Index(content, `id="calendar-grid-panel"`)
	detailsIdx := strings.Index(content, `id="details-pane"`)
	if calendarIdx < 0 || detailsIdx < 0 {
		t.Fatalf("missing calendar grid panel (%d) or details-pane (%d)", calendarIdx, detailsIdx)
	}
	if calendarIdx >= detailsIdx {
		t.Fatalf("calendar grid (idx %d) must precede details-pane (idx %d)", calendarIdx, detailsIdx)
	}
}
