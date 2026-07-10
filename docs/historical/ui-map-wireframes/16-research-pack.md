# Historical — Research Pack (deprecated 2026-07-10)

This wireframe documents the **pre-issue-#455** `/soldiers/{id}/research-pack/{state|county}`
sub-page. The sub-page has been **deleted** as part of issue #455 slim R&R foldout:

- The Top Units / Top Cemeteries / Related Person Records panels it carried
  already live on `/insights` and cover the same use case.
- The foldout menu entry and soldier_card tile for Research Packs were
  removed in slice 1 (commits `cdd08a5`, `8438e1f`, `96d8ba2`).
- The `handleResearchPack` handler, `UnitCamaraderieGraph` /
  `ResearchPackForPersonRecord` facade methods, and the
  `ResearchPackForSoldier` service method were deleted in slice 3
  (commit `2ae9dc7`).
- Templates `research_pack.templ` + `research_empty_states.templ` and
  the presentation views `/internal/presentation/views.go` wrappers
  are gone as of slice 3.

This wireframe is retained for archaeology per `docs/historical/`
retention rule. For the current surface inventory see
[`docs/ui-map/INDEX.md`](../ui-map/INDEX.md).

The original wireframe text follows below.

---

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