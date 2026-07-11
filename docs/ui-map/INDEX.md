# Screen × Component Matrix

One row per routable screen. Columns are panels / tabs / overlays that
live on the screen. Cells hold the DOM ID and link to the screen's
wireframe.

Legend: `P` = panel · `T` = tab · `O` = overlay · `—` = none.

## Global (every screen)

The `Layout` shell renders these on every page; they are not columns
below but worth knowing:

- `header.top-shell` — brand + nav pill links (top)
- `footer` — build identity, optional debug button
- `[P] panel.floating.dock` — persistent bottom dock (issue #283)
- `[P] panel.floating.scratchpad-status` — live region in the dock
- `[P] panel.share-queue.pill` — persistent Share Queue status pill
  (hidden when queue empty, issue #182)
- `[O] overlay.floating.menu` — toggled from floating dock Menu btn
- `[O] overlay.feedback.modal` — toggled from floating dock
- `[O] overlay.jobs.progress` — fixed-position, polled 3s
- `[O] overlay.image.viewer` — opened from image cards
- `[O] overlay.print-config.modal` — opened from export popouts
- `[O] overlay.google-calendar-prefs.modal` — Google settings
- `[N] layout.share.menu` + `layout.share.menu.trigger` — top-nav
  Share foldout (issue #264)
- `[N] layout.research.menu` + `layout.research.menu.trigger` —
  top-nav Research & Review foldout (issue #378 slice 2)
- `[N] layout.tags.link` — top-nav Tags link (issue #256, #342)
- `.toast-region` — top-right transient notifications

## Screens

| # | Screen | Panels | Tabs | Overlays | Wireframe |
| --- | --- | --- | --- | --- | --- |
| 00 | Global Layout Shell | `layout.research.menu{,.trigger}`, `layout.share.menu{,.trigger}`, `layout.tags.link`, `data-layout-research-review-count`, `data-floating-scratchpad-status`, `data-floating-nav-panel`, `data-share-queue-pill` | — | `overlay.floating.menu` (toggled from floating-dock Menu btn) | [wireframes/layout.md](wireframes/layout.md) |
| 01 | Calendar | `panel.calendar.quote`, `panel.calendar.grid`, `panel.calendar.details` | — | `overlay.print-config.modal` (via popout) | [01-calendar.md](wireframes/01-calendar.md) |
| 02 | Calendar Day | (inline w/ Calendar) | — | — | (in `01-calendar.md`) |
| 03 | Soldiers List (Search/Quick View) | `panel.soldiers.search.basic`, `panel.soldiers.search.advanced`, `panel.soldiers.results` | `tab.soldiers.search.basic`, `tab.soldiers.search.advanced` | — | [wireframes/03-soldiers-list.md](wireframes/03-soldiers-list.md) |
| 04 | Browse | `panel.browse.results` | — | — | [wireframes/04-browse.md](wireframes/04-browse.md) |
| 05 | Soldier Detail | `panel.soldier.detail.summary`, `panel.soldier.detail.records`, `panel.soldier.detail.images` | — | `overlay.image.viewer` | [wireframes/05-soldier-detail.md](wireframes/05-soldier-detail.md) |
| 06 | Soldier New | `panel.soldier.form.scratchpad`, `panel.soldier.form.records`, `panel.soldier.form.images` | — | — | [wireframes/06-soldier-new.md](wireframes/06-soldier-new.md) |
| 07 | Soldier Edit | (same as New) | — | — | [wireframes/07-soldier-edit.md](wireframes/07-soldier-edit.md) |
| 08 | Share (landing) | `panel.share.quick-actions`, `panel.share.recent`, `panel.share.support` | — | — | [wireframes/08-export.md](wireframes/08-export.md) |
| 08a | Share / Exports | `panel.share.exports` | — | `overlay.print-config.modal` | [wireframes/08a-exports.md](wireframes/08a-exports.md) |
| 08b | Share / Imports | `panel.share.imports` | — | — | [wireframes/08b-imports.md](wireframes/08b-imports.md) |
| 08c | Share / Sync | — | — | `overlay.google-calendar-prefs.modal` | [wireframes/08c-sync.md](wireframes/08c-sync.md) |
| 09 | Insights | `panel.insights.overview`, `panel.insights.cemeteries`, `panel.insights.homes`, `panel.insights.pensions`, `panel.insights.units`, `panel.insights.chronology`, `panel.insights.duplicate-audit` | — | `overlay.print-config.modal` | [wireframes/09-insights.md](wireframes/09-insights.md) |
| 10 | Insights Drilldown | (panel-only) | — | — | [wireframes/10-insights-drilldown.md](wireframes/10-insights-drilldown.md) |
| 11 | Review Queue | `panel.review-queue.list` | — | — | [wireframes/11-review-queue.md](wireframes/11-review-queue.md) |
| 12 | Review Queue Compare | `panel.review-queue.compare` | — | — | [wireframes/12-review-queue-compare.md](wireframes/12-review-queue-compare.md) |
| 12a | Research Picker | `page.research.picker`, `panel.research.picker.continue`, `panel.research.picker.search`, `panel.research.picker.results`, `panel.research.picker.pack-sub-screen`, `panel.research.picker.recent` | — | — | [wireframes/research-picker.md](wireframes/research-picker.md) |
| 13 | Research Collections Hub | — | — | — | [wireframes/13-research-collections-hub.md](wireframes/13-research-collections-hub.md) |
| 14 | Research Collection Detail | — | — | — | [wireframes/14-research-collection-detail.md](wireframes/14-research-collection-detail.md) |
| 15 | Research Log | — | — | — | [wireframes/15-research-log.md](wireframes/15-research-log.md) |
| 16 | Research Pack | — | — | — | [DEPRECATED — moved to historical/wireframes-2026-07-10](../../historical/ui-map-wireframes/16-research-pack.md) |
| 17 | Service Timeline | — | — | — | [wireframes/17-service-timeline.md](wireframes/17-service-timeline.md) |
| 18 | Unit Camaraderie | — | — | — | [DEPRECATED — moved to historical/wireframes-2026-07-10](../../historical/ui-map-wireframes/18-unit-camaraderie.md) |
| 19 | Merge Review Ledger | — | — | — | [DEPRECATED — moved to historical/wireframes-2026-07-10](../../historical/ui-map-wireframes/19-merge-review-ledger.md) |
| 19a | Review Queue Resolved tab | `panel.review-queue.resolved` | — | — | [wireframes/19a-review-queue-resolved.md](wireframes/19a-review-queue-resolved.md) |
| 20 | Jobs | `panel.job.status` | — | — | [wireframes/20-jobs.md](wireframes/20-jobs.md) |
| 21 | Settings | `panel.settings.layout`, `panel.settings.initialize`, `panel.settings.updates`, `panel.settings.debug` | — | — | [wireframes/21-settings.md](wireframes/21-settings.md) |
| 22 | Initial Setup | — | — | — | [wireframes/22-initial-setup.md](wireframes/22-initial-setup.md) |
| 23 | Recovery | — | — | — | [wireframes/23-recovery.md](wireframes/23-recovery.md) |
| 24 | Events List | — | — | — | [wireframes/24-events-list.md](wireframes/24-events-list.md) |
| 25 | Event Detail | `panel.event.detail.linked-persons`, `panel.event.detail.sources`, `panel.event.detail.tags`, `panel.event.detail.images` | — | `overlay.print-config.modal` (via EventPDFExport popout) | [wireframes/25-event-detail.md](wireframes/25-event-detail.md) |
| 26 | Event New | — | — | — | [wireframes/26-event-new.md](wireframes/26-event-new.md) |
| 27 | Event Edit | `panel.event.form.sources`, `panel.event.form.linked-persons`, `panel.event.form.tags` | — | — | [wireframes/27-event-edit.md](wireframes/27-event-edit.md) |
| 28 | Event PDF | — | — | `overlay.print-config.modal` (via popout) | [wireframes/28-event-pdf.md](wireframes/28-event-pdf.md) |

> All 30 wireframes drafted (v60 Event Records added in #342, top-nav
> Share foldout + floating-dock Menu + Tags link folded into the
> Global section; Research Picker added per issue #381 slice 1;
> Global Layout Shell wireframe added per issue #381 slice 2).
> Pilot validated — same format throughout.

## Cross-references

- Atomic components: [components.md](components.md)
- Surface IDs (DOM IDs): [surfaces.md](surfaces.md)
- Routes & builders: [routes.md](routes.md)
- States: [states.md](states.md)
- Gaps: [gaps.md](gaps.md)