# 15 — Research Log

- **Route**: `/soldiers/{id}/research-log` (GET)
- **Builder**: `routebuilder.ResearchLogTasksCreate(soldierID)`
- **Template**: `internal/templates/research_log.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Research Log ──────────────────────────────────────────────────┐
│ [← Back btn] [Open Person Record pill]                           │
│                                                                    │
│ responsive-2-col:                                                 │
│  ┌──[aside]────────────────┐  ┌──[main]──────────────────────┐  │
│  │ h2 Name + DisplayID      │  │ [Add Research Task card]     │  │
│  │ Open Tasks (N)           │  │   Title | Evidence Type      │  │
│  │ Resolved Tasks (N)       │  │   Research Notes             │  │
│  │ Suggested Next Leads (N) │  │   <btn: Add Research Task>   │  │
│  │ intro copy                │  │                              │  │
│  └──────────────────────────┘  │ if suggestions:               │  │
│                                 │  [Missing-Evidence Suggestions]│ │
│                                 │  grid of pre-filled forms    │  │
│                                 │                              │  │
│                                 │ [Task History section]       │  │
│                                 │   per task: status pill,     │  │
│                                 │   evidence type pill, title, │  │
│                                 │   notes, created/resolved ts │  │
│                                 │   <btn: Mark Resolved>       │  │
│                                 └──────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

`page.research-log` registered. No inner panels.

## Atomic components

- `Button` — Add Task, Mark Resolved.
- `Card` — section wrappers.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Add Task form | POST | `routebuilder.ResearchLogTasksCreate(id)` | (default) | |
| Suggestion form | POST | `routebuilder.ResearchLogTasksCreate(id)` | (default) | Pre-filled hidden inputs |
| Mark Resolved btn | POST | `/soldiers/{id}/research-log/tasks/{taskID}/resolve` | (default) | Bare URL — routebuilder gap |

## Footguns

- **Bare `/soldiers/{id}/research-log/tasks/{taskID}/resolve`** —
  routebuilder gap.
- **Suggestions are server-generated** based on record gaps. Verify
  they re-render after task creation.
- **`data-history-back` fallback** to `/soldiers/{id}` — verify
  back navigation.

## See also

- [05-soldier-detail.md](05-soldier-detail.md) (entry point)