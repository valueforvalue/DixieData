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
change `hx-target` to a stable parent like `[data-progress-region]`.

**Defensive tooling:** `internal/templates/hx_guard_test.go`
warns about ad-hoc `#id` selectors that aren't in the
`internal/uiids.Registry`. Promote them if they're durable.

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

### 6.1 Native `<dialog>` vs custom modal markup

**Symptom:** Modal doesn't trap focus, doesn't close on Esc, doesn't
restore focus on close.

**Why it happens:** Custom modal markup (positioned divs) doesn't
have native dialog semantics.

**Fix:** Use the `<dialog>` element with `showModal()`. Browsers
handle focus trap, Esc, and ARIA correctly.

**Real example:** `1548407 fix(a11y): switch the three modal dialogs
to native <dialog>`.

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