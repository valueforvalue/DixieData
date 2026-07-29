# 26 — Event New

- **Route**: `/events/new` (GET, POST)
- **Builder**: `routebuilder.EventNew()`
- **Template**: `internal/templates/event_form.templ:EventForm`
  (with `isEdit=false`)
- **Layout**: relaxed
- **Owner**: package `templates`

Create form for an Event Record. Renders the Event-specific fields
only (kind, begin_date, end_date, description, pdf_excerpt_override,
notes) — no Person Record fields (first/last name, rank, unit,
pension), no entry-type selector, no marriage candidates. The
handler routes through `events.CreateEvent` which sets
`entry_type="event"` and clears person-specific fields defensively.

## Regions

```
┌── Header ────────────────────────────────────────────────────────┐
│ h2 "New Event Record"                                            │
├─────────────────────────────────────────────────────────────────┤
│ [page.event.new]                                                  │
│   if errorMessage: red callout (Save failed. <msg>)              │
│   <form action=routebuilder.EventNew() method=post               │
│         data-dixie-submit>                                       │
│     <hidden> display_id="" entry_type=event                      │
│     <grid 2-col>                                                  │
│       DisplayID (readonly, "Auto-allocated EVT-NNNNN on first…") │
│       Kind (free-text)                                            │
│       Begin Date (text MM/DD/YYYY)                                │
│       End Date (text MM/DD/YYYY)                                  │
│     Description (textarea 6 rows)                                 │
│     <details Advanced: PDF Excerpt Override> (collapsed)        │
│       PDF Excerpt Override (textarea 4 rows)                      │
│     Internal Notes (textarea 3 rows)                              │
│     [Source Records card]                                         │
│       [+ Add Source Record] (data-record-add)                    │
│       <details open Show/hide source records>                    │
│         <div data-record-list>                                    │
│           @RecordInputRow(<empty>)                                │
│         <template data-record-template>                          │
│         empty rows skipped on save                                │
│     <action stack> [Create Event Record primary] [Cancel]         │
└─────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `page.event.new` | Page wrapper | `<div id={uiids.PageEventNew}>` wrapping the create form |

No inner panels in the v1 surface; the Source Records card is a
templ `<section>`, not a UIID panel (mirrors the soldier form's
`panel.soldier.form.records` — that's tracked as a follow-up).

## Atomic components

- `Button` — Create Event Record (primary), Add Source Record
  (ghost), Cancel (ghost-link).
- `Card` — wrapping the form.
- `Field` — DisplayID (readonly), Kind, Begin Date, End Date,
  Description, PDF Excerpt Override, Internal Notes.
- `Pill` — none.
- `EmptyState` — none.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Form submit | POST | `routebuilder.EventNew()` | `body` | default | Full nav on success; re-render with error on validation failure |
| Cancel | — | `data-history-back` + `data-fallback-href=/events` | `body` | default | JS-only back button |
| + Add Source Record | — | `data-record-add` (DOM clone from `<template>`) | `#record-list` | append | JS-only |

## Modals / overlays

None.

## State variants

- **Validation error**: red callout at top + form re-renders.
- **Empty source records**: one empty `RecordInputRow` is rendered
  by default; the user clicks `+ Add Source Record` to add more.
- **Empty submit**: server returns 400 with `errorMessage`; the
  form re-renders with the red callout.

## Footguns

- **`data-dixie-submit`** — the form posts to `/events/new`. Verify
  the dispatcher's POST passthrough (no Wails override needed for
  POST → POST).
- **Empty rows skipped on save** — the server-side handler iterates
  `s.EventSources` and skips rows where every field is empty. Verify
  the test `TestEventCreate_SkipsEmptySources` (or equivalent) pins
  the contract.
- **DisplayID is readonly** — the input is rendered with the
  `readonly` attribute AND a slate background class. The form
  still includes a hidden `display_id=""` so the server's
  `event.AllocateDisplayID` allocates the next EVT-NNNNN on first
  save. Do NOT pre-allocate on the client.
- **Nested forms** — the Linked Persons and Tags sections in the
  *edit* form (see 27-event-edit.md) render OUTSIDE the main
  `<form>` to avoid HTML-invalid nested forms. The create form has
  neither section (linked persons + tags are post-create concerns
  per the v1 scope). For the canonical bug class entry see
  [`docs/COMMON_BUGS.md` §2.7](../../COMMON_BUGS.md#27-nested-form-rendering-defect--html5-parser-silently-closes-the-outer-form-682-release-blocker-for-rcv11).
  Live instances at time of writing: `entry_form.templ` (Person
  Record edit), `soldier_card.templ` (Person Record detail). Both
  are fixed by #682.
- **PDF excerpt override is advanced** — collapsed by default
  inside `<details>` so the simple create path doesn't surface the
  override field.

## See also

- [24-events-list.md](24-events-list.md) (parent list + CTA source)
- [25-event-detail.md](25-event-detail.md) (caller surface)
- [27-event-edit.md](27-event-edit.md) (sibling edit form)
- [06-soldier-new.md](06-soldier-new.md) (sibling create form)
