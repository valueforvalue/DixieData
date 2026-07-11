# Global Layout Shell

- **Route**: every route (the chrome that wraps every page)
- **Builder**: N/A (layout primitive, not a route)
- **Template**: `internal/templates/layout.templ`
- **Layout**: N/A (the shell that hosts the per-page layout mode)
- **Owner**: package `templates`
- **Audit**: issue #264 (Share foldout pattern), issue #378 (Research foldout), issue #455 (Review Queue relocation into the R&R foldout + slim Soldier Card R&R tiles)

## Purpose

The persistent chrome around every page. Hosts three regions:
the **top nav** (`<nav class="top-shell">`) for primary destinations,
the **breadcrumb** (under the top nav) for current-page context, and
the **floating dock** (fixed to the bottom) for persistent affordances
(Scratch Pad, Feedback, Menu, Share Queue pill).

The top nav carries the current 11 destinations (10 pills + 1
primary CTA) per issue #380's inventory. Per issue #380 Open Question
4, foldouts appear **only in the top nav** — the floating-dock
Quick Nav stays flat pills only.

## Top nav (rendered inside `<header class="top-shell">`)

```
[ DixieData — {Codename} ] ← brand pill, top-left

[Calendar][Search/Quick View][Browse][Events][Articles][Insights]
   ┌──────────────────────────┐    ┌──────────────────┐
   │ Research & Review    [▾] ┄┼────┤ Share        [▾] ┼─
   └──────────────────────────┘    └──────────────────┘
   [Tags][Settings][+ Add Person Record]                      ← primary CTA
```

### Top nav destinations (locked-in post #455)

| # | Trigger | Type | Sub-destinations | Surface IDs |
| --- | --- | --- | --- | --- |
| 1 | Calendar | pill | `/calendar`, `/calendar/*`, `/anniversary/*` | — |
| 2 | Search/Quick View | pill | `/soldiers` | — |
| 3 | Browse | pill | `/browse`, `/browse/results` | — |
| 4 | Events | pill | `/events`, `/events/{id}*` | — |
| 5 | Articles | pill | `/articles`, `/articles/{id}*` | `data-article-nav-link` |
| 6 | Insights | pill | `/insights`, `/insights/drilldown` | — |
| 7 | **Research & Review** | **foldout** | Open Review Queue · Open Timeline · Open Research Log · Research Collections · Change Person… | `layout.research.menu` + `layout.research.menu.trigger`; badge `data-layout-research-review-count` (live-count span via `/layout/review-count`, hx-trigger="load, every 30s") |
| 8 | **Share** | **foldout** | Export · Import · Share Queue · Sync | `layout.share.menu` + `layout.share.menu.trigger` |
| 9 | Tags | pill | `/tags` | `layout.tags.link` |
| 10 | Settings | pill | `/settings` | — |
| 11 | Add Person Record | primary CTA | `/soldiers/new` | — |

### Foldout internals

**Research & Review** (`layout.research.menu`) — `<ul role="menu">` of `<li role="none">` rows; each row is a `<a role="menuitem" class="foldout-menuitem pill-link">`:

| Row | href | Data-attribute marker |
| --- | --- | --- |
| Open Review Queue | `/review-queue` (or red-bordered variant when `layoutHasOpenReview(ctx)` is true) | `data-research-menu-review-queue`, `data-research-review-has-count` |
| Open Timeline | `routebuilder.ResearchPicker() + "?next=timeline"` | `data-research-menu-timeline` |
| Open Research Log | `routebuilder.ResearchPicker() + "?next=research-log"` | `data-research-menu-research-log` |
| Research Collections | `/research-collections` | `data-research-menu-research-collections` |
| Change Person… | `/research` | `data-research-menu-change-person` |

**Share** (`layout.share.menu`) — same `<ul role="menu">` shape:

| Row | href | Data-attribute marker |
| --- | --- | --- |
| Export | `routebuilder.ShareExports()` | `data-share-menu-export` |
| Import | `routebuilder.ShareImports()` | `data-share-menu-import` |
| Share Queue | `routebuilder.ShareQueuePage()` | `data-share-queue-page-link` |
| Sync | `routebuilder.ShareSync()` | `data-share-menu-sync` |

### Top-nav badges

- **Research & Review trigger** carries a live-count badge inside
  the trigger button (between label and chevron) — `<span
  data-layout-research-review-count hx-get="/layout/review-count"
  hx-trigger="load, every 30s" hx-swap="innerHTML" hx-target="this"
  hx-ext="none"></span>`. Rendered via `components.FoldoutWithBadge`
  (new variant in issue #455 slice 1.5). When the queue has 0 open
  items the badge renders an empty span; when non-zero, it renders
  the count number. The Share foldout keeps the plain `Foldout()`
  API (no badge).

## Floating dock (fixed bottom)

```
┌────────────────────────────────────────────────────────────────────────┐
│ data-floating-scratchpad-status (live region, aria-live=polite)        │
│ [Scratch Pad btn]  [Feedback btn]  [Menu btn]   ← primary nav-toggle   │
└────────────────────────────────────────────────────────────────────────┘
```

Plus a separately-positioned **Share Queue status pill** (issue #182):

```
┌────────────────────────────────────────────────────┐
│ Share queue: [N]  Review & export                  │ ← bottom-[6.5rem]
└────────────────────────────────────────────────────┘
```

Hidden when queue is empty (`hidden` class toggled by JS).

### Menu btn → Quick Nav panel (issue #283)

When the Menu btn is clicked, the floating-dock Quick Nav panel
toggles visible (`data-floating-nav-panel` classList.toggle('hidden')):

```
┌─ Quick Navigation ──────────────────────────────────┐
│ [Calendar] [Search/Quick View] [Browse]             │
│ [Review Queue] [Insights] [Share]                    │
│ [Tags] [Settings] [+ Add Person Record]              │
│ ─────────────────────────────────────────────────── │
│ Layout Mode: [Relaxed] [Auto/Compact/Split btn]     │
└──────────────────────────────────────────────────┘
```

**Per #380 Open Question 4**, the Quick Nav panel mirrors **flat
pills only** — it does NOT include the foldouts (no "Research &
Review" entry, no "Share" foldout entry). Review Queue appears
as a flat pill here (it's no longer a top-nav pill post #455
slice 1.5).

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Research & Review badge | GET | `routebuilder.LayoutReviewCount()` | `[data-layout-research-review-count]` | `innerHTML` | `hx-trigger="load, every 30s"`, `hx-ext="none"` (no other htmx processing) |
| Jobs progress overlay (when debug mode) | GET | `routebuilder.ActiveJobs()` | `.jobs-progress-overlay` | `innerHTML` | `hx-trigger="load, every 3s"`; suppressed when current page already shows the job's progress inline |
| Layout mode switcher (inside floating dock) | GET | `/layout/mode?value=…` | (page) | (default) | Picks Relaxed / Compact / Split; persists to `localStorage[dixiedata.layout.mode]` |

## Footguns

- **Search/Quick View label** — the top-nav + dock Quick Nav
  both render this label. Issue #380's open amendment notes the
  label should change to "Search" (URL stays `/soldiers`). The
  change touches `layout.templ` × 2, `breadcrumb_helpers.go`
  × 3, `audit/smoke.mjs` selectors, the wireframe (this file),
  and the user manual. Not yet applied.
- **Floating-dock mirror policy** — foldouts must NOT mirror
  to the dock. Confirm this holds if a Tools foldout ships per
  #380 Phase 2.
- **`/jobs/{id}` placement** — currently dock-only via the
  active-jobs overlay (when a job is running). Per #380 OQ5,
  could be promoted into the Tools foldout OR the Share foldout
  OR stay dock-only. Maintainer call.
- **Settings sub-menu** — per #380 OQ3, Settings could grow a
  sub-menu (Debug Mode, Updates, Image Orphans, Quality,
  Initialize) without becoming a foldout. Decision pending.
- **Review Queue badge plumbing** — the `/layout/review-count`
  endpoint is read by the Research & Review badge. If the
  endpoint changes shape, both `internal/appshell/app.go` and
  the JS-driven swap target (via the span attribute) must move
  together. Pin: existing appshell test covers the endpoint
  contract.
- **Breadcrumb vs dev badge** — issue #309 invariant: the
  breadcrumb + dev badge must always agree on the current
  page. If they disagree, layout is broken. Both read from
  `layoutCurrentPath(ctx)`.

## Sections locked / pending (per #380 Phase 1 decisions)

| Section | Status | Notes |
| --- | --- | --- |
| **A: Research & Review foldout** | ✅ Shipped (#378 + #455 slices 1 + 1.5) | Documented above |
| **B: Tools foldout (Insights · Tags · Jobs · Debug Console)** | ⏳ Pending | Per #380 OQ2 maintainer call — recommend keep flat for v1 unless /jobs/{id} promoted |
| **C: Settings sub-menu (sub-actions vs page-internal)** | ⏳ Pending | Per #380 OQ3 maintainer call — recommend "page is the hub" pattern (no sub-menu) |
| **D: Promote `/jobs/{id}` to Tools or Share foldout** | ⏳ Pending | Per #380 OQ5 maintainer call |
| **E: Drop `POST /events/{id}/sources/attach`** | ⏳ Pending | Per #380 Phase 3; author note at `routes.go:117-123` documents "orphan-by-design" for `.ddshare` replay (issue #360). If kept, surface as intentional-orphan in `gaps.md`; if dropped, verify no `.ddshare` replay path uses it first |
| **F: Records cluster (Search · Browse · Events · Articles)** | ⏳ Pending | Per #380 OQ1 maintainer call — recommend keep flat (each has a distinct workflow) |
| **G: Floating-dock mirror policy** | ✅ Locked | Foldouts stay top-nav only; dock stays flat pills |
| **H: "Search/Quick View" → "Search" label** | ⏳ Pending | Per #380 amendment — label change only, URL stays `/soldiers` |

When a decision lands, this wireframe + `surfaces.md` + `routes.md`
need a follow-up edit. Sections B/C/D remain blocked on Phase 1
per issue #381.

## See also

- [research-picker.md](research-picker.md) — picker landing the R&R foldout routes to
- [13-research-collections-hub.md](13-research-collections-hub.md) — R&R sub-page
- [14-research-collection-detail.md](14-research-collection-detail.md) — R&R sub-page
- [15-research-log.md](15-research-log.md) — R&R sub-page
- [08-export.md](08-export.md) — Share foldout → Exports
- [08b-imports.md](08b-imports.md) — Share foldout → Imports
- [11-review-queue.md](11-review-queue.md) — Open Review Queue entry (lives inside the R&R foldout post #455)
- `docs/ui-map/surfaces.md` — `layout.research.menu{,.trigger}`, `layout.share.menu{,.trigger}`, `layout.tags.link`
- `docs/ui-map/gaps.md` — buried-surface decisions + intentionally-orphan routes
