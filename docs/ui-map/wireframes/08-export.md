# 08 — Share Landing (sub-overview)

- **Route**: `/share` (GET)
- **Builder**: N/A (no data-action submits on the landing; all 4 tiles are navigate-to-subpage links)
- **Template**: `internal/templates/share.templ`
- **Layout**: both
- **Owner**: package `templates`
- **Subpages**: see [08a-exports.md](08a-exports.md), [08b-imports.md](08b-imports.md), [08c-sync.md](08c-sync.md)

The /share landing was a single inline surface pre-#284 (Export & Backup card + Import & Restore card + Google Integration card + Support & Diagnostics + Merge Review). Issue #284 split the three primary surfaces into dedicated subpages; the landing is now a sub-overview that links to them.

## Regions (relaxed mode)

```
┌── Share Archive (sub-overview) ────────────────────────────────┐
│ if zero records: EmptyStateCard("share", counts)              │
│ h2 "Share Archive" + intro copy                                │
├────────────────────────────────────────────────────────────────┤
│ [panel.share.quick-actions] (4 tiles, sm:grid-cols-2 lg:grid-cols-4)│
│  ┌──Export──┐ ┌──Import──┐ ┌──Share Queue──┐ ┌──Sync──┐     │
│  │→/exports │ │→/imports │ │→/share/queue  │ │→/sync  │     │
│  └──────────┘ └──────────┘ └───────────────┘ └────────┘     │
│  Each tile is a plain navigate-to-subpage <a> (no data-action │
│  submit; the actions live on the subpages).                    │
├────────────────────────────────────────────────────────────────┤
│ [panel.share.recent] (last 3 terminal jobs, StartedAt desc)   │
│   per row: [Kind] [status pill] [relative time]                │
│   empty state: "No exports or imports yet."                   │
│   each row links to /jobs/{id}                                 │
├────────────────────────────────────────────────────────────────┤
│ [Support & Diagnostics card] (full-width)                      │
│  Export Feedback Log | Export Bug Report Bundle                 │
│  /export/feedback-log + /export/bug-report (data-action submit)│
├────────────────────────────────────────────────────────────────┤
│ if len(conflicts) > 0:                                         │
│   [Merge Review section]  #merge-review-section                │
│     [Loaded status pill] "Data Loaded: N Conflicts Found"     │
│     per conflict:                                               │
│       [Conflict card]                                           │
│         Local vs Incoming (responsive-two-col)                  │
│         Inspect Diff (data-merge-review-diff-toggle)           │
│         Keep Local / Keep Incoming / Keep Both                 │
│         (collapsible) field-by-field diff                     │
│         "remembers that mapping" copy (display-id-collision)   │
└────────────────────────────────────────────────────────────────┘
```

## Modals / overlays

None. The print-config + Google Calendar Preferences modals moved
to the owning subpages (`/share/exports` + `/share/sync` respectively).

## Atomic components

- `QuickAction` — the 4 Quick Action tiles
- `ButtonContent` — Support & Diagnostics buttons + Merge Review per-conflict action buttons
- `Card` — section wrappers
- `RecentJobs` — last 3 jobs card
- `EmptyState` — zero-archive variant

## HTMX wiring

| Trigger | URL | Notes |
| --- | --- | --- |
| Quick Action tile Export | `GET /share/exports` | Navigate-to-subpage (no htmx) |
| Quick Action tile Import | `GET /share/imports` | Navigate-to-subpage |
| Quick Action tile Share Queue | `GET /share/queue` | Navigate-to-subpage |
| Quick Action tile Sync | `GET /share/sync` | Navigate-to-subpage |
| Support: Export Feedback Log | `POST /export/feedback-log` | `data-dixie-submit` |
| Support: Export Bug Report Bundle | `POST /export/bug-report` | `data-dixie-submit` |
| Merge Review: Keep Local | `POST /merge-review/{id}/keep-local` | `data-merge-review-action`, `hx-confirm` |
| Merge Review: Keep Incoming | `POST /merge-review/{id}/keep-shared` | `data-merge-review-action`, `hx-confirm` |
| Merge Review: Keep Both | `POST /merge-review/{id}/keep-both` | `data-merge-review-action`, only on display-id-collision, `hx-confirm` |
| Merge Review diff toggle | — | JS (`data-merge-review-diff-toggle`) |

## State variants

- **Zero archive**: `EmptyStateCard("share", counts)` at top.
- **No merge conflicts**: Merge Review section omitted entirely.
- **Non-zero conflicts (any type)**: Merge Review section renders with per-conflict Keep Local / Keep Incoming actions + the diff + the "remembers that mapping" copy.
- **Display-id-collision conflicts**: the per-conflict card also renders the Keep Both button + the "preserves the local record" copy.

## Footguns

- **No modals on the landing** — the print-config and Google Calendar
  Preferences modals moved to /share/exports and /share/sync
  respectively. Any audit or test that looks for these modals on
  the landing is a bug.
- **Quick Action tiles are NOT data-action submits** — pre-#284 the
  Export JSON + Load Backup tiles ran the action immediately.
  Post-#284 they navigate to the subpage where the user clicks
  the action. Two clicks vs one — intentional per the locked
  decision (the surfaces are now focused; the previous landing
  was too dense to scan).
- **Foldout: 4 items, Build folded into Export** — Export /
  Import / Share Queue / Sync. Build Share Archive folded into
  Export (the Build button lives on /share/exports). The 4-item
  menu (without Build) gives the user a direct entry to the
  Sync subpage from the top nav; if the menu were 3 items, Sync
  would only be reachable via the /share landing's 4th Quick
  Action tile. The
  in-page anchor deep links (`/share#export-section`,
  `/share#import-section`) still work for backward compat; the
  anchors are harmless on the new landing because the IDs no
  longer exist (the old inline sections were removed).
- **Recent Activity card** shows the last 3 jobs across ALL
  surfaces (export / import / merge / share-queue). The kind
  label distinguishes them. See [20-jobs.md](20-jobs.md) for
  the per-kind result summary.
- **Merge Review** can grow long with N conflicts — verify
  scroll preservation + the `data-merge-review-loaded-status`
  `aria-live` announces when conflicts arrive.
- **`mergeReviewConfirmMessage` is templated** — verify long
  incoming display IDs don't overflow the confirm dialog.

## See also

- [08a-exports.md](08a-exports.md) — /share/exports subpage
- [08b-imports.md](08b-imports.md) — /share/imports subpage
- [08c-sync.md](08c-sync.md) — /share/sync subpage
- [20-jobs.md](20-jobs.md) (job status for exports/imports)
- [gaps.md](../gaps.md) (folded-in Build button is the future target for the dialog-guard audit)
