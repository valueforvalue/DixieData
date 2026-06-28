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