# 25 — Event Detail

- **Route**: `/events/{id}` (GET, POST, PUT, DELETE)
- **Builders**: `routebuilder.EventEdit`, `routebuilder.EventPDF`,
  `routebuilder.EventResearchLog`, `routebuilder.EventLinksAttach`,
  `routebuilder.EventLinksDetach`, `routebuilder.EventTagAttach`,
  `routebuilder.EventTagDetach`, `routebuilder.EventImages`,
  `routebuilder.EventImagesImport`, `routebuilder.EventImagesDelete`,
  `routebuilder.EventSourcePosition`.
- **Template**: `internal/templates/event_detail.templ:EventDetail`
- **Layout**: relaxed
- **Owner**: package `templates`
- **Audit**: n/a

Detail page for an Event Record. Renders kind pill, begin / end
dates, description (or PDF excerpt override when set), and stacked
sections for Linked Persons / Source Records / Tags / Images. The
apply-sites (Sources, Tags, Linked Persons, Images) are tracked as
follow-up slices per the issue #320 RPCI spec's "Slice 4" out-of-
scope list.

## Regions

```
┌── Top strip ─────────────────────────────────────────────────────┐
│ <btn data-history-back data-fallback-href=/events> ← Back       │
├─────────────────────────────────────────────────────────────────┤
│ [page.event.detail]                                              │
│   DisplayID (gold) | Kind pill | Title (h2)                     │
│   if dates: <p> begin — end </p>                                │
│   <action stack> [Export PDF popout] [Edit] [Research Log]     │
│                  [Open Scratch Pad] [Delete Event]              │
├─────────────────────────────────────────────────────────────────┤
│ if description:                                                   │
│   [Description card]                                              │
│   if pdf_excerpt_override:                                       │
│     [amber callout: PDF Excerpt Override active]                │
├─────────────────────────────────────────────────────────────────┤
│ if notes:                                                         │
│   [Internal Notes card]                                          │
├─────────────────────────────────────────────────────────────────┤
│ [Linked Person Records section]                                  │
│   count + [Edit Event] link                                      │
│   if empty: EmptyState                                            │
│   else: <ul> each → pill-link to /soldiers/{id}                 │
├─────────────────────────────────────────────────────────────────┤
│ [panel.event.detail.sources]                                     │
│   <div id=data-event-sources-list>                               │
│   [EventSourcesListFragment]                                     │
├─────────────────────────────────────────────────────────────────┤
│ [panel.event.detail.tags]                                        │
│   <div id=data-event-tags-list>                                  │
│   [EventTagsListFragment]                                        │
├─────────────────────────────────────────────────────────────────┤
│ [panel.event.detail.images]                                      │
│   <div id={uiids.PanelEventDetailImages}>                        │
│   [EventImagesListFragment]                                      │
│   <form data-event-image-import-form> + Add Images From Computer │
└─────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `page.event.detail` | Page wrapper | `<div id={uiids.PageEventDetail}>` wrapping the whole detail page |
| `panel.event.detail.linked-persons` | Linked Persons list | `<ul>` of linked Person Records (or empty state) |
| `panel.event.detail.sources` | Source Records | `<div id=data-event-sources-list>` swap target |
| `panel.event.detail.tags` | Tags chips | `<div id=data-event-tags-list>` swap target |
| `panel.event.detail.images` | Images gallery | `<div id={uiids.PanelEventDetailImages}>` swap target |

No tabs.

## Atomic components

- `Button` — Edit Event, Research Log, Open Scratch Pad, Delete
  Event, Add Images From Computer.
- `Card` — wrapping the summary block + per-section wrappers.
- `EmptyState` — no linked persons, no sources, no tags, no images.
- `Pill` — View linked person, Edit Event (ghost-link variant).
- `EventPDFExport` — per-event PDF button partial with `data-pdf-pref-scope="event"`.
- `LinkedText` — not used here; the description is plain text.
- `Modal` — not used directly; the PDF btn opens `overlay.print-config.modal`.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Edit Event btn | GET | `routebuilder.EventEdit(id)` | `body` | default | Full nav |
| Research Log btn | GET | `routebuilder.EventResearchLog(id)` | `body` | default | Full nav |
| Open Scratch Pad | POST | `/scratchpad/open` | `this` | `none` | Native form submit, `data-dixie-submit` |
| Delete Event | DELETE | `/events/{id}` | `body` | default | Destructive `hx-confirm`; wrapped by `data-dixie-submit` |
| Source position up/down | PATCH | `routebuilder.EventSourcePosition(id, srcID)` | `#data-event-sources-list` | default | Re-renders the list fragment |
| Tag detach (chip ×) | POST | `routebuilder.EventTagDetach(id, tagID)` | `#data-event-tags-list` | default | Re-renders the list fragment |
| Link Unlink (per row) | POST | `routebuilder.EventLinksDetach(id, personID)` | `this` | `none` | Re-renders via redirect; lands back on /events/{id} |
| Add Images From Computer | POST | `routebuilder.EventImagesImport(id)` | `#panel.event.detail.images` | default | Native dialog guarded per dialog-guard.md |
| Delete selected images | POST | `routebuilder.EventImagesDelete(id)` | `#panel.event.detail.images` | default | `hx-confirm` |

## Modals / overlays

- `overlay.print-config.modal` — opened by the PDF btn popout
  (`data-pdf-pref-scope="event"`).
- Global overlays available.

## State variants

- **404**: `respondNotFound` when the event id is unknown (no
  templ renders).
- **Render failure**: `respondErrorFragment` swaps
  `EmptyStateError` into the page wrapper per the slice-2 contract.
- **No description / no notes**: sections omitted entirely (not
  EmptyState).
- **No linked persons**: `EmptyState` with "Manage linked Person
  Records from the event editor." copy.
- **No sources / no tags / no images**: same pattern via
  `EventSourcesListFragment` / `EventTagsListFragment` /
  `EventImagesListFragment`.
- **PDF excerpt override active**: amber callout above the
  description block.

## Footguns

- **`data-dixie-submit` everywhere** — the form dispatcher in
  `frontend/app.js` rewrites PATCH/PUT/DELETE to POST +
  `X-HTTP-Method-Override` for the Wails webview (per the Quirk 1
  / Quirk 2 in AGENTS.md). The Event handlers rely on the same
  override; verify `internal/appshell/app.go:487` is the
  `requestMethodOverride` middleware.
- **Native dialog guard** on `Add Images From Computer` — MUST be
  guarded with `a.inFlight.LoadOrStore` per
  [dialog-guard.md](../../agents/dialog-guard.md). The handler is
  `handleEventImagesRoute` in `events_handlers.go`; the dialog call
  is `OpenFileDialog` for the multi-file image picker.
- **Open Scratch Pad** form uses `data-dixie-submit` + a hidden
  `display_id` field. Verify the dispatcher's FormData-to-urlencoded
  conversion preserves the field (Quirk 2 in AGENTS.md).
- **Delete Event** uses `data-method="DELETE"` + `data-dixie-submit`
  with a `data-action` URL string. Verify the dispatcher strips the
  method correctly per the Wails-PATCH comment in `frontend/app.js`.
- **Source position PATCH** — the route is
  `/events/{id}/sources/{sourceID}/position`; the fragment swap
  target is the literal `#data-event-sources-list`. Verify the
  response body is the full re-rendered list, not just a status
  message.
- **Tag detach** — `POST /events/{id}/tags/{tagID}/detach` returns
  the re-rendered list fragment. The chip × button on the detail
  page is a `<button data-action=...>` (not a form) so the Wails
  override kicks in.
- **Linked Persons list** — bare URL string
  `/soldiers/{id}` for each linked person's display ID. Routebuilder
  coverage gap. See [gaps.md](../gaps.md).
- **`PDFExcerptOverride` source-of-truth** lives in the event
  record; the detail page surfaces a yellow callout when set so
  the user remembers the override is in effect.

## See also

- [24-events-list.md](24-events-list.md) (parent list)
- [26-event-new.md](26-event-new.md) (form)
- [27-event-edit.md](27-event-edit.md) (edit form)
- [28-event-pdf.md](28-event-pdf.md) (PDF)
- [05-soldier-detail.md](05-soldier-detail.md) (sibling detail page)
