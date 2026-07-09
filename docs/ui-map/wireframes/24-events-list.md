# 24 — Events List

- **Route**: `/events` (GET)
- **Builder**: `routebuilder.EventList()`
- **Template**: `internal/templates/event_list.templ:EventList`
- **Layout**: relaxed
- **Owner**: package `templates`
- **Audit**: n/a

Browse page for Event Records. Mirrors the soldier list page shape
(header + grid of cards + empty state), but events are sorted by
DisplayID (no full-text search yet — out of scope for the v1
landing per issue #320's RPCI spec).

## Regions

```
┌── Event Records header ─────────────────────────────────────────┐
│ h2 "Event Records"                                              │
│ <primary-button> + Add Event Record → /events/new               │
├─────────────────────────────────────────────────────────────────┤
│ if len(events) == 0:                                            │
│   [EmptyState] "No event records yet"                            │
│ else:                                                            │
│   for each event:                                                │
│     [EventCard]                                                   │
│       DisplayID (gold) | Kind (small slate)                       │
│       [pill-link View Event → /events/{id}]                       │
└─────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `page.event.list` | Page wrapper | The `<div id={uiids.PageEventList}>` wrapping the header + grid |

No inner panels.

## Atomic components

- `Button` — Add Event Record (primary CTA).
- `Card` — per-event row.
- `EmptyState` — no-records copy.
- `Pill` — View Event link.

## HTMX wiring

None on the list page itself. The page is a full GET. Per-event
links are full navigations.

## Modals / overlays

None on the list page. The per-Event PDF btn (in the detail page)
opens `overlay.print-config.modal`.

## State variants

- **Empty**: `EmptyState` copy.
- **Non-empty**: simple card grid. No sort or filter UI yet (deferred
  per the v1 scope lock).

## Footguns

- **Bare href `/events/{id}`** in `EventCard` — the per-event link
  is a literal sprintf rather than `routebuilder.EventDetail(id)`.
  Routebuilder coverage gap. See [gaps.md](../gaps.md).
- **`eventBadgeLabel` fallback** — when the entry type is not
  `EntryTypeEvent`, the label falls back to `entryBadgeLabel(s)` so
  legacy rows in the events list (entry_type != event) still render.
  The v1 lock is events only; this fallback exists for the migration
  window.
- **Pagination** — the v1 list renders all events on one page. If
  the local archive grows past ~200 events, page or virtualize.
  Out of scope for the slice-1 RPCI.
- **`+ Add Event Record`** uses the primary button style — verify
  the test harness hits the same anchor via the layout
  `data-history-back` button contract.

## See also

- [25-event-detail.md](25-event-detail.md) (caller surface)
- [26-event-new.md](26-event-new.md) (CTA target)
- [03-soldiers-list.md](03-soldiers-list.md) (sibling list page)
