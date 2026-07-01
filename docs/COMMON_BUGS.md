# Common Bugs — DixieData Working Guide

This document captures the recurring bug patterns from DixieData's
commit history (79 `fix:` commits across 408 commits). Each pattern
includes the symptom, how to find it, and the fix. Use this as a
checklist when reviewing changes or hunting regressions.

The patterns are grouped by where the bug lives in the stack:
**HTMX wiring** (templ + htmx attributes), **`.templ` markup**,
**Frontend JS** (`frontend/app.js` + `app.css`), **Go backend**
(`internal/appshell`), **Typst PDF templates**, and
**Accessibility**. A final section covers debugging workflow.

---

## 1. HTMX wiring bugs

These are the highest-frequency bug class in the codebase
(~8 of 79 fix commits). They all share a pattern: the markup looks
right but the request never fires, or fires twice, or fires against
a stale target.

### 1.13 Pre-mux placeholder cascades into an infinite layout stack

**Symptom:** During the brief window between the Wails process
starting and the chi router being ready, the layout's polling
fragments (`/jobs/active` every 3s, `/layout/review-count` every
30s, `/jobs/{id}/status` while jobs run) all return a full HTML
document — the startup placeholder from
`renderStartupPlaceholder`. htmx innerHTML-swaps that into the
small target region. The placeholder body carries
`hx-get="..." hx-trigger="load delay:700ms" hx-target="body"
hx-swap="outerHTML"`, so htmx processes the inner body's
triggers, fires a GET, and — if mux is still nil — receives
another placeholder, outerHTML-swaps it onto `<body>`, and the
new body's own triggers fire 700ms later. Each cycle stacks a
fresh `<div class="app-shell">` inside the previous one. After
~30s the user sees 77 nested copies of the entire layout, 738KB
of body innerHTML, and a diagonal cascade of `<header class="top-shell">`
cards across the screen. Scrollbars shrink toward zero until the
system runs out of memory.

**Why it happens:** `App.ServeHTTP` falls through to
`renderStartupPlaceholder` whenever `a.mux == nil`. The placeholder
is a full HTML doc, not a fragment, because the Wails assetServer
sometimes receives requests before the chi router has been
mounted. The placeholder's body attributes were added when the
shell page (frontend/index.html) was modeled — the assumption
was that the placeholder would be replaced by the layout
response within one htmx cycle, so the body triggers would fire
once and stop. That assumption breaks when mux is nil because
the next request also returns the placeholder.

**Find it:**
```bash
grep -n 'hx-get\|hx-trigger\|hx-target\|hx-swap' internal/appshell/app.go
```
The placeholder's HTML body must contain none of these. Search
for the same attrs on `<body>` in any other code path that
returns a full HTML document during startup.

**Fix:** Two-part, in `renderStartupPlaceholder`:
- (a) Detect htmx fragment requests via the `HX-Request` header
  and return `204 No Content`. Fragment polls become harmless
  no-ops during the pre-mux window. (The Layout's `setupRequestAllowed`
  allowlist already exempts `/jobs/active` and
  `/jobs/{id}/status` from the setup-required 303 redirect, but
  that allowlist is irrelevant when `a.mux == nil` because
  `ServeHTTP` short-circuits to the placeholder BEFORE the
  allowlist check runs.)
- (b) Drop the `hx-get` / `hx-trigger` / `hx-target` / `hx-swap`
  attributes from the placeholder's `<body>`. The meta refresh
  header and the inline `window.location.replace` script already
  cover the full-page retry mechanism; the htmx body trigger adds
  no value but creates the cascade. (a) handles the fragment-poll
  path; (b) handles the initial `/` load and any meta-refresh
  fallbacks.

**Regression net:** two tests in `internal/appshell/app_test.go`:
- `TestRenderStartupPlaceholderReturns204ForHtmxFragmentRequests` —
  a 204 No Content with empty body when the request carries
  `HX-Request: true`.
- `TestAppServeHTTPStartupPlaceholderAutoRefreshesWithoutMux` — the
  existing placeholder test grew a new block that asserts none of
  the four htmx trigger attrs appear in the placeholder body,
  with the cascade bug named in the failure message.

**Real example:** captured during the issue #180 follow-up. Artifacts:
`uibug.png` and `uibug2.png` in repo root, showing the diagonal
cascade of the layout chrome before the fix landed.

---

### 1.14 Polling fragment cascades during setup / recovery / startupErr blocks

**Symptom:** During any "blocked" state (setup-required, pending
recovery, fatal startup error), polling fragments that aren't in
the corresponding allowlist (`setupRequestAllowed`,
`recoveryRequestAllowed`) get redirected (303) or errored (500)
on every poll. The browser's XHR follows the 303 to the block
page (`/setup` or `/recovery` — both full HTML documents), and
htmx innerHTML-swaps that doc into the badge wrapper
(`hx-target="this"`). After ~30s the badge wrapper's innerHTML
is a copy of the setup or recovery form. Same class as §1.13
but via the post-mux blocked-state branches. The startupErr
variant is slightly different: the response is `text/plain` 500
with the raw Go error message, not a full HTML doc, so the
cascade doesn't stack — but the badge shows the error text and
htmx logs the 500 to the console.

**Why it happens:** Every blocked-state branch in
`App.ServeHTTP` (`setupRequired`, `pendingRecovery`, `startupErr`)
returns a redirect or error unconditionally. The pre-mux window
is handled (§1.13) but the post-mux blocked-state branches were
not.

**Find it:** Search the blocked branches in
`internal/appshell/lifecycle.go` for `http.Redirect` or
`http.Error` calls that fire before any `HX-Request` check:

```bash
grep -n 'http.Redirect\|http.Error' internal/appshell/lifecycle.go
```

Every block branch must check `r.Header.Get("HX-Request") == "true"`
and return `204 + X-DixieData-Redirect` before falling through to
the redirect/error.

**Fix:** Same shape as #212 and #214: detect `HX-Request: true`
in each blocked branch and return `204 No Content` with
`X-DixieData-Redirect` pointing at the destination page
(`/setup`, `/recovery`). Full-page nav still gets the 303 / 500.

**Regression net:** one test per blocked branch in
`internal/appshell/app_test.go`:
- `TestAppServeHTTPSetupRequiredFragmentReturns204WithRedirectHint` (5 cases)
- `TestAppServeHTTPRecoveryFragmentReturns204WithRedirectHint` (6 cases, incl. priority over setupRequired)
- `TestAppServeHTTPStartupErrFragmentReturns204WithRedirectHint` (4 cases)

Each test asserts: (a) fragment gets 204 + correct redirect hint,
(b) full-page nav still gets the existing 303 / 500, (c) sanity
check that allowlisted paths pass through to the real handler.

**Real example:** captured during the issue #212 follow-up. The
user saw the badge wrapper's innerHTML become a copy of the
setup form on every poll. Issues #212 (setup), #214 (recovery +
startupErr) closed with the fix shape above.

---

### 1.1 Button doesn't submit because `type="submit"` is missing

**Symptom:** User clicks button. Nothing happens. Network tab shows
no request.

**Why it happens:** HTML default for `<button>` inside a `<form>` is
`type="submit"`, but `<button>` outside a `<form>` defaults to
nothing. If a template author copies a button outside its original
form, or if htmx swap removes the form wrapper, the button silently
stops working.

**Find it:**
```bash
grep -rn '<button ' internal/templates/*.templ | grep -v 'type='
```
Every `<button>` should declare `type` explicitly.

**Fix:** Add `type="submit"` to the button, OR wrap the form around it
explicitly.

**Real example:** `cf19dfc fix(ui): calendar + insights PDF buttons now
submit their forms`.

---

### 1.2 Form inputs not wired to htmx polling because not inside a `<form>`

**Symptom:** User types in a search input. Results don't refresh.
Initial page load shows the search input but no polling fires.

**Why it happens:** htmx's `hx-trigger="keyup"` needs to be inside a
`<form>` so the submit event is intercepted; without the form
wrapper, the attribute fires but the request has nothing to bind to.

**Find it:**
```bash
grep -B 1 -A 2 'hx-trigger="keyup' internal/templates/*.templ
```
Look for an `<input>` with `hx-trigger` whose nearest ancestor is
not a `<form>`.

**Fix:** Wrap the input in `<form hx-get="..." hx-trigger="...">`.

**Real example:** `5b022a7 fix(ui): resolve false 5.7s load time on
/soldiers by wrapping search in a form (issue #85)`.

---

### 1.3 htmx-swap destroys JS event listeners

**Symptom:** Click handler works on first page load. After any
htmx navigation (e.g. clicking a pagination link that swaps the
list area), the handler is dead. Toggling the floating nav, opening
a modal, etc.

**Why it happens:** `addEventListener` binds at DOM-creation time.
When htmx replaces the parent element, the listener is GC'd along
with the removed node. Common with `app.js` bootstrappers that run
once on document load.

**Find it:**
```bash
grep -rn "addEventListener" frontend/app.js
```
Every `addEventListener` is a candidate. Check what triggers it
(`DOMContentLoaded`?) and whether the parent survives htmx swaps.

**Fix:** Use `onclick="..."` inline attributes that survive swap, OR
delegate the listener to a stable ancestor (e.g. `document`).

**Real example:** `46a10cc fix(ui): bind floating-nav toggle via
inline onclick, not addEventListener`.

---

### 1.4 In-flight requests race with newer ones

**Symptom:** User types fast. Old search results replace newer ones.
Click a pagination link while a previous request is still pending;
wrong page renders.

**Why it happens:** Without `hx-sync`, htmx fires a new request on
every keystroke but doesn't cancel the previous one. Whichever
responds last wins.

**Find it:**
```bash
grep -rn 'hx-trigger="keyup' internal/templates/*.templ | grep -v "hx-sync"
```
Every keyup-driven form should have `hx-sync="this:replace"` (or
similar) to cancel in-flight requests.

**Fix:** Add `hx-sync="this:replace"` to the form. Or
`hx-sync="this:abort"` to fully abort.

**Real example:** `e16ae8b fix(ui): hx-sync search form to cancel
in-flight requests on new keystrokes`.

---

### 1.5 Polling fragment stops refreshing after job completes

**Symptom:** Progress bar updates every 2s while job runs. After
job completes, polling doesn't stop — it keeps firing against a
terminal-status job.

**Why it happens:** The polling fragment needs a conditional
`hx-trigger`. When the job hits a terminal state, the fragment
should switch to `hx-trigger="none"`.

**Find it:**
```bash
grep -B 2 -A 8 'JobStatusFragment' internal/templates/jobs.templ
```
Check that the templ block has an `if Status == StatusDone` branch
that sets `hx-trigger="none"`.

**Fix:** Add the conditional trigger in the templ block.

**Real example:** `e3cd413 fix(jobs): repair htmx polling + remove
unused SSE endpoint`.

---

### 1.6 Double-click produces duplicate async work

**Symptom:** User double-clicks "Export PDF". Two exports run. Toast
shows success twice. Both jobs hit the queue.

**Why it happens:** No debounce on the button. htmx doesn't
inherently dedupe rapid clicks; the handler runs for each request.

**Find it:**
```bash
grep -rln 'hx-post="/export/' internal/templates/*.templ
```
Export buttons are the most common offender.

**Fix:** Add a debounce in the Go handler (check job registry for
in-flight job with same kind + params), OR add a JS click guard
that disables the button after first click.

**Real example:** `55414e0 fix(appshell): dedup rapid double-clicks
on calendar PDF export`.

---

### 1.7 Redirect loop on certain routes

**Symptom:** Browser hangs loading `/jobs/...`. Network tab shows
302 → 302 → 302 chain.

**Why it happens:** Middleware redirects a route to itself. Common
when `/jobs/*` is wildcard-matched but the middleware matches too
broadly.

**Find it:** Look at the redirect middleware logic in
`internal/appshell/lifecycle.go`. Walk the matching chain.

**Fix:** Tighten the route pattern OR add a guard condition to skip
the redirect for already-redirected paths.

**Real example:** `0551fbb fix(appshell): stop setup-redirect loop on
/jobs/* + nil-guard jobs handlers`.

---

### 1.8 htmx swap target doesn't exist or is detached

**Symptom:** Click fires. Request succeeds. Nothing visible changes.
No error in console.

**Why it happens:** `hx-target="#some-id"` references an ID that
either doesn't exist in the rendered HTML, or was detached by a
prior swap.

**Find it:**
```bash
grep -rn 'hx-target="#' internal/templates/*.templ
```
For each, verify the target ID exists in the rendered output of the
endpoint being called. Run `make dev`, open DevTools, click the
trigger, watch the response — does the target ID still exist?

**Fix:** Either render the target in the endpoint response, or
change `hx-target` to a stable parent like `[data-jobs-progress-region]`
(the `uiids.OverlayJobsProgress` global overlay; see
`internal/templates/components/conventions.md` for the canonical
naming).

**Defensive tooling:** `internal/templates/hx_guard_test.go`
warns about ad-hoc `#id` selectors that aren't in the
`internal/uiids.Registry`. Promote them if they're durable.

---

### 1.12 Polling fragment wipes the whole layout because `hx-target` inherits from `<body>`

**Symptom:** Page loads briefly with the full layout, then blanks
out. Only a small `<span>` remains in the top-left where the layout
chrome used to be. Body's child list shrinks from ~10 nodes to 1.
Console is clean; no JS error.

**Why it happens:** `frontend/index.html` declared
`<body hx-get="/calendar" hx-trigger="load" hx-target="body"
hx-swap="outerHTML">`. The htmx swap replaces body's children
with the layout response (header, nav, main, dock), but leaves
the `<body>` element itself in place — so the body retains its
`hx-target="body"` attribute after the swap.

Inside the layout is a polling fragment like
`<span data-layout-review-count hx-get="/layout/review-count"
hx-trigger="load, every 30s" hx-swap="innerHTML">`. When this
span enters the DOM during the swap, htmx walks up the tree to
resolve its `hx-target`. Default `hx-target` for an unboosted
element is the element itself, but htmx's `getClosestAttributeValue`
finds the inherited `hx-target="body"` on the parent `<body>`
first and uses that.

When the polling response comes back, htmx does an innerHTML swap
against `<body>`. The badge fragment replaces everything inside
`<body>`, leaving only the badge.

**Find it:**
```bash
grep -rn 'hx-trigger' internal/templates/*.templ | grep -v 'hx-target'
```
Every polling element must declare `hx-target="this"` (or a stable
ID) so it does not inherit from `<body>`.

**Fix:** Add `hx-target="this"` to the polling wrapper. The wrapper
itself becomes the swap target; the badge fragment fills its
innerHTML. The rest of the layout stays put.

**Regression net:** `internal/templates/layout_test.go` →
`TestLayoutReviewCountBadgeTargetsItself` pins the contract on
the rendered output: the badge wrapper open tag must contain
`hx-target="this"` AND `hx-swap="innerHTML"`. Test fails with the
exact symptom (showing the wrapper without the attribute).

**Real example:** introduced in `89372a2 feat(ui): review-queue
count badge via htmx poll (issue #180)`. Discovered via
`git bisect`: app boots normally on `87a18c9`, broken on `38b5c14`,
so the suspect window was 27 commits. Bisection to a single
commit + Playwright reproduction against a minimal mirror server
isolated the swap target as the cause.

---

### 1.9 [REGRESSION-PRONE] POST-then-navigate handler must use Option C contract

The original §1.9 documented the "export options status pages not
landing" bug class: handlers wrote 303 + Location but the browser
+ custom dispatcher didn't navigate. The legacy fix was to add
`HX-Redirect` (also dead code; htmx was never the dispatcher).

**Status as of Option C:** Contract stabilized but regressions
continue to ship. The custom dispatcher (`dispatchDixieDataForm`
in `frontend/app.js`) replaces the legacy parallel `request()`
function. Handlers now return `200 OK + X-DixieData-Redirect`,
the dispatcher reads the header, and `window.location.assign()`
navigates the user. Templates use `data-dixie-submit` + `action=`
(instead of `hx-post` / `hx-put` / `hx-delete` for click-driven
forms). The static-archive export opts into
`enqueueExportOpt{NativeRedirect: true}` to keep the 303 + Location
path because it's reached by a plain `<form method="post">` that
the browser follows natively.

**BUT** new handlers keep forgetting the contract. See §1.10
`redirect-contract-drift` for the recurring failure mode and
recipe.

**Regression nets** that would catch any reintroduction:
- `internal/appshell/redirect_headers_test.go::TestPostThenNavigateUsesDixieRedirect` — source-scan guard fails any handler writing 303 + HX-Redirect without X-DixieData-Redirect.
- `internal/templates/hx_guard_test.go` — enforces that hx-get / hx-post URLs come from `routebuilder.X()` builders (defense in depth against the URL-drift class that caused §1.9's historical bugs).
- `audit/smoke.mjs` `share-${btn.path}-navigates-to-jobs` — asserts `page.url()` after click, not just response headers.

See `internal/templates/components/conventions.md` §"Buttons
that POST and expect navigation" for the canonical recipe.

### 1.10 `redirect-contract-drift` — handlers keep forgetting `X-DixieData-Redirect`

**Symptom:** User clicks a button (save feedback, import backup,
import shared archive, export single record PDF, export insights
PDF, etc.). The handler runs (side effect happens — file is
written, record is saved, job is enqueued), the response headers
include `X-DixieData-Toast: "..."` (so the user *thinks* it
worked), but the page never navigates. The user is stuck on the
form page and re-clicks, producing duplicate work.

**Why it happens:** Two flavours:

1. *Handler-side*: a new POST handler returns 200 + toast header
   but forgets to call `writeExportRedirect(w, "/jobs/{id}")` or
   set `X-DixieData-Redirect` directly. The dispatcher's toast
   branch fires, but the redirect branch does not → user stays
   on the form page.
2. *Templ-side*: a new form uses `hx-post=` + `hx-swap="none"`
   instead of `data-dixie-submit` + `action=` + `method="post"`.
   htmx fires the POST and ignores the response headers entirely
   (htmx is not the dispatcher); the `X-DixieData-*` headers are
   dropped on the floor. This is the bug that hid the feedback
   save confirmation until the fix landed.

**Find it:**
```bash
# All 303/redirect writers in appshell that bypass writeExportRedirect
grep -rn 'StatusSeeOther\|http\.Redirect' internal/appshell/*.go \
  | grep -v _test.go | grep -v writeExportRedirect

# All 200-response handlers that set X-DixieData-Toast
# without setting X-DixieData-Redirect (for click-driven flows)
grep -rn 'X-DixieData-Toast' internal/appshell/*.go \
  | grep -v _test.go | while read line; do
    file=$(echo "$line" | cut -d: -f1)
    if ! grep -q 'X-DixieData-Redirect' "$file"; then
      echo "$line  # ← no X-DixieData-Redirect in same file"
    fi
  done

# Templates shipping hx-post + hx-swap="none" (pre-Option-C residue)
grep -rln 'hx-swap="none"' internal/templates/ | xargs grep -l 'hx-post='
```

**Fix:**
- Handler side: call `writeExportRedirect(w, routebuilder.X(...))`
  (or set the header directly) before writing the response body.
- Templ side: replace `hx-post` + `hx-swap="none"` with
  `action={ templ.SafeURL(routebuilder.X(...)) }` +
  `method="post"` + `data-dixie-submit`. Use the canonical recipe
  in `internal/templates/components/conventions.md`.

**Real examples:**
- `3612dab` — original POST-then-navigate bug, led to Option C
  dispatcher
- `4f561e6` — swept 13 handlers that forgot `X-DixieData-Redirect`
- `11f1c01` — swept 5 more sites the previous sweep missed
- `e5a7909` — replaced htmx-clone dispatcher with
  `dispatchDixieDataForm`
- `7a80f6d` (this session) — feedback form was htmx-only, headers
  dropped, no confirmation

**Regression nets:**
- `appshell.TestAll303sWriteHXRedirect` (already exists)
- `appshell.TestPostThenNavigateUsesDixieRedirect` (already exists)
- `appshell.TestXxxHandlersWriteDixieRedirect` (per-handler suite,
  grows as new handlers are added)
- `audit/smoke.mjs` `share-${btn.path}-navigates-to-jobs` +
  `[7d] feedback-save-*` block

### 1.11 `htmx-attr-strip-by-boot-js` — form's `data-dixie-submit` is required for the dispatcher

**Symptom:** A form submits, the network request fires, the
server runs, but the dispatcher (`dispatchDixieDataForm` in
`frontend/app.js`) never reads the response headers, so the
toast doesn't display, the modal doesn't close, the form doesn't
clear. The handler side is correct; the JS post-response path is
bypassed entirely.

**Why it happens:** `frontend/app.js` registers a single
`document.addEventListener("submit", ...)` listener that filters
by `event.target.matches("[data-dixie-submit]")`. Forms without
`data-dixie-submit` are ignored by the dispatcher. htmx has its
own submit listener that fires the POST and processes the
response (with `hx-swap="none"` it just discards the body and
any custom headers). The `X-DixieData-*` contract is only
honoured by the dispatcher.

**Find it:**
```bash
# Templ forms with hx-post/hx-get but no data-dixie-submit
# (pre-Option-C residue or forgotten opt-in)
for f in $(grep -rl 'hx-post=\|hx-get=' internal/templates/ --include="*.templ"); do
  if ! grep -q 'data-dixie-submit' "$f"; then
    echo "$f"
  fi
done

# The single boot-JS submit listener + strip loop
grep -n 'addEventListener.*"submit"\|matches.*data-dixie-submit\|removeAttribute.*hx-' frontend/app.js
```

**Fix:** Add `data-dixie-submit` to the form element. If the
form has htmx-attrs (`hx-post` etc.) but no native `action=` /
`method="post"`, also retag to use `action=` + `method="post"`
+ `data-dixie-submit` (the dispatcher reads `form.action`, not
`hx-post`).

**Real examples:**
- `e5a7909` — Option C dispatcher landed; `data-dixie-submit`
  became the new opt-in
- `7a80f6d` (this session) — feedback form had htmx-attrs only,
  was missed by the templ retag sweep

---

## 2. `.templ` markup bugs

### 2.1 Typst template fidelity — matching the legacy fpdf renderer

**Symptom:** PDF exports look subtly different from the old
fpdf-produced PDFs. Typography, alignment, page breaks, N/A
display.

**Why it happens:** DixieData migrated from fpdf to typst for PDF
rendering. The typst templates were reverse-engineered to match
fpdf's output but edge cases keep surfacing.

**Find it:** Compare a current export against a known-good old PDF
(fpdf-era baseline if available). Look at:
- Date formatting (leading zeros, "00" sentinel, partial dates)
- N/A handling (lowercase "n/a", uppercase "N/A", "Not Recorded")
- Typography (font choice, weight, alignment)
- Page breaks

**Fix:** Edit the relevant `.typ` file in `templates/` (typst
sources). Most fixes are 1-5 lines.

**Real examples:**
- `9046afb fix(templates): match fpdf typography (Times title, Helvetica body); N/A normalization`
- `799fd73 fix(templates): true two-column label/value grid; left-aligned title`
- `7fd9577 fix(templates): respect options.orientation for landscape page size`
- `118e03b fix(templates): strip leading zeros from day in long-date`
- `2603047 fix(templates): handle '00' date sentinel; add prefix to title block`
- `500f936 fix(templates): hide empty fields properly in soldier_landscape`

---

### 2.2 Stray syntax in templ output

**Symptom:** Page renders with a literal `\_ = summary` showing on
screen, or other templ syntax artifacts.

**Why it happens:** Typos in templ code, or copy-paste of Go syntax
into a templ block.

**Find it:**
```bash
grep -rn '\\_ =\|@_' internal/templates/*.templ
```

**Fix:** Correct the templ syntax. Common mistake: writing `\_ = foo`
in a templ block (templ doesn't support Go statements outside
expressions).

**Real example:** `a3620ce fix(templates): drop stray '\_ = summary'
in calendar grid`.

---

### 2.3 Date/timezone handling in display

**Symptom:** Anniversary shows on the wrong day, or birthday shows
a day early/late.

**Why it happens:** Date construction without explicit timezone.
`time.Date(...)` defaults to UTC; anniversary candidates need to
be built in the caller's local timezone.

**Find it:**
```bash
grep -rn "time.Date\|time.Now" internal/calendar/ internal/dates/
```

**Fix:** Pass `time.Local` or use the user's stored timezone offset.

**Real example:** `4d5e517 fix(calendar): build anniversary
candidate in caller's timezone`.

---

### 2.4 `00` day sentinel handling

**Symptom:** Birth date "1923-00-00" renders as "January 0, 1923" or
"N/A" instead of just "1923".

**Why it happens:** Many Civil War records have partial dates
recorded with day=00 meaning "unknown day". The formatters don't
always skip the day gracefully.

**Fix:** Detect `day==0` and render year-only.

**Real example:** `2603047 fix(templates): handle '00' date
sentinel; add prefix to title block`.

---

### 2.5 PDF output shape — bulk export wrong shape

**Symptom:** Bulk export produces a folder of PDFs instead of one
combined PDF. Or vice versa.

**Why it happens:** The export service has multiple paths and the
shape contract was unclear.

**Find it:** Trace `internal/appshell/exports_handlers.go` and
`internal/exportcontract/`. Look at which path the bulk button hits.

**Fix:** Make the bulk endpoint always emit a single sorted PDF.
Drop the folder-of-PDFs code path.

**Real example:** `75afe81 fix(export): bulk export emits single
sorted PDF instead of folder of PDFs (issue #64)`.

---

## 3. Frontend JS / CSS bugs

### 3.1 Floating dock overlap / mobile overflow

**Symptom:** On mobile, content is cut off at 6px. Floating dock
overlaps the bottom content area.

**Why it happens:** Layout chrome wasn't tested at narrow viewports.

**Find it:** Open DevTools responsive mode, set width to 375px
(iPhone SE), scroll the page.

**Fix:** Cap top-shell width; add bottom padding to main content
to clear the dock.

**Real examples:**
- `2bb78d7 fix(ui): cap top-shell width so 6px mobile overflow disappears (issue #78)`
- `f439a20 fix(ui): relocate floating dock so it doesn't overlap content (issue #76)`

---

### 3.2 Stale data after async response

**Symptom:** User clicks "Load Backup". Backup loads. The shared
status panel still shows the old archive contents.

**Why it happens:** htmx swapped a fragment that didn't include the
panel, OR the handler returned 200 but didn't trigger a refresh.

**Find it:**
```bash
grep -rln 'Load Backup\|load.*backup' frontend/app.js internal/templates/
```

**Fix:** Either include the panel in the swap target, OR add an
explicit htmx trigger after the load completes.

**Real examples:**
- `7292f4f fix(share): Load Backup button updates the shared status panel`
- `2b492ee fix(ui): scroll shared status panel into view after import response`

---

### 3.3 htmx not loaded on every page

**Symptom:** Some pages silently drop htmx behavior because
`htmx.min.js` wasn't loaded.

**Why it happens:** Template loads htmx conditionally based on path
or feature flag. When a new page is added, the loading condition
misses it.

**Find it:** Check `internal/templates/layout.templ` for the
`<script src="/htmx.min.js">` tag. Verify it loads on every page.

**Fix:** Load htmx unconditionally in layout.

**Real example:** `3f75356 fix(web): load htmx on every page; JS
handlers own all network round-trips`.

### 3.4 JS submit interceptor bypasses htmx redirect

**Symptom:** A form has `hx-post` (so it posts via htmx) AND a
`submit` event listener that calls `event.preventDefault()`. The
listener dispatches the work itself — typically by calling a
Wails bridge method, which returns inline HTML markup that the
listener injects into a status panel. The form's `HX-Redirect`
header from the server is NEVER honored because htmx never sees
the response: the JS path owns the entire submit cycle.

User clicks "Generate Printable PDF" on `/share`. Modal closes.
Status text dumps into `#share-status`. No `/jobs/{id}` page ever
lands. User has no idea where the export went.

**Why it happens:** The Wails bridge was added to a form that
already had `hx-post`. The bridge path was simpler to write than
threading the job through HTTP and reading the redirect. The
developer kept both paths as a "fallback": JS calls bridge if
available, else calls `request(form)` to defer to htmx. The JS
path was the one that actually ran in production.

**Find it:**

```bash
# Every form with a submit listener that calls preventDefault
grep -rn 'addEventListener.*"submit"' frontend/app.js
grep -rn 'submitPrintConfig\|submitExport\|onSubmit' frontend/app.js

# Every form with a hx-post AND a data attribute that implies JS
# interception
grep -rn 'hx-post=' internal/templates/*.templ | grep -v '_test.go' \
  | while read line; do
    file=$(echo "$line" | cut -d: -f1)
    grep -l 'data-.*-submit\|data-async' "$file" 2>/dev/null
  done
```

Any form that has BOTH `hx-post` (or `hx-put`) AND a sibling JS
submit handler that calls `event.preventDefault()` is a candidate
for this bug class.

**Fix:** Pick ONE path and own it:

- **htmx path (preferred for status-page parity).** Delete the JS
  submit listener. Add `hx-target="this" hx-swap="none"` to the
  form. The form's server endpoint must write both `Location` and
  `HX-Redirect` (see §1.9). htmx handles everything end-to-end and
  the user lands on `/jobs/{id}` like every other export button.

- **JS path (preferred for synchronous flow with file path).** Delete
  the `hx-post` attribute from the form (and the
  `hx-on::after-request` 303 shim that some templates carry). The
  JS handler is now the sole submit owner. The bridge return value
  IS the status UI; document that the user will not land on
  `/jobs/{id}` from this button (or make the bridge return
  `/jobs/{id}` markup that links to the job status page).

Never carry both paths. The "bridge fallback to htmx" pattern is
the bug.

**Regression net:**

- `internal/appshell/redirect_headers_test.go::TestAll303sWriteHXRedirect`
  catches the §1.9 half (server side). The JS-intercept half has
  no automated guard yet — review the new PR template's
  "Click-driven surfaces checklist" to enforce it manually until a
  `submitPrintConfig`-style detector ships.

- `audit/smoke.mjs` `[5b]` Printable PDF modal flow asserts
  `share-print-modal-hx-redirect-to-jobs` and
  `share-print-modal-navigates-to-jobs` — pins the htmx path
  end-to-end.

**Real example:** The Printable PDF modal at
`internal/templates/share.templ:167` carried
`hx-on::after-request` 303 shim while `frontend/app.js::submitPrintConfig`
called `event.preventDefault()` and dispatched to the Wails bridge.
The bridge returned inline file-link markup. User never saw
`/jobs/{id}`. Fixed by dropping the JS interceptor, dropping the
shim, and routing the form through `handleExportDatabasePDF` like
every other export button.

---

### 3.5 Stale status panel after submit (4 instances in 60 days)

**Symptom:** User clicks a button. The handler runs, the side
effect happens (record is saved, job is enqueued, file is
written), the toast displays, but the **target panel** (status
panel, list, scan results, etc.) **does not refresh**. The user
sees the toast but no visible state change, and has to navigate
away and back to confirm the action took effect.

**Why it happens:** The 4 known instances in the last 60 days
fall into 4 distinct sub-patterns:

1. *Wrong `hx-target`*: form's `hx-target` points at a sibling
   that doesn't exist, so htmx logs `htmx:targetError` and the
   swap silently fails (Load Backup form, 7292f4f).
2. *No redirect after success*: handler writes 200 +
   `X-DixieData-Toast` but no `X-DixieData-Redirect` and no
   `data-results-target` opt-in, so the dispatcher has no
   follow-up action to take (handleImportBackup, 0f59909).
3. *Response lands below the viewport fold*: form is at the top
   of the page, status panel is below the fold, the user never
   scrolls. The page IS refreshing; the user just can't see it
   (2b492ee).
4. *Scan/quality results render into wrong div*: form has
   `data-results-target` but the value doesn't match an element
   in the DOM, or the target element's id is a typo
   (`#settings-orphan-results` vs `#orphan-results`)
   (2bb145b).

**Find it:**
```bash
# Forms with hx-swap="none" that should swap into a panel
grep -rn 'hx-swap="none"' internal/templates/*.templ

# Handlers writing X-DixieData-Toast but no X-DixieData-Redirect
# in the same file (likely a 2 sub-pattern above)
for f in $(grep -l 'X-DixieData-Toast' internal/appshell/*.go | grep -v _test.go); do
  if ! grep -q 'X-DixieData-Redirect\|writeExportRedirect' "$f"; then
    echo "$f"
  fi
done

# data-results-target / data-status-target without matching target id
for d in $(grep -rho 'data-results-target="[^"]*"' internal/templates/ | sort -u); do
  target_id=$(echo "$d" | sed 's/data-results-target="//;s/"//')
  if ! grep -rq "id=\"$target_id\"" internal/templates/; then
    echo "missing target id: $target_id"
  fi
done
```

**Fix (per sub-pattern):**

1. Verify the form's `hx-target` is an id that exists in the
   DOM. `htmx:targetError` is your friend in DevTools console.
2. Call `writeExportRedirect(w, routebuilder.X(...))` in the
   handler. The dispatcher will navigate.
3. Add a one-line `aria-live="polite"` + scroll the panel into
   view on submit (`element.scrollIntoView({behavior:"smooth"})`
   in `frontend/app.js`).
4. Pin the id with a `routebuilder.X()` builder or
   `internal/uiids` constant; assert in the test that
   `data-results-target` and the matching id agree.

**Real examples:**
- `7292f4f` (Load Backup wrong hx-target)
- `0f59909` (handleImportBackup missing redirect)
- `2b492ee` (status panel below the fold)
- `2bb145b` (scan/quality results, issue #134)

**Regression nets:**
- `audit/smoke.mjs` `[7c] orphan-scan-results-render` and
  `quality-scan-results-render` — verify the target div
  non-empty after submit
- `audit/smoke.mjs` `[7d] feedback-save-shows-toast` — verifies
  the toast is rendered (not queued) after submit

---

## 4. Go backend bugs

### 4.1 Goroutine / subscription leak

**Symptom:** Over time, goroutines accumulate. Memory grows.
Eventually OOM.

**Why it happens:** `Subscribe` to a channel creates a goroutine.
If the subscriber doesn't `Unsubscribe` when done, the goroutine
runs forever (waiting for next event).

**Find it:** Search for `go func` near `Subscribe`. Check whether
the goroutine has a `defer Unsubscribe()`.

**Fix:** Add `defer unsub()` at the top of the goroutine. Or use a
context-cancel pattern.

**Real example:** `271149a fix(appshell): 3 bug-hunter findings —
Subscribe leak, fd race, BrowserOpenURL error`.

---

### 4.2 Race condition on shared state

**Symptom:** Sporadic test failures or production data corruption.
Passes 99% of the time.

**Why it happens:** Concurrent read/write to a map or struct
without a mutex. Or a check-then-act without atomicity.

**Find it:** Run `go test -race ./...`. The race detector finds
these.

**Fix:** Add `sync.Mutex` around the shared state, OR use
`sync.Map`, OR restructure to immutable reads.

**Real example:** `271149a fix(appshell): 3 bug-hunter findings —
Subscribe leak, fd race, BrowserOpenURL error`.

---

### 4.3 Nil-guard gap

**Symptom:** Panic on a path that's rare in dev but common in prod
(after a fresh install, after a failed upgrade, etc.).

**Why it happens:** Code path that "always works in dev" because the
test setup pre-populates state. Production hits the path with empty
state.

**Find it:** Look for places that dereference a value without a nil
check. Common: `jobs.Get(id).Snapshot()`, `app.services.X()`.

**Fix:** Add the nil check, or restructure so the call site
guarantees non-nil.

**Real example:** `0551fbb fix(appshell): stop setup-redirect loop
on /jobs/* + nil-guard jobs handlers`.

---

### 4.4 Dead context parameter

**Symptom:** Handler signature accepts `ctx context.Context` but
never uses it. Operations can't be cancelled.

**Why it happens:** Refactor that added context didn't update all
call sites.

**Find it:**
```bash
grep -rn "func.*ctx context.Context" internal/appshell/ | grep -v "_test.go"
```
Look for signatures that take ctx but don't pass it to downstream
calls.

**Fix:** Either remove the parameter, or actually use it (pass to
DB queries, pass to HTTP clients, etc.).

**Real example:** `f55bba0 fix(appshell): 3 bug-hunter findings —
setup order, dead ctx param, dup export`.

---

### 4.5 Setup order — middleware depends on services that haven't started

**Symptom:** On startup, the first request crashes because service X
isn't initialized yet.

**Why it happens:** Middleware or route handler runs before
`app.Startup()` completes.

**Find it:** Trace the order: `main.go` → `app.NewApp()` →
`wails.Run()` → `app.Startup()` (async). Middleware that touches
services runs in the first request, which can arrive before Startup
completes.

**Fix:** Move service init before the server starts, OR add a
"ready" check in middleware.

**Real example:** `f55bba0 fix(appshell): 3 bug-hunter findings —
setup order, dead ctx param, dup export`.

---

### 4.6 SQLite BUSY under load

**Symptom:** Stress test produces intermittent `database is locked`
errors.

**Why it happens:** SQLite serializes writers. Under concurrent
load, writers wait and eventually time out.

**Find it:** Run stress tests repeatedly. Check
`internal/jobs/jobs.go` for write paths.

**Fix:** Add retry logic with backoff. Tune `busy_timeout` PRAGMA.

**Real example:** `bb6e685 fix(concurrency): retry reads on
SQLITE_BUSY; harden stress harness`.

---

### 4.7 Build / file path — wrong dir

**Symptom:** Production build fails to find templates or binaries
that work in dev.

**Why it happens:** Hardcoded paths from dev environment leak into
release code.

**Find it:** Diff `internal/templates/templates_dir_test.go` and
the actual lookup logic. Check whether `templates/` (typst source)
is found via the same path in dev vs release.

**Fix:** Use `runtime.Caller` or `os.Executable()` to anchor the
path, not `os.Getwd()`.

**Real example:** `0f485d5 fix(appshell): find Typst templates dir,
not internal/templates`.

---

### 4.8 Wails runtime guards

**Symptom:** Tests that exercise app handlers crash because wails
runtime isn't available (no frontend).

**Why it happens:** Handler calls `runtime.EventsEmit(...)` or
similar directly. The wails runtime is nil in test contexts.

**Find it:** Grep for `runtime.EventsEmit` / `runtime.WindowReload`
etc. Check whether each call is nil-guarded.

**Fix:** Add nil-guard: `if a.runtime != nil { a.runtime.EventsEmit(...) }`.

**Real example:** `d7afd0a fix(appshell): guard wails runtime calls
against missing frontend`.

---

### 4.9 Auto-opening exported files

**Symptom:** Click "Export PDF". The PDF downloads AND opens
automatically. Annoying for users.

**Why it happens:** A leftover `runtime.BrowserOpenURL(...)` call
after export.

**Fix:** Remove the auto-open call.

**Real example:** `eaac840 fix(appshell): stop auto-opening exported
files`.

### 4.10 Unguarded native dialog crashes the app

**Symptom:** Click "Export PDF" (or JPG, or screenshot, or
"Printable PDF"). Native save dialog appears, then the app
dies. `crash.log` shows `[WebView2 Error] The parameter is
incorrect.` followed by `Failed to unregister class
Chrome_WidgetWin_0. Error = 1412` and a goroutine stack that
ends in `Chromium.errorCallback` → `os.Exit(1)`. User loses
unsaved state.

**Why it happens:** Wails v2.12.0 on Windows hosts every native
`SaveFileDialog` and `OpenFileDialog` on the UI thread. Wails'
`onFocus` handler calls `Chromium.Focus()` →
`controller.MoveFocus(...)` whenever the host window regains
focus. When two native dialogs land on the message loop at the
same time (a double-click, a parallel JS bridge call, or a
htmx race), both block the UI thread, the focus event chain
fires while WebView2 is unstable, and the COM call returns
`The parameter is incorrect.` The
[`go-webview2`](https://github.com/wailsapp/go-webview2)
global error callback is hard-coded to call `os.Exit(1)`
(`chromium.go:151`), so the entire process dies.
[wailsapp/wails#2807](https://github.com/wailsapp/wails/issues/2807).

**Find it:**

```bash
# Every site that calls a.SaveFileDialog / OpenFileDialog
# without going through a guard.
grep -rn "a\.SaveFileDialog\|a\.OpenFileDialog\|a\.OpenDirectoryDialog\|a\.OpenMultipleFilesDialog" \
  internal/appshell/ --include="*.go"
```

For every match, confirm the handler either:

1. Routes through `a.guardedSaveFileDialog(...)` (pattern A),
2. Carries an inline `a.inFlight.LoadOrStore(dupKey, ...)` +
   `defer a.inFlight.Delete(...)` block (pattern B), or
3. Returns a sentinel error like `errExportInFlight` from a
   helper that owns the guard (pattern C).

**Fix:**

- **Pattern A** — call sites in
  `internal/appshell/exports_handlers.go` should route through
  `guardedSaveFileDialog(kind, opts)`. Caller checks `ok` and
  maps `false` to a 429 / friendly toast.
- **Pattern B** — call sites in `internal/appshell/app.go`
  carry inline guards modelled after `handleCalendarPDF`. Key
  is `kind|filename|filters` (or include `id` /
  `orientation` for soldier-record exports).
- **Pattern C** — `exportFullDatabasePDFPath` is the example.
  Returns `errExportInFlight`. HTTP handler maps it to
  `respondError(..., KindUnavailable, ...)`; Wails binding
  returns a friendly toast string.

**Required follow-up for every new guard:**

- Add a test to
  `internal/appshell/save_dialog_guard_test.go` that
  exercises the dupKey, asserts the second call is rejected,
  asserts the slot is released after the dialog returns, and
  asserts two distinct kinds don't collide.
- Update the table in
  [`docs/agents/dialog-guard.md`](agents/dialog-guard.md).
- Update `CHANGELOG.md` under `### Fixed`.

**Real examples:**

- `162c353 fix(appshell): guard every SaveFileDialog call against
  duplicate requests` — introduced `guardedSaveFileDialog` and
  wired 9 export handlers through it. Missed 5 sites (soldier
  PDF / JPG, screenshot, database PDF).
- The follow-up commit that closed those 5 sites — see
  `CHANGELOG.md` > Unreleased > Fixed.

**Why this is a bug class, not a one-off:** every new export /
import handler adds another call site. Any handler that reaches
`a.SaveFileDialog` without a guard is one double-click away from
killing the process. Code review must reject new unguarded sites
outright. See `CONTEXT.md` "Laws (non-negotiable)" and
[`docs/agents/dialog-guard.md`](agents/dialog-guard.md) for the
full law.

### 4.11 `duplicate-job-handling` — dedup helper, in-flight slot, redirect on duplicate (3 instances in 60 days)

**Symptom:** User clicks "Export Database PDF" twice in quick
succession. The expected behaviour is one job is enqueued and
the second click navigates to the existing `/jobs/{id}`. The
actual failure modes have been:

1. Second click produces a second job (dedup helper confuses
   "user cancelled the save dialog" with "duplicate click").
2. Reloading services mid-job clobbers the jobs registry and
   `jobs.NewWithConcurrency` returns a new registry; the
   duplicate check passes against the wrong registry.
3. Second click returns an error body instead of redirecting
   to the existing `/jobs/{id}` — the user has no way to find
   the in-flight job from the error page.

**Why it happens:** Three independent failure surfaces.

*Dedup map*: the `inFlight` map in `internal/appshell` is keyed
by `dupKey := guardedSaveFileDialogKey(kind, opts)`. The key
is deleted on every code path *including* the `user cancelled`
path. A genuine "user cancelled, then clicked again" is treated
as a duplicate of the original click. The fix is to only
delete the entry on success (or on handler return) — see
`14a2aa8`.

*Registry clobber*: `reloadServices` (called from the
`/setup` POST handler) replaces the entire `jobs.Registry`
with a fresh `NewWithConcurrency`. The in-flight slot is in
the OLD registry, so the dedup check in the NEW registry
returns "not in flight, proceed" and a second job is enqueued.
The fix is to either (a) preserve the in-flight map across
reload, or (b) drain before reload — see `ec451f4`.

*Redirect on duplicate*: the second click is supposed to be a
no-op + redirect. The handler must check `if dupJob != nil`
and call `writeExportRedirect(w, routebuilder.Job(dupJob.ID))`
before returning. Forgetting this makes the user think nothing
happened — see `16cd4e4`.

**Find it:**
```bash
# The dedup map / in-flight tracking
grep -rn 'inFlight\|inFlightEntry\|alreadyInFlight' internal/appshell/*.go \
  | grep -v _test.go

# Handlers that should redirect to existing /jobs/{id} on duplicate
grep -rn 'errExportInFlight\|Export already in progress' internal/appshell/

# The jobs registry re-allocation site (single point of failure)
grep -rn 'jobs\.NewWithConcurrency\|jobs\.New(' internal/appshell/

# Reload sites that could clobber the registry
grep -rn 'reloadServices\|a\.jobs' internal/appshell/*.go | grep -v _test.go
```

**Fix:**

1. Only `defer a.inFlight.Delete(dupKey)` on the happy path
   (no error returned), not on user-cancel.
2. `reloadServices` must preserve the in-flight map; or
   callers must not call `reloadServices` while a job is
   in-flight.
3. Add the `if dupJob != nil { writeExportRedirect(...); return }`
   branch as the **first** thing the handler does after
   dedup.

**Real examples:**
- `55414e0` (root cause for dialog-guard law)
- `14a2aa8` (cancel-vs-duplicate fix)
- `ec451f4` (reloadServices clobber fix)
- `16cd4e4` (redirect on duplicate, issue #130)

**Regression nets:**
- `audit/smoke.mjs` `share-${btn.path}-navigates-to-jobs` —
  fires two clicks in quick succession and asserts both land
  on the same `/jobs/{id}` URL

### 4.12 `toast-encoding-mojibake` (2 instances in 60 days)

**Symptom:** A toast displays a Unicode character (ellipsis `…`,
em-dash `—`, en-dash `–`, smart quotes `'`/`"`/`'`/`"`,
non-breaking space `\u00A0`, arrow `→`, checkmark `✓`, etc.)
in source, but on screen the user sees mojibake
(`â€¦`/`â€"`/`Ã—`/`Â `). Most often visible when the toast
appears via the `X-DixieData-Toast` HTTP header, not the in-page
toast renderer.

**Why it happens:** Chromium's WHATWG Fetch implementation
decodes HTTP/1.x response headers as Windows-1252 (the legacy
default), not UTF-8, when no explicit charset is provided. The
Go `net/http` package does not set `charset=utf-8` on header
values. Source files contain the polished Unicode; the wire
bytes contain the polished Unicode; the browser decodes them
as Windows-1252 and re-encodes as UTF-8 → mojibake.

Two historical fixes:

- `d91e32c` (fix(toasts+labels)) — replaced `"\u2026"` (a
  Go escape sequence for the U+2026 ellipsis) in source with
  the actual `\u2026` rune. The escape was a no-op and
  produced `â€¦` in the rendered toast. The fix is "use the
  real character."
- `3052251` (fix(appshell)) — added the
  `sanitiseToastForHeader` + `toastHeaderASCIIReplacements`
  helper next to `setToastHeader` in
  `internal/appshell/exports_handlers.go`. The helper
  translates the polished Unicode to an ASCII twin (`…` → `...`,
  `—` → `--`, `–` → `-`, etc.) only at the header boundary;
  source keeps the polished characters. Captured in
  `docs/adr/0005-toast-header-ascii-safe.md`.

**Find it:**
```bash
# All toast header writers
grep -rn 'X-DixieData-Toast\|setToastHeader' internal/appshell/*.go \
  | grep -v _test.go

# Test that guards against reintroduction of the escape
grep -rn 'TestInProgressToastStringsContainActualEllipsis\|TestSetInfoToastHeaderWritesInfoKind\|TestSanitiseToastForHeader' internal/appshell/
```

**Fix:**

- Source: use the actual Unicode rune, not a Go escape.
- Header boundary: route every `X-DixieData-Toast` write
  through `sanitiseToastForHeader` (or extend the replacement
  table if a new character is needed).
- For user-data toasts (e.g. names that may contain accented
  Latin, CJK), the helper passes them through unchanged; only
  the well-known punctuation characters are substituted.

**Real examples:**
- `d91e32c` (escape vs rune fix)
- `3052251` (header sanitisation, ADR 0005)

**Regression nets:**
- `internal/appshell/toast_header_sanitise_test.go` —
  unit tests the replacement table
- `internal/appshell/in_progress_toast_test.go` — source
  sweep for `\u2026` / `\u2014` etc.
- `internal/appshell/exports_handlers_test.go` — wires
  the ASCII twin assertion to the live handler

### 4.13 `route-misregistered-or-wrong-verb` (2-3 instances in 60 days)

**Symptom:** A clickable button or form posts and the server
returns 405 (Method Not Allowed) with no error in the UI. Or:
the button looks like it does nothing because the route is
registered with a different HTTP method (e.g. `r.Get` on a
POST-only handler).

**Why it happens:** New routes are registered against the chi
router in `internal/appshell/routes.go`. When a handler is
refactored (e.g. POST → DELETE, or GET → POST) the route
registration can drift. Also: a route is registered against
the wrong path (`/share/feedback` vs `/feedback/submit`)
and the templ still references the old path.

**Find it:**
```bash
# Every chi route registration
grep -rn 'r\.Get\|r\.Post\|r\.Put\|r\.Delete\|chi\.Route\|chi\.HandleFunc' \
  internal/appshell/routes.go internal/appshell/*.go \
  | grep -v _test.go

# Cross-check verb against handler
grep -rn 'http\.MethodPost\|http\.MethodGet\|http\.MethodDelete' \
  internal/appshell/*_handlers.go | grep -v _test.go

# Templ URLs that don't have a matching route
for url in $(grep -rho 'hx-get="[^"]*"\|hx-post="[^"]*"\|action="[^"]*"' internal/templates/ | grep -oE '"[^"]+"' | tr -d '"' | sort -u); do
  if ! grep -rq "\"$url\"" internal/appshell/routes.go internal/appshell/*.go; then
    echo "templ uses $url but no route matches"
  fi
done
```

**Fix:**

- Match the HTTP method to the handler's allowed methods. A
  POST handler must be registered with `r.Post` (not
  `r.Get` or `r.Handle`).
- The chi router returns 405 when the path matches but the
  method does not; fix the registration.
- Update the templ URL to match the new path. Use a
  `routebuilder.X()` builder so a route rename auto-propagates
  to every templ.

**Real examples:**
- `10d0d46` (caught 16 mis-registered chi routes in a single
  sweep)
- `e3cd413` (removed an SSE route that was registered but
  never reachable)

**Regression nets:**
- `internal/appshell/routes_test.go` — asserts the path/method
  matrix
- `internal/templates/hx_guard_test.go` — enforces that templ
  URLs come from `routebuilder.X()` builders
- `audit/smoke.mjs` `share-${btn.path}-fires-request` —
  asserts every discovered button triggers a 2xx or 3xx

### 4.14 `floating-dock-layout-overlap` (4 instances in 60 days)

**Symptom:** The floating dock at the bottom of the screen
covers content (anniversary list bottom, modal footer,
toast region) at certain viewport widths. On mobile the
dock overflows horizontally or pushes the feedback / menu
buttons off-screen.

**Why it happens:** The dock is `position: fixed; bottom: 0`
and uses a flex layout. The bottom padding on `<main>` and
on the toast region must be at least the dock height +
spacer, but as content height changes (more nav items, more
modal text), the dock height grows and the padding drifts.

The 4 known fixes:
- `db8db0b` — dock pinned to right edge
- `d3a4c3e` — dock background strip
- `03accbf` — dock inner padding
- `f439a20` — dock overlap with content
- `2bb78d7` — mobile overflow

**Find it:**
```bash
grep -rn 'floating.dock\|floating-dock' frontend/app.css internal/templates/layout.templ
grep -rn 'bottom-6\|right-6\|top-shell' frontend/app.css internal/templates/layout.templ
```

**Fix:** Add a `--floating-dock-height` CSS variable on `<html>`,
set it in `applyResponsiveLayout` from JS (measure the dock
height with `getBoundingClientRect()`), and reference it in
the `<main>` bottom padding and the toast region's `bottom`
offset. One source of truth, no drift.

**Real examples:** see the 5 commits above.

**Regression nets:** visual sweep via `audit/run-round3.mjs`
at the 3 viewports (mobile / tablet / desktop).

---

## 5. Typst PDF rendering bugs

(See section 2.1 for most typst-related patterns — they overlap.)

### 5.1 Image path resolution against dataDir

**Symptom:** Export produces PDF with missing images. Or images
show as broken-image placeholders.

**Why it happens:** Image paths stored relative-to-archive but
resolved relative-to-cwd.

**Fix:** Resolve against `appdata.DataDir()` not `os.Getwd()`.

**Real example:** `473b5d6 fix(export): resolve bulk-export image
paths against dataDir`.

---

### 5.2 Streaming records — OOM on large archives

**Symptom:** Export hangs or OOMs on archives with 500+ Person
Records.

**Why it happens:** Loading all records into memory before streaming.

**Fix:** Stream in batches of 500.

**Real example:** `11acc70 Fix export service: stream records in
batches of 500 instead of loading all at once`.

---

## 6. Accessibility bugs

(12 of 79 fix commits — second-largest category.)

### 6.1 Custom modal markup must trap focus + handle Esc manually

**Symptom:** Modal doesn't trap focus, doesn't close on Esc, doesn't
restore focus on close.

**Why it happens:** A custom `<div role="dialog" aria-modal="true">`
overlay has no built-in focus semantics.

**Fix:** Use the helpers in `frontend/app.js` —
`showOverlayModal(modal)` / `hideOverlayModal(modal)` /
`overlayModalKeydown(event)`. They swap the `hidden` / `flex`
classes, move focus to the first focusable child, trap Tab
inside the dialog, and unregister the keydown listener on
close.

**Why not `<dialog>`?** Native `<dialog>` was tried in
`1548407 fix(a11y): switch the three modal dialogs to native
<dialog>` and reverted because `showModal()` fires a focus event
on the WebView2 host that races the Wails `onFocus` handler —
the exact `MoveFocus` race described in section 4.10 below. Keep
the div overlay until Wails fixes the upstream interaction.
Tests in `internal/templates/{layout,share}_test.go` lock in the
overlay shape so this can't drift back.

---

### 6.2 Missing form label associations

**Symptom:** Screen reader reads input but no label. Click on label
doesn't focus the input.

**Why it happens:** `<label>` without `for` attribute, OR `<input>`
without matching `id`.

**Fix:** Add matching `for=...` and `id=...`.

**Real example:** `80a842c fix(a11y): associate form labels with
inputs via for/id pairs (issue #77)`.

---

### 6.3 Missing ARIA roles / labels on dynamic content

**Symptom:** Screen reader announces raw "div" or skips content
entirely.

**Why it happens:** Dynamically inserted content (modals, panels,
popovers) doesn't declare ARIA roles.

**Fix:** Add `role="dialog"`, `aria-labelledby="heading-id"`, etc.

**Real examples:**
- `e445b1f fix(a11y): dialogs declare role + aria-labelledby heading reference`
- `705e542 fix(a11y): feedback message textarea declares aria-required`
- `bd5a64e fix(a11y): disabled Compare button gets aria-describedby help text`

---

### 6.4 Missing landmarks on pagination / nav

**Symptom:** Screen reader user has no way to find pagination
controls.

**Fix:** Wrap pagination in `<nav aria-label="Search results">` and
add `aria-current="page"` to the active page link.

**Real example:** `ad28467 fix(a11y): search pagination gets nav
landmark + aria-current`.

---

### 6.5 Table headers missing `scope="col"`

**Symptom:** Screen reader doesn't associate header cells with data
cells in a column.

**Fix:** Add `scope="col"` to `<th>` elements.

**Real example:** `11cc587 fix(a11y): browse table headers declare
scope='col'`.

---

### 6.6 Nested-interactive violations

**Symptom:** axe-core / Lighthouse flag a button-inside-button or
anchor-inside-button.

**Why it happens:** Tooltip/hover handlers wrap interactive
elements.

**Fix:** Restructure: the outer is a `<div>` with `role="button"`,
the inner is the actual button.

**Real example:** `2bcb638 fix(a11y): resolve nested-interactive
violations on soldier detail/edit pages (issue #84)`.

---

### 6.7 Alt text on images

**Symptom:** Image preview modal shows image but no description for
screen readers.

**Fix:** Sanitize and apply `alt` text. Fall back to Person Record
display ID if no caption.

**Real examples:**
- `5098cf6 fix(a11y): sanitise alt text on the image preview modal`
- `b353f35 fix(a11y): image thumbs fall back to Person Record alt text`

---

## 7. Calendar / external API bugs

### 7.1 Google Calendar payload format

**Symptom:** Events sync to Google Calendar with wrong field
formatting.

**Fix:** Match the Google Calendar API's expected payload schema
exactly. Test with a real account.

**Real example:** `1150883 Fix Google Calendar reminder payload
format`.

---

### 7.2 Timezone handling on sync

**Symptom:** Events sync with UTC offset instead of user's local
timezone.

**Fix:** Pass the user's stored timezone offset; let Google handle
the display.

**Real example:** `21f9f4c Fix Google Calendar sync timezone
requirement`.

---

## 8. Build / CI bugs

### 8.1 PowerShell `$?` vs `$LASTEXITCODE`

**Symptom:** CI step runs `Expand-Archive` which returns success but
sets `$LASTEXITCODE` differently. Subsequent step fails.

**Fix:** Use `$?` (the boolean success indicator) instead of
`$LASTEXITCODE` for the previous command's success.

**Real example:** `5944773 fix(ci): use $? instead of $LASTEXITCODE
after Expand-Archive`.

---

### 8.2 Build flag not passed

**Symptom:** A script's `-Root` flag is needed for some path
operations. CI doesn't pass it.

**Fix:** Audit every script's flags against CI invocation.

**Real example:** `364585f fix(ci): pass -Root to
Restore-DixieDataTypstBinary in test workflow`.

---

### 8.3 Windows-native extraction tool

**Symptom:** Cross-platform tar.exe extraction differs from
PowerShell's `Expand-Archive`. Subtle differences in path handling.

**Fix:** Use the right tool for the right artifact. PowerShell for
.zip, tar.exe for .tar.gz.

**Real example:** `918fa5e fix(build): extract Typst zip with
Expand-Archive` and `fix(build): use Windows native tar.exe for
pdfium extraction`.

---

## 9. Database bugs

### 9.1 FTS5 delete not actually deleting

**Symptom:** Person Record updates leave stale entries in the
FTS5 index. Search returns ghost results.

**Why it happens:** FTS5 needs explicit delete when the row changes.

**Fix:** Wrap UPDATE in a transaction that deletes the old FTS row
and inserts the new one.

**Real example:** `583f7ab Fix FTS5 delete operation in soldier
Update`.

---

### 9.2 Normalization mismatch

**Symptom:** Filtering by pension_state="Virginia" returns
results for "VA" too, but not for " virginia" (with leading space).

**Why it happens:** Filter doesn't normalize before comparing.

**Fix:** Normalize via `pensionstate.Normalize(input)` before the
where clause.

**Real example:** `347cc0b Fix normalized pension-state filtering
for v1.2.34`.

---

## 10. Debugging workflow

When you see a regression, work this checklist in order:

### 10.1 Identify the layer

Before reading code, name the layer:

- **Did the user click something?** → HTMX wiring (section 1).
- **Did a page render wrong?** → `.templ` markup (section 2).
- **Did a button or toggle stop working after navigation?** →
  htmx-swap destroyed a listener (1.3).
- **Is it slow / hangs?** → Go backend (section 4). Check for
  leaks, race conditions, SQLite BUSY.
- **Is it a PDF?** → Typst (sections 2.1, 5).
- **Screen reader broken?** → a11y (section 6).
- **Build / CI failure?** → section 8.

### 10.2 The four diagnostic commands

```bash
# What's currently in this template?
grep -n 'hx-\|@\|templ ' internal/templates/somefile.templ

# What template renders a URL? (catches the wrong-selector class)
go test ./internal/templates/ -run TestHXURLsUseBuilders -v

# Is the boundary intact? (catches architectural drift)
go test ./internal/architecture/ -v

# Are there goroutine races?
go test -race ./...
```

### 10.3 Three files to read first when a regression appears

1. `internal/templates/hx_guard_test.go` — is the URL going
   through a routebuilder?
2. `internal/architecture/architecture_test.go` — is the import
   boundary still intact?
3. The recent commits touching the affected file
   (`git log --oneline -- internal/appshell/routes.go` etc.).

### 10.4 When to add a regression test

If a bug was found by accident (manual testing, code review,
prod report) and the fix is non-obvious, **add a snapshot or
behavioral test that would have caught it.** This is what the
`page_snapshot_test.go` and `hx_guard_test.go` files do — they
codify "we already burned fingers on this."

---

## 11. Bug class → first place to look

Quick reference table for "the page does X wrong, where's the bug":

| Symptom | First place to look | Pattern ref |
|---|---|---|
| Click does nothing | Section 1.1, 1.2 | button type / form wrap |
| Click fires but nothing changes | Section 1.8 | htmx target missing |
| Worked before, broken after navigation | Section 1.3 | listener destroyed by swap |
| Stale results | Section 1.4 | hx-sync missing |
| Progress bar keeps polling | Section 1.5 | terminal-state trigger |
| Handler runs, toast shows, page doesn't navigate | Section 1.10 | `X-DixieData-Redirect` missing |
| Form submits, server runs, JS post-response ignored | Section 1.11 | `data-dixie-submit` missing |
| Submit OK but target panel never refreshes | Section 3.5 | stale status panel after submit |
| Double-click produces duplicate job | Section 4.11 | dedup helper / in-flight slot |
| Toast shows mojibake | Section 4.12 | HTTP/1.x header charset |
| 405 from a clickable form | Section 4.13 | wrong HTTP method on route |
| Floating dock covers content | Section 4.14 | dock height + `<main>` padding drift |
| Panic on rare path | Section 4.3 | nil guard |
| Goroutine leak | Section 4.1 | missing Unsubscribe |
| Date wrong by 1 day | Section 2.3 | timezone |
| PDF missing images | Section 5.1 | dataDir resolution |
| Search returns ghost | Section 9.1 | FTS5 sync |
| Screen reader silent | Section 6.3, 6.5 | ARIA missing |
| Mobile layout broken | Section 3.1 | viewport test |
| Memory grows over time | Section 4.1, 4.2 | leak/race |
| Works in dev, fails in release | Section 4.7 | hardcoded paths |
| Tests crash on missing frontend | Section 4.8 | wails runtime nil |

For copy-paste greps for each pattern, see
[`docs/agents/bug-pattern-grep.md`](agents/bug-pattern-grep.md).

---

## 12. Checklist for new features

Before merging a feature, run this:

- [ ] All `hx-get`/`hx-post` go through `routebuilder.X()`
  (strict guard test passes)
- [ ] All `<button>` elements have explicit `type`
- [ ] All `<input>` with htmx polling are inside a `<form>`
- [ ] No `addEventListener` that should survive htmx swap
- [ ] Forms with rapid input have `hx-sync="this:replace"`
- [ ] Buttons that trigger expensive work have debounce or job
  dedup
- [ ] New routes registered in `routes.go` AND have a builder in
  `routebuilder.go`
- [ ] New surface IDs added to `uiids.Registry`
- [ ] New components use existing primitives
  (`components.Button`, `components.Card`, etc.) — no raw
  `<button class="primary-button">`
- [ ] `<dialog>` for modals, not positioned divs
- [ ] All form inputs have matching `<label for=...>`
- [ ] New goquery page snapshot test for any new top-level page
- [ ] Boundary test still passes (`./internal/architecture`)
- [ ] `go test -race ./...` clean