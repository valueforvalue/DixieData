# Making Code Changes in DixieData

This document is the working contract for changes that touch
multiple layers (templ + htmx + JS + Go handler). It exists because
the chi router migration (PR #1 of the stabilization sprint) and
the htmx-* attribute strip in `app.js` shipped independently and
silently broke every click-driven button across the app. Two halves
of one system drifted apart; the bug surfaced only at runtime.

The patterns below are the ones that prevent drift. Read the
section for the layer you're touching. If your change crosses
layers, read all the relevant sections in order.

## The architecture in one paragraph

DixieData is a Go-rendered templ + HTMX + chi-router app:

- **`internal/templates/*.templ`** are the rendered HTML. They
  declare `hx-get`, `hx-post`, `hx-target`, `hx-swap`, `hx-trigger`
  on `<form>` and `<button>` elements.
- **`internal/htmxattr.Mux`** is the typed builder for htmx
  attributes. Prefer `Mux{ Get, Post, Target, Swap, Trigger,
  Confirm }.Attrs()` over raw attribute strings — it validates
  swap values and warns about ad-hoc target selectors that
  aren't in the uiids registry.
- **`internal/routebuilder.X()`** is the typed builder for URLs.
  Prefer it over raw string literals — there's a goquery guard
  test that flags bare `hx-get="/foo"` strings as a code smell.
- **`frontend/app.js`** owns all network round-trips. It reads
  the `hx-*` attrs from each element and fires `request()` /
  `queueRequest()` itself, instead of letting htmx fire. This is
  to avoid duplicate fetches when both handlers would otherwise
  run on the same event.
- **`internal/appshell/routes.go`** is the chi route table.
  Every `r.Get` / `r.Post` / `r.Put` / `r.Delete` here is
  matched in registration order; specific paths must come before
  wildcards (`/soldiers/search` before `/soldiers/*`).

The drift bug class happens when **two of these layers change in
isolation**. The fix is always the same: introduce or update an
assertion that mechanically catches the drift.

## When you change a route

You touch: `internal/appshell/routes.go` AND `*.templ` files
referencing the URL AND `internal/routebuilder/routebuilder.go`
if it's a new URL.

Checklist:

1. **Choose the right HTTP method.** Read the handler's first
   non-comment statement. If it begins with `if r.Method !=
   http.MethodPost { ... return }`, the route is POST-only.
   The guard test (`internal/appshell/routes_method_guard_test.go`)
   walks the AST and flags mismatches.
2. **Specific paths before wildcards.** If your new route shares
   a prefix with an existing wildcard (`/soldiers/...` next to
   `/soldiers/*`), register the specific one first. The wildcard
   test (`internal/appshell/route_wildcard_test.go`) catches
   re-orderings that violate this.
3. **Add or update the routebuilder.** If templates reference
   this URL via `templ.SafeURL(routebuilder.X())`, update the
   builder to return the new path. If templates still have a
   raw string, migrate it.
4. **Add a route integration test entry.** If the new route is
   POST-only, append the path to `postOnlyPaths` in
   `internal/appshell/route_integration_test.go` so the runtime
   guard covers it.
5. **Run `go test ./internal/appshell -run TestRoute -v`.** All
   three guards should pass.

## When you change a template

You touch: `internal/templates/*.templ` files.

Checklist:

0. **`htmxattr.Mux.Attrs()` returns plain `string` for URL values,
   NOT `templ.SafeURL`.** This is a hard-won finding from the
   stabilization sprint: templ.RenderAttributes' type switch has
   cases for `string`, `*string`, `bool`, etc. but NOT for
   `templ.SafeURL`. When an attribute value is a `SafeURL`,
   `RenderAttributes` silently drops the attribute. Symptom: every
   `hx-get` / `hx-post` button renders without those attrs and
   clicks do nothing. Tests asserting `templ.SafeURL` in the
   value pass — they don't exercise RenderAttributes. The
   smoke test in `audit/smoke.mjs` is the only thing that
   catches this.

1. **Use `htmxattr.Mux` and `routebuilder.X()`** for new
   elements. Don't write `hx-get="/foo"` directly. The goquery
   guard (`internal/templates/hx_guard_test.go`) flags bare
   string literals.
2. **Trigger syntax.** htmx supports `keyup`, `input`, `change`
   events and `delay:`, `throttle:`, `from:`, `changed`
   modifiers. The `changed` modifier is NOT an event — it
   applies to the event before it (`keyup changed` means
   "keyup, but only when value changed"). `input changed
   delay:300ms from:input[name='q']` is the canonical search
   trigger.
3. **hx-target selectors.** Use `uiids.X` for persistent
   targets (panels, regions, modals). Ad-hoc selectors are
   allowed but emit a warning. See the `TestHXTargetsPreferRegistry`
   guard for the current list.
4. **Wrap inputs that need htmx polling in a `<form>`.** htmx's
   `hx-trigger="keyup"` on an `<input>` outside a `<form>` will
   fire but the request has no form data. Without the form,
   the swap target receives nothing meaningful. Bug
   `5b022a7 fix(ui): resolve false 5.7s load time on /soldiers
   by wrapping search in a form (issue #85)` is the precedent.
5. **hx-sync on rapid-fire forms.** If a form submits on every
   keystroke, add `hx-sync="this:replace"` so the new request
   cancels the previous one. Without it, an older response can
   win the race and overwrite newer data. Precedent:
   `e16ae8b fix(ui): hx-sync search form to cancel in-flight
   requests on new keystrokes`.
6. **Run `go test ./internal/templates -v`.** The page-snapshot
   tests verify rendered output shape; the goquery guard
   verifies attribute hygiene.

## When you change app.js

You touch: `frontend/app.js`.

**This file has a structural quirk you must know about.**

At `DOMContentLoaded`, the file strips `hx-get`, `hx-post`,
`hx-put`, `hx-delete`, `hx-trigger`, `hx-confirm`, `hx-include`
from every element in the DOM. The strip exists to prevent
htmx's auto-handler from double-firing alongside app.js's own
`request()` / `queueRequest()` handlers.

**Before stripping, the file caches each attr to a `data-hx-*`
mirror.** All the JS handlers (`getMethod`, `getUrl`, `request`,
`queueRequest`, `triggerInputRequest`) read via the `hxAttr(el,
name)` / `hxHas(el, name)` helpers, which prefer the live attr
and fall back to the mirror.

Checklist:

1. **Use `hxAttr(el, name)` and `hxHas(el, name)` instead of
   `el.getAttribute(name)` / `el.hasAttribute(name)`** for any
   `hx-*` attribute. The originals are stripped at boot. Direct
   reads return `null` / `false`.
2. **Use `[hx-X], [data-hx-X]` in `closest()` selectors.** Same
   reason — the original attr may be stripped.
3. **`triggerInputRequest` matches `keyup`, `input`, and
   `changed`.** If your trigger uses a different event name
   (e.g. `keydown`, `blur`), add it to the regex or your handler
   silently drops the request.
4. **Don't re-introduce the strip.** If you find yourself
   tempted to re-add `el.removeAttribute("hx-X")` somewhere,
   the right answer is to use `e.stopImmediatePropagation()`
   inside the handler instead, so htmx doesn't double-fire.
5. **Parse-check after every edit:** `node -c frontend/app.js`.

## When you extract helpers from .templ to .go

You touch: `internal/templates/*.templ` AND
`internal/templates/*_helpers.go`.

The PRD §PR scope for PR #4 (entry form split) and PR #F2/F3
(soldier_card / share helpers) called for byte-stable
extraction: the rendered HTML must match exactly, so existing
snapshot tests pass unchanged.

Checklist:

1. **Render the same HTML.** Every character matters. Whitespace,
   attribute order, escaping. `templ` is not whitespace-sensitive
   in the same way as JSX, but the goquery snapshot tests check
   HTML byte-by-byte.
2. **Add unit tests for the helper.** The helper is now a
   pure-Go function, so unit-test it directly. The byte-stability
   guarantee means the snapshot tests will catch any drift
   between the templ and the helper, but a unit test on the
   helper alone gives faster feedback during development.
3. **Don't move things that depend on templ syntax.** If your
   helper returns markup that's tightly coupled to templ
   (e.g. conditional selected attribute, list-rendering), keep
   it in the templ file. The soldier_card searchSummary helper
   was kept in `.templ` for this reason (it reads
   `models.EntryTypeLinkedPerson` and friends).
4. **Run `go test ./internal/templates -v`.** All page-snapshot
   tests must pass.

## When you add (or migrate) a fragment-swap action

You touch: `internal/templates/*.templ` (add a templ helper that
emits the wrapper's innerHTML only) AND
`internal/appshell/*.go` (handler returns the helper render, not
a redirect + plain text) AND `internal/appshell/routes.go` (a
dedicated chi route BEFORE the catch-all wildcard when
extracting from `handleSoldierByID` / `handleEventByID`).

This is the architecture event-side #332 / #341 and
soldier-side #391 (Slice B) migrated to. Pre-migration the
soldier detail page used `X-Dixiedata-Redirect: /soldiers/{id}`
on Delete + Set-Primary, forcing a full-page reload every
action. Post-migration those handlers write the
`SoldierImagesListFragment` / `EventImagesListFragment`
directly, htmx swaps the wrapper's innerHTML via
`data-results-target="#panel.{entity}.detail.images"`, the
user never loses scroll position.

Checklist:

1. **Fragment helper is wrapper-inner-only.** The fragment
   helper (`SoldierImagesListFragment`,
   `EventImagesListFragment`) renders the inner content
   only — the `<div id={ uiids.Panel... }>` wrapper lives
   in the page-level template (e.g. `soldier_card.templ`
   for the soldier detail, `event_detail.templ` for the
   event detail). htmx's `data-results-target` swaps the
   wrapper's innerHTML on response, so the fragment must
   contain everything inside the wrapper but exclude the
   wrapper element itself. Mirrors
   `internal/templates/event_panels.templ:93`
   `EventImagesListFragment`.
2. **Per-card Delete + Set-Primary forms carry
   `data-results-target`.** Mirrors
   `internal/templates/event_panels.templ:107` per-card
   Delete — the action button's synthetic form
   (constructed by `frontend/app.js` for bare button
   submitters) inherits `data-results-target` from the
   button so the swap target matches the wrapper's ID.
   For the bulk form, the outer `<form>` (or its
   `<button data-dixie-submit="true">`) gets
   `data-results-target` too so multi-select deletes also
   swap in place instead of stranding the gallery at a
   stale state.
3. **Handler returns the fragment render, not a
   redirect.** Replace `X-Dixiedata-Redirect` + plain text
   body with `renderSoldierImagesListFragment(w, r, id)`
   (or event-side equivalent). Toast header via
   `setToastHeader(w, ...)` carries the user-visible
   confirmation; `savePendingToast()` surfaces it on the
   next page-load tick.
4. **Add a dedicated chi route BEFORE any catch-all.**
   `routes.go` registers the explicit `/soldiers/{id:[0-9]+}/images`
   route before `r.Get("/soldiers/*", a.handleSoldierByID)`.
   chi walks the route table in registration order; the
   explicit regex matches first when both patterns cover
   the same path. Pre-B.1 the path was dispatched from
   the catch-all's `parts[1] == "..."` switch.
5. **Pin the swap in tests + smoke.** Add a Go handler
   test that POSTs the action and asserts:
   - `X-Dixiedata-Redirect` is empty,
   - response body contains the per-card `data-image-card` markers + the wrapper ID,
   - DB row was updated.
   Add smoke probe steps that click the per-card button
   and assert `page.url()` is unchanged after the click
   (the in-place swap guarantee). See
   `audit/smoke_soldier_images.mjs` step-04 + step-05
   (soldier, #391 Slice B.2) and the parity assertion in
   `audit/smoke_events.mjs` step-12 + step-14 (event,
   #391 Slice B.3).

## When you add a new top-level page

You touch: `internal/templates/<page>.templ` AND the page-snapshot
test file AND `internal/appshell/routes.go` (if a new route).

Checklist:

1. **Add a `TestPageSnapshot<Page>` test.** Use the
   `renderIntoDoc` helper in `internal/templates/page_snapshot_test.go`
   to render the page into a buffer, parse with goquery, and
   assert shape invariants (heading present, primary action
   present, no debug-overlay attributes). Five page-snapshot
   tests already exist (Browse, Layout, SoldierDetail,
   EntryForm, JobsStatus) — model yours on those.
2. **Register the route.** Specific before wildcards. Add to
   `postOnlyPaths` if POST-only.
3. **Add a routebuilder.** Templates will reference it via
   `routebuilder.<Page>(args)`.
4. **Add surface IDs to uiids.** Every persistent target (panel,
   region, modal) gets a constant in `internal/uiids/uiids.go`
   with a `Kind` and `Description`. The htmxattr target check
   prefers these over ad-hoc selectors.
5. **Run the full test sweep:** `go test ./... -short`.

## What catches bugs at code-review time

The guard tests catch the following bug classes automatically.
Don't disable them. Don't add `[skip ci]` for them.

| Guard | Test name | Catches |
|---|---|---|
| Boundary | `TestPackageBoundaries` | `internal/records` or `internal/db` accidentally importing Wails |
| Architecture | `TestArchitectureMapsToContract` | Forgotten deep-module package added to architecture map |
| Routes-method | `TestRouteMethodMatchesHandler` | `r.Get` paired with POST-only handler |
| Routes-integration | `TestPostOnlyHandlersRejectGET` | Any runtime path ending in 405 from a misconfigured handler |
| Routes-wildcard | `TestWildcardRoutesDoNotShadowSpecific` | Specific route registered after its sibling wildcard |
| Htmx URLs | `TestHXURLsUseBuilders` | Bare `hx-get="/foo"` string in any template |
| Htmx targets | `TestHXTargetsPreferRegistry` | Ad-hoc `hx-target="#foo"` selectors |
| Page snapshots | `TestPageSnapshot*` | Top-level page missing required structural element |
| Debug-overlay | `assertNoDebugOverlayAttrs` | `data-ui-id` reintroduction (PR #0 removed) |
| Form-contract | `TestFormContractMatchesHandler` (planned: #684) | Form field name / `data-method` / `data-results-target` drift between template and handler |
| Button-actions-resolve | `lint-button-actions-resolve.py` (planned: #687) | Templ `data-action` / `action="..."` resolving to a 404 |
| JS-form-mutation | `lint-no-form-mutation.js` (planned: #687 extended scope) | JS-side `form.action` / `form.method` / `form.enctype` mutation outside `dispatchDixieDataForm`'s synthetic-form branch |
| Nested-form | `lint-no-nested-forms.py` (planned: #682) | `<form>` inside `<form>` in any `.templ` file |
| Embed-tree | `verify-embed-tree.py` (planned: #686) | `frontend/_lib/*.js` files referenced by `index.html` but skipped by Go `//go:embed` |
| Init-guard | `lint-js-init-guards.mjs` (planned: #685) | `__<feature>Wired` idempotency guards missing from `initializeXxx` funcs |
| Wails-PATCH CI | `audit/dispatcher_patch_method.test.mjs` (planned: #683) | Wails-PATCH body-stripping workaround regressions |
| Empty-body dispatch | `lint-no-form-mutation.js` (planned: #687 extended scope, class 9) | `button.closest("form")` outside the form-finding branch — body construction uses raw DOM traversal instead of the resolved form |
| Form-construct-runtime | `[DD DEBUG] raw body` log + smoke probe (planned: #691) | Server-side parse of an empty body — `raw body len=0 body=""` + `parseSoldierForm result firstName="" lastName=""` |

When you add a new guard for a new bug class, **add it to this
table**. Future contributors will need the map.

## What catches bugs at runtime (not yet covered)

These bug classes currently have no automated guard. If you
hit one, add a test before fixing.

- **Background-task crashes** (e.g. bulk PDF crash, per the
  user's report). No test fires an actual Typst invocation.
  Could be added as a smoke test that runs the typst binary
  against a single-record fixture and asserts the output PDF
  exists.
- **Wails-runtime nil-guards** (e.g. handler calls
  `runtime.EventsEmit` without nil-checking). No test mocks
  the runtime. Could be added by introducing a tiny
  `runtimeWails` interface in appshell that has a `MockRuntime`
  impl returning errors on every method.
- **HTMX swap target detach after htmx-swap**. No test renders
  a page, swaps a fragment, and re-queries for the target.
  Could be added to the page-snapshot suite as a
  `TestPageSnapshot<Page>AfterSwap` variant.

## Pre-mortem — "every export crashed the app" (2026-06-27)

**Symptom (as the user reported it):** "I think this actually
fixed the crashes. No crashes however I did find another bug."

The crash was: clicking any export button (soldier PDF, soldier
JPG, screenshot, share-page printable PDF) opened the native
save dialog and then killed the process with `[WebView2 Error]
The parameter is incorrect.` followed by `Failed to unregister
class Chrome_WidgetWin_0. Error = 1412`. Calendar PDF worked.
Standalone native dialogs from the file menu worked. Bulk
exports crashed. Reproducible in 100% of attempts.

**Investigation:**

1. Caught the goroutine stack via
   `scripts/run-crash-dump.ps1` → `build/bin/crashlogs/`.
2. Bottom of every stack was `runtime.main` → `wails.Run` →
   `Frontend.RunMainLoop` → `DispatchMessage` →
   `generalWndProc` → `EventManager.Fire` → `Frontend.onFocus`
   → `Chromium.Focus()` → `controller.MoveFocus(...)` →
   `Chromium.errorCallback` → `os.Exit(1)`.
3. Two `iFileSaveDialog.show` frames appeared in the same
   goroutine (frames 14–17 and 29–32), both originating from
   `showCfdDialog.func1` in `dialog.go:156` (Wails v2.12.0
   internals). The same COM call was being entered twice in
   the same Windows message-loop tick.
4. Hypothesis #1: native `<dialog>` modals (issue #117) caused
   the focus reentry. Reverted the three modals to
   `<div role="dialog" aria-modal="true">` overlays and
   re-tested. **Same crash.** Hypothesis falsified — `<dialog>`
   was a red herring (though still reverted defensively because
   it complicates the focus-event surface).
5. Hypothesis #2: Typst / templates. Rejected by inspecting
   the stack — crash fires before any Typst code runs (it's a
   `MoveFocus` COM error, not a Typst invocation error).
6. Hypothesis #3: the guard pattern from `162c353` was
   incomplete. Re-read that commit's message — it claimed
   "every SaveFileDialog call" was guarded, but missed the
   five call sites in `internal/appshell/app.go` and the
   `exportFullDatabasePDFPath` helper. Added inline guards to
   all five. **Crash gone.**

**Root cause:** Every `a.SaveFileDialog` call from a Go handler
without a guard is one double-click away from killing the
process. The Wails v2.12.0 Windows frontend is the wrong
place to fix this (upstream wailsapp/wails#2807); the
`go-webview2` global error callback is hard-coded to
`os.Exit(1)` which means any COM failure takes the app down.
We can't prevent the focus event chain, but we can prevent
two native dialogs from ever being on screen at the same time.

**Why the previous "fix every export" commit didn't catch
this:** `162c353` introduced `guardedSaveFileDialog` and
routed 9 handlers through it, but it only audited the
exports_handlers.go file. The 5 call sites in
`internal/appshell/app.go` were skipped — they predate the
exports-handler extraction and were never revisited when the
guard was added. Code review of `162c353` should have
required a full repo grep for `SaveFileDialog` and a
line-by-line check that every match was guarded.

**Guardrails added this round:**

1. Inline `a.inFlight.LoadOrStore` + `defer a.inFlight.Delete`
   guards added to `handleSoldierPDF`,
   `handleSoldierPDFNoImages`, `handleSoldierJPG`, and
   `handleImageScreenshot` in `internal/appshell/app.go`.
2. `exportFullDatabasePDFPath` in
   `internal/appshell/exports_handlers.go` returns a new
   `errExportInFlight` sentinel. Both the HTTP handler and
   the Wails binding map it to a 429 / friendly toast.
3. New test `TestExportFullDatabasePDFPathGuardKeys` plus the
   existing 5 assertions in `save_dialog_guard_test.go` lock
   the contract.
4. `CONTEXT.md` "Laws (non-negotiable)" section encodes the
   rule at glossary level.
5. `docs/agents/dialog-guard.md` is the canonical reference
   with the bug history, patterns, table of guarded call
   sites, and instructions for extending coverage.
6. `docs/COMMON_BUGS.md` section 4.10 documents the bug
   class so future bug-hunter scans flag any unguarded
   `SaveFileDialog` / `OpenFileDialog` call site.
7. `docs/COMMON_BUGS.md` section 6.1 was wrong (it told
   readers to use native `<dialog>` for modals); rewritten
   to point at `showOverlayModal` instead.
8. `CHANGELOG.md` Unreleased > Fixed has the user-facing
   description.

**Open follow-ups (not in scope this round):**

- `OpenFileDialog` / `OpenDirectoryDialog` /
  `OpenMultipleFilesDialog` race the same way but don't
  have a guarded helper yet. Today they each carry their own
  ad-hoc guard (or none). Worth a follow-up issue.
- The Wails v2.12.0 / go-webview2 `MoveFocus` race should
  be reported upstream to wailsapp/wails and
  wailsapp/go-webview2 with our crash log. The hard-coded
  `os.Exit(1)` in `errorCallback` is hostile — at minimum
  it should be configurable so apps can recover instead of
  die.

## When in doubt

The architecture intentionally has belt-and-braces guards at
every layer:

- AST-level guard (catches structural issues at compile time)
- Page-snapshot test (catches rendering drift)
- Integration test (catches runtime behavior)

If your change crosses a layer, **add a guard for the next layer
up**. The chi migration needed three guards (AST, integration,
wildcard) because the migration touched three layers (routes.go,
handlers, templates). The htmx attr strip needed the data-hx-*
mirror because the strip touched both the JS strip and the JS
readers.

The pattern is: **every layer transition is a place bugs hide.
Add an assertion at each one.**
## 8-class button-bug audit (2026-07-28)

**Symptom (then-current state):** the original Save Changes button
failure on `/soldiers/{id}/edit` (issue #676, the Wails-PATCH
body-stripping bug) re-surfaced after the 3896b46b + a18f5e1f
fixes; the data-method elimination (commit `92fb264c`) didn't
fully resolve it. The user applied two band-aids while chasing a
nested-form hypothesis (issue #682):

1. `id="entry-edit-form"` on the outer form + `form="entry-edit-form"`
   on the Save button.
2. `button.form` fallback in `dispatchDixieDataForm` to associate
   the button with the truncated outer form via the HTML5 `form`
   attribute.

These band-aids masked the bug class but revealed a third one:
`syncEntryTypeFields` (frontend/app.js:3946) was unconditionally
assigning `form.action = "/soldiers"` on every
`initializeDynamicContent` pass, clobbering the edit URL
`/soldiers/{id}` set by the server. The save dispatched to the
create URL, the create handler returned 400, the user saw no
toast.

**Investigation:** the diagnostic session ran four discrete layers
of debugging:

Layer 1 — server routing: confirmed working. `handleSoldierByID`
dispatches POST to `handleUpdateSoldier` correctly. (Added a
[DD DEBUG] log to confirm.)

Layer 2 — nested form: confirmed root cause for the *form-finding*
failure (button has no `<form>` ancestor in the rendered DOM
after the HTML5 parser auto-closes the outer form at the inner
form's open tag). Live instances in `entry_form.templ` and
`soldier_card.templ`.

Layer 3 — band-aids: applied `id="entry-edit-form"` +
`form="entry-edit-form"` + `button.form` fallback. The Save
button now finds the truncated outer form via the HTML5 `form`
attribute. The dispatch fires.

Layer 4 — wrong URL: the truncated outer form has its
server-rendered `action` attribute preserved (the HTML5 parser
keeps opening-tag attributes on auto-closed forms), but JS-side
`syncEntryTypeFields` overwrites `form.action` to `/soldiers`
on every init pass. The 400 from the create handler is the
visible symptom.

**The 8-class taxonomy:**

The audit catalogued 12 button-failure fixes across the
2026-06 → 2026-07 window into 7 distinct bug classes. The
diagnostic session surfaced an 8th. Children:

1. WebView2 body-stripping (5 fixes)
2. Dispatcher/form contract drift (5 fixes)
3. htmx swap re-binding (2 fixes)
4. Nested `<form>` rendering defect — NEW: 2 live instances
5. Embed asset skip (`_`/`.` prefix) — 1 fix
6. Template split / mega-menu refactor — 2 fixes
7. JS silent failure — 1 fix
8. JS-side form-action mutation — NEW: `syncEntryTypeFields`
9. Empty-body dispatch — NEW: body construction uses raw `closest()` instead of the resolved form

**Detection gates** (filed as children of #681):

- #682 — `lint-no-nested-forms.py` (class 4)
- #683 — `audit/dispatcher_patch_method.test.mjs` CI gate (class 1)
- #684 — `TestFormContractMatchesHandler` (class 2)
- #685 — `lint-js-init-guards.mjs` (class 3)
- #686 — `make verify-embed-tree` (class 5)
- #687 — `lint-button-actions-resolve.py` + `lint-no-form-mutation.js`
  (class 6 + class 8)
- #688 — `frontend/lib/dixie-debug.js` + `/debug/client-logs` ingest
  (class 7)
- #689 — `fix(frontend): syncEntryTypeFields` clobber (class 8)
- #691 — `fix(frontend): body construction uses resolved form` (class 9, release-blocker)

The class-9 lint scope extends the existing `lint-no-form-mutation.js`
(planned in #687) to catch `button.closest("form")` outside the
form-finding branch. The scope extension is small (~30 LoC) and
shares the parser plumbing with the existing form.action mutation
rule.

**The lesson:** when a fix lands as a band-aid (the
`id="entry-edit-form"` workaround), the next investigation must
ask "what does the band-aid actually mask?" The diagnostic session
correctly identified that the band-aid masked a nested-form
defect (true) but incorrectly attributed the failure to it. The
real cause was a JS-side mutation that the band-aids accidentally
revealed. A band-aid that accidentally makes the next layer of
the bug visible is doing more work than a band-aid should.

**The corrective discipline:** when a band-aid lands, follow it
with a "what could this be hiding?" investigation. The
diagnostic session would have been shorter if the user had
asked that question after Layer 3 instead of after Layer 4.

**The detection discipline:** every bug class needs a gate that
fails at PR time. The 9-class taxonomy above has 9 gates. The
gates are intentionally small (50-100 LoC each) and targeted.
A `make audit` suite that invokes all 9 in CI is the final
regression net.

**The class-9 follow-up lesson (2026-07-28, second diagnostic session):**

After the #682 / #689 fixes landed, the user reported "Save
Changes wipes out all of the existing data for a record." The
user had to use a new soldier ID each test because the previous
soldier was wiped. Investigation surfaced a server log entry
that nailed the cause:

```
[DD DEBUG] raw body len=0 body=""
[DD DEBUG] parseSoldierForm result firstName="" lastName="" displayID="" err=<nil>
```

The body was empty. The fix found at `frontend/app.js:5206`
(the body-construction branch) used `button.closest("form")`
which returns `null` for the reparented Save button. The
form-finding branch (line 5110) correctly applied the
`closest() || button.form` fallback, but the body-construction
branch didn't.

**The second-order lesson:** a band-aid that fixes one branch
can mask a sibling branch. The form-finding branch's correct
fallback resolved the form, which made the body-construction
branch's missing fallback visible: the form was found, but the
body was empty. **Every place that needs a form reference must
apply the same fallback, not just the first one.** The
corrected rule: extract the fallback into a helper at the top of
`dispatchDixieDataForm` and use it everywhere a form reference
is needed.

**The diagnostic discipline that surfaced this:** the user
added a `[DD DEBUG] raw body` log to `handleUpdateSoldier` that
dumped the request body length and the first 200 chars. Without
that log, the empty body would have been invisible — the
server-side parse would have returned an empty `models.Soldier`
silently, and `Update` would have written empty values without
an error. The log made the empty body visible. **Server-side
debug logs that surface the actual request body are
non-negotiable for any handler that processes form data.** The
prototype is at `internal/appshell/soldiers_handlers.go:638`;
the pattern should be added to every handler that calls
`ParseForm` or `ParseMultipartForm`.

**Related:**

- #681 (umbrella)
- #682, #683, #684, #685, #686, #687, #688, #689, #691 (children)
- `docs/COMMON_BUGS.md` §2.7 (class 4), §3.9 (class 8), §3.10 (class 9)
- `docs/agents/bug-pattern-grep.md` entries 11, 12, 13
- `docs/ui-map/wireframes/26-event-new.md` and
  `docs/ui-map/wireframes/27-event-edit.md` (extended cross-references)
- `AGENTS.md` "Form-attribute mutation hazard",
  "Nested-form hazard", and "Empty-body dispatch hazard" sections
- `CHANGELOG.md` [Unreleased] Maintenance block
- `frontend/app.js:5110` (form-finding branch with the correct fallback)
- `frontend/app.js:5206` (body-construction branch missing the fallback)
- `internal/appshell/soldiers_handlers.go:638` (`[DD DEBUG] raw body` log — the diagnostic primitive that surfaced this bug)
