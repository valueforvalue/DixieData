# 21 — Settings

- **Route**: `/settings` (GET)
- **Builders**: `routebuilder.SettingsDebugMode`,
  `routebuilder.SettingsInitialize`, `routebuilder.SettingsUpdateSource`,
  `routebuilder.SettingsUpdateCheck`, `routebuilder.SettingsUpdateApply`,
  `routebuilder.SettingsImagesOrphansScan`,
  `routebuilder.SettingsImagesOrphansCleanup`,
  `routebuilder.SettingsQualityScan`, `routebuilder.SettingsQualityApply`,
  `routebuilder.ExportBackup`
- **Template**: `internal/templates/entry_form.templ:SettingsView`,
  `SettingsUpdatePanel`, `SettingsUpdateStatus`,
  `SettingsUpdateStatusMessage`, `SettingsUpdateApplyStarted`,
  `SettingsOrphanedImages`, `SettingsOrphanCleanupResult`,
  `SettingsQualityScanResults`, `SettingsQualityScanApplyResult`
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Settings ───────────────────────────────────────────────────────┐
│ h2 "Settings"                                                     │
│                                                                    │
│ [panel.settings.updates] #settings-update-panel                  │
│  h3 Software Updates                                              │
│  Current version + build identity                                  │
│  Notice / Last apply status card                                  │
│  <form> Save Update Source (hx-post UpdateSource, outerHTML)     │
│  <form> Use Default GitHub Feed (clears source_url)              │
│  Source card (default or custom + URL)                           │
│  if CanApply + Disabled reason: error callout                     │
│  <form> Check for Updates                                        │
│  <form> Export Backup                                             │
│  if CanApply: <form> Download and Apply Latest Update            │
│  [SettingsUpdateStatus result card — swaps into same panel]       │
│                                                                    │
│ [panel.settings.layout]                                          │
│  h3 Responsive Layout Mode                                       │
│  Current mode + preference pill                                   │
│  3 option buttons (auto / relaxed / split-screen)                │
│                                                                    │
│ [panel.settings.debug]                                            │
│  h3 Debug Mode                                                     │
│  <form> hx-post SettingsDebugMode (no swap)                       │
│    debug_mode checkbox + Apply                                    │
│                                                                    │
│ [panel.settings.initialize]                                       │
│  h3 Initialize Data (RED — destructive)                          │
│  <form> hx-post SettingsInitialize                                │
│    "Type <confirmationWord> to confirm" input                    │
│    <btn: Initialize Data (danger)> <btn: Back (ghost)>           │
│                                                                    │
│ [Image Maintenance card]                                          │
│  <form> hx-post SettingsImagesOrphansScan → #settings-orphan-…   │
│    <btn: Scan for Orphaned Images>                                │
│  [SettingsOrphanedImages result card]                              │
│    if orphans > 0: <form hx-post Cleanup>                         │
│      list of orphans (path, mtime, size)                          │
│      <btn: Move Listed Files to Temp Trash (danger, hx-confirm)>  │
│  [SettingsOrphanCleanupResult] card                              │
│                                                                    │
│ [Data Quality Scan card]                                          │
│  radio: high-confidence / advanced                                │
│  <form> hx-post SettingsQualityScan → #settings-quality-…        │
│    <btn: Run Data Quality Scan>                                   │
│  [SettingsQualityScanResults card]                                │
│    if issues > 0:                                                 │
│      <form hx-post SettingsQualityApply>                          │
│        per-group <details> with checkboxes                       │
│        <btn: Move Selected to Review Queue (primary, hx-confirm)>│
│  [SettingsQualityScanApplyResult] card                            │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `panel.settings.layout` | Layout Mode card | Auto / Relaxed / Split-screen |
| `panel.settings.initialize` | Initialize Data card | Destructive confirmation |
| `panel.settings.updates` | Software Updates panel | Source + check + apply |
| `panel.settings.debug` | Debug Mode card | Toggle |

Image Maintenance and Data Quality Scan are NOT registered in
uiids. Candidate additions.

## Atomic components

- `Button` — Apply (debug), Initialize (danger), Back (ghost), Scan,
  Move to Trash, Run Quality Scan, Move Selected.
- `Card` — wraps each section.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Save Update Source | POST | `routebuilder.SettingsUpdateSource()` | `#settings-update-panel` | `hx-swap="outerHTML"` re-renders whole panel |
| Use Default Feed | POST | `routebuilder.SettingsUpdateSource()` | `#settings-update-panel` | Empty source_url hidden input |
| Check for Updates | POST | `routebuilder.SettingsUpdateCheck()` | `this` | `hx-swap="none"` — UI uses `SettingsUpdateStatus` slot |
| Export Backup | POST | `routebuilder.ExportBackup()` | `this` | `hx-swap="none"` |
| Download + Apply Update | POST | `routebuilder.SettingsUpdateApply()` | `this` | `hx-swap="none"`, `data-progress-label` |
| Debug Apply | POST | `routebuilder.SettingsDebugMode()` | `this` | `hx-swap="none"` |
| Initialize | POST | `routebuilder.SettingsInitialize()` | `this` | `hx-swap="none"`, requires confirmation word |
| Scan Orphaned | POST | `routebuilder.SettingsImagesOrphansScan()` | `#settings-orphan-results` | |
| Move to Trash | POST | `routebuilder.SettingsImagesOrphansCleanup()` | `#settings-orphan-results` | `hx-confirm` |
| Quality Scan | POST | `routebuilder.SettingsQualityScan()` | `#settings-quality-results` | |
| Apply Quality | POST | `routebuilder.SettingsQualityApply()` | `#settings-quality-results` | `hx-confirm` |
| Layout mode buttons | — | — | — | JS-only (`data-layout-mode-option`) |

## Modals / overlays

None local. Global overlays (floating menu, jobs, feedback).

## State variants

- **Update applied**: `LastApply.Status="success"` — green card.
- **Update failed**: red card.
- **Apply disabled**: `DisabledReason` rendered as error callout.
- **No orphans found**: explicit copy.
- **No quality issues**: green "no issues" card.
- **Quality scan found nothing**: same as above.

## Footguns

- **Destructive Initialize** — relies on user typing the
  `confirmationWord` correctly. Verify server validation matches.
- **Native dialog** — `SettingsUpdateApply` opens update installer.
  MUST be guarded per [dialog-guard.md](../../agents/dialog-guard.md).
- **`SettingsUpdateCheck` uses `hx-swap="none"`** but presumably
  renders the result somewhere — verify the result injection.
  `SettingsUpdateStatus` is a separate templ that renders the
  result card. The wiring might rely on a specific OOB swap or
  manual DOM insert.
- **`data-layout-mode-option` switches** — JS-only state. Verify
  `localStorage` persistence.
- **Image orphan scan** can find N orphans; cleanup moves them to
  trash for 30 days. Verify the 30-day retention claim.
- **Quality scan groups are `<details>`** — collapsed by default.
  Verify user can scan, expand groups, select, and apply in one
  session without losing state on re-scan.
- **`disabled` attribute on "Apply Update"** when `!CanApply` — verify
  the visible alternative copy ("DisabledReason") is informative.
- **All bare URL candidates** have routebuilders, but the
  `data-history-back` for Initialize's Back button uses
  `/calendar` — fine, but check it's the intended landing.

## See also

- [22-initial-setup.md](22-initial-setup.md) (first-launch flow)
- [23-recovery.md](23-recovery.md) (post-update recovery)
- [08-export.md](08-export.md) (Export Backup)