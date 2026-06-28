# 14 — Research Collection Detail

- **Route**: `/research-collections/{id}` (GET), `?from=…`
- **Builder**: none
- **Template**: `internal/templates/research_collections.templ:ResearchCollectionDetailView`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Research Collection ───────────────────────────────────────────┐
│ [← Back btn (data-history-back → hub or soldier)]                │
│ [Collection header card]                                         │
│   Name | Description | N person records | "Current included" pill │
│                                                                    │
│ [Collection Members section]                                     │
│   if empty: "No records yet."                                    │
│   else: grid 2-col:                                              │
│     per member:                                                   │
│       Name | DisplayID                                           │
│       <a: Open Person Record>                                    │
│       if CurrentPersonRecord && Current.ID != member.ID:          │
│         <a: Compare Person Records> (bare compare URL)            │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

`page.research-collections.detail` registered; no inner panels.

## Atomic components

- `Button` — none directly.
- `Card` — header + members section.
- `Pill` — Open Person Record.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Open Person Record | GET | `/soldiers/{id}` | (full nav) | Bare URL |
| Compare Person Records | GET | `/compare?id1=…&id2=…&from=…` | (full nav) | Bare URL |

## Footguns

- **Bare URLs everywhere** — `/soldiers/{id}`, `/compare?id1=…`. Same
  gaps as Soldier Detail.
- **`researchCollectionFallbackHref` returns
  `/research-collections?from=…` if current, else `/research-collections`**
  — verify back navigation works on both paths.

## See also

- [13-research-collections-hub.md](13-research-collections-hub.md)