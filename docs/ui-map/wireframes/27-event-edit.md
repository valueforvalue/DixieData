# 27 — Event Edit

- **Route**: `/events/{id}/edit` (GET, POST)
- **Builders**: `routebuilder.EventEdit(id)`,
  `routebuilder.EventLinksAttach(id)`,
  `routebuilder.EventLinksDetach(id, personID)`,
  `routebuilder.EventTagAttach(id)`,
  `routebuilder.EventTagDetach(id, tagID)`.
- **Template**: `internal/templates/event_form.templ:EventForm`
  (with `isEdit=true`)
- **Layout**: relaxed
- **Owner**: package `templates`

Edit form for an Event Record. Renders the same form as the create
form, plus three post-create sections (Linked Persons, Tags) that
sit OUTSIDE the main `<form>` to avoid HTML-invalid nested forms
(per the comment block in `event_form.templ`).

## Regions

```
┌── Header ────────────────────────────────────────────────────────┐
│ h2 "Edit Event {DisplayID}"                                      │
├─────────────────────────────────────────────────────────────────┤
│ [page.event.edit]                                                 │
│   if errorMessage: red callout (Save failed. <msg>)              │
│   <form action=/events/{id} method=post data-dixie-submit>       │
│     <hidden> display_id entry_type=event                        │
│     <grid 2-col>                                                  │
│       DisplayID (readonly) | Kind (free-text)                    │
│       Begin Date | End Date                                       │
│     Description (textarea 6 rows)                                 │
│     <details Advanced: PDF Excerpt Override> (collapsed)        │
│       PDF Excerpt Override (textarea 4 rows)                      │
│     Internal Notes (textarea 3 rows)                              │
│     [Source Records card]                                         │
│       [+ Add Source Record] (data-record-add)                    │
│       <details open Show/hide source records>                    │
│         <div data-record-list>                                    │
│           for each event source: @RecordInputRow(record)         │
│           @RecordInputRow(<empty>)                                │
│         <template data-record-template>                          │
│     <action stack> [Save Changes primary] [Cancel]                │
├─────────────────────────────────────────────────────────────────┤
│ [panel.event.form.linked-persons] (OUTSIDE main form)            │
│   <p> Type a Display ID below; use Unlink on a row to remove.   │
│   @EventLinksListFragment(s.ID, s.LinkedPersons)                 │
│   <details open Add a linked Person Record>                      │
│     <form action=routebuilder.EventLinksAttach(id) method=post>  │
│       <input display_id required placeholder=DXD-00001>          │
│       <submit> Add Link                                          │
├─────────────────────────────────────────────────────────────────┤
│ [panel.event.form.tags] (OUTSIDE main form)                      │
│   <p> Free-text labels; type a name below; use × on chip.       │
│   <div id=data-event-tags-list>                                  │
│     @EventTagsListFragment(s.ID, s.Tags)                         │
│   <details open Add a tag>                                        │
│     <form action=/events/{id}/tags method=post                   │
│           data-results-target=#data-event-tags-list>             │
│       <input tag_name required placeholder=vc-shiloh>             │
│       <submit> Add Tag                                            │
└─────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `page.event.edit` | Page wrapper | `<div id={uiids.PageEventEdit}>` wrapping the whole edit page |
| `panel.event.form.sources` | Source Records editor | The RecordInputRow list (same shape as the soldier form's records section) |
| `panel.event.form.linked-persons` | Linked Persons | `<section>` rendered outside the main `<form>` (HTML validity) |
| `panel.event.form.tags` | Tags | `<section>` rendered outside the main `<form>`; Add Tag form swaps into `#data-event-tags-list` |

No tabs.

## Atomic components

- `Button` — Save Changes (primary), Add Source Record (ghost),
  Cancel (ghost-link), Add Link (ghost), Add Tag (ghost).
- `Card` — wrapping the main form + each post-create section.
- `Field` — every input (DisplayID readonly, Kind, Begin/End Date,
  Description, PDF Excerpt Override, Notes, Display ID for new
  link, tag_name for new tag).
- `EmptyState` — none in v1 (the Linked Persons + Tags sections
  have their own `LinkedPersonsListFragment` / `TagsListFragment`
  empty states).
- `RecordInputRow` — the per-source input row templ shared with
  the soldier form.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Form submit | POST | `/events/{id}` | `body` | default | Full nav on success; re-render with error on validation failure |
| Cancel | — | `data-history-back` + `data-fallback-href=/events/{id}` | `body` | default | JS-only back button (falls back to detail page) |
| + Add Source Record | — | `data-record-add` (DOM clone from `<template>`) | `#record-list` | append | JS-only |
| Add Link | POST | `routebuilder.EventLinksAttach(id)` | `body` | default | `X-DixieData-Redirect` lands back on /events/{id}/edit (issue #361) |
| Unlink (per row) | POST | `routebuilder.EventLinksDetach(id, personID)` | `body` | default | Same redirect target |
| Add Tag | POST | `/events/{id}/tags` | `#data-event-tags-list` | default | `data-results-target` swap preserves the user's unsaved form state |
| Tag detach (× on chip) | POST | `routebuilder.EventTagDetach(id, tagID)` | `#data-event-tags-list` | default | Per-row button |

## Modals / overlays

None.

## State variants

- **Validation error**: red callout at top + form re-renders.
- **No linked persons / no tags**: the fragment returns an empty
  state inside the section; the Add Link / Add Tag forms stay
  available.
- **No source records**: one empty `RecordInputRow` plus the
  existing-event sources (if any).
- **404**: `respondNotFound` when the event id is unknown.

## Footguns

- **Nested forms** — the Linked Persons and Tags sections render
  OUTSIDE the main `<form>` because HTML forbids `<form>` inside
  `<form>`. If a future change moves either section inside, the
  browser will silently drop the inner form, breaking the Add
  Link / Add Tag buttons. Verify by viewing the page source and
  checking `<form>` tag nesting.
- **`data-results-target` on Add Tag** — the form sets
  `data-results-target="#data-event-tags-list"` so JS swaps the
  new tag list fragment in place, preserving the user's unsaved
  edits to kind/description/notes across attach/detach. The same
  pattern is used on the Add Link form (`data-dixie-submit` +
  `X-DixieData-Redirect`). Verify both contracts.
- **PII prompt on Add Link** — the section copy instructs the user
  to type the full Display ID (case-insensitive, whitespace-
  trimmed). The handler resolves via `LookupPersonIDByDisplayID`
  then calls `AttachEventToPerson` (issue #361 slice 2).
- **Tag dedupe** — Add Tag uses the soldier-side tag picker
  contract: case-insensitive, new names dedup automatically via
  `TagService.UpsertByName`.
- **Source reordering** — the v1 form has no UI for reordering
  sources. The detail page has up/down buttons that PATCH
  `/events/{id}/sources/{sourceID}/position`; verify the source
  list re-renders correctly after a position change.
- **Source deletion** — the form re-renders sources from
  `s.EventSources` on every save; deleting a source requires
  editing the per-row `RecordInputRow` to clear all fields
  (server-side filters empty rows). A dedicated "Remove Source"
  button is out of scope for v1.
- **`data-dixie-submit`** on every form — verify the Wails
  override / FormData quirks per AGENTS.md Quirk 1 + Quirk 2.

## See also

- [25-event-detail.md](25-event-detail.md) (caller surface)
- [26-event-new.md](26-event-new.md) (sibling create form)
- [07-soldier-edit.md](07-soldier-edit.md) (sibling soldier form)
