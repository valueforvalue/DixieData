/**
 * audit/smoke_static_archive_revamp.mjs — RED-first regression net for
 * issue #498 (static-archive revamp: Calendar landing + Browse + Insights
 * multi-page read-only viewer).
 *
 * Source-scan probe — no live server needed. Reads
 * `internal/archive/static_archive.go` (the rendered-HTML template
 * source-of-truth) and pins the revamp surface end-to-end:
 *
 *   1. Calendar landing page (month grid, day cells with markers,
 *      archive title + owner) renders from bundle.calendar[].
 *   2. Browse page (search input, 5 filter chips per locked decision 2,
 *      sort selector, page-size selector, pagination) hash-routes from
 *      Insights drilldowns via #/browse?{field}={value}.
 *   3. Insights page (analytics snapshot cards: record_types +
 *      cemetery_density + confederate_home_status + pension_distribution
 *      + unit_representation + birth_decade_distribution +
 *      death_decade_distribution) hash-routes drilldowns back to Browse.
 *   4. Person Record / Event / Article detail screens re-route through
 *      #/person/{id} / #/event/{id} / #/article/{id}; legacy #record=,
 *      #event=, #article= hashes stay backwards-compatible.
 *   5. Nav menu (Calendar / Browse / Insights / Person Records / Events
 *      / Articles) is present on every page; active route is marked.
 *   6. Bundle shape (`archive_data.js`) carries records + events +
 *      articles + calendar + insights — all populated for a seeded
 *      archive; companion test in `static_archive_test.go` exercises
 *      the actual rendering function.
 *
 * The static archive's HTML is rendered via a Go html/template
 * constant string; the same file is the source of truth. A regression
 * that strips the nav menu, drops a filter chip, removes a page
 * renderer, or breaks the hash-routed dispatch fails the
 * corresponding assertion.
 */

import { readFileSync, existsSync } from 'node:fs';
import { strict as assert } from 'node:assert';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');

const STATIC_ARCHIVE_GO = join(ROOT, 'internal', 'archive', 'static_archive.go');
const EXPORT_SERVICE_GO = join(ROOT, 'internal', 'archive', 'export_service.go');
const JOBS_GO = join(ROOT, 'internal', 'jobs', 'jobs.go');

let pass = 0;
let fail = 0;

function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  PASS ${name}`);
  } catch (err) {
    fail++;
    console.log(`  FAIL ${name}`);
    console.log(`    ${err.message}`);
  }
}

for (const path of [STATIC_ARCHIVE_GO, EXPORT_SERVICE_GO, JOBS_GO]) {
  if (!existsSync(path)) {
    console.error(`fatal: missing ${path}`);
    process.exit(2);
  }
}

const html = readFileSync(STATIC_ARCHIVE_GO, 'utf8');
const exportSrc = readFileSync(EXPORT_SERVICE_GO, 'utf8');
const jobsSrc = readFileSync(JOBS_GO, 'utf8');

// --- Slice 1: Calendar bundle shape ---

test('slice1-01 archive bundle carries a Calendar field', () => {
  assert.ok(
    /Calendar\s+StaticArchiveCalendar/.test(exportSrc),
    'export_service.go must declare the Calendar field on the bundle struct',
  );
  assert.ok(
    exportSrc.includes('staticArchiveCalendar()'),
    'export_service.go must call staticArchiveCalendar() to populate the Calendar field',
  );
});

test('slice1-02 static_archive.go declares StaticArchiveCalendar + Day types', () => {
  assert.ok(
    html.includes('type StaticArchiveCalendarMonth struct'),
    'static_archive.go must declare StaticArchiveCalendarMonth',
  );
  assert.ok(
    html.includes('type StaticArchiveCalendarDay struct'),
    'static_archive.go must declare StaticArchiveCalendarDay',
  );
  // The 12-month invariant: the helper loops m := 1; m <= 12.
  assert.ok(
    /for\s+m\s*:=\s*1;\s*m\s*<=\s*12/.test(html),
    'staticArchiveCalendar must loop months 1-12 (slice 1 locked decision 1)',
  );
});

// --- Slice 2: Hash router + nav menu + Calendar landing ---

test('slice2-01 hash router parses #/calendar #/browse #/insights #/persons #/events #/articles', () => {
  for (const route of ['#/calendar', '#/browse', '#/insights', '#/persons', '#/events', '#/articles']) {
    assert.ok(
      html.includes(route),
      `static_archive.go JS must carry the ${route} route pattern`,
    );
  }
});

test('slice2-02 nav menu has data-route markers for all 6 routes', () => {
  for (const route of ['calendar', 'browse', 'insights', 'persons', 'events', 'articles']) {
    assert.ok(
      html.includes(`data-route="${route}"`),
      `nav menu must carry data-route="${route}"`,
    );
  }
});

test('slice2-03 nav menu label text matches the issue spec', () => {
  for (const label of ['Calendar', 'Filter', 'Insights', 'View All', 'Events', 'Articles']) {
    assert.ok(
      html.includes(label),
      `nav menu must render the label "${label}"`,
    );
  }
});

test('slice2-04 routeFromHash handles legacy #record= #event= #article= aliases', () => {
  assert.ok(
    html.includes('legacyRecord') && html.includes('legacyEvent') && html.includes('legacyArticle'),
    'routeFromHash must carry the legacy #record= / #event= / #article= alias handlers',
  );
});

test('slice2-05 Calendar landing renders single-month grid with selector (issue #500)', () => {
  assert.ok(
    html.includes('renderCalendarPage'),
    'JS must define renderCalendarPage (slice 2 Calendar landing)',
  );
  assert.ok(
    html.includes('bundle.calendar'),
    'renderCalendarPage must read bundle.calendar',
  );
  // Issue #500: month selector + prev/next buttons + single-month view.
  assert.ok(
    html.includes('calendar-month-select'),
    'Calendar must render month selector dropdown (issue #500)',
  );
  assert.ok(
    html.includes('calendar-prev-month'),
    'Calendar must render previous-month button (issue #500)',
  );
  assert.ok(
    html.includes('calendar-next-month'),
    'Calendar must render next-month button (issue #500)',
  );
  assert.ok(
    html.includes('parseCalendarQuery'),
    'Calendar must define parseCalendarQuery for month hash pre-fill (issue #500)',
  );
});

// --- Slice 2.5: Calendar items page (issue #502) ---

test('slice2-5-01 archive bundle carries a calendar_items field', () => {
  assert.ok(
    /CalendarItems\s+\[\]StaticArchiveCalendarItem/.test(exportSrc),
    'export_service.go must declare the CalendarItems field on the bundle struct (issue #502)',
  );
  assert.ok(
    html.includes('staticArchiveCalendarItems()'),
    'static_archive.go must define staticArchiveCalendarItems() helper (issue #502)',
  );
  assert.ok(
    html.includes('StaticArchiveCalendarItem'),
    'static_archive.go must declare StaticArchiveCalendarItem type (issue #502)',
  );
});

test('slice2-5-02 Calendar items page renders with route #/calendar-items', () => {
  assert.ok(
    html.includes('renderCalendarItemsPage'),
    'JS must define renderCalendarItemsPage (issue #502)',
  );
  assert.ok(
    html.includes('bundle.calendar_items'),
    'renderCalendarItemsPage must read bundle.calendar_items (issue #502)',
  );
  assert.ok(
    html.includes('calendar-items'),
    'routeFromHash must support #/calendar-items (issue #502)',
  );
});

// --- Slice 3: Browse page (filters + sort + pagination) ---

test('slice3-01 Browse renders 5 filter dropdowns per issue #499', () => {
  assert.ok(
    html.includes('BROWSE_FILTER_FIELDS'),
    'JS must define BROWSE_FILTER_FIELDS array (5 filter fields per locked decision 2)',
  );
  for (const field of ['entry_type', 'pension_state', 'unit', 'buried_in', 'confederate_home_status']) {
    assert.ok(
      html.includes(`'${field}'`),
      `BROWSE_FILTER_FIELDS must include ${field}`,
    );
  }
  // Issue #499: filter chips replaced with native <select multiple> dropdowns.
  assert.ok(
    html.includes('renderFilterDropdown'),
    'JS must define renderFilterDropdown (issue #499)',
  );
  assert.ok(
    html.includes('class="filter-select"'),
    'Browse must render filter-select class (issue #499)',
  );
  assert.ok(
    html.includes('multiple'),
    'Browse filter selects must be multiple (issue #499)',
  );
  assert.ok(
    html.includes('data-filter-field'),
    'Browse filter selects must carry data-filter-field (issue #499)',
  );
});

test('slice3-02 Browse renders search + sort + page-size controls', () => {
  assert.ok(
    html.includes('browse-search'),
    'Browse must render the search input',
  );
  for (const opt of ['display_id', 'name', 'last_edited']) {
    assert.ok(
      html.includes(`value="${opt}"`),
      `Browse sort selector must include option value="${opt}"`,
    );
  }
  for (const size of ['25', '50', '100']) {
    assert.ok(
      html.includes(`value="${size}"`),
      `Browse page-size selector must include option value="${size}"`,
    );
  }
});

test('slice3-03 Browse pre-fill reads #/browse?{field}={value} from route hash', () => {
  assert.ok(
    html.includes('parseBrowseQuery'),
    'JS must define parseBrowseQuery for hash pre-fill',
  );
  assert.ok(
    html.includes('applyBrowseFilters'),
    'JS must define applyBrowseFilters to apply dropdowns + search + sort + page-size',
  );
  assert.ok(
    html.includes('getBrowseFilterValues'),
    'JS must define getBrowseFilterValues for multi-select (issue #499)',
  );
  // The Calendar day-cell drilldown routes via date=.
  assert.ok(
    html.includes('date='),
    'Browse must honour date= pre-fill (Calendar day-cell drilldown)',
  );
  // Removable chips + clear-filters button.
  assert.ok(
    html.includes('class="filter-removable-chip"'),
    'Browse must render removable filter chips (issue #499)',
  );
  assert.ok(
    html.includes('browse-clear-filters'),
    'Browse must render clear-filters button (issue #499)',
  );
  // Issue #504: default sort = Last name (alphabetical).
  assert.ok(
    html.includes('value="name" selected'),
    'Browse sort must default to Last name (issue #504)',
  );
  // Issue #504: Browse page heading renamed to Filter.
  assert.ok(
    html.includes('<h2>Filter</h2>'),
    'Browse page heading must be Filter (issue #504)',
  );
});

// --- Slice 4: Insights page (analytics snapshot + drilldowns) ---

test('slice4-01 Insights page reads bundle.insights', () => {
  assert.ok(
    /Insights\s+AnalyticsSnapshot/.test(exportSrc),
    'export_service.go bundle struct must declare Insights field',
  );
  assert.ok(
    exportSrc.includes('staticArchiveInsights()'),
    'export_service.go must call staticArchiveInsights() to populate the Insights field',
  );
  assert.ok(
    html.includes('bundle.insights'),
    'renderInsightsPage must read bundle.insights',
  );
});

test('slice4-02 Insights page renders 7 spec cards (record_types + 6 dimensions)', () => {
  // renderRecordTypesCard + renderInsightsCountCard (used 5 times for the
  // 5 count dimensions: cemetery_density, confederate_home_status,
  // pension_distribution, unit_representation) + renderDecadeCard (used
  // 2 times for birth_decade + death_decade) cover all 7 sections.
  assert.ok(html.includes('renderRecordTypesCard'), 'must render Person Record Type card');
  assert.ok(html.includes('renderInsightsCountCard'), 'must render count-table cards');
  assert.ok(html.includes('renderDecadeCard'), 'must render decade-distribution cards');
});

test('slice4-03 Insights drilldown links route to Browse via #/browse?{field}={value}', () => {
  assert.ok(
    html.includes('#/browse?'),
    'Insights card rows must link to #/browse?{field}={value} per locked decision 3',
  );
});

// --- Slice 5: Job summary card extensions ---

test('slice5-01 jobs.StaticArchiveResult carries CalendarDaysWithData + InsightsSections', () => {
  assert.ok(
    jobsSrc.includes('CalendarDaysWithData'),
    'jobs.StaticArchiveResult must carry CalendarDaysWithData field',
  );
  assert.ok(
    jobsSrc.includes('InsightsSections'),
    'jobs.StaticArchiveResult must carry InsightsSections field',
  );
});

test('slice5-02 appendStaticArchiveStats renders Calendar + Insights lines conditionally', () => {
  assert.ok(
    jobsSrc.includes('Calendar days with data:'),
    'appendStaticArchiveStats must render the Calendar days with data line',
  );
  assert.ok(
    jobsSrc.includes('Insights sections:'),
    'appendStaticArchiveStats must render the Insights sections line',
  );
});

test('slice5-03 ExportStaticArchiveWithStats aggregates Calendar + Insights counts', () => {
  assert.ok(
    exportSrc.includes('result.CalendarDaysWithData++'),
    'ExportStaticArchiveWithStats must aggregate CalendarDaysWithData from calendarMonths',
  );
  assert.ok(
    exportSrc.includes('result.InsightsSections++'),
    'ExportStaticArchiveWithStats must aggregate InsightsSections from insightsSnapshot',
  );
});

// --- Cross-cutting: nav menu on every page + Soft theme + no server round-trip ---

test('cross-01 archive keeps Soft theme hardcoded (issue #494 inheritance)', () => {
  assert.ok(
    html.includes('data-theme="soft"'),
    'static_archive.go must stamp data-theme="soft" on <html> (locked in #494)',
  );
  assert.ok(
    !html.includes('theme-picker') && !html.includes('theme-pick'),
    'static_archive.go must NOT carry a theme picker (Soft-only since #494)',
  );
});

test('cross-02 archive keeps single self-contained file (no extra JS/CSS files)', () => {
  // The only <script src> reference is archive_data.js (data bundle).
  // No external CSS imports. archive_data.js is the only extra file.
  assert.ok(
    html.includes('./archive_data.js'),
    'archive index.html must load ./archive_data.js (the data bundle)',
  );
  // The archive_data.js filename is the only external resource; the
  // CSS lives inline in <style>.
  assert.ok(
    !/<link\s+[^>]*rel=["']stylesheet["']/i.test(html),
    'archive index.html must NOT carry an external stylesheet link (CSS inlined)',
  );
});

test('cross-03 nav-menu + Calendar + Browse + Insights are accessible on every page (rendered into #archive-page)', () => {
  // The hash router dispatches every route into the same #archive-page
  // container — nav menu lives in the static hero, not inside the page
  // container, so it's present on every page mount.
  assert.ok(
    html.includes('id="archive-page"'),
    'index.html must carry the #archive-page container that every page renders into',
  );
  assert.ok(
    html.includes('id="archive-nav-menu"'),
    'nav menu container must live outside #archive-page so it persists across page changes',
  );
});

// --- Slice 6: Export Report button (issue #505) ---
test('slice6-01 Export Report button renders on Person Record detail toolbar', () => {
  assert.ok(
    html.includes('id="detail-export-report"'),
    'detail toolbar must carry the Export Report button (issue #505)',
  );
  assert.ok(
    html.includes('target="_blank"'),
    'Export Report button must open in new tab (issue #505)',
  );
  assert.ok(
    html.includes('report-'),
    'JS must reference report-{displayId}.html pattern (issue #505)',
  );
});

// --- Slice 7: Calendar cell contrast (issue #508) ---
test('slice7-01 calendar-day.empty has no opacity or distinct background (issue #508)', () => {
  // Pin: empty cells share the base .calendar-day tint. Only difference
  // is `cursor: default` so the cell is visually non-clickable. No
  // `opacity:` and no explicit `background:` on `.calendar-day.empty`.
  const emptyRule = html.match(/\.calendar-day\.empty\s*\{[^}]*\}/);
  assert.ok(emptyRule, 'static_archive.go must define a .calendar-day.empty rule');
  const ruleBody = emptyRule[0];
  assert.ok(
    !/opacity\s*:/.test(ruleBody),
    `.calendar-day.empty must not set opacity (issue #508): ${ruleBody}`,
  );
  assert.ok(
    !/background\s*:/.test(ruleBody),
    `.calendar-day.empty must not override background (issue #508): ${ruleBody}`,
  );
});

console.log(`\nResults: ${pass} pass, ${fail} fail`);
if (fail > 0) {
  process.exit(1);
}
process.exit(0);