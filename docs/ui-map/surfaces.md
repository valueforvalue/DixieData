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
| `page.research-collections.detail` | page | Research collection detail |
| `page.research-log` | page | Research log page |
| `page.research-pack` | page | Research pack page |
| `page.service-timeline` | page | Service timeline page |
| `page.unit-camaraderie` | page | Unit camaraderie page |
| `page.merge-review-ledger` | page | Merge review ledger page |
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