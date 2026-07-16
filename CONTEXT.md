# DixieData

DixieData is a local-first research archive for Civil War people and evidence. This glossary captures the domain language so future features, docs, and discussions use the same terms.

## Language

**Person Record**:
A primary archive entry for one person, whether that person is a soldier, wife, or widow.
_Avoid_: Record, soldier record, archive entry, profile

**Display ID**:
The canonical user-facing identifier for a Person Record.
_Avoid_: Record ID, person ID

**Local Archive**:
The live working collection of Person Records, Source Records, notes, images, and review state on one machine.
_Avoid_: Archive, database

**Shared Archive**:
A merge-oriented archive package exchanged between DixieData users.
_Avoid_: Archive, backup

**Backup Archive**:
A full replacement archive snapshot used for restore and safekeeping.
_Avoid_: Archive, export

**Restore Point**:
An automatic pre-update recovery bundle that preserves a recoverable Local Archive state and the previously safe app build for rollback.
_Avoid_: Temp backup, update backup

**Static Archive**:
A read-only browser-viewable archive export.
_Avoid_: Archive, website

**Source Record**:
An attached evidence item that documents or supports a Person Record, such as a pension or application record.
_Avoid_: Record, attachment

**Claim**:
An assertion about a person that is extracted from a Source Record.
_Avoid_: Fact, evidence

**Finding**:
A researcher-endorsed conclusion reached by weighing one or more Claims.
_Avoid_: Claim, fact

**Scratch Pad**:
An informal working note tied to a single Person Record.
_Avoid_: Research log, journal

**Research Log**:
A structured record of research activity, findings, or open questions that should remain intelligible over time.
_Avoid_: Scratch pad, notes blob

**Confederate Home Status**:
The archive field that records a person's relationship to a Confederate Home. The canonical no-status value is `N/A`.
_Avoid_: None, blank status

**Service Timeline**:
An evidence-backed chronological view of a Soldier's known life or service events.
_Avoid_: Notes, narrative

**Timeline Marker**:
A dated item shown on a Service Timeline.
_Avoid_: Claim, service event, timeline event

**Service Event**:
A Timeline Marker specifically about military service.
_Avoid_: Timeline event

**Research Collection**:
A user-curated grouping of archive material assembled inside a Local Archive for an ongoing research purpose.
_Avoid_: Pack, folder

**Research Pack**:
*Deprecated after slim (issue #455, slices 1 + 3). The same Top Units / Top Cemeteries / Related Person Records data lives on Insights.*

A prepared archive bundle organized around a defined scope such as a county or state.
_Avoid_: Collection, export

**Unit Membership**:
The factual claim that a Soldier served in a particular unit.
_Avoid_: Camaraderie

**Unit Camaraderie Graph**:
*Deprecated after slim (issue #455, slices 1 + 3). Use Insights drilldown scoped to unit instead.*

An inferred relationship network between Soldiers based on evidence such as shared units, time overlap, or other signals.
_Avoid_: Roster, unit membership

**Review Queue**:
The holding area for Person Records that need human attention before they should be treated as clean archive data.
_Avoid_: Audit, merge review

**Share Queue**:
The in-memory list of Person Records a researcher has staged for inclusion in a Shared Archive (.ddshare) before exporting. Distinct from the Review Queue, which tracks records needing human attention; the Share Queue is the explicit staging surface for a deliberate subset export. Persisted in the browser's localStorage so the queue survives navigation, reloads, and app restarts.
_Avoid_: share list, export set

**Duplicate Audit**:
An archive-wide scan that detects likely duplicate Person Records for human review.
_Avoid_: Review queue, merge review

**Merge Review**:
The workflow for resolving conflicts introduced by importing a Shared Archive into a Local Archive.
_Avoid_: Review queue, duplicate audit

**Local Record**:
The Person Record that already exists in the Local Archive during Merge Review.
_Avoid_: Existing record, current record

**Incoming Record**:
The Person Record arriving from a Shared Archive during Merge Review.
_Avoid_: Shared record, new record

**Soldier**:
A Person Record subtype for the servicemember being researched.
_Avoid_: Record, main record

**Tag**:
A user-defined free-text label applied to a Person Record for ad-hoc grouping. Common patterns include a FindAGrave "virtual cemetery" or a research-project scope. A Tag has zero or more Person Records; a Person Record has zero or more Tags. Tag names are case-insensitive and unique across the Local Archive; display preserves the original casing.
_Avoid_: label, category, virtual cemetery (when used generically)

**Spouse Record**:
A Person Record for the spouse linked to a Soldier, regardless of whether her subtype is Wife or Widow.
_Avoid_: Wife record, widow record, dependent record

**Wife**:
A Spouse Record subtype used when the archive should describe the person as a wife rather than a widow.
_Avoid_: Spouse

**Widow**:
A Spouse Record subtype used when the archive should describe the person as a widow rather than a wife.
_Avoid_: Spouse

**Event Record**:
A primary archive entry for a dated event in Civil War history (a Battle, a Campaign, a Death, a Marriage, a Hospital stay, etc.). Has a free-text `kind` (no enum), optional begin and end dates in MM/DD/YYYY canonical-date form, an optional long-form Description with a per-PDF excerpt override, and zero or more attached images (reusing the `images` table). Carries the same paper trail as a Person Record (Source Records, Claims, Findings, Scratch Pad, Research Log, Tags, Review Queue). Linked to one or more Person Records via the `event_person_links` many-to-many junction. Lives in the same `soldiers` table as every other Person Record subtype; the column `entry_type = 'event'` distinguishes it. Display ID namespace: `EVT-NNNNN`.
_Avoid_: timeline event, archive event, cross-record link, historical event (too generic)

**Linked Person**:
A Person Record subtype for a person who is significant to the research but is not a Soldier, a Wife, or a Widow. Common cases: a Brother, Daughter, Executor, Witness, attending physician, or civilian contact mentioned in the archive. Lives in the same `soldiers` table as every other Person Record subtype; the column `entry_type = 'linked_person'` distinguishes it. Carries a free-text `relationship_label` (no enum — researchers name the relationship as it appears in the source) and a `spouse_soldier_id` FK that points at the Soldier Person Record the linked person is associated with (the semantics mirror Spouse Record's link to its Soldier). Display ID namespace: shares the Person Record namespace — `DXD-NNNNN`. See issue #491 for the Calendar / /inventory surface that exposes this subtype as its own rollup.
_Avoid_: contact (too generic), significant other (collides with Spouse Record), relative (too narrow)

**Article**:
A primary archive entry for a long-form, markdown-bodied essay with inline Person Record references (`[Private John Doe](#person/D-00123)`). Lives in its own `articles` table — NOT a Person Record subtype, NOT an Event Record sibling in the `soldiers` table. Articles are a parallel primary entity (sibling to Person Records + Event Records at the archive-entry level) because the authoring unit (the whole essay) does not match the per-row unit Person Records use (a single person). Articles have a title (required), an optional subtitle, a markdown body, and zero or more Person Record refs via the `article_refs` junction. They can be snapshotted ("Save copy" → read-only sibling row keyed by `snapshot_of_id`) for user-managed Revisions. Display ID namespace: `ART-NNNNN`. See issue #321 for the locked decisions + the slice plan.
_Avoid_: note (too small), essay (too literary), document (collides with Source Record), blog post, post

**Article Reference**:
A per-row link from an Article to a Person Record, stored in the `article_refs` junction table. Carries the article id + sync_id (distributed-merge mirrors), the person record id + sync_id + a denormalized display_id cache (so the Cite-in reverse-lookup + the PDF / Static-HTML resolver can render without a join), and a `position` column reserved for a future reorder UI (mirroring #368's source-record reorder). Authored via the in-body inline syntax `[Text](#person/D-00123)`; resolved strictly via the Display ID — an unresolved token renders as `⚠ [Unknown: D-00123]` in the PDF / Static-HTML output (fail-loud; no silent fallback per locked decision #6).
_Avoid_: citation (too literary), bookmark, cross-reference

**Article Snapshot**:
A read-only sibling row of an Article, created via the "Save copy" button (locked decision #11). Stored in the same `articles` table with `is_snapshot = 1` and `snapshot_of_id = <original-row-id>`. The read path filters snapshots (`is_snapshot = 0` only) so the live detail page is unaffected; slice 2.5 introduces the Revisions tab to list + restore + delete snapshots. Snapshot-of-snapshot is rejected; edit on a snapshot row is rejected — the original is the only writable instance per artifact.
_Avoid_: revision (implies auto-versioned), backup (implies filesystem), diff (implies visual diff between revisions)

## Relationships

- A **Person Record** may have zero or more **Source Records**
- A **Person Record** has exactly one **Display ID**
- A **Source Record** belongs to exactly one **Person Record**
- A **Source Record** may support one or more **Claims**
- One or more **Claims** may support a **Finding**
- A **Person Record** may have a **Scratch Pad**
- A **Service Timeline** is derived from **Findings** about a **Soldier**
- A **Service Timeline** contains one or more **Timeline Markers**
- A **Service Event** is a kind of **Timeline Marker**
- A **Research Collection** groups related archive material inside a **Local Archive**
- A **Research Pack** packages archive material for a defined scope
- A **Unit Membership** may contribute evidence to a **Unit Camaraderie Graph**
- A **Local Archive** may be exported as a **Shared Archive**, **Backup Archive**, or **Static Archive**
- A **Restore Point** preserves a pre-update recovery state for a **Local Archive**
- A **Duplicate Audit** may place a **Person Record** into the **Review Queue**
- A **Merge Review** resolves conflicts created by importing a **Shared Archive** into a **Local Archive**
- A **Merge Review** compares a **Local Record** and an **Incoming Record**
- A **Soldier** is a kind of **Person Record**
- A **Spouse Record** is a kind of **Person Record**
- A **Spouse Record** may be linked to exactly one **Soldier**
- A **Wife** and a **Widow** are subtypes of **Spouse Record**
- An **Event Record** is a kind of **Person Record**
- An **Event Record** may link to one or more **Person Records**
- An **Article** may cite zero or more **Person Records** via **Article References**
- An **Article Snapshot** is derived from exactly one **Article** (or **Article Snapshot**)
- A **Person Record** may appear in zero or more **Articles** via **Article References**
- A **Person Record** may be linked to one or more **Event Records**
- A **Person Record** may have zero or more **Tags**
- A **Tag** may be applied to zero or more **Person Records**

## Example dialogue

> **Dev:** "This pension application came in from a shared archive — should it become a new **Person Record**?"
> **Domain expert:** "No. It is a **Source Record** unless it represents a different person we do not already track."

## Historical Artifact

A doc, audit result, or rendering review retained for traceability
but no longer loaded by default. Lives under `docs/historical/`
with the retention rule documented in that directory's README. An
agent does not load a Historical Artifact unless the issue, PR, or
ADR explicitly names it. Resolved audits, superseded handoffs, and
per-surface iteration rounds past the latest three are Historical
Artifacts by definition.
_Avoid_: legacy doc, old doc, archive (use one of the archive terms
already defined)

## Flagged ambiguities

- "record" was used to mean both **Person Record** and **Source Record** — resolved: use **Person Record** for the main person entry and **Source Record** for attached evidence items.
- "archive" was used for the live dataset and multiple export forms — resolved: use **Local Archive**, **Shared Archive**, **Backup Archive**, and **Static Archive** for those distinct concepts.
- "ID" and "record ID" risked reintroducing overloaded language — resolved: use **Display ID** for the user-facing identifier of a **Person Record**.
- "facts" and source-derived assertions needed separation — resolved: use **Claim** for assertions extracted from a **Source Record**.
- "claim" still needed a stronger research conclusion term — resolved: use **Finding** for a researcher-endorsed conclusion supported by one or more **Claims**.
- "timeline evidence" needed a confidence boundary — resolved: the visible **Service Timeline** is derived from **Findings**, with **Claims** as support beneath it.
- "event" needed a timeline-specific hierarchy — resolved: use **Timeline Marker** for any dated timeline item and **Service Event** for the military-service subset. The dated-event-as-archive-entry concept is **Event Record** (a Person Record subtype), distinct from the on-timeline rendering.
- "notes" risked covering both informal and structured research writing — resolved: use **Scratch Pad** for informal per-record notes and **Research Log** for structured research activity.
- "collection" and "pack" risked collapsing into the same idea — resolved: use **Research Collection** for in-archive grouping and **Research Pack** for a prepared scoped bundle.
- "camaraderie" risked meaning mere unit assignment — resolved: use **Unit Membership** for factual service in a unit and **Unit Camaraderie Graph** for inferred soldier-to-soldier relationships.
- "merge conflict sides" risked inconsistent naming — resolved: use **Local Record** and **Incoming Record** during **Merge Review**.
- "timeline" could have meant prose or chronology — resolved: use **Service Timeline** for the derived evidence-backed chronology of a Soldier.
- "review" was used for queueing, duplicate detection, and merge conflict handling — resolved: use **Review Queue**, **Duplicate Audit**, and **Merge Review** for those separate workflows.
- "soldier" was used both for one subtype and for the whole main table — resolved: use **Soldier** only for that subtype and **Person Record** for the umbrella.
- "spouse", "wife", and "widow" were used interchangeably — resolved: use **Spouse Record** as the umbrella term, with **Wife** and **Widow** as specific subtypes when the distinction matters.
- "virtual cemetery" was used generically for any Person Record grouping — resolved: use **Tag** as the generic term; reserve "virtual cemetery" for a specific FindAGrave pattern that the user explicitly names.
- "old doc", "legacy doc", and "archived doc" risked ambiguity with the archive terms above — resolved: use **Historical Artifact** for retained-for-traceability docs that are not loaded by default.
- "person-scoped FK columns" risked carrying the soldier subtype's name into Event Record rows — resolved: the v60 schema renames `soldier_id` to `person_record_id` in `records`, `images`, `scratchpad_cache`, `research_tasks`, and the merge-review / duplicate-audit side columns. The self-referential `spouse_soldier_id` (Soldier-to-Spouse) is intentionally left as-is because it is a Soldier-to-Soldier relationship, not a Person Record FK.

## Adding features

The canonical procedure for adding a new feature to DixieData lives
in [`docs/agents/feature-protocol.md`](docs/agents/feature-protocol.md).
The protocol covers the 3-tier commit rule (issue #183 worked
example), the deep-module discipline (define the facade before the
internals, service is the seam, two-adapter rule), the 7-phase
pipeline phasing (lightweight issue body by default;
`docs/RESEARCH.md` → `docs/PRD.md` → `docs/TASKS.csv` only for 6+
file cross-layer features), and the per-layer "when to load what"
table.

Read it end-to-end before any feature work. The protocol references
RPCI; you don't need to re-read `docs/agents/rpci.md` separately.

## Laws (non-negotiable)

These are not style preferences. Each one was earned by a real bug that
crashed the app, lost data, or confused a researcher. Treat any code that
violates a law as a bug that must be fixed before the change can ship.

### Released code lands on `stable`; `main` is frozen at `56e31f0`

The three-branch model (per ADR 0009) is a law, not a convention:

- **`dev`** — integration. Direct commits are the default flow
  for agents and humans (per AGENTS.md §Branch policy).
- **`stable`** — released-code home. The promotion chain
  documented in ADR 0008 lands here. Every release that reaches
  users is tagged on `stable`.
- **`main`** — frozen legacy production record. Sits at commit
  `56e31f0` (HEAD `main` at the moment this law was written, 2026-07-03)
  and accepts no new commits, ever. Preserved as the audit
  anchor for "what users had on 2026-07-03."

Treat any code, script, or CI workflow that pushes to `main` as
a bug. The GitHub default branch stays on `main` until the first
release has shipped from `stable` and proven itself; the flip
itself is a one-line repo-settings change tracked separately.

### Exported Go identifiers carry doc comments

Every exported identifier in a DixieData Go package — `func`,
`type`, `var`, `const`, including methods on exported types —
carries a doc comment. The doc comment:

- Starts with the identifier name (the `go doc` synopsis line is
  the first sentence).
- Explains the contract, not the implementation. What does the
  caller need to know? What are the preconditions? What does it
  return on success and on the common failure modes?
- Lives immediately above the declaration, with no blank line
  between the comment and the identifier.

Every Go package has a `// Package <name> <one-sentence purpose>`
synopsis at the top of at least one of its files. Multi-paragraph
package docs are fine; the synopsis is what `go doc <pkg>` shows
in its first paragraph.

**Floor (regression gate, enforced by CI):** every Go package
under `internal/` and `pkg/` with ≥5 exported identifiers must
have ≥70% identifier-level doc-comment coverage. Packages below
the floor fail `go test ./internal/buildinfo/ -run
TestPerPackageDocCoverageFloor`. The 70% floor is a regression
gate, not a target; the working rule is **aim for 100% on every
new PR**. The CI test exists so a future commit that strips docs
in bulk gets caught; it is not a license to land a 70% patch.

Exemptions (skipped by the test): `cmd/*` (unexported `main`
packages), templ-generated files (churn that disappears on the
next `make tpl`), and build-tag-gated packages whose `go doc`
output cannot be parsed without the matching tag.

The audit script (`.scratch/audit/go_doc_audit.py`) is the
source of truth for the overall coverage metric. Run it before
opening a PR that adds new exported surface; if it drops the
overall metric, the PR is expected to add the docs inline, not
push the work to a follow-up.

### Every native dialog call is guarded against re-entry

Wails v2.12.0 on Windows runs every native `SaveFileDialog` and
`OpenFileDialog` on the UI thread. If two of them land on the message
loop at the same time, WebView2 loses focus while the Wails
`onFocus` handler calls `Chromium.Focus()` → `MoveFocus()` and the
frontend process dies with `Chrome_WidgetWin_0. Error = 1412`
(wailsapp/wails#2807). The double-click is enough.

The contract:

- Every HTTP handler that calls `a.SaveFileDialog` /
  `a.OpenFileDialog` / `a.OpenDirectoryDialog` /
  `a.OpenMultipleFilesDialog` MUST guard the call with
  `a.inFlight.LoadOrStore(dupKey, struct{}{})` and `defer a.inFlight.Delete(dupKey)`.
- Every Wails binding that opens a native dialog from JS MUST go
  through a Go helper that carries the same guard.
- The guard key must be unique enough to distinguish concurrent
  exports of different kinds but stable enough to collapse
  duplicate clicks on the same button. Use
  `kind|filename|filters` or the equivalent.
- The slot is released AFTER the dialog returns (`defer Delete`),
  never before. Releasing early re-opens the race.

Existing helpers (use them; do not invent new ones):

- `(*App).guardedSaveFileDialog(kind, opts)` in
  `internal/appshell/exports_handlers.go` — covers all export
  handlers in that file.
- Inline `a.inFlight.LoadOrStore` + `defer a.inFlight.Delete` in
  `internal/appshell/app.go` — pattern from `handleCalendarPDF`
  for the soldier-record / image / calendar handlers.

New native dialog calls added in the future MUST either route
through one of those helpers or follow the inline pattern with a
duplicate test added to `save_dialog_guard_test.go`.

Full bug history, reproduction steps, and references:
[`docs/agents/dialog-guard.md`](docs/agents/dialog-guard.md).

### Do not regress to native `<dialog>` for in-app modals

The native `<dialog>` element was tried for the three in-app
modals (feedback, print-config, google-prefs) in issue #117 and
shipped a transient WebView2 focus-event reentry that compounded
the dialog-race crash above. The modals were reverted to
`<div role="dialog" aria-modal="true">` overlays with manual
focus trap and ESC close handlers in `frontend/app.js`
(`showOverlayModal` / `overlayModalKeydown`). Keep it that way
until Wails fixes the upstream interaction. Tests in
`internal/templates/{layout,share}_test.go` lock in the
overlay shape.

### No feature PR ships a backend surface without a UI apply-site

The 2026-06 "shipped but invisible" sweep (issue #257) surfaced
four features that landed backend + service + handler + routes
with no UI to invoke them. Each was functionally complete,
fully tested, and reachable only by `curl`. Users had no way to
discover them; the discoverable entry points were buried in
unrelated screens. The pattern drifted across issues #183 (tags),
#184 (templates "Show details" toggle), #185 (live preview
StaleSummary), #186 (PATCH `/export/templates/{id}`), and
#187 (search-bar history).

The contract:

- Every feature PR that adds a backend surface (new HTTP route,
  new service method, new CLI subcommand) MUST land at least one
  matching UI apply-site in the same PR. A "UI apply-site" is a
  templ button / form / link / CLI command / job that a user (or
  a smoke probe driving the live UI) can click to exercise the
  surface.
- The apply-sites are a **checklist** in the feature issue body
  AND the PR description, not a prose sentence. Each box must be
  ticked as the matching commit lands. Backend-only commits with
  no matching UI apply-site in the same PR are not mergeable.
- The only exception is a backend surface whose apply-site is
  already wired (e.g. extending an existing route). Mark it
  "extends existing apply-site" in the issue body; the reviewer
  verifies the surface is actually reachable from the UI before
  approving.
- Tracked follow-up PRs are acceptable for the SECOND-and-later
  apply-sites (e.g. add the Browse row chip after the Person
  Record detail page picker lands). Each follow-up PR carries
  its own apply-sites checklist; the original issue stays open
  until every box is checked.

`audit/discover_orphan_handlers.mjs` runs in CI and greps the
registered routes against the templ invokers. Handlers with no
invoker are flagged. This is the automated tripwire that catches
the pattern before a user does.

### Fresh debug build = `make freshness`

A debug build is not "DixieData.exe compiles". It is "every
debug subtool builds AND every CLI subcommand still parses AND
every subtool's sanity probe passes". The contract:

- `make debug` builds the Wails binary. `make freshness` builds
  DixieData + dixiedata-web + seed-data + gold-master +
  dixiedata-tune, then probes each (`--help` / `--smoke` /
  `--self-test`), then runs `dixiedata debug cli-coverage` to
  assert every documented CLI subcommand is implemented.
- A subtool that rots (flag renamed, dispatcher updated but
  doc not, subcommand removed but `cli-plan.md` not updated)
  is the same class of bug as the dialog-guard crash: silent
  drift that the user hits first. The freshness check is the
  tripwire.
- The `make freshness` target is required by `make release-
  pipeline` (the ordered release gate chain). PRs that touch
  `main.go`, `internal/appshell/cli_*.go`, `docs/agents/cli-
  plan.md`, `tools/tune/`, or any `cmd/*/main.go` MUST land
  with `make freshness` passing.

See [`docs/agents/build-protocol.md`](docs/agents/build-protocol.md)
§1 + §2 for the canonical reference.

### PRs that touch schema must bump

The schema version (`CurrentSchemaVersion` in
`internal/versioninfo/versioninfo.go`) is the single source
of truth for the local update feature. Every schema-touching
PR MUST bump in the same PR — the migration has to ship with
the bump so users on older DBs can apply the update.

The contract:

- A PR "touches the schema" if any commit subject matches
  `feat(db):` / `feat(schema):` / `fix(db):` AND the change
  actually affects schema shape (new table, new column,
  drop, rename, new index, modified JSON shape).
- `feat(db):` commits that don't change schema shape
  (performance-only index, EXPLAIN-friendly rewrite) use the
  `chore: skip-schema-bump` hatch — one extra commit with
  that subject and a reason in the body.
- CI runs the schema-touching detector on every PR. If the
  PR has a touching commit AND `CurrentSchemaVersion` is
  unchanged from the base branch, the check fails with a
  drift message. The merge is blocked.
- The bump is gated by `scripts/bump-version.ps1`: refuses
  to advance by more than `+1` (use `-Force` for jumps),
  requires `docs/migrations/v{N+1}.md` with at least one
  bullet, refuses uncommitted changes to `versioninfo.go`.

The 2026-06 sweep that surfaced four "shipped but invisible"
features (issue #257) also surfaced the parallel risk: a
schema change that ships without a bump leaves users on older
DBs unable to update. Don't repeat.

### Release counter N ≠ schema version

The release line lives at three counters in
`internal/versioninfo/versioninfo.go` (issue #266):

- **`CurrentSchemaVersion`** — SQLite `user_version`; the data plane.
- **`CurrentUpdateFlowVersion`** (U) — the in-place update flow's
  shape gate. U mismatch in either direction forces a
  reinstall. Default U=1 covers every legacy v1.2.N release.
- **`CurrentAppVersionInt`** (N) — the release counter. Every
  release bumps N; bug-fix-only releases bump N without a
  schema change.

The composite app version is `v{MAJOR}.{U}.{N}`. N is the
**release counter**, not the schema version — bug-fix-only
releases bump N without touching the schema, and the inverse
is possible but rare (a schema bump that doesn't roll a new
release). Treat them as independent axes in any script or doc
that wants to compare versions.

`bump-version.ps1` exposes three switches — `-BumpSchema`,
`-BumpUpdateFlow`, `-BumpRelease` — one per counter. They
are mutually exclusive. The default (no switch) is
`-BumpSchema` to match historical behavior.

See [`docs/RELEASING.md`](docs/RELEASING.md) §Versioning rules
for the canonical contract.

See [`docs/agents/build-protocol.md`](docs/agents/build-protocol.md)
§4 for the canonical reference.

### No slice ships until a live probe confirms the fix on the real archive

A "ship" commit is not the same as a "verified" commit. This law
exists because the 2026-07 #607 sequence demonstrated the failure
mode end-to-end:

- A bug was filed (Preview button does nothing on /articles/{id}/edit).
- An agent shipped two patches citing it as fixed (commits in the
  #607 sequence). The user's `markdown.png` screenshot + a live
  Playwright probe proved neither fix addressed the actual cause.
- The real bug was a missing chi mux route for `/_lib/*` (filed
  separately as #609).

Slice work is complete when **a live probe** (Playwright against
`dixiedata-web` OR a Wails-binary smoke against the real
`.dixiedata` archive at the repo root — see `Data directory`
below) shows:

- `modalOpen: true` (or the equivalent end-state assertion for the
  report's "expected behavior") **after** the user's reported
  trigger.
- The pre-fix failing assertion **flips to PASS** in the audit
  probe (no ship-then-claim; ship-then-verify is the rule).
- The git comment + the CHANGELOG bullet cite the live probe by
  file path (`audit/_probe_*.mjs`) so a future audit can replay
  the verification.

If a slice's fix is static-only (e.g. a CSS class swap, a string
swap, a templator rename), the live probe is a single `curl
http://127.0.0.1:8765/path | grep "<new-content>"` check. If a
slice requires the Wails binary (modal opens with WebView2
quirks), the probe runs against the binary, not just the web
harness. The `ready-for-agent` label is not a closure signal; the
`gh issue close` call happens only after the probe flips.

### Data directory: `.dixiedata` lives at the repo root, always

The canonical Local Archive the Wails desktop binary reads/writes
is `<repo-root>/.dixiedata/` (per `internal/appdata/appdata.go`
§`DefaultDir` — the resolution walks up from `build/bin/` looking
for `wails.json`). `dixiedata-web` honors the same default; passing
`-scratch-dir` overrides it for the audit harness.

This law exists because the `~/.dixiedata/` (home-dir) convention
was the legacy v1.1 default on some install paths, and the agent
sessons that touched both `~/.dixiedata/` + `<repo>/.dixiedata/`
ended up reporting "I imported 665 soldiers" against one while
the other stayed unchanged — same import code path, two file
systems, two outcomes. The audit-harness smoke probes crashed
in the home-dir because the post-import `import_batches` was
written to the repo root.

Therefore:

- **Never assume a data directory.** When the bug, plan, or issue
  says "the live archive", it means `<repo-root>/.dixiedata/`.
  When the user passes a custom path via `-data-dir` / `-scratch-dir`,
  pin that path explicitly in the issue body.
- **Never operate against `~/.dixiedata/` without a confirmation
  step.** The home-dir archive is usually a v0 + April-2026 orphan
  (per issue #608's discovery). If a probe finds data in `~/`, it
  is silently testing an unmigrated empty archive, not the live
  one.
- **Audit scripts that probe the live app** resolve the data dir
  via `appdata.DefaultDir()` (the same call the app makes), not
  by constructing paths from `$HOME` or from environment defaults
  that may differ across the Wails binary, the web harness, and
  the CLI runner.

A future slice that wants to delete or relocate the Wails binary's
output directory (the repo-root `.dixiedata/`) must update
`internal/appdata/appdata.go` + the `Data directory` law here +
the `docs/agents/manual-audit-playbook.md` "where the archive
lives" section. Three changes; one ADR-style commit.

