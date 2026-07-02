# Build / Release / Schema / Update-in-Place Protocol

The canonical procedure for building, releasing, bumping the
schema, and gating update-in-place safety in DixieData. Pairs
with [`feature-protocol.md`](feature-protocol.md) (which covers
the change itself) and [`RELEASING.md`](../RELEASING.md) (which
covers the release mechanics). This doc owns the build/release
process AND the safety contracts that protect users on in-place
updates.

## Table of contents

1. [Makefile hygiene](#1-makefile-hygiene)
2. [CLI freshness](#2-cli-freshness)
3. [Release pipeline](#3-release-pipeline)
4. [Schema bumps](#4-schema-bumps)
5. [Update-in-place safety](#5-update-in-place-safety)

---

## 1. Makefile hygiene

The `Makefile` is the entry point for every build, test, render,
bump, and release operation. Five targets cover the daily work;
two new targets (`make freshness`, `make release-pipeline`) cover
the regressions that bit us.

### Daily targets

| Target | What it does | When |
|---|---|---|
| `make help` | List every target + one-line summary | First run of the day |
| `make debug` | Build DixieData (Wails) + dixiedata-web + seed-data + gold-master + dixiedata-tune | Iterating on UI or backend |
| `make test` | `go test ./... -short -count=1` | Before any commit |
| `make run` | Build + launch debug binary | Smoke-checking a change |
| `make clean` | Remove generated artifacts | Switching branches that touch `templ` or `frontend/` |

### Freshness targets

| Target | What it does | When |
|---|---|---|
| `make freshness` | Build every debug subtool + run each one's sanity probe + walk every registered CLI subcommand | Before any release; on first CI run after a subtool change |
| `make release-pipeline` | The ordered release chain (test → tpl → css → bump-verify → debug → freshness → audit → archive → release-github), halt-on-first-failure | When cutting a release |

### Render / iteration targets

| Target | What it does |
|---|---|
| `make tune` | Run dixiedata-tune against live archive |
| `make render-round` | Render all PDF export surfaces for one iteration round |
| `make render-round-ONE SURFACE=...` | Render one surface |
| `make update-snapshots-ONE SURFACE=...` | Regen the byte-stable snapshot fixture for one surface |
| `make tune-snapshots` | Regen all 22 snapshot fixtures at once |

### Why these exist

The debug chain (`make debug → web + seed + gold + tune-bin`)
catches the case where the user opens the app, hits a button,
and the audit smoke fails because `dixiedata-web.exe` isn't
in `build/bin/`. One-shot `make debug` produces everything a
debug session needs.

The release chain (`make release-pipeline`) catches the case
where a release ships with a stale `dixiedata-web.exe` from
before today's commit, or with a CLI subcommand that no longer
parses, or with a typst fixture that drifted. Each gate is a
pass-or-stop decision.

---

## 2. CLI freshness

The `dixiedata` CLI has 7 phases of subcommands, documented in
[`docs/agents/cli-plan.md`](cli-plan.md). Drift between the
docs and the code is the failure mode: a subcommand gets
removed, the doc stays, the user runs `dixiedata <cmd>` and
gets "unknown subcommand".

### The freshness check

`make freshness` runs three checks:

1. **Build check.** `go build -tags debug -o build/bin/dixiedata.exe .`
   must succeed. Catches broken CLI parser / dispatch.
2. **Dispatch check.** `dixiedata debug cli-coverage` walks the
   parser's switch table in `main.go` and the documented
   subcommand list in `cli-plan.md`. Emits:
   - **Documented, not implemented** (drift: remove from docs)
   - **Implemented, not documented** (drift: add to docs)
   - **Both** (clean)
3. **Sanity probe.** For every documented subcommand, run
   `<subcommand> --help` (or `--version` where appropriate)
   and assert exit 0 + non-empty output. Catches dead
   subcommands that parse but no-op.

### `dixiedata debug cli-coverage` output

```
dixiedata debug cli-coverage
===========================
Documented subcommands (28): --smoke, --version, doctor,
  export {pdf,jpg,json,csv,ical,static-archive,backup},
  import {backup,shared-archive,images,memorial-json},
  list soldiers, search, show soldier, migrate {status,up},
  backup {list,prune}, restore point {list,create,apply},
  logs {path,tail}, config {show,set}, debug {dump,
  hx-invariants,browser-tree,request,cli-coverage}
Implemented subcommands (28): <matched>
Documented, not implemented: 0
Implemented, not documented: 0
coverage: 28/28 (100%)
exit 0
```

### When to run

- Before every release (`make release-pipeline` runs it).
- On first CI run after any commit that touches `main.go`,
  `internal/appshell/cli_*.go`, or `docs/agents/cli-plan.md`.
- Manually when adding/removing a subcommand.

### What it does NOT check

- Argument validation. A subcommand that requires an arg may
  fail at runtime; `--help` is the smoke.
- Subcommand interaction. Cross-subcommand effects (e.g. a
  backup import then a shared-archive import) are integration
  tests' job, not this check's.
- Performance. A subcommand that hangs is a timeout test's job.

---

## 3. Release pipeline

The release is the user-facing contract: a tagged GitHub release
that the in-place updater downloads and applies. Every gate
between commit and release must pass.

### Ordered gates

```
make release-pipeline
├── 1. make test          (Go test -short)
├── 2. make tpl           (regenerate templ files)
├── 3. make css           (rebuild Tailwind bundle)
├── 4. make bump -VerifyOnly  (schema discipline intact)
├── 5. make debug         (build DixieData + 4 subtools)
├── 6. make freshness     (CLI coverage + subtool probes)
├── 7. make audit         (visual sweep)
├── 8. make archive       (build + zip release/)
└── 9. make release-github  (tag + push + draft gh release)
```

Each gate must pass before the next runs. The wrapper halts on
the first non-zero exit.

### Gate pass criteria

| # | Gate | Pass criterion |
|---|---|---|
| 1 | `make test` | All tests pass under `-short -count=1`. Both `//go:build debug` and `//go:build !debug` trace harness variants pass. |
| 2 | `make tpl` | Templ files regenerate cleanly. No diff against the working tree (catches stale generated files). |
| 3 | `make css` | Tailwind builds without warnings. `frontend/app.css` regenerated. |
| 4 | `make bump -VerifyOnly` | `CurrentSchemaVersion` matches all doc references (user-manual, implementation-and-features, ai-handoff). `docs/migrations/v{N}.md` exists. CHANGELOG has section. |
| 5 | `make debug` | Wails build succeeds. All 4 subtools build. No `//go:build debug` file rotted the no-op stub. |
| 6 | `make freshness` | All subtool probes pass. `cli-coverage` shows 100% documented/impl match. |
| 7 | `make audit` | Visual sweep clean. No new findings. (Manual gate — passes when the user runs it and says so.) |
| 8 | `make archive` | `release/DixieData-release-v1.2.{N}.zip` exists. Includes pdfium.dll + typst. |
| 9 | `make release-github` | Tag pushed, draft release created. Per `release-github.ps1` 5 safety gates. |

### Manual override

For emergencies (CI outage, gh unavailable, network issues),
each gate can be run individually. Document the skip + reason
in the PR description.

---

## 4. Schema bumps

`CurrentSchemaVersion` in `internal/versioninfo/versioninfo.go`
is the single source of truth. App version `v1.2.N` is computed
from it. The local update feature applies migrations on top of
the existing `.dixiedata` database — every bump must be paired
with a migration that runs in `applySchema`.

### When to bump

**Rule:** every PR that touches the schema MUST bump in the same
PR, OR follow up with a separate bump PR before the first
reaches `main`.

A PR "touches the schema" if any of:

- Adds, removes, or renames a table (`CREATE TABLE`, `DROP
  TABLE`, `ALTER TABLE ... RENAME`).
- Adds, removes, or renames a column (`ALTER TABLE ... ADD
  COLUMN`, `DROP COLUMN`).
- Adds or removes an index (`CREATE INDEX`, `DROP INDEX`).
- Adds a new `PRAGMA user_version = N` constant in
  `internal/db/schema.go`.
- Modifies a stored JSON shape (e.g. `soldier_ids_json` in
  `share_queue_presets`) — the parser must handle both old
  and new shape for at least one schema version.

### The bump PR

A bump PR contains:

1. `feat(db):` or `feat(schema):` commit(s) — the schema change
   itself, with `applySchema` updated to handle the new shape.
2. `docs(migrations/v{N+1}.md)` — human-readable note explaining
   the schema change for reviewers and the update flow.
3. `internal/versioninfo/versioninfo.go` — `CurrentSchemaVersion`
   bumped from `N` to `N+1`.
4. `CHANGELOG.md` — section header `## [v1.2.{N+1}]` with the
   schema-change bullet.
5. `docs/user-manual.md`, `docs/implementation-and-features.md`,
   `docs/ai-handoff.md` — reference the new app version (the
   `bump -VerifyOnly` check enforces this).

The bump PR is one logical commit (`feat(schema): bump to v{N+1}`)
or two commits (the schema change + the bump itself). Either
shape is acceptable as long as the schema change lands BEFORE the
bump in the merge order — otherwise the migration test runs
against the wrong version.

### The detector

CI runs `scripts/bump-version.ps1 -DetectDrift` on every PR. If
the PR has a commit subject matching `feat(db):` / `feat(schema):`
/ `fix(db):` and `CurrentSchemaVersion` is unchanged from the
base branch, the check fails with:

```
DRIFT: PR #N touches schema but did not bump CurrentSchemaVersion.
Either add the bump in this PR, or add a commit with subject
"chore: skip-schema-bump" + reason in the body.
```

The `chore: skip-schema-bump` escape hatch covers the rare case
where a PR adds a migration but doesn't bump (e.g. a new index
on an existing column for performance, no schema-shape change).
The reason goes in the commit body and is the human-readable
record of why.

### The bump script

`scripts/bump-version.ps1` enforces:

- `+1` only (no `-Force` = no jumps).
- `docs/migrations/v{N+1}.md` exists with at least one bullet.
- No uncommitted changes to `versioninfo.go`.
- Working tree otherwise clean (the `-VerifyOnly` mode skips
  this; the actual bump requires it).

### When NOT to bump

- A pure Go refactor (handler rename, internal restructure)
  with no schema shape change. The schema-touching detector
  doesn't fire on these.
- A frontend-only change (templ, JS, CSS). No schema touched.
- A new `feat:` that adds a new service in Go but stores its
  data in existing tables. No schema change.

---

## 5. Update-in-place safety

The local update feature downloads a tagged release and applies
it to the existing `.dixiedata` database. **Anything that ships
on `main` reaches users via in-place update.** The safety gate
prevents destructive changes from reaching `main` without an
explicit acknowledgment.

### Version shape (note for the future)

Today the app version string is `v1.2.{N}` where `N` is the
schema version. This conflates the schema with the update-flow
shape. Issue #266 proposes splitting that into
`v{MAJOR}.{U}.{N}` where `U` is a separate gate for the
in-place update flow itself (restore-point contract,
eligibility check, migration runner mechanics). When that lands,
this section will need an update to define what U-mismatch
means for the safety gate (likely: U-higher = incompatible
flow, refuse in-place, require re-install). For now, the
`safe-for-in-place` label is the manual acknowledgment that
the change is safe given the current single-version-string
shape.

### What makes a change "safe for in-place update"

A change is safe for in-place update if:

1. **No destructive schema migrations.** The `applySchema`
   block in `internal/db/schema.go` runs forward-only; any
   `DROP TABLE`, `DROP COLUMN`, `ALTER TABLE ... RENAME` without
   a backstop, or removal of an `IF EXISTS` guard, breaks users
   on older DBs.
2. **No handler signature changes that break existing
   routes.** Renaming a route or changing a route's HTTP
   method without keeping a backward-compat shim breaks the
   in-app updater flow.
3. **No auto-destructive flows on startup.** No first-launch
   trigger that deletes records, drops tables, or replaces
   the data dir.
4. **No removal of safety gates.** Removing `updateEligibility`'s
   `IsDevelopmentBuild` check, removing the `inFlight` guard
   on native dialogs, removing the dialog-guard law from
   `CONTEXT.md` — these are silent regressions.

### The label protocol

Every PR to `dev` or `main` carries one of:

- `safe-for-in-place` — the change has been reviewed against the
  four rules above; safe to ship via in-place update.
- `unsafe-for-in-place` — the change is intentional and known
  destructive; ships via full re-install + restore-point safety
  net only. PR body must explain WHY the change is destructive
  (e.g. "DROP TABLE removes a deprecated v55 audit log table;
  restore-point flow covers rollback").

The PR template (`.github/pull_request_template.md`) requires
the label as the first checkbox. CI comments on the PR with a
**stern warning** if the label is missing — the merge is not
blocked, but the warning is loud and visible.

### The check

`dixiedata debug in-place-safety` walks `git diff <last-release-tag>..HEAD`
and flags:

- **Schema operations:** `DROP TABLE`, `DROP COLUMN`, `RENAME`
  without a preserve-old pattern, `DELETE FROM` in a migration
  block.
- **Handler changes:** routes registered in `routes.go` whose
  patterns or methods changed.
- **Startup flows:** first-launch handlers that touch data
  without a confirmation gate.

Output is a list of findings with file:line. Exit code is the
count of findings (0 = clean). The check is informational;
the `safe-for-in-place` label is the human acknowledgment.

### Why "safe for in-place" instead of "blocking"

The current `main` is the in-place-update release line. Blocking
merges to `main` would force every release through `dev` first,
which is the current workflow (per `AGENTS.md`). The
`safe-for-in-place` label surfaces the question at PR time
without changing the merge gate — the human reviewer is the
gate, the label is the prompt.

### ADR 0007

The formal contract for in-place update safety is documented in
[`docs/adr/0007-in-place-update-safety.md`](../adr/0007-in-place-update-safety.md).
This section summarizes; the ADR is the source of truth.

---

## References

- [`CONTEXT.md`](../../CONTEXT.md) §Laws — dialog-guard law,
  Backend-First law (read first)
- [`AGENTS.md`](../../AGENTS.md) §Branch policy — when dev vs
  main is the target
- [`docs/RELEASING.md`](../RELEASING.md) — release mechanics
  (this doc covers the protocol; RELEASING.md covers the steps)
- [`docs/agents/cli-plan.md`](cli-plan.md) — CLI subcommand
  roadmap (every documented subcommand lives here)
- [`docs/agents/feature-protocol.md`](feature-protocol.md) —
  the change protocol that feeds into this build protocol
- [`docs/agents/INDEX.md`](INDEX.md) §Tier 0 — load this file
  before any release work
- [`scripts/bump-version.ps1`](../../scripts/bump-version.ps1) —
  the bump script (-VerifyOnly, -Force, -DetectDrift)
- [`scripts/release-github.ps1`](../../scripts/release-github.ps1) —
  the tag + push + draft gh release script
- [`docs/adr/0001-in-place-update-restore-points.md`](../adr/0001-in-place-update-restore-points.md) —
  ADR for the restore-point flow
- [`docs/adr/0007-in-place-update-safety.md`](../adr/0007-in-place-update-safety.md) —
  ADR for the safety gate (this protocol's contract)
- Issue #257 — the "shipped but invisible" sweep that
  motivated the Backend-First law; same shape of pattern
  motivates the freshness + safety checks here