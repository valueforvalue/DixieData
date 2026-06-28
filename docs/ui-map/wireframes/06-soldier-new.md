# 06 — Soldier New

- **Route**: `/soldiers/new` (GET, GET-with-error)
- **Builder**: `routebuilder.SoldierCreate()`
- **Template**: `internal/templates/entry_form.templ:EntryForm`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── EntryForm ──────────────────────────────────────────────────────┐
│ h2 "New Person Record"                                           │
│ <details Scrape Find a Grave> (collapsed by default)             │
│   source label, warnings count, confidence, spouses, error chips  │
│   <textarea name=findagrave_source> + <Fetch Data> btn            │
│   error callout (if scrape failed)                                │
│   warnings list (review scraped data)                              │
│   spouses found list                                              │
├───────────────────────────────────────────────────────────────────┤
│ [panel.soldier.form.scratchpad]                                  │
│  (data-record-persistence block — local draft status / undo)      │
├───────────────────────────────────────────────────────────────────┤
│ errorMessage callout (if save failed)                            │
│                                                                    │
│ <form> hx-post SoldierCreate() hx-target="body" enctype=multipart│
│   <hidden> existing_needs_review, existing_review_reason,         │
│            scrape_source_label, scrape_confidence_score          │
│                                                                    │
│   § Identity & Relationship                                      │
│     Display ID (readonly) | Person Record Type (select)           │
│     Prefix (+ show prefix before name) | First/Middle/Last/Suffix│
│     [Person Record Link sub-section]                              │
│       Linked Soldier (select) | Relationship Label (linked) |     │
│       Maiden Name (spouse-only)                                   │
│                                                                    │
│   § Service, Pension & Archive Details                            │
│     Rank In/Out | Unit | Pension State                            │
│     Confederate Home Status/Name                                 │
│     Pension ID / Application ID (soldier-or-widow)                │
│                                                                    │
│   § Life Details & Burial                                        │
│     Birth Date / Death Date (MM/DD/YYYY w/ 00 for unknown)       │
│     Birth Info / Buried In                                        │
│                                                                    │
│   § Source Records                                                │
│     [+ Add Source Record] btn (data-record-add)                   │
│     <details Show / hide source records>                         │
│       for each: [RecordInputRow]                                  │
│     <template data-record-template> for client-side row cloning  │
│                                                                    │
│   § Biography & Internal Notes (collapsed details)               │
│     Biography textarea + live char count                          │
│     [details Advanced PDF Excerpt Override]                       │
│       PDF Excerpt Override + budget live count                   │
│     Internal Notes (supports [[DISPLAY-ID]] link syntax)         │
│                                                                    │
│   § Images                                                         │
│     [Add Images From Computer btn (disabled until record exists)]│
│                                                                    │
│   <btn: Create Person Record> <btn: Cancel (ghost)>              │
│   11 SuggestionDatalist for autocomplete                          │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `panel.soldier.form.scratchpad` | Top of form | Local-draft persistence UI |
| `panel.soldier.form.records` | Source Records section | Add/remove rows |
| `panel.soldier.form.images` | Images section | Import-after-create gate |

Scrape Find a Grave is a `<details>` not in uiids.Registry. Person
Record Link sub-section is a `<div>` not registered.

## Atomic components

- `Button` — Create, Cancel, Fetch Data, Add Source Record, Add
  Images.
- `Card` — wraps the form.
- `Field` — every input/select/textarea.
- `SuggestionDatalist` — autocomplete for ranks, units, etc.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Scrape form submit | POST | `routebuilder.SoldierScrapeFindAGrave()` | `#entry-form-shell` | `outerHTML` | Replaces entire shell with new fragment |
| Form submit (Create) | POST | `routebuilder.SoldierCreate()` | `body` | default | Full redirect to new record |
| Add Source Record btn | — | — | — | — | JS-only (`data-record-add`) — clones `<template>` |
| Image import btn | (disabled) | — | — | — | Gates until record exists |

## Modals / overlays

Global only.

## State variants

- **Save error**: rendered as top-of-form red callout (re-uses form
  fragment via `EntryFormWithError`).
- **Scrape success**: same form re-renders with scraped values
  pre-filled; no separate "preview" step.
- **Scrape error**: scrape details collapse stays open with red
  callout.
- **Person Record Type=Linked Person**: shows Relationship Label,
  hides Maiden Name; toggled via `data-entry-type-special` JS.
- **Entry Type=Widow**: Pension/Application fields appear.
- **Entry Type=Soldier**: Rank/Unit fields appear.

## Footguns

- **`hx-target="body"` on the form** — relies on server returning a
  303 redirect. Verify handler does so.
- **`hx-swap="outerHTML"` on the scrape form** — replaces the entire
  shell (`#entry-form-shell`). If the user has scrolled, focus is
  lost. Verify the new shell preserves scroll position.
- **`data-record-add`** + `<template data-record-template>` is
  client-side JS — verify rows added on the client get included in
  the form submission (named inputs match).
- **`data-clear-draft-trigger`, `data-confirm-clear-draft`** —
  multi-step confirm flow. Verify the JS state machine.
- **Local-draft persistence** uses `data-draft-key` + `data-draft-record-version`
  for the stale-draft detection. Verify the `kind` attribute
  branches ("base" vs "stale").
- **Live char count** uses `data-live-count-input` + `data-live-count-target`.
  Verify the budget applies correctly for `pdf-excerpt`.
- **`data-entry-type-special` / `data-soldier-only-field` /
  `data-spouse-only-field` / `data-soldier-or-widow-field`** — all
  client-side toggles. If the user's localStorage or JS is broken,
  fields stay visible/hidden incorrectly.
- **`internal/templates/partials/empty_state.templ` vs
  `components/empty_state.templ`** — `EmptyStateCard` is the older
  partial; `EmptyState` is the newer component. New form uses
  neither directly; the form is shown even when archive is empty.
- **`/setup` page** uses `entry_form.templ`'s sibling InitialSetupView;
  separate wireframe.

## See also

- [07-soldier-edit.md](07-soldier-edit.md)
- [05-soldier-detail.md](05-soldier-detail.md)