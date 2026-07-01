# DixieData User-Facing Services

A guided map of every user-facing surface, the DOM regions the user
touches, the backend it calls, and the open bugs / improvement ideas
attached to each. Distinct from `docs/ui-map/` (which is screen × DOM
× component) and `docs/ui-map/routes.md` (which is URL → handler);
this doc is **service-first, user-journey-ordered, with bug triage
and improvement log on each card**.

Use this doc for:

- **Guided UI/UX audit** — work through cards in order, check the
  bug table, file new findings in the Improvement Log.
- **Bug triage** — bottom Bug Ledger shows every known issue across
  all services in one triage surface.
- **Feature redesign brainstorm** — Cross-cutting Improvements section
  collects themes that span multiple services.

## How to read a service card

Each service has the same shape:

| Section | What it tells you |
|---|---|
| **Purpose** | One-sentence answer to "what does this let the user do?" |
| **Trigger / Outcome** | What user action enters; what they get when done. |
| **Inputs / Outputs** | Data the user provides + what the system returns. |
| **Happy path** | 3-6 numbered steps the user takes, with screen states. |
| **Failure modes** | Each failure → UX response. |
| **DOM roots** | Where in the layout the user sees it. |
| **Backend it calls** | `internal/appshell` packages / methods invoked. |
| **Cross-refs** | Links to wireframe, route table, CHANGELOG, smoke check, audit report. |
| **Bugs** | Structured table. |
| **Improvement log** | Dated ideas + status. |

### Status badges (Journey Map)

| Badge | Meaning |
|---|---|
| 🟢 | Stable — no open issues, recent changes verified |
| 🟡 | Recent change — last commit in this area within 30 days |
| 🟠 | Known bug — open issue filed |
| 🔵 | In redesign — active work in flight |
| ⚪ | Deprecated / planned removal |
| 🆕 | New since last audit |

### Bug row fields

| Field | Purpose |
|---|---|
| `status` | `open` / `fixed` / `wontfix` / `investigating` |
| `severity` | `blocker` / `high` / `medium` / `low` / `nit` |
| `summary` | One-line description |
| `issue#` | GitHub issue number if filed |
| `fix-commit` | Commit SHA that closed it (if fixed) |
| `audit-ref` | `audit/reports-rN/` finding ID if audit-sourced |

## The journey (map of all 23 services)

User flow order, top to bottom:

```
┌─────────────────────────────────────────────────────────────────────┐
│  01 Calendar ── 02 Calendar Day (fragment)                          │
│       │                                                              │
│       ▼                                                              │
│  03 Soldiers List (Search/Quick View) ── 04 Browse (Local Archive)   │
│       │                                                              │
│       ▼                                                              │
│  05 Soldier Detail ── 06 Soldier New ── 07 Soldier Edit              │
│       │                          │                                    │
│       ├─► 17 Service Timeline   ├─► 18 Unit Camaraderie             │
│       ├─► 15 Research Log       ├─► 19 Merge Review Ledger           │
│       ├─► 14 Research Collection Detail                              │
│       │                                                              │
│       ▼                                                              │
│  08 Share / Export ── 20 Jobs (async export pipeline)                │
│       │                                                              │
│       ▼                                                              │
│  09 Insights ── 10 Insights Drilldown                                │
│       │                                                              │
│       ├─► 11 Review Queue ── 12 Review Queue Compare                 │
│       │                                                              │
│       ├─► 13 Research Collections Hub ── 14 Detail                   │
│       ├─► 16 Research Pack (county/state scoped)                     │
│       │                                                              │
│       ▼                                                              │
│  21 Settings ── 22 Initial Setup (first-launch only)                 │
│       │                                                              │
│       ▼                                                              │
│  23 Recovery                                                         │
└─────────────────────────────────────────────────────────────────────┘

Cross-cutting (any screen):
  • Floating dock (Scratch Pad / Feedback / Menu)
  • Top nav (brand + 8 pill-links + primary CTA)
  • Toast region (top-right transient)
  • Jobs progress overlay (3s poll)
  • Image viewer overlay
  • Print-config modal
  • Google Calendar prefs modal
```

## Service cards (user-journey order)

---

## 01 · Calendar

**Status**: 🟢 stable · 🟡 recent (Unreleased: pension_field visibility)

| Field | Value |
|---|---|
| **Purpose** | Track soldier anniversaries alongside custom holidays and events on a month grid. |
| **Trigger / Outcome** | User opens `/calendar` (or a month link) → sees monthly grid + per-day detail pane; clicking a day loads anniversaries + holidays for that date. |
| **Inputs / Outputs** | In: `m` (month, optional; defaults to current). Out: month grid, soldier/spouse/person counts, anniversary detail pane. |
| **Happy path** | (1) User lands on `/calendar`. (2) Counts chips render. (3) User clicks a day cell → `/anniversary/{m}/{d}` loads into `#details-pane`. (4) User adds a custom event via the Add Event form. (5) User clicks Export → `/calendar/{m}/report.pdf` posted from print-config modal. |
| **Failure modes** | (a) Day has no data → `EmptyStateCard` "no anniversaries". (b) PDF render fails → toast `Failed to render PDF`. (c) Custom event POST fails → status banner in details pane. |
| **DOM roots** | `panel.calendar.quote`, `panel.calendar.grid`, `panel.calendar.details`, `#details-pane`. |
| **Backend it calls** | `app.handleCalendar`, `app.handleAnniversaryDay`, `app.handleCalendarPDF` (via `guardedSaveFileDialog` → Typst render → Save). |
| **Cross-refs** | [wireframe](ui-map/wireframes/01-calendar.md), [routes](ui-map/routes.md#calendar-route-group), [smoke check #8](appshell/smoke.go) `routes_registered` probes `/` but not `/calendar` (gap noted below). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| open | low | `/calendar` not probed by smoke `routes_registered` | — | — | — |
| fixed | medium | Calendar + Insights PDF buttons didn't submit (missing `type=submit`) | — | `cf19dfc` | r1 |

### Improvement log

- **2026-06-29**: Add `/calendar` and `/anniversary/{m}/{d}` probes to `checkRoutesRegistered` so registration regressions fail fast (audit coverage).
- **2026-06-29**: Calendar quote panel could pull from a daily rotating pool instead of being static.
- **2026-06-29**: Consider week-start preference (Sun vs Mon) — locale-dependent, currently hardcoded.

---

## 02 · Calendar Day (fragment)

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Render the per-day detail (anniversaries + custom items + add/edit form) inside Calendar's `#details-pane`. |
| **Trigger / Outcome** | User clicks a day on Calendar → fragment swaps in; can add/edit/delete custom events and jump to linked Person Records. |
| **Inputs / Outputs** | In: `m`, `d`, optional `edit={id}`. Out: day detail HTML fragment. |
| **Happy path** | (1) User clicks day → fragment loads. (2) User reviews anniversaries. (3) User adds custom event via form (POST `/anniversary/{m}/{d}/items`). (4) User clicks Person Record pill → navigates to `/soldiers/{id}`. |
| **Failure modes** | (a) Edit mode for non-existent item → 404 status. (b) Delete fails → status banner. |
| **DOM roots** | Inside `#details-pane`; not its own page-level ID. |
| **Backend it calls** | `app.handleAnniversaryDay`, `app.handleAnniversaryItemCreate`, `app.handleAnniversaryItemUpdate`, `app.handleAnniversaryItemDelete`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/02-calendar-day.md), [routes](ui-map/routes.md#calendar-route-group). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| open | nit | Anniversaries on a Sunday (June 1) render empty state with no hint that this is correct | — | — | audit/notes-2026-06-29 |

### Improvement log

- **2026-06-29**: Empty-state copy could explicitly note "no anniversaries on this date" instead of generic empty.

---

## 03 · Soldiers List (Search / Quick View)

**Status**: 🟡 recent (b7e659d require first/last name; 4afa15e pension field wife fix)

| Field | Value |
|---|---|
| **Purpose** | Find Person Records by name / unit / pension state with two search modes (basic + advanced). |
| **Trigger / Outcome** | User types in search → HTMX debounced request fires → results render in `#soldier-list` without full page reload. |
| **Inputs / Outputs** | In: `q` (basic), or filter set (advanced: scope, sort, entry_type, pension_state, unit, etc.). Out: matching rows + link to detail. |
| **Happy path** | (1) User lands on `/soldiers` with empty query → recent records hydrate. (2) User types in basic search → 200ms-debounced HTMX GET `/soldiers/search`. (3) User switches to Advanced tab → `/soldiers/search/advanced` swap. (4) User clicks a row → `/soldiers/{id}`. |
| **Failure modes** | (a) Empty query → recent results. (b) No matches → `EmptyStateCard`. (c) Audit walker found a false-positive match (scrapes FindAGrave form first) — documented. |
| **DOM roots** | `panel.soldiers.search.basic`, `panel.soldiers.search.advanced`, `panel.soldiers.results`, `tab.soldiers.search.{basic,advanced}`, `#soldier-list`. |
| **Backend it calls** | `app.handleSoldiersList`, `app.handleSoldierSearch` (basic + advanced). |
| **Cross-refs** | [wireframe](ui-map/wireframes/03-soldiers-list.md), [routes](ui-map/routes.md#soldiers--search--browse). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | medium | 5.7s load on `/soldiers` (search input not in `<form>`) | #85 | `5b022a7` | r1 |
| fixed | high | Could create Person Record without first/last name | — | `b7e659d` | — |
| fixed | medium | `pension_state` field hidden for `wife` entry type | #75 | `4afa15e` | — |
| fixed | nit | Browse walker locator too broad (matched `/soldiers/new` button before real rows) | — | — | audit/notes-2026-06-29 (suggestion) |

### Improvement log

- **2026-06-29**: Tighten `audit/run-interactive.mjs` browse locator to skip `/soldiers/new` button.
- **2026-06-29**: Add a "Recent searches" pill row under basic search (currently shows recent records; would be richer if it showed recent queries).
- **2026-06-29**: Advanced tab has 9 filter inputs (audit round-2 #7) — consider collapsible drawer.

---

## 04 · Browse (Local Archive)

**Status**: 🟢 stable · 🟡 audit-finding (small-tap-target mobile)

| Field | Value |
|---|---|
| **Purpose** | Filter + sort + paginate the entire Local Archive with deep filter chips. |
| **Trigger / Outcome** | User applies filter set → `/browse/results` HTMX fragment swaps in matching rows; URL deep-links restore state. |
| **Inputs / Outputs** | In: 9 filter inputs (scope, sort, page size, entry type, pension state, review status, unit, buried in, confederate home status). Out: filtered rows + counts. |
| **Happy path** | (1) User lands on `/browse`. (2) User opens Filters `<details>` → 9 inputs. (3) User clicks Apply → `/browse/results` fragment swap. (4) User clicks Compare checkbox on 2 rows → Compare button enabled. (5) User clicks Compare → `/review-queue/compare`. |
| **Failure modes** | (a) No matches → `EmptyStateCard`. (b) Filter resets → default scope/sort. (c) URL deep-link with stale query → handled gracefully. |
| **DOM roots** | `panel.browse.results`, `#soldier-list` (shared with soldiers list). |
| **Backend it calls** | `app.handleBrowse`, `app.handleBrowseResults`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/04-browse.md), [routes](ui-map/routes.md#soldiers--search--browse). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | high | `confederatehomestatus.Normalize` silently rewrote unknown values to N/A | #23 | `4359c1f` | — |
| fixed | medium | Browse-row text labels render <24×24 on mobile (WCAG 2.5.5) — framing was inaccurate; elements are non-interactive `<dt>` labels, so the real defect was readability. Fixed by bumping `<dt>` from 0.65rem → 0.75rem | — | this branch | r3 (PASS) |
| fixed | low | Browse-filter debounce regressed to 0ms | — | `e7ece2c` | — |
| fixed | low | `browse-filter-persists-from-url` flow | — | — | r3 (PASS) |

### Improvement log

- **2026-06-29**: Replace dense 9-column browse table with card list on mobile, virtualised table on desktop (audit r1 top-1).
- **2026-06-29**: Wrap the entire `<tr>` in click handler so tap-target spans row width (already works; audit concern is theoretical).
- **2026-06-29**: Add pagination/virtualisation (50 records = 4369px scroll) — audit r1 top-8.

---

## 05 · Soldier Detail

**Status**: 🟢 stable · 🟡 recent (d91e32c ellipsis fix)

| Field | Value |
|---|---|
| **Purpose** | Show one Person Record: summary, source records, images, scratch pad, links to timeline / camaraderie / research log / conflict ledger. |
| **Trigger / Outcome** | User clicks a row anywhere (search/browse/recent) → lands on `/soldiers/{id}`; can drill into sub-views or trigger Edit / Export / Compare. |
| **Inputs / Outputs** | In: `id`. Out: full Person Record render + sub-nav. |
| **Happy path** | (1) User lands. (2) Reads summary. (3) Clicks image → `overlay.image.viewer`. (4) Clicks Timeline → `/soldiers/{id}/timeline` HTMX swap. (5) Clicks Edit → `/soldiers/{id}/edit`. (6) Clicks Export Record ▾ popout → print-config modal → PDF or JPG. |
| **Failure modes** | (a) Record deleted → 404. (b) Primary image missing → fallback rendering. (c) Export PDF fails → toast + job failure. |
| **DOM roots** | `panel.soldier.detail.summary`, `panel.soldier.detail.records`, `panel.soldier.detail.images`, `overlay.image.viewer`. |
| **Backend it calls** | `app.handleSoldierDetail`, `app.handleSoldierPDF`, `app.handleSoldierImagesDownload`, `app.handleSoldierImagesPrimary`, `app.handleSoldierReviewFlag`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/05-soldier-detail.md), [routes](ui-map/routes.md#soldiers--search--browse). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | low | Broken `\u2026` escape in toast labels | — | `d91e32c` | — |
| fixed | high | Memorial confirmation flow missing (jobs integration) | #137 | `331b58d`, `00f0ba9` | — |

### Improvement log

- **2026-06-29**: Detail header could include the Display ID more prominently (currently a small pill).
- **2026-06-29**: Sub-nav (Timeline / Camaraderie / Research Log / Conflict Ledger) is a deep-link list; consider a sticky tab strip.

---

## 06 · Soldier New

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Create a new Person Record with optional FindAGrave prefill. |
| **Trigger / Outcome** | User clicks "+ Add Person Record" CTA → form renders; on submit → POST `/soldiers` → redirect to `/soldiers/{id}`. |
| **Inputs / Outputs** | In: form fields (name, type, dates, units, pensions, etc.) + optional FindAGrave scrape. Out: new Person Record + Display ID. |
| **Happy path** | (1) User opens `<details>Scrape Find a Grave</details>` (collapsed). (2) Pastes memorial URL + clicks Fetch Data. (3) Form pre-fills, warnings render. (4) User picks Entry Type (Soldier/Spouse/Person). (5) Submits → toast + redirect to detail. |
| **Failure modes** | (a) Missing first/last name → 422 with field error (fixed `b7e659d`). (b) Scrape fails → inline error callout, user can still submit manually. (c) Duplicate detected → review queue redirect. |
| **DOM roots** | `panel.soldier.form.scratchpad`, `panel.soldier.form.records`, `panel.soldier.form.images`. |
| **Backend it calls** | `app.handleSoldierNew`, `app.handleSoldierCreate`, `app.handleScrapeFindAGrave`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/06-soldier-new.md), [routes](ui-map/routes.md#soldiers--search--browse). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | high | Could submit without first/last name | — | `b7e659d` | — |
| fixed | medium | Form action missing (manual-walk audit confused with scrape form) | — | — | audit/notes-2026-06-29 (false positive, walk fix noted) |

### Improvement log

- **2026-06-29**: Audit walker should scope manual-walk form locator to `form[action*="/soldiers"]` to skip scrape form.
- **2026-06-29**: After-save redirect could land on Edit instead of Detail (small UX win, lets user add Source Records immediately).

---

## 07 · Soldier Edit

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Edit an existing Person Record; add Source Records + images; toggle Review Queue. |
| **Trigger / Outcome** | User clicks Edit pill on detail → form pre-populates; on submit → PUT `/soldiers/{id}` → toast + redirect. |
| **Inputs / Outputs** | Same as New, minus FindAGrave scrape (hidden on edit). |
| **Happy path** | (1) User opens Edit. (2) Edits field. (3) Adds Source Record row. (4) Imports images. (5) Submits → toast + redirect to detail. |
| **Failure modes** | (a) Optimistic concurrency conflict → toast + revert. (b) Image import fails → inline error. |
| **DOM roots** | Same as Soldier New. |
| **Backend it calls** | `app.handleSoldierEdit`, `app.handleSoldierUpdate`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/07-soldier-edit.md), [routes](ui-map/routes.md#soldiers--search--browse). |

### Bugs

None open. Share bugs with New/Detail where applicable.

### Improvement log

- **2026-06-29**: Show a "last saved at" timestamp + version counter for confidence.

---

## 08 · Share / Export

**Status**: 🟡 recent (4a9b275 feedback log export; 8503f3a render build-tag)

| Field | Value |
|---|---|
| **Purpose** | Single page for all export/import flows: JSON backup, PDF database export, shared-archive import, restore-from-backup, feedback log download. |
| **Trigger / Outcome** | User opens `/share` → sees Export & Backup card + Import & Restore card + Collaborative Merge card; each button triggers a native dialog (guarded) or async job. |
| **Inputs / Outputs** | In: button click + native dialog selection. Out: file on disk, or async job → jobs page. |
| **Happy path** | (1) User clicks Export JSON → guarded `SaveFileDialog` → backup file written. (2) User clicks Export PDF (async) → job created → redirect to `/jobs/{id}`. (3) User clicks Import Shared → `OpenFileDialog` → confirm modal → job. |
| **Failure modes** | (a) Dialog guard race → UI thread crash (LAW: must be guarded; see dialog-guard.md). (b) Job fails → job page with log download. (c) Merge conflicts → Review Queue. |
| **DOM roots** | `panel.export.actions`, `panel.export.google`, `overlay.print-config.modal`, `overlay.google-calendar-prefs.modal`. |
| **Backend it calls** | `app.guardedSaveFileDialog`, `app.handleExportBackup`, `app.handleExportDatabasePDFAsync`, `app.handleImportShared`, `app.handleFeedbackExportLog`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/08-export.md), [routes](ui-map/routes.md#export--share--jobs), [dialog-guard law](agents/dialog-guard.md). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | medium | Shared import re-copied files unnecessarily | — | `d26a8b0` | — |
| fixed | medium | Feedback log not exported via enqueueExport pattern | #137 | `4a9b275` | — |
| fixed | blocker | Memorial import log download button committed but not wired | — | `70878ac` (added) | gaps.md |
| open | high | Dialog guard audit not yet run on Export handlers | — | — | gaps.md (deferred) |

### Improvement log

- **2026-06-29**: **Run dedicated dialog-guard audit** on all export/import handlers (per gaps.md).
- **2026-06-29**: Wire Memorial import log download button — `jobSummaryCard` should branch on `job.Result.LogPath != ""`. ✅ **Fixed in commit on `fix/jobs-wire-memorial-log-download`** — added `streamJobLog` handler with `os.TempDir()` containment check, route via `routebuilder.JobLog(jobID)`, button in `jobSummaryCard`, 4 regression tests.
- **2026-06-29**: Add deep-link pill on shared-import summary card when `Conflicts > 0` (per gaps.md).
- **2026-06-29**: Share page is the most action-dense screen — consider progressive disclosure (Export / Import / Merge as separate tabs).

---

## 09 · Insights

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Dashboard view of Local Archive: record-type snapshot, cemeteries, homes, pensions, units, chronology, duplicate audit. |
| **Trigger / Outcome** | User opens `/insights` → 7 panels render with counts; clicking a panel drills into `/insights/drilldown`. |
| **Inputs / Outputs** | In: none (full-page render). Out: 7 panel summaries + counts. |
| **Happy path** | (1) User lands. (2) Reviews overview counts. (3) Clicks "Person Record Type Snapshot" → drilldown. (4) Clicks Export Analytics Report → PDF via print-config modal. |
| **Failure modes** | (a) Zero records → `EmptyStateCard`. (b) PDF render fails → toast. |
| **DOM roots** | `panel.insights.{overview,cemeteries,homes,pensions,units,chronology,duplicate-audit}`. |
| **Backend it calls** | `app.handleInsights`, `app.handleInsightsDrilldown`, `app.handleInsightsReportPDF`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/09-insights.md), [routes](ui-map/routes.md#insights). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | medium | Calendar + Insights PDF buttons didn't submit | — | `cf19dfc` | r1 |

### Improvement log

- **2026-06-29**: Each panel could link to its source query so user can verify the count (currently opaque).

---

## 10 · Insights Drilldown

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Detailed row list for one Insights panel (e.g. all soldiers by unit, all pensions by state). |
| **Trigger / Outcome** | User clicks an Insights panel card → drilldown renders the matching rows. |
| **Inputs / Outputs** | In: panel kind, filter. Out: table of rows. |
| **Happy path** | (1) User lands via Insights click. (2) Reviews rows. (3) Clicks a row → Person Record detail. |
| **Failure modes** | (a) HTMX fragment → axe misfires on `html-has-lang` (audit carve-out, not a real bug). |
| **DOM roots** | Inside Insights page (`/insights/drilldown` handler direct). |
| **Backend it calls** | `app.handleInsightsDrilldown`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/10-insights-drilldown.md), [routes](ui-map/routes.md#insights). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| wontfix | low | Axe reports false positives on HTMX fragments | — | — | r2 carve-out |

### Improvement log

- **2026-06-29**: Carve out fragment endpoints from axe scans (audit harness improvement, r2 recommendation).

---

## 11 · Review Queue

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Hold Person Records flagged for human attention (duplicates, merge conflicts, manual review flags). |
| **Trigger / Outcome** | User opens `/review-queue` → list of flagged records; can select 2+ for compare or apply bulk action. |
| **Inputs / Outputs** | In: filter / search. Out: list with checkboxes + Compare button. |
| **Happy path** | (1) User lands. (2) Selects 2 rows. (3) Compare enabled. (4) Clicks Compare → `/review-queue/compare`. |
| **Failure modes** | (a) Empty queue → `EmptyStateCard`. (b) Bulk action fails → toast. |
| **DOM roots** | `panel.review-queue.list`. |
| **Backend it calls** | `app.handleReviewQueue`, `app.handleReviewQueueBulk`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/11-review-queue.md), [routes](ui-map/routes.md#review-queue). |

### Bugs

None open. Audit notes 50-soldier seed leaves this empty (partial coverage).

### Improvement log

- **2026-06-29**: Audit needs a richer seed with seeded duplicates + merge conflicts to fully exercise this surface.

---

## 12 · Review Queue Compare

**Status**: 🟡 recent (round-3 fixes: tabindex, anchor pills)

| Field | Value |
|---|---|
| **Purpose** | Side-by-side diff of two Person Records (Local vs Incoming, or two candidates for merge). |
| **Trigger / Outcome** | User clicks Compare from Browse or Review Queue → `/compare?id1={a}&id2={b}` renders two-column diff. |
| **Inputs / Outputs** | In: `id1`, `id2`. Out: two-column table + "DIFFERENCES TO REVIEW FIRST" pill row. |
| **Happy path** | (1) User lands. (2) Reviews pill row of differences. (3) Clicks a pill → anchor-scroll to row. (4) Picks Local or Incoming for each field. (5) Submits merge. |
| **Failure modes** | (a) Mobile overflow → fixed in r3 (`tabindex="0"`). (b) Pill anchors don't scroll → fixed in r3. |
| **DOM roots** | `panel.review-queue.compare`. |
| **Backend it calls** | `app.handleCompare`, `app.handleMergeApply`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/12-review-queue-compare.md), [routes](ui-map/routes.md#review-queue), [audit r3](audit/reports-r3/audit-v3.md). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | serious | Mobile `scrollable-region-focusable` (no keyboard scroll) | #90 | r3 | r2 #1 |
| fixed | high | "DIFFERENCES TO REVIEW FIRST" pills didn't anchor-scroll | — | r3 | r2 #2 |
| open | low | Cell padding tight (6px vs 8px) | — | — | r3 (cosmetic) |

### Improvement log

- **2026-06-29**: Could collapse to stacked cards on mobile (audit r2 alternative); current fix is minimum viable.

---

## 13 · Research Collections Hub

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | User-curated grouping of archive material assembled inside a Local Archive for an ongoing research purpose. |
| **Trigger / Outcome** | User opens `/research-collections` → list of collections + create form. |
| **Inputs / Outputs** | In: collection name. Out: collection detail page link. |
| **Happy path** | (1) User lands. (2) Sees existing collections. (3) Creates new → POST → refresh. (4) Clicks collection → detail. |
| **Failure modes** | (a) Empty → `EmptyStateCard`. (b) Audit notes seeded data leaves this empty. |
| **DOM roots** | (no panel ID registered). |
| **Backend it calls** | `app.handleResearchCollections`, `app.handleResearchCollectionsCreate`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/13-research-collections-hub.md), [routes](ui-map/routes.md#research). |

### Bugs

None open.

### Improvement log

- **2026-06-29**: Add a panel ID (`panel.research-collections.hub`) — currently missing from `uiids`.

---

## 14 · Research Collection Detail

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | One collection's contents: list of Person Records + add/remove actions. |
| **Trigger / Outcome** | User clicks a collection from hub → `/research-collections/{id}` renders contents. |
| **Inputs / Outputs** | In: `id`. Out: collection metadata + items. |
| **Happy path** | (1) User lands. (2) Reviews items. (3) Adds new item via POST. (4) Removes via inline action. |
| **Failure modes** | (a) Empty → `EmptyStateCard`. |
| **DOM roots** | (no panel ID registered). |
| **Backend it calls** | `app.handleResearchCollectionDetail`, `app.handleResearchCollectionAdd`, `app.handleResearchCollectionRemove`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/14-research-collection-detail.md), [routes](ui-map/routes.md#research). |

### Bugs

None open.

### Improvement log

- **2026-06-29**: Add a panel ID (`panel.research-collection.detail`).
- **2026-06-29**: Could show a mini-timeline or unit-membership summary per item.

---

## 15 · Research Log

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Structured record of research activity, findings, or open questions tied to a Person Record. |
| **Trigger / Outcome** | User opens `/research-log/{soldierID}` → log entries render with task creation. |
| **Inputs / Outputs** | In: `soldierID`, task text. Out: chronological log + tasks. |
| **Happy path** | (1) User lands. (2) Reads past entries. (3) Adds new task via POST `/soldiers/{id}/research-log/tasks`. (4) Marks task complete inline. |
| **Failure modes** | (a) Empty → empty state. |
| **DOM roots** | (no panel ID registered). |
| **Backend it calls** | `app.handleResearchLog`, `app.handleResearchLogTasksCreate`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/15-research-log.md), [routes](ui-map/routes.md#research). |

### Bugs

None open.

### Improvement log

- **2026-06-29**: Distinguish Scratch Pad (informal) from Research Log (structured) more clearly in the UI — currently both show as text fields. Per CONTEXT.md, they are different domain concepts.

---

## 16 · Research Pack

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Prepared archive bundle organized around a defined scope (county or state). |
| **Trigger / Outcome** | User opens `/research-pack` or clicks `/soldiers/{id}/research-pack/{state|county}` → pack view renders via HTMX swap. |
| **Inputs / Outputs** | In: scope (county / state). Out: scoped bundle view. |
| **Happy path** | (1) User picks scope. (2) Pack view renders. (3) Reviews included material. (4) Exports pack. |
| **Failure modes** | (a) Empty scope → `EmptyStateCard`. (b) Fragment endpoint → axe misfires (carve-out). |
| **DOM roots** | (no panel ID registered). |
| **Backend it calls** | `app.handleResearchPack`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/16-research-pack.md), [routes](ui-map/routes.md#research). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| wontfix | low | Axe reports false positives on HTMX fragment | — | — | r2 carve-out |

### Improvement log

- **2026-06-29**: Research Pack is a Shared Archive variant; could clarify difference in copy.

---

## 17 · Service Timeline

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Evidence-backed chronological view of a Soldier's known life or service events. |
| **Trigger / Outcome** | User clicks Timeline from Soldier Detail → `/soldiers/{id}/timeline` swaps in. |
| **Inputs / Outputs** | In: `id`. Out: timeline of Timeline Events (incl. Service Events). |
| **Happy path** | (1) User lands. (2) Reviews chronology. (3) Clicks event → source detail. |
| **Failure modes** | (a) No Findings → empty state. (b) Fragment endpoint → axe carve-out. |
| **DOM roots** | (no panel ID registered). |
| **Backend it calls** | `app.handleSoldierTimeline`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/17-service-timeline.md), [routes](ui-map/routes.md#soldiers--search--browse). |

### Bugs

None open. Audit notes: "read-only, clean".

### Improvement log

- **2026-06-29**: Add a panel ID (`panel.soldier.timeline`).
- **2026-06-29**: Distinguish Service Events (military) from other Timeline Events visually per CONTEXT.md.

---

## 18 · Unit Camaraderie

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Inferred relationship network between Soldiers based on shared units / time overlap. |
| **Trigger / Outcome** | User clicks Camaraderie from Soldier Detail → `/soldiers/{id}/camaraderie` swaps in. |
| **Inputs / Outputs** | In: `id`. Out: graph view of related Soldiers. |
| **Happy path** | (1) User lands. (2) Reviews nodes. (3) Clicks node → other Soldier's detail. |
| **Failure modes** | (a) No shared units → empty state. |
| **DOM roots** | (no panel ID registered). |
| **Backend it calls** | `app.handleSoldierCamaraderie`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/18-unit-camaraderie.md), [routes](ui-map/routes.md#soldiers--search--browse). |

### Bugs

None open.

### Improvement log

- **2026-06-29**: Add a panel ID (`panel.soldier.camaraderie`).
- **2026-06-29**: Graph layout could show weight (number of shared units) on edges.

---

## 19 · Merge Review Ledger

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Workflow for resolving conflicts during Shared Archive import. |
| **Trigger / Outcome** | User opens `/soldiers/{id}/conflict-ledger` → ledger of Local vs Incoming decisions. |
| **Inputs / Outputs** | In: `id`. Out: ledger of merge decisions + status. |
| **Happy path** | (1) User reviews ledger. (2) Picks Local or Incoming per row. (3) Submits → merge applied. |
| **Failure modes** | (a) No conflicts → empty state. |
| **DOM roots** | (no panel ID registered). |
| **Backend it calls** | `app.handleSoldierConflictLedger`, `app.handleMergeApply`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/19-merge-review-ledger.md), [routes](ui-map/routes.md#soldiers--search--browse). |

### Bugs

None open.

### Improvement log

- **2026-06-29**: Add a panel ID (`panel.soldier.conflict-ledger`).

---

## 20 · Jobs (async export pipeline)

**Status**: 🟡 recent (00f0ba9 AwaitingConfirmation; 3748db7 re-home to /jobs/{id}; 2bb145b scan/quality)

| Field | Value |
|---|---|
| **Purpose** | Status page for async jobs (PDF export, shared import, image orphan scan, quality scan, update apply). |
| **Trigger / Outcome** | Any async action → redirect to `/jobs/{id}`; user sees status panel + AwaitingConfirmation card if needed. |
| **Inputs / Outputs** | In: `id`, optional `slot`. Out: job status, artifact download, optional log download. |
| **Happy path** | (1) User redirected after async action. (2) Sees status polling. (3) On completion, sees summary card + Open/Save artifact. (4) For AwaitingConfirmation (memorial import), user confirms before processing. |
| **Failure modes** | (a) Job fails → log download button + failure count. (b) Browser open (file://) blocked by OS → Copy path fallback (`362d989`). |
| **DOM roots** | `panel.job.status`, `overlay.jobs.progress`. |
| **Backend it calls** | `app.handleJobStatus`, `app.handleJobStatusSlot`, `app.handleJobArtifactOpen`, `app.handleConfirmMemorialJSONImport`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/20-jobs.md), [routes](ui-map/routes.md#export--share--jobs). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | high | Memorial confirmation flow missing | #137 | `331b58d`, `00f0ba9`, `3748db7` | — |
| fixed | high | `Job.Snapshot` returned stale AwaitingConfirmation | — | `331b58d` | — |
| fixed | high | Shared import log not exported via enqueueExport | #137 | `4a9b275` | — |
| fixed | high | Memorial import log download button not wired in `jobSummaryCard` | — | (commit on `fix/jobs-wire-memorial-log-download`) | — |
| fixed | medium | Dead "Open file" button on `/jobs/{id}`, `/jobs/{id}/report`, and layout progress slot | #166 | (commit on `fix/jobs-remove-open-file-button`) | — |

### Improvement log

- **2026-06-29**: Wire `jobSummaryCard` to render log download when `job.Result.LogPath != ""` (per gaps.md).
- **2026-06-29**: Add a deep-link pill to Merge Review on shared-import summary when `Conflicts > 0`.
- **2026-06-29**: Jobs progress overlay (3s poll) could throttle when window not focused.
- **2026-06-30**: "Open file" button removed (does nothing in user's runtime; Copy path is the reliable fallback). ✅ **Done in commit on `fix/jobs-remove-open-file-button`** — three surfaces cleaned (`jobSummaryCard`, report artifact section, layout slot), two regression tests inverted, backend handler kept for future callers.

---

## 21 · Settings

**Status**: 🟡 recent (2bb145b scan/quality results rendering)

| Field | Value |
|---|---|
| **Purpose** | User preferences + maintenance actions: layout mode, data initialization, updates, debug toggle, image orphan scan/cleanup, image quality scan/apply. |
| **Trigger / Outcome** | User opens `/settings` → 4 panels: layout, initialize, updates, debug. Maintenance actions: orphan scan (HTMX swap to `#settings-orphan-results`), quality scan. |
| **Inputs / Outputs** | In: settings form values. Out: persisted state + scan results. |
| **Happy path** | (1) User lands. (2) Toggles layout mode → POST `/settings/layout` → toast. (3) Triggers update check → POST `/settings/update/check` → status. (4) Runs orphan scan → results render into `#settings-orphan-results`. (5) Triggers cleanup → guarded dialog → job. |
| **Failure modes** | (a) Scan/quality results not rendering into target div (fixed `2bb145b`). (b) Update apply dialog race → guarded. |
| **DOM roots** | `panel.settings.{layout,initialize,updates,debug}`, `#settings-orphan-results`, `#settings-quality-results`. |
| **Backend it calls** | `app.handleSettings`, `app.handleSettingsLayout`, `app.handleSettingsDebugMode`, `app.handleSettingsInitialize`, `app.handleSettingsUpdateSource`, `app.handleSettingsUpdateCheck`, `app.handleSettingsUpdateApply`, `app.handleSettingsImagesOrphansScan`, `app.handleSettingsImagesOrphansCleanup`, `app.handleSettingsQualityScan`, `app.handleSettingsQualityApply`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/21-settings.md), [routes](ui-map/routes.md#settings--recovery--debug). |

### Bugs

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | high | Scan/quality results not rendering into target div | — | `2bb145b` | — |
| open | high | Dialog guard audit not yet run on Settings handlers | — | — | gaps.md |

### Improvement log

- **2026-06-29**: Run dialog-guard audit on `SettingsImagesOrphansCleanup`, `SettingsQualityApply`, `SettingsUpdateApply` (per gaps.md).
- **2026-06-29**: Settings page has 4 panels + 3 maintenance actions — consider sidebar nav instead of stacked sections.
- **2026-06-29**: Debug toggle (currently a pill) could show last-N log lines inline.

---

## 22 · Initial Setup

**Status**: 🟢 stable · first-launch only

| Field | Value |
|---|---|
| **Purpose** | First-launch wizard: pick data directory, initialize Local Archive. |
| **Trigger / Outcome** | App starts with no data dir → redirects to `/setup`. User completes setup → home (`/`). |
| **Inputs / Outputs** | In: data dir path. Out: initialized Local Archive + session. |
| **Happy path** | (1) User lands (forced redirect). (2) Reviews default path. (3) Clicks Initialize → POST `/settings/initialize` → redirect to home. |
| **Failure modes** | (a) Path unwritable → toast + retry. (b) Already initialized → redirect to home (idempotent). |
| **DOM roots** | (own page, no panel ID registered). |
| **Backend it calls** | `app.handleSetup`, `app.handleSettingsInitialize`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/22-initial-setup.md), [routes](ui-map/routes.md#initial-setup). |

### Bugs

None open.

### Improvement log

- **2026-06-29**: Add `routebuilder.Setup()` constant — currently bare string in redirect (gaps.md).

---

## 23 · Recovery

**Status**: 🟢 stable

| Field | Value |
|---|---|
| **Purpose** | Recovery flow: restore from a Backup Archive or Restore Point after corruption / failed update. |
| **Trigger / Outcome** | User opens `/recovery` (or auto-redirected after failed boot) → picks restore source → confirm → restore. |
| **Inputs / Outputs** | In: Backup Archive or Restore Point selection. Out: restored Local Archive. |
| **Happy path** | (1) User lands. (2) Lists available backups + restore points. (3) Picks one + confirms. (4) Restore runs → redirect to home. |
| **Failure modes** | (a) No backups → empty state. (b) Restore fails → toast + retry. |
| **DOM roots** | (own page, no panel ID registered). |
| **Backend it calls** | `app.handleRecovery`, `app.handleRestore`. |
| **Cross-refs** | [wireframe](ui-map/wireframes/23-recovery.md), [routes](ui-map/routes.md#settings--recovery--debug). |

### Bugs

None open.

### Improvement log

- **2026-06-29**: Add `routebuilder.Recovery()` constant — currently bare string.

---

## Cross-cutting surfaces (every screen)

These are not services themselves but render on every page. Bug fixes here have app-wide impact.

### Floating dock (Scratch Pad / Feedback / Menu)

**Status**: 🔵 in redesign (audit r2 #2 — overlaps content on every page)

| Field | Value |
|---|---|
| **DOM roots** | `header.floating.dock` (fixed bottom-right). |
| **Backend** | `app.handleScratchPad`, `app.handleFeedbackSubmit`. |
| **Known issues** | Overlaps content on `/compare` mobile, `/calendar`, `/browse`, deep soldier routes. |

**Bugs**

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | high | Overlaps content on every page (audit r1 top-2, r2 confirmed) | — | (commit on `fix/layout-floating-dock-overlap`) | r2 #2 |
| fixed | medium | Floating-nav toggle bound via `addEventListener` died after htmx swap | — | `46a10cc` | — |

**Improvement log**

- **2026-06-29**: Move dock to left edge OR collapse to single icon that expands on hover — top-priority visual fix per audit. ✅ **Fixed in commit on `fix/layout-floating-dock-overlap`** (neither option chosen — the actual root cause was content-vs-dock padding, not dock positioning). Implemented the JS-measured `--floating-dock-height` + direct `padding-bottom` write per `docs/COMMON_BUGS.md §4.14` prescription.

### Top nav (8 pill-links + primary CTA)

**Status**: 🟡 audit-finding (nav-density)

**Bugs**

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| open | info | 8 nav items at 886px / 111px each (WCAG no hard limit, flagged info-only) | — | — | r3 (carried) |
| fixed | high | Mobile nav overflow 6px on 390px | #82 | r4 | r3 |

**Improvement log**

- **2026-06-29**: Add `routebuilder.NavHome()`, `NavSearch()`, `NavBrowse()`, `NavReviewQueue()`, `NavInsights()`, `NavShare()`, `NavSettings()`, `NavNew()` (per gaps.md).
- **2026-06-29**: Consider hamburger drawer always (not just mobile) given density concern.

### Toast region

**Status**: 🟢 stable · 🟡 recent (3052251 sanitise ASCII)

**Bugs**

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | high | Toast header with non-ASCII crashed HTTP/1.x | — | `3052251` | — |
| fixed | low | Broken `\u2026` escape in toast labels | — | `d91e32c` | — |

### Jobs progress overlay (3s poll)

**Status**: 🟢 stable

**Bugs**

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| open | low | Polls even when window not focused | — | — | — |

**Improvement log**

- **2026-06-29**: Pause poll when `document.visibilityState === 'hidden'` (battery + network politeness).

### Modals (image viewer, print-config, google-prefs, feedback)

**Status**: 🟢 stable · LAW: native `<dialog>` forbidden

**Bugs**

| status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|
| fixed | blocker | Native `<dialog>` caused WebView2 focus reentry crash | #117 | revert | — |

**Improvement log**

- **2026-06-29**: Re-evaluate native `<dialog>` after Wails upstream fix (CONTEXT.md law).

---

## Bug Ledger (all services, triage view)

Sortable by severity. Re-run quarterly.

| service | status | severity | summary | issue# | fix-commit | audit-ref |
|---|---|---|---|---|---|---|
| Settings | open | high | Dialog guard audit not yet run on Settings handlers | — | — | gaps.md |
| Export | open | high | Dialog guard audit not yet run on Export handlers | — | — | gaps.md |
| Jobs | open | high | Memorial import log download button not wired in `jobSummaryCard` | — | — | gaps.md |
| Floating dock | open | high | Overlaps content on every page | — | — | r2 #2 |
| Browse | fixed | medium | Browse-row text labels <24×24 on mobile (WCAG 2.5.5) — see audit finding note above; readability fix landed | — | this branch | r3 |
| Jobs | open | low | Polls even when window not focused | — | — | — |
| Calendar | open | low | `/calendar` not probed by smoke `routes_registered` | — | — | — |
| Compare | open | low | Cell padding tight (6px vs 8px) | — | — | r3 |
| Calendar Day | open | nit | Empty state copy could be more explicit | — | — | audit/notes-2026-06-29 |
| Top nav | open | info | 8 nav items at 886px (info-only) | — | — | r3 |

## Cross-cutting improvements

Themes that span multiple services. Track here until they graduate to per-service cards.

### Audit harness

- **2026-06-29**: Carve out HTMX fragment endpoints from axe scans (r2 recommendation, repeated across services 10/16/17/18/19).
- **2026-06-29**: Add richer seed data with seeded duplicates + merge conflicts for Review Queue + Merge Review surfaces.
- **2026-06-29**: Tighten walker locators to avoid false-positives (Browse walker hitting `/soldiers/new` button first).
- **2026-06-29**: Add `audit-notes-TEMPLATE.md` "automated" column since most surfaces are auto-walked (per audit/notes-2026-06-29 follow-up).

### Routebuilder coverage

- **2026-06-29**: Add `routebuilder.Setup()`, `routebuilder.Recovery()` (per gaps.md).
- **2026-06-29**: Add nav link builders: `NavHome()`, `NavSearch()`, `NavBrowse()`, `NavReviewQueue()`, `NavInsights()`, `NavShare()`, `NavSettings()`, `NavNew()` (per gaps.md).

### Panel ID coverage (uiids)

- **2026-06-29**: Research services (13, 14, 15, 16) have no panel IDs registered — add `panel.research-collections.hub`, `panel.research-collection.detail`, `panel.research-log`, `panel.research-pack`.
- **2026-06-29**: Soldier sub-views (17, 18, 19) have no panel IDs — add `panel.soldier.{timeline,camaraderie,conflict-ledger}`.

### Component cleanup

- **2026-06-29**: Resolve `partials/empty_state.templ` vs `components/empty_state.templ` duplication (per gaps.md).
- **2026-06-29**: Atomic component coverage in `components/` (button/card/empty_state/field/pill/toast) is correct; consider adding `tooltip.templ`, `tabs.templ`, `drawer.templ` if reused.

### Dialog-guard law

- **2026-06-29**: Run dedicated dialog-guard audit on `ExportBackup`, `ExportDatabasePDFAsync`, `SettingsImagesOrphansCleanup`, `SettingsQualityApply`, `SettingsUpdateApply` (per gaps.md + CONTEXT.md law).

### Polish (round-5 candidates)

- **2026-06-29**: Reduce header pill density on mobile (currently wraps to 2 rows on small viewports).
- **2026-06-29**: Standardise stats tile heights on `/calendar` (audit r1).
- **2026-06-29**: Investigate 5.7s load on `/soldiers` + `/soldiers/search/advanced` (audit r1 #11) — recent commit `5b022a7` addressed one but worth re-measuring.

### Domain model alignment (CONTEXT.md)

- **2026-06-29**: Scratch Pad (informal) vs Research Log (structured) deserve clearer UI distinction.
- **2026-06-29**: Service Event vs Timeline Event could use visual differentiation in service timeline (17).
- **2026-06-29**: Research Collection vs Research Pack boundary could be clearer in copy.

## How to update this doc

- **New bug found** → add row to service's bug table + add to Bug Ledger.
- **Bug fixed** → mark `fixed`, add fix-commit, keep row (history).
- **Improvement landed** → mark `done` with date + commit.
- **New service added** → append in user-journey order + update Journey Map.
- **Service removed** → mark `⚪ deprecated` + add Improvement Log entry explaining why.

## See also

- [docs/ui-map/README.md](ui-map/README.md) — screen × DOM × component matrix (orthogonal)
- [docs/ui-map/routes.md](ui-map/routes.md) — every route → handler (URL-first)
- [docs/ui-map/wireframes/](ui-map/wireframes/) — per-screen ASCII wireframes
- [docs/ui-map/gaps.md](ui-map/gaps.md) — architectural debt surfaced by UI map
- [docs/CONTEXT.md](../CONTEXT.md) — domain glossary + non-negotiable laws
- [docs/COMMON_BUGS.md](COMMON_BUGS.md) — bug-pattern catalog by layer
- [docs/agents/dialog-guard.md](agents/dialog-guard.md) — native dialog guard law
- [audit/notes-2026-06-29.md](../audit/notes-2026-06-29.md) — latest audit round