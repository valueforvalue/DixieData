# 17 — Service Timeline

- **Route**: `/soldiers/{id}/timeline` (GET), via `routebuilder.SoldierTimeline(id)`
- **Template**: `internal/templates/timeline.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Service Timeline ──────────────────────────────────────────────┐
│ [← Back btn] [Open Person Record pill]                           │
│                                                                    │
│ responsive-2-col:                                                 │
│  ┌──[aside]────────────────┐  ┌──[main]──────────────────────┐  │
│  │ h2 Name + DisplayID      │  │ intro copy                    │  │
│  │ [Service Profile card]   │  │ if events == 0:              │  │
│  │ [Timed Events / Exact / │  │   empty-state copy            │  │
│  │  Inferred counters]      │  │ else:                         │  │
│  │ [Chronology Span]        │  │   per event:                   │  │
│  │ [Undated Source Records] │  │     category pill, Title,     │  │
│  └──────────────────────────┘  │     DateLabel                 │  │
│                                 │     Confidence + Source pills │  │
│                                 │     LinkedText(description)   │  │
│                                 │                              │  │
│                                 │ if undatedSourceRecords:     │  │
│                                 │   [Undated Source Records]   │  │
│                                 └──────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

`page.service-timeline` registered. No inner panels.

## Atomic components

- `Button` — Back.
- `Card` — aside + main sections + event cards.
- `Pill` — Open Person Record.
- `LinkedText` — event descriptions.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Open Person Record | GET | `/soldiers/{id}` | (full nav) | Bare URL |
| Back btn | JS | — | — | `data-history-back`, falls back to `/soldiers/{id}` |

## Footguns

- **Bare URLs** — same pattern as Soldier Detail.
- **`soldierHasTimeline(s)` gate** — page returns "no events"
  empty state if birth/death dates or source records are absent.
  Verify the gate logic.
- **`serviceTimelineCategoryClass`** returns slate-500 by default —
  categories default to "Source Context".
- **Date parsing from SourceRecord Details** — fragile. Verify
  regex/parser handles common formats.

## See also

- [05-soldier-detail.md](05-soldier-detail.md)