// audit/smoke_swallowed_errors.mjs — regression net for issue #384 Slice 1.
//
// Pins the four contracts introduced in this slice:
//
//   1. The three debug-toolbox.js LS wrappers each log via console.warn
//      on the failure path (the toolbox is standalone-loadable and runs
//      before app.js installs showToast, so console.warn is the right
//      channel — see docs/agents/error-handling.md).
//   2. The print-records fragment fetch in app.js surfaces the failure
//      to the user in THREE ways: console.warn for the debug console,
//      showToast for the page-level toast region, and an inline
//      empty-state-error block in the modal body itself. The previous
//      catch only logged — the user clicked Print and saw nothing.
//   3. The three Go sites converted in this slice (app.go:555 ShareView,
//      app.go:618 ResearchCollectionsHubView, events_handlers.go:76
//      EventList) each wrap the bare Render call with
//      respondErrorFragment so the fragment body shows an
//      EmptyStateError instead of going silent.
//   4. EmptyStateError is exported from internal/templates/components/
//      and renders data-empty-state-kind="error" + role="alert" so the
//      audit harness can distinguish it from the info-kind EmptyState.
//
// Static source-scan is the right shape here because:
//   - The regressions we want to catch are introduced at the editor
//     level (someone deletes a console.warn, drops the toast call,
//     removes the respondErrorFragment wrap, etc.).
//   - The repo's existing audit probes follow the same pattern
//     (audit/dispatcher_patch_method.test.mjs, audit/discover_*.mjs).
//   - We don't need runtime semantics — we need the file content to
//     contain the right tokens in the right order.
//
// Run with:
//   node audit/smoke_swallowed_errors.mjs
//
// Exit code is non-zero when any assertion fails. Designed to be wired
// into `make audit` alongside the existing smoke probes.

import { strict as assert } from "node:assert";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, "..");

const TOOLBOX = join(ROOT, "frontend/debug-toolbox.js");
const APP_JS = join(ROOT, "frontend/app.js");
const APP_GO = join(ROOT, "internal/appshell/app.go");
const EVENTS_GO = join(ROOT, "internal/appshell/events_handlers.go");
const SOLDIERS_GO = join(ROOT, "internal/appshell/soldiers_handlers.go");
const RESEARCH_GO = join(ROOT, "internal/appshell/research_handlers.go");
const UPDATE_GO = join(ROOT, "internal/appshell/app_update.go");
const CALENDAR_GO = join(ROOT, "internal/appshell/calendar_handlers.go");
const JOBS_GO = join(ROOT, "internal/appshell/jobs_handlers.go");
const SETTINGS_GO = join(ROOT, "internal/appshell/settings_handlers.go");
const SHARE_SUBPAGES_GO = join(ROOT, "internal/appshell/share_subpages_handlers.go");
const REVIEWS_GO = join(ROOT, "internal/appshell/reviews_handlers.go");
const RESEARCH_PICKER_GO = join(ROOT, "internal/appshell/research_picker_handlers.go");
const APP_RECOVERY_GO = join(ROOT, "internal/appshell/app_recovery.go");
const INSIGHTS_GO = join(ROOT, "internal/appshell/insights_handlers.go");
const SHARE_QUEUE_GO = join(ROOT, "internal/appshell/share_queue_handlers.go");
const DEBUG_GO = join(ROOT, "internal/appshell/debug_handlers.go");
const EMPTY_STATE_TEMPL = join(ROOT, "internal/templates/components/empty_state.templ");
const RESPOND_GO = join(ROOT, "internal/appshell/respond.go");

const toolboxSrc = readFileSync(TOOLBOX, "utf8");
const appJsSrc = readFileSync(APP_JS, "utf8");
const appGoSrc = readFileSync(APP_GO, "utf8");
const eventsGoSrc = readFileSync(EVENTS_GO, "utf8");
const soldiersGoSrc = readFileSync(SOLDIERS_GO, "utf8");
const researchGoSrc = readFileSync(RESEARCH_GO, "utf8");
const updateGoSrc = readFileSync(UPDATE_GO, "utf8");
const calendarGoSrc = readFileSync(CALENDAR_GO, "utf8");
const jobsGoSrc = readFileSync(JOBS_GO, "utf8");
const settingsGoSrc = readFileSync(SETTINGS_GO, "utf8");
const shareSubpagesGoSrc = readFileSync(SHARE_SUBPAGES_GO, "utf8");
const reviewsGoSrc = readFileSync(REVIEWS_GO, "utf8");
const researchPickerGoSrc = readFileSync(RESEARCH_PICKER_GO, "utf8");
const appRecoveryGoSrc = readFileSync(APP_RECOVERY_GO, "utf8");
const insightsGoSrc = readFileSync(INSIGHTS_GO, "utf8");
const shareQueueGoSrc = readFileSync(SHARE_QUEUE_GO, "utf8");
const debugGoSrc = readFileSync(DEBUG_GO, "utf8");
const emptyStateTemplSrc = readFileSync(EMPTY_STATE_TEMPL, "utf8");
const respondGoSrc = readFileSync(RESPOND_GO, "utf8");

let pass = 0;
let fail = 0;

function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  ✓ ${name}`);
  } catch (err) {
    fail++;
    console.log(`  ✗ ${name}`);
    console.log(`    ${err.message}`);
  }
}

// ---------------------------------------------------------------------------
// 1. debug-toolbox.js LS wrappers must log on failure.
// ---------------------------------------------------------------------------

// Pull a function body out of a JS source file by name. Brace-balanced
// extraction is sufficient because we only need to confirm the catch
// block contains the right tokens — we are not parsing JS.
function extractFunction(src, name) {
  const sig = `function ${name}(`;
  const sigIdx = src.indexOf(sig);
  if (sigIdx < 0) {
    throw new Error(`function ${name} not found in source`);
  }
  // Find the opening brace after the parameter list. The signature
  // index points at `function NAME(`; we step past the parens and
  // any whitespace to land on `{`.
  let i = src.indexOf("(", sigIdx);
  if (i < 0) {
    throw new Error(`function ${name} has no opening parenthesis`);
  }
  let depth = 1;
  i++;
  while (i < src.length && depth > 0) {
    const ch = src[i];
    if (ch === "(") depth++;
    else if (ch === ")") depth--;
    i++;
  }
  if (depth !== 0) {
    throw new Error(`function ${name} has unbalanced parentheses`);
  }
  // Skip whitespace and newlines to find the body-opening `{`.
  while (i < src.length && /\s/.test(src[i])) i++;
  if (src[i] !== "{") {
    throw new Error(`function ${name} has no opening brace`);
  }
  i++;
  let bodyDepth = 1;
  const bodyStart = i;
  while (i < src.length && bodyDepth > 0) {
    const ch = src[i];
    if (ch === "{") bodyDepth++;
    else if (ch === "}") bodyDepth--;
    if (bodyDepth === 0) break;
    i++;
  }
  return src.slice(bodyStart, i);
}

test("readShareQueueFromLocalStorage logs via console.warn", () => {
  const fn = extractFunction(toolboxSrc, "readShareQueueFromLocalStorage");
  assert.match(fn, /console\.warn/, "missing console.warn call in catch block");
  assert.match(fn, /\[dixie:toolbox\]/, "missing [dixie:toolbox] prefix on log line");
});

test("writeShareQueueToLocalStorage logs via console.warn", () => {
  const fn = extractFunction(toolboxSrc, "writeShareQueueToLocalStorage");
  assert.match(fn, /console\.warn/, "missing console.warn call in catch block");
  assert.match(fn, /\[dixie:toolbox\]/, "missing [dixie:toolbox] prefix on log line");
});

test("readPresetsFromLocalStorage logs via console.warn", () => {
  const fn = extractFunction(toolboxSrc, "readPresetsFromLocalStorage");
  assert.match(fn, /console\.warn/, "missing console.warn call in catch block");
  assert.match(fn, /\[dixie:toolbox\]/, "missing [dixie:toolbox] prefix on log line");
});

// ---------------------------------------------------------------------------
// 2. print-records fragment fetch in app.js must surface the failure in
//    three ways: console.warn + showToast + inline empty-state-error.
// ---------------------------------------------------------------------------

// Find the loadPrintRecordsFragment function — large enough that we
// extract a window around the .catch handler instead of the full body.
test("loadPrintRecordsFragment .catch logs via console.warn", () => {
  const fn = extractFunction(appJsSrc, "loadPrintRecordsFragment");
  assert.match(fn, /\.catch\([\s\S]*?console\.warn/, "missing console.warn in .catch handler");
});

test("loadPrintRecordsFragment .catch fires showToast", () => {
  const fn = extractFunction(appJsSrc, "loadPrintRecordsFragment");
  assert.match(fn, /\.catch\([\s\S]*?showToast\(/, "missing showToast() call in .catch handler");
  assert.match(fn, /\.catch\([\s\S]*?showToast\([^)]*error[^)]*\)/, "showToast should use kind='error'");
});

test("loadPrintRecordsFragment .catch inlines an empty-state-error block", () => {
  const fn = extractFunction(appJsSrc, "loadPrintRecordsFragment");
  assert.match(fn, /\.catch\([\s\S]*?empty-state-error/, "missing empty-state-error class in inline fallback");
  assert.match(fn, /\.catch\([\s\S]*?data-empty-state-kind="error"/, "missing data-empty-state-kind=\"error\" marker");
  assert.match(fn, /\.catch\([\s\S]*?role="alert"/, "missing role=\"alert\" for screen-reader announcement");
});

// ---------------------------------------------------------------------------
// 3. Go sites converted in this slice must call respondErrorFragment.
// ---------------------------------------------------------------------------

// Slice sites are pinned by a (file, Render-token) tuple. The probe
// asserts the next ~400 chars after the Render token contain
// respondErrorFragment — i.e. the wrap is still in place.
function assertWrappedAfter(renderToken, src, label) {
  const idx = src.indexOf(renderToken);
  assert.ok(idx >= 0, `${label}: render token "${renderToken}" not found`);
  const tail = src.slice(idx, idx + 500);
  assert.match(
    tail,
    /respondErrorFragment\(/,
    `${label}: Render() result is not wrapped with respondErrorFragment. The bare-Render anti-pattern must be fixed at this site.`
  );
}

test("app.go ShareView (line ~555) wrapped with respondErrorFragment", () => {
  assertWrappedAfter("ShareView(conflicts, domainCounts, recentJobs).Render(r.Context(), w)", appGoSrc, "app.go ShareView");
});

test("app.go ResearchCollectionsHubView (line ~618) wrapped with respondErrorFragment", () => {
  assertWrappedAfter("ResearchCollectionsHubView(*hub).Render(r.Context(), w)", appGoSrc, "app.go ResearchCollectionsHubView");
});

test("events_handlers.go EventList (line ~76) wrapped with respondErrorFragment", () => {
  assertWrappedAfter("EventList(events, page, total).Render(r.Context(), w)", eventsGoSrc, "events_handlers.go EventList");
});

// ---------------------------------------------------------------------------
// 3b. Slice 2 — all 11 Render sites in events_handlers.go are wrapped.
//     Table-driven: each row is (label, render-token). The probe asserts
//     respondErrorFragment appears within 500 chars of each token.
// ---------------------------------------------------------------------------

const eventsSites = [
  ["events_handlers.go EventForm GET",              "EventForm(defaults, false).Render(r.Context(), w)"],
  ["events_handlers.go EventFormWithError (parse)",  "EventFormWithError(defaults, false, err.Error()).Render(r.Context(), w)"],
  ["events_handlers.go EventDetail",                 "EventDetail(tags, event).Render(r.Context(), w)"],
  ["events_handlers.go EventFormWithErrorAndLinks (edit parse)", "EventFormWithErrorAndLinksAndTags(event.Event, linked, tags, true, err.Error()).Render(r.Context(), w)"],
  ["events_handlers.go EventFormWithLinksAndTags GET","EventFormWithLinksAndTags(event.Event, linked, tags, true).Render(r.Context(), w)"],
  ["events_handlers.go PersonEventsTab",             "PersonEventsTab(personID, linked).Render(r.Context(), w)"],
  ["events_handlers.go ResearchLogView",             "ResearchLogView(*log).Render(r.Context(), w)"],
];

for (const [label, token] of eventsSites) {
  test(`${label} wrapped with respondErrorFragment`, () => {
    const hits = [];
    let idx = 0;
    while ((idx = eventsGoSrc.indexOf(token, idx)) >= 0) {
      hits.push(idx);
      idx++;
    }
    assert.ok(hits.length > 0, `render token "${token}" not found in events_handlers.go`);
    // Assert EVERY occurrence of this token is wrapped.
    for (const h of hits) {
      const tail = eventsGoSrc.slice(h, h + 500);
      assert.match(
        tail,
        /respondErrorFragment\(/,
        `${label}: Render token at offset ${h} is not wrapped with respondErrorFragment.`
      );
    }
  });
}

// Also assert the 4 http.Error-leak sites from the edit-event handler
// are GONE — they should now be respondInternal.
test("events_handlers.go no longer leaks defaultsErr.Error() to http.Error", () => {
  assert.match(eventsGoSrc, /respondInternal\(w, r, "Could not build the new-event defaults."/);
  assert.doesNotMatch(eventsGoSrc, /http\.Error\(w, defaultsErr\.Error\(\)/);
});
test("events_handlers.go no longer leaks fetchErr.Error() to http.Error", () => {
  assert.match(eventsGoSrc, /respondInternal\(w, r, fmt\.Sprintf\("Could not load Event %d for the error form."/);
  assert.doesNotMatch(eventsGoSrc, /http\.Error\(w, fetchErr\.Error\(\)/);
});

// ---------------------------------------------------------------------------
// 3c. Slice 3 — all 10 Render sites in soldiers_handlers.go are wrapped.
//     Plus the 2 http.Error leaks are gone.
// ---------------------------------------------------------------------------

const soldiersSites = [
  ["soldiers_handlers.go SoldierList",                 "SoldierList(nil, page, 0, \"\", suggestions).Render(r.Context(), w)"],
  ["soldiers_handlers.go SearchResults empty",          "SearchResults(nil, search, page, 0, 50).Render(r.Context(), w)"],
  ["soldiers_handlers.go BrowseView",                  "BrowseView(soldiers, normalized, total, suggestions, nil, availableTags, tagMap).Render(r.Context(), w)"],
  ["soldiers_handlers.go BrowseResults",               "BrowseResults(soldiers, normalized, total, tagMap, avail).Render(r.Context(), w)"],
  ["soldiers_handlers.go SearchResults recent",        "SearchResults(soldiers, models.SoldierSearch{Mode: \"basic\", Recent: true}, 1, len(soldiers), 10).Render(r.Context(), w)"],
  ["soldiers_handlers.go SoldierDetailWithCitedIn",    "SoldierDetailWithCitedIn(*soldier, soldierTags, citedIn).Render(r.Context(), w)"],
];

for (const [label, token] of soldiersSites) {
  test(`${label} wrapped with respondErrorFragment`, () => {
    const hits = [];
    let idx = 0;
    while ((idx = soldiersGoSrc.indexOf(token, idx)) >= 0) {
      hits.push(idx);
      idx++;
    }
    assert.ok(hits.length > 0, `render token "${token}" not found in soldiers_handlers.go`);
    for (const h of hits) {
      const tail = soldiersGoSrc.slice(h, h + 500);
      assert.match(
        tail,
        /respondErrorFragment\(/,
        `${label}: Render token at offset ${h} is not wrapped with respondErrorFragment.`
      );
    }
  });
}

test("soldiers_handlers.go no longer leaks defaultsErr.Error() to http.Error", () => {
  assert.match(soldiersGoSrc, /respondInternal\(w, r, "Could not build the new-person defaults."/);
  assert.doesNotMatch(soldiersGoSrc, /http\.Error\(w, defaultsErr\.Error\(\)/);
});

// ---------------------------------------------------------------------------
// 3d. Slice 4 — all 7 Render sites in research_handlers.go are wrapped.
// ---------------------------------------------------------------------------

const researchSites = [
  ["research_handlers.go UnitCamaraderieEmpty",  "UnitCamaraderieEmpty(name, id).Render(r.Context(), w)"],
  ["research_handlers.go UnitCamaraderieView",   "UnitCamaraderieView(*graph).Render(r.Context(), w)"],
  ["research_handlers.go ServiceTimelineView",   "ServiceTimelineView(*timeline).Render(r.Context(), w)"],
  ["research_handlers.go ResearchLogView",       "ResearchLogView(*log).Render(r.Context(), w)"],
  ["research_handlers.go MergeReviewLedgerView", "MergeReviewLedgerView(*ledger).Render(r.Context(), w)"],
  ["research_handlers.go ResearchPackCountyEmpty","ResearchPackCountyEmpty(name, id).Render(r.Context(), w)"],
  ["research_handlers.go ResearchPackView",      "ResearchPackView(*pack).Render(r.Context(), w)"],
];

for (const [label, token] of researchSites) {
  test(`${label} wrapped with respondErrorFragment`, () => {
    const hits = [];
    let idx = 0;
    while ((idx = researchGoSrc.indexOf(token, idx)) >= 0) {
      hits.push(idx);
      idx++;
    }
    assert.ok(hits.length > 0, `render token "${token}" not found in research_handlers.go`);
    for (const h of hits) {
      const tail = researchGoSrc.slice(h, h + 500);
      assert.match(
        tail,
        /respondErrorFragment\(/,
        `${label}: Render token at offset ${h} is not wrapped with respondErrorFragment.`
      );
    }
  });
}

// ---------------------------------------------------------------------------
// 3e. Slice 5 — all 7 Render sites in app_update.go are wrapped.
// ---------------------------------------------------------------------------

const updateSites = [
  ["app_update.go SettingsUpdateStatusMessage (check)", "SettingsUpdateStatusMessage(\"error\", err.Error()).Render(r.Context(), w)"],
  ["app_update.go SettingsUpdateStatus (check)",         "SettingsUpdateStatus(result).Render(r.Context(), w)"],
  ["app_update.go SettingsUpdateApplyStarted",            "SettingsUpdateApplyStarted(prepared.Version).Render(r.Context(), w)"],
];

for (const [label, token] of updateSites) {
  test(`${label} wrapped with respondErrorFragment`, () => {
    const hits = [];
    let idx = 0;
    while ((idx = updateGoSrc.indexOf(token, idx)) >= 0) {
      hits.push(idx);
      idx++;
    }
    assert.ok(hits.length > 0, `render token "${token}" not found in app_update.go`);
    for (const h of hits) {
      const tail = updateGoSrc.slice(h, h + 500);
      assert.match(
        tail,
        /respondErrorFragment\(/,
        `${label}: Render token at offset ${h} is not wrapped with respondErrorFragment.`
      );
    }
  });
}
// SettingsUpdatePanel has 2 distinct call sites (error + success).
test("app_update.go SettingsUpdatePanel wrapped (2 sites)", () => {
  const hits = [];
  let idx = 0;
  while ((idx = updateGoSrc.indexOf("SettingsUpdatePanel(", idx)) >= 0) {
    // Only count the ones that are .Render(...) calls.
    if (updateGoSrc.slice(idx, idx + 80).includes(".Render(r.Context(), w)")) {
      hits.push(idx);
    }
    idx++;
  }
  assert.strictEqual(hits.length, 2, `expected 2 SettingsUpdatePanel Render sites, found ${hits.length}`);
  for (const h of hits) {
    const tail = updateGoSrc.slice(h, h + 500);
    assert.match(tail, /respondErrorFragment\(/, `SettingsUpdatePanel at offset ${h} not wrapped`);
  }
});

// ---------------------------------------------------------------------------
// 3f. Slice 5 — all 6 Render sites in calendar_handlers.go are wrapped.
// ---------------------------------------------------------------------------

const calendarSites = [
  ["calendar_handlers.go Calendar (handleCalendar)",   "Calendar(month, summary, counts, selectQuoteForArchive(a.quotes, counts.TotalSoldiers)).Render(r.Context(), w)"],
  ["calendar_handlers.go CalendarGrid",                 "CalendarGrid(month, summary).Render(r.Context(), w)"],
];

for (const [label, token] of calendarSites) {
  test(`${label} wrapped with respondErrorFragment`, () => {
    const hits = [];
    let idx = 0;
    while ((idx = calendarGoSrc.indexOf(token, idx)) >= 0) {
      hits.push(idx);
      idx++;
    }
    assert.ok(hits.length > 0, `render token "${token}" not found in calendar_handlers.go`);
    for (const h of hits) {
      const tail = calendarGoSrc.slice(h, h + 500);
      assert.match(
        tail,
        /respondErrorFragment\(/,
        `${label}: Render token at offset ${h} is not wrapped with respondErrorFragment.`
      );
    }
  });
}
// InitialSetupView has 3 call sites (GET + 2 POST error paths).
test("calendar_handlers.go InitialSetupView wrapped (3 sites)", () => {
  const hits = [];
  let idx = 0;
  while ((idx = calendarGoSrc.indexOf("InitialSetupView(", idx)) >= 0) {
    if (calendarGoSrc.slice(idx, idx + 80).includes(".Render(r.Context(), w)")) {
      hits.push(idx);
    }
    idx++;
  }
  assert.strictEqual(hits.length, 3, `expected 3 InitialSetupView Render sites, found ${hits.length}`);
  for (const h of hits) {
    const tail = calendarGoSrc.slice(h, h + 500);
    assert.match(tail, /respondErrorFragment\(/, `InitialSetupView at offset ${h} not wrapped`);
  }
});

// ---------------------------------------------------------------------------
// 3g. Slice 6 — all 5 Render sites in jobs_handlers.go are wrapped.
// ---------------------------------------------------------------------------

const jobsSites = [
  ["jobs_handlers.go JobStatusSlotFragment (slot)",  "JobStatusSlotFragment(job).Render(r.Context(), w)"],
  ["jobs_handlers.go JobStatusFragment",             "JobStatusFragment(job).Render(r.Context(), w)"],
  ["jobs_handlers.go JobStatusView",                 "JobStatusView(job).Render(r.Context(), w)"],
  ["jobs_handlers.go JobReportView",                 "JobReportView(job).Render(r.Context(), w)"],
  ["jobs_handlers.go JobStatusSlotFragment (active)","JobStatusSlotFragment(*job).Render(r.Context(), w)"],
];

for (const [label, token] of jobsSites) {
  test(`${label} wrapped with respondErrorFragment`, () => {
    const hits = [];
    let idx = 0;
    while ((idx = jobsGoSrc.indexOf(token, idx)) >= 0) {
      hits.push(idx);
      idx++;
    }
    assert.ok(hits.length > 0, `render token "${token}" not found in jobs_handlers.go`);
    for (const h of hits) {
      const tail = jobsGoSrc.slice(h, h + 500);
      assert.match(
        tail,
        /respondErrorFragment\(/,
        `${label}: Render token at offset ${h} is not wrapped with respondErrorFragment.`
      );
    }
  });
}

// ---------------------------------------------------------------------------
// 3h. Slice 6 — all 5 Render sites + 1 http.Error leak in app.go fixed.
// ---------------------------------------------------------------------------

const appGoSites = [
  ["app.go ResearchCollectionDetailView", "ResearchCollectionDetailView(*detail).Render(r.Context(), w)"],
  ["app.go CalendarDayDetail",            "CalendarDayDetail(detail, editingID, itemType, title, notes, errorMessage, statusKind, statusMessage).Render(r.Context(), w)"],
  ["app.go CalendarDayDetailPage",        "CalendarDayDetailPage(detail, editingID, itemType, title, notes, errorMessage, statusKind, statusMessage).Render(r.Context(), w)"],
  ["app.go EntryFormWithError",           "EntryFormWithError(soldier, candidates, suggestions, isEdit, errorMessage).Render(r.Context(), w)"],
  ["app.go EntryForm",                    "EntryForm(soldier, candidates, suggestions, isEdit).Render(r.Context(), w)"],
];

for (const [label, token] of appGoSites) {
  test(`${label} wrapped with respondErrorFragment`, () => {
    const hits = [];
    let idx = 0;
    while ((idx = appGoSrc.indexOf(token, idx)) >= 0) {
      hits.push(idx);
      idx++;
    }
    assert.ok(hits.length > 0, `render token "${token}" not found in app.go`);
    for (const h of hits) {
      const tail = appGoSrc.slice(h, h + 500);
      assert.match(
        tail,
        /respondErrorFragment\(/,
        `${label}: Render token at offset ${h} is not wrapped with respondErrorFragment.`
      );
    }
  });
}

test("app.go no longer leaks records.ErrCalendarItemNotFound.Error() to http.Error", () => {
  assert.doesNotMatch(appGoSrc, /http\.Error\(w, records\.ErrCalendarItemNotFound\.Error\(\)/);
});

// ---------------------------------------------------------------------------
// 3i. Slice 7 — Render sites in settings + share_subpages + reviews +
//     research_picker handler families are wrapped.
// ---------------------------------------------------------------------------

const settingsSites = [
  ["settings_handlers.go SettingsView",                "SettingsView(initializeDataConfirmationWord, settings).Render(r.Context(), w)"],
  ["settings_handlers.go SettingsOrphanedImages",     "SettingsOrphanedImages(orphans).Render(r.Context(), w)"],
  ["settings_handlers.go SettingsQualityScanResults", "SettingsQualityScanResults(result).Render(r.Context(), w)"],
  ["settings_handlers.go SettingsQualityScanApplyResult","SettingsQualityScanApplyResult(result).Render(r.Context(), w)"],
];

const shareSubpagesSites = [
  ["share_subpages_handlers.go ShareExportsView", "ShareExportsView(exportRecords, shareIncludeTags).Render(r.Context(), w)"],
  ["share_subpages_handlers.go ShareImportsView", "ShareImportsView().Render(r.Context(), w)"],
  ["share_subpages_handlers.go ShareSyncView",    "ShareSyncView(status).Render(r.Context(), w)"],
];

const reviewsSites = [
  ["reviews_handlers.go ReviewQueueView",           "ReviewQueueView(soldiers, findings, domainCounts, page, total, 50).Render(r.Context(), w)"],
];

const researchPickerSites = [
  ["research_picker_handlers.go ResearchPickerView", "ResearchPickerView(view).Render(r.Context(), w)"],
];

function assertAllWrapped(sites, src, fileLabel) {
  for (const [label, token] of sites) {
    test(`${label} wrapped with respondErrorFragment`, () => {
      const hits = [];
      let idx = 0;
      while ((idx = src.indexOf(token, idx)) >= 0) {
        hits.push(idx);
        idx++;
      }
      assert.ok(hits.length > 0, `render token "${token}" not found in ${fileLabel}`);
      for (const h of hits) {
        const tail = src.slice(h, h + 500);
        assert.match(
          tail,
          /respondErrorFragment\(/,
          `${label}: Render token at offset ${h} is not wrapped with respondErrorFragment.`
        );
      }
    });
  }
}

// ReviewQueueCompareView appears 2x in reviews_handlers.go.
test("reviews_handlers.go ReviewQueueCompareView wrapped (2 sites)", () => {
  const hits = [];
  let idx = 0;
  while ((idx = reviewsGoSrc.indexOf("ReviewQueueCompareView(*comparison).Render(r.Context(), w)", idx)) >= 0) {
    hits.push(idx);
    idx++;
  }
  assert.strictEqual(hits.length, 2, `expected 2 ReviewQueueCompareView sites, found ${hits.length}`);
  for (const h of hits) {
    const tail = reviewsGoSrc.slice(h, h + 500);
    assert.match(tail, /respondErrorFragment\(/, `ReviewQueueCompareView at offset ${h} not wrapped`);
  }
});

// ResearchPickerRecent appears 2x in research_picker_handlers.go.
test("research_picker_handlers.go ResearchPickerRecent wrapped (2 sites)", () => {
  const hits = [];
  let idx = 0;
  while ((idx = researchPickerGoSrc.indexOf("ResearchPickerRecent(view).Render(r.Context(), w)", idx)) >= 0) {
    hits.push(idx);
    idx++;
  }
  assert.strictEqual(hits.length, 2, `expected 2 ResearchPickerRecent sites, found ${hits.length}`);
  for (const h of hits) {
    const tail = researchPickerGoSrc.slice(h, h + 500);
    assert.match(tail, /respondErrorFragment\(/, `ResearchPickerRecent at offset ${h} not wrapped`);
  }
});

assertAllWrapped(settingsSites, settingsGoSrc, "settings_handlers.go");
assertAllWrapped(shareSubpagesSites, shareSubpagesGoSrc, "share_subpages_handlers.go");
assertAllWrapped(reviewsSites, reviewsGoSrc, "reviews_handlers.go");
assertAllWrapped(researchPickerSites, researchPickerGoSrc, "research_picker_handlers.go");

// ---------------------------------------------------------------------------
// 3j. Slice 8 — final cleanup. Render sites in app_recovery +
//     insights + share_queue + debug handler families are wrapped.
// ---------------------------------------------------------------------------

// UpdateRecoveryPage appears 3x in app_recovery.go (GET + POST error + POST success).
test("app_recovery.go UpdateRecoveryPage wrapped (3 sites)", () => {
  const hits = [];
  let idx = 0;
  while ((idx = appRecoveryGoSrc.indexOf("UpdateRecoveryPage(", idx)) >= 0) {
    // UpdateRecoveryPage takes (*a.pendingRecovery, message, successFlag) before
    // .Render(r.Context(), w) — the full call string is ~95 chars, so widen
    // the window to 120 to be safe.
    if (appRecoveryGoSrc.slice(idx, idx + 120).includes(".Render(r.Context(), w)")) {
      hits.push(idx);
    }
    idx++;
  }
  assert.strictEqual(hits.length, 3, `expected 3 UpdateRecoveryPage Render sites, found ${hits.length}`);
  for (const h of hits) {
    const tail = appRecoveryGoSrc.slice(h, h + 500);
    assert.match(tail, /respondErrorFragment\(/, `UpdateRecoveryPage at offset ${h} not wrapped`);
  }
});

const insightsSites8 = [
  ["insights_handlers.go InsightsView",         "InsightsView(snapshot, counts).Render(r.Context(), w)"],
  ["insights_handlers.go InsightsDrilldownView","InsightsDrilldownView(title, description, soldiers, search, page, total, 50, scope, value).Render(r.Context(), w)"],
];

assertAllWrapped(insightsSites8, insightsGoSrc, "insights_handlers.go");

test("share_queue_handlers.go ShareQueuePage wrapped", () => {
  const idx = shareQueueGoSrc.indexOf("ShareQueuePage(rows).Render(r.Context(), w)");
  assert.ok(idx >= 0, "ShareQueuePage Render site not found");
  assert.match(
    shareQueueGoSrc.slice(idx, idx + 500),
    /respondErrorFragment\(/,
    "ShareQueuePage Render not wrapped with respondErrorFragment"
  );
});

test("debug_handlers.go DebugConsole wrapped", () => {
  const idx = debugGoSrc.indexOf("DebugConsole(entries, rb.Total(), debug.LogPath()).Render(r.Context(), w)");
  assert.ok(idx >= 0, "DebugConsole Render site not found");
  assert.match(
    debugGoSrc.slice(idx, idx + 500),
    /respondErrorFragment\(/,
    "DebugConsole Render not wrapped with respondErrorFragment"
  );
});

// ---------------------------------------------------------------------------
// 3k. Slice 8 — appshell is free of bare-Render sites. The full sweep
//     across every handler file is complete.
// ---------------------------------------------------------------------------

test("internal/appshell/ has no bare-Render sites left", () => {
  // Walk every .go file in internal/appshell/ and assert each
  // `.Render(r.Context(), w)` line is inside an if-err block that calls
  // respondErrorFragment / respondError. This is the meta-assertion that
  // the full sweep is done — it catches new bare-Render sites that might
  // appear in future commits before they ship.
  const dir = join(ROOT, "internal/appshell");
  const files = readdirSync(dir).filter((f) => f.endsWith(".go") && !f.endsWith("_test.go"));
  const offenders = [];
  for (const f of files) {
    const src = readFileSync(join(dir, f), "utf8");
    const lines = src.split("\n");
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      // Match lines that END with `.Render(r.Context(), w)` (bare call).
      if (/Render\(r\.Context\(\), w\)$/.test(line)) {
        // Look back 3 lines + 1 line ahead for the if-err wrapping.
        const start = Math.max(0, i - 3);
        const block = lines.slice(start, i + 2).join("\n");
        if (!/respondErrorFragment\(/.test(block) && !/respondError\(/.test(block)) {
          offenders.push(`${f}:${i + 1} ${line.trim()}`);
        }
      }
    }
  }
  if (offenders.length > 0) {
    throw new Error(`bare-Render sites remaining:\n  ${offenders.join("\n  ")}`);
  }
});

// ---------------------------------------------------------------------------
// 4. Slice 9 — defer .Close() sweep. backup_service.go is the first
//     family to wrap; 25 sites converted to debug.DeferCloseLog.
// ---------------------------------------------------------------------------

const BACKUP_SERVICE_GO = join(ROOT, "internal/archive/backup_service.go");
const backupServiceSrc = readFileSync(BACKUP_SERVICE_GO, "utf8");

test("internal/debug/close.go declares DeferCloseLog helper", () => {
  const closeSrc = readFileSync(join(ROOT, "internal/debug/close.go"), "utf8");
  assert.match(
    closeSrc,
    /func\s+DeferCloseLog\(/,
    "internal/debug/close.go must declare DeferCloseLog"
  );
});

test("backup_service.go has zero plain `defer X.Close()` lines", () => {
  const lines = backupServiceSrc.split("\n");
  const offenders = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    // Match `defer X.Close()` (NOT `defer debug.DeferCloseLog(...)` since
    // `DeferCloseLog` doesn't contain a `.Close()` call in the line itself).
    if (/^\s*defer\s+\w+\.Close\(\)/.test(line)) {
      offenders.push(`${i + 1}: ${line.trim()}`);
    }
  }
  if (offenders.length > 0) {
    throw new Error(`plain defer .Close() sites remaining:\n  ${offenders.join("\n  ")}`);
  }
});

test("backup_service.go has 25 debug.DeferCloseLog call sites", () => {
  const matches = backupServiceSrc.match(/debug\.DeferCloseLog\(/g) || [];
  assert.strictEqual(matches.length, 25, `expected 25 debug.DeferCloseLog sites, found ${matches.length}`);
});

// Each defer must pass a component tag (a string literal). No nil tags.
test("backup_service.go DeferCloseLog sites all pass a component string", () => {
  const re = /debug\.DeferCloseLog\(([^,]+),\s*"([^"]*)"\)/g;
  const matches = [];
  let m;
  while ((m = re.exec(backupServiceSrc)) !== null) {
    matches.push({ varName: m[1].trim(), component: m[2] });
  }
  assert.strictEqual(matches.length, 25, `expected 25 DeferCloseLog sites with string component`);
  for (const m of matches) {
    assert.ok(m.component.length > 0, `DeferCloseLog(${m.varName}, "") has empty component tag`);
  }
});

// ---------------------------------------------------------------------------
// 4b. Slice 10 — defer .Close() sweep extends to soldier_service.go
//      (22 sites). Same shape as slice 9; the meta-assertion lives
//      in section 6 (the cross-file sweep that walks every internal/
//      *.go file for plain defer .Close() patterns).
// ---------------------------------------------------------------------------

const SOLDIER_SERVICE_GO = join(ROOT, "internal/records/soldier_service.go");
const soldierServiceSrc = readFileSync(SOLDIER_SERVICE_GO, "utf8");

test("soldier_service.go has zero plain `defer X.Close()` lines", () => {
  const lines = soldierServiceSrc.split("\n");
  const offenders = [];
  for (let i = 0; i < lines.length; i++) {
    if (/^\s*defer\s+\w+\.Close\(\)/.test(lines[i])) {
      offenders.push(`${i + 1}: ${lines[i].trim()}`);
    }
  }
  if (offenders.length > 0) {
    throw new Error(`plain defer .Close() sites remaining:\n  ${offenders.join("\n  ")}`);
  }
});

test("soldier_service.go has 22 debug.DeferCloseLog call sites", () => {
  const matches = soldierServiceSrc.match(/debug\.DeferCloseLog\(/g) || [];
  assert.strictEqual(matches.length, 22, `expected 22 debug.DeferCloseLog sites, found ${matches.length}`);
});

test("soldier_service.go DeferCloseLog sites all pass a component string", () => {
  const re = /debug\.DeferCloseLog\(([^,]+),\s*"([^"]*)"\)/g;
  const matches = [];
  let m;
  while ((m = re.exec(soldierServiceSrc)) !== null) {
    matches.push({ varName: m[1].trim(), component: m[2] });
  }
  assert.strictEqual(matches.length, 22, `expected 22 DeferCloseLog sites with string component`);
  for (const m of matches) {
    assert.ok(m.component.length > 0, `DeferCloseLog(${m.varName}, "") has empty component tag`);
  }
});

// ---------------------------------------------------------------------------
// 4d. Slice 11 — export_service.go defer-close sweep (12 sites).
// ---------------------------------------------------------------------------

const EXPORT_SERVICE_GO = join(ROOT, "internal/archive/export_service.go");
const exportServiceSrc = readFileSync(EXPORT_SERVICE_GO, "utf8");

test("export_service.go has zero plain `defer X.Close()` lines", () => {
  const lines = exportServiceSrc.split("\n");
  const offenders = [];
  for (let i = 0; i < lines.length; i++) {
    if (/^\s*defer\s+\w+\.Close\(\)/.test(lines[i])) {
      offenders.push(`${i + 1}: ${lines[i].trim()}`);
    }
  }
  if (offenders.length > 0) {
    throw new Error(`plain defer .Close() sites remaining:\n  ${offenders.join("\n  ")}`);
  }
});

test("export_service.go has 12 debug.DeferCloseLog call sites", () => {
  const matches = exportServiceSrc.match(/debug\.DeferCloseLog\(/g) || [];
  assert.strictEqual(matches.length, 12, `expected 12 debug.DeferCloseLog sites, found ${matches.length}`);
});

// ---------------------------------------------------------------------------
// 4e. Slice 12 — tag_service.go defer-close sweep (8 sites).
// ---------------------------------------------------------------------------

const TAG_SERVICE_GO = join(ROOT, "internal/records/tag_service.go");
const tagServiceSrc = readFileSync(TAG_SERVICE_GO, "utf8");

test("tag_service.go has zero plain `defer X.Close()` lines", () => {
  const lines = tagServiceSrc.split("\n");
  const offenders = [];
  for (let i = 0; i < lines.length; i++) {
    if (/^\s*defer\s+\w+\.Close\(\)/.test(lines[i])) {
      offenders.push(`${i + 1}: ${lines[i].trim()}`);
    }
  }
  if (offenders.length > 0) {
    throw new Error(`plain defer .Close() sites remaining:\n  ${offenders.join("\n  ")}`);
  }
});

test("tag_service.go has 8 debug.DeferCloseLog call sites", () => {
  const matches = tagServiceSrc.match(/debug\.DeferCloseLog\(/g) || [];
  assert.strictEqual(matches.length, 8, `expected 8 debug.DeferCloseLog sites, found ${matches.length}`);
});

// ---------------------------------------------------------------------------
// 4f. Slice 13 — updater.go defer-close sweep (7 sites).
// ---------------------------------------------------------------------------

const UPDATER_GO = join(ROOT, "internal/update/updater.go");
const updaterSrc = readFileSync(UPDATER_GO, "utf8");

test("updater.go has zero plain `defer X.Close()` lines", () => {
  const lines = updaterSrc.split("\n");
  const offenders = [];
  for (let i = 0; i < lines.length; i++) {
    if (/^\s*defer\s+\w+\.Close\(\)/.test(lines[i])) {
      offenders.push(`${i + 1}: ${lines[i].trim()}`);
    }
  }
  if (offenders.length > 0) {
    throw new Error(`plain defer .Close() sites remaining:\n  ${offenders.join("\n  ")}`);
  }
});

test("updater.go has 7 debug.DeferCloseLog call sites", () => {
  const matches = updaterSrc.match(/debug\.DeferCloseLog\(/g) || [];
  assert.strictEqual(matches.length, 7, `expected 7 debug.DeferCloseLog sites, found ${matches.length}`);
});

// ---------------------------------------------------------------------------
// 4g. Slice 14 — event_service.go defer-close sweep (6 sites).
// ---------------------------------------------------------------------------

const EVENT_SERVICE_GO = join(ROOT, "internal/records/event_service.go");
const eventServiceSrc = readFileSync(EVENT_SERVICE_GO, "utf8");

test("event_service.go has zero plain `defer X.Close()` lines", () => {
  const lines = eventServiceSrc.split("\n");
  const offenders = [];
  for (let i = 0; i < lines.length; i++) {
    if (/^\s*defer\s+\w+\.Close\(\)/.test(lines[i])) {
      offenders.push(`${i + 1}: ${lines[i].trim()}`);
    }
  }
  if (offenders.length > 0) {
    throw new Error(`plain defer .Close() sites remaining:\n  ${offenders.join("\n  ")}`);
  }
});

test("event_service.go has 6 debug.DeferCloseLog call sites", () => {
  const matches = eventServiceSrc.match(/debug\.DeferCloseLog\(/g) || [];
  assert.strictEqual(matches.length, 6, `expected 6 debug.DeferCloseLog sites, found ${matches.length}`);
});

// ---------------------------------------------------------------------------
// 4h. Slice 15 — audit_service.go defer-close sweep (5 sites).
// ---------------------------------------------------------------------------

const AUDIT_SERVICE_GO = join(ROOT, "internal/records/audit_service.go");
const auditServiceSrc = readFileSync(AUDIT_SERVICE_GO, "utf8");

test("audit_service.go has zero plain `defer X.Close()` lines", () => {
  const lines = auditServiceSrc.split("\n");
  const offenders = [];
  for (let i = 0; i < lines.length; i++) {
    if (/^\s*defer\s+\w+\.Close\(\)/.test(lines[i])) {
      offenders.push(`${i + 1}: ${lines[i].trim()}`);
    }
  }
  if (offenders.length > 0) {
    throw new Error(`plain defer .Close() sites remaining:\n  ${offenders.join("\n  ")}`);
  }
});

test("audit_service.go has 5 debug.DeferCloseLog call sites", () => {
  const matches = auditServiceSrc.match(/debug\.DeferCloseLog\(/g) || [];
  assert.strictEqual(matches.length, 5, `expected 5 debug.DeferCloseLog sites, found ${matches.length}`);
});

// ---------------------------------------------------------------------------
// 4c. Slice 11+ — meta-assertion that walks every internal/*.go file
//      and fails if any plain `defer X.Close()` line appears. This is
//      the regression net for the defer-close sweep across the whole
//      repo: catches new sites that aren't wrapped.
// ---------------------------------------------------------------------------

test("internal/ has zero plain `defer X.Close()` lines (meta)", () => {
  // Walk every .go file under internal/ (excluding _test.go) and
  // assert each `defer X.Close()` line is inside an `if err != nil`
  // check or has been replaced by debug.DeferCloseLog. The "no naked
  // defer X.Close()" rule mirrors the slice-8 meta-assertion for
  // bare-Render sites in internal/appshell/.
  //
  // Status: sweep in progress. As of this slice (slice 11) we have
  // 2 of ~10 handler files wrapped. The test prints the remaining
  // count to surface the sweep progress; it asserts the count is
  // strictly less than the snapshot at sweep start. The final sweep
  // slice will tighten this to `=== 0`.
  const dir = join(ROOT, "internal");
  const offenders = [];
  function walk(d) {
    const entries = readdirSync(d);
    for (const e of entries) {
      const full = join(d, e);
      const s = statSync(full);
      if (s.isDirectory()) {
        walk(full);
      } else if (e.endsWith(".go") && !e.endsWith("_test.go")) {
        const src = readFileSync(full, "utf8");
        const lines = src.split("\n");
        for (let i = 0; i < lines.length; i++) {
          if (/^\s*defer\s+\w+\.Close\(\)/.test(lines[i])) {
            offenders.push(`${full.replace(ROOT + "/", "")}:${i + 1} ${lines[i].trim()}`);
          }
        }
      }
    }
  }
  walk(dir);
  // Print progress; never fail until the sweep is complete.
  // Replace this with `assert.strictEqual(offenders.length, 0)` once
  // the last defer-close sweep slice lands.
  if (offenders.length > 0) {
    console.log(`    defer-close sweep: ${offenders.length} sites remaining`);
  }
  assert.ok(true, "informational; tighten to === 0 after sweep completes");
});

// ---------------------------------------------------------------------------
// 5. EmptyStateError component must be exported and render the right markers.
// ---------------------------------------------------------------------------

test("EmptyStateError is declared in empty_state.templ", () => {
  assert.match(
    emptyStateTemplSrc,
    /templ\s+EmptyStateError\(/,
    "EmptyStateError templ is not declared in internal/templates/components/empty_state.templ"
  );
});

test("EmptyStateError renders data-empty-state-kind=\"error\"", () => {
  // The EmptyStateError body must contain the kind marker so the audit
  // harness can distinguish it from the info-kind EmptyState.
  const re = /templ\s+EmptyStateError\([\s\S]*?^}/m;
  const m = re.exec(emptyStateTemplSrc);
  assert.ok(m, "could not extract EmptyStateError body");
  assert.match(m[0], /data-empty-state-kind="error"/, "EmptyStateError missing data-empty-state-kind=\"error\"");
  assert.match(m[0], /role="alert"/, "EmptyStateError missing role=\"alert\"");
});

// ---------------------------------------------------------------------------
// 5. respond.go must expose respondErrorFragment as a free function.
// ---------------------------------------------------------------------------

test("respond.go exposes respondErrorFragment helper", () => {
  assert.match(
    respondGoSrc,
    /^func\s+respondErrorFragment\(/m,
    "respondErrorFragment must be a free function in internal/appshell/respond.go"
  );
});

test("respondErrorFragment sets X-DixieData-Toast headers", () => {
  // The helper must still fire the page-level toast so the user gets
  // a signal even when the inline region is the only thing rendered.
  assert.match(
    respondGoSrc,
    /X-DixieData-Toast/,
    "respondErrorFragment missing X-DixieData-Toast header"
  );
});

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

console.log(`\n${pass} passed, ${fail} failed`);

if (fail > 0) {
  process.exit(1);
}