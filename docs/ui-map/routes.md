# Routes → Screens

Every route registered in `internal/appshell/routes.go`, mapped to the
templ screen that renders it. URL builders live in
`internal/routebuilder/routebuilder.go`.

> Auto-derived. If a route is missing here, it's missing from the
> routebuilder and probably shouldn't be referenced from templates.
> See [gaps.md](gaps.md) for handlers without builders.

## Layout shell (every page)

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/app.css` | — | — | Tailwind output, served as static asset |
| `/app.js` | — | — | Frontend bundle |
| `/htmx.min.js` | — | — | HTMX runtime |
| `/debug.js` | — | — | Debug-mode runtime |
| `/jobs/active` | `routebuilder.ActiveJobs()` | polled by `Layout.jobs-progress-overlay` | 3s poll, filtered by `jobs.SilentKinds` |

## Calendar (route group)

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/calendar` | — (handler direct) | `calendar.templ:Calendar` | Month landing |
| `/calendar/{m}` | — (handler direct) | `calendar.templ:Calendar` | Month selected |
| `/calendar/{m}/report.pdf` | `routebuilder.CalendarReportPDF(month)` | — (PDF) | Posted from popout |
| `/anniversary/{m}/{d}` | `routebuilder.Anniversary(month, day)` | `calendar_day.templ` | HTMX swap target `#details-pane` |
| `/anniversary/{m}/{d}?edit={id}` | `routebuilder.AnniversaryEdit(...)` | `calendar_day.templ` | Edit-mode anniversary |
| `/anniversary/{m}/{d}/items/{id}` DELETE | `routebuilder.AnniversaryItemDelete(...)` | — | Item delete |
| `/anniversary/{m}/{d}/items/{id}` PUT | `routebuilder.AnniversaryItemUpdate(...)` | — | Item update |
| `/anniversary/{m}/{d}/items` POST | `routebuilder.AnniversaryItemCreate(...)` | — | Item create |

## Soldiers / Search / Browse

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/soldiers` | — (handler direct) | `soldier_card.templ` | Quick view / search |
| `/soldiers/search` | `routebuilder.SoldierSearch(browse)` | (HTMX) | Search results |
| `/soldiers/search/advanced` | `routebuilder.SoldierSearchAdvanced()` | (HTMX) | Advanced tab |
| `/soldiers/new` | — (handler direct) | `entry_form.templ` | New Person Record |
| `/soldiers/{id}` | — (handler direct) | `soldier_card.templ` | Person Record detail |
| `/soldiers/{id}/edit` | `routebuilder.SoldierEdit(id)` | `entry_form.templ` | Edit |
| `/soldiers/{id}/pdf` | `routebuilder.SoldierPDF(id)` | — (PDF) | Printable |
| `/soldiers/{id}/images/download` | `routebuilder.SoldierImagesDownload(id)` | — (binary) | ZIP download |
| `/soldiers/{id}/images/{imageID}/primary` | `routebuilder.SoldierImagesPrimary(...)` | — | Set primary image |
| `/soldiers/{id}/scrape/findagrave` | `routebuilder.SoldierScrapeFindAGrave()` | (HTMX) | Scrape |
| `/soldiers/{id}/review-flag` | `routebuilder.SoldierReviewFlag(id)` | — | Flag for review |
| `/soldiers/{id}/research-log/tasks` POST | `routebuilder.ResearchLogTasksCreate(soldierID)` | — | Create task |
| `/soldiers/{id}/camaraderie` | `routebuilder.SoldierCamaraderie(id)` | `camaraderie.templ` | HTMX swap |
| `/soldiers/{id}/conflict-ledger` | `routebuilder.SoldierConflictLedger(id)` | `conflict_ledger.templ` | HTMX swap |
| `/soldiers/{id}/timeline` | `routebuilder.SoldierTimeline(id)` | `timeline.templ` | HTMX swap |
| `/browse` | — (handler direct) | `browse.templ` | Local archive browse |
| `/browse/results` | `routebuilder.BrowseResults()` | (HTMX) | Browse results |

## Review queue

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/review-queue` | — (handler direct) | `review_queue.templ` | List |
| `/review-queue/bulk` | `routebuilder.ReviewQueueBulk()` | — | Bulk action |
| `/review-queue/compare` | — (handler direct) | `review_queue.templ` | Compare page |

## Insights

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/insights` | — (handler direct) | `insights.templ` | Dashboard |
| `/insights/drilldown` | — (handler direct) | `insights.templ` | Drilldown |
| `/insights/report.pdf` | `routebuilder.InsightsReportPDF()` | — (PDF) | Posted from insights |

## Research

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/research-collections` | — (handler direct) | `research_collections.templ` | Hub |
| `/research-collections` POST | `routebuilder.ResearchCollectionsCreate()` | — | Create collection |
| `/research-collections/{id}` | — (handler direct) | `research_collections.templ` | Detail |
| `/research-collections/{id}/items` POST | `routebuilder.ResearchCollectionAdd(id)` | — | Add item |
| `/research-log/{id}` | — (handler direct) | `research_log.templ` | Per-person log |
| `/research-pack` | — (handler direct) | `research_pack.templ` | County/state pack |

## Export / share / jobs

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/share` | — (handler direct) | `share.templ` | Share/export page |
| `/export/backup` | `routebuilder.ExportBackup()` | — | Backup archive |
| `/export/database.pdf.async` | `routebuilder.ExportDatabasePDFAsync()` | — | Async PDF (job) |
| `/jobs/active` | `routebuilder.ActiveJobs()` | (polled) | Active job list |
| `/jobs/{id}/status` | `routebuilder.JobStatus(jobID)` | `jobs.templ` | Job status panel |
| `/jobs/{id}/status?slot=1` | `routebuilder.JobStatusSlot(jobID)` | `job_slot_fragment.templ` | Slot fragment |

## Settings / recovery / debug

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/settings` | — (handler direct) | (templ inside) | Settings page |
| `/settings/layout` (state) | — (handler direct) | — | Layout mode toggle handler |
| `/settings/debug` | `routebuilder.SettingsDebugMode()` | — | Toggle debug |
| `/settings/initialize` | `routebuilder.SettingsInitialize()` | — | Initialize data |
| `/settings/update-source` | `routebuilder.SettingsUpdateSource()` | — | Set update channel |
| `/settings/update/check` | `routebuilder.SettingsUpdateCheck()` | — | Check for updates |
| `/settings/update/apply` | `routebuilder.SettingsUpdateApply()` | — | Apply update |
| `/settings/images/orphans/scan` | `routebuilder.SettingsImagesOrphansScan()` | — | Scan orphans |
| `/settings/images/orphans/cleanup` | `routebuilder.SettingsImagesOrphansCleanup()` | — | Cleanup orphans |
| `/settings/quality/scan` | `routebuilder.SettingsQualityScan()` | — | Quality scan |
| `/settings/quality/apply` | `routebuilder.SettingsQualityApply()` | — | Apply quality fixes |
| `/recovery` | — (handler direct) | `recovery.templ` | Recovery page |
| `/debug/console` | `routebuilder.DebugConsole()` | (HTMX swap) | Log console |
| `/feedback/submit` | `routebuilder.FeedbackSubmit()` | — (modal) | Feedback POST |
| `/google/calendar/preferences` (save) | `routebuilder.GoogleCalendarPreferencesSave()` | — | Google prefs modal |

## Initial setup

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/setup` | — (handler direct) | `initial_setup.templ` | First-launch only |

> Note: routes listed as "— (handler direct)" don't have a routebuilder
> constant but are referenced in templates. They are candidates for
> routebuilder coverage. See [gaps.md](gaps.md).