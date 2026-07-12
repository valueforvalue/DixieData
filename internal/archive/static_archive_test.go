// static_archive_test.go — package tests for the static
// archive pipeline (issue #320 + #321 + #490).
package archive

import (
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// TestExportStaticArchive_IncludesArticles pins the slice-5.3
// contract: the static archive index emits
// window.DIXIE_DATA.articles[] alongside the existing records +
// events arrays. Each article carries the body_html + the
// resolvedRefs projection.
func TestExportStaticArchive_IncludesArticles(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	if _, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	exportSvc := NewExportService(d, soldierSvc)
	articleSvc := records.NewArticleService(soldierSvc)

	// Seed a person + an article.
	person, err := soldierSvc.Create(models.Soldier{
		DisplayID: "DXD-00099",
		FirstName: "Test",
		LastName:  "Person",
		Rank:      "Private",
		Unit:      "Test Unit",
	})
	if err != nil {
		t.Fatalf("Create person: %v", err)
	}
	art, err := articleSvc.Create(models.Article{Title: "Static target", BodyMD: "# Hello"})
	if err != nil {
		t.Fatalf("Create article: %v", err)
	}
	if _, err := articleSvc.AttachRef(art.ID, person.ID); err != nil {
		t.Fatalf("AttachRef: %v", err)
	}

	outPath := filepath.Join(testtemp.New(t).Path(), "static.zip")
	if err := exportSvc.ExportStaticArchive(outPath, testtemp.New(t).Path()); err != nil {
		t.Fatalf("ExportStaticArchive: %v", err)
	}
	// The zip contains archive_data.js; extract the file
	// into memory via zip.NewReader (avoids disk extraction).
	zr, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer zr.Close()
	var data []byte
	for _, f := range zr.File {
		if f.Name == "archive_data.js" {
			rc, rerr := f.Open()
			if rerr != nil {
				t.Fatalf("open archive_data.js: %v", rerr)
			}
			defer rc.Close()
			data, err = io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read archive_data.js: %v", err)
			}
			break
		}
	}
	if data == nil {
		t.Fatalf("archive_data.js not in zip")
	}
	if err != nil {
		t.Fatalf("read archive_data.js: %v", err)
	}
	contents := string(data)
	if !strings.Contains(contents, `"articles"`) {
		t.Errorf("archive_data.js missing articles key")
	}
	if !strings.Contains(contents, `"Static target"`) {
		t.Errorf("archive_data.js missing article title")
	}
	if !strings.Contains(contents, `DXD-00099`) {
		t.Errorf("archive_data.js missing attached person display id")
	}
}

// renderIndexForTest renders the static archive index HTML with
// placeholder template data so tests can string-match against
// the markup without seeding a full archive.
func renderIndexForTest(t *testing.T) string {
	t.Helper()
	html, err := renderStaticArchiveIndex(staticArchiveIndexData{
		ArchiveTitle: "Test Archive",
		Version:      "test",
		Build:        "test",
		GeneratedAt:  "2026-01-01",
	})
	if err != nil {
		t.Fatalf("renderStaticArchiveIndex: %v", err)
	}
	return html
}

// TestStaticArchiveIndex_RendersEventsAndArticlesTabs (issue #490,
// re-pinned in #498 slice 2) asserts the rendered index.html carries
// nav-menu labels for all three entity kinds (Person Records, Events,
// Articles) and the route markers the JS uses to render each list.
func TestStaticArchiveIndex_RendersEventsAndArticlesTabs(t *testing.T) {
	html := renderIndexForTest(t)

	// Nav menu labels — the small fixed nav the revamp uses (issue
	// #498 slice 2 replaces the legacy three-tab segmented control).
	for _, label := range []string{"Person Records", "Events", "Articles"} {
		if !strings.Contains(html, label) {
			t.Errorf("rendered index.html missing nav label %q (issue #490 / #498 slice 2)", label)
		}
	}

	// Nav-link route markers — the JS uses these to hide empty
	// entities (no Events tab when bundle.events is empty, no
	// Articles tab when bundle.articles is empty).
	if !strings.Contains(html, "data-route=\"events\"") {
		t.Errorf("missing data-route=\"events\" nav link (issue #490 / #498 slice 2)")
	}
	if !strings.Contains(html, "data-route=\"articles\"") {
		t.Errorf("missing data-route=\"articles\" nav link (issue #490 / #498 slice 2)")
	}
}

// TestStaticArchiveIndex_RendersEventDetailMarkup (issue #490,
// re-pinned in #498 slice 2) asserts the JS carries a
// renderEventDetail function that produces the Event detail screen
// (kind, date range, linked persons).
func TestStaticArchiveIndex_RendersEventDetailMarkup(t *testing.T) {
	html := renderIndexForTest(t)

	// The JS must have a function that renders event details.
	if !strings.Contains(html, "function renderEventDetail") {
		t.Errorf("missing renderEventDetail function in index.html JS (issue #490)")
	}
	// The new routeFromHash must handle #/event/ hashes (legacy
	// #event= alias still resolves per _LegacyHashAliasesStillResolve).
	if !strings.Contains(html, "/event/") {
		t.Errorf("hash router missing /event/ route pattern (issue #490 / #498 slice 2)")
	}
	// Event list rendering function.
	if !strings.Contains(html, "function renderEventRow") {
		t.Errorf("missing renderEventRow function in index.html JS (issue #490)")
	}
}

// TestStaticArchiveIndex_RendersArticleDetailMarkup (issue #490,
// re-pinned in #498 slice 2) asserts the JS carries a
// renderArticleDetail function that produces the Article detail
// screen (title, subtitle, body HTML, resolved refs).
func TestStaticArchiveIndex_RendersArticleDetailMarkup(t *testing.T) {
	html := renderIndexForTest(t)

	if !strings.Contains(html, "function renderArticleDetail") {
		t.Errorf("missing renderArticleDetail function in index.html JS (issue #490)")
	}
	if !strings.Contains(html, "/article/") {
		t.Errorf("hash router missing /article/ route pattern (issue #490 / #498 slice 2)")
	}
	if !strings.Contains(html, "function renderArticleRow") {
		t.Errorf("missing renderArticleRow function in index.html JS (issue #490)")
	}
}

// TestStaticArchiveIndex_LinkedEventsSectionInPersonDetail (issue #490)
// asserts the Person detail screen carries a "Linked Events" section
// that filters bundle.events by linkedDisplayIds membership.
func TestStaticArchiveIndex_LinkedEventsSectionInPersonDetail(t *testing.T) {
	html := renderIndexForTest(t)

	// The renderDetail function (or a helper it calls) must produce
	// a "Linked Events" section heading.
	if !strings.Contains(html, "Linked Events") {
		t.Errorf("Person detail missing \"Linked Events\" section heading (issue #490)")
	}
	// The JS must filter events by linkedDisplayIds.
	if !strings.Contains(html, "linkedDisplayIds") {
		t.Errorf("JS missing linkedDisplayIds filter for Linked Events section (issue #490)")
	}
}

// TestStaticArchiveIndex_ArticleBodyUsesRenderLinkedText (issue #490)
// asserts the Article detail screen renders bodyHtml via the existing
// renderLinkedText helper (reuses the Person Record cross-link machinery).
func TestStaticArchiveIndex_ArticleBodyUsesRenderLinkedText(t *testing.T) {
	html := renderIndexForTest(t)

	// renderArticleDetail must call renderLinkedText for body HTML.
	if !strings.Contains(html, "renderLinkedText") {
		t.Errorf("renderLinkedText helper missing from index.html (issue #490)")
	}
	// The function must exist and be referenced by article rendering.
	if !strings.Contains(html, "function renderLinkedText") {
		t.Errorf("renderLinkedText function definition missing (issue #490)")
	}
}

// TestStaticArchive_CalendarShape_PinsBundleField (issue #498 slice 1)
// pins the contract that the archive bundle carries a top-level
// `calendar` field with all 12 months. The static archive's
// Calendar landing page renders from this snapshot.
func TestStaticArchive_CalendarShape_PinsBundleField(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	if _, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	exportSvc := NewExportService(d, soldierSvc)

	outPath := filepath.Join(testtemp.New(t).Path(), "static.zip")
	if err := exportSvc.ExportStaticArchive(outPath, testtemp.New(t).Path()); err != nil {
		t.Fatalf("ExportStaticArchive: %v", err)
	}
	zr, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer zr.Close()
	var data []byte
	for _, f := range zr.File {
		if f.Name == "archive_data.js" {
			rc, rerr := f.Open()
			if rerr != nil {
				t.Fatalf("open archive_data.js: %v", rerr)
			}
			defer rc.Close()
			data, err = io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read archive_data.js: %v", err)
			}
			break
		}
	}
	contents := string(data)
	if !strings.Contains(contents, `"calendar"`) {
		t.Errorf("archive_data.js missing calendar key (issue #498 slice 1)")
	}
	// Per locked decision 1 (user-chosen), the archive ships all
	// 12 months even when empty so the JS grid renders a full
	// wall-calendar. The JSON must carry the month number for
	// every month from 1 to 12. Note: encoding/json indents with
	// a space after each colon, so the bundle emits `"month": 1`.
	for _, m := range []string{`"month": 1`, `"month": 6`, `"month": 12`} {
		if !strings.Contains(contents, m) {
			t.Errorf("archive_data.js calendar missing %s (issue #498 slice 1)", m)
		}
	}
}

// TestStaticArchive_CalendarHelper_ReturnsAllTwelveMonths (issue #498
// slice 1) pins the helper contract: staticArchiveCalendar always
// returns 12 month entries, in order, even with an empty DB.
func TestStaticArchive_CalendarHelper_ReturnsAllTwelveMonths(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	months, err := exportSvc.staticArchiveCalendar()
	if err != nil {
		t.Fatalf("staticArchiveCalendar: %v", err)
	}
	if len(months) != 12 {
		t.Fatalf("staticArchiveCalendar returned %d months, want 12 (issue #498 slice 1)", len(months))
	}
	for i, m := range months {
		if m.Month != i+1 {
			t.Errorf("month index %d has Month=%d, want %d (issue #498 slice 1)", i, m.Month, i+1)
		}
	}
}

// TestStaticArchive_CalendarHelper_PopulatesDayCounts (issue #498
// slice 1) pins that day cells carry the CalendarDaySummary
// counts when the calendar_items table has rows for that month.
func TestStaticArchive_CalendarHelper_PopulatesDayCounts(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)
	calendarSvc := records.NewCalendarService(d)

	// Seed a holiday on May 5 + an event on May 20.
	if _, err := calendarSvc.CreateCalendarItem(5, 5, records.CalendarItemInput{
		ItemType: models.CalendarItemTypeHoliday,
		Title:    "Confederate Memorial Day",
	}); err != nil {
		t.Fatalf("CreateCalendarItem holiday: %v", err)
	}
	if _, err := calendarSvc.CreateCalendarItem(5, 20, records.CalendarItemInput{
		ItemType: models.CalendarItemTypeEvent,
		Title:    "Battle of Palmito Ranch",
	}); err != nil {
		t.Fatalf("CreateCalendarItem event: %v", err)
	}

	months, err := exportSvc.staticArchiveCalendar()
	if err != nil {
		t.Fatalf("staticArchiveCalendar: %v", err)
	}
	var may *StaticArchiveCalendarMonth
	for i := range months {
		if months[i].Month == 5 {
			may = &months[i]
			break
		}
	}
	if may == nil {
		t.Fatalf("staticArchiveCalendar missing May (issue #498 slice 1)")
	}
	if day5, ok := may.Days[5]; !ok || day5.HolidayCount != 1 || day5.EventCount != 0 {
		t.Errorf("May 5 = %+v, want HolidayCount=1 (issue #498 slice 1)", may.Days[5])
	}
	if day20, ok := may.Days[20]; !ok || day20.EventCount != 1 || day20.HolidayCount != 0 {
		t.Errorf("May 20 = %+v, want EventCount=1 (issue #498 slice 1)", may.Days[20])
	}
}

// TestStaticArchiveIndex_HashRouterRendersCalendarLanding (issue #498
// slice 2) asserts the viewer ships the new hash-routed nav: the
// rendered index.html carries the nav-menu surface (Calendar /
// Browse / Insights / Person Records / Events / Articles) and a
// Calendar landing screen with a per-month grid renderer.
func TestStaticArchiveIndex_HashRouterRendersCalendarLanding(t *testing.T) {
	html := renderIndexForTest(t)

	// Nav menu — every entry per the spec.
	for _, label := range []string{"Calendar", "Browse", "Insights", "Person Records", "Events", "Articles"} {
		if !strings.Contains(html, label) {
			t.Errorf("nav menu missing %q (issue #498 slice 2)", label)
		}
	}

	// Hash router — the new #/calendar, #/browse, #/insights, #/person/{id},
	// #/event/{id}, #/article/{id} routes. The router parses each
	// route and dispatches to the per-page render function.
	for _, needle := range []string{
		"function routeFromHash",
		"function renderCalendarPage",
		"function renderBrowsePage",
		"function renderInsightsPage",
		"#/calendar",
		"#/browse",
		"#/insights",
		"#/person/",
		"#/event/",
		"#/article/",
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("rendered index.html missing %q (issue #498 slice 2)", needle)
		}
	}
}

// TestStaticArchiveIndex_CalendarGridRendersAllTwelveMonths (issue #498
// slice 2) asserts the Calendar landing page renders a 12-month grid
// from bundle.calendar[]. The grid's per-day cell must read
// bundle.calendar[month-1].days[day] and surface the AnniversaryCount /
// EventCount / HolidayCount markers.
func TestStaticArchiveIndex_CalendarGridRendersAllTwelveMonths(t *testing.T) {
	html := renderIndexForTest(t)

	// The renderCalendarPage function must walk bundle.calendar and
	// emit a 12-month grid (per locked decision 1).
	if !strings.Contains(html, "renderCalendarPage") {
		t.Errorf("missing renderCalendarPage function (issue #498 slice 2)")
	}
	// Must read bundle.calendar in JS.
	if !strings.Contains(html, "bundle.calendar") {
		t.Errorf("renderCalendarPage must read bundle.calendar (issue #498 slice 2)")
	}
	// Must surface the per-day marker counts in JS (Anniversary / Event /
	// Holiday — abbreviated a/e/h in the bundle, so the JS must
	// reference those keys).
	for _, key := range []string{".a", ".e", ".h"} {
		if !strings.Contains(html, key) {
			t.Errorf("renderCalendarPage must read .a/.e/.h day-marker keys (issue #498 slice 2)")
		}
	}
}

// TestStaticArchiveIndex_NavMenuHighlightsActiveRoute (issue #498 slice 2)
// asserts the nav menu marks the active page based on the current
// hash, so the user has visual confirmation of where they are.
func TestStaticArchiveIndex_NavMenuHighlightsActiveRoute(t *testing.T) {
	html := renderIndexForTest(t)

	// The nav-menu structure must exist as a renderable element
	// (the test of 'highlighting' is the JS, but the markup shell
	// must carry the container + the per-page data-route attributes).
	if !strings.Contains(html, `data-route="calendar"`) {
		t.Errorf("nav menu missing data-route=\"calendar\" (issue #498 slice 2)")
	}
	if !strings.Contains(html, `data-route="browse"`) {
		t.Errorf("nav menu missing data-route=\"browse\" (issue #498 slice 2)")
	}
	if !strings.Contains(html, `data-route="insights"`) {
		t.Errorf("nav menu missing data-route=\"insights\" (issue #498 slice 2)")
	}
}

// TestStaticArchiveIndex_LegacyHashAliasesStillResolve (issue #498
// slice 2) asserts the rewrite preserves the legacy hash forms from
// issue #320/#490 (#record=, #event=, #article=) so existing
// exported archives (pre-revamp) still navigate correctly. Note:
// html/template's JS-context URL escape strips a leading '#' from
// URL-like substrings in script contexts, so we assert the literal
// regex patterns the JS uses to parse the legacy hashes instead.
func TestStaticArchiveIndex_LegacyHashAliasesStillResolve(t *testing.T) {
	html := renderIndexForTest(t)

	// The routeFromHash() function must carry the legacy
	// regex matchers for #record=, #event=, #article= (rendered
	// in JS as bare record= / event= / article= after html/template
	// strips the leading '#' for safety).
	for _, needle := range []string{"^record=(.+)$", "^event=(.+)$", "^article=(.+)$"} {
		if !strings.Contains(html, needle) {
			t.Errorf("rendered index.html missing legacy hash regex %q (issue #498 slice 2)", needle)
		}
	}
	// And the function name itself, for greppability.
	if !strings.Contains(html, "legacyRecord") {
		t.Errorf("rendered index.html missing legacyRecord handler (issue #498 slice 2)")
	}
}
