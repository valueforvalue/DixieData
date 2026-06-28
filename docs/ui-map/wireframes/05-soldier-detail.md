# 05 — Soldier Detail

- **Route**: `/soldiers/{id}` (GET)
- **Builders**: `routebuilder.SoldierEdit`, `routebuilder.SoldierPDF`,
  `routebuilder.SoldierImagesDownload`, `routebuilder.SoldierImagesPrimary`,
  `routebuilder.SoldierReviewFlag`, etc.
- **Template**: `internal/templates/soldier_card.templ:SoldierDetail`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Detail header ─────────────────────────────────────────────────┐
│ [← back btn (data-history-back)]                                 │
│ <h2> Name </h2> (subhead: rank/unit for soldiers)                │
│ [DisplayID pill] [Edit pill] [Export Record ▾ popout]            │
│   popout: orientation, printer_friendly, include_images           │
│           <Export PDF> <Export JPG>                              │
│ [Compare Family Person Records btn] (if linked)                  │
├───────────────────────────────────────────────────────────────────┤
│ [panel.soldier.detail.summary]                                   │
│   if not soldier OR linked:                                       │
│     [Family & Relationships card] — links to linked soldier,      │
│                                    Compare action                │
│   <dl> responsive-two-col:                                        │
│     Type / DisplayID / Middle / Rank In-Out / Unit / Pension …    │
│     Birth / Death / BirthInfo / Buried In                        │
│   [Biography card] if biography != ""                            │
│   <details Advanced Research & Review>  (collapsed by default)   │
│     [Internal Notes]                                              │
│     [Unit Camaraderie] / [Service Timeline] (conditional)        │
│     [Research Log] / [Merge Review Ledger]                       │
│     [Research Packs state/county] (conditional)                  │
│     [Research Collections manage]                                 │
│     [Review Queue] — review note textarea + Update/Resolve/Flag  │
│   <details Record Metadata & History> (collapsed)                │
│     Review status / reason / created by / updated by / time       │
│     Recent Field Changes list                                    │
│   [back btn bottom strip]                                         │
│   [Danger Zone card] — Delete Person Record (hx-delete)          │
├───────────────────────────────────────────────────────────────────┤
│ [panel.soldier.detail.records] (if SourceRecords > 0)            │
│   h3 Source Records                                              │
│   for each record:                                                │
│     <card> type / AppID / LinkedText(details)                   │
├───────────────────────────────────────────────────────────────────┤
│ [panel.soldier.detail.images]                                    │
│   h3 Images                                                       │
│   <form> select-all / import / download selected / delete        │
│   for each image:                                                 │
│     [image card] primary badge, Set as Primary btn, Preview btn  │
│     <img thumb> caption / filename                               │
│   #image-download-status target                                   │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `panel.soldier.detail.summary` | Top card | Bio + dl + advanced research details + danger zone |
| `panel.soldier.detail.records` | Source records section | Pensions, applications, etc. |
| `panel.soldier.detail.images` | Images grid + import/delete toolbar | |

No tabs.

## Atomic components

- `Button` — Edit, Export PDF/JPG, Compare, Update Review, Resolve,
  Mark as Resolved, Delete, Set as Primary, Add/Download/Delete
  Images.
- `Card` — wraps summary, records, image cards.
- `Field` — review note textarea.
- `Pill` — nav (back, open).
- `EmptyState` — no images copy.
- `LinkedText` — renders biography/notes/details with link detection.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Export PDF btn | POST | `routebuilder.SoldierPDF(id)` | `this` | `none` | Triggers download |
| Export JPG btn | POST | `/soldiers/{id}/jpg` | `this` | `none` | Bare URL string |
| Add Images btn | POST | `/soldiers/{id}/images/import` | `#image-download-status` | `none` | Native dialog guarded |
| Download selected images | POST | `routebuilder.SoldierImagesDownload(id)` | `#image-download-status` | `none` | |
| Delete selected images | POST | `/soldiers/{id}/images/delete` | `#image-download-status` | `none` | `hx-confirm` |
| Set as Primary | POST | `routebuilder.SoldierImagesPrimary(id, imgID)` | `#image-download-status` | `none` | |
| Update review note | POST | `routebuilder.SoldierReviewFlag(id)` | `this` | `none` | |
| Mark as Resolved | POST | `/soldiers/{id}/review/resolve` | `this` | `none` | Bare URL |
| Delete Person Record | DELETE | `/soldiers/{id}` | `body` | default | Full nav, destructive `hx-confirm` |

Export popout uses `data-pdf-pref-scope="soldier"` to scope PDF
preferences.

## Modals / overlays

- `overlay.image.viewer` — opened via Preview button
  (`data-image-preview`, `data-image-caption`, `data-image-file`).
- Global overlays.

## State variants

- **Wife / Widow / Linked Person**: extra [Family & Relationships]
  card + Compare action.
- **Soldier with no unit**: Unit Camaraderie section hidden.
- **No source records**: section omitted entirely.
- **No images**: friendly empty state inside the images section.
- **No biography**: biography card omitted.

## Footguns

- **Bare URL strings** in many buttons:
  `/soldiers/{id}/jpg`, `/soldiers/{id}/images/import`,
  `/soldiers/{id}/images/delete`, `/soldiers/{id}/review/resolve`,
  `/soldiers/{id}`, `/compare?id1=…&id2=…&from=…`. Routebuilder
  coverage gap. See [gaps.md](../gaps.md).
- **Native dialog guard**: `Add Images From Computer` opens a native
  picker — MUST be guarded per [dialog-guard.md](../../agents/dialog-guard.md).
  Verify `inFlight.LoadOrStore` is in the handler.
- **Image primary `hx-target="#image-download-status"`** — but the
  Set-as-Primary actually mutates state on the image card. The status
  region is just a placeholder; verify the card re-renders via OOB
  swap or full-page reload.
- **Preview button** is JS-only (`data-image-preview`). Confirm
  `frontend/app.js` wires it to `overlay.image.viewer`.
- **`imageURL(filePath)` returns `file:///` for absolute paths** — fine
  in Wails webview, fragile if ever served from a browser. Document.
- **`stripImageTags`** in alt-text — simple regex stripping. Acceptable
  for alt, not for security.
- **`data-open-external="true"`** — JS intercepts external links. Verify
  it doesn't open browser tabs in Wails webview accidentally.
- **`data-history-back`** — JS-only back navigation. Verify it falls
  back to `data-fallback-href` when no history entry exists.
- **Danger Zone delete** uses `hx-target="body"` — entire body swap
  = full redirect, works because server returns redirect after delete.
- **LinkedText** handles link detection client-side; verify URLs in
  biographies are sanitized (no `javascript:`).

## See also

- [06-soldier-new.md](06-soldier-new.md)
- [07-soldier-edit.md](07-soldier-edit.md)
- [17-service-timeline.md](17-service-timeline.md) (advanced detail)
- [18-unit-camaraderie.md](18-unit-camaraderie.md)