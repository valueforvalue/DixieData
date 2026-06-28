# 10 — Insights Drilldown

- **Route**: `/insights/drilldown?scope=…&value=…&page=…` (GET)
- **Builder**: none (templ helper `insightDrilldownHref`)
- **Template**: `internal/templates/insights.templ:InsightsDrilldownView`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── <dynamic title> ────────────────────────────────────────────────┐
│ h2 + description (from handler)                                  │
│ <btn: ← Back to Insights>  (data-history-back)                   │
├───────────────────────────────────────────────────────────────────┤
│ "Showing N matching record(s)" banner                            │
│                                                                    │
│ if records == 0:                                                  │
│   "No records matched this insight."                              │
│ else:                                                             │
│   [Manual Comparison row] — Compare Selected (disabled until 2)  │
│   for each personRecord:                                          │
│     [Compare checkbox + Quick View btn] + [SoldierCard highlighted]│
│     [SearchPreviewContent hidden]                                 │
│   pagination (Prev / Next)                                        │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

None registered in uiids. The whole page IS the panel.

## Atomic components

- `Button` — Back, Compare Selected, Quick View.
- `Card` — wrapper.
- `SoldierCard` (highlighted=true) — list rows.
- `SearchPreviewContent` — hidden quick-view drawer.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Pagination Prev/Next | GET | `insightDrilldownPageHref(scope, value, page)` | (full nav) | `href=` only — could be HTMX but isn't |
| Compare selected | — | — | — | JS (`data-compare-selected`, `data-compare-group="insight-compare"`) |
| Quick View | — | — | — | JS toggle |
| Per-row Detail link | GET | `/soldiers/{id}?from=…` | (full nav) | Bare URL |

## State variants

- **No matches**: explicit copy.
- **Single page** (total ≤ pageSize): pagination hidden.

## Footguns

- **Pagination is plain `<a href>`, not HTMX** — every page click is
  a full page render. Verify acceptable for expected dataset sizes.
- **No routebuilder** for `/insights/drilldown` — `insightDrilldownHref`
  hand-builds query. Drift risk.
- **Compare group `insight-compare` is unique** — verify no other
  page accidentally reuses it (would cause cross-page compare
  interference).
- **`?from=…` query** — used for back-navigation. Verify handler
  respects it.

## See also

- [09-insights.md](09-insights.md)