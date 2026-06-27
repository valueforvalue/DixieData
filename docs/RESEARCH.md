# RESEARCH.md — Stabilization Sprint

**Audience:** Future AI agents (machine-actionable). Human reviewers skim.
**Scope:** Inventory current-state for the 5-PR stabilization sprint, grounded in
file paths and line numbers. Bug-class enumeration with evidence.

---

## 1. Problem statement

DixieData feature changes ship 2-3 UI bugs per change. Three forces compound:

1. **Edit-distance vs context-window math.** Largest templates (`entry_form.templ`
   1397 lines, `soldier_card.templ` 1176 lines) exceed what an LLM (or human) can
   hold whole across edit turns.
2. **Three orthogonal concerns collapsed into one `data-*` convention.** Test
   selectors, runtime JS hooks, and developer-overlay labels all share the
   `data-` namespace; renames and refactors collide silently.
3. **String literals for routes and HTMX targets.** Templates reference URLs
   and CSS selectors as bare strings; renames in `appshell/routes.go` or
   `uiids/` don't propagate. The "wrong selector / hx-* attr" bug class is
   fully attributable to this.

This document inventories the surface area the sprint must touch and the
evidence for each bug class.

---

## 2. Bug class evidence

### 2.1 Wrong selector / hx-* attr

`hx-get`/`hx-post` and `hx-target` use string literals in `.templ`. If a route
or panel ID is renamed, the template silently breaks — click does nothing,
form posts to nowhere.

Evidence (raw `hx-get`/`hx-post` in templates):

| File | Line | Attribute |
|---|---|---|
| `internal/templates/layout.templ` | 59 | `hx-get="/jobs/active"` |
| `internal/templates/layout.templ` | 126 | `hx-post="/feedback/submit"` |
| `internal/templates/layout.templ` | 176-177 | `hx-get="/debug/console"` `hx-target="body"` |
| `internal/templates/browse.templ` | 27 | `hx-get="/browse/results"` `hx-target="#browse-results"` |
| `internal/templates/soldier_card.templ` | 171 | `hx-get="/soldiers/search"` `hx-target="#soldier-list"` |
| `internal/templates/soldier_card.templ` | 178 | `hx-get="/soldiers/search?browse=1"` |

Route table is the single source of truth in `internal/appshell/routes.go:28-92`.
No compiler linkage between route patterns and template references.

### 2.2 Template/state drift

Templates inlined with imperative string-building via `fmt.Sprintf` for routes:

| File | Line | Pattern |
|---|---|---|
| `internal/templates/calendar.templ` | 130 | `hx-get={ fmt.Sprintf("/anniversary/%d/%d", month, day) }` |
| `internal/templates/calendar_day.templ` | 108 | `hx-get={ fmt.Sprintf("/anniversary/%d/%d?edit=%d", ...) }` |
| `internal/templates/calendar_day.templ` | 161 | `hx-get={ fmt.Sprintf("/anniversary/%d/%d", ...) }` |
| `internal/templates/job_slot_fragment.templ` | 25 | `hx-get={ templ.SafeURL("/jobs/" + job.ID + "/status?slot=1") }` |
| `internal/templates/jobs.templ` | 71 | `hx-get={ templ.SafeURL("/jobs/" + job.ID + "/status") }` |

Mixed: some templates use route-builder helpers (`browsePageHref`),
others inline strings. No single convention enforced.

### 2.3 Concurrent / stale-state

`layout.templ:59` polls `/jobs/active` every 3s. When a job completes during a
user form edit, the HTMX swap can replace the `progress-region` content;
test coverage on the poll path is implicit via integration tests only.

### 2.4 Developer overlay bleeds into test/JS surface

`data-ui-id` attribute is referenced for three distinct purposes:

1. **Developer visualizer** (its intended purpose): `SurfaceBadge` renders a
   dashed-outline badge + label when `DIXIEDATA_DEBUG_UI_IDS=1` or
   `--debug-ui-ids` is passed. This is a v1 leftover.
2. **Test selectors** (incidental use): snapshot tests in
   `internal/templates/entry_form_test.go` and `layout_test.go` use the
   attribute for assertions.
3. **Production DOM identifier** (collision): `data-ui-id` is set on every
   major page/panel via `data-ui-id={ uiids.X }`. If `app.js` ever queries
   it (it doesn't today, but future edits could), it couples production
   code to a debug-overlay attribute.

JS helper at `frontend/app.js:19`:
```js
const debugSurfaceIDsEnabled = () =>
  document.body?.getAttribute("data-debug-ui-ids") === "true";
```
Used at `frontend/app.js:972` to conditionally render the overlay.

CSS at `frontend/app.css:1` (compressed but verifiable via source):
```css
[data-debug-ui-ids=true] [data-ui-id]{outline:1px dashed ...;outline-offset:2px}
.ui-debug-badge{...}
.ui-debug-inline{...}
```

### 2.5 Glossary / domain mismatch

Domain terms defined in `CONTEXT.md` (Person Record, Source Record, Finding,
Confederate Home Status, Service Timeline, Research Collection, etc.) are
enforced inconsistently in templates. Out of scope for this sprint;
tracked separately.

---

## 3. Current-state inventory

### 3.1 Debug overlay surface (TO REMOVE)

**Go:**

| File | What |
|---|---|
| `internal/uiids/uiids.go` | `DebugEnvVar`, `DebugArg`, `DebugEnabled()` L7-9, `EnableFromArgs()` L139-149, `truthy()` L151-158 |
| `internal/uiids/uiids_test.go` | `TestDebugEnabledUsesEnvVar` L24-32, `TestEnableFromArgsRecognizesDebugArg` L34-42, `TestEnableFromArgsIgnoresUnknownArgs` L44-54 |
| `internal/templates/ui_debug.templ` | `SurfaceBadge` L3-7, `InlineSurfaceBadge` L9-13 |
| `internal/templates/ui_debug.go` | `uiDebugEnabled()` (helper, used by templates) |
| `internal/templates/ui_debug_templ.go` | generated |
| `internal/archive/diagnostics_service.go` L136 | `"DIXIEDATA_DEBUG_UI_IDS": os.Getenv(...)` |

**Templates with `data-ui-id` and `@SurfaceBadge(...)` calls:**

| File | `data-ui-id` count | `@SurfaceBadge` count |
|---|---|---|
| `internal/templates/browse.templ` | 2 | 2 |
| `internal/templates/calendar.templ` | 4 | 4 |
| `internal/templates/camaraderie.templ` | 1 | 1 |
| `internal/templates/conflict_ledger.templ` | 1 | 1 |
| `internal/templates/entry_form.templ` | 9 | 9 |
| `internal/templates/insights.templ` | 9 | 9 |
| `internal/templates/jobs.templ` | 1 | 1 |
| `internal/templates/layout.templ` | 2 | 2 |
| `internal/templates/research_collections.templ` | 2 | 2 |
| `internal/templates/research_log.templ` | 1 | 1 |
| `internal/templates/research_pack.templ` | 1 | 1 |
| `internal/templates/review_queue.templ` | 4 | 4 |
| `internal/templates/share.templ` | 4 | 4 (incl. 1 `InlineSurfaceBadge`-via-reference) |
| `internal/templates/soldier_card.templ` | 10 | 12 (incl. 2 `InlineSurfaceBadge`) |
| `internal/templates/timeline.templ` | 1 | 1 |
| **Total** | **52** | **54** |

**Frontend:**

| File | Lines | What |
|---|---|---|
| `frontend/app.js` | 19 | `debugSurfaceIDsEnabled` helper |
| `frontend/app.js` | 972 | conditional badge render in image viewer overlay |
| `frontend/app.css` | (see content) | `[data-debug-ui-ids=true] [data-ui-id]{outline:...}`, `.ui-debug-badge`, `.ui-debug-inline` |
| `frontend/tailwind.css` | (generated) | mirror of above |

**Generated files regenerated by `make tpl`:** 24 `_templ.go` files
(listed in `git grep -l "data-ui-id\|SurfaceBadge"`).

### 3.2 Debug console surface (KEEP — distinct feature)

The runtime log console is a different feature, do not touch:

| File | Role |
|---|---|
| `internal/debug/log.go` | Ring buffer of log entries |
| `internal/debug/handler.go` | Log handler |
| `internal/debug/requestid.go` | Request ID propagation |
| `internal/debug/uictx.go` | UI context |
| `internal/appshell/debug_handlers.go` | `/debug/console` route handlers |
| `internal/presentation/debug_console.templ` | Console panel UI |
| `internal/templates/entry_form.templ:802-803` | Settings → Debug panel |
| `internal/appshell/routes.go:100-102` | `/debug/console` route registration |

### 3.3 Routing surface (PR #1 target)

| File | Role |
|---|---|
| `internal/appshell/routes.go` | Single `setupRoutes()` L28-92; uses `net/http` stdlib `ServeMux` |
| `internal/appshell/app.go` | Wails `Options.AssetsHandler` wiring |
| `internal/appshell/respond.go` | Response helpers |
| `internal/templates/browse.templ` | Existing `browsePageHref()` helper as precedent |

Pattern: `mux.HandleFunc("/path", a.handleX)`. No middleware composition.

### 3.4 HTMX attribute surface (PR #2 target)

| Pattern | Where | Count |
|---|---|---|
| Raw `hx-get="..."` string | 6 templates | ~8 sites |
| `hx-get={ fmt.Sprintf(...) }` | 5 templates | 5 sites |
| `hx-get={ templ.SafeURL(...) }` (good) | 3 templates | 3 sites |
| `hx-get={ browsePageHref(...) }` (good, route helper) | 1 template | many sites |
| `hx-target="#some-id"` | 4 templates | 5 sites |
| `data-ui-id={ uiids.X }` (debug attribute; see §2.4) | 15 templates | 52 sites |

### 3.5 Boundary / package shape (PR #3 target)

| Layer | Allowed imports |
|---|---|
| `internal/records`, `internal/archive`, `internal/db`, `internal/models`, `internal/appdata`, `internal/dates` | stdlib only; no `net/http`, no `github.com/a-h/templ`, no Wails, no `internal/appshell` |
| `internal/viewmodel`, `internal/presentation` | stdlib + `internal/records`/`internal/archive`/etc.; no Wails |
| `internal/templates`, `internal/templates/components` | stdlib + viewmodels + uiids; no `net/http`, no Wails |
| `internal/appshell` | stdlib + everything + Wails; the only Wails-aware layer |

Current boundary enforcement: **none automated**. `internal/architecture_test.go`
does not exist; `grep -r "wails" internal/records` returns empty by convention,
not by enforcement.

### 3.6 Template size (PR #4 target)

| File | Lines | Notes |
|---|---|---|
| `internal/templates/entry_form.templ` | 1397 | Soldier form, scratchpad launcher, settings, initial setup all in one file |
| `internal/templates/soldier_card.templ` | 1176 | List, detail, search, edit (overlaps with entry_form) |
| `internal/templates/share.templ` | 807 | Export page, all variants (JSON/CSV/iCal/PDF/Backup/Shared Archive) |
| `internal/templates/browse.templ` | 494 | Manageable but the bug-factory site due to JS hooks |
| `internal/templates/review_queue.templ` | 313 | Manageable |
| `internal/templates/calendar.templ` | 277 | Manageable |
| `internal/templates/calendar_day.templ` | 255 | Manageable |

Total template LoC: 6250.

### 3.7 Test surface

**Existing:**
- `internal/templates/components/*_test.go` — 6 component snapshot tests
  (`Button`, `ButtonContent`, `Card`, `EmptyState`, `Field`, `Pill`, `Toast`).
  Byte-equality golden. Run via `make tpl` then `go test ./...`.
- `internal/templates/{browse,layout,soldier_card,browse_frontend}_test.go`
  — page-level renders, some via `bytes.Buffer` + `strings.Contains` checks.

**Missing:**
- No goquery assertion layer.
- No test asserts `hx-target` resolves to a known `uiids` constant.
- No test asserts every `data-ui-id` reference is a real constant.
- No boundary AST scanner.

### 3.8 `uiids` package (registry survives; rename deferred)

**Current shape:** `internal/uiids/uiids.go` exports 78 surface constants and
a `Registry` slice of `Surface{ID, Kind, Description}`. Used as `uiids.X`
in templates for both `data-ui-id` attributes (debug) and would-be typed
HTMX targets (PR #2).

**Proposed follow-up rename (NOT in this sprint):**
| Candidate | Rationale |
|---|---|
| `internal/surfaces` | Matches `Surface` type name; clear |
| `internal/panels` | Closer to `Panel*` constant naming convention |
| `internal/regions` | UI-region abstraction; broader than panels |
| `internal/surfaceregistry` | Verbose but explicit; matches `Registry` field |

The registry stays as `internal/uiids` through the sprint; rename lands as
a follow-up PR after stabilization. Defer the naming discussion.

### 3.9 Templ reference doc conventions

`TEMPL_REFERENCE.md` (7698 lines) is the canonical a-h/templ doc bundled
in the repo. Relevant sections for this sprint:

- L286-355: expectation testing with `goquery` — official assertion engine
- L569-575: `templ.SafeURL` is the required escape hatch for `href`/`hx-get`
- L2073-2095: `templ.Handler(component)` pattern for HTTP endpoints
- L2750+: `htmx` section confirms `hx-*` attribute conventions DixieData uses

Two convention divergences from the templ doc that the sprint must lock in
(not change, but document):

1. **Selector namespace:** Templ doc uses `data-testid` for goquery. DixieData
   uses `data-ui-id` (debug-overlay) for the same purpose, plus `data-<feat>-*`
   ad-hoc for runtime JS hooks. **Sprint codifies three namespaces:**
   - `data-testid="..."` (new convention) — goquery selectors only
   - `data-ui-id="..."` (existing) — debug overlay only; **removed in PR #0**
   - `data-<feature>-...` (existing) — runtime JS hooks only

2. **Constants package name:** Templ doc examples use `core/constants.go`.
   DixieData uses `internal/models/constants.go` (domain enums) and
   `internal/uiids/uiids.go` (surface registry). **Sprint keeps both.**

---

## 4. Out-of-scope items (recorded but not in this sprint)

| Item | Owner / follow-up |
|---|---|
| `internal/uiids` rename | Follow-up PR after stabilization; see §3.8 candidates |
| Glossary enforcement (CONTEXT.md terms) | Tracked in separate domain audit |
| `Wails` event bus typing (`runtime.EventsEmit`) | Tracked separately |
| `templ generate` CI hook | Currently `make tpl` is manual; consider CI gate |
| Snapshot golden regeneration tooling | Tracked separately |
| Existing test rewrites (e.g. `entry_form_test.go` selectors that depended on `data-ui-id`) | In-scope for PR #0; tests must pass without `data-ui-id` |
| Server-side rendering of debug snapshot | Tracked separately; PR #0 only removes dev overlay |

---

## 5. Sprint shape (forward reference; full plan in PRD.md)

5-PR sequence, each independently mergeable:

1. **PR #0** — Kill developer overlay (`data-ui-id`, `SurfaceBadge`,
   `DebugEnabled`, debug CSS, debug JS helper). Keep registry.
2. **PR #1** — Chi router + typed route builders in
   `internal/appshell/routes.go`.
3. **PR #2** — `HTMXMux` struct in `internal/htmxattr/` + adopt for new
   HTMX usage; `uiids` constants for `hx-target`.
4. **PR #3** — AST boundary test in `internal/architecture_test.go` +
   goquery guard test asserting every `hx-target`/`hx-get` resolves to
   known constants.
5. **PR #4** — Split `entry_form.templ`, `soldier_card.templ`,
   `share.templ`; add page-render snapshots for top 5 flows.

Full plan, slices, and acceptance criteria in `PRD.md` and `TASKS.csv`.
---

# Appendix: Follow-up inventory (post-sprint)

After the 5-PR stabilization sprint landed, eight concrete follow-ups
remain. The PRD §7 list (`FU.1`–`FU.5`) plus four discovered during
PR execution (`FU.6`–`FU.9`). This appendix inventories them so the
follow-up sprint can plan against a single source of truth.

## FU.1 — `internal/uiids` rename

From PRD §7. Candidates: `internal/surfaces`, `internal/panels`,
`internal/regions`, `internal/surfaceregistry`. Current 78 surface
constants are referenced from `internal/templates/*.templ`,
`internal/htmxattr/htmxattr.go` (registry check), and the architecture
test (forbidden list). Renaming the package requires coordinated
updates across all three. Low value relative to other follow-ups;
do it opportunistically when a future PR is already touching the
constants.

## FU.2 — Glossary enforcement (CONTEXT.md)

CONTEXT.md defines the ubiquitous language (Person Record, Source
Record, Finding, Confederate Home Status, Service Timeline, etc.).
Templates currently mix "Person Record" / "record" / "entry" /
"soldier" inconsistently. A glossary enforcement tool would lint
`.templ` files for non-canonical terms. Out of scope for this
follow-up plan; tracked separately because it intersects with
CONTEXT.md ownership and is a UX/copy concern, not an architecture
one.

## FU.3 — Wails event bus typing

**Status (verified during PR #4 execution):** no `runtime.EventsEmit`
calls exist in the codebase. The Wails event bus is unused. The
follow-up is a no-op until a feature needs events. Re-evaluate when
the first event-emitting feature lands.

If/when needed: create `internal/wailsevents` package mirroring the
`routebuilder` pattern. Pure functions returning typed event-name
strings. Templates and handlers import them. AST boundary test adds
`internal/wailsevents` to the forbidden list for the same packages
that can't import `routebuilder`.

## FU.4 — CI gate for `make tpl`

Makefile has `tpl:` target but no CI integration. The
`internal/templates/*_templ.go` files are gitignored so a stale
generated file would only surface at test time. Adding a CI step:

```yaml
- name: Regenerate templ
  run: make tpl
- name: Verify no changes
  run: git diff --exit-code internal/templates/*_templ.go
```

Catches drift between checked-out `.templ` source and generated code.

## FU.5 — `templ.Handler` migration of remaining handlers

`internal/appshell/respond.go` (208 lines) uses raw `fmt.Fprintf` /
`http.Error` for error responses. Migrating to `templ.Handler` would
standardize the response shape and unlock typed fragments. Low value
because the existing handlers all return JSON envelopes that
HTMX-side swap correctly; visual rendering is unchanged. Track for
opportunistic rewrite when a handler is touched anyway.

## FU.6 — Route builder inventory expansion

PR #3 goquery guard test reports raw `hx-get`/`hx-post` strings that
don't go through a builder. The advisory report from the last run
enumerates 19 sites across 8 templates:

| Template | Site | Pattern |
|---|---|---|
| `entry_form.templ` | 57 | `hx-post="/soldiers/scrape-findagrave"` |
| `entry_form.templ` | 113 | `hx-post="/soldiers"` |
| `entry_form.templ` | 794 | `hx-post="/settings/debug-mode"` |
| `entry_form.templ` | 813 | `hx-post="/settings/initialize"` |
| `entry_form.templ` | 833 | `hx-post="/settings/images/orphans/scan"` |
| `entry_form.templ` | 843 | `hx-post="/settings/quality/scan"` |
| `entry_form.templ` | 891, 900 | `hx-post="/settings/updates/source"` |
| `entry_form.templ` | 916 | `hx-post="/settings/updates/check"` |
| `entry_form.templ` | 919 | `hx-post="/export/backup"` |
| `entry_form.templ` | 923 | `hx-post="/settings/updates/apply"` |
| `entry_form.templ` | 976 | `hx-post="/settings/images/orphans/cleanup"` |
| `entry_form.templ` | 1014 | `hx-post="/settings/quality/apply"` |
| `insights.templ` | 26 | `hx-post="/insights/report/pdf"` |
| `research_collections.templ` | 38 | `hx-post="/research-collections"` |
| `review_queue.templ` | 26 | `hx-post="/review-queue/bulk"` |
| `share.templ` | 243 | `hx-post="/export/database-pdf?async=1"` |
| `share.templ` | 701 | `hx-post="/integrations/google/calendar/preferences/save"` |
| `soldier_card.templ` | 188 | `hx-get="/soldiers/search/advanced"` |

Plus 15 `fmt.Sprintf` URL sites (`/soldiers/%d`, `/soldiers/%d/edit`,
`/soldiers/%d/camaraderie`, etc.) that need parameterised builders.

## FU.7 — Tighten goquery guard to strict

`internal/templates/hx_guard_test.go::TestHXURLsUseBuilders` is
currently advisory. Flip to strict (count offenders == 0) once FU.6
lands. ~5 lines of test code change.

## FU.8 — Soldier-card template split

`internal/templates/soldier_card.templ` is 1174 lines, the largest
remaining. Structure:

- `templ SoldierCard(s viewmodel.PersonRecord, highlighted bool)` (50 lines)
- helper funcs (soldierCardClass, hasActiveSearch, deathDate, emptyDetail, blankDetail, formatAuditTimestamp, auditHistoryLines) (~75 lines)
- `templ SoldierList(...)` (~170 lines)
- helper funcs (pageHref, pageRequestURL, searchParams, searchSummary, etc.) (~210 lines)
- `templ SoldierDetail(s viewmodel.PersonRecord)` (~370 lines)
- helper funcs (reviewStatusLabel, soldierFullName, isSoldierEntry, ...) (~290 lines)

Same pattern as PR #4 entry_form split: extract helpers to
`soldier_card_helpers.go`, extract `SoldierList` to `soldiers_list.templ`,
extract `SoldierDetail` to `soldier_detail.templ`. Target: each file
≤ 600 lines.

## FU.9 — Share template split

`internal/templates/share.templ` is 803 lines. Largest single
contributor is `templ ShareView(...)` (~660 lines including
subcomponents). Extract:

- helper funcs to `share_helpers.go` (~85 lines)
- `templ ShareView(...)` stays in `share.templ` (~660 lines)
- Optional: split `ShareView` into smaller templates per format
  (JSON/CSV/iCal/PDF/Backup/Shared Archive) if the structure allows
  clean seams

Target: `share.templ` ≤ 400 lines.

## FU.10 — Jobs template real snapshot

PR #4 added `TestPageSnapshotJobsStatus` but it's a fake (renders
Layout instead of JobStatusView) because `jobs.Job` struct has an
unexported `sync.Mutex` field that can't be constructed from a test
file in a different package. Add a `NewJob` constructor in
`internal/jobs/jobs.go` that returns a properly-initialised `Job`,
update the snapshot test to use it, and assert actual job-status
markup (status badge, progress bar, cancel button, etc.).

Smallest follow-up; ~30 minutes of work. Worth doing before FU.8
because the new constructor unblocks a real test surface that the
soldier-card split will lean on.
