# 03 — Soldiers List (Search / Quick View)

- **Route**: `/soldiers` (GET, full page render)
- **Builders**: `routebuilder.SoldierSearch(browse)`,
  `routebuilder.SoldierSearchAdvanced()`,
  `routebuilder.SoldierScrapeFindAGrave()`
- **Template**: `internal/templates/soldier_card.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Person Records ────────────────────────────────────────────────┐
│ h2 + [+ Add Person Record]                                       │
│ [tab: tab.soldiers.search.basic]  Quick Search          (active) │
│ [tab: tab.soldiers.search.advanced] Advanced Search              │
│                                                                    │
│ [panel.soldiers.search.basic] (visible when basic tab active)     │
│  <input name=q …>  — hx-get SoldierSearch(false) → #soldier-list  │
│  <btn> Browse Alphabetically </btn>                                │
│  <p> hint copy </p>                                                │
│                                                                    │
│ [panel.soldiers.search.advanced] (visible when advanced tab)      │
│   <form> → hx-get SoldierSearchAdvanced() → #soldier-list         │
│     [Display ID] [Entry Type] [Source Record Type]                │
│     [First/Middle/Last/Maiden/Relationship]                       │
│     [Rank In/Out] [Unit] [Pension State]                          │
│     [Confederate Home Status/Name] [Buried In] [Status]           │
│     [Birth Year/Through] [Death Year/Through] [Birth/Death Date] │
│     <btn: Run Advanced Search>  <btn: Reset Filters (ghost)>      │
│   + 9 SuggestionDatalist for autocomplete                         │
│                                                                    │
│ [panel.soldiers.results] #soldier-list                            │
│   if active search:                                               │
│     "Matched Results / Browse Results / Recently Accessed" card   │
│     "Manual Comparison" row — [Compare Selected (disabled)]      │
│   for each soldier:                                               │
│     [Compare checkbox + Quick View btn] + [SoldierCard]          │
│     [SearchPreviewContent hidden, opened via Quick View]          │
│   empty-state variants by search mode                             │
│   pagination nav at bottom                                        │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `tab.soldiers.search.basic` | Tab trigger | Quick Search |
| `tab.soldiers.search.advanced` | Tab trigger | Advanced Search |
| `panel.soldiers.search.basic` | Tab panel | Quick search input + Browse Alphabetically |
| `panel.soldiers.search.advanced` | Tab panel | 20+ field advanced search form |
| `panel.soldiers.results` | Results region (`#soldier-list`) | Cards, comparison header, empty states, pagination |

Tab switching driven by `data-tab-group="soldier-search"` attributes
(JS in `frontend/app.js`), NOT HTMX.

## Atomic components

- `Button` — Quick View, Compare Selected, Browse Alphabetically,
  Run/Reset.
- `Field` — every advanced search input/select.
- `Card` — wraps search results.
- `EmptyState` — archive empty / no results / no recent.
- `Pill` — View Person Record on SoldierCard.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Quick search input | GET | `routebuilder.SoldierSearch(false)` | `#soldier-list` | default | `hx-sync="this:replace"`, `input changed delay:300ms from:input[name='q']` |
| Browse Alphabetically btn | GET | `routebuilder.SoldierSearch(true)` | `#soldier-list` | default | |
| Advanced search form submit | GET | `routebuilder.SoldierSearchAdvanced()` | `#soldier-list` | default | |
| Pagination | GET | `pageRequestURL(search, page)` | `#soldier-list` | default | |

`reset` on the advanced form is HTML5 native; `app.js` may need to
also clear server state. Verify.

## Modals / overlays

Global only (floating menu, feedback, jobs).

## State variants

- **Empty archive**: `emptyArchiveSetupCard(search)` —
  links to `/soldiers/new` and Settings → Initialize.
- **No results — quick search**: "Nothing in the local archive matches X."
- **No results — advanced**: "Adjust the advanced filters."
- **No recent**: "No recent person records yet."
- **No browse results**: "No person records in browse mode."
- **Recent (default landing)**: shows last 10 person records opened.

## Footguns

- **Tab panel switch** uses `hidden` class on `data-tab-panel`. If
  `frontend/app.js` doesn't wire `data-tab-default="true"` on the
  active tab, both panels show on first load.
- **Quick View** is a `data-preview-open` JS toggle, not HTMX. If the
  preview fragment (`SearchPreviewContent`) is heavy, every card open
  re-injects it. Verify the fragment is lightweight.
- **Compare selection** uses `data-checkbox-group="search-compare"`.
  Multiple selection groups exist across the app — verify each screen
  uses a unique group name so cross-screen compare doesn't bleed.
- **Recent list** — viewmodel derives from "recently opened"
  tracking. Verify session/localStorage hygiene.
- **Bare `/soldiers/new` href in template** — candidate for
  `routebuilder.SoldierNew()`.
- **Bare `/soldiers/{id}` hrefs on SoldierCard** — same.

## See also

- [05-soldier-detail.md](05-soldier-detail.md)
- [04-browse.md](04-browse.md) (separate page, same domain)
- [13-research-collections-hub.md](13-research-collections-hub.md) (compare flow)