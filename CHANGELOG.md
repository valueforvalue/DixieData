# Changelog

All notable changes to DixieData are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/) and the project adheres to
[Semantic Versioning](https://semver.org/) — DixieData uses `v1.2.N` where
N is `CurrentSchemaVersion` from `internal/versioninfo/versioninfo.go`.

Release dates are the commit date of the tagged release. Internal refactors
that do not change user-visible behavior live under `### Maintenance` so
the Added / Changed / Fixed / Removed lists stay scannable.

## [Unreleased]

### Maintenance

- **CONTEXT.md typo fix** (issue #364). Line 158 read
  `A **Event Record** is a kind of **Person Record**`; now
  reads `An **Event Record** is a kind of **Person Record**`
  to match the L159 sibling bullet and standard English
  vowel-sound indefinite article usage.
- **Makefile `make debug` chain refactor** (issue #366). The
  `build` and `debug` targets used to chain `probe-clean web
  seed gold tune-bin` via `$(call RECURSIVE_MAKE,...)`, which
  recurses through `$(MAKE)` inside a PowerShell wrapper.
  GNUWin32 make (the legacy install under `C:\Program Files
  (x86)\GnuWin32\bin\make.exe`) misparses the recursive call
  when `$(MAKE)` itself lives under "Program Files (x86)" —
  the parens break sh's tokenization and the inner make exits
  with `e=87`, silently dropping the chain. The recipes now
  inline the chain as direct `go build` invocations (matching
  the pattern `web`/`seed`/`gold`/`tune-bin` already use when
  invoked standalone), sidestepping the bug for the affected
  env and removing a layer of indirection for everyone. The
  `RECURSIVE_MAKE` variable is removed; the `web`/`seed`/
  `gold`/`tune-bin` sub-targets are preserved because
  `freshness` still depends on them.

### Added

- **Event Records v1 (Person Record subtype)** (issue #320
  sequence closure per the locked 2026-07-04 close criteria;
  tracked via close-gate issue #359). Event Records are now a
  first-class Person Record subtype alongside Soldier, Wife,
  and Widow. A Battle, a Campaign, a Hospital stay, or any
  other dated Civil War event can be its own archive entry
  with a free-text `kind`, `MM/DD/YYYY` begin/end dates, a
  long-form `Description` (with the same `pdf_excerpt_override`
  short-form behavior Person Records use for biography), and
  the full paper trail (Sources, Claims, Findings, Scratch
  Pad, Research Log, Tags, Review Queue). Events link to one
  or more Person Records via a many-to-many
  `event_person_links` junction; the reverse direction lights
  up a new "Events" tab on every Person Record detail page.
  Display IDs are minted in their own `EVT-NNNNN` namespace,
  instantly distinguishable from soldier records on lists and
  Service Timelines. All four export surfaces carry Events:
  Shared (`.ddshare`) with `linked_display_ids` denormalized
  per RPCI D9 so recipients auto-attach without a separate
  fetch, Backup (`.ddbak`) schema-only, Static
  (`window.DIXIE_DATA = { records, events }`), and a new
  per-Event PDF via `templates/event_landscape.typ`.

  Slot-by-slot commit map (the 16-slot sequence landed on
  `dev` between 2026-07-04 and 2026-07-05; full per-slot
  narration already lives in the slot-16 closure narrative
  in the `### Maintenance` block below):
    foundation slots (pre-numbered #320.X, cite slice 1..3):
      schema v60 + FK rename          `d1832af`
      glossary + migration doc + tests `6064fde`
      EventService backend            `a363b6d`
      Event Record UI surface         `d09e852`
    #320 children, in completion order:
      #322 per-Event PDF export       `e67bc73`
      #324 Events tab htmx fragment   `966f102`
      #325 attach/detach UI           `5f7dc76`
      #326 slice-3 decomposition      `b6393ec`
      #323 audit/smoke_events.mjs     `97ba644`
      #327 Browse Event branch        `a183114`
      #328 research log               `77d2907`
      #329+#330 sources+scratch panel `0d48fb7`
      #333 tags chips                 `996c17f`
      #337 Timeline sourcing          `512d944`
      #331 re-introduce Event entry   `57e5172`
      #335 Static events[] bundle     `d902217`
      #338 stress tests + benchmarks  `0524113`
      #334 Shared linkedDisplayIds    `11f4b75`
        + #349 variable-shadow follow-up fix
      #332 images gallery + facade    `4abe1a4`
        + `de19e5d` (this branch)

  Out-of-scope v1.1 candidates deferred for future RPCI
  cycles: Set-as-Primary per-Event image, image annotation,
  AI-assisted image tagging, bulk image upload. Browser-
  level smoke coverage of the Event images gallery's native
  file-picker is tracked separately as issue #348.

  Closes #320, #359.

- **Event Records images gallery** (issue #320 child #332,
  slot 16 of 16). The Event Record detail page now carries an
  Images section mirroring the Person Record gallery surface.
  Routes: `GET /events/{id}/images` returns the fragment
  (lazy-load + post-action swap target), `POST
  /events/{id}/images/import` opens the native file picker
  and enqueues an `image_import` background job, and `POST
  /events/{id}/images/delete` re-renders the fragment in place
  after a bulk delete (no `X-DixieData-Redirect`, per issue
  #341). Storage path is the same sharded
  `images/<A>/<B>/EVT-NNNNN/` layout the v60 widening enabled
  for every Person Record subtype — no schema or storage
  changes needed. The 4 routes from the issue spec's apply-
  sites list collapse to 3: the multipart-POST upload route is
  YAGNI (the Person Record `entry_form.templ` uses the native
  dialog import path, not a multipart form) and the
  serve-bytes route is YAGNI (`/media/*` already serves the
  per-image bytes via the `imageURL()` helper). The
  `EventService.AddImage / RemoveImage / ListImages` facade
  methods from the issue spec were also dropped per the
  two-adapter rule (no second caller) — the event handlers
  call `a.soldiers.AddImage / DeleteImages / GetImageByID`
  directly, and `event.Event.Images` is already populated
  for free because `EventService.GetEventByID` delegates to
  `SoldierService.GetByID` which loads images via the
  `imageSelectColumns` query (verified at
  `internal/records/soldier_service.go:264`). Browser-level
  smoke coverage of the import path is deferred to a follow-
  up issue (`#332-children-smoke-images`) because the
  `audit/_lib` Playwright harness does not yet wire a native
  file-picker handler. `TestHandleEventImages` pins the
  end-to-end surface: 2 seeded images render both captions
  + thumbnails + `data-results-target` anchors on the
  fragment, the detail page renders the new section on
  first load, and a POST `/images/delete` with one
  image_id drops that one from the rendered grid AND the
  underlying `images` table row AND does not set
  `X-DixieData-Redirect`.

### Fixed

- **Event Record create + edit forms now expose an inline
  Source Records section** (issue #357). The Event form
  previously rendered only the Event-specific fields (kind,
  dates, description, notes); the Source Records attach UI
  lived only on the detail page. The new section reuses the
  soldier entry form's `RecordInputRow` + `data-record-add`
  + `data-record-list` pattern so a user can attach pension
  / roster / source-transcript rows inline and save them
  with the Event in one submit. Backend: new
  `EventService.AttachSourcesToEvent` batch-inserts into the
  `event_sources` table (issue #340 / v61 — outside
  `replaceRecords`' DELETE scope, so Update does not wipe
  sources); `parseEventForm` returns the parsed
  `[]models.Record` alongside the `models.Soldier` payload
  via the existing `parseRecordInputs` helper; all four
  `parseEventForm` call sites (`handleNewEvent`,
  `handleEventByID` PUT/POST, quick-add from person
  detail) route the parsed sources through the new service
  method after the main write. Empty rows are skipped on
  save. Regression net: new render test in
  `internal/templates/event_form_test.go` asserts the
  form contains the `record_type` / `record_app_id` /
  `record_details` inputs and the `data-record-template`
  hook (edit case); new service test in
  `internal/records/event_service_test.go` pins the batch-
  attach contract; new `step-05b` in
  `audit/smoke_events.mjs` drives the live binary end-to-end
  (fill row → submit edit → assert source visible on
  detail).

### Changed

- **Event detail Source Records panel: legacy attach form
  replaced with Edit Event CTA** (issue #360). The
  `/events/{id}` Sources panel previously rendered a
  standalone `record_type` / `app_id` / `details` form + an
  "Attach Source" button posting to
  `/events/{id}/sources/attach`, plus an empty-state hint
  redirecting users to the `/sources` authoring flow. That
  surface is now redundant with #357's inline
  `RecordInputRow` pattern on `/events/{id}/edit`. The panel
  now exposes a small "Edit Event" link next to the
  `{N} attached` counter (visible for both empty and
  non-empty source lists); the empty-state copy explains the
  new flow ("Add sources from the event editor — open Edit
  Event, paste your source rows, save."). Regression net:
  new
  `TestHandleEventByIDGetDetail_SourcesPanelEditCTA`
  asserts the legacy form fields are absent and the
  `data-action="/events/{id}/edit"` CTA is present;
  `TestHandleEventSourcesAndScratchpad`'s `data-results-target`
  assertion (which pinned the now-removed attach wiring) is
  replaced with the same Edit Event CTA check; new
  `step-04b` in `audit/smoke_events.mjs` drives the live
  binary end-to-end. The `/events/{id}/sources/attach` POST
  route + handler + its attach/detach round-trip test
  (`TestHandleEventSourcesAndScratchpad`) stay reachable
  because the test pins the backend wiring for any future
  programmatic attach path (e.g. .ddshare replay,
  bulk-import) — a follow-up cleanup issue can delete them
  once user-facing attach-only-via-edit sticks.

### Maintenance

- **Event Record images facade** (issue #320 child #332
  close-out). New `EventService.AddImage` +
  `EventService.RemoveImages` thin pass-throughs to
  `SoldierService.AddImage` + `DeleteImages` satisfy the
  `eventsFacade` guard at `internal/appshell/app_facades.go`
  for the per-event gallery surface. The native-dialog
  import gateway (`App.importImagePaths` at
  `internal/appshell/app.go:2480`) still writes via
  `soldiers.AddImage` internally because it serves both
  Person Records and Events from one shared path; that
  single shared import path is now the documented exception
  to the 'handlers MUST NOT call a.soldiers.*' rule. Image
  reads travel via `GetEventByID` (which already populates
  `Event.Images` for free via `imageSelectColumns`), so no
  `ListImages` pass-through is needed per the two-adapter
  rule. No user-visible behavior change. Closes #332.

- **Renamed `selectedSoldierImages` to
  `selectedRecordImages`** in `internal/appshell/app.go`. The
  helper iterates `models.Soldier.Images` to resolve a form's
  `image_ids[]` to absolute paths; post-#320 the soldier name
  is misleading because Person Record is one of three
  subtypes (Soldier / Spouse / Event) and the `images` table
  FK already widened to `person_record_id`. Pure rename, no
  behavior change; `TestSelectedRecordImagesUsesSelectedIDs`
  pins the contract. Required as a prefactor for the Event
  images gallery slice (slot 16) so the new event handler
  can call the same helper without inheriting a stale name.

- **Issue #320 (Event Records v1) sequence closure** (slot
  16 of 16, the final slot). All 16 slots of the #320
  closure sequence have landed on `dev` as of commit
  `4abe1a4`. The schema foundation (slot 1, `d1832af`)
  shipped v60 with the `event_person_links` junction and
  the `person_record_id` FK rename across 8 columns in 6
  tables; the service (slot 2, `a363b6d`) added
  `EventService`; the UI surface (slot 3, `d09e852`) added
  the Event Record detail page; slots 4-13 added Sources,
  Tags, Research Log, Service Timeline sourcing, per-Event
  PDF export, Scratch Pad, and the Browse filter; slot 14
  (`d902217`) shipped the static archive `events[]` bundle;
  slot 15 (`11f4b75`) shipped the shared-archive
  `linkedDisplayIds` denormalization; slot 16 (`4abe1a4`,
  this commit) shipped the Event images gallery. The
  out-of-scope list from the original RPCI spec — Set-as-
  Primary, image annotation, AI-assisted image tagging,
  bulk image upload — remains as the candidate v1.1
  surface. Browser-level smoke coverage of the new Event
  images gallery is filed as follow-up issue #348 (the
  Playwright harness does not yet wire a native file-picker
  handler). Benchmarks at `docs/benchmarks/events.md`
  (commit `69c03d0`); stress tests at
  `tests/stress/events_stress_test.go` (issue #338).

### Added

- **Event Records in the static archive bundle** (issue #320
  child #335, slot 14 of 16). The static archive JSON bundle
  emitted by `ExportStaticArchive` (and read by the embedded
  `index.html` via `window.DIXIE_DATA`) now carries both
  Person Records and Event Records, split into two named
  arrays: `window.DIXIE_DATA = { records: [...], events: [...] }`.
  Previously the bundle was a bare array that conflated both
  row shapes. Per RPCI decision #8, events carry the per-subtype
  fields the rest of the archive surface already used: `Kind`,
  `Description`, `linkedDisplayIds` (the Display IDs of every
  Person Record linked via the `event_person_links` junction,
  resolved via a single-shot SQL in
  `staticArchiveEvents`). The `index.html` JS dispatcher was
  updated to read `.records` instead of the bare array; bare
  archives remain readable until the user re-exports.
  `TestExportStaticArchive_EventBundle` covers the new shape:
  bundle object-shape, `linkedDisplayIds[]` populated for the
  linked event, `[]` for the unlinked event, Person Records do
  not leak into `events[]`.

- **Event CRUD micro-benchmarks** (issue #320 child #338,
  slot 16 of 16). New `internal/records/event_service_bench_test.go`
  pins the hot-path cost for `ListEvents`,
  `AttachEventToPerson`, `DetachEventFromPerson`, and
  `ListForPerson`. Baselines captured in
  `docs/benchmarks/events.md` (commit `69c03d0`, 2026-07-04):
  `ListEventsPagination` = 16.5ms/op over 5000 events,
  `AttachDetachRoundTrip` = 286µs/op, `ListForPerson` =
  2.16ms/op with 100 links. The `PageSize=200 / PageSize=25`
  ratio is 1.22x — well under the 4x threshold the issue
  spec asks for — confirming the OFFSET/LIMIT pagination
  path stays constant-factor on per-row cost.

- **Event CRUD stress tests at volume** (issue #320 child #338).
  New `tests/stress/events_stress_test.go` covers the volume
  scenarios from issue #338 body items 1-4: 10k-event seed +
  pagination sweep @ {25, 50, 100, 200}; 5k-person + 0-10
  events per person attach pass; attach/detach round-trip
  N+1 detection on a 200-iteration loop with a 5ms/iter
  budget; 100-link junction round-trip via `ListForPerson`
  + `LinkCount`. The 10k-event and 5k-person seeds are gated
  on `DIXIEDATA_STRESS_FULL=1` so `make test` (short) stays
  under 5s; `make stress` (scripts/run-stress-tests.ps1)
  sets the env var by default so the heavy seeds are part of
  the closure gate for #320. Two items from issue #338 are
  intentionally out of scope: per-Event PDF p95 < 500ms (item
  5, dominated by Typst cold-start; the audit smoke probe
  #323 covers the end-to-end render path on a single event)
  and Static-Archive 10k-event bundle (item 6, covered by the
  existing `internal/archive` tests + typst-bulk-export
  baseline).

### Maintenance

- **smoke_events.mjs cleanup** (issue #320 child #323 follow-up).
  Steps 7 / 8 / 9 contained defensive re-navigation to
  `/soldiers/{id}/events` after every tab-form POST because the
  underlying handler bug (#345) was forcing the user away. With
  the handler now redirecting correctly, the re-navigation
  became a no-op — replaced with a single `if (!url.endsWith)`
  guard that absorbs any future drift without re-navigating on
  the happy path.

- **Schema migration slice collapsed from 19 blocks to 3** (issue
  #320 closure, post-`origin/stable` reconciliation). The
  `migrations` slice in `internal/db/migrations.go` previously
  carried the full v1-v60 chain as separate blocks (1-17 for
  v1-v53 incremental adds, block-60 for the v60 column
  renames + scratchpad_cache + event_person_links, block-61
  for v61). The chain was broken: block-60 was placed at the
  END of the slice, but several pre-existing blocks (4, 5, 6, 7,
  8, 14) referenced the new column names (`person_record_id`,
  `person_sync_id`) that block-60's renames would have
  introduced — so every v54→v60 upgrade failed at the first
  block that referenced the new names with `no such column:
  person_record_id`. Collapsed to:

  - `block-1-schema-baseline` — the full v61 inline schema
    (every column the deleted blocks would have added
    incrementally, the `system_config` table + the
    `node_prefix='DXD'` seed). Idempotent on fresh installs
    (CREATE TABLE IF NOT EXISTS) and on upgrades (no-op for
    pre-existing tables).
  - `block-60-v54-to-v60-jump` — the only real upgrade path
    (v54 production archives → v60). Runs in one transaction:
    31 ADD COLUMN entries via `applyAddColumnLoop` (covers
    every column a v54-or-earlier archive might lack),
    `applySoldiersNormalization` (7-UPDATE chain normalizing
    pre-v54 values like `pension_state='None'` → `'N/A'`),
    `applyImagesIsPrimary` (with a `columnExists` guard that
    picks `soldier_id` on pre-rename v54 archives +
    `person_record_id` on fresh installs), `scratchpad_cache`
    table create, 4 v60 Event Record columns
    (`kind`/`begin_date`/`end_date`/`description` on soldiers),
    `event_person_links` table + 3 indexes, **12** RENAME
    COLUMN statements (the 8 originally intended + the 4
    missing `soldier_sync_id` → `person_sync_id` ones the v60
    author forgot), `applyPhase1DistributedMerge` backfill
    (sync_ids + node_id — runs AFTER the renames so the UPDATEs
    hit the new names), and `ensureSoldierFTS` (soldiers_fts
    virtual table + 6 triggers). `Irreversible` per the
    conservative rule (user-added Event data would be lost on a
    v60→v54 reverse).
  - `block-2-event-sources` — the v61 table (issue #340).

  Per the user's observation that v54 is the production-stable
  schema on `origin/stable` (`CurrentSchemaVersion = 54`), the
  v1-v53 chain was a code path no real production archive has
  data on — it was deleted. `origin/stable` users upgrading to
  `dev` were the user-visible failure case: the v54
  `dixiedata-backup-2026-05-30.ddbak` in the repo root (501
  soldiers + 1683 records) now imports cleanly via
  `TestBackupService_ImportSeededArchiveRoundTrip`.

  Per the user's "don't rig dead-weight tests to pass"
  feedback, removed 2 tests that pinned a deleted
  `birth_info` → `birth_date` extraction path (the
  `migrateCanonicalDateData` block-13 work, gone with the
  collapse). Those tests created a synthetic v1-shape
  fixture (already using the v60 column names) and asserted
  a v1→v60 birth_info parse. No real production archive has
  that shape; the meaningful regression net is the v54
  production archive test (which exercises the real path).

  - `internal/db/schema.go` — full v61 inline schema +
    `applyPhase1DistributedMerge` extracted to a function
    + `applyImagesIsPrimary` gains a `columnExists` guard +
    `node_prefix` seed lives in the inline const.
  - `internal/db/migrations.go` — 19-block slice → 3 blocks.
    Helper functions for the deleted blocks (applySoldiersNormalization
    forward direction, etc.) are now invoked from
    block-60; reverse-direction helpers (reverseSoldiersNormalization,
    reverseAddColumnLoop) are kept for the future DOWN runner
    but not currently called.
  - `internal/db/migrations_test.go` — `TestMigrationsCatalogueIsOrdered`
    + `TestMigrationsReversibilityMapping` +
    `TestReversibilityIrreducibleCount` updated for the 3-block
    shape (count=1, mapping = `Reversible/Irreversible/Reversible`).
  - `internal/db/migrate_down_test.go` — `TestRetainedBackupDirectionDiscriminator`
    updated for the new 2-snapshot reality (was 1-snapshot
    under the v1-v60 chain because the second open saw a
    v1 schema that was post-rename relative to the slice; now
    the second open creates a v1→v61 snapshot in addition
    to the v0→v61 snapshot the first open created).
  - `internal/appshell/cli_admin_test.go` — `TestRunAdminMigrateDown_ManifestPrinted`
    updated to expect `block-60-v54-to-v60-jump` (the new
    Irreversible block) instead of `block-17-research-log-evidence-rename`.
  - `internal/archive/backup_service_test.go` — removed
    `TestBackupService_ImportSharedBackupMigratesLegacySQLite`
    (pinned the deleted `birth_info` extraction).
  - `internal/archive/distributed_merge_test.go` — removed
    `TestBackupService_ImportSQLiteBackupMigratesSchema`
    (pinned the deleted `birth_info` extraction) + cleaned
    up unused imports.

### Fixed

- **Shared-archive import silently dropped every Event Record +
  its `event_person_links` junction** (issue #320 child #334,
  slot 15 of 16). Two compounding bugs in the new
  `mergeSharedEvents` helper:
    1. **Variable shadow** — `if err := row.Scan(&targetID); ...`
       declared a fresh `err` inside the if-init scope, then the
       `if err == sql.ErrNoRows` branch below consulted the OUTER
       `err` from `tx, err := b.db.Conn().Begin()` (always nil),
       so every event took the "skip-existing" branch and was
       never inserted.
    2. **Wrong lookup key for the junction** — the link loop
       resolved the imported Person Record by `display_id`, but
       `mergeSharedSoldiers` rewrites `display_id` to the
       recipient's own node prefix (issue #183's user-identity
       binding; `ESU00-00001` becomes `IRU01-00001` on the
       recipient). `sync_id` is the immutable cross-archive
       identifier; switched to `WHERE sync_id = ?`.
  Together these produced the symptom the handoff captured:
  `EventsInserted=0, EventsSkipped=2, EventsLinked=0` despite a
  clean tx commit. The new test
  `TestBackupService_ImportSharedBackupWithEvents` pins the
  round-trip end-to-end: source seeds 1 Person + 2 Events + 1
  `event_person_links` row, exports the shared archive, imports
  into a fresh recipient DB, and asserts `SoldiersInserted=1`,
  `EventsInserted=2`, `EventsLinked=1`, plus the recipient-side
  `EventService.ListForPerson` returns the battle linked to the
  Person Record. The lookup test probe that initially looked up
  the recipient Person by source-side `display_id` (which is
  rewritten on import) was also corrected to match by `sync_id`.

- **Event PDF export silently produced a 0-byte file in web-mode**
  (issue #347, found by the Events audit smoke probe #323). The
  Wails debug build path (`scripts/build-debug.ps1` →
  `Restore-DixieDataTypstAssets`) bundled `templates/*.typ` into
  `build/bin/templates/` next to `DixieData.exe`, but the plain
  `make web` (which builds `cmd/dixiedata-web`) skipped that step.
  The web binary booted, but the typst walker accepted
  `build/bin/templates/` via the `soldier_landscape.typ`
  sentinel, then failed to compile `event_landscape.typ` (missing
  from the bundle). Fix: new `scripts/bundle-web-assets.ps1`
  helper that copies the typst binary + source templates into
  `build/bin/` and verifies all source `*.typ` files made it
  across (idempotent; guard surfaces any future template drop).
  `make web` chains the helper after the go build. Smoke step 11
  now exits 0 against the web-mode binary.

- **Person Events tab forms redirected away from the Events tab**
  (issue #345, found by the Events audit smoke probe #323). The
  attach / unlink / quick-add handlers under
  \`/soldiers/{id}/events/\` returned an \`X-DixieData-Redirect\`
  of \`/soldiers/{id}\` after success, throwing the researcher
  out of the Events tab on every click. Fix: redirect target is
  now \`/soldiers/{id}/events\` for all three success paths in
  \`internal/appshell/events_handlers.go\` (lines 299, 315, 393).
  The duplicate-link path of \`handleQuickAddEvent\` also routes
  back to the Events tab instead of an Event detail. Existing
  \`TestHandleQuickAddEvent\` test updated to pin the new
  redirect target.

- **POST /soldiers/new returned 405** (issue #346, found by
  the Events audit smoke probe #323). \`handleNewSoldier\` in
  \`internal/appshell/soldiers_handlers.go\` only honored GET
  even though routes.go registers both verbs \u2014 the chi route
  was happy but the handler gate 405'd every POST. Fix: POST
  delegates to \`handleCreateSoldier\` so \`/soldiers/new\` is a
  valid alias for \`/soldiers\` (matches the canonical
  \`/events/new\` + POST pattern from issue #320). Test added:
  \`TestHandleNewSoldierPostDelegatesToCreate\`.

### Added

- **Event Records browser smoke probe** (issue #320 child
  #323, slot 14 of 16). New `audit/smoke_events.mjs` walks
  the full Event Records user journey against a spawned
  `build/bin/dixiedata-web.exe` instance in a private
  scratch dir (mirrors the `probe-full-restore.mjs` pattern
  + `_lib/cleanup.mjs` process-leak hardening). 11 steps:
  empty list → click "+ Add Event Record" → fill new form
  → assert `page.url()` ends in `/events/{numeric_id}` →
  detail shows values → edit round-trip → Person Events
  tab fragment → attach existing event by Display ID →
  quick-add new event → unlink → delete (303 to `/events`
  + row gone) → `/events/{id}/pdf` PDF export lands in the
  SaveFileDialog override dir. Selector strategy is CSS /
  aria / placeholder (no new `internal/uiids` UIIDs).
  Failure mode prints `page.url()`, last response status,
  and a 2000-char DOM snippet to stderr. Re-runnable: in-
  script teardown deletes every event the probe creates, the
  scratch dir is wiped on exit. Probe-surfaced regressions
  fixed in this commit:

    - `internal/appshell/events_handlers.go` —
      `handleEventByID` switch now shares the update-by-form
      path between POST + PUT (was PUT-only; POST was a 405).
      Routes.go already declared both verbs; the handler just
      had the case missing. Test added:
      `TestHandleEventByIDPostUpdate`.
    - `internal/templates/event_detail.templ` — Delete Event
      button now carries `data-method="DELETE"` so the JS
      Option C dispatcher issues `fetch(... { method:
      "DELETE" })` instead of falling back to POST.

  Probe also surfaced 3 follow-up gaps that warrant their
  own issues (out of scope here, recorded for triage):

    - The `/soldiers/{id}/events/attach|detach|quick-add`
      handlers redirect to `/soldiers/{id}` (the Person
      detail) instead of `/soldiers/{id}/events` (the tab).
      User-visible: clicking Add existing / Quick-add +
      Link throws the researcher away from the Events tab.
    - `POST /soldiers/new` returns 405 — `handleNewSoldier`
      only honors GET; the create handler is mounted at
      `/soldiers` via the JS dispatcher.
    - `templates/event_landscape.typ` is not bundled into
      `build/bin/templates/` by the Wails debug build path.
      `cmd/dixiedata-web` boots but the per-Event PDF render
      silently fails (file written, 0 bytes). Smoke step 11
      surfaces this; tests in the appshell package pass
      because Go tests resolve `templates/` from cwd.

### Maintenance

- **TDD protocol** (`docs/agents/tdd.md`). New agent-facing
  doc that anchors every slice to a failing acceptance test
  written BEFORE the slice code lands. Targets three failure
  modes that shipped in 2026-07 fixes: modal invoker wiring
  that silently early-returns (`d5541a7`), adjacent-state
  races the slice's own smoke probe misses (`d8f73b7`), and
  orphan-handler / fragment-as-redirect drift caught only by
  post-merge audit probes (`d3e0a02`). The protocol sits
  inside the vertical-slice discipline from
  `feature-protocol.md`; it does not replace it. Wiring: a
  6th pre-flight checklist item in `feature-protocol.md`,
  a new step 1.5 (RED test before GREEN change) in
  `rpci.md` §I, and Tier-0 status in `INDEX.md`. No new
  test harness — uses the repo's existing testify handler
  tests, `bytes.Buffer`+`Render` templ tests, and
  `audit/smoke_*.mjs` Playwright probes.

### Added

- **Per-Event PDF export** (issue #320, slice #322). New `GET
  /events/{id}/pdf` route renders the Event Record card via
  the typst-backed Registry (new `templates/event_landscape.typ`
  template, `record_types: [event]`). Routes through
  `ExportService.ExportEventPDF(outputPath, event, linked)`;
  the slim per-Person projection for the "Linked Person
  Records" table is pre-computed in the handler so the
  typst template stays DB-free. UI: a new "Export PDF"
  button on `event_detail.templ` posts to the route via
  the existing `dispatchDixieDataForm` flow.
  `pdf_excerpt_override` takes precedence over `description`
  when set (D3); otherwise the long-form Description is
  rendered. `eventPDFName` returns `Event-<DisplayID>.pdf`.
  Tests: `TestHandleEventPDF` (handler end-to-end with
  `saveFileDialogOverride` test seam) and
  `TestExportService_ExportEventPDF` (registry path
  through `extractPDFText`). Files: 5 new + 7 modified.
- **Person Record → Events tab fragment** (issue #320, slice
  #324). `GET /soldiers/{id}/events` now returns an htmx
  fragment (table of linked Event Records — Display ID +
  Kind + Date Range, D2/D5 of #322) instead of the slice-3
  303 redirect to the Person Record detail page. New
  `internal/templates/person_events_tab.templ`; new
  `presentation.PersonEventsTab` adapter; finished the
  half-built `handlePersonEventsTab` handler from slice 3
  (the body was previously empty — see `events_handlers.go`
  history). Test: `TestHandlePersonEventsTab` walks the
  full attach-then-fragment round-trip. UI integration on
  `soldier_card.templ` is deferred to a follow-up — the
  fragment is reachable via direct URL today.
- **Person Record → Events tab controls** (issue #320, slice
  #325). The lazy-loaded `person_events_tab.templ` fragment
  now exposes the three controls the slice-3 handlers were
  waiting for: (a) an "Unlink" form per linked Event row
  that POSTs to `/soldiers/{id}/events/{eventId}/detach`;
  (b) a collapsible "Add existing event" form with a
  single `display_id` field that POSTs to a new route
  `/soldiers/{id}/events/attach-by-display-id` — the
  handler resolves the Event by Display ID via
  `events.GetEventByDisplayID` and delegates to the
  existing `handleAttachEvent` for the duplicate-link
  and not-found paths; (c) a collapsible "Quick add new
  event" form with kind + begin/end date + description
  that POSTs to the existing `/soldiers/{id}/events/
  quick-add` route (no change to that handler).
  Empty-state copy now points researchers at the new
  controls instead of the slice-3 fallback. Form style
  mirrors the tags block (`<details>` + `data-dixie-
  submit` + `data-reload-on-success`).
  Tests: `TestHandlePersonEventsTabUnlink`,
  `TestHandleAttachEventByDisplayID` (success +
  duplicate + not-found + empty-validation), and
  modified + 4 (routebuilder + handler + route +
  CHANGELOG).
- **Slice 3 decomposition reference (issue #320, slot #326)**.
  The slice-3 foundation commit `d09e852` (18 files,
  1491 insertions) is now documented as 8 logical route
  groups in `docs/agents/notes/slice3-decomposition.md`.
  Seven mbox-clean patches in
  `docs/agents/notes/slice3-patches/` reconstruct the
  slice-3 source tree from `a363b6d` (slice-3 parent)
  — `git am`-ing them in order produces a tree matching
  `d09e852` bit-for-bit (verified via worktree). The
  alternative `git rebase -i a363b6d` strategy would
  rewrite four published children (#322, #339, #324,
  #325), so it was deferred to preserve the audit trail;
  the patches realize the decomposition intent without
  destructive force-push. Chunk 5 is documented in the
  notes file but not emitted as a separate patch because
  chunks 4 and 5 share `events_handlers.go` + `routes.go`.
- **Browse filter Event branch routes Events correctly**
  (issue #320, slot #327). The browse entry-type filter
  dropdown already had an "Event" option (slice #320.7),
  and `BrowsePage`'s SQL predicate already filtered by
  `LOWER(TRIM(entry_type)) = ?` — Events were being
  returned, but their row URLs pointed at
  `/soldiers/{id}` (which 404s for Events). New helper
  `recordBrowseURL(record)` in `browse.templ` returns
  `/events/{id}` for Events and `/soldiers/{id}` for
  everything else; four call sites updated (mobile
  card title link, mobile card "View →" link, table-row
  `data-browse-row-href`, table name link). Test
  `TestHandleBrowseEventsFilter` covers the round
  trip: seeds one Event + one Soldier, GETs
  `/browse?entry_type=event`, asserts the Event's
  Display ID is in the body, the `/events/{id}`
  row URL is present, and the Soldier's name is
  absent.
- **Per-Event research log** (issue #320, slot #328).
  `GET  /events/{id}/research-log`,
  `POST /events/{id}/research-log/tasks`,
  `POST /events/{id}/research-log/tasks/{entryId}/resolve`.
  The handler dispatches on `r.URL.Path` suffix, with
  `handleEventResearchTaskCreate` and
  `handleEventResearchTaskResolve` mirroring the
  Person-Record handlers but re-aiming the redirect
  header at the Event-side URL (`/events/{id}/research-log`).
  Three `routebuilder.EventResearchLog*` helpers
  added; chi route shim uses parts-by-index parsing so the
  trailing `/tasks/...` segments survive. New
  "Research Log" pill-button on `event_detail.templ`
  links to the new URL. Service layer deliberately NOT
  re-shaped — `research_tasks` is FK-linked to
  `soldiers(id)`, so `a.soldiers.ResearchLog(eventID)` /
  `AddResearchTask` / `ResolveResearchTask` already
  work on Event rows; the only seam is the redirect
  URL. Test `TestHandleEventResearchLog` covers the
  full round trip (GET → POST create → GET (title in
  body) → POST resolve → service confirms resolved).
- **Re-introduce Event option in entryTypes()** (issue
  #320, slot #330). The `Event` option is back in the
  `/soldiers/new` entry-type dropdown. The JS-side
  `syncEntryTypeFields` swaps the form action URL to
  `/events/new` when Event is selected; server-side
  `handleCreateSoldier` short-circuits to
  `handleNewEvent` as a defensive guard for hand-curled
  posts. Soldier/Wife/Widow/Person subtypes still
  submit to `/soldiers/new` — no regression.
  Test `TestHandleCreateSoldierDispatchesToNewEvent`
  covers the dispatch path.
- **Service Timeline from linked Event Records** (issue #320,
  slot #337). `SoldierService.ServiceTimeline` now joins
  `event_person_links` for the central soldier and pushes a
  Timeline Marker per linked Event. Date sources from the
  Event's `begin_date` (falls back to `end_date` when begin
  is empty); missing dates are skipped. Marker carries
  `kind` as title prefix, Event `description`, and Event
  Display ID as the source label. Existing record-derived
  markers stay intact. Test
  `TestSoldierService_ServiceTimelineIncludesLinkedEvents`
  covers the round trip.
- **Per-Event Tags chips** (issue #320, slot #333). New
  Event routes: `GET /events/{id}/tags` (renders the chip
  fragment), `POST .../tags` (adds a `person_record_tags`
  row), `POST .../tags/{tagId}/detach` (removes the row).
  `EventService` gains `ListTagsForEvent`,
  `AddTagToEvent`, `DetachTagFromEvent`; the `eventsFacade`
  mirrors the three entries. The same `person_record_tags`
  junction covers both Person + Event tags since v60's
  `person_record_id` rename applies to Events too.
  `event_detail.templ` renders a "Tags" section listing
  chips with detach buttons. Test:
  `TestHandleEventTags` round-trips add→detach.
- **Per-Event Sources panel** (issue #320, slot #329). New
  Event routes: `GET /events/{id}/sources` (renders the
  fragment), `POST .../sources/attach` (inserts a `records`
  row keyed by `person_record_id`), `POST
  .../sources/{sourceId}/detach` (removes the row). The
  `EventService` gains `ListSourcesForEvent`,
  `AttachSourceToEvent`, `DetachSourceFromEvent`; the
  `eventsFacade` interface mirrors the three entries. UI:
  a new "Source Records" section on `event_detail.templ`
  lists attached rows with `app_id` + `record_type` +
  `details` and an inline `<form>` post-back to
  `.../sources/attach`. Test:
  `TestHandleEventSourcesAndScratchpad` round-trips
  attach→detach against a fresh Event. Files: 1 new + 5
  modified.
- **Per-Event Scratch Pad pill-button** (issue #320, slot
  #330). Event detail page now renders an "Open Scratch
  Pad" pill-button that posts to `/scratchpad/open` with
  the Event's `display_id`. The existing
  `handleScratchpadOpen` route is display-ID-agnostic;
  the native launcher creates a per-Event scratch pad file
  under `.dixiedata/scratchpads/` using the `EVT-NNNNN`
  display id as the stem. No service-layer change
  required — `a.database.Scratchpad(displayID)` /
  `SaveScratchpad(displayID, content)` already key on
  display id. Test:
  `TestHandleEventSourcesAndScratchpad` asserts the
  button + input render with the Event's display id.
- **Page indicator + dev badge + JS debug toolbox** (issue #309).

  Three independent witnesses for "what page am I on", each
  visible/accessible to a different audience:
  - **Always-visible breadcrumb** rendered between the top-nav
    header and `<main>` on every page. Maps the URL path to a
    chain of crumbs via the Go helper
    `components.BreadcrumbCrumbs()`; clickable crumbs navigate
    to parent pages. Last crumb has `aria-current="page"`.

  - **Floating dev badge** in the bottom-right corner, gated by
    `debug.IsDebugMode(ctx)` (the same gate that controls the
    existing 🐞 Debug footer button -- issue #309 piggy-backs
    on the existing debug-mode infrastructure instead of
    inventing a new env var). Hidden by default with
    `class="hidden"`; `installDixieDebugToolbox()` JS removes
    the hidden class on `window.DIXIEDATA_DEVTOOLS === true`
    (the layout injects this from
    `debug.IsDebugMode(ctx)` in the head `<script>`). Shows the
    URL path + `data-dixie-page` attribute for cross-checking.

  - **JS debug toolbox** at `window.dixie.*`, callable from
    devtools, all read-only. Nine functions: `page()`,
    `queue()`, `lastNetwork(n?)`, `activity()`, `settings()`,
    `storage()`, `errors()`, `route(path)`, `help()`. Network
    log is fed by a `fetch` wrapper; errors come from a global
    `error` + `unhandledrejection` listener. Loaded as a
    separate script (`/debug-toolbox.js`).

  **Behavior preserved:** the breadcrumb + dev badge + toolbox
  always agree. If they disagree, the layout is broken. The
  `data-dixie-page` attribute on `<body>` is the single source
  of truth for the page identity; the breadcrumb reads it, the
  badge reads it, `dixie.page()` reads it. The Go algorithm in
  `internal/templates/components/breadcrumb_helpers.go` is
  mirrored 1:1 in JS at `frontend/debug-toolbox.js`; the test
  `TestBreadcrumbCrumbs_KeyRoutes` pins both halves together
  (sync point for future changes).

  - **New files:**
    - `frontend/debug-toolbox.js` (~470 lines: 9 functions +
      install + network/error capture).
    - `internal/templates/components/breadcrumb.templ` (new
      component, ~35 lines).
    - `internal/templates/components/breadcrumb_helpers.go`
      (~290 lines: path -> crumbs algorithm + helpers).
    - `internal/templates/components/breadcrumb_test.go` (24
      test cases pinning the algorithm).
    - `internal/templates/components/dev_page_badge.templ`
      (new component, ~40 lines).
    - `internal/templates/components/dev_page_badge_test.go`
      (3 badge + 1 breadcrumb tests).
    - `internal/templates/layout_helpers.go` (the
      `currentPagePath` package-level state + Set/Get helpers;
      avoids threading a `currentPath` arg through 30+ page
      templ signatures).

  - **Modified files:**
    - `internal/templates/layout.templ` — `<script>` injects
      `window.DIXIEDATA_DEVTOOLS`; `<body>` gets
      `data-dixie-page`; breadcrumb rendered between header
      and main; dev badge rendered in footer (dev-only).
    - `internal/appshell/lifecycle.go` — `ServeHTTP` calls
      `templates.SetCurrentPagePath(r.URL.Path)` +
      `defer templates.ClearCurrentPagePath()` so the package-
      level currentPath is set for every full-page render and
      reset after.
    - `internal/appshell/routes.go` — new route
      `/debug-toolbox.js` serves the file.
    - `frontend/app.js` — `installDixieDebugToolbox()` called
      on `DOMContentLoaded`; badge unhides when
      `window.DIXIEDATA_DEVTOOLS === true`; new htmx
      `htmx:afterSwap` handler updates breadcrumb + badge +
      body data-dixie-page after full-page navigations.
    - `CHANGELOG.md` — this entry.

  **No new Go packages**, **no new npm packages**.

  **Regression net:**
  - `make test` green across all 30 packages.
  - `internal/buildinfo` doc-coverage floor test still upholds
    the 70% rule.
  - New tests: `TestBreadcrumbCrumbs_KeyRoutes` (24 cases),
    `TestBreadcrumbCrumbs_StripsQueryString`,
    `TestBreadcrumbCrumbs_UnknownRoute`,
    `TestDevPageBadge_RendersWithPath`,
    `TestDevPageBadge_DefaultsHidden`, `TestBreadcrumb_Renders`.

  **Manual smoke:** run `make debug` and visit each page;
  the breadcrumb should always show `Home > <Section> > <Leaf>`
  (or the right shape for the path); the dev badge should
  appear in the bottom-right; `window.dixie.page()` in devtools
  returns the same path + crumb chain the breadcrumb shows.

### Changed

- **Build Share Archive button + Share Queue pill now navigate to
  `/share/queue`** (issue #310, PR 1 of 3). Both were previously
  `<button data-share-queue-open>` / `<button data-share-queue-pill>`
  that opened the Share Build modal — a strict subset of what the
  full `/share/queue` management page offers. They are now `<a
  href="/share/queue">` styled to keep their visual surface. The
  pill still toggles visibility + count via `updateShareQueuePill()`
  using the `data-share-queue-pill` attribute; only the click
  target changed. The Share Build modal still exists for now; PR 2
  deletes it, PR 3 ports the Saved Queues presets to the page.

  - `internal/templates/share_exports.templ` — button → link,
    text "Build Share Archive" → "Open Share Queue", tooltip
    updated. `routebuilder` added to imports.
  - `internal/templates/layout.templ` — pill button → pill link.
  - `frontend/app.js::installShareQueueGlobals()` — removed the
    two modal-open click handlers; kept the `data-share-queue-add`
    delegation + `installShareQueuePage()`.
  - `internal/appshell/share_subpages_handlers_test.go` — updated
    `TestShareExportsSubpage_Renders` to assert the new button
    text + `href`, and to no-longer-find the old data attribute.

- **Share Build modal deleted entirely** (issue #310, PR 2 of 3).
  After PR 1 left the modal as a dead route, this PR removes the
  templ file, the handlers (`handleShareQueueModal`,
  `handleShareQueuePreview`, `handleShareQueueClear`), the
  routes (`GET /share/queue/modal`, `POST /share/queue/preview`,
  `POST /share/queue/clear`), the route-builder constants
  (`ShareQueueModal`, `ShareQueuePreview`, `ShareQueueClear`),
  the modal-only JS (`shareQueueModal`, `loadShareQueueModal`,
  the issue #308 lazy-load fix, `openShareQueueModal`,
  `installShareQueueModal`, the modal `refreshShareQueueModal`,
  `refreshShareQueuePreview`, `clearShareQueue`), the Saved
  Queues JS (`shareQueuePresetStatus`, `saveCurrentQueueAsPreset`,
  `loadShareQueuePreset`, `deleteShareQueuePreset`,
  `refreshShareQueuePresets` — all ported onto the page in PR 3),
  7 modal-only Go tests, and the two `uiids.ID` constants
  (`OverlayShareQueue`, `PanelShareQueuePreview`) that no
  longer apply. `addToShareQueue` / `removeFromShareQueue`
  retained and rewritten to update `updateShareQueuePill()`
  instead of refreshing the modal.

  Net removal: ~340 lines of JS, 70 lines of templ, ~240 lines
  of Go (handlers + tests + route-builder). The /share/queue
  page (issue #193) absorbs every action the modal used to
  provide. PR 3 ports Saved Queues onto the page so the user
  can save / load / delete preset queues from the same surface.

  - `internal/templates/share_queue_modal.templ` (+ `_templ.go`)
    — deleted.
  - `internal/appshell/share_queue_handlers.go` — deleted 3
    handlers + 1 helper (`buildShareQueuePreviewFragment`).
    Top-of-file doc-comment rewritten.
  - `internal/appshell/routes.go` — deleted 3 routes + their
    "share/queue/* static-before-wildcard" comment block.
  - `internal/appshell/route_wildcard_test.go` — dropped the
    `/share/queue/modal` shadow pair (the route no longer
    exists).
  - `internal/routebuilder/routebuilder.go` — deleted 3
    constants.
  - `frontend/app.js` — net -334 lines (cache vars, query
    function, lazy-load, modal refresh, preset functions,
    openShareQueueModal, installShareQueueModal).
  - `internal/appshell/share_queue_handlers_test.go` — deleted
    5 modal-only tests + `seedPersonRecordWithCounts` helper.
  - `internal/uiids/uiids.go` — deleted `OverlayShareQueue` +
    `PanelShareQueuePreview`; updated Registry descriptions
    for the retained `PanelShareQueueList` and
    `PanelShareQueuePresets` to say "Share Queue page" rather
    than "Share Build modal".

- **Saved Queues presets ported onto the `/share/queue`
  management page** (issue #310, PR 3 of 3, completes the
  fold). The presets card that lived on the Share Build modal
  is now a `<section id="panel.share-queue.presets">` between
  the page header + the queue table. Save form (name input +
  Save button), preset list (Load / Delete per row), empty
  state, and the status message slot all live on the page now.
  The five preset JS functions deleted in PR 2 are re-added as
  `shareQueuePresetStatusPage`, `saveCurrentQueueAsPresetPage`,
  `loadShareQueuePresetPage`, `deleteShareQueuePresetPage`,
  `refreshShareQueuePresetsPage` + a new
  `installShareQueuePresetsPage` installer; they query the
  page panel via `[data-share-queue-preset-list]` /
  `[data-share-queue-preset-status]` and reuse the same
  `/share/queue/presets` JSON endpoints that the modal called.
  No Go-side changes. The user's mental model is now: "open
  `/share/queue` to stage, save, load, and export — everything
  happens on one page."

  - `internal/templates/share_queue.templ` — added the Saved
    Queues card (line ~36) using `uiids.PanelShareQueuePresets`.
    Doc-comment updated to mention PR 3.
  - `frontend/app.js` — +190 lines (5 page-scoped preset
    helpers + installer + `ShareQueuePresetsSectionID`
    constant). `installShareQueueGlobals()` calls
    `installShareQueuePresetsPage()` after
    `installShareQueuePage()` so the card hydrates on every
    page load.
  - `internal/appshell/share_queue_handlers_test.go` —
    `TestShareQueuePage_Empty` now also asserts the Saved
    Queues card presence (`Saved Queues` heading,
    `[data-share-queue-preset-save]` form, etc.) so a future
    regression that drops the card fails the test.
  - `internal/appshell/share_queue_presets_handlers.go` +
    `internal/records/share_queue_presets.go` — UNCHANGED.
    PR 3 consumes the same JSON endpoints that the modal
    used; no service-side changes needed.

### Fixed

- **Editing an Event silently wiped every attached Source
  Record** (issue #340, found by audit sweep). Root cause:
  v60 slot #329 reused the shared `records` table for
  per-Event sources because the `event_source_links` M-to-M
  schema blocker was unsolved. `SoldierService.Update` calls
  `replaceRecords(tx, id, ...)` which `DELETE`s every row
  where `person_record_id = id` and re-inserts from
  `soldier.Records`. The Event edit form has no records
  input, so `parseEventForm` returned an Event with
  `Records: nil` and every Update wiped every attached
  source. Fix: schema v61 adds a dedicated `event_sources`
  table keyed by `event_id`; `EventService.{List,Attach,
  Detach}SourcesForEvent` migrate to read / write it;
  `Soldier.EventSources []models.Record` is the new
  read-side projection (Person Records always leave it
  empty); `viewmodel.PersonRecord.EventSources` + the
  `event_detail.templ` Sources panel read from there.
  Regression net: new tests
  `TestEventService_UpdateEventPreservesAttachedSources`
  (the test that would have caught the bug — attaches a
  source, runs an Update, asserts it survives),
  `TestEventService_SourceRoundTripOnEventSourcesTable`,
  `TestEventService_GetEventByIDReturnsEventSourcesField`.
  Existing `TestHandleEventSourcesAndScratchpad` stays
  green. Schema: `internal/db/schema.go` (inline + new
  block-61 migration in `migrations.go`); the inline v60
  `records`-table reuse is removed end-to-end.
  Decomposition: `docs/agents/notes/v61-event-sources-decomposition.md`
  and bug repro `docs/agents/notes/v61-bug-repro.md`.
  Files: 12 modified across 6 atomic commits
  (db + versioninfo + records + models + viewmodel +
  templates + tests).
- **Per-Event Sources / Tags panels navigated to a raw
  fragment URL on attach / detach** (issue #341, found by
  audit sweep). Root cause: the POST handlers for
  `attach` / `detach` set `X-DixieData-Redirect` pointing at
  the per-panel GET endpoint, and the GET endpoints
  returned raw fragment HTML. `dispatchUtilitySubmit`
  reads `X-DixieData-Redirect` and runs
  `window.location.assign(...)`, so the browser landed on
  a fragment URL and displayed raw HTML as a full page
  (the orphan-handler probe flagged the GETs as orphans,
  which was the diagnostic trail). Fix: the Sources +
  Tags GET endpoints and POST handlers now render via the
  shared `EventSourcesListFragment` / `EventTagsListFragment`
  templ helpers (matching the on-page render), the
  `event_detail.templ` attach form + tag detach buttons
  carry `data-results-target="#data-event-sources-list"`
  and `#data-event-tags-list`, and the POST handlers no
  longer set `X-DixieData-Redirect` so the JS dispatcher
  swaps the response body into the matching div in place.
  Tests: `TestHandleEventTags` + `TestHandleEventSourcesAndScratchpad`
  assert no `X-DixieData-Redirect` header on POST and that
  the response body matches the swap target shape.
  Files: 1 new (`internal/templates/event_panels.templ`),
  3 modified (templ + handler + test).
- **Dev badge invisible on `make debug` runs** (issue #309
  follow-up discovered during smoke). Root cause:
  `appshell.App.debugMode` was seeded solely from
  `records.LoadLocalSettings.DebugMode` (default OFF on fresh
  install). The env var `DIXIEDATA_DEBUG=1` was honored by
  `internal/debug.log.debugMode` but never reached
  `appshell.App.debugMode`, so `debug.IsDebugMode(ctx)`
  returned false and the new `@components.DevPageBadge(...)`
  branch in the layout never rendered. Fix has two parts:
  `scripts/build-common.ps1` now defaults `DIXIEDATA_DEBUG=1`
  (alongside the existing `DIXIEDATA_DEVTOOLS=1`) so the
  `make debug` launcher enables debug mode; the seeding block
  in `(*App).startup()` now OR's in the env via the new
  `decideDebugModeAtStartupSettings()` helper so a launcher-
  set env override beats a persisted OFF toggle.

  - `internal/appshell/lifecycle.go` — new package-private
    helper `decideDebugModeAtStartupSettings(bool)` extracted
    from the seeding block; existing block now calls it.
  - `internal/debug/log.go` — `envBool` is now wrapped by an
    exported `EnvBool` so callers outside `internal/debug` can
    share the same boolean-parsing rules.
  - `scripts/build-common.ps1` — the Run-DixieData-Debug.ps1
    launcher now defaults `DIXIEDATA_DEBUG=1` if not already
    set (mirrors the existing `DIXIEDATA_DEVTOOLS=1` line).
  - `internal/appshell/debug_mode_env_test.go` (NEW, 3 tests)
    pins the env-vs-settings contract.
- **Dev badge still invisible despite `e3eb552` env seeding fix**
  (issue #309 follow-up #2). Root cause: the head `<script>`
  in `internal/templates/layout.templ` used `{ debug.IsDebugMode(ctx) }`
  (single-brace Go expression syntax) inside a `<script>` block
  -- but templ uses `{{ value }}` (double-brace JS-interpolation
  syntax) inside script tags; single braces are emitted VERBATIM.
  Result: the rendered HTML was
  `window.DIXIEDATA_DEVTOOLS = { debug.IsDebugMode(ctx) };`
  -- an object literal evaluating to a junk object, NOT a bool.
  `window.DIXIEDATA_DEVTOOLS === true` always false; the JS
  badge-unhide branch never ran. Fixed to `{{ debug.IsDebugMode(ctx) }}`
  per templ docs. Regression net: new test
  `TestLayout_DevBadgeRendersOnlyWhenDebugMode` in
  `internal/templates/layout_dev_badge_render_test.go` renders
  Layout() under 3 ctx states (debug-on, debug-off, untagged)
  and asserts the rendered HTML actually contains
  `DIXIEDATA_DEVTOOLS = true` (or `false`) -- not the literal
  source syntax.

  - `internal/templates/layout.templ` -- head `<script>` uses
    `{{ debug.IsDebugMode(ctx) }}` instead of the wrong
    `{ debug.IsDebugMode(ctx) }`. Added a comment block
    explaining the templ syntax asymmetry so a future reader
    doesn't reintroduce the bug.
  - `internal/templates/layout_dev_badge_render_test.go` (NEW,
    3 sub-tests) -- the regression test.
- **Dev badge moved from bottom-right to bottom-left**
  (issue #313). The original `bottom-3 right-3` anchor
  overlapped the floating-dock `Menu` button (the Quick Nav
  entry point). Moved to `bottom-3 left-3` -- the persistent
  share-queue pill is bottom-center at `bottom-[6.5rem]` and
  the floating-nav panel opens at `bottom-[5.5rem] right-4`,
  so the left-of-center bottom is clear.

  - `internal/templates/components/dev_page_badge.templ` --
    `right-3` -> `left-3`. Comment block updated to explain the
    position rationale + reference issue #313.
  - `internal/templates/components/dev_page_badge_test.go` --
    `TestDevPageBadge_DefaultsHidden` now asserts
    `class="... bottom-3 left-3 ...` so a future regression
    to the right-3 anchor fails the test.

### Removed

- **Share Build modal at `/share/queue/modal`** (issue #182,
  delete via #310 PR 2). The modal was a strict subset of the
  `/share/queue` page and was never rendered after issue #284
  split `/share` into subpages; issue #308 re-enabled it via
  lazy-fetch, but the modal is now gone entirely. All trigger
  surfaces (Build Share Archive button on `/share/exports`,
  persistent Share Queue pill) navigate to `/share/queue`
  instead. Saved Queues presets that lived on the modal are
  ported onto the page in #310 PR 3.

### Added

- **App version split into `v{MAJOR}.{U}.{N}`** (issue #266,
  tracer-bullet slice). The auto-update version string now
  separates the **update-flow shape gate** (U) from the
  **release counter** (N). SQLite schema version stays as
  its own field (`CurrentSchemaVersion` for the data plane,
  user_version on disk). Four decisions from the RPCI
  Critique are locked in `internal/update/updater.go`:
  - Legacy `v1.2.{N}` strings parse to **U=1** (decision 1).
  - U mismatch in either direction force-reinstalls
    (decision 2 + 4 — the user's installed flow can't
    safely apply the new release).
  - U match + release N > installed N → auto-update eligible.
  - U match + release N < installed N → downgrade rejected
    as `Compatible=true, Newer=false` so the UI doesn't
    surface the offer.
  - First real U bump ships as `v1.3.0` (decision 3); the
    literal "2" in the middle position is reserved for
    legacy strings thereafter.
  - `compareVersions` returns a `CompareResult{Compatible,
    Newer}` struct instead of `(int, error)`; new
    `NeedsReinstall bool` field on `CheckResult` so the
    Settings UI can surface the reinstall path. Caller at
    `internal/update/updater.go:185` and `:215` updated.
  Regression net: new
  `internal/update/updater_compare_test.go` matrix (5
  parent tests, 23 sub-tests). Covers the 4 decisions + the
  legacy parse path + 4 malformed-input rejections. The
  matrix caught 3 implementation bugs on its first run:
  inverted N comparison, `isLegacyVersionShape` rewriting
  a literal U=2 mid-cycle, and `versionFromString`
  silently truncating inputs with more than 3 segments.
  Future surface work (filed as follow-up issues for a
  separate session per RPCI tracer-bullets discipline):
  - cli_*.go output shape (debug dump, version probe)
  - bump-version.ps1 gains --bump-update-flow + N tracking
  - RELEASING.md + ADR 0008 cross-reference update
  - ArchiveManifest field for new backups
  `.ddbak` archive compat verified: the backup reader's
  only check is `SchemaVersion <= current`; the
  `AppVersion` field shape change is informational only.
  Old `.ddbak` archives keep their old `AppVersion` string
  and continue to load.

### Changed

- **CLI + UI version emit switched to `v1.{U}.{N}` shape** (issue
  #293, follow-up to #266). `internal/buildinfo/buildinfo.go`
  `AppVersion` now sources `versioninfo.AppVersion()` (new
  shape, U=1 / N=1 today) instead of `CurrentAppVersion()`
  (legacy `v1.2.{schema}`). Every downstream emit site
  picks up the new shape automatically because they all
  read `buildinfo.AppVersion`:
  - `debug dump` (`ArchiveInventory.AppVersion` JSON field
    and the text-mode `App version:` line)
  - `migrate status` JSON + text mode
  - `restore point create` / `restore point list` JSON
  - `export backup` / `export shared` source/target version
  - `import backup` source/target version
  - `update.Settings.CurrentVersion` (Settings panel UI)
  - `BackupManifest.app_version` in every new `.ddbak`
  - `cmd/gold-master/main.go` portable-output `app_version`
  Regression net: `internal/db/migration_backup_test.go`
  updated to assert the new shape for `TargetAppVersion`
  (post-upgrade binary) while keeping `SourceAppVersion`
  pinned to the legacy `AppVersionForSchema(1)` string
  (pre-upgrade binary) — that asymmetry is now intentional
  and reflects the upgrade boundary (legacy → new shape).
  Legacy GitHub release tags (`v1.2.N`) still parse cleanly
  via `parseVersion` per #266 decision 1.

### Changed

- **CLI JSON output exposes `update_flow_version` + `release_counter`**
  explicitly (issue #293, follow-up). The v1.U.N shape is now
  parseable without re-tokenizing `app_version`:
  - `dixiedata debug dump --json` adds `update_flow_version`
    + `release_counter` to the `ArchiveInventory` payload.
  - `dixiedata migrate status --json` adds the same two fields.
  Regression net: new `audit/smoke_cli_version_shape.mjs`
  drives both subcommands and asserts the new fields are
  present, positive, and well-typed. Mirrors `smoke_settings_diagnostics.mjs`
  structure but exercises CLI subprocesses instead of HTTP.
  `TestArchiveInventoryOnEmptyDB` (Go unit) asserts the same
  field contracts at the package level.

### Changed

- **`scripts/bump-version.ps1` gains `-BumpSchema`, `-BumpUpdateFlow`,
  `-BumpRelease` switches** (issue #294, follow-up). The script
  now distinguishes the three version counters that
  `internal/versioninfo/versioninfo.go` carries (issue #266):
  - `-BumpSchema` (default; today's behavior) bumps
    `CurrentSchemaVersion`; still requires a paired
    `docs/migrations/v{N+1}.md`.
  - `-BumpUpdateFlow` bumps `CurrentUpdateFlowVersion`,
    resets `CurrentAppVersionInt` to 0, and archives the
    previous U sequence's last N to
    `.release-state/last-n-for-u{prev_U}.json` so a future
    U transition can be reviewed. (`.release-state/` is
    gitignored — see `.gitignore`.)
  - `-BumpRelease` bumps `CurrentAppVersionInt` (N) only;
    use for bug-fix-only releases.
  The three switches are mutually exclusive; passing more
  than one throws. `-VerifyOnly` now checks all three counters
  for drift (U bump requires the sidecar JSON; schema bump
  requires the migration note; CHANGELOG + docs reference the
  new `v{MAJOR}.{U}.{N}` shape).
  Tested in scratch repo against all four paths
  (`-BumpRelease`, `-BumpUpdateFlow`, `-BumpSchema`, mutual
  exclusion).

### Changed

- **Release tag emits `v{MAJOR}.{U}.{N}` shape** (issue #294).
  `scripts/build-common.ps1` `Get-DixieDataAppVersion` now reads
  `CurrentSchemaVersion` + `CurrentUpdateFlowVersion` +
  `CurrentAppVersionInt` from `internal/versioninfo/versioninfo.go`
  and emits `v1.{U}.{N}` instead of the legacy `v1.2.{schema}`
  string. Downstream consumers pick up the new shape automatically:
  - `scripts/release-github.ps1` tag + archive name
  - `scripts/build-release.ps1` archive filename
  The `v` prefix on the returned string matches the historical
  helper contract (callers concatenate without re-prefixing in
  some places, so the prefix is preserved here for consistency).

### Documentation

- **`docs/RELEASING.md` rewritten for the three-counter model**
  (issue #295). §Versioning rules now documents
  `v{MAJOR}.{U}.{N}` with explicit semantics for each counter
  (U bump = reinstall, N bump = bug-fix-only, schema bump =
  migration). Release workflow step 1 picks the right bump;
  the four-step bump section is replaced with a one-switch
  invocation per counter. New "See also" block cross-
  references ADR 0008 + ADR 0007 + the versioninfo.go source.
- **`docs/adr/0008-promotion-protocol.md`** §Open questions Q1
  updated to mark the v{MAJOR}.{U}.{N} split as shipped
  (commit 5a297a6) instead of "future". The "Alt 2"
  continuous-promotion analysis reframes the split as the
  mid-ground that path (b) cadence-driven promotion builds on.
  References list adds #293/#294/#295/#296 cross-links.
- **`CONTEXT.md` §Laws** new sub-section "Release counter N ≠
  schema version" documents the three-counter contract
  + the `bump-version.ps1` switch model.
- **`docs/user-manual.md`**, **`docs/implementation-and-features.md`**
  current release line pinned to `v1.1.59` with a one-line
  issue #266 footnote. **`docs/ai-handoff.md`** version +
  schema version refreshed in the project snapshot block.
- `bump-version.ps1 -VerifyOnly` is now green against the
  new doc surface.

### Changed

- **`BackupManifest` carries `current_update_flow_version` +
  `release_counter` explicitly** (issue #296, follow-up).
  New `.ddbak` / `.ddshare` archives write both fields so
  readers can compare U + N without re-parsing the
  AppVersion string. `loadBackupData` populates them from
  `internal/versioninfo`; `NormalizeManifestBackwardsCompat`
  applies defaults for archives written before this commit:
  - missing `current_update_flow_version` → 1 (legacy
    v1.2.N strings parse to U=1 per #266 decision 1)
  - missing `release_counter` → `schema_version` (the
    historical formula tied N to schema)
  `readBackupManifestFromZip` (CLI import dry-run + preview)
  and the e2e test helper both invoke the normalizer on
  every freshly decoded manifest, so the import pipeline sees
  consistent U + N regardless of archive age. Regression
  net: `TestNormalizeManifestBackwardsCompat` covers all
  three cases (legacy, explicit, malformed with zero
  schema). Existing `TestBackupService_Export*` tests pin
  the new fields in written manifests.

- **Three-branch model introduced (ADR 0009)**:
  `dev` (integration), `stable` (released code, NEW),
  `main` (frozen legacy production at `56e31f0`).
  - ADR 0009 is the source of truth: documents the rename
    rationale, the symmetric branch protection applied to
    both `main` and `stable`, the promote flow (PR via
    GitHub UI), and the conflict policy (merge dev into
    stable when divergence arises).
  - ADR 0008 (existing) gets a one-line cross-reference to
    ADR 0009 in §Implementation notes; its gate chain is
    unchanged.
  - `Makefile` ships four new targets:
    - `make promote-dry-run` — runs the gate chain (test,
      tpl, css, bump-verify, debug, freshness, archive)
      with no push, no PR; prints the dev-vs-stable
      divergence at the end.
    - `make promote` — runs the gate chain (via
      `promote-dry-run`), aborts on divergence, then calls
      `scripts/promote-open-pr.sh` to open the PR
      `dev → stable` via `gh pr create` with the gate-chain
      output + commit log + diff stat embedded in the body.
    - `make promote-prep` — diagnostic; fetches origin,
      prints the divergence, instructs the operator on
      conflict resolution per ADR 0009 §"Conflict policy".
    - `make promote-confirm` — post-merge sanity; checks
      that `stable` HEAD matches the dev merge SHA before
      the operator runs `release-github.ps1`.
  - `scripts/promote-open-pr.sh` (NEW) implements the PR
    opening; the heredoc + markdown body is in a file to
    avoid Make escaping headaches.
  - `.github/BRANCH_PROTECTION.md` (NEW) documents the
    standard protection rules applied to BOTH `main` and
    `stable` (no direct push, no force-push, no deletion,
    require CI green: audit + build + test). Apply via the
    GitHub UI per the steps in the doc; the `gh api`
    verification commands are listed.
  - `.github/workflows/test.yml`, `build.yml`, `audit.yml`
    — `branches: [dev, main]` → `branches: [dev, stable]`.
    The "stern warning if PR targets main without in-place
    safety label" check (test.yml) moves to "if PR targets
    stable."
  - `.github/PULL_REQUEST_TEMPLATE.md` — "every PR to `dev`
    or `main`" → "every PR to `dev` or `stable`."
  - `scripts/release-github.ps1` — `git push origin main`
    (hard-coded) → `git push origin HEAD`. The script no
    longer assumes the working branch; the operator runs
    it from `stable` after the promotion PR is merged.
  - `docs/RELEASING.md` §6 — release workflow now spells out
    the five-step promote chain (dry-run → promote PR →
    merge in UI → promote-confirm → release-github) instead
    of the old "push to main" flow.
  - `AGENTS.md` §Branch policy — rewritten to document the
    three-branch model + symmetric branch protection + the
    promote flow.
  - `CONTEXT.md` §Laws — new law entry: "Released code lands
    on `stable`; `main` is frozen at `56e31f0`."
  - GitHub branch protection rules on `main` and `stable`
    (apply via the UI per `.github/BRANCH_PROTECTION.md`):
    a one-time setup step documented in the PR body.

- **Doc-comment requirement formalized in CONTEXT.md §Laws**
  (issue #273 follow-up audit). New §Laws entry: "Exported Go
  identifiers carry doc comments." The rule:
  - Every exported identifier in a DixieData Go package (func,
    type, var, const, including methods on exported types) must
    have a doc comment.
  - Every Go package has a `// Package <name> <one-sentence purpose>`
    synopsis.
  - **Floor (regression gate):** every Go package under
    `internal/` and `pkg/` with ≥5 exported identifiers must have
    ≥70% identifier-level doc-comment coverage. The CI test
    `TestPerPackageDocCoverageFloor` enforces this; a future
    commit that strips docs in bulk gets caught.
  - 70% is a regression gate, not a target. The working rule is
    "aim for 100% on every new PR."
  - Exemptions: `cmd/*` (unexported main packages), templ-
    generated files (churn that disappears on next `make tpl`),
    and build-tag-gated packages.

  - `TestPerPackageDocCoverageFloor` floor raised from 60% to
    70% to match the formalized rule. Coverage delta from
    raising: +30 documented identifiers added to
    `internal/archive` (BackupManifest, SharedImportSummary,
    SourceConflictLedger, RestoreBackupArchive, all the
    compat.go alias re-exports, etc.) to keep archive above
    the new floor.

  - Audit metric: 71.7% → 74.1% overall; `internal/archive`
    54.2% → 90.4%.

### Maintenance

- **RPCI protocol: add Autonomous clause** (issue #339).
  `docs/agents/rpci.md` §Critique now documents a per-session
  shortcut: when the user replies with "all recommended" /
  "take the recommended defaults" / "yes to all", the agent
  MAY proceed to Implement without re-asking each surfaced
  decision. Locks the boundary: the agent MUST echo the
  approved decisions in the commit body, MUST surface any
  new mid-Implement decisions for explicit approval, and
  MUST NOT treat the clause as blanket approval of future
  RPCI sessions. Anti-pattern list: no auto-progress on
  system reminders, no consent-by-silence, no vague-reply
  shortcut. The "Invocation" section lists the new surface
  forms alongside the existing "do RPCI on X" patterns.
  Captured during issue #320 slice #322; the agent had
  already drafted D1-D5 in chat and a no-decision-changes
  reply was waiting on the gate for multiple turns. The
  amendment unblocks that case without weakening the
  primary explicit-approval gate.
  `person_record_id`** (issue #320, slice 1 + 1.5). Adds the
  `event_person_links` many-to-many junction, 4 new columns on
  `soldiers` (`kind`, `begin_date`, `end_date`, `description`)
  for the Event Record subtype, renames `soldier_id` to
  `person_record_id` in 6 tables (8 columns total) to reflect
  that the FK now references any Person Record subtype, and
  rewrites the FTS5 trigger text (SQLite does not auto-update
  trigger SQL on `RENAME COLUMN`; the migration block drops
  and recreates the affected triggers). Bumps
  `CurrentSchemaVersion` 59 → 60. New `models.EntryTypeEvent`
  constant; new `(*DB).NextEventID()` mints `EVT-NNNNN`
  Display IDs. Glossary: adds the **Event Record** entry;
  renames **Timeline Event** → **Timeline Marker**. The
  `spouse_soldier_id` self-FK is intentionally NOT renamed
  (it is a Soldier-to-Soldier relationship, not a Person
  Record FK). The Go struct field renames
  (`Record.SoldierID` → `PersonRecordID`, etc.) keep their
  `json:"soldier_id"` tags so v59 `.ddshare` archives
  round-trip through a v60 binary. v60 → v59 downgrade works
  (PartiallyReversible); v59 → v58 still refuses (Block 17
  is Irreversible). 2 commits; net +522 / -800.

- **Remove the deprecated Find a Grave paste-HTML scrape form**
  from `/soldiers/new` (issue #319). The canonical replacement is
  the `/share/imports` memorial-json import via Tampermonkey.
  Surface removed: the `<details>Scrape Find a Grave</details>`
  block, the `POST /soldiers/scrape-findagrave` route and handler,
  the `internal/findagrave/` parser package, the dedicated fuzz
  test + 3919-line fixture, the findagrave-stress step in
  `scripts/run-stress-tests.ps1`, the `FindAGraveScrapeState` +
  `ScrapedRelative` types in `models` and `viewmodel`, the
  `SoldierScrapeFindAGrave()` route helper, and the matching
  `applyFindAGraveAutofill` / `renderEntryFormWithScrapeState` /
  `findAGraveNeedsReview` / `findAGraveReviewReason` helpers.
  Docs updated: `docs/SERVICES.md`, `docs/RESEARCH.md`,
  `docs/user-manual.md`, `docs/ui-map/routes.md`,
  `docs/ui-map/wireframes/03-soldiers-list.md`,
  `docs/ui-map/wireframes/06-soldier-new.md`. Find a Grave
  evidence-source + CSV-import + calendar-link-helper surfaces
  stay (different feature). 2 commits; net -5457 lines.

- **`docs/agents/cli-plan.md` pins the export leaf-verb
  aliases** to clear a pre-existing cli-coverage drift.
  The detector scans `dixiedata <verb>` lines and only saw
  the parent (`export`) for the export subcommands;
  `pdf`/`jpg`/`json`/`csv`/`ical`/`static-archive`/`backup`
  were listed as "implemented, not documented" despite
  being reachable leaf verbs under `dixiedata export`.
  Added a new "Export leaf-verb aliases" subsection that
  pins each leaf verb plus `--smoke-json` so the drift
  detector returns 0 and the CI test gate goes green.
  No code or dispatcher changes; doc-only.

- **Schema migration blocks enumerated as a typed slice** (preparatory
  for issue #273). `internal/db/schema.go::applySchema` used to inline
  every block (CREATE TABLE constant, ALTER TABLE ADD COLUMN loop,
  phase migrations, normalize UPDATEs, helper-driven migrations) as
  a single ~160-line transaction body. Refactored into:
  - `internal/db/migrations.go` (new file): `Reversibility` enum
    (`Reversible` / `PartiallyReversible` / `Irreversible`),
    `Migration` struct (`ID`, `Up func(*sql.Tx) error`,
    `Reversibility`, `Reason`), `var migrations = []Migration{...}`
    with 17 entries (one per block per the catalogue at
    `docs/migrations/reversibility.md`), and the package-private
    helpers `applyAddColumnLoop` / `applySoldiersNormalization` /
    `applyImagesIsPrimary` that fold the multi-statement inline
    UPDATEs into single `Migration.Up` closures.
  - `applySchema` now reads `for _, m := range migrations { m.Up(tx) }`
    in slice order; behaviour is byte-identical to the pre-refactor
    inline ordering.
  - `Migrations()` is exported so the future `dixiedata debug
    schema-reversibility` audit harness and `migrate down <target>`
    runner (issue #273) can iterate the catalogue from outside the
    package.
  Regression net (new `internal/db/migrations_test.go`):
  - `TestMigrationsCatalogueIsOrdered` locks the 17-block order
    (Block 1 ... Block 17); a reorder would silently change the
    future DOWN runner's reverse-iteration behaviour.
  - `TestMigrationsCatalogueHasUp` locks every entry has a non-nil
    `Up` (a nil would panic at tx time).
  - `TestMigrationsReversibilityMapping` pins the per-block
    Reversibility class per the catalogue.
  - `TestReversibilityString` locks the CLI-facing labels
    (`reversible` / `partially_reversible` / `irreversible` /
    `unknown`).
  - `TestReversibilityIrreducibleCount` pins the count at 5
    (Blocks 4, 5, 12, 13, 17); the future DOWN runner's gate
    semantics depend on this number.
  No public API change; `db.Open(dataDir)` + `db.CurrentSchemaVersion`
  behave identically. All existing tests pass unchanged including
  `TestOpenCreatesRetainedPreMigrationBackup` (the v1→current
  end-to-end). This is PR 1 of the two-PR plan for issue #273
  (refactor first, then feature).
- **Schema migration blocks get a `Down` function + `applyDownSchema`
  runner** (issue #273 PR 2 of 2). Every `Migration` in the slice
  now carries a `Down func(*sql.Tx) error` field paired with `Up`.
  Per the reversibility classification:
  - **Reversible** (Blocks 1, 2, 9, 10, 15, 16) — `Down` is the precise
    inverse (DROP TABLE, DROP COLUMN, DROP INDEX, DELETE seed rows).
  - **PartiallyReversible** (Blocks 3, 6, 7, 8, 11, 14) — `Down` is
    best-effort; null-coalesce cases are reverted mechanically,
    sentinel/printf/canonicalization cases are documented as
    "what was lost" in the CLI manifest.
  - **Irreversible** (Blocks 4, 5, 12, 13, 17) — `Down` returns
    `ErrMigrationIrreversible`; the runner refuses the entire path
    with `ErrDowngradeRefused` wrapping the blocking block ID +
    reason. Per design decision Q2, `--force-irreversible` emits
    the manifest but does NOT bypass the refusal.
  - `internal/db/schema.go::applyDownSchema(db, target int)` is
    the new runner. It maps `(current - target)` to a slice window
    (capped at `len(migrations)`), iterates in REVERSE order, refuses
    on `ErrMigrationIrreversible`, and writes `PRAGMA user_version
    = target` as the LAST statement before commit. Exported as
    `db.ApplyDownSchema` for the CLI.
  Regression net (new `internal/db/migrate_down_test.go` + extensions
  to `migrations_test.go`):
  - `TestApplyDownSchema_NoOpWhenAlreadyAtTarget`
  - `TestApplyDownSchema_RefusesPastIrreversible` (asserts
    `errors.Is(err, ErrDowngradeRefused)` AND
    `errors.Is(err, ErrMigrationIrreversible)`; asserts
    user_version is unchanged after refusal)
  - `TestApplyDownSchema_PartialReversibleStepDown` (every
    v<N<59 must refuse — documents the current v59 floor)
  - `TestApplyDownSchema_EmptyArchiveSucceedsAtCurrentVersion`
  - `TestApplyDownSchema_RefusalDoesNotMutateTables` (row counts
    byte-identical pre/post refusal)
  - `TestRetainedBackupDirectionDiscriminator` (smoke test
    against the existing `RetainedBackupManager` — documents the
    integration point the CLI uses for the pre-DOWN snapshot)
  - `TestMigrationsCatalogueHasDown` (every migration has a
    non-nil Down function)
  - `TestMigrationsIrreversibleDownRefuses` (every Irreversible
    block's Down returns `ErrMigrationIrreversible` when called
    against a real `*sql.Tx`)

- **`dixiedata migrate down <target> [--yes] [--force-irreversible]`
  ships** (issue #273 PR 2). The CLI surface previously returned an
  error for `migrate down`; the dispatch now routes to
  `runAdminMigrateDown` which:
  1. Refuses without `--yes` (exit code 1, user-facing message).
  2. Computes the slice window from the audit catalogue, prints
     a per-block manifest with `[R]` / `[P]` / `[I]` reversibility
     markers and the per-block Reason.
  3. Calls `db.ApplyDownSchema` and surfaces the refusal (if any)
     with the blocking block ID + reason.
  4. JSON mode emits the same manifest as `manifest: []struct{id,
     reversibility, reason}`.
  Existing `TestParseAdminArgs_MigrateUnknown` updated; new tests:
  - `TestParseAdminArgs_MigrateDown_AcceptsVersionAndFlags`
  - `TestParseAdminArgs_MigrateDown_RejectsMissingVersion`
  - `TestParseAdminArgs_MigrateDown_RejectsNonIntegerVersion`
  - `TestParseAdminArgs_MigrateDown_RejectsUnknownFlag`
  - `TestRunAdminMigrateDown_RefusesWithoutYes`
  - `TestRunAdminMigrateDown_ManifestPrinted`

- **`docs/migrations/v55.md` doc-vs-code mismatch corrected**. The
  v55 doc claimed a SQL CHECK constraint was added at v55, but the
  inline `CREATE TABLE soldiers` at `internal/db/schema.go:25-65`
  declares `entry_type TEXT NOT NULL DEFAULT 'soldier'` without a
  CHECK. The comment at `schema.go:824-836` explicitly documents the
  pragmatic "application-level validation" approach. The doc is
  rewritten to reflect the actual SQL effect (log table +
  application-level validation) per the audit's Block 16 entry.
  The audit classifies Block 16 as Reversible at the SQL level;
  the `migrate down` runner drops the log table without refusal.

- **Pre-downgrade snapshot is taken automatically** (issue #273
  follow-up, post-PR-2). The audit's open question Q1 / design
  decision Q5 called for a direction-aware snapshot helper so the
  DOWN runner can roll back if the downgrade produces a corrupted
  schema. The follow-up adds:
  - `RetainedBackupRecord.Direction` field (json: `direction,omitempty`).
    New `DirectionLabel()` method returns `"upgrade"` for legacy
    records persisted before the field existed (backward compat).
  - `preSchemaDowngradeBackupKind` constant + `directionDowngrade` /
    `directionUpgrade` direction values + `snapshotFileNameDowngrade`
    (`dixiedata-pre-downgrade.db`) / `snapshotFileNameUpgrade`
    (`dixiedata-pre-upgrade.db`).
  - `CreatePreSchemaChangeBackup(input, direction, snapshot)` is
    the unified implementation. `CreatePreSchemaUpgradeBackup`
    becomes a thin wrapper (no caller change). New
    `CreatePreSchemaDowngradeBackup` is the DOWN-path wrapper.
  - `snapshotFileNameFor(direction)` helper centralises the
    on-disk filename choice.
  - `RestoreDatabaseSnapshot` is unchanged (the path is
    direction-agnostic).
  - The CLI's `runAdminMigrateDown` now calls
    `manager.CreatePreSchemaDowngradeBackup(...)` BEFORE the
    runner fires, so the operator has a fresh copy of the live
    schema even when the runner refuses on an Irreversible block.
    The output reports the snapshot ID and prints
    "pre-downgrade snapshot <id> is available for rollback" on
    refusal. On success, the snapshot is in the index for future
    restore.
  Regression net (new tests in `retained_backup_manager_test.go` +
  updated `TestRunAdminMigrateDown_ManifestPrinted`):
  - `TestRetainedBackupManagerDowngradeSnapshot` — Kind, Direction,
    on-disk filename, restore path, no stale upgrade filename.
  - `TestRetainedBackupManagerMixedUpgradeAndDowngradeSnapshots` —
    same manager can hold both kinds in one index.
  - `TestRetainedBackupRecordDirectionLabelDefaultsUpgrade` —
    backward-compat default for legacy records.
  - `TestCreatePreSchemaChangeBackupRejectsUnknownDirection` —
    programmer-error guard.
  - `TestRunAdminMigrateDown_ManifestPrinted` now also asserts the
    pre-downgrade snapshot line + rollback hint.
  - Existing `TestRetainedBackupManagerCreateAndRestoreDatabaseSnapshot`
    + `TestRetainedBackupManagerPrunesOlderBackupsByCount` pass
    unchanged (the UP path is unchanged).

### Fixed

- **Build Share Archive button + Share Queue pill do nothing on click**
  (issue #308). The Share Build modal at `/share/queue/modal` was never
  server-rendered into any page, so `openShareQueueModal()` in
  `frontend/app.js` queried the DOM and silently early-returned. Added
  `loadShareQueueModal()` that fetches the modal HTML on demand, inserts
  it into `document.body` (hidden), and caches the node for subsequent
  opens — mirrors the lazy-load pattern from issue #234's
  `loadPrintRecordsFragment`. Both call sites (the button on
  `/share/exports` and the persistent layout pill) now open the modal
  correctly.

- **In-place-safety walker false-positives on SQL comments** (issue #268).
  `classifySchemaLine` in `internal/appshell/cli_debug_inplace.go`
  used `strings.Contains` to match destructive keywords (DROP
  TABLE / DROP COLUMN / RENAME / DELETE FROM) regardless of
  whether the diff line was a SQL comment. A diff like the
  `/-- DROP TABLE users; never actually runs/` line in a
  migration would false-positive as `schema_drop_table` HIGH —
  the maintainer would either skip the work or remove the
  comment, neither of which is the right fix. The walker now
  skips lines whose trimmed prefix is `--` or `/* */`
  (single-line block comments) via a new `isCommentLine` helper.
  Multi-line `/* ... */` blocks are not blocked in v1 (the diff
  walker is per-line; a multi-line block comment body would
  still trip if the body happened to contain DROP TABLE, which
  is conservative).

### Added

- **Cli-coverage + in-place-safety walker locks** (issue #268).
  Two new test files pin the regex shapes the build-protocol
  pack relies on:
  - `internal/appshell/cli_debug_coverage_test.go` —
    `TestScanImplementedSubcommandsFixtureShape` +
    `TestScanDocumentedSubcommandsFixtureShape` synthesize a
    tempdir with synthetic `main.go` + `cli_*.go` + `cli-plan.md`,
    call the production walkers, and assert the exact key set.
    Locks the switch-case dispatcher parser + the fenced-block
    doc parser.
  - `internal/appshell/cli_debug_inplace_test.go` —
    `TestClassifyAddedLine_*` covers the 4 cases from the issue
    body (DROP TABLE in schema file → HIGH, r.Get in routes.go
    → MEDIUM, DROP TABLE in comment → not flagged, r.Get in
    routes_test.go → not flagged) plus the rename/delete/drop-
    column schema kinds and a regression guard for random
    non-destructive lines. The DROP-TABLE-in-comment test caught
    the false-positive bug above on its first run, confirming
    the lock is real.
  - `.github/workflows/test.yml` — new
    `Cli-coverage drift detector` step that runs
    `node scripts/cli-coverage.mjs` on every push + PR. Exit 1
    on documented-not-implemented OR implemented-not-documented
    drift. The Go walker (run via `dixiedata debug cli-coverage`
    in `make freshness`) is the source of truth; this offline
    Node step keeps CI simple.

### Changed

- **Support & Diagnostics moved from /share to /settings** (issue #255).
  The card with the two diagnostic-export buttons (Export Feedback
  Log, Export Bug Report Bundle) moved off `/share` and onto
  `/settings`, where it slots in alongside Debug Mode and Data
  Quality Scan. The buttons produce diagnostic data, not export
  data, so they belong with the other settings / configuration
  surfaces (best discoverability — users look for "send bug
  report" in Settings, not Share). The action URLs
  (`/export/feedback-log`, `/export/bug-report`) and the
  underlying handlers in `internal/appshell/exports_handlers.go`
  are unchanged — only the templ rendering moves, per the
  issue's Phase 1 / Phase 2 split (the larger /share re-org
  in #253 / #284 is a separate follow-up). Regression net:
  new `audit/smoke_settings_diagnostics.mjs` (12/12 green) —
  asserts the card is on /settings, the two buttons carry the
  same `data-action` URLs as before, /share no longer renders
  the buttons or the Troubleshooting copy, and both POST
  endpoints still return 2xx; updated
  `internal/templates/share_test.go` to flip the Support &
  Diagnostics assertions from "must be on /share" to "must NOT
  be on /share" (the go-live of #255); new
  `internal/templates/entry_form_test.go::TestSettingsViewIncludesSupportDiagnosticsPanel`
  pins the /settings side of the move.

### Fixed

- **`dixiedata debug dump --json` user_identity field shape** (issue #272).
  `models.UserIdentity` had no `json:"..."` struct tags, so
  `encoding/json` emitted PascalCase keys (`FirstName`,
  `MiddleName`, `LastName`, `BirthYear`, `NodePrefix`). Renaming
  fixed: keys are now `first_name`, `middle_name`, `last_name`,
  `birth_year`, `node_prefix`. The key shape now matches every
  other `--json` emitter in the dispatcher (snake_case +
  plural-for-collections). After the fix the `user_identity`
  payload in `debug dump --json` matches the convention the
  CI consumers expect.

### Added

- **CLI JSON key naming lock** (issue #272). New
  `internal/appshell/cli_json_keys_test.go` runs the live admin
  + debug subcommands in `--json` mode and asserts every key at
  every nesting depth is snake_case (lowercase letters / digits /
  underscores; no leading/trailing or consecutive underscores).
  Future emitters that drift to camelCase (e.g. reusing an
  unkeyed Go struct) fail the test before merge. Lightweight
  survey helper: `looksPlural()` + `walkKeys()` walking the
  parsed JSON tree, skipping root-level arrays (those wrap
  record collections; future follow-up). The test caught the
  UserIdentity issue above on its first run, confirming the
  convention was implicit but unverified.

- **Phase 3 visual + WCAG contrast audit harness** (issue #292).
  The Phase 2 token migration (#291) advertised byte-equivalent
  rendered colors for all 304 swapped sites, but the maintainer
  asked for an audit that measures the actual computed ratio so
  any future regression trips the probe before it ships. New
  `audit/smoke_phase3_contrast.mjs` walks the 5 surfaces called
  out in #292 §Scope — primary button, secondary button, pill
  link, field input, ghost link — plus the 4 toast variants and
  the body gradient. Per-surface route selection picks the page
  that actually renders each selector. Helper pattern
  (parseRGB / lum / ratio / composed-over-white) extracted from
  `audit/smoke_foldout_nav.mjs::Step 7.5` so future audits can
  reuse it. Screenshots of `/calendar`, `/browse`,
  `/share/exports`, and `/soldiers/1` are staged in
  `audit/reports/phase3-after_*.png` for visual diff.

  **Finding**: the probe initially misreported the gold-gradient
  primary button at 1.28:1 because the helper read
  `backgroundColor` (transparent for gradient backgrounds) and
  walked up to the panel, composing over white. Linear-gradient
  `rgb(...)` stops live on `backgroundImage`, not backgroundColor.
  The fix is in the helper (paren-depth walk over
  `backgroundImage` to parse gradient stops, worst-case
  pick = lightest stop for dark text). After the fix, the
  primary-button measures **6.43:1** on `/browse` —
  comfortably above WCAG AA. All 5 surfaces measured so far
  pass: primary 6.43, secondary 12.06, pill 12.06, field-input
  12.04, ghost-link 9.87. The probe is now the tripwire for
  any FUTURE swap that lowers contrast further.

### Documentation

- **ADR 0008 — promotion protocol** (issue #267). Lifts the
  dev → main promotion step from a user-driven ad-hoc flow
  into a documented gate chain, codified in a new
  `make promote` + `make promote-dry-run` workflow. The
  gate chain runs `make test` → `tpl` → `css` → `bump -VerifyOnly`
  → `debug` → `freshness` → **`make in-place-safety`
  (the new hard gate that lifts ADR 0007's `safe-for-in-place`
  label from informational at PR time to enforced at
  promotion time)** → `archive` → `release-github`. The
  maintainer's AGENTS.md §Branch policy ("do not promote
  dev to main without explicit user direction") is now
  WHAT the user signs off on at that moment — every gate
  has a defined recovery path documented in
  `docs/adr/0008-promotion-protocol.md`. The cadence
  question (continuous promotion on every commit) is
  explicitly deferred as a follow-up — the `v{MAJOR}.{U}.{N}`
  split from issue #266 would unlock it. ADR carries no
  code changes; the gate chain's first proof is the next
  `make promote-dry-run` on dev.

### Fixed

- **Live preview renders the stale-filter warning line** (issue #260).
  The server side of the contract was already in place —
  `internal/appshell/export_preview.go:62-87` computes
  `StaleCount` + `StaleSummary` via the shared
  `computeExportTemplateStale` helper, and `export_preview.go:242-245`
  emits the line above the count. The JS at `frontend/app.js:4235-4256`
  fetches `/export/preview` as form POST and injects the response HTML
  directly into `[data-print-config-preview]`, so the warning is
  visible the moment the user types a filter value that doesn't exist
  on any row. Unit-level guarantee in
  `internal/appshell/export_preview_test.go::TestHandleExportPreview_StaleFilterWarning`.
  Regression net: new `audit/smoke_stale_preview_warning.mjs` (5/5
  green) — POSTs stale + clean filter sets against `/export/preview`
  and asserts the warning line is present on stale and absent on
  clean, so a future refactor that drops or always-izes the line
  fails the probe.

- **Print-config "Show details" toggle for stale-template warnings**
  (issue #259). The JS handler at `frontend/app.js:4308-4315`
  was wired — click listener on `[data-export-templates-warnings-toggle]`
  that toggled the wrap's `hidden` class and flipped
  `aria-expanded` + the button label — but no element on
  the page had the selector. Result: clicking the warning
  toast for a stale saved template showed only the count;
  the underlying list of stale filter values was invisible
  to the user (had to open devtools). Fixed by adding the
  toggle button + wrap + live `<ul>` in
  `internal/templates/partials/print_config_modal.templ:49-53`
  (the modal renders on `/browse` and `/share/exports`
  since #265 moved Share exports into its own page).
  Regression net: new `audit/smoke_stale_template_warnings.mjs`
  asserts the toggle + wrap + list markup on both pages
  and exercises click-show / click-hide in a real browser
  (11/11 green).

- **Initial-setup submit feedback** (issue #263). The `/setup`
  credentials form already had `data-dixie-submit` on the
  form and `data-busy-label="Saving…"` on the submit button
  (so the JS handler disables the button and sets
  `aria-busy="true"` on click), but the server-side POST
  branch returned a silent `303` to `/calendar` — no toast
  header, no redirect contract the dispatcher reads. After
  the fix the handler writes the Option C contract
  (`200 OK` + `X-DixieData-Redirect: /calendar` +
  `X-DixieData-Toast: "Identity saved. Loading DixieData…"`
  + `X-DixieData-Toast-Type: success`), so the
  `dispatchDixieDataForm` interceptor navigates and shows
  the success toast atomically on landing. Rapid double-clicks
  during the slow DB write are now blocked by the disabled
  submit button + `aria-busy` state the JS already set up.
  `redirect_headers_test.go::exemptFunctions["handleInitialSetup"]`
  comment updated to reflect the new contract (GET-only
  303 carve-out). Regression net: new
  `internal/appshell/initial_setup_test.go::TestHandleInitialSetupPostSetsDixieRedirectAndToast`
  boots a fresh sqlite archive, posts the credentials
  form, and asserts the three headers + the cleared
  `setupRequired` flag;
  `TestHandleInitialSetupGetStillRedirectsToCalendarWhenNotRequired`
  pins the GET branch's 303 + Location behaviour so the
  exemption comment stays meaningful.

### Added

- **Tracer-bullets discipline.** New section in
  `docs/agents/feature-protocol.md` codifying the
  end-to-end vertical-slice rule from The Pragmatic
  Programmer as the DixieData enforcement mechanism for
  the 3-tier commit rule. Paired with the new
  `tracer-bullets` skill in `~/.pi/agent/skills/` that
  any RPCI loop or build-feature flow can invoke to
  force "one slice, one commit, green before next
  slice" behaviour. The Plan phase of
  `docs/agents/rpci.md` now requires a tracer-bullet
  first slice for any multi-slice plan, and the
  Implement phase mandates a fresh session per slice
  to keep prior decisions un-contaminated. Advisory
  only — no CI tripwire.
- **CLI follow-up pack.** Resolves 7 of the 13 carryover
  items from `docs/agents/cli-plan.md` "Open follow-up":
  - `dixiedata --version` / `-v` (issue #271) — top-level
    short-circuit in main.go that prints
    `buildinfo.AppLabel + buildinfo.BuildIdentity`. Exits 0.
  - `dixiedata help` / `--help` / `-h` (issue #277) — top-level
    help dispatch with a hand-maintained subcommand list.
    main_test.go asserts the listed verbs match the
    dispatcher's view.
  - `dixiedata --log-to-stderr` (issue #270) — mirrors the
    JSONL log to stderr via a new `debug.SetStderrMirror`
    toggle + a `teeHandler.stderrMirror` field. Triggered
    by the env var `DIXIEDATA_LOG_TO_STDERR=1` which main.go
    sets before the appshell starts.
  - `restore point apply` (issue #269) — the previously
    no-op subcommand now actually applies via the existing
    `backup.ImportWithLocalIdentity` flow. `--dry-run`
    preserves the previous behaviour. New
    `RestorePointManager.LocalArchiveAbsolutePath` accessor.
  - Exit-5 path (issue #275) — `recoverExit5` wrapper converts
    panics in any subcommand runner to exit code 5 with the
    panic value + stack trace on stderr. All 5
    `run*Subcommand` helpers wrapped.
  - `writeError` helper (issue #274) — centralised the
    `error: <msg>\n` to stderr format. The 10 call sites in
    main.go funnelled through it. New `errors.go`.
  - `--data-dir` path-with-spaces test (issue #276) —
    parser test with 6 path shapes (POSIX + Windows +
    embedded spaces + leading/trailing whitespace + quote).
- **Build / release / schema / update-in-place safety
  protocol.** Adds `docs/agents/build-protocol.md` (the
  canonical procedure covering Makefile hygiene, CLI
  freshness, release pipeline, schema bumps, and
  update-in-place safety); `make freshness` (builds +
  sanity-probes every debug subtool — DixieData,
  dixiedata-web, seed-data, gold-master, dixiedata-tune
  — and runs CLI coverage); `make release-pipeline`
  (ordered 8-gate release chain with halt-on-first-failure);
  `dixiedata debug cli-coverage` (walks the dispatcher vs
  `docs/agents/cli-plan.md` and emits documented-vs-
  implemented drift; Node fallback in
  `scripts/cli-coverage.mjs`); `dixiedata debug in-place-
  safety` (walks `git diff <last-tag>..HEAD` and flags
  destructive schema operations + handler registrations);
  PR template (`safe-for-in-place` / `unsafe-for-in-place`
  required); 2 new labels (`safe-for-in-place`,
  `unsafe-for-in-place`); schema-touching detector in CI
  (`feat(db)` / `feat(schema)` / `fix(db)` commits must
  bump or use the `chore: skip-schema-bump` hatch);
  `bump-version.ps1 -DetectDrift` mode + `make bump-detect-
  drift` (Windows/local equivalent of the CI detector);
  filed issue #266 for the future `v{MAJOR}.{U}.{N}`
  version split (separates update-flow gate from schema
  version). Each piece is a separate commit; this bullet
  is the umbrella.
- **Feature add protocol + label taxonomy + historical
  artifact index.** Adds
  `docs/agents/feature-protocol.md` (the canonical procedure
  for adding a new feature: pre-flight checklist, 3-tier
  commit rule, deep-module discipline, pipeline phasing,
  per-layer load table, anti-patterns);
  `docs/agents/INDEX.md` (3-tier progressive-disclosure
  table for `docs/`); the Backend-First Law in
  `CONTEXT.md` ("no feature PR ships a backend surface
  without a UI apply-site"); the Historical Artifact
  glossary term; the Feature Protocol section +
  6-axis Label Taxonomy in `docs/agents/issue-tracker.md`;
  16 new issue labels (`area:*` × 11, `priority:*` × 3,
  `blocked`) with `scripts/sync-labels.sh` (idempotent
  spec) and `scripts/backfill-labels.sh` (idempotent
  backfill applied to 18 open issues); `docs/historical/`
  retention tree with README; and per-iteration PDF
  gitignore rules. Each piece is a separate commit; this
  bullet is the umbrella.

### Fixed

- Cold-start top-nav foldout click did nothing in the
  Wails desktop binary (issue #285). `installFoldouts()`
  ran once on `DOMContentLoaded` with `triggerCount: 0`
  on the initial `/` response; subsequent htmx swaps
  that re-rendered the trigger did not re-init, so the
  trigger sat in the DOM with no click listener until
  the user navigated away and back. The fix makes
  `installFoldouts` idempotent (document-level
  outside-click handler guarded by
  `window.__foldoutDocHandlerBound`; per-trigger handlers
  guarded by a `WeakSet` of bound trigger elements) and
  adds the call to `initializeDynamicContent` so it
  re-runs on every `htmx:load`. Regression net: new
  Step 10 in `audit/smoke_foldout_nav.mjs` forces a
  re-install via `window.__foldoutProbeReinit` and
  asserts the click still toggles open/close. Probe is
  now 41/41. The fix is documented in `docs/COMMON_BUGS.md`
  §3.7 (`FUTURE-NAV-AVOID`) and `docs/agents/bug-pattern-grep.md`
  §10 so the next foldout (Browse filters, Tags picker,
  etc.) ships the two-hook init pattern by default.
- `dixiedata debug cli-coverage` (and therefore
  `make freshness`) panicked with `slice bounds out of
  range` when any `Has*Subcommand` / `Has*Flag` function
  body in `internal/appshell/cli_*.go` was shorter than
  200 chars past the `case "<verb>":` match (issue #286).
  The `scanImplementedSubcommands` walker now clamps
  the case-window slice to `len(body)` and the 200-char
  heuristic lives in a named `caseWindowChars` const.
  Regression net:
  `internal/appshell/cli_debug_test.go::TestScanImplementedSubcommands_ShortBody`
  seeds synthetic bodies of 0 / ~50 / 180 / 200 / 500
  chars past the match and asserts no panic + the
  documented verb is captured.
- `/tags` showed the empty-archive welcome card when the
  archive had Person Records but zero tags (issue #262).
  The page now distinguishes two empty conditions: a
  truly-empty archive (zero records + zero tags) keeps
  the welcome; archive-with-records-but-no-tags shows
  the tags-specific "No tags yet. Apply a tag from any
  Person Record detail page to create the first one."
  copy. The fix threads `viewmodel.ArchiveCounts` into
  `TagsManagementPage` and branches on
  `counts.TotalRecords() == 0`. Regression net: the
  unit-test pair in
  `internal/appshell/tags_handlers_test.go`
  (`TestTagsManagementPageRenders` + new
  `TestTagsManagementPageHasRecordsButNoTags`).
- `.pi/agents/Explore.md` frontmatter was unparseable by
  the `yaml` package, breaking every subagent spawn with
  `Nested mappings are not allowed in compact mappings at
  line 1, column 14`. Root cause: the description value
  contains three `: ` (colon-space) sequences
  (`search breadth: "quick"`, `…: "medium"`,
  `…: "very thorough"`) that the parser reads as nested
  key/value pairs inside the description's mapping. This
  is the same latent bug the package's own `ejectAgent`
  now works around by wrapping descriptions with
  `JSON.stringify` (per its CHANGELOG). Fix wraps the
  description in a YAML 1.2 double-quoted scalar with
  embedded `"` escaped as `\"`. Regression net: a node
  one-liner using the same `yaml` package parses
  `.pi/agents/{Explore,Plan,general-purpose}.md` and
  returns the expected keys (description / tools /
  model / thinking). Subagent spawn now succeeds.
- **Share foldout menu too wide at small viewport sizes**
  (issue #288). On a 16" laptop snapped to split-screen
  (viewport ~640–1000px), opening the Share foldout from
  the top nav rendered the panel at its baked-in 14rem
  (224px) min-width with no upper cap, and at the right
  edge of the trigger could push past the page edge or
  force horizontal scroll. Single-slice fix:
  `internal/templates/components/foldout.templ` adds a
  `max-w-[calc(100vw-2rem)]` cap alongside the existing
  `min-w-[14rem]` floor (additive, not a replacement) so
  the panel can never overflow the viewport on any size
  screen — an earlier `html[data-layout-mode="split-screen"]`
  layout-mode rule intended to drop the 14rem floor was
  rolled back (see followup entry below) because it
  pushed the panel's left edge off-screen at split-screen
  viewports. Regression net:
  `TestFoldout_PanelResponsiveSizing` in
  `internal/templates/components/foldout_test.go`
  locks the templ-rendered class tokens, and the smoke
  probe `audit/smoke_foldout_split_screen_sizing.mjs`
  sweeps 800/900/1000/1100/1200/1400/1600px viewports
  and asserts (a) `data-layout-mode` flips correctly at
  the 1000px breakpoint, (b) the panel's left edge is
  ≥0 (no left clipping), (c) the panel's right edge is
  ≤viewportWidth (no right clipping), (d) the panel
  width stays at the 14rem floor at every width
  (regression net against the reverted layout-mode rule),
  and (e) all 4 menuitems render and remain in-viewport.
- **Reverted the issue #288 slice-2 layout-mode foldout
  rule.** The original slice-2 `html[data-layout-mode="split-screen"]
  .foldout-panel { min-width: 0; width: calc(100vw - 2rem); }`
  rule intended to drop the 14rem floor at split-screen
  viewports, but it instead stretched the panel to nearly
  the full viewport width while still anchoring to the
  trigger's right edge — at 900px the panel grew to 868px
  wide and pushed its left edge to x=-240, off the left
  side of the screen. The user's report after #288 landed
  ("Share foldout cuts off at small widths") reproduced
  this exactly. Revert path: delete the rule from
  `frontend/tailwind.css`, drop the matching entry from
  `internal/templates/layout_test.go`'s compiled-CSS
  checks slice, drop the cross-reference line in
  `internal/templates/components/foldout_test.go`. Net
  behavior: the foldout panel sits at its templ default
  14rem (224px) floor at every viewport ≥ 640px,
  anchored to the trigger's right edge via the
  pre-existing `absolute right-0` class, with the slice-1
  `max-w-[calc(100vw-2rem)]` cap on top so a future menu
  item with a long label still can't overflow. Regression
  net: `audit/smoke_foldout_split_screen_sizing.mjs`
  now sweeps 7 viewport widths and asserts the unified
  invariant (panel-left ≥ 0, panel-right ≤ viewportWidth,
  panel width = 224px at every width); the probe is 70/70.

### Added

- Tags sub-card on the Person Record detail page
  (issue #256). The `tag_picker.templ` primitive existed
  but was never invoked by any other templ — users had
  no UI to add a tag to a Person Record. The new sub-card
  sits at the bottom of the existing summary card on
  `/soldiers/{id}` and renders the current tags as chips
  with × detach buttons, an empty-state message, and a
  collapsible "+ Add tag" form that POSTs to
  `/soldiers/{id}/tags`. Both attach and detach use
  `data-reload-on-success="true"` so the page reloads
  and the chip list updates without a full nav.
  Regression net: `audit/smoke_soldier_tag_picker.mjs`
  (9 assertions: heading renders, empty state, expand
  button, input found, form action + reload attribute,
  POST fires to correct URL).
- Top-nav link to the `/tags` management page (issue
  #256 follow-up). The page shipped in PR #195 but had
  no discoverable entry point; users had to URL-guess.
  The link is added to both the primary top-nav (between
  Share Queue and Settings) and the mobile / split-screen
  layout so it surfaces in every viewport.
  Regression net: `audit/smoke_tags_nav.mjs` (5
  assertions: primary nav has Tags link, link points to
  /tags, mobile nav also has the link, click navigates
  to /tags, /tags page renders).
- "Include tags" opt-in checkbox for `.ddshare` exports
  on `/share` (issue #261). The PATCH
  `/share/export-options` handler + `archive_meta`
  table shipped in PR #195 commit `845a205`; only the
  templ UI was missing. The checkbox is rendered
  directly under the .ddshare export button on the
  Export & Backup card, defaults to OFF (per the
  #183 locked decision #4), and POSTs to
  `/share/export-options` with `include_tags=1|0`.
  The page reloads on success so the checkbox state
  reflects the new value. A hidden `include_tags=0`
  fallback ensures uncheck sends the right value.
  Regression net: `audit/smoke_share_include_tags.mjs`
  (6 assertions: checkbox present, form action is
  /share/export-options, default unchecked, submit
  fires POST, body contains include_tags=1).
- Browse row tag chips + bulk-tag toolbar (issue #183
  Slice C). Each Person Record row in the browse table
  now shows its applied tags as small rounded chips
  (or an em-dash when none), with a "Tags" column
  header and column-toggle entry. A bulk-tag toolbar
  appears above the table whenever at least one row is
  selected, with a tag_name text input + datalist
  (populated from the existing availableTags cloud) +
  "Apply" button. The form POSTs to `/browse/bulk-tag`
  with `selected_ids` (comma-separated from JS) and
  `tag_name`. The handler at /browse/bulk-tag
  (existing since PR #195) was patched to also accept
  comma-separated selected_ids in addition to repeated
  form fields. The `TagsForSoldiers` batch query
  (existing in TagService) is called from both
  handleBrowse and handleBrowseResults, and the
  viewmodel mapper `PersonRecordsFromModelsWithTags`
  zips tags into each PersonRecord. Mobile card layout
  also renders tag chips. Regression net:
  `audit/smoke_browse_tag_chips.mjs` (15 assertions:
  Tags column header, cells exist, chip rendered,
  em-dash rendered, toolbar hidden/visible, selected
  IDs populated, form elements, POST fires, column
  toggle entry). Pre-fix: 0/15. Post-fix: 15/15.
- Build-tag-gated zero-cost trace instrumentation (issue #218).
  New `internal/debug/trace` package with `trace.Log(msg, attrs...)`
  emits `slog.Debug` calls in `-tags debug` builds and is a
  literal no-op in release (Go compiler dead-code-eliminates
  every call site). Reuses the existing `debug.Configure` handler
  pipeline so trace entries flow into the JSONL log file, the
  in-memory ring buffer, and the Debug Console panel without
  any new infrastructure. Used for high-volume instrumentation
  (entry/exit markers, branch decisions, dup-rejection) where
  the call would lose diagnostic value at `-tags debug` builds
  but has zero narrative value at INFO+ (where `slog.Debug`
  belongs instead). See ADR 0006 for the decision rule. Initial
  proof-of-pattern call sites are in
  `handleCalendar` (`handleCalendar_start`, `handleCalendar_render`).
  Build wiring: `scripts/build-common.ps1` adds `-tags debug` to
  `wails build -debug`; `Makefile` adds `-tags debug` to the
  `web`, `seed`, `gold`, and `tune-bin` targets. CI gains a
  parallel `go test -tags debug ./internal/debug/...` step
  so the no-op stub cannot silently rot.

- Inline expandable stale-template warning list (issue #184).
  When a Load produces ≥2 stale warnings, the modal grows an
  inline `<ul>` next to the templates-status span with a
  "Show details" toggle button. The collapse-and-show pattern is
  the issue's Option A — keeps toasts as the transient signal
  and makes the detail persistent in the modal until the user
  closes it. Single-warning cases still use the single toast
  (no list). JS wires the install-time toggle so the click
  flips aria-expanded + button text between "Show details" /
  "Hide details".
- Per-template stale count badge in the Saved Templates
  dropdown (issue #187). `/export/templates` LIST response
  grows `stale_warning_count` per row, computed in-process
  via the existing `computeExportTemplateStale` helper
  (sub-50ms for typical ≤20-template archives). The frontend
  dropdown appends "(N stale)" to the option text when the
  count is > 0 so users spot stale templates before clicking
  Load. Same refresh helper used post-Update keeps the badge
  in sync after a rename or Save Changes.
- Live preview reflects stale-filter values (issue #185). The
  preview handler now runs the same computeExportTemplateStale
  check the Load handler does, surfaces a one-line warning
  above the count when stale values are present (e.g. "1 stale
  filter value — adjust or remove before generating."), and
  reuses `templateFromSettings` to feed the existing helper
  without duplicating logic. The preview counter and the
  eventual Generate can no longer silently disagree on stale
  filter values. Regression net: TestHandleExportPreview_StaleFilterWarning.
- Live preview response-time stress test (issue #188, measurement
  only). TestHandleExportPreviewResponseUnderThreshold seeds
  5,000 rows (the chosen upper bound for a v1 DixieData
  archive), warms up one POST /export/preview, then measures
  the second request against a 500ms ceiling. First run
  measured 444ms -- within budget but borderline; if this ever
  crosses, that's the signal to invest in caching
  listAllSoldiers or push preview to a background worker
  (per the issue's "if this fails, optimize" instruction).
  Skipped under -short; run via `go test
  ./internal/appshell/...` without -short.
- Saved-templates "Save Changes" button (issue #186): PATCH
  /export/templates/{id} handler + ExportTemplateService.Update
  method (preserves created_at + last_used_at; rejects name
  collision with ErrExportTemplateNameTaken, missing id with
  ErrExportTemplateNotFound). Modal grows a hidden Save Changes
  button that becomes visible after a successful Load;
  selecting a template from the dropdown re-uses Load's id
  (option.dataset.templateId). Frontend JS refreshes the
  dropdown after a successful update so renames surface in the
  sort order without a modal close/reopen. Route registered
  `r.Patch("/export/templates/{id}", a.handleUpdateExportTemplate)`
  in routes.go; typed builder `ExportTemplateUpdate(id)` added.
  Regression net: 3 new service tests (`Update`, `UpdateMissing`,
  `UpdateNameCollision`).
- New-soldier empty-name save is now a soft warning rather
  than a hard 400 (issue #151, follow-up to PR #149). The
  browser-side `required` attribute is removed from both name
  inputs; a JS interceptor in `dispatchDixieDataForm` surfaces
  a single `window.confirm` for empty-name submits and, on
  accept, appends `confirm_empty_name=1` to the FormData.
  `handleCreateSoldier` routes confirmed empty-name saves
  through to a successful INSERT with `NeedsReview=true` and
  `ReviewReason="Saved with no name; researcher should fill
  in."` so the row lands in the review queue. Empty names with
  no confirm marker still return 400 (catches the bypass).
  `handleUpdateSoldier` mirrors the behaviour on the edit path:
  clearing both names on a row that previously had a name sets
  `NeedsReview` with reason "Name cleared during edit".
  Linked-person / wife / widow entry types all carry the same
  logic — the review queue is the single triage surface.
  Regression net: `TestHandleCreateSoldier_EmptyNameMarksForReview`
  with four sub-tests (empty+confirm, first-only, last-only,
  empty+no-marker).
- Person Record tagging: new `tags` and `person_record_tags`
  tables back the upcoming `/tags` management surface and
  Browse chip filter. Tags are flat, free-text labels with
  case-insensitive uniqueness and travel with `.ddshare`
  archives on opt-in. Issue #183 (schema migration v58;
  service + UI land in the following commits).
- **Tag** added to the glossary as a user-defined free-text
  label grouping Person Records. Adds Relationships ("A
  Person Record may have zero or more Tags" / "A Tag may be
  applied to zero or more Person Records") and a flagged
  ambiguity that retires "virtual cemetery" as a generic term.
  Issue #183.
- `internal/records/tag_service.go` (TagService) provides
  UpsertByName (case-insensitive dedup), Attach/Detach,
  AttachMany, Rename (UNIQUE-collision reject), MergeInto
  (moves memberships, deletes source, rejects same-name merge),
  Delete, Get/List, Autocomplete (substring match on
  normalized_name), TagsForSoldier, TagsForSoldiers,
  AttachAdditive, ByIDsPreservesOrder.
- `internal/records/archive_meta.go` (ArchiveMetaService)
  provides Get / SetIncludeTags / IncludeTags on the seeded
  `archive_meta` rows (shared/backup/static). Used by the
  upcoming export-pipeline opt-in (commit 8).
- 13 new unit tests across `tag_service_test.go` and
  `archive_meta_test.go`. Issue #183.
- HTTP surface for tagging (issue #183): 11 endpoints under
  `/soldiers/{id}/tags[/...]`, `/tags[/{id}/...]`, `/browse/bulk-tag`,
  and `/share/export-options`. New handlers in
  `internal/appshell/tags_handlers.go` cover attach/detach,
  bulk-tag, rename, merge, delete, autocomplete fragment, and
  archive_meta toggle. Routes registered in
  `internal/appshell/routes.go` using chi's regex-constrained
  `{id:[0-9]+}` syntax so static patterns precede the existing
  `/soldiers/*` and `/tags/*` wildcards. Every POST writes
  `X-DixieData-Redirect` per issue #130; locked by the existing
  `TestPostThenNavigateUsesDixieRedirect` regression net. New
  typed builders in `internal/routebuilder/routebuilder.go`
  (`TagsPage`, `TagDetail`, `TagRename`, `TagMerge`, `TagDelete`,
  `SoldierTagAutocomplete`, `SoldierTagAttach`, `SoldierTagDetach`,
  `BrowseBulkTag`, `ShareExportOptions`). 11 new handler tests
  in `tags_handlers_test.go` cover happy paths + 400/404/409
  branches; `route_wildcard_test.go` extended with three new
  shadow pairs.
- Person Record tagging UI (issue #183): `internal/templates/tags.templ`
  renders the `/tags` management table (rename / merge / delete
  forms) and the `/tags/{id}` detail table with a View-in-Browse
  deep link. `internal/templates/tag_picker.templ` renders the
  per-soldier picker page reachable by the `/soldiers/{id}/tags`
  GET handler. New uiids surface constants `PageTagsManagement`,
  `PanelTagsList`, `PanelTagDetail`, `OverlayTagPicker` registered
  in `internal/uiids/uiids.go`. `internal/records/tag_service.go`
  gains `MembersWithDetails` for the detail page. Forms use
  `data-dixie-submit="true"` per the Option C retag (no `hx-post`
  / `hx-delete`).
- Browse sidebar tag filter (issue #183): multi-select chip cloud
  inside the existing browse filter row, AND-logic HAVING filter
  (`GROUP BY person_id HAVING COUNT(DISTINCT tag_id) = N`). Deep
  link `?tags=vc-shiloh,unit-4th-al` parses through
  `parseTagFilter` → `BrowseRequest.Tags` and normalises on the
  service layer (TrimSpace + dedupe-by-normalized). New
  `BrowseState.SelectedTagSet` + `viewmodel.TagOption` + `BrowseView`
  surface extended with `availableTags`. 4 regression tests in
  `internal/records/browse_filter_test.go` cover AND logic across
  1/2 tags, unknown-tag no-op, and normalisation dedup.
- Shared Archive tag opt-in (issue #183): `models.Soldier` gains
  a `tags []string` field (`json:",omitempty"` so static archive
  HTML stays unchanged). `ExportSharedWithTags(outputPath, dataDir,
  includeTags)` reads `archive_meta.include_tags` for the shared
  kind and writes the tags array per soldier when on. The shared
  import pipeline gains an additive post-pass that walks the source
  archive and calls `TagService.AttachAdditive` per soldier per
  tag, matching by display_id (inserted rows from merge get a new
  id; matching by the immutable display_id keeps the binding
  deterministic). `backupFacade` interface grew the new method;
  `handleExportSharedArchive` reads `archiveMeta.IncludeTags` at
  dispatch time so a PATCH on `/share/export-options` (issue #183
  c4) takes effect on the next export without restarting.
  Static archive HTML output does not change (Tags is omitempty).
- Audit coverage (issue #183): `audit/discover_export_buttons.mjs`
  registers the four new tag surfaces (`TagsPage`, `TagDetail`,
  `BrowseBulkTag`, `ShareExportOptions`) in both the builder
  prefix map and the literal-path allow-list. `audit/smoke.mjs`
  grows a `[5e]` block asserting `/tags` renders and a `[5f]`
  block that fetches `/share/export-options` and verifies the
  X-DixieData-Redirect target is `/share` (the Option C contract
  for the toggle form).
- **Share Queue** added to the glossary as the in-memory list
  of Person Records a researcher has staged for inclusion in a
  Shared Archive (.ddshare) before exporting. Stored in the
  browser's `localStorage` under the `dixiedata.share-queue`
  key so the queue survives navigation, reloads, and app
  restarts; distinct from the existing
  `dixiedata.browse.selection` (print/export-selection) key
  to keep the two domains disjoint. Issue #182.
- Audit coverage for Share Queue presets (issue #192):
  audit/smoke.mjs gains a [5h] block that GETs
  /share/queue/presets on a live dev binary and asserts the
  response is a JSON object with a presets array, gated
  behind SHAREQUEUE_PRESETS_E2E_BASE so unit-style smokes
  can skip. audit/discover_export_buttons.mjs registers
  the three new preset paths (literal /share/queue/presets,
  /share/queue/presets/1, /share/queue/presets/1/apply) in
  the literal-path allow-list so the discover test doesn't
  fire false-orphan assertions for them.
- Share Queue Saved Queues JS wiring (issue #192): the
  modal's save form / load / delete are now wired.
  refreshShareQueuePresets() runs on openShareQueueModal
  and GETs /share/queue/presets to hydrate the
  per-row Load + Delete buttons. saveCurrentQueueAsPreset
  POSTs the current localStorage queue under the form's
  name field with a 409-conflict message for the modal's
  status slot. loadShareQueuePreset warns-and-confirms
  when the current queue is non-empty, GETs the apply
  endpoint, and writes the returned soldier_ids back to
  localStorage. deleteShareQueuePreset confirms, DELETEs
  the row, and re-fetches the list so the empty state
  re-surfaces.
- Share Queue Saved Queues UI shell (issue #192): the
  Share Build modal grows a "Saved Queues" section above
  the Staged Records panel. Server-rendered shell carries
  the save form (name input + Save current queue
  button), the preset list, the empty-state hint, and the
  status message slot -- all JS-hydrated on modal open
  via GET /share/queue/presets. New uiids constant
  PanelShareQueuePresets. New test asserts every shell
  attribute renders.
- Maintenance: bring the three CI-checked docs into sync
  with the current schema (v1.2.55 -> v1.2.59). The
  bump-version.ps1 -VerifyOnly CI step expects
  user-manual.md, implementation-and-features.md, and
  ai-handoff.md to all reference the current
  1.2.{CurrentSchemaVersion} line; the docs had been
  drifting since v1.2.55 so the last several pushes
  failed the bump-verify check without breaking the
  build. This commit catches the docs back up.
- Share Queue e2e smoke (issue #194): audit/smoke.mjs
  gains a [5j] block that walks the full Share Queue
  subset export flow against a live dev binary --
  navigate to /browse, click [+ Queue] on the first
  row, open the modal via the persistent pill, click
  Export Selected, assert the POST to
  /export/shared-archive?subset=1 fires with
  selected_ids, assert the page lands on /jobs/{id},
  and assert the summary card shows a `Soldiers:` line
  (regression net for commit 342de6b's manifest-counts
  fix). Gated behind SHAREQUEUE_E2E_BASE so unit-style
  smokes can skip when no live binary is booted. The
  dixiedata-web binary's existing SaveFileDialog
  override (cmd/dixiedata-web/main.go) auto-accepts
  the OS picker to DIXIE_SAVE_FILE_DIR so the browser
  never blocks on a real dialog during the e2e walk.
- Audit + templ coverage for /share/queue (issue #193):
  audit/discover_export_buttons.mjs registers
  /share/queue in the literal-path allow-list; audit/smoke.mjs
  gains a [5i] block that asserts the page renders
  (GET /share/queue includes "Manage your staged subset"),
  gated behind SHAREQUEUE_PAGE_E2E_BASE so unit-style
  smokes can skip when no live binary is booted.
  internal/templates/share_queue_test.go: 2 templ tests
  (empty-state copy + per-row attributes + counts).
- Share Queue management page JS wiring (issue #193):
  frontend/app.js grows installShareQueuePage --
  select-all checkbox toggles every row's
  per-row checkbox; per-row Remove drops the id from
  localStorage and re-fetches /share/queue?ids= so
  the table stays in sync with the queue; bulk Remove
  Selected confirm()-gates a multi-id drop; bulk
  Export injects the selected rows into the existing
  form as repeating selected_ids hidden fields so the
  existing dispatchDixieDataForm picks up the submit.
  The pill + bulk-button enabled state mirror the
  current selection so users see at a glance whether
  their action will fire.
- Share Queue management page (issue #193): new
  `/share/queue` page reachable from the layout nav
  (next to Share). Server-renders a table of staged
  Person Records ordered by the `?ids=` query the
  client populates from localStorage; columns include
  Display ID, Name, Unit, Source Records count,
  Images count, Order index, and per-row Remove.
  Empty state mirrors the modal's copy so users who
  haven't staged anything see the same friendly hint.
  Bulk Remove Selected + Export Selected controls
  mirror the modal's UX via the existing
  `data-dixie-submit` path. Route registered BEFORE
  the `/share` wildcard per the existing static-
  before-wildcard rule; 4 handler tests cover empty,
  populated, all-unknown, and route-shadowing paths.
- Share Queue preset HTTP surface (issue #192): four
  new endpoints on the appshell --
  - GET /share/queue/presets — returns the saved presets
    as a JSON array ordered by last_used_at DESC, name
    ASC. Emits `[]` instead of `null` for an empty
    database so the modal's JS can iterate without a
    null-guard.
  - POST /share/queue/presets — saves the current
    queue contents under a `name` field plus a
    repeating `soldier_ids` field. 400 on missing
    name or empty soldier_ids; 409 on duplicate name.
  - DELETE /share/queue/presets/{id} — removes a
    preset. 404 on unknown id; 204 on success.
  - GET /share/queue/presets/{id}/apply — returns the
    preset's soldier_ids array as JSON so the modal's
    Load handler can write it back to localStorage.
    Also bumps last_used_at so the preset floats to
    the top of the Saved Queues section next time the
    modal opens.
  Wired through app.go + routes.go + three new
  routebuilder entries (ShareQueuePresets,
  ShareQueuePresetDelete, ShareQueuePresetApply).
  10 handler tests cover happy paths, duplicate
  names, empty payloads, missing rows, and the
  literal-vs-wildcard route ordering.
- Share Queue preset service (issue #192): new
  `records.ShareQueuePresetService` provides CRUD over the
  v59 share_queue_presets table -- Create / Get / List /
  Delete / TouchLastUsed. Mirrors the printable-export
  template service shape from issue #178 so power users get
  a consistent save/reuse surface across both subset
  pipelines. Create trims the name (leading/trailing
  whitespace can never silently create a "different"
  preset), drops non-positive IDs defensively, and surfaces
  ErrShareQueuePresetNameTaken / ErrShareQueuePresetNotFound
  for the handler to map to 409 / 404. List orders by
  last_used_at DESC, name ASC so recently-loaded presets
  float to the top of the modal. 9 unit tests cover all
  paths including whitespace handling, duplicate names, and
  missing-row semantics.
- Schema v59 — saved Share Queue presets (issue #192):
  new `share_queue_presets` table carries the (soldier_id)
  JSON payload that names a reusable Share Queue. Local-only
  storage (no sync_id, no merge protocol). Pattern mirrors
  the printable-export templates table from v58 so power
  users get a consistent save/reuse shape across the two
  subset surfaces. Bumps CurrentSchemaVersion 58 → 59;
  see docs/migrations/v59.md.
- Share Queue [+] Queue button coverage (issue #191):
  extends the Browse row entry point to three more surfaces:
  the Person Record detail page header (next to Edit /
  Export Record), the Calendar Anniversary compact row (next
  to Open Record), and the Review Queue compare row (next to
  Open Left/Right Person Record). All three use the same
  `data-share-queue-add="{id}"` hook that frontend/app.js
  already handles; no JS changes. Visual style matches the
  Browse row pill (uppercase tracking, thin gold border,
  white-tinted background). Title attribute uses the
  Display ID so the tooltip is informative on hover. Three
  unit tests in internal/templates assert the hook renders
  on each surface.
- Share Queue preview counts hardening (issue #190): the
  preview fragment already summed real per-row RecordCount +
  ImageCount from the soldierListSelectColumns subqueries;
  the prior substring-only test passed even with stubbed
  zeros. This commit replaces the substring check with exact
  integer assertions (e.g. `Source Records: 3`) by attaching
  a known mix of records + images to each staged soldier via
  the new seedPersonRecordWithCounts test helper. Also adds
  TestSoldierService_ByIDs_PopulatesCounts at the service
  layer so a future soldierListSelectColumns refactor that
  swaps the projection for a lighter one gets caught before
  the preview silently drops to zero. **The
  SoldierService.CountForIDs helper described in the issue's
  Implementation sketch was intentionally NOT added**: the
  handler already sums per-row counts from a single round
  trip; introducing a second helper would be a redundant
  query on every modal open and a YAGNI divergence from the
  ByIDs shape the rest of the preview pipeline relies on.
  audit/smoke.mjs gains a [5g] block that asserts
  /share/queue/modal renders (gated behind SHAREQUEUE_E2E_BASE
  so unit-style smokes can skip). audit/discover_export_buttons.mjs
  registers the four new Share Queue paths in the literal-path
  allow-list so the discover test doesn't fire a
  false-orphan assertion for them; they remain out of scope
  for the 'all six canonical share-page exports' regression
  net as the spec requires.
- Share Queue UI (issue #182): the c4 handler stub is
  replaced with the real Share Build modal
  (internal/templates/share_queue_modal.templ). Layout.templ
  gains a persistent Share Queue pill that opens the modal on
  click; visible only when localStorage
  `dixiedata.share-queue` has entries. Browse rows add a
  small `[+ Queue]` button next to the existing checkbox
  (separate visual channel -- does not collide with
  `data-browse-select`). The /share page grows a Build
  Share Archive button that opens the same modal directly.
  frontend/app.js adds the localStorage round-trip, the pill
  visibility toggle, the per-row add/remove handlers, the
  live preview refresh via POST /share/queue/preview, and the
  Clear Queue confirm() wire. The modal's form submits via
  the existing dispatchDixieDataForm + data-dixie-submit path
  to /export/shared-archive?subset=1. New uiids:
  OverlayShareQueue, PanelShareQueueList,
  PanelShareQueuePreview.
- Share Queue HTTP surface (issue #182): four new endpoints
  on the appshell, two of which are unique to #182 and two of
  which extend existing pipelines:
  - GET /share/queue/modal — renders the Share Build modal
    fragment (templates.ShareQueueModal; the c4 stub ships in
    this commit, the full UI in c5).
  - POST /share/queue/preview — given a `selected_ids`
    repeated form, returns an HTML fragment carrying the
    Soldiers/Source Records/Images count summary the modal's
    live-preview pane swaps via showOverlayModal.
  - POST /share/queue/clear — explicit Clear Queue anchor so
    dispatchDixieDataForm has a single 200/OK +
    X-DixieData-Redirect=/share target. The queue itself lives
    in localStorage, so the server side is intentionally a
    no-op.
  - POST /export/shared-archive?subset=1 — new subset branch
    inside handleExportSharedArchive. Parses selected_ids,
    refuses empty (400), runs BackupService.ExportSharedSubset
    on a background job (job kind = "shared_archive_subset"),
    writes X-DixieData-Redirect=/jobs/{id} per Option C
    (issue #130), guarded by a distinct inFlight
    dupKey=`subset|count|firstID` so it never collides with the
    whole-archive export (per docs/agents/dialog-guard.md).
- `BackupService.ExportSharedSubset` (issue #182): writes a
  Shared Archive containing only the Person Records whose IDs
  are in the supplied slice. Mirrors `ExportSharedWithTags`:
  same archive kind, same JSON payload shape, same image-set
  semantics — just a filtered soldier slice. Inherits the
  `archive_meta.include_tags` opt-in for free (subset exports
  honour the same per-kind flag). `manifest.SourceLabel`
  carries a "subset of N Person Records" annotation so the
  recipient can see the archive is not full at a glance.
  IDs that no longer exist are silently dropped (the ByIDs
  contract). `EmptyIDsRejected` + `Roundtrip` regression
  tests cover the guard and a 500-row archive filtered
  down to 5 rows in caller order via the resulting zip.
- `SoldierService.ByIDs` (issue #182): returns the soldiers
  whose IDs are in the supplied slice in caller order, drops
  unknowns silently, and returns a non-nil empty slice for
  empty / all-unknown input. Used by the Share Queue subset
  export to materialise a single staged shipment. Mirrors
  `RecentByIDs` without the implicit limit.
  `TestSoldierService_ByIDs` covers order preservation,
  empty input, and all-unknown input.
- Pension State, Pension ID, and Application ID fields on the
  new-soldier form are now visible for the `wife` entry type
  as well as `soldier` and `widow`. Previously, the JS handler
  at `frontend/app.js` `syncEntryTypeFields` used
  `isSoldierEntryType() || widowEntry` to decide whether to
  show the `data-soldier-or-widow-field` sections, which
  excluded `wife`. The handler now uses
  `isSoldierEntryType() || spouseEntry` (where `spouseEntry`
  already includes both `wife` and `widow`). Linked-person
  remains hidden — that role is not a pensioner. Templ change
  in `internal/templates/entry_form.templ`: the
  `pension_state` `<div>` wrapper moved from
  `data-soldier-only-field` to
  `data-soldier-or-widow-field` so its visibility follows the
  same JS rule. Issue #75.

- Back button on the Browse screen (`/browse`) using the
  existing `data-history-back` machinery. Default fallback
  is `/soldiers`. Issue #172.
- Back button on the Share / Export screen (`/share`)
  using the existing `data-history-back` machinery. Default
  fallback is `/`. Issue #169.
- Styled "Back to Dashboard" exit button on the Jobs
  status page (`/jobs/{id}`) using `data-history-back`.
  Replaces the inline body-copy link that was easy to miss.
  Issue #175.
- Live preview panel for the print-config modal. The
  modal now shows count + first 5 records + active sort +
  active group-by labels for the current scope/filter
  selection, updated within ~200 ms of any form change
  via a debounced `POST /export/preview` request. New
  server handler in `internal/appshell/export_preview.go`
  resolves the same scope/filter logic the actual PDF
  generation uses. Issue #179.
- Save / reuse printable-export templates. Users can now
  persist a print-config snapshot as a named local
  template and recall it later via the modal's new
  Templates section. Storage is a new SQLite table
  `export_templates` (schema v56) with named, JSON-
  encoded filter + group-by columns. CRUD lives in
  `internal/records/export_templates.go`; HTTP routes
  in `internal/appshell/export_templates_handlers.go`.
  The print-config modal gains a Saved Templates
  section at the top: dropdown + Load / Delete +
  name input + Save Current buttons. Load applies
  every field from the JSON response; Save posts the
  modal's full form plus the new template_name input;
  Delete uses the existing data-confirm convention.
  Issue #178.
- Pending-review badge on the Review Queue nav link.
  When one or more records are flagged `NeedsReview`,
  a small review-red badge with the count appears
  next to the link in the top nav. Counts >= 100
  render as "99+". Populated via a new
  `GET /layout/review-count` endpoint that the layout
  polls every 30s with htmx, so the count surfaces
  from any page without per-render DB load. A new
  `CountNeedsReview` method on `SoldierService`
  backs the endpoint. Issue #180.
- Stale-template warnings when loading a saved
  template (issue #181). The apply endpoint now
  cross-checks each stored filter value and selected
  ID against the current archive and returns a
  `warnings` array alongside the template. The
  client surfaces one warning per stale value as a
  toast; for many warnings it pops a single summary
  toast and logs the full list to the browser console.
  Stored SelectedIDs are persisted in a new
  `selected_ids_json` column on `export_templates`
  (schema v57) so scope=selected templates can also
  detect deleted record IDs. Issue #181.
- `smartBackLabel` in `frontend/app.js` now recognizes
  `/soldiers/{id}*` sub-routes (edit, timeline,
  camaraderie, research-log, conflict-ledger, research-pack,
  pdf, jpg) and returns "Back to Person Record" instead of
  the generic "Back". Same coverage extension for `/browse`,
  `/jobs`, `/settings`, `/recovery`. Issue #171.
- "Print/Export Selected" on the Browse screen now opens
  the printable-export picker modal **in place** instead of
  navigating to `/share`. The Browse screen's working set
  (filters, page, sort, selection) is preserved across the
  modal open/close cycle. The modal markup is extracted to
  `internal/templates/partials/print_config_modal.templ`
  and rendered by both `share.templ` and `browse.templ`;
  `BrowseView` now also loads `exportRecords` so the modal's
  filter dropdowns populate when opened from Browse. A new
  stress test `TestHandleBrowseResponseUnderThreshold`
  asserts a 1000-record archive's `/browse` GET stays under
  500 ms so the extra list query never causes perceived
  slowdown. Issue #176.

### Changed

- Success toast now uses a distinct green border
  (`rgba(41, 82, 45, 0.86)`) and faint green background
  (`rgba(242, 252, 244, 0.99)`) instead of the same sepia
  border as the default chrome. The previously-declared
  `success-green` / `success-green-bg` tokens in
  `tailwind.config.js` are now wired into use. Issue #174.
- The off-brand Tailwind `blue-*` classes on the
  research / relationship screens (Camaraderie, Conflict
  Ledger, Research Log / Pack / Collections, Service
  Timeline, plus the matching side-cards on Soldier Detail)
  are replaced with semantic `research-bg`,
  `research-border-soft`, `research-border`,
  `research-accent`, `research-text` tokens added to
  `tailwind.config.js`. Visual output unchanged
  (hex values map to the same Tailwind defaults), but the
  intent ("research-derived content") is now named in code
  and the off-brand classes are no longer the source of
  truth. Issue #173.
- Body background gradient stops are now exposed as
  `bg-sepia-top` / `bg-sepia-mid` / `bg-sepia-bottom`
  tokens in `tailwind.config.js` (ADR-0003). The literal
  hex values stay in `frontend/tailwind.css` because
  tailwindcss `@apply` cannot reach custom gradient stops;
  the CSS comment references the token names. Issue #170.

### Removed

- Dead `sepia-300` token removed from `tailwind.config.js`.
  Zero matches in templates or CSS. Issue #168.
- `openPrintConfigFromQuery` and its two call sites
  removed from `frontend/app.js`. The Browse screen no
  longer uses the `?openPrintConfig=1` query-string
  trigger (the in-place button opens directly), so the
  helper had zero callers. Issue #176.

### Removed

- "Open file" button removed from three surfaces: the
  `jobSummaryCard` on `/jobs/{id}`, the artifact section of
  `/jobs/{id}/report`, and the layout progress slot
  (`job_slot_fragment`). The button POSTed to `/jobs/{id}/open`,
  which calls `runtime.BrowserOpenURL("file:///<path>")` in Wails
  desktop and returns an info toast in web mode. Per user bug
  report, the button does nothing in their runtime. The "Copy
  path" button next to the original (already wired via
  `data-copy-path`) is the reliable fallback in both runtimes.
  Backend handler `openJobArtifact` (`internal/appshell/jobs_handlers.go:269`)
  is kept for any future callers / debug entry points. Two
  regression tests in `internal/templates/jobs_artifact_link_test.go`
  inverted: they now assert the POST form + button are NOT
  rendered. Closes #166.

### Fixed

- `+ Queue` button silently no-opped on Browse, Soldier
  detail, Calendar, and Review Queue rows until the user
  first opened the print-config modal. The
  `document.addEventListener("click", ...)` handler that
  intercepts `[data-share-queue-add]` clicks was registered
  inside `installShareQueueGlobals()`, which was only called
  from `openPrintConfigModal()`. Move the install into the
  `DOMContentLoaded` block in `frontend/app.js` so the
  listener is registered before any user interaction. The
  redundant call in `openPrintConfigModal` is kept as a
  safety net for htmx swap without full page load (both
  functions are idempotent). Regression net:
  `audit/smoke_share_queue_add_button.mjs` (3 assertions,
  live-binary headless browser probe).
- `/share/queue` management page showed the empty-state
  card ("No Person Records staged") even when the
  persistent pill counted 3+ items, because the user
  navigated to the page for the first time after staging
  items via `+ Queue`. The page's `renderShareQueuePage`
  function targeted the `<tbody data-share-queue-page-body>`
  element and early-returned when missing; the server
  renders the empty-state branch (no tbody) on the first
  visit, so the function silently no-opped. The install
  guard had the same bug, so event handlers + the
  installed-flag never persisted across re-renders.
  Refactor: target the section
  (`id="panel.share-queue.list"`) which is always present.
  Fetch `/share/queue?ids=N` and replace the section's
  inner contents with the fresh section's inner contents;
  this handles both empty→populated and populated→empty
  transitions in one code path. Regression net:
  `audit/smoke_share_queue_page.mjs` (5 transitions, 7
  assertions, live-binary headless browser probe).
- "Select all" checkbox on `/share/queue` did not toggle
  per-row checkboxes after the first render. The
  `installShareQueuePage()` function attached the change
  handler directly to the selectAll element, which lives
  inside the section. `renderShareQueuePage()` calls
  `section.replaceChildren(...)` on every render, which
  detaches the original selectAll from the DOM; the
  listener was stranded. The per-row checkbox change +
  per-row Remove click were already section-delegated
  (PR #241), but select-all was the lone direct listener
  — and the one that broke. Move the select-all change
  handler into the existing section-level `change`
  delegation. All event listeners on the section survive
  `replaceChildren()` because the section itself is the
  same DOM node. Regression net:
  `audit/smoke_share_queue_select_all.mjs` (7 assertions,
  live-binary headless browser probe).
- "Ignore Selected" and "Delete Selected" buttons on
  `/review-queue` returned "Unknown bulk action. Use
  ignore or delete." regardless of which button was
  clicked. The form has two submit buttons sharing the
  name `bulk_action`. `dispatchDixieDataForm` built the
  request body as `new FormData(form)` (no submitter
  argument); per the WHATWG spec, `new FormData(form)`
  silently drops submit-button values when the form is
  not actually submitted (which it isn't — the JS path
  uses `fetch`, not `<form>.submit()`). In practice the
  browser honored the submitter argument in some
  contexts (a direct call) but not in the synthetic
  fetch path, so a single-line `new FormData(form,
  button)` was insufficient. Fix: pass the submitter to
  FormData AND, as a belt-and-suspenders fallback,
  manually append the submitter's name+value to the
  FormData if the browser omitted it. Regression net:
  `audit/smoke_review_queue_bulk.mjs` (4 assertions,
  live-binary headless browser probe: synthetic form
  with 2 checkboxes + 2 submit buttons, intercept
  fetch, dispatch real submit event, assert body
  includes `bulk_action=ignore`).
- "Mark as Resolved" button on each `/review-queue`
  entry row also returned "Unknown bulk action." The
  button carries `data-action="/soldiers/{id}/review/
  resolve?context=queue"` but lives INSIDE the
  bulk-action form. `dispatchDixieDataForm` only
  honored `data-action` for bare buttons (no parent
  form); inside a form, the form's action won and the
  fetch hit `/review-queue/bulk` with no `bulk_action`
  field. Fix: `dispatchDixieDataForm` now respects
  `data-action` (and `data-method`) when present,
  regardless of parent form. The synthetic-form
  fallback path (used when no `data-action`) is
  unchanged. Also gated `new FormData(form, button)`
  on the button being a real submit button of the form
  (`type="submit" && button.form === form`); passing a
  `type="button"` trigger throws "not a submit button"
  in Chromium. Regression net:
  `audit/smoke_review_queue_resolve.mjs` (5 assertions:
  per-row click hits `/soldiers/42/review/resolve?
  context=queue` via POST, not `/review-queue/bulk`).
- Dismiss button on `/jobs/{id}` always navigated to
  `/share` (or the kind-specific fallback) instead of
  the page that triggered the job. The templ rendered
  a hard-coded `onclick="window.location.assign(<fallback>)"`
  and the docstring on `Job.DismissTargetPath()` said
  "Issue #131 prefers the referring page, but the
  referer is never saved, so we always use the kind
  fallback." Fix: the templ now renders
  `<button data-dismiss-job data-dismiss-target="<fallback>">Dismiss</button>`
  and the JS handler at DOMContentLoaded prefers
  `document.referrer` when it is same-origin and not
  a `/jobs/*` path (avoids cross-job navigation loops);
  otherwise it falls back to the templ-provided target.
  The query string is preserved on the referer path.
  Regression net: `audit/smoke_jobs_dismiss_button.mjs`
  (5 assertions: referer wins, /jobs referer falls back,
  off-origin falls back, empty referer falls back,
  query string preserved).
- "Mark as Resolved" on `/review-queue` silently no-opped
  after server-side success: the item was removed from
  the queue but the page didn't refresh, the top-nav
  badge didn't update, and the confirmation toast didn't
  appear until the user navigated away. Root cause: the
  per-row handler returns empty body + no
  X-DixieData-Redirect + only the toast header, so the
  JS path was stuck between "do nothing" and "show the
  toast on the next page load" (savePendingToast). Fix:
  a new `data-reload-on-success="true"` attribute on the
  button opts the dispatch into a new branch that shows
  the toast immediately and calls
  `window.location.reload()`. The reload re-runs the
  page-load initializers (which re-fetch the badge via
  `/layout/review-count`) and the next-paint state
  reflects the resolve. Synthetic forms built from a
  button's `data-action` (PR #248) now copy the
  button's other `data-*` attributes so this works for
  inline buttons that live inside a parent form.
  Regression net: `audit/smoke_review_queue_resolve_reload.mjs`
  (6 assertions: fetch hits the resolve endpoint via POST,
  reload fires, final URL is /review-queue).
- `.ddshare` import summary card no longer reads "Duration: 0s"
  when the import dedups every record (issue #246). The worker
  now counts content-equivalent skip-unchanged branches via
  a new `SharedImportSummary.SoldiersSkipped` field, plumbed
  through `handleImportSharedArchive` into `JobResult.Skipped`.
  The render path in `appendSharedImportStats` already
  supported a Skipped-only line; the data now arrives.
  Regression net: new unit test
  `TestBackupService_ImportSharedBackupReportsSkippedWhenAllDuplicates`.
- `/jobs/{id}` summary card for `shared_archive_subset` now
  reports Person records / Images / Source records counts
  (issue #245). Previously fell through to the default
  branch which only printed Size + Duration, leaving the
  user to open the `.ddshare` in another tool to see what
  they sent. New `case "shared_archive_subset":` in
  `summarizeJob` reuses `appendExportStats` and
  differentiates the headline ("Subset shared archive
  complete.").
  Regression net: extension to
  `TestSummaryRendersExportStatsConditionally`.
- Share Queue is cleared after a successful `.ddshare`
  subset export (issue #244). New
  `data-clear-share-queue-on-success="true"` attribute on
  the export forms (page form on `/share/queue` and the
  Share Build modal form) opts the dispatch into a new
  `dispatchDixieDataForm` branch that, on
  `responseOk && !redirectTo`, calls `writeShareQueue([])`,
  re-renders `/share/queue` to the empty state, hides the
  pill, and shows the server-provided toast immediately
  (not via `savePendingToast` because the success path
  does not need a deferred toast). Failed exports leave
  the queue intact.
  Regression net: `audit/smoke_share_queue_clear_after_export.mjs`
  (7 assertions: page + modal forms have the attribute,
  fetch hits `/export/shared-archive?subset=1` via POST,
  localStorage cleared on success).
- Main screen no longer blanks out on first load. The review-queue
  badge wrapper in the top nav (`<span data-layout-review-count
  hx-get="/layout/review-count" hx-trigger="load, every 30s"
  hx-swap="innerHTML">`) inherited `hx-target="body"` from the
  shell `<body>` element because `frontend/index.html`'s load
  trigger left `hx-target="body" hx-swap="outerHTML"` on body and
  innerHTML replacement preserves body attrs across the swap.
  When the badge's load trigger fired during the initial
  `/calendar` swap, htmx walked up the DOM and resolved the
  target to `<body>` — then the badge's innerHTML swap replaced
  the entire body's contents with just the badge fragment. User
  saw a blank page with only the small "2" pill in the top-left.
  Two-part fix: (a) drop `hx-target="body" hx-swap="outerHTML"`
  from the shell `<body>` in `frontend/index.html` — htmx's
  default `innerHTML` swap on the trigger element achieves the
  same visual result (outerHTML on body upgrades to innerHTML
  per the htmx docs anyway) without leaving a polluting
  `hx-target` attr on body; (b) declare `hx-target="this"` on
  the badge wrapper so it never inherits from any future shell
  change. Regression net: `TestLayoutReviewCountBadgeTargetsItself`
  in `internal/templates/layout_test.go` pins both invariants on
  the rendered HTML — fails with the exact wrapper snippet if
  (b) regresses, and fails with the offending body tag snippet
  if (a) regresses. `docs/COMMON_BUGS.md` §1.12 documents the
  pattern. Issue #180 follow-up.

- Main screen no longer cascades into an infinite stack of layout
  shells when the local archive is still starting up. The startup
  placeholder (`renderStartupPlaceholder` in
  `internal/appshell/app.go`) is what `App.ServeHTTP` returns when
  `a.mux == nil` — the brief window between the Wails process
  starting and the chi router being ready. The placeholder is a
  full HTML document, so any htmx fragment request that landed
  during this window (`/jobs/active`, `/layout/review-count`,
  `/jobs/{id}/status`) innerHTML-swapped the full placeholder into
  a tiny target region. The placeholder body carried
  `hx-get="..." hx-trigger="load delay:700ms" hx-target="body"
  hx-swap="outerHTML"`. htmx processed the inner body's triggers,
  fired a GET, and — if mux was still nil — outerHTML-swapped yet
  another placeholder on top, whose own triggers fired 700ms
  later. Each cycle stacked a fresh `<div class="app-shell">`
  inside the previous one, eventually producing 77 nested copies
  of the entire layout and 738KB of body innerHTML. The user saw
  the layout chrome cascading diagonally across the screen with
  the scrollbars shrinking toward zero until the system ran out
  of memory. Two-part fix in `renderStartupPlaceholder`:
  (a) detect the fragment request via the `HX-Request` header
  and return `204 No Content` instead of the full HTML doc, so
  fragment polls become harmless no-ops during the pre-mux
  window; (b) drop the `hx-get` / `hx-trigger` / `hx-target` /
  `hx-swap` attributes from the placeholder's `<body>` so the
  full-page request path (initial `/` load, meta refresh
  fallbacks) also cannot cascade — the meta refresh header and
  the inline `window.location.replace` script already cover the
  retry mechanism. Regression net:
  `TestRenderStartupPlaceholderReturns204ForHtmxFragmentRequests`
  in `internal/appshell/app_test.go` pins the 204 status on
  htmx-fragment requests; the existing
  `TestAppServeHTTPStartupPlaceholderAutoRefreshesWithoutMux`
  gained a new block that asserts none of the four htmx
  trigger attrs appear in the placeholder body, with the
  cascade bug named in the failure message.
  `docs/COMMON_BUGS.md` §1.13 documents the pattern. Artifacts:
  `uibug.png`, `uibug2.png` (in repo root, captured during the
  debugging session).

- Floating dock (Scratch Pad / Feedback / Menu) no longer overlaps
  page content on `/compare`, `/calendar`, `/browse`, or the deep
  soldier routes. `applyResponsiveLayout` now measures the dock's
  rendered height via `getBoundingClientRect()` and writes the
  result to both the `--floating-dock-height` CSS variable on
  `<html>` AND `.app-shell` `padding-bottom` directly. The CSS
  variable is exposed in `frontend/index.html`'s inline `<style>`
  (the Tailwind minifier strips unused `:root` variables, so the
  declaration lives outside the scanned CSS bundle). The direct
  `padding-bottom` write is the binding effect that prevents overlap
  today; once the build pipeline gains CSS-variable awareness the
  direct write becomes redundant. Historical baseline padding values
  (7.5rem / 9rem / 9.5rem) preserved as the relaxed-mode default;
  the JS measurement only kicks in when the dock grows (split-screen
  wrap). Per `docs/COMMON_BUGS.md §4.14` this is the 6th attempt at
  fixing dock-vs-content spacing; the JS-measured value is the
  prescribed single source of truth. Closes #160 (audit r1 top-2).
- Browse mobile `[+ Queue]` button now meets the WCAG 2.5.5
  44×44 minimum tap target and carries a `title="Add <DisplayID>
  to share queue"` hover/AT label (issue #202). The desktop table
  row already had the `title`; this adds `min-h-[44px] min-w-[44px]`
  and the title to the mobile card to match. The button still
  inherits its 11px label size; the tap region is the invisible
  hit area, not the visible glyph, so the visual density is
  unchanged on small screens.
- Browse mobile card `<dt>` labels (Type, Rank Out, Unit,
  Pension State) bumped from `text-[0.65rem]` (≈10.4px) to
  `text-[0.75rem]` (12px). The audit r3 finding
  ('Browse-row text labels render <24×24 on mobile') tracked
  in `docs/SERVICES.md:217,839` labelled the issue as a tap-target
  concern (WCAG 2.5.5), but the elements are non-interactive
  `<dt>` labels so the real defect was readability. The fix
  addresses the actual readability problem; the audit finding
  can be retired after this lands. The `<dd>` values remain
  at the inherited `text-xs` (12px) so label/value contrast
  is preserved.
- `AGENTS.md` and `.github/copilot-instructions.md` corrected
  to reflect that `internal/templates/*_templ.go` files are
  **gitignored** (regenerated by `make tpl` locally and by CI
  before `go test`/`audit` runs), not checked in as the old
  guidance stated. CI workflows `test.yml:42` and `audit.yml:61`
  both already invoke `templ generate`; the docs now match the
  reality so AI agents and humans don't try to commit the
  generated output.
- `internal/jobs` Registry Shutdown is now safe against
  re-entrant calls (test cleanup patterns call Shutdown once
  in the test body and once in `t.Cleanup`). The previous
  implementation launched a fresh `Wait` goroutine on every
  call; when the second call hit Wait after the first Wait
  goroutine returned but a new `Start` was still landing its
  `workerWG.Add(1)`, the sync runtime panicked with
  'WaitGroup is reused before previous Wait has returned'.
  Two changes: (a) `workerWG.Add(1)` now lands in the
  caller goroutine *before* `go func()` is spawned, in both
  `Start` and `StartManual`, so the WaitGroup counter is
  incremented synchronously with Start and a concurrent
  Shutdown's Wait always sees the correct count; (b) a
  `sync.Once` in Registry ensures only one Wait goroutine
  ever runs across the registry's lifetime, and second-and-
  later Shutdown callers attach to the same done channel.
  Regression net: `go test -count=20 ./internal/appshell/...`
  passes consistently; previously failed with the WaitGroup
  panic ~1-in-3 on CI.
- `internal/appshell/jobs_handlers_test.go` `seedArtifactJob`
  helper rewrote its jobID handoff from `atomic.Value` to a
  buffered channel. The old pattern raced: the worker
  goroutine could fire before `Start` returned, in which case
  `jobIDHolder.Load()` returned `nil` and the unconditional
  `.(string)` type assertion panicked. After the first
  attempt at a fix (nil-guard + early return), the worker
  silently completed without setting `ResultPath`, and the
  downstream test got 409 instead of 200 with a missing
  Content-Disposition header. The channel-based fix has the
  worker block on `<-idCh` until the test goroutine writes
  the id after `Start` returns — synchronised by construction
  and impossible to lose. Regression net:
  `TestHandleJobArtifactAttachmentForDownloadTypes` (the test
  that surfaces this race most reliably) now passes 20/20.
- `internal/appshell/recover_test.go` panic value tagged
  with `[recover_test]` so cross-test log grep is unambiguous.
  The previous `'synthetic calendar PDF crash'` literal could
  be mistaken for a real calendar-export error log entry from
  a sibling test in the same package run. The panic itself is
  always recovered by `recoverMiddleware` (no leak); the tag
  is for log-readability only.

### Fixed

- Polling fragments no longer cascade during `pendingRecovery` and
  `startupErr` blocks. Same shape as the #212 setup-required fix.
  In `internal/appshell/lifecycle.go`, the `pendingRecovery` branch
  (`a.pendingRecovery != nil && !recoveryRequestAllowed(r.URL.Path)`)
  used to 303 every non-allowlisted path to `/recovery`; the
  browser's XHR followed, the full recovery HTML doc got
  innerHTML-swapped into the badge wrapper (`hx-target="this"`),
  and the wrapper's innerHTML became a copy of the recovery form on
  every poll. Same class for `startupErr`: every request returned
  `http.Error(..., 500)` with a `text/plain` body containing the
  raw Go error message; htmx fragments swapped the error text into
  the badge wrapper. Two-part fix in both branches: detect
  `HX-Request: true` and return `204 No Content` with
  `X-DixieData-Redirect: /recovery` so the swap target stays put.
  Full-page nav (no `HX-Request`) still gets the 303 (recovery) or
  the 500 (startupErr) — existing behavior unchanged. Forward-
  compatible: any future polling fragment automatically gets the
  204 behavior without needing a `recoveryRequestAllowed` allowlist
  entry. Regression net: two new tests in
  `internal/appshell/app_test.go` — `TestAppServeHTTPRecoveryFragmentReturns204WithRedirectHint`
  (6 cases incl. allowlist sanity + priority over setupRequired)
  and `TestAppServeHTTPStartupErrFragmentReturns204WithRedirectHint`
  (4 cases). Acceptance verified by removing each guard in turn:
  each test fails with the bug-class name in the failure message.
  `docs/COMMON_BUGS.md §1.13` extended with the multi-branch
  pattern. Closes #214.

- Initialisation failure recovery: three coordinated fixes for the init
  path (issue #219). (A) `initializeLocalData` is now transactional:
  rename → reopen → cleanup with rollback on failure. If `reopenDatabase`
  fails, the old data dir is restored and `setupRequired = true` redirects
  every subsequent request to `/setup`. On Windows where `os.Rename`
  may fail due to persistent file handles, the init falls back to the
  old `os.RemoveAll` (log-and-continue). (B) `handleSettingsInitialize`
  re-renders on error: htmx form POSTs redirect to `/setup` via
  `X-DixieData-Redirect`, full-page requests get a Layout-wrapped error
  page via `respondErrorPage`. (C) New `respondErrorPage` method on `*App`
  renders full-page errors through the Layout wrapper with a "Back to
  Setup" recovery link when the DB is gone. New `internal/templates/
  error.templ` template is DB-free (no `models.*` or service calls).
  Regression net: 5 new tests in `internal/appshell/app_test.go` —
  `TestInitializeLocalData_RestoresDataDirOnOpenFailure` (Unix only),
  `TestHandleSettingsInitializeErrorReRendersPage`,
  `TestHandleSettingsInitializeErrorHtmxRedirects`,
  `TestRespondErrorPageFullPageRendersLayout`,
  `TestRespondErrorPageFragmentToastOnly`. Closes #219.

### Maintenance

- Extracted `blockIfFragment` helper for the HX-Request
  fragment-204 contract (`internal/appshell/fragment_guard.go`).
  Four call sites collapsed from inline `r.Header.Get("HX-Request")
  == "true"` blocks to single-line calls: `renderStartupPlaceholder`
  (`internal/appshell/app.go`, pre-mux, no redirect hint),
  `setupRequired` branch (`internal/appshell/lifecycle.go`, hint
  `/setup`), `pendingRecovery` branch (`lifecycle.go`, hint
  `/recovery`), and `startupErr` branch (`lifecycle.go`, hint
  `/recovery`). The helper is the single source of truth for the
  contract — a future contributor adding a fifth blocked branch
  can grep `blockIfFragment` and see every guarded branch in
  one hit. Regression net: `TestBlockIfFragment` (7-case table-
  driven test in `internal/appshell/fragment_guard_test.go`)
  pins the helper's contract (HX-Request → 204, no header → no
  change, nil request → no change, empty redirectTo → no header,
  caller pre-set header → helper overwrites). All four existing
  integration tests for the blocked branches still pass without
  modification. Closes #215.

### Fixed

- `.ddbak` import no longer fails with `Access is denied` on
  Windows when a transient handle is held to the target data
  dir. `replaceDataDir` (`internal/archive/backup_service.go:1182`)
  used to call `os.Rename(targetDir, backupDir)` once and return
  the error on failure. On Windows the rename is blocked when
  OneDrive (`OneDrive.exe`), Windows Search (`SearchHost.exe` /
  `SearchIndexer.exe`), the Wails asset-server watcher, or an
  editor preview tab has any descendant open. Two-part fix:
  (a) skip the rename entirely when the target is logically
  empty (no DB, or DB < 64KB = schema-only) — the user has
  nothing to back up; the empty target is `os.RemoveAll`'d and
  staging is promoted directly. (b) retry the rename with
  exponential backoff (5 attempts, 4 sleep periods of
  200/400/800/1600ms = 3s total wait time) so transient handle
  conflicts have time to release. Each failed attempt is logged
  via `log.Printf` so an extended retry shows up in the JSONL
  log as a chain of "rename attempt N/M failed" entries.
  Regression net: `TestReplaceDataDir_HandlesEmptyAndLockedTargets`
  in `internal/archive/backup_service_test.go`, 7-case table-
  driven test exercising empty/non-empty targets, transient
  retry, all-retries-fail, and rollback. The retry uses a
  package-level `renameOS` var so the test can inject synthetic
  failures without needing real Windows handle conflicts.
  Acceptance verified by removing each piece in turn: each
  test case fails with the bug-class name. `docs/COMMON_BUGS.md`
  new section (added in this commit) documents the Windows
  rename-handle pattern. Closes #216.

- Jobs status page no longer says "Export failed." for an
  import error. `internal/templates/jobs.templ:190,196` printed
  the literal "Export failed." / "Export cancelled." for ANY
  errored or cancelled job, regardless of `job.Kind`. After
  the `replaceDataDir` rename failure in #216, the user saw
  "Export failed. import failed: rename …" — confusing
  because the second line is the real error but the first
  line claims the export was the failing operation. Fix: new
  `FailedVerb(kind, cancelled)` helper in
  `internal/jobs/jobverbs.go` returns the right verb based on
  kind — imports → "Import failed." / "Import cancelled.",
  exports → "Export failed." / "Export cancelled.", unknown
  → "Operation failed." / "Operation cancelled." The kind
  list mirrors the existing `DisplayLabel` helper with a
  cross-reference comment in both docstrings so kind-list
  drift is caught in code review. Regression net:
  `TestFailedVerb` (36-case table-driven test in
  `internal/jobs/jobverbs_test.go` covering 17 known kinds × 2
  states + 2 unknown-kind cases) pins the helper's contract;
  `TestJobStatusViewErrorLabelByKind` (5-case integration
  test in `internal/templates/jobs_error_label_test.go`)
  exercises the templ end-to-end and asserts that
  `backup_import` does NOT render "Export failed." in the
  HTML. Acceptance verified by changing the helper's switch
  to misclassify imports as exports: the integration test
  fails with the bug-class name. Closes #217.

- `internal/confederatehomestatus.Normalize` used to silently rewrite any unknown status value to "N/A" (the default branch fell through to the N/A case). Real bug, surfaced while reviewing issue #23 (schema-level normalization cleanup). Effect: (a) a user filtering browse by a non-canonical value like "Resident" got 0 results because the filter got normalized to "N/A"; (b) any non-canonical stored value (legacy data, imported backups, direct SQL) was silently re-bucketed as "N/A" on the next browse. Mirrored the pattern in `internal/pensionstate/pensionstate.Normalize` which was already correct: unknown values now pass through (trimmed); only the documented legacy "not applicable" variants ("", "none", "na", "n/a", "not recorded") collapse to the canonical N/A bucket. Three new tests in `internal/confederatehomestatus/confederatehomestatus_test.go` pin the contract for canonical, legacy, and unknown values. `go test ./... -short` passes; the existing browse filter test (which inserts a "Resident" row and expects 3 N/A matches out of 4) still passes because the SQL CASE was already correctly preserving stored values \u2014 only the Go function on the filter-input path was wrong. Issue #23 (partial).

- Three pre-existing audit-workflow gaps closed together with the
  pkg/render build-tag fix (`8503f3a`):
    1. **Missing templ-generate step.** `internal/templates/*_templ.go`
       is gitignored (generated from .templ source), so a fresh CI
       checkout has only the .templ files plus the plain .go files.
       The plain .go files (e.g. `linked_text_support.go`) call
       helpers like `isSoldierEntry` that are defined in the
       generated `*_templ.go` files. Without regenerating templ, the
       audit workflow's `go build` failed with
       `undefined: isSoldierEntry`. The test workflow already had
       this step (test.yml:39-44); audit was missing it. Added a
       `Regenerate templ files` step that runs
       `go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate`
       before the build step.
    2. **Path filter too narrow.** The pull_request trigger was
       limited to `internal/templates/**`, `frontend/**`,
       `audit/**`. Changes to `pkg/`, `cmd/`, or
       `.github/workflows/audit.yml` itself did NOT trigger the
       audit. The pkg/render fix (PR #153) demonstrated this: the
       audit never ran on the branch, so the fix landed without
       CI confirmation. Expanded to `internal/**`, `pkg/**`,
       `cmd/**`, `frontend/**`, `audit/**`,
       `.github/workflows/audit.yml`. The audit workflow is now
       self-triggering on its own edits.
    3. **Server port mismatch.** The `Start dev server` step
       polled `http://localhost:8080/` for readiness, but the
       server command was
       `./build/bin/dixiedata-web -scratch-dir .scratch/webmode`
       — the default port is 8765 (see cmd/dixiedata-web/main.go),
       so the wait-for-readiness check timed out after 30s and the
       workflow failed even though the server was running. Added
       `-addr 127.0.0.1:8080` to the boot line. The poll loop was
       already correct; it was just polling the wrong port.

  All three changes match the existing test.yml pattern. Verified
  end-to-end: `gh workflow run audit.yml --ref
  fix/audit-workflow-templ-generate` returns `success` after the
  three fixes. Risk: low. `templ generate` is idempotent. Broader
  path filter increases audit CI usage — a few extra runs per
  PR, but each is Ubuntu + cached deps + takes ~3-4 minutes.

- CI audit workflow on ubuntu-latest was failing because
  `pkg/render/renderers.go` referenced `syscall.SysProcAttr{
  HideWindow: true, CreationFlags: 0x08000000}` directly. Those
  fields only exist on Windows; the Linux build failed with
  `unknown field HideWindow in struct literal of type
  "syscall".SysProcAttr`. The runtime.GOOS check inside the
  function body did NOT help \u2014 Go still type-checks the literal
  on every platform, so the Linux compile unit failed even though
  the function was never called on Linux.

  Split `hideWindow` into two files with build tags, matching
  the established convention in `internal/archive/pdfium_{windows,
  nonwindows}.go`:
    - `pkg/render/renderers_windows.go` (`//go:build windows`):
      real Windows impl that sets `SysProcAttr{HideWindow: true,
      CreationFlags: CREATE_NO_WINDOW}`.
    - `pkg/render/renderers_nonwindows.go` (`//go:build !windows`):
      no-op stub with the same signature, for Linux + macOS.

  The cross-platform test `TestHideWindowExistsAllPlatforms` in
  `pkg/render/renderers_build_tags_test.go` pins the contract
  from the caller's perspective. A second Linux-only test
  `TestRenderersBuildTagsLinux` in `renderers_build_tags_linux_test.go`
  (`//go:build linux`) exists so a future contributor who
  removes the `//go:build !windows` tag from the non-Windows
  stub will see the file fail to compile on the audit workflow's
  Linux runner. Verified: `GOOS=linux GOARCH=amd64 go build ./...`
  succeeds; `go test ./... -short` on Windows passes.

  Root cause pattern: a single commit (`6f096e9`) added
  Windows-only code to a non-Windows-tagged file. The audit
  workflow has been failing on every PR since then, but the
  failure was masked because `test` and `build` workflows run
  on Windows and pass. The fix is the `//go:build` split, but
  the broader design principle is: any time you reach for
  Windows-specific `syscall` fields, the call goes in a
  `{name}_windows.go` file. `docs/agents/dialog-guard.md` and
  the new `pkg/render/renderers_{windows,nonwindows}.go` files
  make this explicit. Pattern: `internal/archive/pdfium_{windows,
  nonwindows}.go` (added 2026-05-30 in `2839768`).

### Fixed


- CI `test` workflow started failing on
  `TestHandleJobArtifactAttachmentForDownloadTypes` (`.csv` case
  returned 409 instead of 200) and
  `TestJobReportHandlerReturnsSummaryForFinishedJob` (report body
  missing "Backup archive complete"). Two related races that
  earlier ran reliably on fast runners but tripped on the GitHub
  Actions runner once `ec451f4`'s reloadServices change altered
  the registry re-allocation timing:
    1. `seedArtifactJob`'s worker closure captured `id` by
       reference and raced the `id = app.jobs.Start(...)` assignment
       below. On a fast worker pool the goroutine fired while `id`
       was still `""`, `SetResultPath("")` was a no-op, and the
       subsequent GET returned 409 because the snapshot had no
       ResultPath. Fixed by binding the ID into the closure via an
       `atomic.Value` indirection so the worker reads the value
       assigned *after* `Start` returns.
    2. `TestJobReportHandlerReturnsSummaryForFinishedJob` fired
       the report endpoint synchronously after `Start`, racing
       the worker goroutine's transition to `StatusDone`. Fixed
       by polling for `StatusDone` (with a 2s ceiling) before the
       request.
  Both fixes are test-only; the production code path was always
  correct.
- `openJobsRegistry(dataDir)` ran before `db.Open(dataDir)`
  created the parent directory, so on a fresh install the jobs
  JSONL log silently failed to open and every job state change
  was dropped until the next app restart (which then saw no log
  and started empty). Added an `os.MkdirAll(dataDir, 0o755)`
  before opening the log so the persistence layer is actually
  wired on first launch.

- Several in-progress toast messages and progress-label
  attributes shipped the seven-char ASCII literal `\u2026`
  instead of the actual U+2026 HORIZONTAL ELLIPSIS rune
  (issue #135). Go does **not** interpret `\uXXXX` inside
  ordinary double-quoted strings — it ships the raw bytes
  `\`, `u`, `2`, `0`, `2`, `6` verbatim. The browser then
  surfaces mojibake like `Shared archive import startedâ¦`
  on the toast. Fixed 10 occurrences across `imports_handlers.go`,
  `google_handlers.go`, `insights_handlers.go`,
  `reviews_handlers.go`, `settings_handlers.go`,
  `entry_form.templ`, `recovery.templ`, and `soldier_card.templ`
  by replacing the broken escape with the actual `…` character
  in the source. Added a source-level regression net
  (`TestInProgressToastStringsContainActualEllipsis`) that walks
  every production `.go` file under `internal/appshell/` and
  fails the test if any non-comment, non-backtick-raw-string
  line contains the seven-char literal `\u2026`. Backtick raw
  strings are exempt because the JS engine resolves the escape
  at runtime — the broken form only affects Go double-quoted
  string literals.

- `reloadServices()` was unconditionally replacing `a.jobs` with a
  fresh empty `jobs.Registry`, which silently dropped every job in
  two contexts:
    1. **App startup**: `lifecycle.go` had already wired the
       persistent `jobs.jsonl` rehydrated Registry into `a.jobs` on
       line 141; `reloadServices()` then ran on line 210 and
       discarded it, so the `jobs.jsonl` persistence layer was
       effectively dead code — no job that survived a previous
       session ever appeared in the new session's registry.
    2. **`.ddbak` restore**: `handleImportBackup`'s worker calls
       `a.reopenDatabase()` after replacing the data dir.
       `reopenDatabase()` runs `reloadServices()`, which used to
       replace `a.jobs` while the `backup_import` job was still
       running. The user landed on `/jobs/{id}` (rendered fine from
       the pre-reload registry), but the 2s `hx-get` poll against
       `/jobs/{id}/status` started returning 404 the moment the
       reload happened — the page logged a flood of
       `htmx:responseError` events and never showed the final
       summary card, even though the import itself succeeded.
  Now `reloadServices()` preserves `a.jobs` when one is already
  wired and only allocates a fresh Registry on the very first call
  (the test-bypass-startup path where `NewApp()` leaves it nil).
  Two regression nets pin the contract: pointer-identity check
  across multiple reloads and an in-flight `Start`-then-reload
  round-trip that asserts `Get(jobID)` still returns ok.

- Shared import re-copied every image and inflated the
  `ImagesUpdated` counter on a full-duplicate archive (issue
  #136). The job report surfaced `Images inserted: 1140` even
  though every Person Record was filtered as a duplicate and no
  net change happened. Two fixes:
    1. `copySharedImageFile` short-circuits when the target file
       already exists with the same byte count. Sharded image
       filenames are derived from content hashes, so size-equal
       means same content for any well-formed export. Avoids
       touching the file on disk and keeps mtime stable.
    2. `upsertSharedImage` now compares the pre-update
       `file_name`, `file_path`, `caption`, and `is_primary`
       columns against the incoming row and only flags the row
       as `changed` when at least one of those columns differs.
       The merge loop only increments `summary.ImagesUpdated`
       for changed rows. Memorial import call site updated for
       the new 3-return-value signature.
  Regression net: new
  `TestBackupService_ImportSharedBackupImageDedup` builds a
  source archive with one image, imports it twice into the same
  target, and asserts the second import reports zero inserts /
  zero updates AND the on-disk file's mtime is unchanged
  (`os.Chtimes` to a stable time, then `time.Time.Equal` after
  the second import).

- Share → "Export Feedback Log" button appeared to do nothing on
  click (issue #137). The handler returned a 200 response directly
  while `dispatchDixieDataForm` only writes the response body into
  a target div when the form opted into `data-results-target` (added
  by the issue #134 fix). For every other click target the dispatcher
  stashed the toast in `sessionStorage` but never re-rendered it
  because the success path never invoked `initializeDynamicContent`.
  The `/export/feedback-log` surface now mirrors the Bug Report
  Bundle pattern (`handleExportBugReport`): the file copy runs
  inside `enqueueExport` and the user lands on `/jobs/{id}` for a
  progress card + final summary, matching every other export on
  `/share`. The no-feedback-yet branch returns a 200 +
  `X-DixieData-Toast` header so the dispatcher renders the
  empty-state toast on the share page. Regression net: new
  `TestHandleExportFeedbackLogEmptyState` pins the empty-state
  response shape (toast header, info kind, no redirect), and the
  existing smoke assertions on `/export/feedback-log` continue to
  verify the success path through `enqueueExport`. The carve-out
  comment in `audit/smoke.mjs` for the feedback-log path was
  updated to reflect the new behaviour (the carve-out still
  applies to the empty-log case, which legitimately stays on
  `/share`).

- Settings → "Scan for Orphaned Images" and "Run Data Quality Scan"
  buttons appeared to do nothing on click (issue #134). The forms
  used `data-dixie-submit` so the click hit `dispatchDixieDataForm`,
  which read the response headers but discarded the response body.
  The handlers were returning rendered HTML fragments
  (`SettingsOrphanedImages`, `SettingsQualityScanResults`) that
  never landed in `#settings-orphan-results` /
  `#settings-quality-results`. Added a `data-results-target`
  convention: when a form opts in via `data-results-target="#id"`,
  the dispatcher writes the response body into the matched element
  and re-runs `initializeDynamicContent` on the subtree (mirrors the
  browse-view refresh pattern). Wired the convention onto both
  scan forms in `entry_form.templ`. Regression net: new
  `TestSettingsOrphanScanEndpointRendersResults` asserts the orphan
  handler still returns 200 + the empty-state marker (no
  `X-DixieData-Redirect` / `Location` header), and `audit/smoke.mjs`
  now submits both scan forms against the live `dixiedata-web` server
  and asserts the result divs are non-empty.

- Toast text still rendered as mojibake after issue #135 shipped
  the real U+2026 / U+2014 runes into the source. The source was
  correct (`curl -i` shows valid UTF-8 bytes on the wire), but
  Chromium / WebView2 decode HTTP/1.x response headers as
  Windows-1252, not UTF-8, per the WHATWG Fetch spec — every byte
  above `0x7F` gets reinterpreted as a separate codepoint,
  producing `Shared archive import startedâ¦` on the toast.
  Introduced `sanitiseToastForHeader` next to `setToastHeader` /
  `setToastHeaderWithType` in `exports_handlers.go`. Source code
  keeps the polished Unicode characters; the helper rewrites a
  short table of common punctuation to ASCII twins (`…` → `...`,
  `—` → `--`, `–` → `-`, curly quotes → straight, NBSP → space,
  `→` → `->`, `✓` → `OK`, `·` → `*`, `§` → ``) at the boundary
  where the toast text enters the response header. User-data
  characters (accented Latin, CJK) pass through unchanged so
  future toasts that quote user input are not silently mangled.
  Every existing `setToastHeader*` call site benefits without
  changes — the substitution is centralised at the contract
  boundary. Captured the decision in
  `docs/adr/0005-toast-header-ascii-safe.md`. Regression net:
  `TestSanitiseToastForHeaderReplacements` pins every table entry
  including ASCII / user-data passthrough and empty input;
  `TestSetToastHeaderAppliesSanitisation` asserts no byte above
  `0x7F` reaches the wire; `TestToastHeaderSourceStillContainsUnicode`
  pins the contract that source keeps the polished characters so
  future contributors update the table instead of stripping
  Unicode at the source. The existing
  `TestInProgressToastStringsContainActualEllipsis` source sweep
  is updated to allow legitimate single-quoted rune literals
  (`'\u2026'`) which were previously false-positives after the
  helper table landed.

- `pkg/render.SoldierLister` interface removed (issue #143). The
  interface was declared but never referenced outside its own
  declaration site — `grep -rn "SoldierLister" pkg/` matches
  only `pkg/render/render.go` itself. The accompanying doc
  comment claimed the interface existed "so the render package
  does not import internal/records transitively," but the file
  already imports `internal/records` for `AnalyticsSnapshot` /
  `AnalyticsCount` re-aliases, so the rationale was stale. No
  call sites to update (interface was dead); `pkg/exportbridge`
  uses `*archive.SoldierService` directly via its own
  `BulkRenderer` type. `pkg/render` still imports `records` for
  the analytics re-aliases; that import is documented and
  load-bearing.

- Architectural boundary test tightened (issue #141). Two new
  layers of enforcement in `internal/architecture/architecture_test.go`:
    1. `forbiddenByPackage` now covers the grey-box layer too:
       `internal/viewmodel` is forbidden from `appshell`,
       `a-h/templ`, `wails`, and `templates` (the delivery
       surface); `internal/presentation` is forbidden from
       `appshell` and `wails` (templ is allowed because
       presentation IS the templ-rendering adapter). Both
       packages are still allowed to import deeper modules
       (`records`, `archive`, `models`, `jobs`, `update`, `debug`)
       because that is their documented grey-box role.
    2. New `TestPkgImportsAreAllowlisted` + the
       `allowedInternalImportsPerPackage` table enforce that each
       `pkg/*` package only imports the `internal/...` types it
       genuinely needs. Allowlists mirror the current imports:
       `pkg/render` → `{models, records}`, `pkg/exportbridge` →
       `{archive, db, models}`, `pkg/encode` → `{buildinfo,
       models}`, `pkg/templatespec` → `{}`. Any new `internal/`
       import requires updating the allowlist in the same commit.
  Also: `TestArchitectureMapsToContract` now requires
  `internal/viewmodel` and `internal/presentation` to be in the
  forbidden table. No production code changed.

- `internal/services/` shim deleted (issue #142). The 89-line
  shim was 55 type/func re-exports of `records`, `archive`,
  `integrations`, and `db` symbols with zero behavioral
  purpose. The three consumer files (`cmd/gold-master/main.go`,
  `cmd/gold-master/portability.go`, `internal/seed/seed.go`) now
  import the deep modules directly. `services.NewSoldierService`
  → `records.NewSoldierService`,
  `services.NewExportService` → `archive.NewExportService`,
  `services.NewBackupService` → `archive.NewBackupService`,
  `services.NewAnalyticsService` → `records.NewAnalyticsService`,
  `services.PrintSettings` / `BackupManifest` /
  `SharedImportSummary` / `SoldierService` → `archive.*` /
  `records.*`. The boundary test from issue #141 now guarantees
  no future re-introduction of `internal/services/` — if a new
  file accidentally re-imports it, CI fails.

- Feedback modal no longer silently swallows confirmation. Saving
  feedback through the floating-dock modal used to close the
  window and queue a toast for the next page nav — but no nav
  fires on the close-feedback path, so the toast never displayed
  and the user saw a closed modal with no acknowledgment. Two
  coordinated changes:
    1. `internal/templates/layout.templ`: the feedback form now
       carries `data-dixie-submit` + native `action=`
       + `method="post"` instead of relying on the htmx-only
       `hx-post` / `hx-swap="none"` wiring. The htmx-attrs were
       never read by the `app.js` dispatcher; without
       `data-dixie-submit` the form was htmx-only, htmx fired
       the POST, and the `X-DixieData-Close-Feedback` /
       `X-DixieData-Toast` headers were dropped on the floor.
       `action=` + `method="post"` + `data-dixie-submit` routes
       through the existing dispatcher (matches the
       calendar PDF export form pattern, the only
       previously-working form of this shape).
    2. `frontend/app.js`: when the dispatcher reads
       `X-DixieData-Close-Feedback`, it (a) hides the modal
       (existing), (b) **clears the form** via `form.reset()`
       so the next open starts blank and the save is visible,
       and (c) **renders the toast immediately** via
       `showToast(...)` instead of queueing via
       `savePendingToast(...)`. The trailing `savePendingToast`
       is suppressed for the close-feedback path so the same
       toast isn't queued for a nav that will never happen.
  `audit/smoke.mjs` grows a `[7d]` block with six
  end-to-end assertions: `feedback-modal-openable`,
  `feedback-save-sends-close-header`,
  `feedback-save-sends-toast-header`,
  `feedback-save-hides-modal`, `feedback-save-clears-form`,
  `feedback-save-shows-toast`.

- `docs/COMMON_BUGS.md` grown with five new sections from the
  60-day UI fix survey: �1.10 `redirect-contract-drift` (7
  instances), �1.11 `htmx-attr-strip-by-boot-js` /
  `data-dixie-submit` opt-in (3 instances), �3.5
  `stale-status-panel-after-submit` (4 instances), �4.11
  `duplicate-job-handling` (3 instances), �4.12
  `toast-encoding-mojibake` (2 instances), �4.13
  `route-misregistered-or-wrong-verb` (2-3 instances), �4.14
  `floating-dock-layout-overlap` (4 instances). �1.9 status
  updated from �Eliminated� to �REGRESSION-PRONE� with a
  pointer to the new �1.10. The �Bug class → first place to
  look� table at �11 grows rows for each new pattern. New file
  `docs/agents/bug-pattern-grep.md` is the copy-paste grep
  cookbook for all 8 patterns: one section per pattern with
  the grep, the false-positive filter, and a link back to the
  canonical recipe in `COMMON_BUGS.md`. No code change.

- Manual UI audit playbook (this slice). Three new artifacts:
    1. `docs/agents/manual-audit-playbook.md` — guided protocol
       for walking every UI surface by hand, capturing findings,
       and filing them as GitHub issues. Includes a “what to look
       for” checklist (visual / behaviour / a11y / performance /
       data integrity), per-finding templates for [BUG] /
       [FEATURE] / [CORRECTION], and a growth path.
    2. `audit/run-interactive.mjs` — Playwright walker that
       automates the deterministic parts (page loads, form
       submits, network round trips, screenshot capture) and
       flags `? (manual)` for the human-only checks. Writes
       `audit/audit-interactive-report.json` (machine-readable
       summary) + `audit/screenshots-interactive/<surface>-{before,after}.png`.
       Surfaces covered: calendar, soldier-new, browse, share,
       settings, feedback-modal, floating-dock-layout, jobs-page.
       18 auto checks pass on the current `dev`.
    3. `docs/agents/audit-notes-TEMPLATE.md` — drop-in template
       for capturing findings during a manual audit round. One
       block per finding. Links to COMMON_BUGS.md pattern
       reference and to the playbook for the full protocol.
  Phase 1 deliverable complete. Next: Phase 2 (smoke.mjs
  expansion + Wails-free test path via OpenDirectoryDialog +
  BrowserOpenURL override hooks).

- `internal/appshell/runtime.go` grows two override hooks
  matching the existing `SetOpenFileDialogOverride` /
  `SetSaveFileDialogOverride` / `SetOpenMultipleFilesDialogOverride`
  pattern. The web-mode binary now installs them via the
  `DIXIE_OPEN_DIRECTORY_DIALOG_PATH` and
  `DIXIE_BROWSER_OPEN_URL_LOG` env vars, closing two of the
  four Wails-only gaps that the smoke harness could not reach:
    1. `SetOpenDirectoryDialogOverride` lets the
       "Download images to folder" and "Choose where to copy
       record images" flows run end-to-end in the audit
       harness. Without it, the web-mode binary returns
       `errWailsFrontendUnavailable` and the user sees an
       uninformative toast.
    2. `SetBrowserOpenURLOverride` records the requested
       `file://` URL into a log file so the audit harness can
       assert the "Open result" + "Open log folder" flows
       land the right path. Without it, the user sees an
       info-toast "Open in OS file manager" fallback and the
       harness has no way to assert correctness.
  Both overrides follow the same precedence as the existing
  three: hook first, frontend guard second, real Wails call
  last. Four new unit tests in `runtime_test.go` cover the
  override-takes-precedence + without-override-still-sentinel
  pattern for each. The `runtime.go` and `runtime_test.go`
  changes are the only Go changes in this slice; the next
  slice wires the new hooks into the smoke harness.

- `audit/smoke.mjs` grows a `[9]` block that exercises the two
  new Wails-free hooks end-to-end against the live server.
  `[9a] jobs-open-button-uses-browser-open-override`: re-seeds
  a job via `/export/json` from `/share`, navigates to
  `/jobs/{id}`, polls the job until terminal, POSTs directly
  to `/jobs/{id}/open` (bypassing the polling overlay via
  `page.request.post`), then reads the
  `DIXIE_BROWSER_OPEN_URL_LOG` file and asserts a `file://`
  URL was recorded. `[9b] open-directory-dialog-override-is-wired`:
  indirect assertion that confirms the runtime_test.go suite
  covers the override precedence. The summary line at the
  end of the smoke now reports a “skipped” count alongside
  pass/fail so a missing env var is visible in the output
  (not a failure). 58 pass / 1 fail / 0 skipped after the
  change; the `memorial-import-flow` carve-out remains the
  only failure, unrelated.

- `audit/run-interactive.mjs` grows a Phase 2 surface
  `jobs-open-artifact` that exercises the new
  BrowserOpenURL override hook end-to-end. Re-seeds a job via
  `/export/json` from `/share`, navigates to `/jobs/{id}`,
  polls until terminal, POSTs directly to `/jobs/{id}/open`
  via `page.request.post()` to bypass the polling overlay,
  then reads `DIXIE_BROWSER_OPEN_URL_LOG` and asserts a
  `file://` URL was recorded. The summary line now reports
  a 'skipped' count alongside pass/fail/manual. 19/0/4/0 after
  the change. Phase 2 surface coverage closes the BrowserOpenURL
  gap from the Wails-free test feasibility audit.

### Maintenance

- Stopped `dixiedata-web.exe` from leaking across probe runs.
  Three audit probes (`audit/probe-backup-status.mjs`,
  `probe-full-restore.mjs`, `probe-share-status-scroll.mjs`)
  used `go run ./cmd/dixiedata-web` + a `finally` cleanup that
  could not reach the grandchild process tree on Windows, so a
  Ctrl-C or thrown error left the server running and the next
  `make debug` failed with `unlinkat ... dixiedata-web.exe: The
  process cannot access the file`. Switched the probes to spawn
  the prebuilt `build/bin/dixiedata-web.exe` directly and use a
  new `audit/_lib/cleanup.mjs` helper that installs SIGINT /
  SIGTERM / uncaughtException handlers and taskkills the named
  exe as a safety net. Added `make probe-clean` to nuke any
  straggler `dixiedata-web.exe` / `DixieData.exe` /
  `seed-data.exe` / `gold-master.exe` processes; `make debug`
  and `make build` now run it automatically before rebuilding
  the sibling binaries.

### Changed

- The recurring "export options status pages not landing" bug
  is fixed at the architecture level. Every post-then-navigate
  flow (export buttons, import buttons, merge-review actions,
  delete confirmations, settings toggles, soldier create/update)
  now navigates reliably because the contract is single-sourced.
  The browser always lands on the destination page or back on
  the originating page with a clear toast on dedup — never
  silently in the background. (Verified end-to-end via the
  dev-server smoke harness; Wails desktop smoke is manual — see
  `docs/adr/0004-option-c-dispatcher.md` for the rationale.)

### Fixed

- Web-mode (`cmd/dixiedata-web.exe`) save-dialog exports
  (`/export/json`, `/export/csv`, `/export/ical`,
  `/export/backup`, `/export/shared-archive`,
  `/export/database-pdf`, `/export/bug-report`) silently
  bounced users back to `/share` because the binary never
  installed `SetSaveFileDialogOverride`. Wired the override
  (commit `30ab8e7`) so the web-mode binary auto-routes every
  export to `<DIXIE_SAVE_FILE_DIR>` (defaulting to
  `<dataDir>/exports/`). The Wails desktop binary is
  unaffected — it has a real native `SaveFileDialog`.
- Split `guardedSaveFileDialog`'s outcome into three states:
  `SaveOutcomeOK`, `SaveOutcomeDuplicated`,
  `SaveOutcomeDialogAborted` (commit `14a2aa8`). The old
  bool-shape collapsed "duplicate in flight" and "user
  cancelled" into one branch, which was the proximate cause
  of the misleading "Export already in progress" toast
  surfaced on every cancel. Handlers updated for all 9
  save-dialog-backed exports plus `handleExportFeedbackLog`.
  The `(*App).inFlight` dedup map stays — the Wails v2.12.0
  UI-thread crash from two simultaneous native dialogs is
  still real even though the dual-JS-handler race is gone.
- Audit smoke harness tightened to require `/jobs/{id}`
  specifically for non-carve-out exports (commit `c9e5da3`),
  with two documented carve-outs: `/export/static-archive`
  (plain `<form method="post">` carve-out, follows 303
  natively) and `/export/feedback-log` (no-data early
  return). The previous `/share`-as-success acceptance
  masked the missing save-dialog override.

### Maintenance

- Replaced `frontend/app.js`'s custom htmx-clone dispatcher
  (`request()`, plus all helper functions) with a 32-line
  `dispatchDixieDataForm`. Net -411 lines from `app.js`.
- Migrated 13 Go handlers from `303 + Location + HX-Redirect`
  to `200 + X-DixieData-Redirect` via the new `writeExportRedirect`
  helper. `handleExportStaticArchive` opts into
  `enqueueExportOpt{NativeRedirect: true}` to keep the 303 path
  for its plain-`<form method="post">` carve-out.
- Retagged 9 templ files (`calendar`, `calendar_day`, `entry_form`,
  `insights`, `research_collections`, `research_log`,
  `review_queue`, `share`, `soldier_card`) from
  `hx-post`/`hx-put`/`hx-delete`/`hx-confirm` to
  `action`/`data-action` + `data-dixie-submit` + `data-confirm`.
  ~75 attribute changes. htmx stays loaded for GET-only polling
  on `/jobs/active` and `/jobs/{id}`.
- Registered `htmx.on("htmx:load", ...)` to re-init swapped
  subtrees. Polling fragments swap fresh DOM every 2–3s; without
  re-init, JS handlers on those subtrees never re-bind.
- Restored the 200ms debounce on the browse-filter change
  handler. The legacy `queueRequest` had it; it was dropped in
  the initial dispatcher rewrite because the harness test waited
  50ms. Restoring it prevents fetch storms on rapid filter
  changes (e.g. typing in a select).
- Trimmed the dead `hx-post` / `hx-delete` / `data-hx-*` selectors
  from the dispatcher interceptors. After the templ retag, no
  elements match those selectors; the translator window is gone.
- Rewrote `internal/templates/components/conventions.md` §"Buttons
  that POST and expect navigation" to describe the Option C
  contract instead of the dead `HX-Redirect` recipe. Without
  this rewrite, the next author would write the same broken
  contract the bug class was built on.
- Replaced `docs/COMMON_BUGS.md` §1.9 (the original
  "export-options-status-pages-not-landing" bug postmortem)
  with a short pointer to the new contract and the regression
  nets that prevent reintroduction. The postmortem's "fix"
  (adding `HX-Redirect`) is documented as dead code so the
  next reader understands why the section was removed.
- Wrote `docs/adr/0004-option-c-dispatcher.md` capturing the
  architectural decision (why the bug class recurred, what the
  new contract is, which regression nets guard it).

### Added

- Three source-scan regression nets that fail the build if the
  Option C bug class is reintroduced:
  `TestPostThenNavigateUsesDixieRedirect` (appshell) — fail on
  303 writers without `X-DixieData-Redirect`.
  `TestNoPostThenNavigateHXXAttrs` (templates) — fail on any
  `hx-post` / `hx-put` / `hx-delete` / `hx-confirm` in templ.
  `TestNoDeadHXRedirectWrites` (appshell) — fail on any handler
  writing `HX-Redirect`. Together they form a tripwire: any author
  who tries to write the old contract hits a build failure with a
  file:line citation.
- `audit/discover_export_buttons.mjs` learned the `data-action`
  literal pattern so the auto-discovery for smoke tests still
  finds every share-page button after the templ retag.
- `audit/smoke.mjs` `share-${btn.path}-navigates-to-jobs` asserts
  the user-visible contract (page lands on `/jobs/{id}` or back at
  `/share` on dedup) instead of asserting a specific response
  shape, so the contract switch can't silently regress navigation.

- `/jobs/{id}` summary cards now show per-kind stats so the
  user can see what an export or import actually contained
  without re-opening the artifact. Six Wails share-page
  exports and three import flows were upgraded:

  **Exports** (kinds that surface `Person records:`,
  `Images:`, and/or `Source records:`):
  - JSON export → `Person records: N` (records count)
  - Excel export → `Person records: N`
  - iCalendar export → `Person records: N` (soldiers enumerated)
  - Printable archive PDF → `Person records: N` + `Images: N`
  - Backup (.ddbak) → `Person records: N` + `Images: N` +
    `Source records: N`
  - Shared archive (.ddshare) → same as backup

  **Imports** (kinds that surface the merge-review headline or
  the replace + schema migration line):
  - Shared archive import → `N added, N merged, N skipped`,
    plus `Conflicts staged for review: N` when >= 1 (so the
    user is reminded to open Merge Review), plus
    `Images imported: N`.
  - Memorial JSON import → `N added, N skipped, N failed`,
    plus `Images imported: N` when applicable.
  - Backup restore → `Replaced: N records, N images`, plus a
    schema line that reads `Schema migrated: backup vX → current vY`
    when the migration ran or `Schema: backup vX = current vY (no migration)`
    when schema parity held.

  Lines render conditionally on the populated count (zero
  counts stay absent), so legacy kinds that don't fill the
  struct are unaffected.

- Plumbed end-to-end:
  - `internal/jobs/jobs.go`: new `JobResult` struct + `Job.Result`
    field + `Registry.SetResult` setter. Promotes `Path` to
    `ResultPath` so `/jobs/{id}/artifact` still streams when
    callers forget to call `SetResultPath` explicitly.
  - `internal/jobs/jobs.go`: `Summary()` now surfaces the new
    counts via four helpers — `appendExportStats`,
    `appendSharedImportStats`, `appendMemorialImportStats`,
    `appendBackupRestoreStats`. Each kind's existing copy is
    preserved; stats lines append only when populated.
  - `internal/archive/export_service.go`: new with-stats
    variants — `ExportJSONWithStats`,
    `ExportExcelWithStats`,
    `ExportICalendarWithStats`,
    `ExportFullDatabasePDFWithStats`,
    `ExportStaticArchiveWithStats`. Existing `ExportXxx`
    methods are unchanged; the CLI in
    `internal/appshell/cli_export.go` still calls the
    count-less variants because shell output does not surface
    per-record stats. When the CLI gains structured output it
    should switch.
  - `internal/appshell/app_facades.go`: facade lists the new
    with-stats methods so `a.export.ExportXxxWithStats` type-checks.
  - `internal/appshell/exports_handlers.go`: new
    `enqueueExportWithResult` helper alongside the existing
    `enqueueExport`. The six handlers that produce structured
    artifacts (`json_export`, `excel_export`, `icalendar_export`,
    `database_pdf`, `backup_archive`, `shared_archive`) now use
    it. The remaining kinds (`soldier_pdf`, `soldier_jpg`,
    `monthly_pdf`, `insights_pdf`, `image_import`, `bug_report`,
    `static_archive`) continue to use the original helper
    unchanged.
  - `internal/appshell/imports_handlers.go`: the three import
    workers (`backup_import`, `shared_import`, `memorial_import`)
    now call `SetResult` with the appropriate counts before
    returning nil. Memorial import also records `LogPath` so a
    future UI iteration can wire the error log download.

### Maintenance

- The global layout progress popup is now named consistently
  with the rest of the UI surface vocabulary:
  - `uiids.OverlayJobsProgress` is the canonical surface ID
    (kind: overlay). Added to `internal/uiids/uiids.go`
    alongside the other overlays (FloatingMenu, FeedbackModal,
    ImageViewer, etc.).
  - CSS class `progress-region` renamed to
    `jobs-progress-overlay` in `frontend/tailwind.css`.
  - Data attribute `data-progress-region` renamed to
    `data-jobs-progress-region` (follows the three-attribute
    namespace rule: `data-<feature>-...` for runtime hooks).
  - `hx-target` selector in
    `internal/templates/job_slot_fragment.templ` updated
    accordingly.
  - All 25 grep matches across 9 files updated: 5 test files
    (job_slot_swap_test.go, page_snapshot_test.go,
    jobs_handlers_test.go, audit/smoke.mjs,
    audit/probe-setup-stacking.mjs), 3 doc files (CHANGELOG,
    COMMON_BUGS, RESEARCH), and the live audit smoke
    assertion (renamed `progress-region-survives-polls` to
    `jobs-progress-overlay-survives-polls`).

### Fixed

- `internal/appshell`: duplicate export requests (issue #130) no
  longer strand the user on an error page. Each in-flight dedup
  key now stores the background `JobID` once the worker has been
  started, so a duplicate click that races against the save
  dialog roundtrip is redirected 303 to `/jobs/{id}` instead of
  replacing the modal/document with the "Export already in
  progress" body. When no `JobID` is known yet (the dialog is
  still open), the duplicate still receives an `HX-Redirect` +
  toast so the originating page stays put. Covers the five
  SaveFileDialog sites in `app.go` (soldier PDF / soldier PDF
  no-images / soldier JPG / calendar PDF / image screenshot),
  the printable-PDF flow in `exports_handlers.go`, and every
  `guardedSaveFileDialog` caller (`json`, `insights_pdf`,
  `excel`, `icalendar`, `static_archive`, `backup_archive`,
  `shared_archive`, `bug_report`, `feedback_log`).
- `scripts/build-common.ps1` + `scripts/build-debug.ps1`:
  `make debug` now actually builds a debug binary. Previously
  the recipe passed `wails build -clean -trimpath` (a
  production build with stripped source paths) and only
  generated a thin launcher wrapper. The wrapper was a no-op
  that just re-exec'd the production binary. With this fix:

    - `Invoke-DixieDataBuild -DebugBuild` swaps the default
      Wails args to drop `-trimpath` and add `-debug`, which
      makes Wails:
      * Preserve source paths in DWARF (so dlv can set
        breakpoints by file:line; the existing
        `scripts/debug-crash.dlv` workflow now works as
        written).
      * Add `-gcflags=all=-N -l` automatically (Go's
        optimiser no longer elides frames or inlines past
        breakpoints).
      * Enable the WebView2 DevTools + default context menu
        in the running Wails app. `F12` / `Ctrl+Shift+I`
        now opens the inspector without rebuilding.

    - The `Run-DixieData-Debug.ps1` launcher regenerated with
      debug-friendly env defaults:
      * `GOTRACEBACK=all` — full stack on panic.
      * `DIXIEDATA_DEVTOOLS=1` — forces the Wails
        `EnableDefaultContextMenu` env-gate (new in
        `main.go`) to enable DevTools in any build, including
        a release binary launched via the debug launcher.
      * `DIXIEDATA_WAIT_FOR_DEBUGGER` — opt-in pause at
        process start so `dlv attach $PID` from another shell
        can attach before Startup runs.

  Regression net in `internal/appshell/build_flags_test.go`
  pins down: DWARF source paths present, 10k+ symbols, the
  launcher writes the new env vars. Skips cleanly when
  `build/bin/DixieData.exe` is absent so release-only CI
  doesn't fail.
- `internal/templates/jobs.templ`: the `/jobs/{id}` landing
  page (`JobStatusView`) was a static snapshot — it rendered
  the body of the page but did NOT include the `hx-get` /
  `hx-trigger="every 2s"` that drives the 2s poll. The page
  froze at the value captured in the 303 redirect even while
  the job ran to completion in the background. Fast exports
  (`static_archive` in particular) finished during the
  redirect window, so the user always landed on a page that
  read "running" / "queued" forever even though the artifact
  sat ready in `/jobs/{id}/artifact`.

  Fix: extract the body of the status page into a single
  `jobStatusBody` sub-template that both `JobStatusView` (the
  full page) and `JobStatusFragment` (the polling fragment
  served from `/jobs/{id}/status`) call. Now both render the
  same `id="job-status-body"` wrapper with the same `hx-get`
  / `hx-trigger` so the landing page polls automatically. The
  extraction also prevents the view and the fragment from
  drifting apart in future edits.

  Regression net:
  - `internal/templates/jobs_artifact_link_test.go`:
    * `TestJobStatusViewPollsForUpdates/running_job_wires_the_poll`
      asserts the page renders `hx-get="/jobs/{id}/status"`.
    * `TestJobStatusViewPollsForUpdates/done_job_stops_polling`
      asserts the page renders `hx-trigger="none"` when the
      job is done (so polling stops once the summary card
      is visible).
    * `TestJobStatusViewPollsForUpdates/view_and_fragment_share_the_poll_url`
      asserts the view and the fragment agree on the poll
      URL — the extraction cannot drift.
  - `internal/templates/page_snapshot_test.go`:
    `TestPageSnapshotJobsStatus` now also asserts the running
    page renders `hx-get="/jobs/job-abc/status"`.
  - `internal/appshell/jobs_handlers_test.go`:
    `TestHandleJobStatusFullPageWiresThePoll` is the
    end-to-end net: GET `/jobs/{id}` returns a body that
    wires the poll (holds the worker on a channel so the
    job stays running through the render).
- `internal/appshell/exports_handlers.go` +
  `internal/appshell/imports_handlers.go` +
  `internal/appshell/app.go`:
  Fixed the share-page export-lands-on-blank-page bug that
  hid the new per-kind stats summary card. htmx 2.x with
  `hx-swap="none"` silently swallows 303 responses unless the
  server also writes `HX-Redirect`; the export + import + dedup
  helpers only wrote `Location`, so the user clicked the
  button, the export ran to completion in the background, and
  the page silently stayed on `/share`. Now `enqueueExport`,
  `enqueueExportWithResult`, `respondDuplicateInFlight`, and
  the backup restore's in-flight redirect write both
  `Location` (for plain `<form method="post">` submits like
  static archive) and `HX-Redirect` (for htmx). Static archive
  was unaffected because it already uses a plain HTML form,
  not htmx.
  Regression net:
  - `TestEnqueueExportRecordsJobIDOnEntry` now also asserts
    `HX-Redirect`.
  - `TestImportBackupInFlightGuardRedirectsToExistingJob`
    same.
  - `TestEnqueueExportWithResultSetsHXRedirect` (new) pins
    both headers on the with-stats helper.
- `audit/smoke.mjs`: every share-page export button now also
  asserts `share-{path}-navigates-to-jobs` — after the click,
  `page.url()` must include `/jobs/`. The previous
  `share-{path}-redirects-303` assertion only checked the
  response headers; it did NOT prove the browser actually
  followed the redirect, which is how the htmx `hx-swap="none"`
  + 303 silent-swallow bug slipped through. Now the live
  harness catches both: response shape AND navigation.
- "Upload Backup to Google Drive" and "Export CSV to Google
  Sheets" share-page buttons now land the user on `/jobs/{id}`
  after the worker starts. Previously the two Google handlers
  wrote a `Location` header but no `HX-Redirect`, so with the
  buttons' `hx-swap="none"` htmx 2.x swallowed the redirect and
  the user stayed on `/share`. Pinned by
  `appshell.TestGoogleHandlersRedirectToJobs` (two assertions:
  `/integrations/google/backup` and
  `/integrations/google/sheets/export`) and the new
  `share-/integrations/google/backup-navigates-to-jobs` /
  `share-/integrations/google/sheets/export-navigates-to-jobs`
  smoke assertions.
- The Printable PDF export modal (Share → "Printable PDF…")
  now lands on `/jobs/{id}` instead of dumping markup into the
  `#share-status` panel. Dropped the Wails-bridge JS interceptor
  in `app.js::submitPrintConfig` and the brittle
  `hx-on::after-request` 303 shim on the modal form, and made
  the form a plain htmx form that relies on
  `handleExportDatabasePDF`'s existing `HX-Redirect` header
  (same pattern as every other share-page export). Pinned by
  the new `[5b]` smoke block.
- `internal/appshell` 303-redirect handlers now ship HX-Redirect
  alongside Location so `hx-swap="none"` buttons land the user
  on the destination page instead of silently swallowing the
  redirect. Five additional handlers were missed by the original
  3612dab sweep and were repaired in the same commit that added
  the global guard:
  - `handleImportSoldierImages` (`app.go`)
  - `handleRunDuplicateAudit` (`insights_handlers.go`)
  - `handleReviewQueueBulk` (`reviews_handlers.go`)
  - `handleCleanupImageOrphans` (`settings_handlers.go`)
  - `handleCreateSoldier` / `handleSoldierByID` (DELETE branch) /
    `handleUpdateSoldier` (`soldiers_handlers.go`)
  The new `appshell.TestAll303sWriteHXRedirect` walks every
  function in the package, finds every `StatusSeeOther` write,
  and asserts a sibling `HX-Redirect` is set on the same
  handler (with an explicit allow-list for server-initiated
  middleware redirects). Verified to fail when the header is
  removed and pass when restored; the allow-list requires a
  one-line reason per exempt function so the next reader knows
  why no htmx button reaches it.
- `audit/smoke.mjs` now auto-discovers share-page export buttons
  by scanning `internal/templates/*.templ` instead of
  hand-maintaining the `shareButtons` array. New export routes
  added to `share.templ` are covered by `share-{path}-navigates-
  to-jobs` assertions without manual harness edits. The new
  `audit/discover_export_buttons.mjs` walks every form and bare
  button, resolves label inference for both `components.Button`
  and `components.ButtonContent` patterns, and gates inclusion
  on an explicit override table (`builderPrefixOverrides` for
  routebuilder-driven buttons, `literalPathOverrides` for
  literal-string hx-post paths, `actionPathOverrides` for plain
  `<form method="post">` actions). The companion
  `discover_export_buttons.test.mjs` pins the manifest shape
  (10 canonical share-page buttons, Google Calendar / connect
  / disconnect excluded, printable PDF modal excluded because
  its dedicated `[5b]` smoke block covers it). The hand-written
  `shareButtons` array now derives from the discovery result.

### Maintenance

- **Doc consolidation for click-driven surfaces.** Five
  edits land in one commit so the htmx `hx-swap="none"` + 303
  trap and the surrounding patterns have a single source of
  truth:
  - `internal/templates/components/conventions.md`: new
    section "Buttons that POST and expect navigation" —
    recipe for the canonical `Location` + `HX-Redirect`
    pair, checklist for new POST-then-navigate handlers.
  - `docs/COMMON_BUGS.md`: new §1.9 — bug catalog entry with
    grep commands, root cause, fix recipe, and the regression
    net (audit/smoke.mjs `-navigates-to-jobs` assertion).
  - `AGENTS.md`: new "Commits and branches" section —
    one-commit-one-logical-change rule, message shape,
    branch naming, pre-push checks, CHANGELOG rule, and the
    cross-link to the new conventions rule for any new
    click-driven button.
  - `docs/ai-handoff.md`: new "Adding a feature: canonical
    workflow" section — 8-step skeleton (surface → routebuilder
    → service → handler → templ → regression net → verify →
    CHANGELOG) with cross-links to per-layer checklist docs
    and explicit warnings about the htmx + 303 trap.
  - `audit/smoke.mjs`: comment block above the share-page
    export assertions tightened to clarify that the success
    path (enqueueExport) writes BOTH Location AND HX-Redirect,
    not just the dedup-fallback path.

  `CONTEXT.md` Laws stays slim — the trap is documented in
  `conventions.md` (recipe) + `COMMON_BUGS.md` (postmortem),
  cross-linked from AGENTS.md.

### Maintenance

- `Makefile`: `make debug` now builds every sibling binary
  the debug workflow expects to be present:
  `build/bin/DixieData.exe`, `build/bin/dixiedata-web.exe`,
  `build/bin/seed-data.exe`, `build/bin/gold-master.exe`,
  `tools/tune/bin/dixiedata-tune.exe`. New standalone targets:
  `make web`, `make seed`, `make gold`, `make tune-bin` (the
  existing `make tune` target runs the render harness, so the
  build step is split off under a new name to avoid a
  collision). `migrate-logs` is intentionally NOT included —
  no script in this repo calls it; add it when a workflow needs
  it.
- `internal/jobs/jobs.go`: new `SilentKinds` set + `IsSilentKind`
  helper, and `Registry.MostRecentActive` filters out kinds in
  the set. The global layout progress popup is now opt-out
  per kind: jobs whose `/jobs/{id}` status page is the
  intended landing (and whose artifact does not preview well
  in a new tab) get filtered out so the floating popup card
  never appears. Kinds register by adding to the map; the
  call site (the export handler) is unchanged.

- `static_archive` is the first silent kind: clicking "Export
  Static Web Archive" used to render a popup card whose
  "Open result" link opened a blank tab (the artifact is a
  .zip, which falls through to `Content-Disposition:
  attachment` and the browser consumes the response in its
  download manager without rendering anything). With this
  fix the popup stays empty and the user lands on
  `/jobs/{id}` via the standard 303.

- `internal/jobs/jobs_test.go` +
  `internal/appshell/jobs_handlers_test.go`: 3 new tests pin
  down the contract (silent kinds are filtered, non-silent
  kinds still surface, `/jobs/{id}` still renders for the
  silent job so the user isn't stranded).

- `internal/templates/job_slot_fragment.templ`: comment now
  documents the SilentKinds filter so future authors know
  why some jobs don't show up in the popup.

- `audit/smoke.mjs`: closed the three live regression gaps
  that commit b185f0e deferred. New assertions cover:

    - `share-{path}-redirects-303` on every share-page export
      button (proves the issue #130 redirect path fires
      end-to-end; accepts either the Wails `Location: /jobs/{id}`
      header OR the `HX-Redirect: /share` fallback that web-mode
      uses because it has no native dialog).
    - `debug-console-panel-appends-beforeend` (proves the
      b185f0e beforeend swap fix is in place; without it the
      debug-mode toggle would wipe the document).
    - `jobs-progress-overlay-survives-polls` (proves the
      `JobStatusSlotFragment` `outerHTML`->`innerHTML` fix is
      in place; without it the progress bar would freeze after
      the first poll).

  Live regression net jumped from 26 to 32 assertions.
- `internal/appshell`: native OpenFileDialog, OpenDirectoryDialog,
  and OpenMultipleFilesDialog callsites now route through
  dedicated guarded helpers (`guardedOpenFileDialog`,
  `guardedOpenDirectoryDialog`, `guardedOpenMultipleFilesDialog`
  in `internal/appshell/exports_handlers.go`) so the
  WebView2 `Chrome_WidgetWin_0. Error = 1412` re-entry race
  is closed for the import flows the original save-dialog
  law deferred. Closes the "open question" item in
  `docs/agents/dialog-guard.md`. Covers
  `handleImportSharedArchive`, `handlePreviewMemorialJSONImport`
  (file pickers), `handleImportSoldierImages` (multi-file
  picker), and `handleDownloadSoldierImages` (directory
  picker). The 3-value return shape (`path, admitted, ok`)
  lets each handler distinguish dup-hit (redirect to
  `/jobs/{id}`) from cancel (validation error) without
  re-reading the in-flight map. Regression net:
  `internal/appshell/open_dialog_guard_test.go`.
- `internal/appshell`: new `/jobs/{id}/report` route renders
  the job's terminal-state payload on a printable layout
  (status, summary, timeline, artifact metadata, error log
  when present). Wired through the redesigned job status
  page's "Show report" button (issue #131 follow-up). New
  `renderJobReport` handler in `jobs_handlers.go` and
  `templates.JobReportView` in `jobs.templ`. Regression
  net: `internal/appshell/jobs_report_handler_test.go`.
- `internal/templates/jobs.templ`: redesigned the terminal-state
  status card around a structured summary (issue #131). The new
  `jobSummaryCard` renders a kind-specific headline + size +
  duration detail lines, a primary Dismiss button that routes
  back to the page that kicked off the export
  (`jobs.Job.DismissTargetPath()`), a Show report button that
  links to `/jobs/{id}/report`, and demotes the artifact action
  (Open / Save) to a secondary link. `jobs.Job.Summary()`
  owns the structured payload so the template stays declarative;
  `formatBytes` rounds file sizes to a user-friendly unit.
- `internal/appshell`: .ddbak restore now runs as a background
  job (issue #133). The handler reads the local identity,
  enqueues the restore, and 303-redirects the user to
  /jobs/{id} so they see real progress during the multi-second
  restore instead of being blocked on the HTTP goroutine.
  Replaces the synchronous `X-DixieData-Redirect: /` flow that
  left the user on a blank /share tab for 10+ seconds on a
  500 MB archive. A new `a.importInFlight` atomic flag + an
  `importInFlightJobID` global coordinate the worker; a second
  click during a running restore redirects to the existing
  /jobs/{id} instead of opening a second dialog or crashing.
  The toast text now reads "Restoring backup: <name>" (info
  kind, issue #132) and the user lands on a real status page.
- `internal/appshell`: in-progress toasts (image import,
  shared-archive import, memorial-JSON import, Google Drive /
  Sheets exports, duplicate audit, bulk reviews, orphan
  cleanup) now emit `X-DixieData-Toast-Type: info` instead of
  the default `success` (issue #132). Combined with the
  existing `success || info` auto-dismiss branch in
  `frontend/app.js`'s `showToast`, every "X started…" toast
  fades out after 4 s on both the originating page and the
  page the user lands on after the 303 redirect. New
  `setInfoToastHeader` helper centralises the kind so future
  in-progress sites cannot regress to success-by-default.
  Error and warning toasts keep the manual-dismiss contract
  from issue #54. The 4 s and 320 ms timing values are now
  named constants (`toastAutoDismissMs`, `toastFadeOutMs`)
  at the top of `app.js` so future tuning is one edit.
- `internal/templates/jobs.templ`: non-viewable job artifacts
  (.ddbak, .ddshare, .zip, .csv, .ics) now render with a `download`
  attribute instead of `target="_blank"` (issue #129). The old
  combination opened a blank tab and triggered a silent download
  that the user couldn't see or find. PDFs, JPGs, PNGs, and other
  viewable extensions still open in a new tab as before. New
  `jobs.Job.IsViewableArtifact()` + `jobs.Job.ArtifactFilename()`
  helpers own the classification so the template stays declarative.
- `internal/templates/share.templ`: print-config modal renders
  with the centering classes required to display the dialog in
  the middle of the page (`justify-center`, `items-center` on
  `>=sm` viewports). Issue #128 reported the modal "loading on
  the left of the page" — root cause was a duplicate export
  click replacing the modal contents with the in-flight error
  body, fixed by the issue #130 redirect. The new
  `TestSharePrintConfigModalIsCentered` test pins down the
  CSS classes so a future refactor cannot silently remove
  them.

### Added

- `internal/routebuilder` package providing typed URL builders for
  every route templates reference (`ActiveJobs`, `JobStatus`,
  `JobStatusSlot`, `Anniversary`, `AnniversaryEdit`,
  `AnniversaryItemDelete`, `AnniversaryItemUpdate`,
  `AnniversaryItemCreate`, `FeedbackSubmit`, `DebugConsole`,
  `BrowseResults`, `SoldierSearch`). Templates call these via
  `templ.SafeURL(routebuilder.X(...))` instead of string literals.
  When a route moves, only `routes.go` and the matching builder need
  to change. 16 unit tests cover URL escaping, whitespace trimming,
  path-segment validation, and per-builder output stability.
- `github.com/go-chi/chi/v5` v5.3.0 added as a direct dep.

### Changed

- `internal/appshell/routes.go`: swapped `net/http.ServeMux` for
  `github.com/go-chi/chi/v5`. Chi provides explicit pattern routing,
  middleware composition (`middleware.Recoverer`,
  `middleware.RequestID`), and wildcard segments (`/*`) without
  changing handler signatures — every handler still reads
  `r.URL.Path` directly, so existing `strings.TrimPrefix` logic
  works unchanged. Wildcard routes register GET, POST, PUT, and
  DELETE methods where the handler dispatches by `r.Method` (soldier
  records, soldier display IDs).

### Added (continued)

- Persistent progress slot in the layout: a top-center progress bar
  (below the toast region) that polls `/jobs/active` every 3s and
  shows real progress for whatever background task the user kicked
  off most recently. The slot stays visible across page navigation
  so a user who starts an export from `/share` and navigates to
  `/soldiers` still sees the progress bar at the top of the page.
  Implemented as `JobStatusSlotFragment` in
  `internal/templates/job_slot_fragment.templ`.
- Toast kinds now have distinct CSS: success = warm cream + gold
  border (existing), error = warm red (existing), warning = amber
  (new), info = blue (new). `showToast()` in `frontend/app.js`
  switched to a header label matrix (Success/Heads up/Warning/
  Attention) and auto-dismisses `success` and `info` toasts after
  4 seconds. `error` and `warning` toasts remain manual-dismiss
  per the Issue #54 decision.
- Jobs registry hardening: `Registry.Shutdown(ctx)` cancels every
  running/queued job and waits on a new `workerWG` for worker
  goroutines to drain. Wired into `lifecycle.go` shutdown sequence
  before `database.Close()`, bounded by a 5s deadline. Prevents
  file-handle leaks on app exit (same family as the WJ-2 fix in
  `271149a`).
- New `openMultipleFilesDialogOverride` test hook on `*App`,
  mirroring the existing `openFileDialogOverride`. Required by the
  image-import migration so httptest can inject file paths.
- Migrated the following long-running handlers to the jobs registry
  (each now reports real progress via the persistent slot):
  JSON export, InsightsPDF export, Excel export, iCalendar export,
  Static web archive export, Printable database PDF export, Backup
  archive export, Shared archive export, Bug report bundle
  export, soldier PDF export (with and without images), soldier
  JPG export, monthly anniversary PDF export, image import on
  soldier detail and edit pages, shared archive import, memorial
  JSON import, duplicate audit, image orphan cleanup, review queue
  bulk-resolve and bulk-delete, Google Drive backup upload, Google
  Sheets export.
- Repaired the `JobStatusFragment` htmx polling: added the missing
  `hx-trigger="every 2s"` attribute so the `/jobs/{id}` page
  actually polls (previously the comment claimed 2s but no trigger
  was set, so htmx used the default `natural` trigger and never
  fired).
- **`audit/smoke.mjs`** — live Playwright regression net for
  click-driven surfaces. Boots a real Chromium against
  `dixiedata-web`, walks every button on the search / browse /
  share / insights / settings pages, asserts that each one
  fires the expected network request and that the swap target
  updates. 25 assertions. This is the test that finally
  caught the four bugs that PR #1 + PR #2 + PR #F1 shipped
  silently. Every commit that changes templ + htmx + JS +
  handler code must keep this green.
- Removed the unused SSE endpoint `/jobs/{id}/stream` and its
  handler (`streamJobProgress`, `writeJobEvent`,
  `isTerminalJobStatus`). No JS consumer in `app.js` opened an
  `EventSource` on the endpoint.

### Changed

- `data-progress-label` indeterminate spinner retained only on
  intentional carve-outs: image-import buttons (open native
  file picker), update-apply and recovery buttons (call
  `a.Quit()` 750ms after responding, cannot use the 303
  redirect pattern), and the six Google Calendar interaction
  buttons (OAuth popup, calendar picker UI).

### Fixed

- **16 chi-mis-registered routes** (PR #1 of the stabilization
  sprint set `r.Get` for every action endpoint whose handler
  rejected anything except `http.MethodPost`). Every export,
  share, insights, merge-review, and Google-connect button
  silently returned 405 Method Not Allowed when clicked.
  Flipped to `r.Post` for: `/export/{json,csv,ical,
  static-archive,backup,shared-archive,bug-report,feedback-log}`,
  `/insights/report/pdf`, `/merge-review/*`,
  `/integrations/google/{connect,disconnect,backup,
  sheets/export}`, `/images/screenshot`, `/open-link`. Two
  regression nets added so the class cannot recur:
  `routes_method_guard_test.go` (AST walk, flags any
  `r.Get` paired with a POST-only handler — pure compile-time
  check) and `route_integration_test.go` (runtime check that
  fires GET against every known POST-only path and asserts
  405 + `Allow: POST`). Plus a wildcard-shadowing test
  (`route_wildcard_test.go`) that fires GET at the more
  specific sibling of every `/parent/*` wildcard.

- **Broken `JobStatusFragment` htmx polling** — added the missing
  `hx-trigger` so the fragment actually re-fetches every 2s.

- **App.js hx-* attribute strip silently broke every click
  handler.** DOMContentLoaded stripped `hx-get`, `hx-post`,
  `hx-trigger`, etc. from the DOM to prevent htmx's auto-handler
  from double-firing alongside app.js's own `request()` /
  `queueRequest()`. But the same handlers READ those attrs to
  construct the fetch. After the strip, every read returned
  empty / null, so every click handler bailed out and the button
  did nothing. Fix: cache each `hx-*` attr to a `data-hx-*`
  mirror BEFORE stripping, then add `hxAttr(el, name)` /
  `hxHas(el, name)` helpers that prefer the live attr and fall
  back to the data-* mirror. Also added `input` to the
  `triggerInputRequest` regex so the quick-search trigger
  (`input changed delay:300ms`) actually fires.

- **htmxattr.Mux.Attrs() used `templ.SafeURL` for URL values
  — which templ.RenderAttributes silently drops.** This was
  the deepest bug in the chain: every `htmxattr.Mux{Get: ...}`
  call rendered the form/button without an `hx-get` attribute
  at all. The 16 unit tests in `internal/htmxattr/` passed
  because they only inspect the `templ.Attributes` map;
  nothing rendered the map through `templ.RenderAttributes`
  in a test. Fix: use plain `string` for URL values (not
  `templ.SafeURL`). The `SafeURL` wrapper is meaningful inside
  templ expression context but breaks in spread-attribute
  context.

- **Browse filter changes now auto-apply** (previously saved
  draft state only). The change handler in app.js calls
  `queueRequest(form)` after saving draft state, so the
  `/browse/results` request fires immediately. Updated the
  `TestBrowseFilterChangeSavesDraftWithoutAutoApplyingIt`
  Node-harness test (renamed to
  `TestBrowseFilterChangeAutoAppliesAndPersistsDraft`) to
  match the new behavior. The harness needed `window.setTimeout`
  added to the `windowMock` object so `queueRequest`'s
  `setTimeout(..., 0)` callback can drain.

- **`hxAttr` / `hxHas` duck-type the Element contract instead
  of `instanceof Element`.** `instanceof Element` is
  browser-only and broke the Node test harness for browse
  filter changes (the harness mocks `HTMLElement` but not
  `Element`). Now they check for `getAttribute` /
  `hasAttribute` method existence, which both real browsers
  and the mock satisfy.

- **Soldier PDF / JPG, image screenshot, and full database PDF
  exports no longer crash the app.** The 4 native `SaveFileDialog`
  call sites in `internal/appshell/app.go` (`handleSoldierPDF`,
  `handleSoldierPDFNoImages`, `handleSoldierJPG`,
  `handleImageScreenshot`) and `exportFullDatabasePDFPath` in
  `internal/appshell/exports_handlers.go` were the missing
  link in the issue #2807 guard net added by commit `162c353`.
  That commit routed 9 export handlers through
  `guardedSaveFileDialog` (or its inline equivalent) but the
  5 above called `a.SaveFileDialog` directly. A double-click
  on any of them queued a second native dialog on the Wails
  UI thread, both blocked, WebView2 lost focus during
  `MoveFocus`, and `errorCallback` killed the process with
  `Chrome_WidgetWin_0. Error = 1412`. All 5 call sites now
  carry the same `a.inFlight.LoadOrStore(...)` guard pattern
  as `handleCalendarPDF`; the database PDF helper returns a
  new `errExportInFlight` sentinel that the HTTP handler maps
  to a 429 and the Wails binding surfaces as a friendly toast.
  See `internal/appshell/save_dialog_guard_test.go` for the
  regression net.

- **Three modal dialogs reverted from native `<dialog>` back to
  the pre-issue-117 `<div role="dialog" aria-modal="true">`
  overlay** (feedback modal in layout, print-config and
  google-prefs in share). The native `<dialog>` swap was
  blamed for the crash but was a red herring — the real
  trigger was the unguarded `SaveFileDialog` race above.
  However, native `<dialog>` still carries a subtle WebView2
  interaction (showModal grabs host focus, which then routes
  through Wails' `onFocus` → `Chromium.Focus()` → `MoveFocus`
  at unexpected times), so reverting keeps the focus-event
  surface small while we wait for an upstream Wails fix.
  Manual focus trap and ESC close handlers live in
  `frontend/app.js` (`showOverlayModal` /
  `overlayModalKeydown`).

### Removed

- Developer visualizer overlay (orphan from v1; no current consumers).
  Removed `data-ui-id` template attributes (52 sites), `@SurfaceBadge`
  and `@InlineSurfaceBadge` calls (54 sites), `SurfaceBadge`/`InlineSurfaceBadge`/
  `uiDebugEnabled`/`uiDebugValue` helpers, `internal/uiids.DebugEnabled`/
  `EnableFromArgs`/`DebugEnvVar`/`DebugArg`/`truthy`, the
  `DIXIEDATA_DEBUG_UI_IDS` env var, the `--debug-ui-ids` flag, the
  `[data-debug-ui-ids=true] [data-ui-id]{...}` CSS outline rule,
  `.ui-debug-badge` / `.ui-debug-inline` styles, and
  `debugSurfaceIDsEnabled()` in `frontend/app.js`. The 78 surface
  constants in `internal/uiids/uiids.go` registry stay — they remain
  the canonical surface identifiers used by future HTMX typing work.
  The runtime log console at `/debug/console` (separate feature) is
  untouched.
- `/jobs/{id}/stream` route + `streamJobProgress`/`writeJobEvent`/
  `isTerminalJobStatus` handlers (dead code, no consumers).
- `enqueueStaticArchive` and `enqueueDatabasePDF` (replaced by
  the unified `enqueueExport` helper).

- Button primitive adopted in `calendar.templ` (Export Month PDF)
  and `jobs.templ` (Cancel x2) — these three sites were missed by
  the original grep pass that scoped to `class="primary-button"`
  with anchor instead of `<button` opening tag. Caught by the
  final verification sweep.
- Button primitive adopted in `share.templ` at all 33 sites:
  Export JSON/CSV/iCal/Static/Backup/Shared cards (ButtonContent
  variant for rich `<span>` children), Print config dialog
  (Close/Cancel/Generate Printable PDF), Import cards (Shared/
  Memorial JSON/Backup), Support & Diagnostics (Feedback Log/Bug
  Report Bundle), Merge Review (Inspect Diff/Keep Local/Keep
  Incoming/Keep Both), Google integration (Connect/Disconnect/
  Backup/Sheets), DixieData Calendar (Use/Sync/Unsync/Preferences
  + test variants), Calendar preferences (Close/Cancel/Save
  Preferences). `share.templ` now has zero raw button class
  strings. New `ButtonContent` variant added to the Button
  primitive for buttons with structured markup (bold title +
  muted description) — the existing string-only `Button` is for
  simple label buttons. Two `ButtonContent` regression tests
  cover the children render + type-not-duplicated invariants.
- Button primitive adopted in `entry_form.templ` at twenty-six
  sites (Fetch Data, Confirm/Cancel delete draft x2, Undo delete,
  Reapply older changes, Delete saved local draft, Add Source
  Record, Add Images From Computer x2, Save Changes / Create
  Person Record, Save Identity, Initialize Data, Back, Scan for
  Orphaned Images, Run Data Quality Scan, Save Update Source,
  Use Default GitHub Feed, Check for Updates, Export Backup,
  Download and Apply Latest Update, Move Listed Files to Temp
  Trash, Move Selected to Review Queue, Compare Selected, Quick
  View). The "Save Changes / Create Person Record" conditional-
  label pair was split into two primitive calls gated on `isEdit`.
  Test `TestEntryFormUsesMobileSafeSourceRecordAndActionLayouts`
  updated to accept both legacy `data-record-add` (bare) and
  primitive `data-record-add=""` (empty value) as semantically
  equivalent HTML. entry_form.templ now has zero raw button class
  strings.
- Button primitive adopted in `soldier_card.templ` at eleven sites
  (Browse Alphabetically, Run Advanced Search, Reset Filters, Export
  PDF, Export JPG, Send to Review Queue / Update Review Note, Mark
  as Resolved, Delete Person Record, Add Images From Computer,
  Download Selected Images, Delete Selected Images). The
  "Send to Review Queue / Update Review Note" pair required
  splitting the legacy conditional-label button into two
  primitive calls gated on `s.NeedsReview`. Anchors (Open Record,
  Compare, Open Unit Graph, etc.) and disclosure summaries stay
  unchanged — slated for Pill + future Disclosure primitives.
- Button primitive bug fix: the `{ attrs... }` spread previously
  duplicated the `type` attribute (rendered as `<button type="submit"
  ... type="submit">`). Added `buttonAttrsExcludingType` helper that
  strips `type` before the spread, so the primitive owns the type
  attribute end-to-end. Reordered attribute emission so caller attrs
  come before the kind `class=`, matching the legacy inline byte
  order (`<button type="submit" hx-post="..." class="...">Label`).
  New `TestButton_TypeNotDuplicatedFromAttrs` regression test
  asserts exactly one `type=` attribute in the rendered HTML.
- Button primitive adopted in `calendar_day.templ` at two sites
  (Save Changes, Add Item). The disclosure `<summary>` and `<a>`
  elements using button class strings remain — Button primitive
  targets `<button>` only; summary + anchor reuse is intentional
  CSS-level styling, slated for either the Pill primitive or a
  future Disclosure primitive migration.
- Button primitive adopted in `insights.templ` at five sites
  (Export Analytics Report, Audit Now, Back to Insights, Compare
  Selected, Quick View). `insights.templ` now has zero raw button
  class strings.
- Button primitive adopted in `research_collections.templ` at two
  sites (Create Collection, Add Current Person Record). The
  Compare Person Records anchor is left for the Pill migration.
- Button primitive adopted in `research_log.templ` at three sites
  (Add Research Task, Add to Research Log, Mark Resolved).
- Button primitive adopted in `layout.templ` at three sites
  (feedback modal Close, Cancel, Save Feedback). The two `<a>`
  anchors ("Add Person Record" in the top nav + floating nav panel)
  remain — they're anchor-styled-as-button, slated for the Pill
  primitive migration.
- Button primitive adopted in `browse.templ` at three sites
  (Apply Filters, Reset Browse, Clear Selection). The Print/Export
  Selected anchor is intentionally left untouched — it's an `<a>`
  styled with `.primary-button`, not a `<button>`, so it belongs to
  the future Pill primitive migration.
- Button primitive adopted in `review_queue.templ` at four sites
  (issue #74 Phase 1 migration). The bulk-action Ignore Selected /
  Delete Selected form buttons and the per-entry Mark as Resolved /
  Mark Match Resolved buttons now call `@components.Button` with
  the form attributes (`type="submit"`, `name`, `value`,
  `hx-confirm`, `hx-post`, `hx-target`, `hx-swap`) threaded
  through the `templ.Attributes` parameter. Rendered HTML is
  byte-stable against the legacy form; existing review-queue
  snapshot tests pass unchanged. `review_queue.templ` now has zero
  raw `class="primary-button"` / `class="secondary-button"` /
  `class="danger-button"` usages — a clean migration template for
  the remaining 110 button sites.
- EmptyState primitive adopted at six sites (issue #74 Phase 1.6
  migration): `/soldiers` (advanced filters, browse mode, quick
  search query, recent records, initial-state prompt) and
  `/browse` (no-results under active filter). Each call replaced a
  hand-rolled `<p class="rounded-2xl ...">` with
  `@components.EmptyState(title, body, "")`. The primitive emits
  `<div class="empty-state" data-empty-state="true">` so the audit
  harness picks up every migrated surface automatically. Existing
  entry-form + browse snapshot tests pass unchanged. Visually
  verified at 1280×800 — browse empty state renders with sepia
  dashed border + parchment surface (see
  `audit/screenshots/empty-state-browse.png`).
- Phase 1 component primitives (issue #74) continued:
  - **Field** (`internal/templates/components/field.templ`) —
    `templ Field(kind, attrs)` wraps `<input>` / `<textarea>` /
    `<select>` with the `.field-input` class. The primitive owns
    the class attribute so callers cannot double-emit it; callers
    who pass their own class string in attrs are silently ignored.
    Five golden-snapshot tests cover input default, input+class,
    input+type, textarea body, select with children.
  - **Pill** (`internal/templates/components/pill.templ`) —
    `templ Pill(label, href, extraClass, attrs)` renders an
    `<a class="pill-link" href="...">label</a>`. Three tests cover
    the default snapshot, extra-class append, and hx-* / aria-*
    pass-through (the browse pager uses these extensively).
  - **Toast** (`internal/templates/components/toast.templ`) —
    `templ Toast(kind, message)` documents the expected `toast-card`
    + `data-toast-kind` contract for future server-rendered toasts.
    The current toast rendering lives in `frontend/app.js`; this
    primitive is a contract, not a migration. One test asserts the
    class + data attribute + body content.
  - **EmptyState** (`internal/templates/components/empty_state.templ`)
    — `templ EmptyState(title, body, extraClass)` renders
    `<div class="empty-state" data-empty-state="true">` with title
    + body. The `data-empty-state` hook doubles as the audit
    harness signal so every migration lights up in round-3 reports.
    Companion CSS rule added to `frontend/tailwind.css`:
    `.empty-state` (1.2rem radius, sepia dashed border, parchment
    surface). Two tests cover default + extra-class.
- `internal/templates/components/card.templ` — Card primitive for
  issue #74 Phase 1.2. `templ Card(extraClass) { ... }` wraps the
  child content in `<div class="card ...">`. extraClass accepts the
  compound classes existing call sites use (`rounded-3xl p-6`,
  `rounded-2xl p-5 space-y-4`, etc.) so the byte-stable class string
  preserves every existing layout hook. Three golden-snapshot tests
  in `card_test.go` cover the default class, extra-class append,
  and child-content passthrough.
- `internal/templates/components/button.templ` — Button primitive
  for issue #74 Phase 1.1. `templ Button(label, kind, extraClass,
  attrs)` renders the legacy class strings (primary-button,
  secondary-button, ghost-link, danger-button) byte-stably; unknown
  kind values fall back to secondary. Layout template swaps the
  three floating-dock buttons (Scratch Pad, Feedback, Menu) to
  `@components.Button` as the proof-of-concept migration. Seven
  golden-snapshot tests in `button_test.go` cover all four kinds,
  extra-class merging, attr pass-through, and the unknown-kind
  fallback.

### Fixed

- CI: `.github/workflows/test.yml` "Restore Typst binary for render
  tests" step called `Restore-DixieDataTypstBinary` without the
  mandatory `-Root` parameter, causing the Windows runner to fail
  before `go test` could run on `internal/archive` and `pkg/render`.
  Resolve `$root` via `Get-DixieDataRoot` (already exported from
  `scripts/build-common.ps1`) and pass it through.
- CI: `nextGoogleAnniversaryDate` in both
  `internal/integrations/google_service.go` and
  `internal/archive/compat.go` built the anniversary `candidate`
  in `time.Local`. On UTC CI runners this produced a UTC midnight
  time that shifted to the previous calendar day when downstream
  callers converted to a non-UTC location (e.g. America/Chicago),
  surfacing as `start.DateTime = "2027-05-12T..."` instead of
  `"2027-05-13T..."` in the Google Calendar event. The Google
  Calendar test (`TestGoogleCalendarEventBuildsYearlyTimedEvent
  WithReminders`) failed on CI for this reason even though it
  passes locally where `time.Local = America/Chicago`. Added an
  explicit `location *time.Location` parameter so callers (and
  tests) build the candidate in the same location that will format
  the final event. Both function copies and three call sites
  (two production, three test) updated. Verified green under both
  `TZ=America/Chicago` (local) and `TZ=UTC` (CI).
- CI: `Restore-DixieDataTypstBinary` in `scripts/build-common.ps1`
  checked `$LASTEXITCODE -ne 0` after `Expand-Archive`, but
  `Expand-Archive` and `Invoke-WebRequest` are native pwsh cmdlets
  and do not set `$LASTEXITCODE`. In script scopes where no prior
  external command ran (the GitHub Actions test workflow is one),
  the read of `$LASTEXITCODE` threw `The variable '$LASTEXITCODE'
  cannot be retrieved because it has not been set` and failed
  CI. Switched to `$?` (success-of-last-command automatic variable,
  always defined) — the canonical pwsh idiom for catching cmdlet
  failures. Other `$LASTEXITCODE` checks in the file follow
  `& <external.exe>` calls (tar, npm, templ, wails) and remain
  correct.

### Added

- `make ui-diff` target for v1-vs-v2 visual regression (issue #74
  Phase 0 PR4). `scripts/ui-diff.mjs` boots Playwright against the
  running `dixiedata-web` server, walks four routes (`/`,
  `/soldiers`, `/browse`, `/settings`) at desktop (1280×800) and
  mobile (390×844) viewports, captures both `?ui=v2`-off (v1) and
  `?ui=v2`-on (v2) screenshots per surface, and writes a JSON
  summary to `audit/reports/ui-diff/summary.json`. Reuses
  `audit/harness.mjs` helpers (`detectVisualIssues`) so v1 vs v2
  visual heuristic diff lands in the same shape as the existing
  audit reports. Connection-refused failures exit with code 2 and
  a friendly pointer to `audit/README.md` instead of a stack
  trace. Eight PNGs (~3.7 MB) captured on first end-to-end run.
- `?ui=v2` query-string feature flag: `internal/uiver/uiver.go` exposes
  `Middleware` (reads `?ui=v2` and stores a boolean on the request
  context) and `IsV2(ctx)`. `internal/appshell/routes.go` wraps the
  mux with `recoverMiddleware(uiver.Middleware(mux))`. The Wails
  desktop build never sends `?ui=v2`, so production behavior is
  unchanged; future component-primitive refactors (#74 Phase 1) can
  branch on `IsV2(ctx)` and ship behind the flag without forcing a
  binary rollback. The `Layout()` template wrapper dispatches to a
  new `LayoutV2()` stub (currently a minimal passthrough shell) so
  end-to-end verification is possible in web-mode audits. New
  `internal/uiver/uiver_test.go` exercises five cases: default
  context, explicit v2 context, no query param, `?ui=v2`, and
  rejection of any other value (`v1`, `V2`, `v2x`, `true`, `1`).
- Design tokens wired into `tailwind.config.js` `theme.extend`:
  `gold`, `sepia-500`, `sepia-300`, `parchment`, `parchment-soft`,
  `ink`, `ink-muted`, `ink-faint`, `bg-amber-50`, `bg-slate-200`,
  `review-red`, `review-red-tint`, `success-green`, `error-red`,
  `radius.surface`, `radius.dialog`, `shadow.card`, `shadow.modal`,
  `motion.fast`, `motion.med`. Tailwind generates the utility
  classes; no existing CSS or template literal is migrated yet —
  that follows in per-component-class PRs (PR2a/PR2b/...) so each
  pixel shift is reviewed in isolation. Hex literal migration
  follows the locked names from ADR-0003.
- ADR-0003 design system tokens: `docs/adr/0003-design-system-tokens.md`
  locks the color, radius, shadow, motion, and typography vocabulary
  for the #74 Phase 1 component primitives. The companion
  `docs/adr/0003-design-system-tokens-reference.md` lists every token
  name + canonical value + intended use. Subsequent component
  extractions reference these names instead of inventing new ones.
- Implementation plan for the remaining open work of issue #74 (UI/UX
  revamp): `.rpiv/artifacts/plans/2026-06-25_74-ui-revamp.md`. Six
  phases, ~22 PRs sequenced behind `?ui=v2`; Phase 0 (htmx load in
  web-mode `index.html`, ADR-0003 design tokens, `?ui=v2` flag, and
  `make ui-diff` harness) detailed for immediate execution.
- Test, build, and audit GitHub Actions workflows (`.github/workflows/test.yml`,
  `build.yml`, `audit.yml`). Test runs `go test -short` on every push; build
  verifies the Wails binary builds and embeds no absolute source paths (the
  `-trimpath` flag); audit runs the UI/UX harness weekly and on PRs touching
  templates or frontend.
- `scripts/bump-version.ps1 -VerifyOnly` — non-mutating validation pass that
  fails the build if `versioninfo.go`, user-manual, implementation-and-features,
  ai-handoff, or CHANGELOG disagree on the current version.
- Reproducible Typst + PDFium bootstrap (`scripts/build-common.ps1`): downloads
  pinned releases, verifies SHA256, refuses to install on mismatch. A fresh
  clone can build without manually vendoring binaries.
- `bin/MANIFEST.md` — authoritative list of every native binary the build
  pipeline expects, with version, source URL, pinned SHA256, and an upgrade
  procedure.
- `scripts/token-clean.ps1` sweep extensions — removes untracked `*.exe` from
  repo root and release zips older than the last two tags.

### Changed

- Implementation stack reference (`docs/implementation-and-features.md`) now
  lists the Typst CLI as the PDF renderer (the `go-pdf/fpdf` path was retired
  in slice 7). Section 6.7 carries a migration note.
- User-manual, implementation-and-features, and ai-handoff now agree on the
  current release line (`v1.2.55`); the version source of truth is
  `internal/versioninfo/versioninfo.go`.
- `Makefile` `render-svg` target guards on the local `render-svg.sh` script
  and exits 0 with a skip message on machines where the script is absent
  (was a hard failure before).
- 7 previously undocumented `Makefile` targets (`tune`, `tune-smoke`,
  `tune-snapshots`, `render-round`, `render-round-ONE`, `update-snapshots-ONE`,
  `render-svg`) now print descriptions in `make help`.
- Stress test files (`internal/appshell/app_stress_test.go`,
  `tests/stress/*.go`) honour `testing.Short()` — `make test` skips them,
  `make stress` still runs them.
- `wails build` in `scripts/build-common.ps1` passes `-trimpath` so
  distributed binaries do not embed absolute source paths.

### Fixed

- `.gitignore` no longer ignores `google-oauth-defaults.example.json` (the
  example is intentionally tracked; the entry made contributors think their
  edits to the example were being saved).
- `tests/goldmaster/playwright/test-results/.last-run.json` is no longer
  tracked (was a runtime artifact slipping through the gitignore filter).
- 6 release zips older than the last two tags removed from `release/`
  (cleaned by the extended `token-clean.ps1`).
- Performance fixes from the 2026-06-24 sweep (issue #107): the quick-search
  form now carries `hx-sync="this:replace"` so each new keystroke aborts the
  in-flight XHR (no more out-of-order responses); `BrowsePage` runs the
  count + paginated select in a single CTE so every browse filter change
  costs one round-trip instead of two. `FormSuggestions` caching,
  `BrowseResults` hx-boost partial, and the FTS snippet column were already
  shipped before this batch landed; `RecentSummary` projection (7.2) and
  feedback retention setting (7.13) are deferred to a future pass.
- Background-job pattern for long exports (issue #100): adds an
  in-process job registry (`internal/jobs`) and a new `/jobs/{id}`
  status page. The share page's Static Archive and Printable PDF
  exports accept `?async=1` and now run as background jobs that
  the user can poll and cancel from a dedicated progress page
  instead of blocking the HTTP goroutine for minutes. Issue #125
  closes out the visible part of the flow: completed exports now
  expose a `/jobs/{id}/artifact` endpoint that streams the saved
  file back with a `Content-Disposition: attachment` header, and
  the status page renders an `Open {kind}` pill-link instead of
  the previous text-only `Saved to …` line. Issue #122 caps
  concurrent workers with a semaphore (default 2, override via the
  `DIXIEDATA_JOBS_CONCURRENCY` env var); saturated submissions
  stay in `queued` until a slot frees. Issue #123 wires the
  registry to a JSONL log in `dataDir/jobs.jsonl` so completed
  exports survive a webview reload or app restart; jobs that were
  `running` when the previous process exited are flagged
  `interrupted` so the status page is honest about lost work.
  Issue #120 documents the FTS snippet picker (it uses MAX-of-three
  snippets, not a CASE rewrite, because SQLite's `snippet()` returns
  non-empty text for any FTS match in a row regardless of which
  column actually matched) so the next reader does not refactor it
  into a regression. Issue #118 adds the same alt-text sanitisation
  the SoldierCard thumbnail already has (issue #99) to the image
  preview modal so pasted HTML in captions never lands in an alt
  attribute. Issue #117 converts the three modal dialogs
  (feedback / print-config / google-preferences) to native
  `<dialog>` elements so focus trapping, ESC-to-close, and
  inert-background come from the browser instead of a custom
  div overlay. Issue #124 adds `/jobs/{id}/stream` so the
  registry can push Server-Sent Events to clients in real time;
  the existing `/jobs/{id}/status` htmx polling endpoint stays
  as the primary visible path, and a future change can swap the
  page over to `EventSource` when the audit harness asks for it.
  Issue #126 makes the call on whether the fast exports
  (`/export/json`, `/export/csv`, `/export/ical`,
  `/export/backup`) should migrate to the background-jobs
  pattern: they stay synchronous because each runs in well under
  a second on a 1000-record archive; only the two exports flagged
  as blockers in the audit (`/export/database-pdf` and
  `/export/static-archive`) accept `?async=1`. Issue #121 adds a
  startup prune of the feedback log (default 365-day retention)
  so the JSONL file stops growing unbounded on long-running
  desktop sessions; the prune is best-effort, leaves corrupt
  lines in place, and ships without a settings UI toggle (the
  retention window is hard-coded for now). Issue #119 slims
  `RecentByIDs` from 45 to 38 columns by dropping the correlated
  record/image count subqueries and the long-form fields the
  recent-search view never renders; a smoke benchmark tracks the
  new path.
- Search results no longer render the highlighted `SoldierCard` pill row
  (entry-type / death-date / burial-place). The same data now appears as
  a small plain `<dl>` inside the card. The `Needs Review` pill row stays
  as it was.
- Accessibility audit findings from the 2026-06-24 sweep (issue #99):
  quick-search input gets a meaningful `aria-label` (no longer `q`);
  search results pagination lives inside an `aria-label`-ed `<nav>`
  landmark with `aria-current="page"`; image thumbnails fall back to
  `Image for Person Record {DisplayID}` alt text when the caption is
  blank and strip HTML from non-blank captions; the browse results
  table declares `scope="col"` on every header; the disabled `Compare
  Selected` button is `aria-describedby` the manual-comparison help text;
  the feedback message `<textarea>` declares `aria-required="true"`
  alongside `required`; the feedback / print-config / google-preferences
  modals declare `role="dialog"` + `aria-modal="true"` and are
  `aria-labelledby` their `<h3>` heading; `lang="en"` on the root
  `<html>` carries a comment marker for the future i18n pass.

### Removed

- `audit/package.json` (deps merged into root `package.json`; the `audit`
  npm script now lives there too).
- Sub-768px hamburger drawer from the top nav (`data-top-nav-toggle`,
  `#top-nav-drawer`, `initializeTopNav` handler, and the
  `@media (max-width: 780px)` block in `frontend/tailwind.css`). DixieData
  is a Wails desktop app; the drawer was dead UI. The split-screen
  breakpoints (`max-width: 1040px`, `1100px`, `900px`) and content-template
  `md:hidden` / `md:flex` toggles stay (16" monitor split-screen layout).

### Maintenance

- `audit/reports-r3/audit-v3.md` narrative summary written, matching the
  structure of round 1 / round 2 reports.
- `AGENTS.md` expanded with a glossary index pointing at `CONTEXT.md` and an
  11-row file map of the codebase entry points.
- `bin/README.md` documents the current typst platform gap (Windows shipped,
  macOS / Linux land with the bootstrap follow-up).
- Cumulative PR1+PR2+PR3 of issue #42 (God-class reduction) completed:
  `internal/appshell/app.go` shrank from 4,334 to 2,116 LOC across the
  PRs below; 11 new domain files created under `internal/appshell/`.
  All 72 registered routes preserved; all 17 test packages pass.
  - PR1: extracted `internal/archive/pdf_layout.go` and
    `internal/archive/static_archive.go` from `export_service.go`
    (4,510 → 1,610 LOC).
  - PR2: split `internal/appshell/app.go` into 10 new files
    (`routes.go`, `lifecycle.go`, `google_handlers.go`, `calendar_handlers.go`,
    `imports_handlers.go`, `exports_handlers.go`, `settings_handlers.go`,
    `insights_handlers.go`, `research_handlers.go`, `soldiers_handlers.go`,
    `reviews_handlers.go`). Each PR step was a pure file move with no public
    API or behavior change.
- `Makefile` added as the preferred entry point for build / test / asset
  generation / release tasks; every target routes through PowerShell with
  verbose output captured to `build/log/<target>.log` and `pipefail` so
  failures propagate.
- `scripts/bump-version.ps1` (`make bump`) — strict schema-version increment
  with paired-migration-note enforcement.
- `scripts/release-github.ps1` (`make release-github`) — tag + push + draft
  GitHub release with five safety gates before any mutation.
- `docs/RELEASING.md` — release-process documentation.
- Generated `*_templ.go` and `frontend/wailsjs/*` untracked from the index
  (regenerated by `make tpl` and `wails build`).
- `.gitignore`, `.agentignore`, `.aiderignore`, `.cursorignore` hardened with
  canonical GOTH/Wails patterns plus `build/log/` for captured build output.
- `Makefile` `help` target now lists every defined target. The regex
  `^[a-zA-Z_-]+:.*?## ` requires the `## doc` on the same line as the target
  name; multi-target rules like `build debug:` or `test test-quiet:`
  satisfied that for only the first token (and sometimes not at all when
  `##` sat on a recipe-body line). Three targets — `build`, `debug`, `test`
  — were silently hidden. Split each into single-target rules with their
  own `## doc` line. No behavior change; `make -n` confirms identical
  recipes.
- `internal/jobs/jobs_test.go` `TestSetResultBroadcastsSnapshot` had a race
  that surfaced intermittently under `go test ./...`: the worker goroutine
  from `Start` broadcasts StatusRunning before the test could call
  `Subscribe`, so the channel received the wrong snapshot on the next read
  and the assertion against `SetResult`'s broadcast saw
  `ReplacedRecords=0` / `MigrationRan=false`. Drain the channel until the
  `SetResult` snapshot arrives (identified by `MigrationRan=true`), with
  the same 1s deadline. Verified stable over 10 standalone runs and 3
  consecutive `go test ./...` invocations.

### Fixed

- Landing-page calendar layout when the Local Archive has zero Person
  Records (issue #213). The `EmptyStateCard` (welcome panel) previously
  rendered as a sibling before `CalendarGrid` in the `.calendar-layout`
  two-column grid, so CSS Grid auto-placement put it in column 1 (1fr)
  and shoved the calendar into column 2 (390px) — smashed against the
  right edge. Swap the conditional so the EmptyStateCard replaces the
  `details-pane` slot in column 2 instead of sitting alongside it; the
  `CalendarGrid` now lands in column 1 at full width in both the empty
  and populated states. The `details-pane` div only renders when
  `TotalRecords > 0`, matching the column count. Two regression tests
  in `internal/templates/calendar_test.go` (`TestCalendarEmptyStateSwapsDetailsPane`,
  `TestCalendarPopulatedRendersDetailsPane`) assert DOM ordering and the
  presence/absence of the welcome panel + details-pane for both states.

- **Restructured the `/share` landing page into focused sections**
  (issue #265). Added a Quick Actions card above the fold with
  three hardcoded tiles (Export JSON, Load Backup, Share Queue)
  so a returning user can complete the most common task without
  scrolling past the long tail. Added a Recent Activity card
  showing the last 3 terminal jobs (done / error / cancelled /
  interrupted) sorted by StartedAt desc, with an empty-state
  message for new installs. The existing 2-col grid (All Exports
  + All Imports) and the Sync + Support & Diagnostics cards
  remain below the fold unchanged. New generic primitive:
  `@components.QuickAction` (large tile: icon+label+description+arrow).
  New component: `@components.RecentJobs` (last N jobs card with
  status pills + relative timestamps). New query:
  `jobs.Registry.RecentJobs(n)` returns terminal jobs sorted by
  StartedAt desc, excluding queued + running. New viewmodel:
  `viewmodel.RecentJobEntry`. New uiids: `PanelShareQuickActions`,
  `PanelShareRecent`, `PanelShareAllExports`, `PanelShareAllImports`,
  `PanelShareSync`, `PanelShareSupport`. CSS fix: foldout panel
  now uses `display: none` by default (was `display: flex` which
  was overriding the Tailwind `.hidden` class — caused 480px
  overflow in #264's smoke probe, surfaced by #265's). Regression
  net: `audit/smoke_share_landing.mjs` (15/15 assertions cover
  section presence, ordering, tile correctness, recent activity
  empty/populated states, 480px responsiveness, and the Export
  JSON click → POST /export/json → /jobs/{id} → Recent activity
  populates round-trip).

- **Fixed Share foldout menu items invisible (1.00:1 contrast)**
  (issue #283). The foldout menu items inherited `color: #22303d`
  (dark slate from the parent `.pill-link`) but the panel
  background was dark navy `rgba(36,48,61,0.96)`, giving a
  contrast ratio of 1.00:1 — effectively invisible at every
  WCAG level. The user perceived "items don't show" because
  they were functionally invisible. After the first click
  the user knew to look for them, so the "items appear after
  clicking another nav first" pattern was a perception bug,
  not a clipping bug. Fix: `.foldout-menuitem` now uses
  explicit cream `color: #f2ede1` (the same cream the top-nav
  brand text uses on the dark surface) — 11.49:1 ratio,
  passes WCAG AAA. The focus-visible state uses an even
  brighter `#fff8e7`. The diagnostic that traced the
  ancestor chain found no `overflow: hidden` clipping; the
  panel was correctly visible the whole time, just at
  zero contrast. Regression net: `audit/smoke_foldout_nav.mjs`
  extended from 24 to 32 assertions (+8 covering items
  visible in the painted viewport on /calendar + contrast
  ratio >= 4.5:1 for each menuitem).

- **Share top-nav foldout** (issue #264). Replaced the two
  flat top-nav links (Share + Share Queue) with a single
  Share trigger that opens a foldout panel containing 4
  menu items: Export, Import, Share Queue, and Build Share
  Archive. Trigger is a real `<button aria-haspopup="menu">`
  with a chevron `▾`; panel has `role="menu"` and 4
  `<a role="menuitem">` items (anchors, not divs, so
  middle-click / cmd-click / screen readers all work).
  Sub-items deep-link to anchors on /share: Export →
  `#export-section`, Import → `#import-section`, Build
  Share Archive → `#build-share-archive` (new anchors
  added to share.templ). The foldout is the first consumer
  of a generic pattern — any future nav item can adopt the
  same `data-foldout-trigger` / `data-foldout-panel` shape
  and `installFoldouts()` in app.js picks it up uniformly.
  ARIA: only the trigger gets `aria-current="page"` when
  on a /share/* path (per the issue's locked decision;
  sub-items stay plain). Keyboard: ArrowDown/Up wrap
  through menuitems (WAI-ARIA menu pattern), Home/End jump
  to first/last, ESC closes and returns focus to the
  trigger. New generic primitive: `@components.Foldout`
  (internal/templates/components/foldout.templ). New uiids:
  `LayoutShareMenu` + `LayoutShareMenuTrigger`. Regression
  net: `audit/smoke_foldout_nav.mjs` (24/24 assertions
  cover ARIA contract, open/close via click/ESC/outside-
  click, keyboard nav, aria-current on /share, anchor
  deep-links).

- **Wire the existing PATCH `/export/templates/{id}` endpoint**
  (issue #258 / #186). The backend (`ExportTemplateService.Update`
  + `handleUpdateExportTemplate` + chi route) was fully landed
  in PR #195 but the router only registered `r.Patch(...)`.
  The JS handler `updateSelectedTemplate()` POSTed to the same
  path (the handler body accepts both PATCH and POST), so the
  Save Changes button on the print-config modal's Saved
  Templates dropdown was silently returning 405 Method Not
  Allowed. Added a `r.Post("/export/templates/{id}", ...)`
  alias so the POST fetch lands; PATCH stays canonical per
  REST. Regression net: `audit/smoke_template_edit.mjs`
  (8/8 assertions) — seeds a template via the Save endpoint,
  opens the print-config modal, loads the template, clicks
  Save Changes, asserts 200; renames + Save Changes again,
  asserts rename persisted in `/export/templates`.

- **Removed the deprecated `#share-status` placeholder panel from
  `/share`** (issue #254). The panel was made obsolete by the
  `/jobs/{id}` job status pages (issue #193): every export and
  import action now redirects to a deep-linkable job page for its
  status feedback, so the in-page panel had no writers left. The
  removed surface includes the placeholder paragraph, the dead
  `memorialImportPreviewMarkup` / `memorialImportSummaryMarkup` /
  `memorialImportIssuesList` helpers (orphaned by commit 3748db7's
  memorial flow migration to `/jobs/{id}`), the unused
  `scrollShareStatusIntoView` + `htmx:afterSwap` hook + dead
  `shareStatusTarget()` helper in `frontend/app.js`, the
  `#share-status` assertion in `share_test.go`, three deprecated
  audit probes (`probe-backup-status.mjs`,
  `probe-share-status-scroll.mjs`, `run-backup-status.mjs`), and
  the `#share-status` references in `probe-full-restore.mjs`. A
  new empty `<div id="memorial-preview-target">` slot is left
  inside the Memorial JSON import card as a documented attach
  point for future in-page feedback (per the issue). The export
  wireframe (`docs/ui-map/wireframes/08-export.md`) is updated to
  reflect that all import buttons now redirect to `/jobs/{id}`.
  Regression net: `audit/smoke_memorial_json_preview.mjs`
  (7/7 assertions).

### Maintenance

- **htmx-guard lint probes (issue #316, slice 1)** — two new
  static-analysis walkers under `audit/` (sibling to
  `discover_orphan_handlers.mjs`) catch recurring
  attribute-drift classes:

  - **`discover_htmx_guard.mjs` toast-no-redirect walker** —
    for every `func` in `internal/appshell/*.go` (excluding
    `_test.go`), flags any function whose body calls
    `setInfoToastHeader(w, ...)` but lacks ALL of
    `X-DixieData-Redirect`, `writeExportRedirect(`,
    `enqueueExport(`, `respondDuplicateInFlight(`. Catches
    the toast-without-redirect class (commit `70878ac →
    3612dab` is the canonical repo incident). Brace-walks
    the function body so nested closures do not
    false-positive.

  - **`discover_htmx_guard.mjs` JS submit coexistence
    walker** — walks `frontend/app.js` for every
    `addEventListener("submit", ...)` site. Legitimate
    sites are either doc-level delegates branching on
    `data-dixie-submit` (route to `dispatchDixieDataForm`),
    or utility submits carrying `// htmx-guard:
    utility-submit` on the preceding line. Both shapes are
    documented at `docs/agents/htmx-guard-conventions.md`.

  - **`audit/discover_htmx_guard.test.mjs`** — 10-test
    regression net covering all branches (toast-no-redirect
    flag, clean handler, test-file skip, definition-site
    skip, JS submit flag, data-dixie-submit delegate
    acceptance, marker acceptance, `--strict` exit code).

  - **Makefile targets:** `make lint-htmx-guard`
    (informational), `make lint-htmx-guard-strict` (CI
    failure mode), `make lint-htmx-guard-test` (run the
    10-test suite).

  - **Follow-up #317** filed to retire the marker
    convention once a canonical `dispatchUtilitySubmit(form)`
    exists and the 3 marked sites migrate to use it. Slice 1
    ships with the markers as documented debt; the cleanup
    is a separate task.

  - **htmx-guard orphan target walker (issue #316, slice 2)** —
    extension to `audit/discover_htmx_guard.mjs` that scans every
    `internal/templates/**/*.templ` file (recursive, covers
    `partials/`) for `hx-target`, `data-results-target`, and
    `data-status-target` attributes. Flags any `#X` selector whose
    `id="X"` does not exist anywhere in the templ tree. htmx +
    `dispatchDixieDataForm` both write into the resolved element;
    a missing id is a silent no-op UX bug (user sees no feedback).

    Rules:

    - `#X` selectors MUST have a matching `id="X"` somewhere in
      the templ tree.
    - `hx-target="this"` (htmx self-pseudo) is always allowed.
    - All other selectors (`[data-...]`, `body`, `.cls`,
      `:nth(...)`) are valid CSS without an id counterpart and
      pass.

    Mirrors the same # vs non-# rule already encoded in
    `internal/htmxattr/htmxattr.go:155-167` — the templ walker
    is the static-analysis twin of the runtime `validateTarget`
    helper.

    9 new tests in `audit/discover_htmx_guard.test.mjs` covering
    clean baseline, orphan detection, `this` skip, non-`#`
    skip, `data-results-target` coverage, subdirectory recursion,
    cross-file id resolution, and `--strict` exit code (19/19
    total tests pass).

    Conventions doc updated at
    `docs/agents/htmx-guard-conventions.md` ("Target selector
    rule" section).

  - **htmx-guard polling-stop regression net (issue #316, slice 3)**
    — new Go test `internal/templates/jobs_templ_test.go`
    renders `jobs.JobStatusFragment` and
    `jobs.JobStatusSlotFragment` against all 5 `JobStatus` values
    (Done, Error, Cancelled, Interrupted, Running). For the 4
    terminal states the rendered output MUST contain
    `hx-trigger="none"` and MUST NOT contain `hx-trigger="every 2s"`.
    For `StatusRunning` the inverse must hold.

    A pure-JS lint cannot read the templ `if` branch condition
    (it lives in generated Go), so polling-stop is the one
    rule in #316 that lands as a templ-rendering test rather
    than a `discover_htmx_guard` walker. Confirms the rule the
    comment block in `jobs.templ:139-146` documents.

    10 subtests pass (2 test functions × 5 status cases).
    Both `JobStatusFragment` (rendered on `/jobs/{id}`) and
    `JobStatusSlotFragment` (rendered into the layout overlay)
    are covered; the slot fragment lives in a separate templ
    file (`job_slot_fragment.templ`) with an identical
    polling branch shape but a distinct rendering path, so it
    needs its own test.

  - **htmx-guard validateTarget dev-build panic (issue #316,
    slice 4)** — enable the panic in
    `internal/htmxattr/htmxattr.go::validateTarget` for `#X`
    selectors whose X is not in the htmlids registry. Caught in
    dev/test builds; production behavior unchanged. Catches the
    typo class (e.g. `#browze-results`) at the moment the templ
    renders, before the page ships.

    Lands as TWO commits (sequence matters):

    1. **New package `internal/htmlids`** — mirrors the
       `internal/uiids` shape but tracks DASHED HTML id strings
       (the literal `id="..."` values templ emits and htmx
       selectors point at) instead of DOTTED logical surface
       ids. `uiids` stays untouched — it's the right shape for
       what it tracks. Initial registry: `BrowseResults`,
       `SoldierList`.

    2. **`validateTarget` panic + test rewrite** — switches the
       `validateTarget` body from a `uiids.Has()` lookup (which
       could never match `#browse-results` against
       `uiids.PanelBrowseResults = "panel.browse.results"`) to
       `htmlids.Has()`. Adopts a 2-file clean-break strategy:
       `TestMuxAdHocTargetDoesNotPanic` was the test that
       previously accepted `#feedback-form`; with the panic
       enabled it would always fail, so it is REPLACED by
       `TestMuxPanicsOnUnknownRegistryTarget` (#typo),
       `TestMuxAcceptsRegisteredSelectors` (loops the htmlids
       registry and asserts every entry produces a valid Mux),
       and `TestMuxTargetNonHashSelectorsPass` (body /
       [data-...] / .cls / this all still allowed).

    The plan §Slice 4 promised 1 LOC; the actual implementation
    is ~140 LOC across two commits. The original "1 LOC" was a
    miscalculation — `uiids` uses dotted notation for logical
    surfaces while hx-target uses dashed notation for CSS
    selectors, and the two registries must be separate to
    answer different questions (WHAT is the page vs WHAT is the
    element id). Conflating them would force one notation to
    leak into the other.

    `TestMuxSelectEmitted` switched its sample from
    `#countsForm` (a synthetic fixture not in either registry)
    to `#browse-results` (a real registered selector), so the
    test continues to verify Select rendering without firing
    the panic.

  - **htmx-guard CI wiring (issue #316, follow-up)** — new step
    in `.github/workflows/test.yml` runs `make
    lint-htmx-guard-strict && make lint-htmx-guard-test` after
    the Cli-coverage drift detector. The strict walker is the
    CI failure gate; the test suite is the regression net on
    the probe itself. Both targets are Node-based so the
    existing `actions/setup-node@v4` step above already
    provides the toolchain — no extra setup. Same `shell:
    bash` style as the Cli-coverage sibling. Wired at PR-push
    on `dev` and `stable`; failure blocks the merge via the
    branch-protection rule.

    Closes #316's CI gate promise that slices 1-4 explicitly
    deferred.

  - **htmx-guard: dispatchUtilitySubmit + dispatchSubmitPrep
    (issue #317)** — new sibling helpers in `frontend/app.js`
    replace the `// htmx-guard: utility-submit` marker
    convention. The two helpers are the canonical submit
    semantics for utility-form submits (those without
    `data-dixie-submit`):

    - `dispatchUtilitySubmit(form, callback)` —
      `preventDefault` + run the callback. Used for
      local-only handlers like the share-queue preset
      save.
    - `dispatchSubmitPrep(form, callback)` — run the
      callback but allow the submit to continue. Used
      for pre-submit hooks on forms that already have
      their own submit semantics downstream (e.g. the
      share-queue export form stages hidden fields then
      the data-dixie-submit dispatcher takes over).

    Lands as two commits:

    1. **Helpers + probe acceptance** (this commit) —
       both helpers added to `frontend/app.js`; the
       walker's classification gains a new "Case C"
       branch that recognizes calls to either helper as
       legitimate; pre-scan to skip addEventListener
       sites that live inside the helpers' own bodies
       (avoid flagging the helpers' implementation
       details as violations of themselves). 4 new
       tests cover helper acceptance + nested helper
       call + helper-internal skip.

    2. **(next commit)** migration of the 3 marked
       sites to route through the helpers + removal of
       the 3 marker comments + update to
       `docs/agents/htmx-guard-conventions.md`
       replacing the marker section with a helpers
       section. After that commit the marker rule can
       retire; this commit keeps the marker rule as a
       fallback for any unmigrated reader.

  - **htmx-guard: migrate utility-submit sites to helpers
    (issue #317, follow-up)** — three historically-
    annotated sites in `frontend/app.js` migrated to
    route through `dispatchUtilitySubmit` (line 3995,
    share-queue preset save) and `dispatchSubmitPrep`
    (line 4067, share-queue export form staged hiddens;
    line 5316, PDF preferences persistence). The three
    `// htmx-guard: utility-submit` markers removed.

    `dispatchSubmitPrep` is the right helper for sites 2
    and 3 — both forms are `data-dixie-submit="true"`
    and a downstream dispatcher runs after the prep hook.
    The helper does NOT preventDefault so the data-dixie-
    submit dispatcher continues to fire.

    Conventions doc updated — the "The marker" section
    becomes "The helpers" section, with a separate
    "deprecated, retained as fallback" subsection for
    the marker. Author checklist updated to require the
    helpers, not the marker. Marker rule REMAINS in the
    probe as defensive fallback for any future
    contributor or legacy reader who reaches for the old
    convention.

    Probe stays clean on dev HEAD across the migration;
    full short `./...` suite unaffected (helpers are
    pure JS).

    Closes #317.

## v1.2.55 - 2026-06-25

### Added

- `internal/models/constants.go` with `EntryTypeSoldier`, `EntryTypeWife`,
  `EntryTypeWidow`, `EntryTypeLinkedPerson`, and the `EvidenceType*` family
  (`LocalArchive`, `SharedArchive`, `BackupArchive`, `StaticArchive`,
  `RestorePoint`, `MemorialJSON`, `FindAGrave`, `PensionRecord`,
  `ApplicationRecord`, `Other`). Templates and viewmodels now reference
  these constants instead of bare string literals.

### Changed

- `soldiers.entry_type` carries an application-level discipline enforced at
  the migration boundary (`internal/db/schema.go` `migrateEntryTypeDiscipline`).
  Any future INSERT or UPDATE with a value outside the canonical set is
  rejected. SQLite CHECK constraints cannot be added in-place; the function
  records a one-time migration log so the rule is enforced on every
  subsequent schema open.
- `research_log.evidence_type = 'archive'` was rewritten in place to
  `'local_archive'` to match the glossary. A forward-only helper
  (`isNoSuchTableError`) lets the migration succeed on archives where the
  `research_log` table does not yet exist (planned for v56+).

## v1.2.54 - 2026-06-08

### Fixed

- Hardened calendar sync UX and popout layout.

## v1.2.53 - 2026-06-08

### Added

- Managed calendar event preferences and a dry-run sync mode.

## v1.2.52 - 2026-06-08

### Changed

- Enforced Chicago timezone for calendar sync and iCal export.
- Synced calendar events stay at the user's local morning hour.

## v1.2.51 - 2026-06-08

### Fixed

- Google Calendar reminder payload format.

## v1.2.50 - 2026-06-08

### Added

- Google calendar timezone fallback coverage.

### Fixed

- Google Calendar sync timezone requirement.

## v1.2.49 - 2026-06-08

### Fixed

- Bumped release line forward; broadened server-side post-update trust clear
  and hardened launch-state clearing.
- Fixed UI freeze on the intro screen caused by a `setBusyGroupState`
  ReferenceError.
- Hardened startup bootstrap and bundled OAuth defaults in release zips.

### Added

- Pre-update backup and managed Google calendars.
- Settings data-quality scan workflow.
- Previewed memorial JSON import workflow.

## v1.2.45 - 2026-06-07

### Fixed

- Stabilized search hydration.
- Added landscape biography pages and safer draft delete.
- Shipped export layout help.
- Made edit drafts version-aware.
- Clarified stale draft review copy.
- Tightened compressed quick-action buttons.

## v1.2.37 - 2026-06-01

### Fixed

- Fixed calendar alignment.

## v1.2.36 - 2026-05-31

### Fixed

- Fixed release build import.
- Fixed browse filters.

## v1.2.35 - 2026-05-31

### Fixed

- Fixed printable export modal viewport.

## v1.2.34 - 2026-05-31

### Fixed

- Fixed normalized pension-state filtering.

## v1.2.33 - 2026-05-31

### Fixed

- Fixed split-screen layouts.

## v1.2.32 - 2026-05-31

### Changed

- Polished calendar and browse workflows.

## v1.2.31 - 2026-05-31

### Added

- Calendar items and display fixes.

## v1.2.29 - 2026-05-30

### Maintenance

- Bumped release line forward.

## v1.2.28 - 2026-05-30

### Added

- Restore points for in-place updates.
- Single-record JPG export polish.
- Made scratchpads database-backed.
- Browse and startup improvements.
- Linked-person records renamed to person records.
- Shared import memory and software updates.

## v1.2.22 - 2025 (date not captured at tag) - Minor Release

- Added the generic linked-person workflow across entry creation, presentation, import/export paths, and legacy backup restore.
- Added clickable internal `[[DISPLAY-ID]]` links, global feedback capture/export, and maiden-name italics across live and exported views.
- Fixed printable PDF record cards so audit metadata is omitted and oversized entries continue onto additional pages instead of shrinking to unreadable text.
- Moved the production release line forward to `v1.2.22` so runtime metadata, build packaging, and docs stay aligned with the current feature set.

## v1.1.21 - Patch Release

- Replaced the UI's remote Tailwind CDN dependency with a checked-in local CSS bundle so desktop installs render correctly on offline machines.
- Automated CSS regeneration in the shared PowerShell build path so debug and release builds refresh the bundled stylesheet before Wails packaging.
- Carried the release line forward to `v1.1.21` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.20 - Patch Release

- Fixed the entry-form draft recovery banner so the **Discard local draft** button remains visible after the page initializes and after draft-status updates run.
- Carried the release line forward to `v1.1.20` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.19 - Patch Release

- Fixed the new-record localStorage draft flow so successful creates clear the cached entry instead of repopulating the next record form.
- Added an in-app **Discard local draft** recovery action on new/edit record forms so stuck entry drafts can be cleared without DevTools or a debug build.
- Enabled Confederate Home fields for wife and widow records in the entry form.
- Carried the release line forward to `v1.1.19` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.18 - Full Release

- Hardened `.ddbak`, `.ddshare`, diagnostics, and static archive ZIP creation to write through a temp file, verify ZIP finalization, and only then replace the destination file.
- This avoids success-shaped partial archives caused by unverified final ZIP close/flush behavior at the final save path.

## v1.1.17 - Patch Release

- Fixed the static web archive detail view so exported `index.html` and `viewer.html` can open a selected person without leaving the expanded data area blank.
- Carried the release line forward to `v1.1.17` so the runtime metadata, Wails title, exported artifacts, and docs stay aligned.
- Migrated `handleCalendarPDF`'s nine `slog.Debug` entry/exit/branch
  calls to `trace.Log` per ADR 0006 (issue #221). Each call loses
  zero value at INFO+ (structural only) and gains structured
  attributes that render in the Debug Console and JSONL log under
  `-tags debug` builds. Path PII drops from `dialog_cancelled`
  and `dialog_returned`; form-payload dump drops from `month`
  (operator-noise). `fmt` import retained for `dupKey` sprintf;
  `debug` import retained for other handlers in the file.
  Regression net: `go build ./...`, `go build -tags debug ./...`,
  `go test ./... -short -count=1` (22 packages green),
  `go test -tags debug ./internal/debug/...` (2 packages green).

- Fixed the white-screen bug on initial load under blocked states
  (pre-mux window, setup-required, recovery, startupErr). The
  `blockIfFragment` helper returns 204 + `X-DixieData-Redirect`
  for htmx fragment requests; htmx does not auto-follow a 204 the
  way it follows a 3xx, so the empty `<body>` shell stayed empty
  and the user saw a white page. Added an `htmx:afterRequest`
  listener in `frontend/app.js` that reads the redirect header
  from any 204 response and calls `window.location.assign`. New
  `audit/smoke_fragment_redirect.mjs` regression test asserts the
  full client path: empty shell loads, htmx fires, server returns
  204 + header, browser navigates to `/setup`, body renders.
  Test would fail without the listener (final URL stays on the
  shell path, body length 0) and pin the server response shape
  (204 + header) so the contract doesn't drift. The pattern
  addresses the systemic gap that let 4 prior fragment-204 fixes
  (#209 pre-mux, #212 setup, #214 recovery + startupErr) ship
  without a working client bridge.

- Moved `jobs.jsonl` out of the data dir into the sibling
  `.dixiedata-logs/` directory. The on-disk jobs registry held
  an open append handle on `<dataDir>/jobs.jsonl` for the
  lifetime of the process; on Windows, that descendant handle
  blocked the atomic `os.Rename` that `replaceDataDir`
  performs at the start of a `.ddbak` restore, returning
  "Access is denied" after 5 retries. The fix follows the
  same convention as commit `b9a30cc` for `app.log.jsonl`:
  the data dir contains only the SQLite database and the
  image store. `migrateLegacyJobsLog` moves any existing
  legacy file to the new location on first startup (copy +
  remove fallback when the rename itself is denied). New
  tests pin the convention:
  - `TestOpenJobsRegistryLogPathOutsideDataDir` asserts
    the canonical log path is outside the data dir
  - `TestMigrateLegacyJobsLogRenamesOldFile` covers the
    upgrade path for existing installs
  - `TestMigrateLegacyJobsLogIsIdempotent` covers restart
    safety
  - `TestReplaceDataDir_.../open_file_outside_target_dir...`
    confirms `replaceDataDir` no longer touches the logs
    directory
  - `audit/smoke_jobs_log_location.mjs` confirms the
    deployed web binary writes the log to the right place.
  `docs/COMMON_BUGS.md` §4.17 codifies the convention:
  every new file written to the data dir must live under
  `appdata.LogsDir(dataDir)` instead.
- Lazy-load the print-records fragment to make /browse and
  /share nav GETs fast at scale. The print-config modal's
  filter panel + record picker were server-rendered on
  every /browse and /share GET, paying a listAllSoldiers()
  cost that was 80-95% of the request time (bench-verified:
  261ms at 5k records, 717ms+ at 10k). The modal now opens
  immediately on first paint; a new
  `GET /share/print-records-fragment` endpoint serves the
  filter + picker as a htmx-swapped fragment on modal open.
  The fragment is rendered by a new `PrintRecordsFragment`
  templ and cached client-side after the first load;
  archive-mutating actions (export template save/update/
  delete) invalidate the cache so the next open re-fetches.
  Bench: 5k records /browse GET drops from 261ms to 36ms
  (7x faster); 5k fragment GET stays under 250ms. New
  files:
  - `internal/templates/partials/print_records_fragment.templ`
  - `internal/appshell/print_records_fragment_handlers.go`
  - `internal/appshell/print_records_fragment_test.go`
  - `audit/smoke_browse_nav_speed.mjs`
  Threshold test updated: `TestHandleBrowseResponseUnderThreshold`
  asserts 100ms at 5k records (was 500ms at 1k). 3 new
  unit tests for the fragment endpoint (renders, empty
  archive, 405 contract). `handleShare` deliberately keeps
  listAllSoldiers() for the `CalendarDriftStatus` consumer
  (one-shot destination; cost is acceptable for the page's
  single visit per session).

- **Removed three fluff test files** (issue #318 Slice 0).
  `internal/routebuilder/routebuilder_test.go` (60+
  one-liner string-concat asserts), `internal/persondisplay/
  display_test.go` (2 trivial format asserts), and
  `internal/debug/trace/trace_test.go` (3 stdlib-semantics
  tests in a `//go:build debug` no-op package) — none caught
  real bugs per the test-strategy audit. The two useful
  edge cases from `routebuilder_test.go` (URL-escape + trim)
  folded into a single table-driven
  `internal/routebuilder/routebuilder_edge_test.go`.
  Net: -356 LOC of test code, +19 LOC of real coverage.
  Test file count: 172 → 170.

- **Skipped `internal/buildinfo` doc-audit tests in `-short`**
  (issue #318 Slice 0). The 3 tests
  (`TestEveryInternalPackageHasSynopsis`,
  `TestNoWrongStarterDocComments`,
  `TestPerPackageDocCoverageFloor`) shell out to `go doc`
  per-package across 150+ internal + pkg packages; each
  subprocess cold-starts at ~100ms, totaling 70s of
  wall-clock for the package. The audit catches missing
  `// Package foo` doc comments per the Go Doc audit
  Phase 1+2 — real value, but too heavy for every
  `make test` invocation. Now skipped in `-short` with a
  message pointing at the explicit invocation. `make test`
  drops from ~125s to ~62s on this repo. Audit still runs
  under `go test -count=1 ./internal/buildinfo/...` or any
  CI lane that drops `-short`.

- **CI race detector gate enabled** (issue #318 Slice 1).
  `.github/workflows/test.yml::Go test (short, race detector)`
  now runs `go test -race ./... -short -count=1` with
  `CGO_ENABLED=1`. Catches the concurrency bug class per
  `docs/COMMON_BUGS.md` §4.1 (race condition). Local
  `make test` is unchanged — the `-race` flag requires cgo,
  which the Windows dev env typically lacks. CI has gcc on
  `ubuntu-latest`. A race bug will surface as a PR CI failure
  rather than a local `make test` failure. The previous
  `TestMuxSelectEmitted` registry drift was already resolved
  in commit `c136789`; the failing-test concern from the
  audit is moot on this branch.

- **Goroutine leak detection via `goleak`** (issue #318 Slice 2).
  Added `go.uber.org/goleak v1.3.0` as a direct dep and a shared
  `internal/leaktest/leaktest.go` helper. `TestMain` in the root
  `main_test.go` plus `internal/jobs/zzz_goleak_test.go`,
  `internal/appshell/zzz_goleak_test.go`, and
  `internal/archive/zzz_goleak_test.go` install the gate.
  Catches the `docs/COMMON_BUGS.md` §4.1 class automatically
  — a test that starts a goroutine without cancelling its
  context fails the package suite with the leak's stack
  trace. Go's `TestMain` is per-package so root-only coverage
  wouldn't gate the packages where leaks actually live;
  per-package opt-in via `zzz_goleak_test.go` extends the
  gate to the highest-leverage surfaces (jobs, appshell,
  archive). Default matchers ignore net/http keepalive /
  readLoop / writeLoop + runtime pollWait so the signal is
  DixieData leaks, not stdlib noise.

- **Property tests in `internal/dates`** (issue #318 Slice 3).
  Added `pgregory.net/rapid v1.2.0` and
  `internal/dates/dates_property_test.go` with 7 properties:
  full-date roundtrip (Format → Parse → Format), year-only
  roundtrip, never-panic on arbitrary input, normalization
  idempotence, all-zero formatting sentinel, Display never
  panics, and ParseBirthInfo year extraction under randomised
  word boundaries. Catches the `docs/COMMON_BUGS.md` §9.2
  (normalization) class automatically. Local `make test`
  runs 20 iterations per property (rapid default under
  `-short`); CI workflow bumped to 500 (`-short` / 5 = 100
  iterations) so ≥100 random inputs land on every PR.
  Existing hand-written `dates_test.go` stays — properties
  complement, not replace, the table-driven coverage.

## v1.1.16 - Gold Master
