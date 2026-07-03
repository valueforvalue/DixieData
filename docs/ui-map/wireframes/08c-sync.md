# 08c — Share / Sync subpage

- **Route**: `/share/sync` (GET)
- **Builder**: `routebuilder.GoogleCalendarPreferencesSave`,
  `routebuilder.GoogleConnect`, `routebuilder.GoogleBackup`,
  `routebuilder.GoogleSheetsExport`, `routebuilder.GoogleCalendarUseManaged`,
  `routebuilder.GoogleCalendarSyncManaged`, `routebuilder.GoogleCalendarUnsyncManaged`,
  `routebuilder.GoogleCalendarUseTest`, `routebuilder.GoogleCalendarSyncTest`,
  `routebuilder.GoogleCalendarUnsyncTest`
- **Template**: `internal/templates/share_sync.templ`
  (uses partial `@partials.GoogleCalendarPreferencesModal(status)`)
- **Layout**: both
- **Owner**: package `templates`
- **Landing**: see [08-export.md](08-export.md) (sub-overview)
- **Related**: [08a-exports.md](08a-exports.md), [08b-imports.md](08b-imports.md)

The Google Integration card that lived inline on the pre-#284
`/share` landing. Now a dedicated subpage so the user lands on a
focused surface (sync only) instead of scrolling past the Export
+ Import surfaces. The Google Calendar Preferences modal moved
with the card (it follows the surface it belongs to, not the
landing it used to live on).

## Regions (relaxed mode)

```
┌── Share Sync ─────────────────────────────────────────────────┐
│ Breadcrumb: ← Back to /share                                 │
│ h2 "Share Sync" + intro copy                                │
├──────────────────────────────────────────────────────────────┤
│ [Google Integration card]                                    │
│  h3 "Google Integration" + intro copy                        │
│  if SharedClientAvailable:                                    │
│    "Shared Google app credentials were detected..."           │
│    "Loaded from: {path}"                                     │
│  else:                                                        │
│    "Save settings first, then connect the account..."         │
│    "If you want shared settings, place                       │
│     google-oauth-defaults.json next to DixieData.exe."       │
│                                                                │
│  [Connect Google Account] (primary)                           │
│  [Disconnect]                                                 │
│  [Upload Backup to Google Drive] (primary)                    │
│  [Export CSV to Google Sheets]                                │
│                                                                │
│  ┌─DixieData Calendar─────────────────────────────────────┐  │
│  │ [Use Calendar] [Sync] [Unsync] [Preferences]            │  │
│  │ "Compact flow: Use once, then Sync."                    │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                │
│  ┌─DixieData Test Calendar────────────────────────────────┐  │
│  │ [Use Test] [Test Sync] [Test Unsync]                    │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                │
│  [Status card]                                                │
│   Status: Connected / Not connected                           │
│   using shared OAuth app / shared OAuth app available         │
│   DixieData Calendar ID: {id}                                 │
│   DixieData Test Calendar ID: {id}                            │
│   Last synced: {ts} (if set)                                  │
│   Out of sync / In sync                                      │
│   Added N • Updated N • Removed N (drift)                     │
│                                                                │
│  [Shared deployment option card]                              │
│                                                                │
│  #google-status (status update attach point)                  │
├──────────────────────────────────────────────────────────────┤
│ [Google Calendar Preferences modal partial]                  │
│   #google-calendar-preferences-modal (data-google-calendar-   │
│   preferences-open / -close)                                  │
│   Form: title format / start time / description fields /      │
│   primary reminder / secondary reminder / sample preview /    │
│   save button. Posts to routebuilder.GoogleCalendarPreferencesSave().│
└──────────────────────────────────────────────────────────────┘
```

## Modals / overlays

| ID | Trigger | Notes |
| --- | --- | --- |
| `overlay.google-calendar-prefs.modal` (`#google-calendar-preferences-modal`) | `data-google-calendar-preferences-open` | DixieData Calendar managed event preferences — title format / start time / reminders / description fields. Form posts to `routebuilder.GoogleCalendarPreferencesSave()`. Lives in `internal/templates/partials/google_calendar_preferences_modal.templ`. |

## Atomic components

- `Button` — every Google action.
- `Card` — section wrappers (main Google card, two calendar sub-cards, status card, shared deployment card).
- `Field` — modal form inputs.

## HTMX wiring

| Trigger | Verb | URL | Notes |
| --- | --- | --- | --- |
| Connect Google | POST | `/integrations/google/connect` | `data-dixie-submit` |
| Disconnect | POST | `/integrations/google/disconnect` | `data-dixie-submit` |
| Upload to Drive | POST | `/integrations/google/backup` | `data-dixie-submit` |
| Export CSV to Sheets | POST | `/integrations/google/sheets/export` | `data-dixie-submit` |
| Use Calendar (managed) | POST | `/integrations/google/calendar/use-managed` | `data-busy-group="google-calendar-actions"`, `data-progress-label` |
| Sync (managed) | POST | `/integrations/google/calendar/sync-managed` | same |
| Unsync (managed) | POST | `/integrations/google/calendar/unsync-managed` | `hx-confirm` + busy group + progress |
| Use Test | POST | `/integrations/google/calendar/use-test` | same |
| Test Sync | POST | `/integrations/google/calendar/sync-test` | same |
| Test Unsync | POST | `/integrations/google/calendar/unsync-test` | busy group + progress (no confirm — only managed Unsync has the destructive `hx-confirm` because that's the one that actually removes events) |
| Preferences | — | — | Opens the Google Calendar Preferences modal via `data-google-calendar-preferences-open` |
| Save Calendar Preferences | POST | `routebuilder.GoogleCalendarPreferencesSave()` | `data-dixie-submit` |

## State variants

- **Not connected**: status pill says "Not connected".
- **Connected + In sync**: status pill says "Connected" + "In sync" + drift counts = 0/0/0.
- **Connected + Out of sync**: status pill says "Connected" + "Out of sync" + drift counts shown.
- **Using shared client**: badge "using shared OAuth app" appears next to the status pill (only if `UsingSharedClient == true`).
- **Shared client available but not in use**: badge "shared OAuth app available" appears (only if `SharedClientAvailable == true && !UsingSharedClient`).

## Footguns

- **`data-busy-group="google-calendar-actions"`** — JS-level
  lockout to prevent concurrent calendar actions. Verify on
  every calendar button (managed + test) and the Disconnect
  button. A click that arrives while a previous action is
  in-flight is ignored.
- **`data-progress-label` on every calendar action** — used by
  the jobs-progress overlay (top of the page) to show "Syncing
  DixieData Calendar..." etc. The label MUST be human-readable
  and identify which calendar (managed vs test) is being acted
  on.
- **Unsync Managed has `hx-confirm`** — it actually removes
  events. The other Unsync (Test) does not, so it has no
  confirm. The asymmetry is intentional: the test calendar is
  throwaway; the managed calendar is the user's real data.
- **Modal opens via JS** — `data-google-calendar-preferences-open`
  opens the modal that lives at the end of the templ (rendered
  as a partial). The modal is NOT loaded on demand; it's
  pre-rendered with the current preferences so the user can
  edit any single field and submit. Outside-click + Esc behavior
  is handled by the modal's JS hook in app.js.
- **Bare URLs on /integrations/google/calendar/...** — no
  routebuilder coverage. Renames will silently break. (Future
  ticket: wrap these in `routebuilder.GoogleCalendar*()`
  helpers.)
- **Native dialog handlers** — Google connect flow uses
  browser-based OAuth (not a Wails native dialog), so the
  dialog-guard law does not apply here. The Wails WebView2
  opens the Google OAuth URL in a separate browser window.

## See also

- [08-export.md](08-export.md) (landing)
- [08a-exports.md](08a-exports.md) (exports subpage)
- [08b-imports.md](08b-imports.md) (imports subpage)
- [21-settings.md](21-settings.md) (Google OAuth default settings
  + Settings surface for the "use shared client" toggle)
- [gaps.md](../gaps.md) (routebuilder coverage gap)
