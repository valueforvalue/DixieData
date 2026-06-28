# 16 — Research Pack

- **Route**: `/soldiers/{id}/research-pack/state`,
  `/soldiers/{id}/research-pack/county` (GET)
- **Builder**: none
- **Template**: `internal/templates/research_pack.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Research Pack ─────────────────────────────────────────────────┐
│ [← Back btn] [Open Person Record pill]                           │
│                                                                    │
│ responsive-2-col:                                                 │
│  ┌──[aside]────────────────┐  ┌──[main]──────────────────────┐  │
│  │ h2 PlaceLabel             │  │ grid 2-col:                   │  │
│  │ Description               │  │  [Top Units card]             │  │
│  │ Related Person Records (N)│  │  [Top Cemeteries card]        │  │
│  │ Review Queue Items (N)   │  │                               │  │
│  │ Anchor Person Record      │  │ [Related Person Records]      │  │
│  └──────────────────────────┘  │  grid of person record cards  │  │
│                                 │  <a: Open> <a: Compare>      │  │
│                                 └──────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

`page.research-pack` registered. No inner panels.

## Atomic components

- `Button` — none directly.
- `Card` — aside + main sections.
- `Pill` — Open Person Record.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Open Person Record | GET | `/soldiers/{id}?from=…` | (full nav) | Bare URL |
| Compare Person Records | GET | `/compare?id1=…&id2=…&from=…` | (full nav) | Bare URL |

## Footguns

- **Bare URLs everywhere** — same pattern as Soldier Detail.
- **`researchPackHeading(scope)`** switches between County / State —
  verify the route param `scope` is validated server-side.
- **`soldierBirthInfoResearchPackLabels`** parses Birth Info via
  regex — fragile to format changes. May surface no pack for valid
  but unusual formats.

## See also

- [05-soldier-detail.md](05-soldier-detail.md) (entry point)