# Global Layout Shell

- **Route**: every route (the chrome that wraps every page)
- **Builder**: N/A (layout primitive, not a route)
- **Template**: `internal/templates/layout.templ`
- **Layout**: N/A (the shell that hosts the per-page layout mode)
- **Owner**: package `templates`
- **Audit**: issue #264 (Share foldout pattern), issue #378 (Research foldout), issue #455 (Review Queue relocation into the R&R foldout), issue #380 (mega-menu restructure — slices 1–7)

## Purpose

The persistent chrome around every page. Hosts three regions:
the **top nav** (`<nav class="top-shell">`) for primary destinations,
the **breadcrumb** (under the top nav) for current-page context, and
the **floating dock** (fixed to the bottom) for persistent affordances
(Scratch Pad, Feedback, Menu, Share Queue pill).

The top nav carries **5 visible items** post-#380: Calendar pill
+ Records mega-menu + Share & Review mega-menu + Settings pill +
Add Person Record CTA. The 2 mega-menus each expose a 2D panel
(NN/g mega-menu pattern — https://www.nngroup.com/articles/mega-menus-work-well/)
with grouped items; flat pills stay flat. Per issue #380 OQ4,
foldouts/mega-menus appear **only in the top nav** — the floating-dock
Quick Nav stays flat and only mirrors the top-nav flat pills + the
first item of each mega-menu.

## Top nav (rendered inside `<header class="top-shell">`)

```
[ DixieData — {Codename} ] ← brand pill, top-left

[Calendar][Records ▾]          [Share & Review ▾]   [Settings][+ Add Person Record]
```

### Top nav destinations (post-#380)

| # | Trigger | Type | Sub-destinations | Surface IDs |
| --- | --- | --- | --- | --- |
| 1 | Calendar | pill | `/calendar`, `/calendar/*`, `/anniversary/*` | — |
| 2 | **Records** | **mega-menu** | Search · Events · Tags · Browse · Articles (5 items, 2 groups) | `layout.records.menu` + `layout.records.menu.trigger`; menuitem markers `data-marker="records-search"\|"records-events"\|"records-tags"\|"records-browse"\|"records-articles"` |
| 3 | **Share & Review** | **mega-menu** | Review Queue (with live-count badge) · Open Timeline · Open Research Log · Research Collections · Change Person… · Insights · Share landing · Export · Import · Share Queue · Sync (11 items, 2 columns) | `layout.share-review.menu` + `layout.share-review.menu.trigger`; menuitem markers `data-research-menu-*` + `data-share-menu-*`; badge `data-layout-research-review-count` (live-count span via `/layout/review-count`, hx-trigger="load, every 30s") |
| 4 | Settings | pill | `/settings` | — |
| 5 | Add Person Record | primary CTA | `/soldiers/new` | — |

### Records mega-menu internals (`layout.records.menu`)

`<div role="menu">` containing a 2D `grid grid-cols-2` wrapper
with one `<section>` per group. Each group has an `<h3>` heading
(used for both visual label and `aria-labelledby` on the section)
followed by a `<ul>` of items. Each item is a real `<a role=
"menuitem" href="...">`.

**Column 1: People**

| Item | href | Data marker |
| --- | --- | --- |
| Search | `/soldiers` | `data-marker="records-search"` |
| Events | `/events` | `data-marker="records-events"` |
| Tags | `/tags` | `data-marker="records-tags"` |

**Column 2: More records**

| Item | href | Data marker |
| --- | --- | --- |
| Browse | `/browse` | `data-marker="records-browse"` |
| Articles | `/articles` | `data-marker="records-articles"` |

### Share & Review mega-menu internals (`layout.share-review.menu`)

Same 2D shape as Records mega-menu (NN/g mega-menu pattern).
Two columns.

**Column 1: Review & Research** (6 items)

| Item | href | Data marker |
| --- | --- | --- |
| Open Review Queue | `/review-queue` (or red-bordered variant when `layoutHasOpenReview(ctx)` is true) | `data-research-menu-review-queue`, `data-research-review-has-count` |
| Open Timeline | `routebuilder.ResearchPicker() + "?next=timeline"` | `data-research-menu-timeline` |
| Open Research Log | `routebuilder.ResearchPicker() + "?next=research-log"` | `data-research-menu-research-log` |
| Research Collections | `/research-collections` | `data-research-menu-research-collections` |
| Change Person… | `/research` | `data-research-menu-change-person` |
| Insights | `/insights` | — |

**Column 2: Share** (5 items; `/share` landing is the NEW first item)

| Item | href | Data marker |
| --- | --- | --- |
| Share landing | `/share` | — |
| Export | `routebuilder.ShareExports()` | `data-share-menu-export` |
| Import | `routebuilder.ShareImports()` | `data-share-menu-import` |
| Share Queue | `routebuilder.ShareQueuePage()` | `data-share-queue-page-link` |
| Sync | `routebuilder.ShareSync()` | `data-share-menu-sync` |

### Mega-menu trigger button

`<button type="button" class="pill-link top-nav-link inline-flex items-center gap-1" aria-haspopup="menu" aria-expanded="false" aria-controls="{menuID}" data-mega-menu-trigger="{menuID}">Label <span aria-hidden="true">▾</span></button>`

Panel: `<div id="{menuID}" role="menu" aria-label="{Label} menu" data-mega-menu-panel="{menuID}" class="mega-menu-panel absolute right-0 top-full z-50 mt-2 hidden w-[36rem] max-w-[calc(100vw-2rem)] rounded-2xl border border-[#8d7440] bg-[rgba(36,48,61,0.96)] p-6 shadow-[0_24px_44px_rgba(23,33,43,0.28)]">`

JS dispatcher: `installMegaMenus()` in `frontend/app.js` runs
alongside `installFoldouts()` from both `bootstrapAfterHTMLSwap`
+ `initializeAll`. Same open/close contract (click toggles,
outside-click closes, ESC closes + returns focus). Arrow-key
navigation falls through to the browser default (Tab cycles
through menuitems in source order).

### Mega-menu top-nav badge

The **Share & Review** trigger carries a live-count badge inside
the button (between label and chevron). Wire shape is
byte-identical to the pre-#380 foldout badge (issue #455 slice
1.5):

```
<span data-layout-research-review-count hx-get="/layout/review-count"
  hx-trigger="load, every 30s" hx-swap="innerHTML"
  hx-target="this" hx-ext="none"></span>
```

Rendered via `components.MegaMenuWithBadge` (new variant added
in issue #380 slice 3). The Records mega-menu uses plain
`components.MegaMenu` (no badge).

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

### Menu btn → Quick Nav panel (issue #283, parity lock #380 OQ4)

When the Menu btn is clicked, the floating-dock Quick Nav panel
toggles visible (`data-floating-nav-panel` classList.toggle('hidden')):

```
┌─ Quick Navigation ──────────────────────────────────┐
│ [Calendar]                                            │
│ [Search]          ← first item of Records mega-menu  │
│ [Share landing]   ← first item of Share & Review mm  │
│ [Settings]                                          │
│ [+ Add Person Record]                                │
│ ─────────────────────────────────────────────────── │
│ Layout Mode: [Relaxed] [Auto/Compact/Split btn]     │
└──────────────────────────────────────────────────┘
```

**Per #380 OQ4 lock**, the Quick Nav panel mirrors **flat pills
+ first item of each mega-menu only** (5 items post-#380,
down from 9). It does NOT include Browse, Events, Articles, Tags,
Review Queue, Insights, Export, Import, Share Queue, Sync — all
reachable via the top-nav mega-menu triggers.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Share & Review badge | GET | `routebuilder.LayoutReviewCount()` | `[data-layout-research-review-count]` | `innerHTML` | `hx-trigger="load, every 30s"`, `hx-ext="none"` |
| Jobs progress overlay (when debug mode) | GET | `routebuilder.ActiveJobs()` | `.jobs-progress-overlay` | `innerHTML` | `hx-trigger="load, every 3s"`; suppressed when current page already shows the job's progress inline |
| Layout mode switcher (inside floating dock) | GET | `/layout/mode?value=…` | (page) | (default) | Picks Relaxed / Compact / Split; persists to `localStorage[dixiedata.layout.mode]` |

## Footguns

- **Mega-menu vs foldout selector confusion** — `[data-mega-menu-*]`
  is parallel to (not a replacement for) `[data-foldout-*]`. Both
  JS dispatchers (`installFoldouts` + `installMegaMenus`) run
  independently. If a new top-nav dropdown uses the mega-menu
  primitive, prefer `components.MegaMenu` / `MegaMenuWithBadge`
  for >5 items grouped into 2+ columns; use `Foldout` only for
  flat dropdowns of ≤5 items.
- **Floating-dock mirror policy** — foldouts + mega-menus stay
  top-nav exclusive. Dock mirrors flat pills + first mega-menu
  item each only. The OQ4 lock is non-negotiable; if a future
  Tools mega-menu ships, the dock gains the first Tool as a flat
  pill but no deeper.
- **`/jobs/{id}` placement** — NOT a nav destination (it's a
  redirect sink after every export). The persistent
  `jobs-progress-overlay` is the real surface. Per #380 OQ5,
  this is locked as "no nav promotion".
- **Settings sub-menu** — per #380 Phase 1, Settings stays a
  flat pill pointing at the page. The page itself acts as the
  hub for sub-actions (Debug Mode, Updates, Image Orphans,
  Quality, Initialize). No sub-menu.
- **Review Queue badge plumbing** — the `/layout/review-count`
  endpoint is read by the Share & Review trigger's badge. If the
  endpoint changes shape, both `internal/appshell/app.go` and
  the JS-driven swap target (via the span attribute) must move
  together. Pin: existing appshell test covers the endpoint
  contract.
- **Breadcrumb vs dev badge** — issue #309 invariant: the
  breadcrumb + dev badge must always agree on the current
  page. If they disagree, layout is broken. Both read from
  `layoutCurrentPath(ctx)`. The "Search/Quick View" → "Search"
  label rename (#380 slice 5) updated both halves.
- **Issue #460 open-count flag** — the "Open Review Queue"
  menuitem gains `data-research-review-has-count` + the red
  border classes when `layoutHasOpenReview(ctx)` is true. The
  flag echo logic moved from inside the foldout's children
  block into a `reviewQueueMenuItem()` helper in
  `internal/templates/review_queue_menuitem.go` (templ's
  parser can't bind an `if/else` inside a Go slice literal,
  so the helper returns the menuitem HTML as a string).

## Sections locked (post-#380 Phase 2)

| Section | Status | Notes |
| --- | --- | --- |
| **A: Research & Review mega-menu** | ✅ Shipped (#380 slice 3) | Absorbed the prior R&R foldout + the standalone Insights pill into the Share & Review mega-menu |
| **B: Tools foldout** | ✅ NOT SHIPPED | Per #380 OQ2, no Tools foldout / mega-menu in v1 — current destinations reachable via Share & Review mega-menu |
| **C: Settings sub-menu** | ✅ NOT SHIPPED | Per #380 OQ3, page-internal pattern; Settings stays a flat pill |
| **D: `/jobs/{id}` placement** | ✅ Locked as "no nav" | Per #380 OQ5, the persistent `jobs-progress-overlay` is the surface; `/jobs/{id}` is a redirect sink only |
| **E: `POST /events/{id}/sources/attach`** | ✅ DELETED (#380 slice 6) | Stale bookmark-compat remnant; app does not support bookmarks; handler + dispatcher entry + chi-route registration + test all removed |
| **F: Records cluster (Search · Browse · Events · Articles · Tags)** | ✅ Shipped (#380 slice 2) | 5 flat pills collapsed into one Records mega-menu with 2 groups (People + More records) |
| **G: Floating-dock mirror policy** | ✅ Locked | Foldouts + mega-menus stay top-nav only; dock mirrors flat pills + first mega-menu item each only |
| **H: "Search/Quick View" → "Search" label** | ✅ Shipped (#380 slice 5) | Label change only; URL stays `/soldiers`; touched `breadcrumb_helpers.go` × 7 + Go + JS breadcrumb witnesses (issue #309 must agree) |

All 8 sections from #380 Phase 1 are now locked + shipped.

## See also

- [research-picker.md](research-picker.md) — picker landing the Share & Review mega-menu routes to
- [13-research-collections-hub.md](13-research-collections-hub.md) — R&R sub-page
- [14-research-collection-detail.md](14-research-collection-detail.md) — R&R sub-page
- [15-research-log.md](15-research-log.md) — R&R sub-page
- [08-export.md](08-export.md) — Share & Review mega-menu → Exports
- [08b-imports.md](08b-imports.md) — Share & Review mega-menu → Imports
- [11-review-queue.md](11-review-queue.md) — Open Review Queue entry (lives inside Share & Review mega-menu)
- `docs/ui-map/surfaces.md` — `layout.records.menu{,.trigger}`, `layout.share-review.menu{,.trigger}`
- `docs/ui-map/gaps.md` — buried-surface decisions + intentionally-orphan routes (POST /sources/attach now retired)