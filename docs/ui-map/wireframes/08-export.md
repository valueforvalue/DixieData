# 08 — Share / Export

- **Route**: `/share` (GET)
- **Builder**: `routebuilder.ExportBackup`, `routebuilder.ExportDatabasePDFAsync`,
  `routebuilder.GoogleCalendarPreferencesSave`
- **Template**: `internal/templates/share.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Share Archive ──────────────────────────────────────────────────┐
│ if zero records: EmptyStateCard("share", counts)                 │
│ h2 + intro copy                                                   │
├───────────────────────────────────────────────────────────────────┤
│ [panel.export.actions] (responsive-two-col → 2 sections side-by-side)│
│                                                                    │
│  ┌── Export & Backup ─────────┐  ┌── Import & Restore ─────────┐  │
│  │ Export JSON               │  │ [Collaborative Merge card]  │  │
│  │ Export Excel (.xlsx)      │  │   Import Shared Archive     │  │
│  │ Export iCalendar          │  │   → #share-status            │  │
│  │ Export Static Web Archive │  │ [Memorial JSON Import card] │  │
│  │ Full Database PDF (modal) │  │   Preview Memorial JSON     │  │
│  │ Export Backup (.ddbak)    │  │ [Replace Local Archive RED] │  │
│  │ Export Shared Archive     │  │   Load Backup (.ddbak)       │  │
│  └───────────────────────────┘  └─────────────────────────────┘  │
│                                                                    │
│ [Support & Diagnostics card] (responsive-span-2)                 │
│   Export Feedback Log | Export Bug Report Bundle                 │
│                                                                    │
│ #share-status — import/export status messages                     │
├───────────────────────────────────────────────────────────────────┤
│ if len(conflicts) > 0:                                            │
│   [Merge Review section]  #merge-review-section                    │
│     [Loaded status pill] "Data Loaded: N Conflicts Found"        │
│     per conflict:                                                  │
│       [Conflict card]                                              │
│         Local vs Incoming (responsive-two-col)                     │
│         Inspect Diff (data-merge-review-diff-toggle)              │
│         Keep Local / Keep Incoming / Keep Both                    │
│         (collapsible) field-by-field diff                        │
├───────────────────────────────────────────────────────────────────┤
│ [panel.export.google]                                             │
│   Status block (shared client availability, loaded-from path)    │
│   Connect / Disconnect                                            │
│   Upload Backup to Drive | Export CSV to Google Sheets            │
│   DixieData Calendar group: Use / Sync / Unsync / Preferences    │
│   DixieData Test Calendar group: Use Test / Test Sync / Test Unsync│
│   [Status card] — Connected? | Out of sync / In sync | drift counts│
│   #google-status                                                   │
│                                                                    │
│   [Google Calendar preferences modal]  overlay.google-calendar-prefs│
└───────────────────────────────────────────────────────────────────┘
```

## Modals / overlays

| ID | Region | Notes |
| --- | --- | --- |
| `overlay.print-config.modal` | `#share-print-config-modal` | Configurable PDF export — scope/filter/sort/group/options |
| `overlay.google-calendar-prefs.modal` | `#google-calendar-preferences-modal` | Title format / start time / reminders / description fields |

Both modals contain `data-print-config-close` /
`data-google-calendar-preferences-close` and the printable modal has
the full export-config form (`data-pdf-pref-scope="archive"`).

## Atomic components

- `Button`, `ButtonContent` — every export/import action.
- `Card` — section wrappers.
- `Field` — modal form inputs.
- `EmptyState` — zero-archive variant.

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
| Import Shared Archive | POST | `/import/shared-archive` | `#share-status` | |
| Preview Memorial JSON | POST | `/import/memorial-json/preview` | `#share-status` | |
| Load Backup | POST | `/import/backup` | `#share-status` | `hx-confirm`, destructive |
| Connect Google | POST | `/integrations/google/connect` | `this` | `hx-swap="none"` |
| Disconnect | POST | `/integrations/google/disconnect` | `this` | |
| Upload to Drive | POST | `/integrations/google/backup` | `this` | |
| Export CSV to Sheets | POST | `/integrations/google/sheets/export` | `this` | |
| Use / Sync / Unsync Calendar | POST | `/integrations/google/calendar/...` | `this` | `data-busy-group="google-calendar-actions"`, `data-progress-label` |
| Use / Sync / Unsync Test Calendar | POST | `/integrations/google/calendar/...test...` | `this` | same |
| Save Calendar Preferences | POST | `routebuilder.GoogleCalendarPreferencesSave()` | `this` | |
| Keep Local | POST | `/merge-review/{id}/keep-local` | `this` | `hx-confirm` per-conflict |
| Keep Incoming | POST | `/merge-review/{id}/keep-shared` | `this` | |
| Keep Both | POST | `/merge-review/{id}/keep-both` | `this` | only on display-id-collision |
| Merge Review diff toggle | — | — | — | JS (`data-merge-review-diff-toggle`) |
| Printable PDF submit | POST | `routebuilder.ExportDatabasePDFAsync()` | `this` | `hx-on::after-request` redirects on 303 |

## State variants

- **Zero archive**: `EmptyStateCard("share", counts)` at top.
- **No merge conflicts**: section omitted entirely.
- **Google not connected**: status pill says "Not connected".
- **Google out of sync**: drift counts shown.

## Footguns

- **Native dialog handlers everywhere** — `/export/backup`,
  `/export/shared-archive`, `/export/static-archive`, image imports,
  calendar actions. Each MUST be guarded per
  [dialog-guard.md](../../agents/dialog-guard.md). Highest-risk page
  in the app. See [gaps.md](../gaps.md).
- **Bare URLs everywhere** — no routebuilder coverage on `/export/*`,
  `/import/*`, `/integrations/*`, `/merge-review/*`. Renames will
  silently break.
- **Printable modal** uses `hx-on::after-request` with inline JS to
  redirect on 303. Wails + HTMX event detail handling — verify it
  works in webview.
- **`data-busy-group="google-calendar-actions"`** — JS-level lockout
  to prevent concurrent calendar actions. Verify on every calendar
  button.
- **`data-progress-label` + `data-async-job-redirect="true"`** on
  printable PDF — confirm the redirect flow handles the job ID
  correctly.
- **Printable modal: `data-print-scope-value` radios + scope-panel**
  shows/hides filter/record sections. JS-driven.
- **`data-print-config-open` opens modal via JS**, `data-print-config-close`
  closes. Verify outside-click + Esc behavior.
- **Merge Review** can grow long with N conflicts — verify scroll
  preservation + the `data-merge-review-loaded-status` `aria-live`
  announces when conflicts arrive.
- **Each conflict's Keep Both button** is conditionally rendered
  (only for `display-id-collision`). Tests must check both branches.
- **`mergeReviewConfirmMessage` is templated** — verify long incoming
  display IDs don't overflow the confirm dialog.

## See also

- [04-browse.md](04-browse.md) (Print/Export Selected deep link)
- [20-jobs.md](20-jobs.md) (job status for exports)
- [21-settings.md](21-settings.md) (debug mode toggle surfaces here)
- [gaps.md](../gaps.md) (dialog-guard audit pending)