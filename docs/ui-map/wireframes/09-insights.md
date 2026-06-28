# 09 — Insights

- **Route**: `/insights` (GET)
- **Builder**: `routebuilder.InsightsReportPDF()`
- **Template**: `internal/templates/insights.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Local Archive Insights ─────────────────────────────────────────┐
│ if zero records: EmptyStateCard("insights", counts)               │
│ h2 + intro copy                                                   │
│ [Export Analytics Report form] (orientation, printer-friendly, btn)│
├───────────────────────────────────────────────────────────────────┤
│ [panel.insights.overview]                                        │
│   "Person Record Type Snapshot" — Soldiers / Spouse / Person Recs │
│   each → drilldown by entry_type                                  │
│                                                                    │
│ [panel.insights.cemeteries]                                       │
│   "Top Cemeteries" — list of {label, count} → drilldown buried_in│
│                                                                    │
│ [panel.insights.homes]                                           │
│   Confederate Home Census "Status Breakdown" (list)              │
│   "Most Frequent Home Names" (list)                              │
│                                                                    │
│ [panel.insights.pensions]                                        │
│   Pension Distribution (list, scope=pension_state)               │
│                                                                    │
│ [panel.insights.units]                                           │
│   Top Units (list, scope=unit)                                   │
│                                                                    │
│ [panel.insights.duplicate-audit]                                 │
│   Advanced Duplicate Discovery                                    │
│   Open / Resolved pairs + similarity threshold                    │
│   Last scan timestamp | <btn: Audit Now> (hx-post)                │
│                                                                    │
│ [panel.insights.chronology] (responsive-span-2)                  │
│   Birth Decades | Death Decades (two columns)                    │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region |
| --- | --- |
| `panel.insights.overview` | Person Record Type Snapshot |
| `panel.insights.cemeteries` | Top Cemeteries |
| `panel.insights.homes` | Confederate Home Status + Names |
| `panel.insights.pensions` | Pension Distribution |
| `panel.insights.units` | Top Units |
| `panel.insights.chronology` | Birth/Death Decades |
| `panel.insights.duplicate-audit` | Duplicate Audit |

No tabs. No overlays local.

## Atomic components

- `Button` — Export Analytics Report, Audit Now.
- `Card` — wraps every section.
- `EmptyState` — zero-archive.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Export Analytics Report | POST | `routebuilder.InsightsReportPDF()` | `this` | `hx-swap="none"`, `data-pdf-pref-scope="insights"` |
| Audit Now | POST | `/insights/audit/duplicates` | `#insights-audit-status` | |
| Each list row link | GET | `/insights/drilldown?scope=…&value=…` | (full nav) | → [10-insights-drilldown.md](10-insights-drilldown.md) |

## Footguns

- **Bare `/insights/audit/duplicates`** URL — routebuilder gap.
- **Analytics counts use templ func `insightDrilldownHref`** that
  hand-builds query params. Not via `routebuilder`. Drift risk if
  drilldown route changes.
- **Section depends on `snapshot.DuplicateAudit.SimilarityThreshold`**
  — verify goquery invariant test asserts the threshold is rendered
  in copy.
- **Two-column layout uses `responsive-two-col`** — verify on small
  screens each section stacks gracefully.
- **`snapshot.PersonRecordTypes.SoldierCount` vs SpouseRecordCount vs
  PersonRecordCount** — verify the link targets pass the correct
  `value` for each (e.g. spouse uses literal `"spouse"`, not an enum
  constant).
- **`LastRunAt` may be empty** — verify rendering.

## See also

- [10-insights-drilldown.md](10-insights-drilldown.md)
- [08-export.md](08-export.md) (PDF export popout family)