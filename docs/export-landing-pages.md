# DixieData export / jobs landing reference

Auto-derived from `internal/appshell/exports_handlers.go`, `app.go`,
`google_handlers.go`, `imports_handlers.go`, `soldiers_handlers.go`,
`reviews_handlers.go`, and `internal/jobs/jobs.go::SilentKinds`. Last
verified: 2026-06-29 against commit `d4db19b` (dev branch).

## Definitions

- **Job landing?** = `enqueueExport` (or `writeExportRedirect` directly) writes
  `X-DixieData-Redirect: /jobs/{id}`. The dispatcher navigates the user to
  `/jobs/{id}` via `window.location.assign`.
- **Summary cards** = the per-kind stats surface on `/jobs/{id}` (Person
  records, Images, Source records, etc). Only `enqueueExportWithResult`
  callers AND the four import flows populate these (the worker returns a
  `jobs.JobResult{Records, Images, Sources, …}` which the layout renders).
- **Silent popup** = the floating `[data-jobs-progress-region]` overlay does
  **not** appear. Currently `static_archive` is the only kind in
  `jobs.SilentKinds`. Every other job surfaces the popup while running and
  self-polls every 2s until terminal.

## Inventory

| Category        | Kind                   | Endpoint                                  | Job landing? | Summary cards        | Silent popup |
|-----------------|------------------------|-------------------------------------------|--------------|----------------------|--------------|
| Export          | `json_export`          | `POST /export/json`                       | YES          | YES (R/I/S)          | NO           |
| Export          | `excel_export`         | `POST /export/csv`                        | YES          | YES (R/I/S)          | NO           |
| Export          | `icalendar_export`     | `POST /export/ical`                       | YES          | YES (R/I/S)          | NO           |
| Export          | `static_archive`       | `POST /export/static-archive?async=1`     | NO (303+L)   | n/a                  | **YES**      |
| Export          | `database_pdf`         | `POST /export/database-pdf?async=1`       | YES          | YES (R/I/S)          | NO           |
| Export          | `backup_archive`       | `POST /export/backup`                     | YES          | YES (R/I/S)          | NO           |
| Export          | `shared_archive`       | `POST /export/shared-archive`             | YES          | YES (R/I/S)          | NO           |
| Export          | `bug_report`           | `POST /export/bug-report`                 | YES          | NO                   | NO           |
| Export          | `insights_pdf`         | `POST /export/insights-pdf`               | YES          | NO                   | NO           |
| Export          | `feedback_log`         | `POST /export/feedback-log`               | YES          | NO                   | NO           |
| Soldier-scoped  | `soldier_pdf`          | `POST /soldiers/{id}/pdf`                 | YES          | NO                   | NO           |
| Soldier-scoped  | `soldier_pdf_no_imgs`  | `POST /soldiers/{id}/pdf-no-images`       | YES          | NO                   | NO           |
| Soldier-scoped  | `soldier_jpg`          | `POST /soldiers/{id}/jpg`                 | YES          | NO                   | NO           |
| Soldier-scoped  | `monthly_pdf`          | `POST /soldiers/{id}/monthly-pdf`         | YES          | NO                   | NO           |
| Soldier CRUD    | soldier create         | `POST /soldiers`                          | n/a → `/soldiers/{id}` | n/a       | n/a          |
| Soldier CRUD    | soldier update         | `PUT /soldiers/{id}`                      | n/a → `/soldiers/{id}` | n/a       | n/a          |
| Soldier CRUD    | soldier delete         | `DELETE /soldiers/{id}`                   | n/a → `/soldiers`     | n/a       | n/a          |
| Import          | backup                 | `POST /import/backup`                     | YES          | YES (added + I + S)  | NO           |
| Import          | shared_archive         | `POST /import/shared-archive`             | YES          | YES (added/merged/skipped + conflicts) | NO |
| Import          | memorial_json          | `POST /import/memorial-json`              | YES          | YES                  | NO           |
| Import          | findagrave_csv         | `POST /import/findagrave-csv`             | YES          | YES                  | NO           |
| Google          | backup                 | `POST /integrations/google/backup`        | YES          | NO                   | NO           |
| Google          | sheets_export          | `POST /integrations/google/sheets/export` | YES          | NO                   | NO           |
| Other           | orphaned_images_cleanup | `POST /jobs/cleanup-image-orphans`       | YES          | NO                   | NO           |
| Other           | bulk_review            | `POST /review-queue/bulk`                | YES          | NO                   | NO           |
| Other           | duplicate_audit        | `POST /jobs/run-duplicate-audit`         | YES          | NO                   | NO           |

R = Records, I = Images, S = Sources, "added/merged/skipped" = import delta.

## Known gaps (work-in-progress)

- **`web-mode` (dixiedata-web.exe) export buttons** silently fail because
  `cmd/dixiedata-web/` does not call `SetSaveFileDialogOverride`. Wails
  `SaveFileDialog` returns `errWailsFrontendUnavailable` and the handler
  short-circuits to `respondDuplicateInFlight`, redirecting the user back to
  `/share` instead of `/jobs/{id}`. The audit smoke harness accepts `/share`
  as a passing result, which masks the failure. Affects every entry in the
  Export section except `static_archive` (which uses a plain `<form method="post">`
  and the browser handles the dialog semantics natively).

- **`handleExportJSON` (and `handleExportCSV`, `handleExportICalendar`,
  `handleExportBackup`, `handleExportSharedArchive`, `handleExportInsightsPDF`,
  `handleExportFeedbackLog`, all soldier-scoped `enqueueExport` callers, and
  Google backup + sheets export) conflate SaveFileDialog failure with
  duplicate-in-flight dedup** — both paths call `respondDuplicateInFlight`,
  even when the first POST never reached `enterInFlight`. See
  `internal/appshell/exports_handlers.go:302` for the JSON case.

- **`audit/smoke.mjs` `share-${btn.path}-navigates-to-jobs`** treats `/share`
  as a successful navigation (line 286). This was a legacy choice to cover
  the pre-Option-C behavior where some buttons correctly returned to the
  share page; the test now passes for the wrong reason in web mode.
