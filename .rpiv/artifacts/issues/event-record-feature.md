# Feature: Event Records (Person Record subtype)

> **Status:** Draft for Critique. Decisions locked. Not yet filed.
> **Triage labels (proposed):** `type:enhancement` `area:domain+schema+exports+ui` `priority:high` `cohort:v-next` `needs-triage` → `ready-for-agent`
> **Applies to:** `dev` (no PR until issue filed and triaged)

## Summary

Add a new "Event Record" entity to the Local Archive. An Event Record is a
**Person Record subtype** (a fourth `entry_type` value alongside Soldier,
Wife, and Widow) that represents a dated event in Civil War history — a
Battle, a Campaign, a Hospital stay, a Death, etc. — and links to one or
more Person Records via a many-to-many junction. Events carry the full
paper trail (Source Records, Claims, Findings, Scratch Pad, Research Log,
Tags, Review Queue), a free-form long-form write-up ("Description") with
a per-PDF excerpt override (mirroring the Person Record biography
override), and attached images (reusing the existing `images` table).

The feature ships in v1 with all four export paths live: Shared Archive
(`.ddshare`), Backup Archive (`.ddbak`), Static Archive (`.ddsite`), and
a new per-Event PDF via a new `event.typ` Typst template.

## Motivation

The current archive can describe *people* and the *evidence* for them, but
it cannot describe *the things those people were part of*. A Battle of
Gettysburg Record cannot be its own archive entry today — it can only
appear as a free-text fragment inside a Soldier's Service Timeline, a
date-clue extraction from a Source Record's body, or a claim encoded in
a Finding. This forces researchers to scatter Battle context across
multiple Person Records and reconstruct it by hand.

Event Records make the dated event a first-class archive entry. A
researcher can now create one Event Record for the Battle of Gettysburg,
attach it to every Soldier who fought there, attach a pension
application as a Source Record, write a long-form historical narrative,
attach a period engraving as an image, and share the whole Event with
another researcher in one go.

## Glossary

### New entry under `CONTEXT.md` ## Language

> **Event Record**: A primary archive entry for a dated event that links
> to one or more Person Records. Has a free-text `kind`, optional begin
> and end dates, an optional long-form Description with a per-PDF excerpt
> override, and zero or more attached images. Carries the same paper
> trail as a Person Record (Source Records, Claims, Findings, Scratch
> Pad, Research Log, Tags, Review Queue).
> _Avoid_: timeline event, archive event, cross-record link, historical
> event (too generic)

### New entries under `CONTEXT.md` ## Relationships

> - An **Event Record** may link to one or more **Person Records**
> - A **Person Record** may be linked to one or more **Event Records**
> - A **Person Record** has zero or more **Timeline Markers** that may
>   reference an **Event Record** for the marker source

### Renames under `CONTEXT.md` ## Language

> **Timeline Marker** (renamed from Timeline Event): A dated item shown
> on a Service Timeline.
> _Avoid_: Claim, service event

### Renames under `CONTEXT.md` ## Relationships

> - A **Service Timeline** contains one or more **Timeline Markers**
> - A **Service Event** is a kind of **Timeline Marker**

## Locked decisions (RPCI Critique gate)

| # | Decision | Choice | Rationale |
|---|---|---|---|
| 1 | Name | **Event Record** | Matches how users describe the entity. `Event` alone collides with `Timeline Event` and `Service Event`. |
| 2 | Domain shape | **Subtype of Person Record** | Reuses the `soldiers` table; the v55 entry-type discipline (`internal/models/constants.go:14-36`) extends cleanly. Person-specific columns become nullable. |
| 3 | Person ↔ Event link | **Many-to-many via new `event_person_links` junction** | A Battle has N Soldiers; a Soldier fights in N Battles. |
| 4 | Event `kind` | **Free-text `kind TEXT`** | No taxonomy. Researcher can label "Battle", "Earthquake", "Political Convention", etc. Schema is one TEXT column. |
| 5 | Paper trail | **Full: Source / Claim / Finding / Scratch Pad / Research Log / Tags / Review Queue** | Consistent with existing Person Record shape. Every Person-Record feature learns the new subtype. |
| 6 | FK column rename | **Rename `soldier_id` → `person_record_id`** in: `records`, `images`, `scratchpad_cache`, `research_tasks`, `merge_review_conflicts` (3 cols), `duplicate_audit_findings` (2 cols) | Semantically correct: the column now references any Person Record subtype. FTS5 trigger rewrite follows. |
| 7 | Display ID | **Own `EVT-NNNNN` namespace** | Instantly distinguishable on lists and Service Timelines. Mirrors the v180 per-user namespace discipline. |
| 8 | Static archive shape | **`window.DIXIE_DATA = { records: [...], events: [...] }`** | One bundle, one site load. `StaticArchiveRecord` gets `Kind`, `LinkedDisplayIDs`, `Description` fields. |
| 9 | Shared + Backup scope | **Always include Event Records** | Events are primary archive entries (per glossary). No new toggle in the Share dialog. Merge review dedupes Events by Display ID. |
| 10 | Exports v1 | **All four**: Shared (`.ddshare`), Backup (`.ddbak`), Static (`.ddsite`), Per-Event PDF (new `event.typ`) | Maximum first-release value. |
| 11 | Long-form text field | **`description TEXT` on `soldiers`** | Free-form. User-facing label "Description". Reuses the existing `description` column name pattern. |
| 12 | PDF excerpt override | **Reuse `pdf_excerpt_override TEXT` on `soldiers`** | Same column. Semantics: "short version of the row's long-form text" — biography for Person Records, description for Events. One Typst helper `render-long-form-inline(s)` dispatches on `entry_type`. |
| 13 | Image storage | **Reuse `images` table** | FK rename `soldier_id → person_record_id`. Storage path keys off Display ID — Event images live under `.dixiedata/events/EVT-NNNNN/...`. One image store, one storage migration, one image-rotation policy. |

## Schema sketch (v60)

```sql
-- Block A: ADD COLUMN on soldiers for event subtype (Reversible)
ALTER TABLE soldiers ADD COLUMN kind TEXT;
ALTER TABLE soldiers ADD COLUMN begin_date TEXT;
ALTER TABLE soldiers ADD COLUMN end_date TEXT;
ALTER TABLE soldiers ADD COLUMN description TEXT;
-- (pdf_excerpt_override already exists at internal/db/schema.go:60)

-- Block B: CREATE event_person_links (Reversible)
CREATE TABLE IF NOT EXISTS event_person_links (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id       INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    person_id      INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    sync_id        TEXT,
    event_sync_id  TEXT,
    person_sync_id TEXT,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (event_id, person_id)
);
CREATE INDEX IF NOT EXISTS idx_event_person_links_event  ON event_person_links(event_id);
CREATE INDEX IF NOT EXISTS idx_event_person_links_person ON event_person_links(person_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_event_person_links_sync_id ON event_person_links(sync_id);

-- Block C: RENAME COLUMN soldier_id → person_record_id (PartiallyReversible)
ALTER TABLE records                  RENAME COLUMN soldier_id TO person_record_id;
ALTER TABLE images                   RENAME COLUMN soldier_id TO person_record_id;
ALTER TABLE scratchpad_cache         RENAME COLUMN soldier_id TO person_record_id;
ALTER TABLE research_tasks           RENAME COLUMN soldier_id TO person_record_id;
ALTER TABLE merge_review_conflicts   RENAME COLUMN local_soldier_id  TO local_record_id;
ALTER TABLE merge_review_conflicts   RENAME COLUMN left_soldier_id   TO left_record_id;
ALTER TABLE merge_review_conflicts   RENAME COLUMN right_soldier_id  TO right_record_id;
ALTER TABLE duplicate_audit_findings RENAME COLUMN left_soldier_id   TO left_record_id;
ALTER TABLE duplicate_audit_findings RENAME COLUMN right_soldier_id  TO right_record_id;

-- Block D: FTS5 trigger column-list extension (PartiallyReversible)
-- ensureSoldierFTS grows from 39 columns to 43: add kind, begin_date, end_date,
-- description to all six CREATE TRIGGER statements + the INSERT INTO soldiers_fts SELECT.
-- See internal/db/schema.go:576-714 for the existing trigger block.
```

Bumps `CurrentSchemaVersion` from 59 to 60. New `docs/migrations/v60.md`
documents the four blocks. Reversibility classification updates
`docs/migrations/reversibility.md` (most blocks Reversible; Block C is
PartiallyReversible per the column-rename lossy side).

## Apply sites (v1 checklist)

The feature is not "shipped" until every box is checked. Backend-only
landings require a tracked follow-up issue per CONTEXT.md Backend-First
Law. Every route must have a templ invoker in the same PR (the
`audit/discover_orphan_handlers.mjs` CI gate enforces this).

- [ ] **Event Record detail page** — `/events/{id}` renders an Event
      card with kind pill, begin/end dates, long-form Description (or
      the PDF excerpt override if set), image gallery, and a section
      listing linked Person Records.
- [ ] **Event Record new page** — `/events/new` GET shows the entry
      form (kind text input, begin_date, end_date, description
      textarea, pdf_excerpt_override textarea); POST creates the row
      in `soldiers` with `entry_type="event"` and mints an `EVT-NNNNN`
      Display ID.
- [ ] **Event Record edit page** — `/events/{id}/edit` GET re-renders
      the form with existing values; POST updates the row.
- [ ] **Event Record delete** — `/events/{id}` DELETE removes the row
      + cascades `event_person_links` rows + cascades `scratchpad_cache`
      row.
- [ ] **Event Record per-PDF export** — `/events/{id}/pdf` POST calls
      `event.typ` (new Typst template) via the Registry; returns the
      PDF inline. `templateForRecordType` branch `case "event" →
      "event_landscape" / "event_portrait"`. Uses the existing
      `guardedSaveFileDialog` helper (per `docs/agents/dialog-guard.md`).
- [ ] **Event Record images** — `/events/{id}/images` GET/POST/DELETE
      reuses the existing `images` table. Upload, rotate, gallery
      render, delete — all reused from Person Record image flows with
      no per-event branch.
- [ ] **Person Record → Events tab** — `/soldiers/{id}/events` GET
      renders the Events tab on the Person Record detail page. Lists
      all Events linked via `event_person_links` for that Person
      Record.
- [ ] **Person Record → link an Event** —
      `/soldiers/{id}/events/{eventId}/attach` POST adds a row to
      `event_person_links`; a UI button "Link existing Event" on the
      Events tab invokes it.
- [ ] **Person Record → unlink an Event** —
      `/soldiers/{id}/events/{eventId}/detach` POST removes the
      `event_person_links` row; a UI button "Unlink" on each linked
      Event row invokes it.
- [ ] **Person Record → quick-add Event** —
      `/soldiers/{id}/events/quick-add` POST creates a new Event with
      the linking `event_person_links` row in one transaction; a UI
      button "Quick-add Event" on the Events tab invokes it.
- [ ] **Browse row chip for Events** —
      `internal/templates/browse.templ` row chip renders an "Event"
      pill for `entry_type=event` rows; existing entry-type filter
      dropdown adds the "Event" option.
- [ ] **Source Records for Events** — `/events/{id}/sources` GET
      renders the attached Source Records section on the Event detail
      page (reuses `record_card` with `entry_type=event`).
- [ ] **Scratch Pad for Events** — `/events/{id}/scratchpad` GET/PUT
      renders the Scratch Pad section on the Event detail page (FK
      rename `scratchpad_cache.soldier_id → person_record_id`).
- [ ] **Research Log for Events** — `/events/{id}/research-log`
      GET/POST renders the Research Log section (FK rename
      `research_tasks.soldier_id → person_record_id`).
- [ ] **Tags for Events** — `/events/{id}/tags` GET/POST reuses
      `person_record_tags` (already `person_id`, no rename needed)
      with the Event's row id.
- [ ] **Review Queue for Events** —
      `internal/records/quality_scan.go` zero-link check fires a
      `event-zero-links` review issue when an Event has zero
      `event_person_links` rows.
- [ ] **Insights drilldown for Events** —
      `internal/templates/insights.templ` adds an Event drilldown
      link; `insights_handlers.go:75-79` adds the `case "event"`
      branch.
- [ ] **Service Timeline → Timeline Markers from Events** — the
      derived Service Timeline on a Person Record page can now show
      Timeline Markers sourced from Event Records linked to that
      Person. New data path: `soldier_service.go:ServiceTimeline`
      joins `event_person_links` for the central Soldier and pulls
      Event Record rows. Existing date-clue inference from
      `records.details` is preserved as a fallback for Events that
      aren't yet linked.
- [ ] **Static Archive Events section** —
      `window.DIXIE_DATA = { records: [...], events: [...] }` mounts
      in `static_archive.go:109`; the search filter + row rendering
      learn the event shape; Events section HTML in
      `staticArchiveIndexHTML`.
- [ ] **Shared Archive Events travel** —
      `ExportSharedWithTags` (`internal/archive/backup_service.go:315-356`)
      denormalizes `linked_display_ids` onto Event rows in the shared
      JSON; shared-import rebuilds `event_person_links`.
- [ ] **Backup Archive Events travel** — `.ddbak` sqlite snapshot
      already includes the new table + columns via `b.db.SnapshotTo`;
      no code change beyond schema.
- [ ] **GLOSSARY entry: Event Record** — `CONTEXT.md` gains the
      "Event Record" entry under `## Language` and the relationship
      "An **Event Record** may link to one or more **Person Records**"
      under `## Relationships`.
- [ ] **GLOSSARY rename: Timeline Event → Timeline Marker** —
      `CONTEXT.md:63-68,140-141,182` updated in the same PR.
- [ ] **`audit/smoke_events.mjs`** — new audit probe covers the
      Events tab, link/unlink, quick-add, per-event PDF response
      shape + `page.url()` after click (per
      `docs/agents/feature-protocol.md` Smoke probe anti-pattern
      note — the response-only assertion is insufficient).

## Acceptance criteria

- [ ] User creates an Event Record via `/events/new`; it lands in
      `soldiers` with `entry_type="event"`, an `EVT-NNNNN` Display ID
      minted by the new `NextEventID` counter, and the typed-in
      kind/begin_date/end_date/description.
- [ ] User uploads images to an Event via `/events/{id}/images`; the
      images live under `.dixiedata/events/EVT-NNNNN/`; the gallery
      renders on the detail page.
- [ ] User sets a PDF excerpt override on the Event edit page; the
      rendered `event.typ` PDF shows the override text instead of the
      full description.
- [ ] User attaches an Event to two Person Records via
      `/soldiers/{id}/events/{eventId}/attach`; both `event_person_links`
      rows exist; the Events tab on each Person Record detail page
      lists the Event.
- [ ] `/events/{id}/pdf` returns a PDF generated by `event.typ`
      listing the Event + linked Person Records + image gallery +
      description (or override).
- [ ] Shared Archive export includes Event Records with denormalized
      `linked_display_ids`; recipient import rebuilds
      `event_person_links` rows.
- [ ] Backup Archive export round-trips Events + links + images
      through SQLite snapshot.
- [ ] Static Archive `window.DIXIE_DATA = { records: [...], events: [...] }`
      renders Events in the search results with a kind pill; the
      detail panel shows the description + image gallery + linked
      Person Records.
- [ ] Service Timeline on a Person Record detail page shows Timeline
      Markers sourced from the linked Event Records (dates come from
      the Event, not from date-clue inference on Source Record text).
- [ ] Review Queue surfaces an `event-zero-links` issue for any Event
      with zero `event_person_links` rows.
- [ ] `make test` green; `make in-place-safety` green;
      `audit/discover_orphan_handlers.mjs --strict` green;
      `node audit/smoke_events.mjs` green.

## Slice plan

### Slice 1 — Schema + glossary (Tier 2 vertical)

- **Files**: `internal/db/schema.go`, `internal/db/migrations.go`,
  `internal/models/constants.go`, `internal/records/soldier_service.go`
  (`soldierSelectColumns` grows to 43 cols), `CONTEXT.md`,
  `docs/migrations/v60.md`, `docs/migrations/reversibility.md`,
  `internal/db/csaid.go` (new `NextEventID`).
- **Success criteria**: `make test` green with
  `CurrentSchemaVersion = 60`; `NextEventID` mints `EVT-00001` on empty
  DB; `event_person_links` table exists with the right indexes; FTS5
  triggers cover the new columns; all FK renames applied.
- **Regression net**: `internal/db/csaid_test.go` new tests for
  `NextEventID`; `internal/db/schema_test.go` extends for the four new
  blocks; `internal/db/v60_migration_test.go` (forward + partial-down
  + zero-data-loss scenarios).

### Slice 2 — Event Record service + handlers (Tier 2 vertical, no UI)

- **Files**: `internal/records/event_service.go` (new),
  `internal/appshell/events_handlers.go` (new),
  `internal/appshell/routes.go` (12+ new routes),
  `internal/records/quality_scan.go` (event-zero-links branch),
  `internal/peopleinfo/name.go` (`DisplayEntryType` event branch),
  `internal/records/soldier_service.go:2312`
  (`normalizeSoldierEntry` event bypass).
- **Success criteria**: every new route registered; curl-invocable
  end-to-end; Quality Scan emits `event-zero-links` issue; image
  upload via existing `images` path works for Event rows.
- **Regression net**: handler tests cover POST/GET/PUT/DELETE + linked
  list + image attach.

### Slice 3 — UI surface (Tier 3 apply-site unit per route)

- One commit per route: events/new form, events/{id} detail,
  events/{id}/edit, events/{id}/pdf, events/{id}/images, events/{id}/sources,
  events/{id}/scratchpad, events/{id}/research-log, events/{id}/tags,
  soldiers/{id}/events tab, attach/detach/quick-add. Each commit
  pairs the templ invoker with the matching
  `audit/smoke_events.mjs` assertion.

### Slice 4 — Static + Shared + Backup + PDF

- Static archive bundle shape change + `static_archive.go` JS updates.
- Shared JSON denormalize `linked_display_ids`; merge alias support for
  events.
- Backup snapshot is no-op (just schema).
- Per-event PDF: new `templates/event_landscape.typ` +
  `templates/event_portrait.typ` + `render-event-card` helper in
  `templates/common/event_card.typ` (new). Reuses `render-image-panel`
  and a new generic `render-long-form-inline(s)` that picks
  `biography` or `description` based on `entry_type`.

### Slice 5 — Service Timeline sourcing

- `soldier_service.go:ServiceTimeline` joins `event_person_links` for
  the central Soldier and pulls Event Record rows as Timeline Markers.
- New viewmodel field `MarkerSource` distinguishes "inferred from
  source record text" vs "sourced from linked Event Record".

## Test plan

- **Unit** — `internal/records/event_service_test.go` (~15 cases: CRUD,
  link/unlink, dedupe by display_id, zero-link detection, image
  attach)
- **Handler** — `internal/appshell/events_handlers_test.go` (~25 cases:
  every new route)
- **Migration** — `internal/db/v60_migration_test.go` (forward + partial
  down + zero-data-loss for the 4 blocks)
- **Typst** — snapshot test for `event_landscape.typ` and
  `event_portrait.typ` (golden output)
- **Glossary** — `internal/db/schema_test.go` regression for the FK
  rename; the legacy `soldier_id` column should not exist on any of the
  8 renamed tables after the migration runs.
- **Smoke probe** — `audit/smoke_events.mjs` (per UI apply-site,
  response shape + `page.url()`)

## Files

- `internal/db/schema.go`, `internal/db/migrations.go`,
  `internal/db/csaid.go`, `internal/db/scratchpad.go`
- `internal/models/constants.go`
- `internal/records/soldier_service.go`, `internal/records/quality_scan.go`,
  `internal/records/event_service.go` (new)
- `internal/appshell/routes.go`, `internal/appshell/events_handlers.go`
  (new), `internal/appshell/soldiers_handlers.go`,
  `internal/appshell/insights_handlers.go`
- `internal/archive/compat.go`, `internal/archive/backup_service.go`,
  `internal/archive/export_service.go`, `internal/archive/static_archive.go`
- `internal/peopleinfo/name.go`
- `internal/templates/entry_form.templ`, `internal/templates/entry_form_helpers.go`,
  `internal/templates/soldier_card.templ`, `internal/templates/browse.templ`,
  `internal/templates/insights.templ`, `internal/templates/timeline.templ`
- `templates/event_landscape.typ` (new), `templates/event_portrait.typ`
  (new), `templates/common/event_card.typ` (new),
  `templates/common/record_card.typ`
- `internal/viewmodel/types.go`, `internal/viewmodel/mappers.go`
- `CONTEXT.md`, `docs/migrations/v60.md` (new),
  `docs/migrations/reversibility.md`, `CHANGELOG.md`
- `audit/smoke_events.mjs` (new)

## Out of scope (explicitly deferred)

- **Bulk Event PDF** — `bulk_event.typ` for batch exports that include
  events. v2.
- **Event-only filter chip on Browse** — v2. v1 uses the shared search
  with kind-pill rows.
- **Rename `soldiers` table → `person_records`** — Tier 3 of
  `GLOSSARY_ALIGNMENT.md`. Not in this PR.
- **`models.Event` as sibling struct to `models.Soldier`** — the
  umbrella rename. v2. v1 extends `models.Soldier` with optional
  `Event *EventRecord` field.
- **Event Record → Event Record links** (e.g., Battle is part of
  Campaign) — v2.

## Related

- CONTEXT.md Backend-First Law (§Laws)
- CONTEXT.md Glossary law (§Laws)
- CONTEXT.md Schema-bump law (§Laws)
- CONTEXT.md Doc-comment law (§Laws)
- CONTEXT.md Apply-sites law (§Laws)
- `docs/agents/feature-protocol.md` — 3-tier commit rule, Module
  discipline, pipeline phasing
- `docs/agents/dialog-guard.md` — per-event PDF handler must use
  `guardedSaveFileDialog` or `a.inFlight.LoadOrStore` per
  `CONTEXT.md:144-160`
- `docs/migrations/reversibility.md` — per-block classification for the
  four new blocks
- `docs/agents/cli-plan.md` — headless CLI subcommands for Event
  Records (`dixiedata event list/show/create`) — staged in a later
  phase per the 7-phase CLI roadmap
- `GLOSSARY_ALIGNMENT.md` — Tier 1/2/3 sweep; this PR lands Tier 1
  glossary rename (`Timeline Event → Timeline Marker`) + part of Tier
  2 (Event Record as new entry type)
- Issue #183 — worked example for apply-sites + slice plan + glossary
- Issue #266 — v{MAJOR}.{U}.{N} version split; `CurrentSchemaVersion`
  advance only
- Issue #257 — "shipped but invisible" sweep; the apply-sites checklist
  above is the explicit anti-pattern guard
- `internal/db/csaid.go` — current `NextDXDID` (the new `NextEventID`
  is a sibling; existing namespace filter must exclude `EVT-` prefix
  to prevent cross-namespace collisions)
- `templates/common/record_card.typ:625-665` — the existing
  `PDFExcerptOverride` helper; the new `render-long-form-inline(s)`
  generalizes this for both Person Records and Events

## Critique gate (RPCI)

This is the **C** (Critique) phase of the RPCI cycle. The previous
phases:

- **R** (Research) — scope-tracer + research sub-agent answered 10
  dense questions with file:line evidence. Findings integrated into
  the locked decisions and the schema sketch.
- **P** (Plan) — locked-decisions table + slice plan + apply-sites
  checklist. The slice plan enforces the Backend-First Law: every
  backend surface in Slice 2 has a matching UI apply-site in Slice 3,
  and every UI apply-site in Slice 3 has a matching smoke probe in
  Slice 3's commit.
- **C** (Critique, this gate) — pending user sign-off. Specifically:
  1. Are the 13 locked decisions acceptable? (See Locked decisions
     table above.)
  2. Is the Slice 1 → 5 ordering the right one? (Slice 5 — Service
     Timeline sourcing — is the only slice that can be deferred to a
     follow-up issue without breaking the v1 feature.)
  3. Is the "Out of scope" list correct?
  4. Should this ship as one PR or split into multiple?

After sign-off, file via `gh issue create --body-file
.rpiv/artifacts/issues/event-record-feature.md` and link from the
issue's first comment to this artifact for traceability.
