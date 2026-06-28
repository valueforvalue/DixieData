# 20 — Jobs (Status / Report / Slot Fragment)

- **Routes**:
  - `/jobs/{id}` — landing page (`JobStatusView`)
  - `/jobs/{id}/status` — polling fragment (`JobStatusFragment`)
  - `/jobs/{id}/status?slot=1` — slot fragment (`JobStatusSlotFragment`)
  - `/jobs/{id}/report` — printable report (`JobReportView`)
  - `/jobs/{id}/artifact` — binary download (no template)
  - `/jobs/{id}/cancel` — POST cancel (no template)
  - `/jobs/active` — polled by Layout overlay
- **Builders**: `routebuilder.JobStatus(id)`, `routebuilder.JobStatusSlot(id)`,
  `routebuilder.ActiveJobs()`
- **Template**: `internal/templates/jobs.templ`,
  `internal/templates/job_slot_fragment.templ`
- **Owner**: package `templates`

## Regions — Job Status Page (`/jobs/{id}`)

```
┌── Background Export ─────────────────────────────────────────────┐
│ "Background Export" + <h2> {kind label} </h2>                     │
│ Job ID <code>                                                     │
├───────────────────────────────────────────────────────────────────┤
│ [panel.job.status] #job-status-body                              │
│  if running/queued:                                                │
│    Status pill + Message                                         │
│    [Progress bar] % complete                                      │
│    <btn: Cancel> (form POST)                                      │
│  if done:                                                         │
│    [jobSummaryCard]                                               │
│      Headline (kind-specific one-liner, may include size)         │
│      DetailLines list (kind-specific — see Stats Variants below) │
│      if ResultPath: "Saved to <code>...</code>"                   │
│      <btn: Dismiss (primary)> <btn: Show report (secondary)>    │
│      if has artifact:                                             │
│        <a: Open> (if viewable) or <a: Save> (download)            │
│      NOTE: Memorial log download button NOT wired — see [gaps]  │
│  if error: red callout                                           │
│  if cancelled: slate callout                                     │
│                                                                    │
│ HTMX poll: hx-get={JobStatus(id)} hx-trigger="every 2s"           │
│           hx-swap="outerHTML" hx-target="#job-status-body"        │
│           hx-trigger="none" when terminal                         │
├───────────────────────────────────────────────────────────────────┤
│ "Status updates automatically…" copy                              │
└───────────────────────────────────────────────────────────────────┘
```

## Stats variants on the summary card

`Job.Summary()` builds a kind-specific `DetailLines` list. As of
commit `70878ac` the helpers `appendExportStats`,
`appendSharedImportStats`, `appendMemorialImportStats`, and
`appendBackupRestoreStats` add per-kind stats lines below the
base Size / Duration.

| Kind(s) | Extra lines (in order) | Trigger field on `JobResult` |
| --- | --- | --- |
| `json_export`, `excel_export`, `icalendar_export`, `database_pdf`, `static_archive` | `Person records: N` · `Images: N` · `Source records: N` (each only if > 0) | `Records`, `Images`, `Sources` |
| `backup_archive`, `shared_archive` | Same as above | Same — populated from `backup.Manifest` |
| `soldier_pdf`, `soldier_jpg`, `monthly_pdf`, `insights_pdf`, `bug_report` | **No stats** (only Size + Duration) | — |
| `image_import` | Uses `j.Message` (worker-supplied) instead of stats | — |
| `backup_import` | `Replaced: N records, M images` + `Schema migrated: backup vN → current vM` *or* `Schema: backup vN = current vM (no migration)` | `ReplacedRecords`, `ReplacedImages`, `BackupSchema`, `CurrentSchema`, `MigrationRan` |
| `shared_import` | `Person records: N added, M merged, K skipped` · `Conflicts staged for review: N — see Merge Review below.` (if N > 0) · `Images imported: N` · `Source records imported: N` | `Added`, `Merged`, `Skipped`, `Conflicts`, `ImagesImported`, `SourcesImported` |
| `memorial_import` | `Person records: N added, K skipped, F failed` · `Images imported: N` | `Added`, `Skipped`, `Failed`, `ImagesImported` |

`JobReportView` (`/jobs/{id}/report`) iterates the same
`Summary().DetailLines` and inherits the new stats without any
templ change.

> **Important**: when `shared_import` shows
> `Conflicts staged for review: N`, the user is expected to open
> Merge Review from the Share page. The summary card itself does
> not link there — the reminder is text-only. See [08-export.md](08-export.md)
> for the Merge Review section.

## Regions — Slot Fragment (`/jobs/{id}/status?slot=1`)

```
┌── compact progress card (rendered into [data-jobs-progress-region])──┐
│ Status pill | DisplayLabel                                        │
│ [Progress bar thin]                                                │
│ if Message: copy                                                  │
│ if done + ResultPath: <a: Open result>                            │
│ if error: red error copy                                          │
│                                                                    │
│ HTMX poll: hx-get={JobStatusSlot(id)} hx-trigger="every 2s"       │
│           hx-swap="innerHTML" hx-target="[data-jobs-progress-…]"  │
│           hx-trigger="none" when terminal                         │
└───────────────────────────────────────────────────────────────────┘
```

> **Critical**: `innerHTML`, NOT `outerHTML`. The outer
> `[data-jobs-progress-region]` div must survive every poll. The
> slot fragment comment in `job_slot_fragment.templ` documents the
> "stuck at 5%" bug if you swap to `outerHTML`.

## Regions — Job Report (`/jobs/{id}/report`)

```
┌── report-shell (printable, white bg) ─────────────────────────────┐
│ header: kind label + Job ID                                       │
│ § Status (Done / Error / Cancelled / in progress)                 │
│ § Summary (headline + detail lines)                               │
│ § Timeline (queued / finished / duration)                         │
│ § Artifact (path + size + open/save link)                         │
│ § Error (if any)                                                  │
│ footer: "Generated by DixieData. Use browser print-to-PDF."       │
└───────────────────────────────────────────────────────────────────┘
```

No poll on this page — it's a static snapshot.

## Atomic components

- `Button` — Dismiss, Show report, Cancel.
- `Card` — wraps summary.

## Modals / overlays

None local. The slot fragment IS the global `overlay.jobs.progress`.

## HTMX wiring summary

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Layout active jobs poll | GET | `routebuilder.ActiveJobs()` | `[data-jobs-progress-region]` | `innerHTML` | `load, every 3s`, filtered by `jobs.SilentKinds` |
| Job status page poll | GET | `routebuilder.JobStatus(id)` | `#job-status-body` | `outerHTML` | `every 2s`, stops when terminal |
| Slot fragment poll | GET | `routebuilder.JobStatusSlot(id)` | `[data-jobs-progress-region]` | `innerHTML` | `every 2s`, stops when terminal |
| Cancel | POST | `/jobs/{id}/cancel` | (default) | (default) | Bare URL — routebuilder gap |
| Artifact | GET | `/jobs/{id}/artifact` | — | — | Binary download |
| Report | GET | `/jobs/{id}/report` | — | — | Full page render |
| Open result (slot) | GET | `/jobs/{id}/artifact` | new tab | — | `<a target="_blank">` |

## State variants

- **Queued / Running**: progress bar + cancel.
- **Done**: summary card with artifact action.
- **Error**: red callout + Error text.
- **Cancelled**: slate callout.

## Footguns

- **Poll stops correctly only when terminal status is rendered into
  the swap**. The `hx-trigger="none"` is conditional on
  `job.Status` being terminal — verify goquery test asserts the
  attribute is absent during running.
- **`innerHTML` vs `outerHTML`**: slot fragment uses `innerHTML`,
  page fragment uses `outerHTML`. Comment in
  `job_slot_fragment.templ` is a load-bearing note. See also
  `docs/COMMON_BUGS.md`.
- **Bare `/jobs/{id}/cancel`, `/jobs/{id}/artifact`,
  `/jobs/{id}/report`** — routebuilder gap.
- **Dismiss button uses inline `onclick="window.location.assign(...)"**
  — `quoteAttr` helper escapes for JS string literal. Fragile if
  `DismissTargetPath` ever contains special chars.
- **`static_archive` is a `SilentKind`** — the slot overlay stays
  empty for it; the landing page is the intended surface. Verify
  `jobs.SilentKinds` registry.
- **Polling at 2s while UI navigates** — verify HTMX cancels in-flight
  requests on `htmx:beforeSwap` or unmount.
- **Printable report (`JobReportView`)** has `bg-white p-6 print:p-0`
  — verify print CSS.
- **Stats lines are conditional on worker discipline** —
  `appendExportStats` etc. only append a line when the count is > 0.
  If a worker forgets to call `jobs.SetResult` (or calls it with zero
  values), the summary card silently shows the legacy "Size + Duration"
  copy with no warning that stats are missing. Verify per-kind tests
  assert the new lines render.
- **Memorial log download button missing** — `JobResult.LogPath` is
  populated by `handleConfirmMemorialJSONImport` but `jobSummaryCard`
  does not render a download button for it. The job's
  `Summary().ResultPath` stays empty for memorial imports, so the
  user has no in-app way to retrieve the error log written to disk.
  The summary card itself loads fine — this is a missing affordance,
  not a load failure. See [gaps.md](../gaps.md).
- **`shared_import.Skipped` is hard-coded to 0** — the service
  (`SharedImportSummary`) doesn't surface a skipped count today.
  The summary card claims the field is plumbed but the data is
  always 0. Either remove the field or extend the service.
- **`shared_import.SourcesImported` also hard-coded to 0** —
  comment in `imports_handlers.go` flags this as "service does not
  surface today". Source records count never appears on shared-import
  summaries.
- **`backup_import` schema lines always render** when either schema
  field is > 0, even if migration is a no-op — copy clarifies
  `(no migration)` but the user still sees the schema equality.
  Verify the equality comparison uses the right `buildinfo.SchemaVersion`
  reference.
- **`Memorial import — Added` can be 0** while `Skipped > 0` —
  the line still renders as `0 added, N skipped`. Verify copy is
  acceptable (some users may misread "0 added").

## See also

- [08-export.md](08-export.md) (jobs spawned here)
- `internal/uiids/uiids.go:OverlayJobsProgress` (global overlay)