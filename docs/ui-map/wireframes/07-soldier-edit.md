# 07 — Soldier Edit

- **Route**: `/soldiers/{id}/edit` (GET, GET-with-error)
- **Builder**: `routebuilder.SoldierEdit(id)`
- **Template**: `internal/templates/entry_form.templ:EntryFormFragment` (same as New)
- **Layout**: both

Identical form structure to [06-soldier-new.md](06-soldier-new.md) with
these differences:

## Differences from New

| Region | New | Edit |
| --- | --- | --- |
| H2 | "New Person Record" | "Edit Person Record" (formTitle helper) |
| Scrape Find a Grave | Shown | Hidden (`if !isEdit` guard) |
| Local-draft persistence | `kind="new"` | `kind="edit"` + `data-draft-record-version` |
| Form action | `hx-post={routebuilder.SoldierCreate()}` | `hx-put="/soldiers/{id}"` (bare URL) |
| Source Records | Empty + add row | Pre-populated + add row |
| Image import btn | Disabled | **Enabled** — `hx-post="/soldiers/{id}/images/import?return=edit"` |
| Image count note | Hidden | "Existing images: N" (if isEdit && images > 0) |
| Submit btn | "Create Person Record" | "Save Changes" |
| Display ID input | Editable in some flows | `readonly` (displayIDInputClass) |
| Hidden `existing_*` fields | Present | Present (carry over needs_review / review_reason) |

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Form submit | PUT | `/soldiers/{id}` | `body` | default | Bare URL — routebuilder candidate |
| Image import btn | POST | `/soldiers/{id}/images/import?return=edit` | `this` | `none` | Native dialog guarded |

## Footguns

- **Bare `/soldiers/{id}` PUT** — routebuilder gap. Same handler as
  delete (different verb).
- **Bare `/soldiers/{id}/images/import?return=edit`** — routebuilder
  gap. The `?return=edit` query is a flag for the handler to redirect
  back to the edit page (not the detail page).
- **Stale-draft preview** — only edit gets the preview UI ("Review
  older saved local changes"). If the version bumps server-side,
  user sees the warning. Verify the version-bump detection.
- **`data-record-persistence-preview`** block (only edit) uses
  multiple data attributes (`data-reapply-stale-draft`,
  `data-clear-draft-trigger="stale"`, etc.) — large JS surface, prone
  to drift.

## See also

- [06-soldier-new.md](06-soldier-new.md) (form structure)
- [05-soldier-detail.md](05-soldier-detail.md) (return target after save)