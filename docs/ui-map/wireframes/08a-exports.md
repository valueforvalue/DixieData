# 08a — Share / Exports subpage

- **Route**: `/share/exports` (GET)
- **Builder**: `routebuilder.ExportBackup`, `routebuilder.ExportDatabasePDFAsync`
- **Template**: `internal/templates/share_exports.templ`
- **Layout**: both
- **Owner**: package `templates`
- **Surface ID**: `uiids.PanelShareExports` (`panel.share.exports`)
- **Landing**: see [08-export.md](08-export.md) (sub-overview)
- **Related**: [08b-imports.md](08b-imports.md), [08c-sync.md](08c-sync.md)

The Export & Backup section that lived inline on the pre-#284
`/share` landing. Now a dedicated subpage so the user lands on a
focused surface (export only) instead of scrolling past the
Import + Sync surfaces to find the export buttons.

## Regions (relaxed mode)

```
┌── Share Exports ──────────────────────────────────────────────┐
│ Breadcrumb: ← Back to /share                                  │
│ h2 "Share Exports" + intro copy                               │
├──────────────────────────────────────────────────────────────┤
│ [panel.share.exports] (Export & Backup section)               │
│  ┌─Export & Backup─────────────────────────────────────────┐ │
│  │ Export JSON                                             │ │
│  │ Export Excel (.xlsx)                                    │ │
│  │ Export iCalendar                                        │ │
│  │ Export Static Web Archive (form submit, async)          │ │
│  │ Full Database Printable PDF (opens print-config modal)   │ │
│  │ Export Backup (.ddbak)                                  │ │
│  │ Export Shared Archive (.ddshare)                         │ │
│  │ [include_tags checkbox]                                 │ │
│  │ [Build Share Archive] (opens Share Queue modal)         │ │
│  └─────────────────────────────────────────────────────────┘ │
├──────────────────────────────────────────────────────────────┤
│ [PrintConfigModal partial]                                    │
│   #share-print-config-modal (data-print-config-open)         │
└────────────────────────────────────────────────────────────┘
```

## Modals / overlays

| ID | Trigger | Notes |
| --- | --- | --- |
| `overlay.print-config.modal` (`#share-print-config-modal`) | `data-print-config-open` | Configurable PDF export — scope/filter/sort/group/options. Form posts to `routebuilder.ExportDatabasePDFAsync()`; `data-pdf-pref-scope="archive"`; `hx-on::after-request` redirects on 303. |
| `overlay.share-queue.modal` | `data-share-queue-open` | The Build Share Archive button opens the Share Queue modal. Modal markup lives on the /share/queue page (it's loaded on-demand via the dispatcher, not pre-rendered here). |

## Atomic components

- `Button`, `ButtonContent` — every export action.
- `Card` — section wrappers.
- `Field` — modal form inputs.
- `EmptyState` — zero-archive variant (rendered on the landing, not here — the subpage is reachable only when the user is already on /share).

## HTMX wiring (heavy surface)

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Export JSON | POST | `/export/json` | `this` | `hx-swap="none"` |
| Export xlsx | POST | `/export/csv` | `this` | `hx-swap="none"` |
| Export iCal | POST | `/export/ical` | `this` | `hx-swap="none"` |
| Static Archive | POST | `/export/static-archive?async=1` | (native form submit) | Triggers job, redirect |
| Full DB PDF | — | — | — | Opens `#share-print-config-modal` |
| Backup | POST | `/export/backup` | `this` | `hx-swap="none"` |
| Shared Archive | POST | `/export/shared-archive` | `this` | `hx-swap="none"` |
| Include Tags checkbox | POST | `/share/export-options` | `this` | `data-reload-on-success="true"`. Per-kind `.ddbak` / `.ddshare` flag. |
| Printable PDF submit | POST | `routebuilder.ExportDatabasePDFAsync()` | `this` | `hx-on::after-request` redirects on 303 |
| Build Share Archive | — | — | — | Opens the Share Queue modal via `data-share-queue-open` |

## State variants

- **Zero archive**: no special state on this subpage (the zero-archive empty state lives on the /share landing, not here). The subpage is reachable only when the user is already on the Share surface, so the empty state is upstream of the navigation.
- **Include tags enabled / disabled**: checkbox reflects the per-kind setting stored in archiveMeta. The POST /share/export-options endpoint updates it.
- **Build Share Archive + Share Queue modal**: the modal is reachable from this page. The queue's "empty / non-empty" state is determined by localStorage on the client; see [gaps.md](../gaps.md) for the queue UI shape.

## Footguns

- **Native dialog handlers everywhere** — `/export/backup`,
  `/export/shared-archive`, `/export/static-archive`, image
  imports. Each MUST be guarded per
  [dialog-guard.md](../../agents/dialog-guard.md). Highest-risk
  page in the app. See [gaps.md](../gaps.md).
- **Bare URLs everywhere** — no routebuilder coverage on `/export/*`.
  Renames will silently break. (Future ticket: wrap these in
  `routebuilder.Export*()` helpers.)
- **Printable modal** uses `hx-on::after-request` with inline JS
  to redirect on 303. Wails + HTMX event detail handling — verify
  it works in webview.
- **`data-print-config-open` opens modal via JS**,
  `data-print-config-close` closes. Verify outside-click + Esc
  behavior. The modal markup is included via
  `@partials.PrintConfigModal(exportRecords)` which passes the
  full export-records list.
- **Open Share Queue** is an `<a href="/share/queue">` link
  (issue #310 PR 1). Clicking it navigates to the
  `/share/queue` management page (issue #193) which is the
  single Share Queue management surface post-issue #310.
  The modal at `/share/queue/modal` was deleted in issue
  #310 PR 2; the page absorbs every action the modal used to
  provide (view staged list, export subset, save / load
  presets — presets land on the page in PR 3).
- **`include_tags` checkbox** is per-kind (shared archive kind
  only). The other kinds (backup, static-archive) don't have
  this option because the recipient's archive controls tags on
  import, not the sender.

## See also

- [08-export.md](08-export.md) (landing)
- [08b-imports.md](08b-imports.md) (imports subpage)
- [08c-sync.md](08c-sync.md) (sync subpage)
- [04-browse.md](04-browse.md) (Print/Export Selected deep link)
- [20-jobs.md](20-jobs.md) (job status for exports)
- [gaps.md](../gaps.md) (dialog-guard audit pending)
