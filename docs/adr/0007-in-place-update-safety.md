# In-place update safety contract

## Status

Accepted 2026-07-02. Locks in the safety gate that ships with the
build-protocol pack (PR for `make freshness`, `make release-
pipeline`, `dixiedata debug in-place-safety`, `safe-for-in-place`
label, schema-touching detector in CI).

## Context

DixieData's release line is schema-driven: `CurrentSchemaVersion`
in `internal/versioninfo/versioninfo.go` is the single source of
truth. The app version string is `v{MAJOR}.{MINOR}.{SCHEMA}`
where `SCHEMA == CurrentSchemaVersion`. The local update feature
(`internal/update/updater.go`) downloads a tagged release and
applies migrations on top of the existing `.dixiedata` database.

**Everything that ships on `main` reaches users via in-place
update.** The only exception today is a full re-install +
restore-point swap, which the user opts into manually.

Three failure modes were observed (or anticipated) before this
ADR:

1. **Destructive schema migration ships without rollback path.**
   A `DROP TABLE` or `DROP COLUMN` in a migration deletes data
   on upgrade; users on the affected version cannot roll back
   via in-place update. The restore-point flow covers binary
   rollback but not data deletion.
2. **Handler signature changes break the in-app updater.**
   The updater itself calls into Go handlers (e.g. `Check()`,
   `PrepareLatest()`, `Apply()` in `internal/update/updater.go`).
   A signature change without a backward-compat shim breaks
   the update flow mid-flight.
3. **Removal of a safety gate.** `updateEligibility` only
   checks `IsDevelopmentBuild`; removing that check would let
   a development build self-update and corrupt the user's
   `.dixiedata/` directory. Removing the dialog-guard
   `inFlight` check or the dialog-guard law in `CONTEXT.md`
   would silently re-introduce the crash that ADR 0001 / the
   `c1d9dc1` fix resolved.

Issue #257's "shipped but invisible" sweep (2026-06) surfaced
four features where backend + handler + route landed with no
UI to invoke them. The same shape of drift (code shipped
without an automated or human tripwire catching it) motivates
this ADR.

## Decision

### Four rules for "safe for in-place update"

A change is safe for in-place update if:

1. **No destructive schema migrations.** No `DROP TABLE`,
   `DROP COLUMN`, `ALTER TABLE ... RENAME` without a
   preserve-old pattern, `DELETE FROM` in a migration block,
   or removal of an `IF EXISTS` guard. The migration runs
   forward-only; the user cannot roll back data deletion via
   the restore-point flow.
2. **No handler signature changes that break existing
   routes.** No route pattern rename without a backward-
   compat shim. No HTTP method change (GET → POST) without
   a shim. The in-app updater flow routes through `a.mux`;
   a broken signature stalls the update.
3. **No auto-destructive flows on startup.** No first-
   launch trigger that deletes records, drops tables, or
   replaces the data dir. Startup flows run before the user
   can intervene.
4. **No removal of safety gates.** `updateEligibility`'s
   `IsDevelopmentBuild` check, `inFlight.LoadOrStore` on
   native dialog calls, the dialog-guard law in `CONTEXT.md`.
   These are the silent regressions that ship without a
   compile error.

### The `safe-for-in-place` / `unsafe-for-in-place` label protocol

Every PR to `dev` or `main` carries one of:

- `safe-for-in-place` — the change has been reviewed against
  the four rules above; safe to ship via in-place update.
- `unsafe-for-in-place` — the change is intentional and known
  destructive; ships via full re-install + restore-point safety
  net only. The PR body explains WHY (e.g. "DROP TABLE
  removes a deprecated v55 audit log table; restore-point
  flow covers rollback").

The PR template (`.github/PULL_REQUEST_TEMPLATE.md`) requires
the label as the first checkbox. CI comments on the PR with a
**stern warning** if the label is missing when targeting
`main`. **The merge is not blocked.**

### Why "label-not-block"

Three reasons:

- The current `main` is the in-place-update release line.
  Blocking merges to `main` would force every release through
  `dev` first, which is the existing workflow (per
  `AGENTS.md` §Branch policy). The promotion step (TBD ADR
  0008) is the natural place for a hard gate.
- The human reviewer is the safety gate. The label is the
  prompt. A warning comment is loud and visible; the reviewer
  either applies the label (acknowledging the rule), adds the
  missing label themselves, or escalates to the user.
- The automated `dixiedata debug in-place-safety` check
  catches the most common violations (schema drops, handler
  registrations) and exits non-zero on high-severity findings.
  The CI check is the tripwire; the label is the paper trail.

### The automated check

`dixiedata debug in-place-safety` walks `git diff <last-release-
tag>..HEAD` and flags:

- `DROP TABLE` / `DROP COLUMN` / `ALTER TABLE ... RENAME` /
  `DELETE FROM` in `internal/db/` or `docs/migrations/`
  files — **high severity**.
- `r.Get(` / `r.Post(` / `r.Put(` / `r.Delete(` /
  `r.Patch(` / `r.Handle(` in `routes.go` — **medium
  severity** (the diff can't distinguish new vs renamed vs
  method-changed; the human reviews the finding).

Exit code = count of high-severity findings (0 = clean).
The check is INFORMATIONAL — it does not block the merge.
`make release-pipeline` runs it as one of the gates.

### Relationship to ADR 0001 (Restore Points)

ADR 0001 establishes the restore-point flow as the binary
rollback path. This ADR (0007) establishes the **schema +
handler + safety-gate** contracts that the restore-point flow
**cannot protect**. The two ADRs compose:

- ADR 0001: binary rollback (the new binary doesn't work,
  restore the previous binary + previous data).
- ADR 0007: change-shape contracts (the new binary's change
  shape — destructive migration, handler rename — breaks
  users on older DBs even when the new binary is healthy).

The restore-point flow is the safety net for binary failures.
This ADR's four rules are the safety net for change-shape
failures. Both ship; both matter.

### Future work (issue #266)

Issue #266 proposes splitting the version string into
`v{MAJOR}.{U}.{N}` where `U` is a separate update-flow gate.
When that lands, this ADR's four rules expand to include the
update-flow gate:

- Rule 5: a change that modifies the in-place update flow
  itself (restore-point contract, eligibility check, migration
  runner mechanics) bumps `U`, and releases with higher `U`
  cannot be auto-applied by an older binary.

For now (current ADR), `U` is implicitly 1; the four rules
above cover the current shape. The expansion is a follow-up.

## Consequences

### Positive

- Every PR to `main` has a human acknowledgment of the four
  rules. The label is the paper trail; the warning comment
  is the loud prompt.
- The `dixiedata debug in-place-safety` check catches the
  most common violations automatically. `make release-pipeline`
  surfaces the findings before the tag is pushed.
- The PR template places the safety question at the top so
  the reviewer can't miss it.

### Negative

- The label is not a hard gate. A reviewer who ignores the
  warning can still merge a destructive change. The mitigation
  is the promotion ADR (TBD 0008) which adds the hard gate at
  the `dev → main` promotion step.
- The medium-severity handler-registration findings are
  noisy: every new route flags. The noise is the point — the
  human reviews each.
- The check walks git diff text, not a structured change log.
  A rename that the diff formats strangely (multi-line, etc.)
  may slip through. The CI check is the tripwire, not the
  guarantee.

### Compatibility

- The PR template replaces the existing `.github/PULL_REQUEST_
  TEMPLATE.md`. The existing template is empty (placeholder
  checkboxes only); the new template is additive.
- The `safe-for-in-place` / `unsafe-for-in-place` labels are
  created by `scripts/sync-labels.sh`. Existing PRs in flight
  at the time of this ADR's merge get a one-time back-fill
  (the operator applies `safe-for-in-place` by default for
  PRs that don't touch schema).
- `dixiedata debug in-place-safety` is additive to the
  existing debug subcommands. Existing `make release` flows
  are unchanged; `make release-pipeline` is opt-in.

## References

- [`docs/agents/build-protocol.md`](../agents/build-protocol.md) §5
  — the canonical procedure (this ADR is the contract; the
  doc is the steps)
- [`CONTEXT.md`](../../CONTEXT.md) §Laws — the cross-cutting
  context that every change ships under
- [`docs/RELEASING.md`](../RELEASING.md) — the release
  mechanics that the safety gate composes with
- [`docs/adr/0001-in-place-update-restore-points.md`](0001-in-place-update-restore-points.md) —
  the binary-rollback contract this ADR composes with
- [`internal/update/updater.go`](../../internal/update/updater.go) —
  `updateEligibility`, `compareVersions`, `Settings`
- [`internal/versioninfo/versioninfo.go`](../../internal/versioninfo/versioninfo.go) —
  `CurrentSchemaVersion` (the single source of truth)
- [`scripts/bump-version.ps1`](../../scripts/bump-version.ps1) —
  the bump script (`-VerifyOnly`, `-Force`, `-DetectDrift`)
- [`.github/PULL_REQUEST_TEMPLATE.md`](../../.github/PULL_REQUEST_TEMPLATE.md) —
  the PR template that requires the safety label
- [`.github/workflows/test.yml`](../../.github/workflows/test.yml) —
  the CI check that posts the stern warning
- `internal/appshell/cli_debug_inplace.go` — the
  `debug in-place-safety` handler
- Issue #257 — the "shipped but invisible" sweep that
  motivated the parallel Backend-First Law
- Issue #266 — the future `v{MAJOR}.{U}.{N}` version split

## Author

Jeremy Morris (@jeremymorris) — backfilled 2026-07-17
