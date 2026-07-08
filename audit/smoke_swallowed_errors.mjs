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
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, "..");

const TOOLBOX = join(ROOT, "frontend/debug-toolbox.js");
const APP_JS = join(ROOT, "frontend/app.js");
const APP_GO = join(ROOT, "internal/appshell/app.go");
const EVENTS_GO = join(ROOT, "internal/appshell/events_handlers.go");
const SOLDIERS_GO = join(ROOT, "internal/appshell/soldiers_handlers.go");
const EMPTY_STATE_TEMPL = join(ROOT, "internal/templates/components/empty_state.templ");
const RESPOND_GO = join(ROOT, "internal/appshell/respond.go");

const toolboxSrc = readFileSync(TOOLBOX, "utf8");
const appJsSrc = readFileSync(APP_JS, "utf8");
const appGoSrc = readFileSync(APP_GO, "utf8");
const eventsGoSrc = readFileSync(EVENTS_GO, "utf8");
const soldiersGoSrc = readFileSync(SOLDIERS_GO, "utf8");
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
// 4. EmptyStateError component must be exported and render the right markers.
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