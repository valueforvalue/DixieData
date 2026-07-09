# Surfaces (canonical DOM IDs)

Surface IDs are the canonical identifiers for UI regions (pages, panels,
tabs, overlays). They are typed Go constants in `internal/uiids/uiids.go`
and registered in the `Registry` slice for lookup. Templates and HTMX
attributes reference these constants instead of string literals so renames
stay in sync.

> Moved from `docs/ui-ids.md` as part of the UI-map consolidation.

Use these IDs when requesting changes to a specific part of the UI.

## Naming rules

- Lowercase dot-separated names (`page.soldier.detail`).
- Prefix by surface type: `page.*`, `panel.*`, `tab.*`, `overlay.*`.
- Names should be human-friendly so they are easy to say in requests.
- Only assign IDs to durable surfaces, not repeated list items.

## Adding a new surface

1. Add a constant to `internal/uiids/uiids.go`.
2. Add a `Surface{ID, Kind, Description}` entry to `Registry`.
3. Reference via `uiids.YourNewSurface` in templates and HTMX attributes.
4. Add a row to the catalog below.

## Catalog

The catalog is the human-readable mirror of the registry. Tests do not
parse this file; tests assert against `Registry` directly. Keep this
table in sync when adding or renaming surfaces.

| ID | Type | Surface |
| --- | --- | --- |
| `page.calendar` | page | Calendar landing page |
| `page.setup` | page | First launch setup page |
| `panel.calendar.quote` | panel | Quote of the Day panel |
| `panel.calendar.grid` | panel | Calendar month grid |
| `panel.calendar.details` | panel | Calendar day detail panel |
| `page.soldiers.list` | page | Soldier list and search page |
| `page.browse` | page | Dedicated local archive browse page |
| `tab.soldiers.search.basic` | tab | Quick Search tab trigger |
| `panel.soldiers.search.basic` | panel | Quick Search tab panel |
| `tab.soldiers.search.advanced` | tab | Advanced Search tab trigger |
| `panel.soldiers.search.advanced` | panel | Advanced Search tab panel |
| `panel.soldiers.results` | panel | Soldier search results area |
| `panel.browse.results` | panel | Browse results table |
| `page.soldier.detail` | page | Person Record detail page |
| `panel.soldier.detail.summary` | panel | Summary and action card |
| `panel.soldier.detail.records` | panel | Source Records section |
| `panel.soldier.detail.images` | panel | Images section |
| `page.soldier.new` | page | New Person Record form |
| `page.soldier.edit` | page | Edit Person Record form |
| `panel.soldier.form.scratchpad` | panel | Scratch pad launcher section in form |
| `panel.soldier.form.records` | panel | Source Record entry editor |
| `panel.soldier.form.images` | panel | Image upload section |
| `page.export` | page | Share / export page |
| `panel.export.actions` | panel | Export/import actions panel |
| `panel.export.google` | panel | Google integration panel |
| `panel.job.status` | panel | Background-job status page panel |
| `page.insights` | page | Archive insights dashboard |
| `panel.insights.overview` | panel | Overview card |
| `panel.insights.cemeteries` | panel | Top cemeteries card |
| `panel.insights.homes` | panel | Confederate Home analytics card |
| `panel.insights.pensions` | panel | Pension distribution card |
| `panel.insights.units` | panel | Unit representation card |
| `panel.insights.chronology` | panel | Chronology card |
| `panel.insights.duplicate-audit` | panel | Duplicate audit card |
| `page.review-queue` | page | Review queue page |
| `panel.review-queue.list` | panel | Review queue list |
| `page.review-queue.compare` | page | Review compare page |
| `panel.review-queue.compare` | panel | Side-by-side compare panel |
| `page.research-collections.hub` | page | Research collections hub |
| `panel.event.detail.sources` | panel | Source Records list on the event detail page; wraps the `<div id=data-event-sources-list>` swap target rendered by `EventSourcesListFragment` (issue #342) |
| `panel.event.detail.tags` | panel | Tags chips on the event detail page; wraps the `<div id=data-event-tags-list>` swap target rendered by `EventTagsListFragment` (issue #342) |
| `panel.event.detail.linked-persons` | panel | Linked Person Records list on the event detail page; the `<ul>` of person records attached to this event (issue #342) |
| `panel.event.form.sources` | panel | Source Records editor section on the event form (`/events/new` + `/events/{id}/edit`); wraps the `RecordInputRow` list rendered inside the main `<form>` (issue #342) |
| `panel.event.form.linked-persons` | panel | Linked Persons section on the event edit page (`/events/{id}/edit`); rendered OUTSIDE the main `<form>` to avoid HTML-invalid nested forms; Add Link + Unlink actions target this panel (issue #342, #361) |
| `panel.event.form.tags` | panel | Tags section on the event edit page (`/events/{id}/edit`); rendered OUTSIDE the main `<form>` for the same nested-form reason; Add Tag form posts to `/events/{id}/tags` and swaps into `#data-event-tags-list` (issue #342, #361) |
| `panel.floating.dock` | panel | Persistent bottom dock rendered once in `layout.templ` (issue #283 / #289 / #313); hosts Scratch Pad + Feedback + Menu buttons. z-40 |
| `panel.floating.nav-panel` | panel | Slide-out nav panel toggled by the Menu button via `data-floating-nav-toggle` (issue #283); duplicates top-nav links + renders the layout-mode picker; positioned bottom-right, z-50 |
| `panel.floating.scratchpad-status` | panel | Live region in the floating dock (`data-floating-scratchpad-status`, `aria-live=polite`) for scratchpad open / save status announcements (issue #283) |
| `panel.share-queue.pill` | panel | Persistent Share Queue status pill (issue #182); fixed bottom-center, hidden when the queue is empty; wraps `data-share-queue-pill` + `data-share-queue-pill-label` + `data-share-queue-pill-count` |
| `layout.tags.link` | nav | Top-nav Tags link (`/tags`); literal href in `layout.templ` between the Share foldout and Settings. Surface ID is registered so a future Tags foldout (mirroring Share / Research) has a stable anchor (issue #256, #342) |
| `panel.research-collections.hub` | panel | Named collections list and create-collection section |
| `page.research.picker` | page | Research & Review Person picker landing (issue #378 slice 1 + slice 2 + slice 3); search + recents + Continue shortcut. With `?partial=1` returns the `#panel.research.picker.results` fragment only (slice 2). |
| `layout.research.menu` | nav | Top-nav Research & Review foldout panel (issue #378 slice 2). Lists 5 soldier-scoped sub-page links (camaraderie / timeline / research-log / conflict-ledger / research-pack) routed through the picker when no `dd_person_ctx` cookie + Research Collections (global) + Change Person… . |
| `layout.research.menu.trigger` | nav | Top-nav Research & Review foldout trigger button (issue #378 slice 2); clicks open `layout.research.menu`. Sits between Insights and the Share foldout per Q2 lock. |
| `panel.research.picker.search` | panel | Search input region on the picker page |
| `panel.research.picker.results` | panel | Live search results region on the picker page |
| `panel.research.picker.recent` | panel | Recent-persons region on the picker page; populated via `localStorage[dixiedata.research.recents]` + `/research/recent?ids=...&next=...` fragment swap (issue #378 slice 3). Hydrated by `app.js#hydrateResearchPickerRecents` on DOMContentLoaded. |
| `panel.research.picker.pack-sub-screen` | panel | Research-pack picker sub-screen (issue #378 slice 3); renders a `<select name="geography">` with state + county options when `?next=research-pack` is set so the picker form submits `?geography=state|county` to the soldier-scoped sub-page. |
| `panel.research.picker.continue` | panel | Continue shortcut region on the picker page (renders only when `dd_person_ctx` cookie is set) |
| `page.research-collections.detail` | page | Research collection detail |
| `panel.research-collection.detail` | panel | Items list and add-row section |
| `page.research-log` | page | Research log page |
| `panel.research-log` | panel | Log entries and task creation form |
| `page.research-pack` | page | Research pack page |
| `panel.research-pack` | panel | Pack contents |
| `page.event.list` | page | Event Record browse page on `/events` (issue #396, #342); wraps the main content area (header + list of `EventCard`). Mirrors `page.soldiers.list` |
| `page.event.detail` | page | Event Record detail page on `/events/{id}` (issue #396, #342); wraps the main content area (back button + summary card + linked persons + tags + images + research log). Mirrors `page.soldier.detail` |
| `page.event.new` | page | Event Record create page on `/events/new` (issue #396, #342); wraps the `EventFormFragment` body when `isEdit=false`. Mutually exclusive with `page.event.edit` |
| `page.event.edit` | page | Event Record edit page on `/events/{id}/edit` (issue #396, #342); wraps the `EventFormFragment` body when `isEdit=true`. Same templ as `page.event.new` |
| `page.service-timeline` | page | Service timeline page |
| `panel.soldier.timeline` | panel | Evidence-backed chronology (soldier detail HTMX swap) |
| `page.unit-camaraderie` | page | Unit camaraderie page |
| `panel.soldier.camaraderie` | panel | Unit camaraderie graph (soldier detail HTMX swap) |
| `page.merge-review-ledger` | page | Merge review ledger page |
| `panel.soldier.conflict-ledger` | panel | Local vs Incoming merge ledger (soldier detail HTMX swap) |
| `page.insights.drilldown` | page | Insights drilldown page |
| `page.settings` | page | Settings page |
| `panel.settings.layout` | panel | Responsive layout controls |
| `panel.settings.initialize` | panel | Initialize Data panel |
| `panel.settings.updates` | panel | Software Updates panel |
| `panel.settings.debug` | panel | Debug mode toggle |
| `overlay.floating.menu` | overlay | Floating quick-navigation menu |
| `overlay.feedback.modal` | overlay | Global feedback modal |
| `overlay.print-config.modal` | overlay | Printable export settings modal |
| `overlay.google-calendar-prefs.modal` | overlay | Google managed calendar event preferences modal |
| `overlay.image.viewer` | overlay | Full-screen image viewer |