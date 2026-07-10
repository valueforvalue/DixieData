# Historical — Unit Camaraderie (deprecated 2026-07-10)

This wireframe documents the **pre-issue-#455** `/soldiers/{id}/camaraderie`
sub-page. The sub-page has been **deleted** as part of issue #455 slim R&R
foldout:

- The use case moves to the existing `/insights/drilldown?scope=unit&value={unit}`
  page (already lists every Person Record sharing the unit).
- The foldout menu entry and soldier_card tile for Camaraderie were
  removed in slice 1.
- The `handleUnitCamaraderie` handler, `UnitCamaraderieGraph` facade
  method, and the `UnitCamaraderieGraph` service method (along with
  its tier-ranking helpers) were deleted in slice 3.
- The `camaraderie.templ` template is gone as of slice 3.

The 3-tier ranking (exact / company variant / regiment) that
Camaraderie added on top of Insights is a noted but accepted loss.
This wireframe is retained for archaeology per `docs/historical/`
retention rule. For the current surface inventory see
[`docs/ui-map/INDEX.md`](../ui-map/INDEX.md).

The original wireframe text follows below.

---

# 18 — Unit Camaraderie

- **Route**: `/soldiers/{id}/camaraderie` (GET), via
  `routebuilder.SoldierCamaraderie(id)`
- **Template**: `internal/templates/camaraderie.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Unit Camaraderie Graph ─────────────────────────────────────────┐
│ [← Back btn] [Open Person Record pill]                           │
│                                                                    │
│ responsive-2-col:                                                 │
│  ┌──[aside]────────────────┐  ┌──[main]──────────────────────┐  │
│  │ h2 Name + DisplayID      │  │ intro copy                    │  │
│  │ [Recorded Unit]          │  │ [Same Recorded Unit section]  │  │
│  │ [Regiment Context]       │  │   grid 2-col of peer cards    │  │
│  │ [Company Signal]         │  │ [Company Variants section]    │  │
│  │ counters:                 │  │   grid 2-col                  │  │
│  │  Same Unit / Company Var │  │ [Same Regiment section]       │  │
│  │  / Same Regiment         │  │   grid 2-col                  │  │
│  └──────────────────────────┘  └──────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

`page.unit-camaraderie` registered. No inner panels.

## Atomic components

- `Button` — Back, Compare (per peer).
- `Card` — aside + main + per-peer cards.
- `Pill` — Open Person Record.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Open Person Record | GET | `/soldiers/{id}?from=…` | (full nav) | Bare URL |
| Compare Person Records | GET | `/compare?id1=…&id2=…&from=…` | (full nav) | Bare URL |

## Footguns

- **Bare URLs** — same pattern.
- **`soldierHasCamaraderie(s)` gate** — page hidden from Soldier
  Detail if no unit. Verify the gate.
- **Three tiers of peer matching** (exact, company variant, regiment)
  — verify the matching algorithm labels each section correctly.
- **`peer.Relation` text** is server-generated — verify it's
  human-friendly.

## See also

- [05-soldier-detail.md](05-soldier-detail.md)