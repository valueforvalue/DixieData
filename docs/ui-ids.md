# UI surface IDs

Use these IDs when requesting changes to a specific part of UI. Canonical source stays in `internal\uiids\uiids.go`.

## Debug visibility

Set `DIXIEDATA_DEBUG_UI_IDS=1` before launch to show surface badges in UI.

- Development example: `set DIXIEDATA_DEBUG_UI_IDS=1`
- Debug build path:
  - `.\scripts\build-debug.ps1`
  - `.\scripts\run-debug.ps1`
- Release builds stay visually clean while variable stays unset.
- Markup still keeps `data-ui-id` attributes for DevTools inspection.

## Responsive audit gate

Every responsive release slice should clear this path before closeout:

1. Build debug app with `.\scripts\build-debug.ps1`.
2. Launch with `.\scripts\run-debug.ps1` or set `DIXIEDATA_DEBUG_UI_IDS=1`.
3. Verify relaxed mode, split-screen mode, narrow-window behavior, and overlay behavior against surface IDs below.
4. Confirm shell overlays count as first-class responsive surfaces: floating menu, feedback modal, print-config modal, image viewer.
5. Keep automated proof green with `go test ./...`, `go build ./...`, and debug build regeneration.

## Naming rules

- Use lowercase dot-separated names.
- Prefix by surface type: `page.*`, `panel.*`, `tab.*`, `overlay.*`.
- Keep names human-friendly so they are easy to say in requests.
- Only assign IDs to durable surfaces, not repeated list items.

## Current catalog

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
| `overlay.floating.menu` | overlay | Floating quick-navigation menu |
| `overlay.feedback.modal` | overlay | Global feedback modal |
| `overlay.print-config.modal` | overlay | Printable export settings modal |
| `overlay.image.viewer` | overlay | Full-screen image viewer |
