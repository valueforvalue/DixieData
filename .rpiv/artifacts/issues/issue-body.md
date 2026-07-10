## Summary

Today the Source Records attached to a Person Record (soldier / wife / widow) and to an Event Record display in `id` (insertion) order only — there is no way for a researcher to put a key pension application ahead of a stray receipt after the records have been saved. We need a persistent, user-controlled order for both lists.

## Current behavior

### Person Record sources
- Storage: `internal/db/schema.go:85-94` — `records` table has `id` (auto-increment) and `sync_id` only. **No `sort_order` / `position` column.**
- Read path: `internal/records/soldier_service.go:251` and `:299` — `ORDER BY id`.
- Persistence: `replaceRecords()` in `internal/records/soldier_service.go:2085-2113` deletes and reinserts in the order the form submits, which then becomes the new id-order. Looks persistent across saves but is reconstructed each time.

### Event Record sources
- Storage: `internal/db/schema.go:322-331` — `event_sources` table. Same gap: no ordering column. (Intentionally separate from `records` since v61 split them; see issue #340.)
- Read path: `internal/records/event_service.go:75` — `ORDER BY id`.
- Persistence: inline source rows on the Event create/edit forms (`issue #357`, commit `7c8c768`) — `EventService.AttachSourcesToEvent` (`internal/records/event_service.go:203-227`) batches inserts in form-array order.

## Why now

- Both detail pages render the lists in insertion order only (`internal/templates/soldier_card.templ:538-552` `panel.soldier.detail.records`; `internal/templates/event_panels.templ:36-53` `#data-event-sources-list`).
- The print/PDF side reads the same DB order (`templates/common/record_card.typ:362-380`), so the gap is visible in Shared and Backup Archive exports too. Event Record landscape PDF (`templates/event_landscape.typ`) does not render sources today — no change needed there.
- A user reorganizing their evidence after the fact has no path other than detach + reattach in the desired order, which is destructive and lossy if they have already wired Claims to those sources.

## Proposed v1 — hybrid reorder UX

Per-row controls on every list surface:

1. **Up / Down arrow buttons** — pure htmx `PATCH`, one source at a time. Single click moves the source one slot toward that end of the list. Accessible by default, no DnD pitfalls on WebView2.
2. **Numeric position input** — small number field on each row; on blur or Enter, `PATCH` the source to that absolute position. Lets a user jump a source from slot 8 to slot 1 without eight up-arrow clicks.

The hybrid covers both "nudge the pension above the receipt" and "pull this one source out of the middle to the top" without dragging a JS framework in.

## Data model

Add a `sort_order INTEGER NOT NULL DEFAULT 0` column to both tables:

- `records` — `internal/db/schema.go:85`
- `event_sources` — `internal/db/schema.go:322`

Bump `CurrentSchemaVersion` per the schema-touch law. Migration in `docs/migrations/v{N+1}.md` covers both adds + a backfill that assigns `sort_order = id` so existing rows preserve current display order on first read after upgrade.

Read paths change `ORDER BY id` → `ORDER BY sort_order, id` (the secondary `id` keeps the order stable when two rows tie).

Write paths:

- `replaceRecords()` (`soldier_service.go:2085`) and `EventService.AttachSourcesToEvent` (`event_service.go:203`) set `sort_order` from the form-array index on insert.
- New `PATCH /soldiers/{id}/sources/{sourceId}/position` and `PATCH /events/{id}/sources/{sourceId}/position` (or one shared `PATCH /sources/{kind}/{ownerId}/{sourceId}/position` route) accepts `{"position": <int>}`. The handler clamps to `[1, N]`, runs a single transaction that rewrites the affected rows' `sort_order` so the requested source lands at `position` and others shift up or down to fill the gap.

## v1 apply sites — checklist

Backend:

- [ ] `internal/db/migrations/` — add `sort_order` to both `records` and `event_sources`, with backfill
- [ ] `CurrentSchemaVersion` bump + `docs/migrations/v{N+1}.md`
- [ ] `internal/records/soldier_service.go` — `replaceRecords` writes `sort_order`; new `MoveRecordWithinPerson(personID, sourceID, position)` service method
- [ ] `internal/records/event_service.go` — `AttachSourcesToEvent` writes `sort_order`; new `MoveEventSource(eventID, sourceID, position)` service method
- [ ] `internal/appshell/` — new PATCH route(s) for position move, wired through `routes.go`
- [ ] `internal/appshell/` — read-path handlers stop mutating `sort_order` on unrelated saves (regression test on the `replaceRecords` rebalance)

UI:

- [ ] **Person Record detail page** — `panel.soldier.detail.records` (`internal/templates/soldier_card.templ:538`) gets up/down arrows + numeric position input on each row
- [ ] **Event Record detail page** — `#data-event-sources-list` (`internal/templates/event_panels.templ:36` + `event_detail.templ:137`) gets the same controls
- [ ] **Person Record Create/Edit forms** — `data-record-list` (`internal/templates/entry_form.templ:301`) gets the same controls so a user can reorder while building
- [ ] **Event Record Create/Edit forms** — inline source rows in `internal/templates/event_form.templ:113-116` get the same controls

Print/export (verify only — no UI):

- [ ] `templates/common/record_card.typ:362-380` — confirm the Typst side still renders in `sort_order, id` order after the column swap; no template change expected.

Regression net:

- [ ] Existing `replaceRecords` / `AttachSourcesToEvent` tests still pass; add coverage for the move endpoints and the post-move display order on both detail pages.
- [ ] `audit/discover_orphan_handlers.mjs` shows no orphan handlers after the new PATCH route lands.

## Non-goals (defer to follow-up)

- Drag-and-drop. Considered; deferred because WebView2 DnD is fragile and the arrow + numeric hybrid covers the common cases.
- Reorder UI on Shared / Backup Archive import (Merge Review). The incoming records' `sort_order` should round-trip via `sync_id`; user reordering after import is a separate UX decision.
- Reordering Claims, Findings, Tags, or Research Log entries — same gap exists but each has its own apply-sites; file separately.

## Open questions

1. Should the numeric input clamp on blur or accept any integer (and the server clamps)? Server-clamp is safer; UI just rounds the visual feedback.
2. Should the position input be visible by default or behind a "Show positions" toggle? Default-visible keeps it discoverable but adds row width on the narrow Event form.
3. Should the move PATCH route be one shared `/sources/{kind}/...` or two parallel `/soldiers/.../sources/...` + `/events/.../sources/...`? Shared is DRYer but breaks the "every route has a screen owner" convention from `routes.md`.