# 04 — Browse (Local Archive)

- **Route**: `/browse` (full page) + `/browse/results` (HTMX fragment)
- **Builder**: `routebuilder.BrowseResults()`
- **Template**: `internal/templates/browse.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Browse Local Archive ──────────────────────────────────────────┐
│ h2 + "Default order: Display ID" note                           │
│                                                                    │
│ [Filters <details>] (collapsible, badge shows active count)       │
│   Scope | Sort | Page Size | Entry Type | Pension State           │
│   Review Status | Unit (xl:2) | Buried In (xl:2)                  │
│   Confederate Home Status (xl:2)                                 │
│   <btn: Apply> <btn: Reset Browse (ghost)>                       │
│                                                                    │
│ [Columns <details>] (always visible toggle region)                │
│   8 column toggles (display_id, name, entry_type, rank_out,       │
│                      unit, pension_state, review_status, last_edited)│
│   "Print/Export Selected" (→ /share?openPrintConfig=1)            │
│   <btn: Clear Selection>                                         │
│   "Select records across pages …" status                         │
│                                                                    │
│ [panel.browse.results] #browse-results                           │
│   summary card (scope | sort + active filter chips + N records)  │
│   [BrowsePager top]                                              │
│   if records == 0:                                               │
│     <EmptyState "No person records matched" …>                   │
│   else:                                                           │
│     mobile cards (<md) OR table (md+) with select + cols         │
│       checkbox per row → cross-page selection                    │
│   [BrowsePager bottom]                                           │
└──────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `panel.browse.results` | Results table / cards | Sortable, paginated, selectable |
| Filters details | Top collapsible card | 8 filter inputs |
| Columns details | Mid collapsible card | Column visibility toggles |

No tabs. No overlays local.

## Atomic components

- `Button` — Apply, Reset Browse, Clear Selection, primary nav CTAs.
- `Field` — every filter input/select.
- `Card` — wrapper for filters/results/pager.
- `EmptyState` — zero-results copy.
- `Pill` — pager links, "View" links.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Filter form submit | GET | `routebuilder.BrowseResults()` | `#browse-results` | default | Carries all filter state via `name=` attrs |
| Page link click | GET | `browsePageHref(state, page)` | `#browse-results` | default | `data-browse-page-link` |

Filter inputs use `data-browse-filter-input` for any JS-driven
auto-apply (verify behavior — may not auto-apply, may require
explicit Apply click).

## Modals / overlays

None local. Global floating menu / feedback / jobs only.

## State variants

- **Empty (no records at all)**: filters card still renders, results
  show EmptyState.
- **No matches**: EmptyState "No person records matched / Adjust the
  browse scope or filters and try again."

## Footguns

- **Multi-page selection persistence** — `data-browse-select` +
  `data-browse-clear-selection` + `data-browse-selection-status`.
  Verify selection survives pager navigation via client-side store
  (`frontend/app.js`). Drift between browse and Share ("Print/Export
  Selected") is a likely bug.
- **Mobile cards vs desktop table** — same data, different DOM. Tests
  must assert both branches.
- **Column toggle** — `data-browse-column-toggle` JS state in
  `localStorage` likely. Verify default columns.
- **`browsePageHref` is a templ function** (not `routebuilder`).
  Reconstructed per page link. Hand-roll of query params is fragile.
- **`data-browse-row-href`** on `<tr>` — clicking a row should
  navigate to the soldier. Verify single-row click handler.
- **Filter form has hidden `page` input** so pager state is preserved
  on filter submit.
- **Active filter chips** rendered from `browseActiveFilters(state)` —
  verify all filter fields are represented.

## See also

- [03-soldiers-list.md](03-soldiers-list.md) (sibling search UX)
- [08-export.md](08-export.md) (Print/Export Selected deep link)