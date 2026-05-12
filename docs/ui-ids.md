# UI surface IDs

Use these IDs when requesting changes to a specific part of the UI. The canonical source of truth lives in `internal\uiids\uiids.go`.

## Debug visibility

Set `DIXIEDATA_DEBUG_UI_IDS=1` before launching the app to show surface badges in the UI.

- Development example: `set DIXIEDATA_DEBUG_UI_IDS=1`
- Release builds stay visually clean as long as that variable is not set.
- The app still keeps `data-ui-id` attributes in the markup so surfaces remain inspectable in DevTools.

## Naming rules

- Use lowercase dot-separated names.
- Prefix by surface type: `page.*`, `panel.*`, `tab.*`, `overlay.*`.
- Keep names human-friendly so they are easy to say in requests.
- Only assign IDs to durable surfaces, not repeated list items.

## Current catalog

| ID | Type | Surface |
| --- | --- | --- |
| `page.calendar` | page | Calendar landing page |
| `panel.calendar.quote` | panel | Quote of the Day panel |
| `panel.calendar.grid` | panel | Calendar month grid |
| `panel.calendar.details` | panel | Calendar day detail panel |
| `page.soldiers.list` | page | Soldier list and search page |
| `tab.soldiers.search.basic` | tab | Quick Search tab trigger |
| `panel.soldiers.search.basic` | panel | Quick Search tab panel |
| `tab.soldiers.search.advanced` | tab | Advanced Search tab trigger |
| `panel.soldiers.search.advanced` | panel | Advanced Search tab panel |
| `panel.soldiers.results` | panel | Soldier search results area |
| `page.soldier.detail` | page | Soldier detail page |
| `panel.soldier.detail.summary` | panel | Summary and action card |
| `panel.soldier.detail.records` | panel | Records section |
| `panel.soldier.detail.images` | panel | Images section |
| `page.soldier.new` | page | New soldier record form |
| `page.soldier.edit` | page | Edit soldier record form |
| `panel.soldier.form.scratchpad` | panel | Scratch pad launcher section in the soldier form |
| `panel.soldier.form.records` | panel | Record entry editor |
| `panel.soldier.form.images` | panel | Image upload section |
| `page.export` | page | Export page |
| `panel.export.actions` | panel | Export/import actions panel |
| `panel.export.google` | panel | Google integration panel |
| `page.settings` | page | Settings page |
| `panel.settings.initialize` | panel | Initialize Data panel |
| `overlay.image.viewer` | overlay | Full-screen image viewer |
| `overlay.soldier.scratchpad` | overlay | Floating soldier scratch pad window |
