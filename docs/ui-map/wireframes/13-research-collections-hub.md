# 13 — Research Collections Hub

- **Route**: `/research-collections` (GET)
- **Builder**: `routebuilder.ResearchCollectionsCreate()`,
  `routebuilder.ResearchCollectionAdd(id)`
- **Template**: `internal/templates/research_collections.templ:ResearchCollectionsHubView`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Named Research Collections ─────────────────────────────────────┐
│ h2 + intro                                                       │
│ if CurrentPersonRecord: [← Back to Person Record btn]            │
│ if CurrentPersonRecord:                                          │
│   [Current Person Record card] (highlighted blue)                │
│                                                                    │
│ [Create Collection card] (blue)                                  │
│   <form> hx-post ResearchCollectionsCreate()                     │
│     Name | Description                                            │
│     if CurrentPersonRecord: hidden input "from"                  │
│     <btn: Create Collection>                                      │
│                                                                    │
│ [Collections section]                                            │
│   h3 "Collections" + count                                      │
│   if empty:                                                       │
│     "No collections yet." copy                                    │
│   else:                                                           │
│     grid 2-col:                                                   │
│       per collection:                                             │
│         [Card] Name | Description | N person records pill         │
│                 [Contains current pill] if applicable            │
│                 <a: Open Collection>                              │
│                 if current && !contains:                          │
│                   <form hx-post ResearchCollectionAdd(id)>        │
│                     <btn: Add Current Person Record>              │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

None registered in uiids. The whole page is the panel.

`page.research-collections.hub` is the surface ID.

## Atomic components

- `Button` — Create, Add Current.
- `Card` — section + per-collection card.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Create form submit | POST | `routebuilder.ResearchCollectionsCreate()` | (default) | Verify response re-renders hub with new card |
| Add Current form | POST | `routebuilder.ResearchCollectionAdd(id)` | (default) | Hidden `soldier_id` + `from` |
| Open Collection | GET | `/research-collections/{id}?from=…` | (full nav) | Bare URL — routebuilder gap |

## Footguns

- **Bare `/research-collections/{id}?from=…`** — routebuilder gap.
- **No `hx-target`** on create/add — relies on default body swap.
  Verify server returns updated hub.
- **No uiids for sub-sections** — only the page-level ID.

## See also

- [14-research-collection-detail.md](14-research-collection-detail.md)
- [05-soldier-detail.md](05-soldier-detail.md) (entry point)