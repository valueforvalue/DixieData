# v61 Event Sources Decomposition — Issue #340

This document captures the slice-level decomposition for the v61
schema migration that addresses the data-loss bug discovered
2026-07-04 in the per-Event Sources panel. The fix replaces the
"reuse the `records` table" hack from slot #329 with a dedicated
`event_sources` table keyed by `event_id`. The decomposition
mirrors the shape of the v60 Event Records decomposition
(`docs/agents/notes/slice3-decomposition.md`) — atomic, single-
concern slices that each land as one commit.

## Why this exists

The v60 work shipped slot #329 ("Per-Event Sources panel") with
the comment that `records` was reused as a side effect of the
schema blocker — the `event_source_links` M-to-M table was the
original plan but the code never actually shared sources between
Events and Person Records (the slot always created fresh rows
via `AttachSourceToEvent`). That shortcut collided with the
existing `replaceRecords` semantics: every
`SoldierService.Update` runs `replaceRecords(tx, soldierID, ...)`
which `DELETE FROM records WHERE person_record_id = ?` then
re-inserts from the form's `Records` field. The Event edit form
has no Records input, so `parseEventForm` returned an Event
struct with `Records: nil`, the Update ran with an empty list,
and **every Edit wiped every attached Source Record**.

## Fix shape

A new `event_sources` table, keyed by `event_id` (which is
`soldiers.id` for `entry_type='event'` rows, per the v60
polymorphic subtype convention). Schema v61 is the migration
that adds the table; slice 1 below. Event Service methods
migrate to read / write the new table; slices 2–4. The view-
model gets an `EventSources []SourceRecord` field separate
from `SourceRecords []SourceRecord` (which still sources from
the `records` table for Person Records); slice 5. The on-page
`event_detail.templ` swap target becomes the new field; slice 6.

The orphaned `records` rows written by v60 slot #329 are left in
place (no FK from `records` to `soldiers` to cascade on). They
are inert after the migration because no Event service method
will read them. Future code that wants to clean them up can
issue a one-time DELETE during a later maintenance task; out of
scope for v61.

## The 6 slices

### Slice 1: Schema migration v61 + versioninfo bump

Files:
- `internal/db/schema.go` — append `event_sources` table to the
  inline schema constant (so fresh installs get it in Block 1)
- `internal/db/migrations.go` — add a new `Migration` entry
  `block-61-event-sources` that runs `CREATE TABLE IF NOT
  EXISTS event_sources (...)` + indexes (idempotent, reverses
  cleanly via DROP TABLE / DROP INDEX)
- `internal/versioninfo/versioninfo.go` — bump
  `CurrentSchemaVersion` from 60 to 61
- `internal/db/migrations_test.go` — extend the round-trip test
  to cover v60→v61 forward and back

Schema:

```sql
CREATE TABLE IF NOT EXISTS event_sources (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id         TEXT,
    event_id        INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    event_sync_id   TEXT,
    record_type     TEXT NOT NULL,
    app_id          TEXT NOT NULL,
    details         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_event_sources_event ON event_sources(event_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_event_sources_sync_id ON event_sources(sync_id);
```

Reversibility: Reversible (DROP TABLE + DROP INDEX).

Why standalone: every other slice needs the table to exist.
Touches no user-facing code path — binary reads / writes are
unaffected because no code yet references the table.

### Slice 2: EventService moves AttachSource / DetachSource / ListSourcesForEvent to event_sources

Files:
- `internal/records/event_service.go` — replace the three
  methods to use `event_sources` instead of `records`. The
  method signatures and return shapes stay the same so the
  handler / viewmodel layers don't need to change in this
  slice. Internal SQL: INSERT / DELETE / SELECT on
  `event_sources`.

Tests:
- `internal/records/event_service_test.go` — add
  `TestEventService_SourceRoundTrip` that creates + attaches +
  lists + detaches via the new methods.
- Update existing event-source-touching tests in
  `internal/appshell/events_handlers_test.go` to verify the
  new table is populated (existing tests pass already because
  they go through the service layer, but they currently read
  via `app.events.ListSourcesForEvent` which is what changes).

Why standalone: the service contract is the same; the handler
layer keeps calling `AttachSourceToEvent` etc. without changes.
This is the smallest atomic change that addresses the data-
loss bug — the bug is *not* in the handler, it's in the
service-level reuse of `replaceRecords` via `soldiers.GetByID`.
Moving the SQL out of the `records` table breaks the
`replaceRecords` interaction.

### Slice 3: Add the regression test that proves the bug is fixed

Files:
- `internal/records/event_service_test.go` — new
  `TestEventService_UpdateEventPreservesAttachedSources`:
  create event, attach source, update event (description
  change), assert source still attached. This is the test
  the v60 work was missing; it would have caught the bug.

Why standalone: separate commit so the regression net is
visible in git history. The test was proven to fail against
the v60 code via a scratch test on 2026-07-04 (see
docs/agents/notes/v61-bug-repro.md).

### Slice 4: EventService.GetEventByID no longer loads Event sources from the records table

Files:
- `internal/records/event_service.go` — `GetEventByID` (and
  the helpers it delegates to) currently rely on
  `SoldierService.GetByID` which loads ALL rows from
  `records` keyed by `person_record_id`. After slice 2, the
  Event sources no longer live there; the new method should
  return a result with `Event.Records` empty for Events and a
  new `Event.EventSources []models.Record` populated by a
  fresh SELECT against `event_sources`.
- `internal/models/models.go` — add an `EventSources
  []models.Record` field to the `Soldier` struct, populated
  only for Events. Person Records leave it nil.

Why standalone: this is the read-side projection that the
view-model and templ layers will pick up in slice 5. Slice 4
ensures the data is on the wire; slice 5 wires it through.

Tests:
- `internal/records/event_service_test.go` — add
  `TestEventService_GetEventByIDReturnsEventSourcesField`.

### Slice 5: viewmodel + presentation + event_detail.templ read from the new field

Files:
- `internal/viewmodel/types.go` — add
  `EventSources []SourceRecord` to `PersonRecord`.
- `internal/viewmodel/mappers.go` — populate the new field
  from `input.EventSources` in `PersonRecordFromModel` (only
  for Events; Person Records leave it empty).
- `internal/templates/event_detail.templ` — replace the
  `event.SourceRecords` references with
  `event.EventSources`. Update the `EventSourcesListFragment`
  helper in `internal/templates/event_panels.templ` to
  accept the new field. Update the `EventSourcesListFragment`
  signature in the generated `_templ.go`.
- `internal/appshell/events_handlers.go` — the
  `renderEventSourcesListFragment` helper now reads
  `event_sources` directly (a tiny SELECT in the handler)
  rather than going through `ListSourcesForEvent` to keep
  the read path independent of the `Soldier` view-model
  projection.

Why standalone: this is the UI-facing change. The view-model
gain keeps `SourceRecords` for Person Records intact
(soldier_card.templ still works unchanged), only adds a new
field for Events.

### Slice 6: Document + close #340

Files:
- `CHANGELOG.md` — `### Fixed` entry for the data-loss bug.
- `docs/agents/domain.md` — note the schema change.
- Comment on issue #340 referencing the regression test
  `TestEventService_UpdateEventPreservesAttachedSources` and
  the migration block `block-61-event-sources`.
- Optionally: orphan-cleanup DELETE statement for the inert
  `records` rows from v60 slot #329 — *not* required for
  correctness (no FK from `records` to `soldiers`, no read
  path touches them after slice 2) but cheap insurance.
  Decision deferred; can ship as slice 6.1 if the user
  wants.

## Why 6 slices, not 1

The data-loss bug is one bug, but the fix touches 5 layers
(db, service, viewmodel, presentation, templ). A single
commit would be ~12 files / 250+ insertions, exactly the
shape AGENTS.md flags as "shipped-but-invisible bug":
backend ships first, UI later, integration test never
runs. Slicing keeps each commit individually buildable +
test-green, so a regression at any layer surfaces in the
commit that introduced it.

Slices 1, 2, 3 can land in any order from a buildability
standpoint, but the natural order is:

  slice 1 (schema) → slice 2 (service writes new table) →
  slice 3 (regression test proves fix) → slice 4 (read path
  populates new field) → slice 5 (UI reads new field) →
  slice 6 (docs + close)

Slices 4 and 5 must land in order; slices 1 and 2 are
prerequisites for slice 3 to be meaningful (the regression
test needs the new table to exist and the writes to go to
it).

## Precedents & lessons

- The v60 slot #329 shipped with a "we'll fix the schema
  blocker later" comment but never filed a follow-up issue.
  This is the root cause of the bug surfacing 6 days after
  merge. Lesson: when a slot ships with a known schema
  shortcut, the slot commit must file a follow-up issue
  before merging. (See: docs/COMMON_BUGS.md "Spec drift in
  follow-up issues".)
- The v60 slot #329 test
  (`TestHandleEventSourcesAndScratchpad`) only asserted POST
  status codes, not the attach-then-update-then-list round
  trip. This is the same test-coverage gap that hid bugs
  #340 and #341 simultaneously. Lesson: handler tests must
  exercise the end-to-end user flow (click → backend →
  re-render → visible state) at least once per surface,
  not just per HTTP verb.
- The orphan-handler probe (`audit/discover_orphan_handlers.mjs`)
  flagged both #340's and #341's broken paths before either
  was caught by users. Both were unrelated to the orphan
  classification — #341 was a wiring contract mismatch,
  #340 was a service-level reuse of `replaceRecords`. The
  probe's heuristic needs refinement but its role as an
  audit-fallout signal is validated; it caught two distinct
  bug classes via the same surface.