# 02 — Calendar Day (fragment)

- **Route**: `/anniversary/{m}/{d}` and `/anniversary/{m}/{d}?edit={id}`
- **Builder**: `routebuilder.Anniversary(month, day)` /
  `routebuilder.AnniversaryEdit(month, day, id)`
- **Template**: `internal/templates/calendar_day.templ`
- **Layout**: rendered INTO `#details-pane` on Calendar page
- **Owner**: package `templates`

## Regions (inside `#details-pane`)

```
┌── CalendarDayDetail ─────────────────────────────────────────────┐
│ <h3> October 15 </h3>                                            │
│ "Review anniversaries first, then manage..."                    │
│ if statusMessage:                                                │
│   [status banner — success / error]                             │
│ if allowCustomItems:                                             │
│   [CalendarDayActions card]                                      │
│     • "Add Event or Holiday" / "Edit Calendar Item" details     │
│       with [CalendarDayActionMenu popout]                       │
│ [CalendarDayAnniversaries section]                               │
│   h4 "Anniversaries" + density toggle (Expanded / Compact)      │
│   [SoldierCard x N, OR empty-state copy]                         │
│   [CalendarAnniversaryCompactRow x N — hidden by default]       │
│ if allowCustomItems:                                             │
│   [CalendarDayCustomItems section]                               │
│     h4 "Events & Holidays"                                       │
│     [item card x N — Edit (hx-get) / Delete (hx-delete)]        │
└──────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

None at the page level — this is an HTMX swap fragment that lives
inside Calendar's `panel.calendar.details`. The detail fragment owns:

- Action popout (`data-popout-panel`)
- Density toggle (`data-calendar-anniversary-density-toggle`)
- Item rows (anniversaries + custom items)

## Atomic components

- `Button` — Save Changes / Add Item / Mark Resolved.
- `Card` — wraps the day details body.
- `Field` — item form fields.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Day button (from Calendar) | GET | `routebuilder.Anniversary(month, day)` | `#details-pane` | `innerHTML` | Loads fragment |
| Item Edit button | GET | `routebuilder.AnniversaryEdit(month, day, item.ID)` | `#details-pane` | `innerHTML` | Same fragment, editing state |
| Item Delete button | DELETE | `routebuilder.AnniversaryItemDelete(...)` | `#details-pane` | `innerHTML` | `hx-confirm` prompt |
| Cancel Edit (in popout) | GET | `routebuilder.Anniversary(month, day)` | `#details-pane` | `innerHTML` | Reverts to view-mode |
| Create item form | POST | `routebuilder.AnniversaryItemCreate(...)` | `#details-pane` | `innerHTML` | New item appended |
| Update item form (editing) | PUT | `routebuilder.AnniversaryItemUpdate(...)` | `#details-pane` | `innerHTML` | Save in place |

The fragment is ALWAYS swapped into `#details-pane`. All mutation
verbs re-render the same fragment, so the swap target stays constant.

## Modals / overlays

None local. Inherits global `overlay.floating.menu`,
`overlay.feedback.modal`, `overlay.jobs.progress` from Layout.

## State variants

- **Empty**: explicit copy ("No person records for this date.",
  "No custom calendar items yet for this date.").
- **Editing**: `day.Form.EditingID > 0` → details opened by default +
  "Editing in place" badge.
- **Error**: `day.Form.ErrorMessage` rendered as red callout above
  the form.

## Footguns

- **Popout form uses `hx-target="#details-pane"`** — same as the day
  load. After save, the entire day-detail fragment re-renders, not
  just the item list. Verify the popout collapses after submission
  (it's inside the details fragment, so it should be replaced).
- **Delete `hx-confirm` is templated** — text interpolates item title
  and date. Long titles may push the confirm prompt into a tall
  dialog.
- **Anniversary density toggle uses `data-calendar-anniversary-density-toggle`** —
  client-side only via JS (`frontend/app.js`). Verify both
  `data-calendar-anniversary-expanded` and
  `data-calendar-anniversary-compact` rows render on every swap and
  the toggle re-applies after HTMX re-renders.
- **`calendarDayStatusClass(kind)` default** returns the error styling
  for any non-`"success"` kind. Audit callers to confirm they only
  pass `"success"` or empty string.

## See also

- [01-calendar.md](01-calendar.md) — host page.