# 08b — Share / Imports subpage

- **Route**: `/share/imports` (GET)
- **Builder**: N/A (static launchpad — no data-action submits on the page itself)
- **Template**: `internal/templates/share_imports.templ`
- **Layout**: both
- **Owner**: package `templates`
- **Surface ID**: `uiids.PanelShareImports` (`panel.share.imports`)
- **Landing**: see [08-export.md](08-export.md) (sub-overview)
- **Related**: [08a-exports.md](08a-exports.md), [08c-sync.md](08c-sync.md)

The Import & Restore section that lived inline on the pre-#284
`/share` landing. Now a dedicated subpage so the user lands on a
focused surface (import only) instead of scrolling past the
Export + Sync surfaces. All three import modes (Collaborative
Merge, Memorial JSON, Replace Local Archive) stay grouped here
because the locked decision in #284 was to keep the destructive
path visually isolated from the non-destructive paths.

## Regions (relaxed mode)

```
┌── Share Imports ──────────────────────────────────────────────┐
│ Breadcrumb: ← Back to /share                                  │
│ h2 "Share Imports" + intro copy                               │
├──────────────────────────────────────────────────────────────┤
│ [panel.share.imports] (Import & Restore section)              │
│  ┌─Collaborative Merge─────────────────────────────────────┐ │
│  │ "Use this when another researcher sends you a shared     │ │
│  │  archive..."                                             │ │
│  │ [Import Shared Archive (.ddshare)] (primary button)      │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  ┌─Memorial JSON Import─────────────────────────────────────┐ │
│  │ "Pre-flight analyses how many rows would be added,       │ │
│  │  skipped, or failed; the actual import only runs after   │ │
│  │  you confirm on its own status page."                    │ │
│  │ [Import Memorial JSON (.json)] (secondary button)        │ │
│  │ #memorial-preview-target (attach point for preview       │ │
│  │   fragment swap)                                         │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  ┌─Replace Local Archive (DESTRUCTIVE)─────────────────────┐ │
│  │ Border + bg color signals destructive.                   │ │
│  │ "Use this only when you intentionally want to overwrite  │ │
│  │  the local database with a backup snapshot."             │ │
│  │ [Load Backup (.ddbak)] (danger button, hx-confirm)        │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  [Restore caution card]                                        │
│   "loading a .ddbak replaces the local archive, while         │
│    importing a .ddshare stages Merge Review when needed."      │
└────────────────────────────────────────────────────────────┘
```

## Modals / overlays

None on this page. The Share Queue modal opens from the
Build Share Archive button on /share/exports (not this page).
Native file dialogs open from the import buttons themselves
(see HTMX wiring).

## Atomic components

- `Button`, `ButtonContent` — every import action.
- `Card` — section wrappers (one for each import mode, with
  different border / bg colors for the destructive variant).
- `EmptyState` — not used here (the subpage is reachable only
  when the user is already on the Share surface, not on a
  fresh archive boot).

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Import Shared Archive | POST | `/import/shared-archive` | redirect `/jobs/{id}` | via `X-DixieData-Redirect` |
| Import Memorial JSON | POST | `/import/memorial-json` | redirect `/jobs/{id}` | via `X-DixieData-Redirect`. Pre-flight swap populates `#memorial-preview-target` with the analysis. |
| Load Backup (.ddbak) | POST | `/import/backup` | redirect `/jobs/{id}` | `hx-confirm` (destructive) |

## State variants

- **All three import modes always render** — the page is a
  launchpad. The user's choice of which mode to use is encoded
  in which button they click.
- **Memorial import preview** swaps into
  `#memorial-preview-target` with the analysis (added / skipped /
  failed counts) before the actual import runs. The user can
  review the preview before clicking the secondary "Import
  Memorial JSON" button on the status page that the pre-flight
  redirects to.

## Footguns

- **Destructive path is visually isolated** — the Replace Local
  Archive card uses a different border color (red) and bg
  tint. The danger button style signals destructive. The
  `hx-confirm` data attribute gates the click with a JS confirm
  dialog. The native file dialog opens AFTER the confirm
  succeeds. All three layers must be preserved.
- **Import job results surface per-kind stats on the `/jobs/{id}`
  summary card** (`backup_import` → Replaced records/images +
  Schema migrated line; `shared_import` → Added/Merged/Skipped +
  Conflicts staged + Images imported; `memorial_import` →
  Added/Skipped/Failed + Images imported). Users land here when
  their import finishes — the stats are the first confirmation
  the import succeeded. See [20-jobs.md](20-jobs.md) for the
  full per-kind table.
- **Memorial import log path is invisible** — `JobResult.LogPath`
  is set by `handleConfirmMemorialJSONImport` but the summary
  card doesn't render a download button for it. If the user hits
  the memorial "Preview → Confirm" flow with any `Failed` count,
  they have no in-app path to the error log. See [gaps.md](../gaps.md).
- **Shared-import conflicts reminder is text-only** — when
  `Conflicts > 0` the summary card shows
  `Conflicts staged for review: N — see Merge Review below.` but
  no button. The user has to navigate to the Share landing to
  find the Merge Review section. Consider adding a deep-link pill.
- **Native dialog handlers** — `/import/backup` MUST be guarded
  per [dialog-guard.md](../../agents/dialog-guard.md).
- **`data-dixie-submit` on every import button** — the dispatcher
  POSTs the form, captures the response, and navigates on 303.
  The dispatcher must remain the single submission path; do not
  add a parallel form submit or a `<form>` wrapping the import
  buttons (the form would submit and bypass the dispatcher's
  toast / redirect handling).

## See also

- [08-export.md](08-export.md) (landing)
- [08a-exports.md](08a-exports.md) (exports subpage)
- [08c-sync.md](08c-sync.md) (sync subpage)
- [19-merge-review-ledger.md](19-merge-review-ledger.md) (the
  shared-import conflicts reminder; the Merge Review section on
  the /share landing is where they get resolved)
- [20-jobs.md](20-jobs.md) (job status for imports)
- [gaps.md](../gaps.md) (memorial log path; dialog-guard audit)
