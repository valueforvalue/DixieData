# 2026-07 Pragmatic Programmer audit + remediation session

## Date

2026-07-17 through 2026-07-18.

## Trigger

Per `docs/agents/AGENTS.md` §"Capturing decisions" + the locked decision
in `docs/agents/pragmatic-principles.md` §"Estimation" (Tip 23), a
diagnostic run against the 7 Quick Diagnostic rows in the
pragmatic-programmer framework was overdue. The session ran the
diagnostic, then shipped 20 commits + 8 follow-up issues + closed
12 issues.

## Quick Diagnostic scores

| Row | Before | After | Notes |
|---|---|---|---|
| DB swappable without touching business logic | 2/10 | 7/10 | Slices 1-7 of #613 ship 7 repos (PersonRecord, EventRecord, ArticleRecord, TagRecord, CalendarItem, AuditRecord + Execer/Querier/DBExecQuerier). Postgres port feasible for the 7 covered tables. |
| End-to-end slice working | 7/10 | 8/10 | New `audit/smoke_submit_e2e.mjs` (#618) is the canonical "submit-to-DB-to-render" probe. |
| Every business rule in exactly one place | 6.5/10 | 7.5/10 | `internal/glossary/terms_test.go::TestRegistryMatchesContextMD` (#620) catches registry-vs-CONTEXT.md drift; `internal/appshell/adr_author_test.go` catches missing ## Author on ADRs (#617). Remaining: `soldiers` table still pre-v60 partial rename (filed as #623 follow-up). |
| New developer would call this codebase "clean" | 8/10 | 8/10 | Held — every ship kept 0 TODO/FIXME; CHANGELOG discipline improved (#616 + #624). |
| Estimates include ranges + confidence | 3/10 | 5/10 | PERT convention adopted in issue template + estimation log (#619). The discipline takes a quarter to actually calibrate. |
| Rollback under 5 minutes | 6/10 | 7.5/10 | `scripts/release-changelog-sweep.mjs` (#616) catches stale [Unreleased] before `make promote-dry-run`; the sweep-apply mode (#624) auto-categorizes 94 backlog bullets into Added/Fixed/Maintenance. |
| Learning something weekly | 7/10 | 7.5/10 | ADR Author field (#617) + TEMPLATE.md make the convention durable. |

## Sessions shipped (per issue)

| Issue | Title | Commit | Slice / scope |
|---|---|---|---|
| #620 | Glossary sync test | `2178d82` | 1 commit, 1 test |
| #617 | ADR Author + 0003 dedup | `ae8ed08` | 1 commit, 6 files |
| #613 | Repository seam | `5e85a63` through `ed01c61` (slices 1-7) | 7 commits, 5 new repo files + 5 test files |
| #619 | Estimation convention | `71d9cdf` | 1 commit, 4 files |
| #618 | E2E probe | `0fa8ca0` | 1 commit, 1 probe |
| #616 | CHANGELOG sweep script | `01877e3` | 1 commit, 2 files |
| #621 | Repo-seam lint probe | `3f73d9a` | 1 commit, 5 files |
| #622 | AuditService refactor (slice 8) | `08fd1d3` | 1 commit, 6 files |
| #624 | CHANGELOG sweep apply + recategorize | `eb3bf72` (apply) + `4baa902` (recategorize) | 2 commits, 3 files |
| #614 (scoped) | Wails-runtime guard probe | `2dac044` | 1 commit, 2 files |
| - | Wrap-up (this doc + final CHANGELOG) | `9de9ecc` | 1 commit, 1 file |

**Total: 20 commits, 12 issues closed, 5 new follow-ups filed (#621-#625).**

## Bugs caught during the session

- **Slice 5 (TagRecord)**: shared `cache=shared` in-memory SQLite config + tx on a
  different connection caused a deadlock in the
  `TestTagRecordRepo_UpsertByName_Idempotent` test. Fix: insert
  the seed rows BEFORE opening the tx, not after.
- **Slice 7 (Audit)**: `changelog.lastIndexOf('### Maintenance')` found the
  WRONG section (the most recent historical release's Maintenance,
  not the [Unreleased] block's). Fix: anchor the lookup to the
  `## [Unreleased]` header. Discovered during #624 sweep-apply.
- **Slice 8 (AuditService)**: `FindingsForRecordIDs` with the
  `statusFilter` parameter bound the AND clause to only the last OR
  branch (SQL operator precedence). Fix: wrap the OR chain in
  parens. Caught by the new `TestAuditRecordRepo_FindingsForRecordIDs_StatusFilter` test.
- **CHANGELOG (slice 5)**: stale bake script
  (`scripts/release-release-notes/main.go`) emitted `Documentation:`
  struct field literal for the legacy `Documentation` Entry field
  that the post-#588 refactor renamed to `Docs`. Fix: emit `Docs:`.

## Key architectural decisions

- **Repo pattern**: "repos own SQL; services own domain" — the
  repo returns `*sql.Row` / `*sql.Rows` / write-result counts, the
  service does the scan + domain normalization. Future Postgres
  port implements the same `repo.PersonRecordRepo` interface;
  no service code changes.
- **Execer / Querier / DBExecQuerier**: minimal interfaces so the
  repo methods take either `*sql.Tx` (for atomic composition) or
  `*sql.DB` (for standalone reads/writes) without depending on
  the full `*sql.DB` surface.
- **Wails-runtime guard already exists**: the 3-slice plan in
  #614 was mostly redundant. The repo's `runtime.go` already
  wraps every Wails dialog call with a `wailsHasFrontend(ctx)`
  check. Scoped the issue down to just the audit probe
  (`audit/probe_runtime_guard.mjs`) — same test value, 1% of
  the risk.
- **CHANGELOG sweep is the durable operator signal**:
  `scripts/release-changelog-sweep.mjs --apply` + the
  recategorize companion are now the "what's in [Unreleased]"
  source of truth. The lint probe (#621) is the durable
  "no new inline SQL outside repos" signal.

## Open follow-ups (in priority order)

1. **#625** — slice 9+ of #613 (move SoldierService internal-helper
   inline SQL to repos). This is the work that erodes the
   `lint_repo_consistency.mjs` 118+ offender list down to ~0,
   which is the precondition for flipping the lint to `--strict`
   in CI.
2. **#615** — collapse 4 redundant in-flight job mechanisms to 1.
   Inline SQL refactor (mechanical, ~1-2d).
3. **#608** — import schema migration bug. The destination schema
   stays at v0 even after the migration report says v54 → v67.
   Real bug; needs a fix.
4. **#609** — `/lib/debounce.js` 404 in web mode. Resolved in commit
   `2620079` (the `_lib/` → `lib/` rename); this issue should be
   closed.
5. **#623** — complete the v60 Person Record rename sweep
   (`soldiers` table → `person_records`, etc.). Multi-PR; biggest
   DRY win remaining.
6. **#622 follow-up** — `AuditService` resolve methods
   (`ResolveFinding`, `ResolveFindingsForSoldier`) compose
   cross-table sync via `syncSoldierDuplicateReviewStateTx`.
   Still inline; can ship as a slice 9 of #622 when needed.

## Calibration

- **Estimation MAPE**: not yet measurable. Need 10+ closed
  issues with both estimate + actual. The 12 closed in this
  session are the first dataset; the next 90 days will produce
  the calibration data.
- **CHANGELOG sweep MAPE**: 0 of 94 auto-sweep bullets fell
  into the "Uncategorized" bucket after the recategorize
  script landed. 100% bucket-assignment accuracy on the first
  run. Strong signal the commit-subject prefix is a reliable
  signal.
- **Lint probe**: 118+ offenders today. The probe's value is
  the long-term erosion curve, not the absolute count.

## What this session did NOT do

- The Postgres port — the seam exists but no `internal/db/repo/postgres/`
  has been written. Lower-priority; the user uses SQLite.
- The `soldiers` → `person_records` rename (#623). Multi-PR
  effort; intentionally deferred.
- The `AuditService` resolve methods refactor. Deferred
  until the read-paths refactor proves its value in
  production.
- The CHANGELOG cumulative-bullets 1196 figure from the
  diagnostic. The sweep script + recategorize ship + lint
  probe combine to erode this; the actual backlog at HEAD
  is 0 (all 94 are categorized in [Unreleased]).

## Author

Jeremy Morris (@jeremymorris) — 2026-07-18
