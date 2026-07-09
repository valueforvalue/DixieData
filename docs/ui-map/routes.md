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

## Events (v60 — issue #320, #342)

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/events` | `routebuilder.EventList()` | `event_list.templ:EventList` | Browse all events. v1 renders every event on one page; no sort or filter UI yet |
| `/events/new` | `routebuilder.EventNew()` | `event_form.templ:EventForm` (isEdit=false) | Create form. Hidden `entry_type=event` field; server allocates next `EVT-NNNNN` DisplayID on first save |
| `/events/{id:[0-9]+}` | — (handler direct) | `event_detail.templ:EventDetail` | Detail page. Same handler dispatches GET/POST/PUT/DELETE; POST/PUT use the Wails `X-HTTP-Method-Override` quirk per AGENTS.md |
| `/events/{id:[0-9]+}/edit` | `routebuilder.EventEdit(id)` | `event_form.templ:EventForm` (isEdit=true) | Edit form. Linked Persons + Tags sections render OUTSIDE the main `<form>` to avoid HTML-invalid nested forms (issue #361) |
| `/events/{id:[0-9]+}/pdf` | `routebuilder.EventPDF(id)` | — (PDF) | Per-event PDF; native `SaveFileDialog` guarded per [dialog-guard.md](../agents/dialog-guard.md) |
| `/events/{id:[0-9]+}/research-log` | `routebuilder.EventResearchLog(id)` | (HTMX fragment) | Per-event research log; dispatched through `a.soldiers.ResearchLog` because `research_tasks` is FK-linked to `soldiers(id)` and Events are rows in the same table |
| `/events/{id:[0-9]+}/research-log/tasks` POST | `routebuilder.EventResearchLogTasks(id)` | — | Create task |
| `/events/{id:[0-9]+}/research-log/tasks/{entryId}/resolve` POST | `routebuilder.EventResearchLogResolve(id, taskID)` | — | Close task |
| `/events/{id:[0-9]+}/sources` | `routebuilder.EventSources(id)` | `event_panels.templ:EventSourcesListFragment` | Lazy-load + post-action fragment target |
| `/events/{id:[0-9]+}/sources/attach` POST | — (handler direct) | — | Orphan — no UI caller as of #360; the detail page now points users to `/events/{id}/edit`. Route stays reachable for `.ddshare` replay / bulk-import |
| `/events/{id:[0-9]+}/sources/{sourceId}/detach` POST | — (handler direct) | — | Detach source |
| `/events/{id:[0-9]+}/sources/{sourceId}/position` PATCH | `routebuilder.EventSourcePosition(id, sourceID)` | — | Reorder; response is the re-rendered list fragment targeting `#data-event-sources-list` (issue #368 slice 2) |
| `/events/{id:[0-9]+}/tags` | `routebuilder.EventTagAttach(id)` | `event_panels.templ:EventTagsListFragment` | GET returns the fragment; POST attaches via `TagService.UpsertByName` (case-insensitive, dedup) |
| `/events/{id:[0-9]+}/tags/{tagId}/detach` POST | `routebuilder.EventTagDetach(id, tagID)` | — | Detach tag; response swaps into `#data-event-tags-list` |
| `/events/{id:[0-9]+}/links` POST | `routebuilder.EventLinksAttach(id)` | — | Attach linked Person Record by `display_id`. Handler resolves via `LookupPersonIDByDisplayID` then calls `AttachEventToPerson` (issue #361 slice 2) |
| `/events/{id:[0-9]+}/links/{personId}/detach` POST | `routebuilder.EventLinksDetach(id, personID)` | — | Detach link; `X-DixieData-Redirect` lands back on `/events/{id}/edit` |
| `/events/{id:[0-9]+}/images` | `routebuilder.EventImages(id)` | `event_panels.templ:EventImagesListFragment` | Lazy-load + post-action fragment target |
| `/events/{id:[0-9]+}/images/import` POST | `routebuilder.EventImagesImport(id)` | — | Native file picker; multi-image import; guarded per [dialog-guard.md](../agents/dialog-guard.md) |
| `/events/{id:[0-9]+}/images/delete` POST | `routebuilder.EventImagesDelete(id)` | — | Bulk delete; re-renders the fragment in place (no redirect) |
| `/soldiers/{id:[0-9]+}/events` | `routebuilder.PersonEventsTab(id)` | `person_events_tab.templ:PersonEventsTab` | Events tab on Person Record detail. Registered as a literal `/soldiers/{id}/events` sub-path so the prefix wins over the generic `/soldiers/*` catch-all |
| `/soldiers/{id:[0-9]+}/events/{eventId}/attach` POST | `routebuilder.PersonEventAttach(id, eventID)` | — | Attach event to person |
| `/soldiers/{id:[0-9]+}/events/{eventId}/detach` POST | `routebuilder.PersonEventDetach(id, eventID)` | — | Detach event from person |
| `/soldiers/{id:[0-9]+}/events/quick-add` POST | `routebuilder.PersonEventQuickAdd(id)` | — | Quick-add an event from the person detail page |
| `/soldiers/{id:[0-9]+}/events/attach-by-display-id` POST | `routebuilder.PersonEventAttachByDisplayID(id)` | — | Attach by Display ID |

## Tags (issue #256, #342)

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/tags` | `routebuilder.TagsPage()` | `tags_management.templ` | Top-nav Tags link; list page with rename / merge / delete actions |
| `/tags/{id:[0-9]+}` | `routebuilder.TagDetail(id)` | `tag_detail.templ` | Single tag detail page; membership list with Remove buttons |
| `/tags/{id:[0-9]+}/rename` POST | `routebuilder.TagRename(id)` | — | Rename tag |
| `/tags/{id:[0-9]+}/merge` POST | `routebuilder.TagMerge(id)` | — | Merge into another tag |
| `/tags/{id:[0-9]+}` DELETE | `routebuilder.TagDelete(id)` | — | Delete tag |
| `/soldiers/{id:[0-9]+}/tags` GET | — (handler direct) | (HTMX autocomplete) | Soldier-side tag autocomplete fragment |
| `/soldiers/{id:[0-9]+}/tags` POST | `routebuilder.SoldierTagAttach(id)` | — | Attach tag to soldier (free-text via `tag_name` or ID) |
| `/soldiers/{id:[0-9]+}/tags/{tagId}` POST | `routebuilder.SoldierTagDetach(id, tagId)` | — | Detach (DELETE-as-POST) tag from soldier |
| `/browse/bulk-tag` POST | `routebuilder.BrowseBulkTag()` | — | Bulk-tag selected records from the Browse page |

## Scratchpad (issue #283)

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/scratchpad/open` POST | — (handler direct) | — | Open scratchpad with the supplied `display_id` (hidden form field). Called from the floating-dock Scratch Pad button + the per-Soldier / per-Event Open Scratch Pad buttons. Posts via `data-dixie-submit` |

## Research

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/research` | `routebuilder.ResearchPicker()` | `research_picker.templ` | Person picker landing. Renders Continue shortcut when `dd_person_ctx` cookie is set. With `?partial=1` returns just the results panel fragment (issue #378 slice 2). Reads `?next=` (allowlisted) and echoes it into picker forms so the post-redirect lands on the correct sub-page (issue #378 slice 2). |
| `/research?q=...&partial=1` | `routebuilder.ResearchSearch()` | `research_picker.templ` (ResearchPickerSearchResults) | htmx live-search fragment target. Same handler branches on `partial=1`; the routebuilder just returns the canonical URL (issue #378 slice 2 — option B2, collapses to a branch because chi can't route by query). |
| `/research/recent` | `routebuilder.ResearchRecent()` | `research_picker.templ` (ResearchPickerRecent) | Recents-list fragment endpoint. Reads `?ids=...&next=...`; returns the populated `<ul data-research-recent-list>` fragment (or the empty-state paragraph when 0 valid ids) so `app.js#hydrateResearchPickerRecents` can swap it in. Issue #378 slice 3. |
| `/research/select` POST | `routebuilder.ResearchSelect()` | — | Records chosen Person in `dd_person_ctx` cookie + redirects via `X-DixieData-Redirect` to the requested sub-page (issue #378 slice 1). Honors `?next=` forwarded from the picker (slice 2). |
| `/research/clear` POST | `routebuilder.ResearchClear()` | — | Clears the cookie (`MaxAge=-1`) + redirects to `/research` (issue #378 slice 1) |
| `/research-collections` | — (handler direct) | `research_collections.templ` | Hub |
| `/research-collections` POST | `routebuilder.ResearchCollectionsCreate()` | — | Create collection |
| `/research-collections/{id}` | — (handler direct) | `research_collections.templ` | Detail |
| `/research-collections/{id}/items` POST | `routebuilder.ResearchCollectionAdd(id)` | — | Add item |
| `/research-log/{id}` | — (handler direct) | `research_log.templ` | Per-person log |
| `/research-pack` | — (handler direct) | `research_pack.templ` | County/state pack |

## Export / share / jobs

| Path | Builder | Templ | Notes |
| --- | --- | --- | --- |
| `/share` | `routebuilder.SharePage()` | `share.templ` | Share landing. Issue #265 reorg: Quick Actions + Recent + Support & Diagnostics above the fold, Export/Import/Sync cards below |
| `/share/exports` | `routebuilder.ShareExports()` | `share_exports.templ` | Share Exports subpage (Export & Backup surface + Build Share Archive button). Top-nav Share foldout link (issue #264) |
| `/share/imports` | `routebuilder.ShareImports()` | `share_imports.templ` | Share Imports subpage (Import & Restore surface). Top-nav Share foldout link |
| `/share/sync` | `routebuilder.ShareSync()` | `share_sync.templ` | Share Sync subpage (Google Integration + Calendar Preferences). Top-nav Share foldout link |
| `/share/queue` | `routebuilder.ShareQueuePage()` | `share_queue.templ` | Share Queue management page. Top-nav Share foldout link |
| `/share/queue/presets` | `routebuilder.ShareQueuePresets()` | (HTMX) | Saved Queues presets list fragment |
| `/share/queue/presets/{id}/apply` POST | `routebuilder.ShareQueuePresetApply(id)` | — | Apply preset |
| `/share/queue/presets/{id}` DELETE | `routebuilder.ShareQueuePresetDelete(id)` | — | Delete preset |
| `/share/queue/bulk` POST | `routebuilder.ExportSharedArchiveSubset()` | — | Bulk export of queued Person Records |
| `/share/print-records-fragment` | `routebuilder.SharePrintRecordsFragment()` | (HTMX) | Lazy-load print-records fragment for the queue management page (issue #234) |
| `/share/export-options` PATCH | — (handler direct) | — | PATCH share export options |
| `/share/build-archive` POST | — (handler direct) | — | Build share archive from queued records |
| `/export/backup` | `routebuilder.ExportBackup()` | — | Backup archive; native `SaveFileDialog` guarded per [dialog-guard.md](../agents/dialog-guard.md) |
| `/export/shared-archive` POST | `routebuilder.ExportSharedArchive()` | — | Build shared archive |
| `/export/database.pdf.async` | `routebuilder.ExportDatabasePDFAsync()` | — | Async PDF (job) |
| `/export/bug-report` POST | — (handler direct) | — | Export bug report |
| `/export/feedback-log` POST | — (handler direct) | — | Export feedback log |
| `/import/backup` POST | `routebuilder.ImportBackup()` | — | Import backup; native `OpenFileDialog` guarded per [dialog-guard.md](../agents/dialog-guard.md) |
| `/import/shared-archive` POST | `routebuilder.ImportSharedArchive()` | — | Import shared archive |
| `/import/memorial-json` POST | `routebuilder.ImportMemorialJSON()` | — | Import memorial JSON |
| `/jobs/active` | `routebuilder.ActiveJobs()` | (polled) | Active job list; polled by the global `overlay.jobs.progress` every 3s |
| `/jobs/{id}/status` | `routebuilder.JobStatus(jobID)` | `jobs.templ` | Job status panel |
| `/jobs/{id}/status?slot=1` | `routebuilder.JobStatusSlot(jobID)` | `job_slot_fragment.templ` | Slot fragment |
| `/jobs/{id}/log` | `routebuilder.JobLog(jobID)` | — (binary) | Stream job log file (memorial imports, etc.); resolves path + verifies containment inside `os.TempDir()` |
| `/feedback/submit` | `routebuilder.FeedbackSubmit()` | — (modal) | Feedback POST from the floating-dock Feedback btn |

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