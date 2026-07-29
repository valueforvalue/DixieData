# Promotion protocol: dev → main gate chain

## Status

Accepted 2026-07-03. Lifts the dev → main promotion step from
a user-driven ad-hoc flow into a documented gate chain, so the
`safe-for-in-place` label (which ADR 0007 establishes as
informational at PR time) becomes a hard gate at the right
moment — the last step before users get the binary.

## Context

ADR 0007 (in-place update safety) leaves a deliberate gap: the
`safe-for-in-place` label is "label-not-block per ADR 0007"
because enforcement at merge time would block the entire
release pipeline (the in-place update flow itself calls into
handlers that may not yet be label-stamped). The maintainer
explicitly offloaded the hard enforcement to "the promotion
ADR (TBD 0008)" (`docs/adr/0007-in-place-update-safety.md`,
Consequences §Negative). This is that ADR.

The current flow is:

```bash
# (On dev, after some number of commits land)
git checkout main
git merge dev
git push origin main
git tag v1.2.N && git push --tags
# PowerShell helper does the rest:
scripts/release-github.ps1
```

Three failure modes were observed in the current flow:

1. **No defined promotion gate.** When does dev → main happen?
   After how many commits? After how many days? Today's answer is
   "whenever the maintainer runs `release-github.ps1`." No
   `versioninfo.go` bump verification, no test pass, no in-place-
   safety check.
2. **No defined promotion cadence.** The release line `v1.2.N`
   is tied to `CurrentSchemaVersion`. A promotion with no schema
   bump still tags a release. A promotion with an unverified
   schema bump tags a release that may not be installable on
   older DBs.
3. **The safety label becomes optional.** If the merge isn't
   gated, the human acknowledgment required by ADR 0007
   erodes. Reviewers approve `safe-for-in-place` because the
   merge button is green, not because the four rules were
   walked. The promotion step is the natural place for a hard
   gate — the LAST step before users get the binary.

The maintainer's rule on this (AGENTS.md §Branch policy) is
already explicit: "do not promote dev to main without explicit
user direction." This ADR codifies WHAT the user is signing off
on at that moment.

## Decision

`just promote` is the single entry point for `dev → main`. It
runs the same gate chain as `just release-pipeline`, with one
extra step that lifts the ADR 0007 label from informational to
hard-enforced.

### The promotion command

```bash
just promote          # user-driven (default)
just promote-dry-run  # prints gate status + diff summary, no push
```

`just promote` is the runnable form of this ADR. The gate
chain below is what `promote` invokes. `promote-dry-run` is the
explicit pre-flight the user reviews before committing to the
real run.

### Gate chain (in order)

1. **`just test-lint && go test -short ./...`** — `go test -short ./...` passes. Halts on
   any failure.
2. **`just tpl`** — templ files current. Halts if any `.templ`
   source has drifted from the generated `_templ.go`.
3. **`just css`** — Tailwind classes current. Halts if
   `frontend/app.css` is stale relative to `frontend/tailwind.css`.
4. **`just bump -VerifyOnly`** — `versioninfo.go`,
   `CHANGELOG.md`, and the docs references are consistent.
   Halts on drift.
5. **`just debug`** — main binary builds + smoke-tests pass.
6. **`just verify-fresh-bake`** — subtools + CLI coverage + the two
   info-only Nix-cache probes are clean.
7. **`make in-place-safety`** — `dixiedata debug in-place-safety
   <base-ref> <head-ref>` where `base-ref` is the last release
   tag and `head-ref` is `dev HEAD`. Exit non-zero on any
   HIGH-severity finding. This is the gate that lifts ADR 0007
   from label-not-block to hard-enforced at the right moment
   (the last step before users get the binary).
8. **`just archive`** — release zip exists, contains the right
   binary + DLLs + README.
9. **`just release-github`** — tag + push + draft gh release
   (with the gate check before the actual tag).

### What `dev` HEAD looks like at promotion time

The maintainer merges `dev` into `main` only after `make
promote-dry-run` prints clean. Until then, `main` is
unaffected. The merge IS the promotion; the tag IS the gate
result.

### Gate failures

Each gate's failure has a defined recovery path:

- **test fails** → fix the failing test, re-run from gate 1.
- **tpl/css fails** → regenerate, re-run from gate 2 or 3.
- **bump-verify fails** → fix the doc drift (`CHANGELOG.md`,
  `AGENTS.md`, `CONTEXT.md`, `RELEASING.md`), re-run from gate 4.
- **in-place-safety fails** → review the HIGH-severity findings;
  either fix the destructive change (preferred), label the
  offending PRs as `unsafe-for-in-place` with a one-paragraph
  WHY in the PR body, or escalate to the user. Re-run from
  gate 7.

### When does promotion happen?

Two paths:

a. **User-driven** (current flow, what this ADR ships): the
   maintainer runs `just promote` when ready. The gate chain
   tells the maintainer what's missing. The dry-run variant
   exists explicitly for "show me the state without
   committing."

b. **Cadence-driven** (future, follow-up issue): after every
   schema bump, a CI job opens a draft promotion PR. Manual
   review + `just promote` to ship. This is the next step but
   NOT in this ADR — the cadence question is its own decision
   (how often, what marks "ready", who reviews the draft).

For this ADR, ship (a). (b) is a follow-up tracked in
`docs/agents/cli-plan.md` (cross-reference added when that doc
gets its promotion paragraph).

### Pre-promotion checklist (PR review)

The maintainer reviews the diff visually before running
`just promote`. The default is:

- `git diff <last-tag>..origin/dev --stat` — the surface area
  (how many files, how many lines, how many new files).
- `git log <last-tag>..origin/dev --oneline` — the commit
  shape (one-commit-per-change? bundled? reverted? long-lived
  branch?) per AGENTS.md §Commits and branches.
- The three review skills (`design-it-twice`, `review`,
  `code-review-global`) on the diff, if the surface is
  non-trivial.

`just promote-dry-run` exposes the gate chain output as a
single ingest; the maintainer reads that output before
committing to `just promote`.

## Alternatives considered

### Alt 1: Per-PR enforcement of `safe-for-in-place`

Enforce the label at PR merge time. Rejected because the
in-place update flow itself calls into Go handlers — the flow
that ships the new binary needs to compile + run BEFORE the
label can be applied, creating a chicken-and-egg with the
release path. ADR 0007 already calls this out in its
Consequences §Negative ("the label is not a hard gate").

### Alt 2: Continuous promotion (every commit lands on main)

What modern CI/CD does for trunk-based development. Rejected
because DixieData's release line was historically `v1.2.N`
per `CurrentSchemaVersion`; the tagged release was a
meaningful boundary that breaks if dev flows continuously
into main. The `v{MAJOR}.{U}.{N}` split that issue #266
shipped in commit 5a297a6 (this ADR accepted on 2026-07-03)
is the mid-ground that unlocks cadence-driven promotion —
tracked as path (b) under "When does promotion happen?" above.

### Alt 3: Manual `release-github.ps1` (current flow) + better docs

The minimum-change path: improve `release-github.ps1`'s inline
help + a `RELEASING.md` walkthrough, no `just promote` target.
Rejected because the script-based flow has no failure-mode
documentation; the maintainer runs the steps, watches stderr,
and improvises recovery. A target-based gate chain documents
the steps in code (executable) rather than prose.

## Consequences

### Positive

- Every PR to `main` has its four ADR 0007 rules reviewed at
  the gate, not at PR time. Reviewers can still apply the
  label informally; the gate is the hard check.
- `just promote-dry-run` is a no-risk pre-flight that the
  maintainer can run any time without affecting the working
  tree (it doesn't push; it doesn't tag; it only reads).
- The gate chain is encoded as code — `just promote` is
  discoverable, executable, and CI-runnable. Future agents
  (human or LLM) can read `just --list` to learn the flow
  rather than memorising `release-github.ps1` steps.
- The cadence question (path b) is now visible: with the
  gate chain in code, the cadence is the only thing missing
  for a fully automated promotion flow.

### Negative

- The maintainer must run `just promote-dry-run` before
  `just promote`; the dry-run is not the default. Drift risk:
  on a tired evening, the maintainer may skip the dry-run and
  find out at gate 7 (in-place-safety). Mitigation: gate 7 has
  a fast sub-second path on a clean diff, so skipping the
  dry-run costs ~1s on average; not worth enforcing.
- The gate chain assumes `just test-lint && go test -short ./...`, `just tpl`, etc. each
  exist and work. If a future refactor removes one, the
  promote target's behaviour changes silently. Mitigation: a
  separate "AGENTS.md §Branches" review notes that any
  removal of a `make` target needs the companion check in
  `just promote` reviewed in the same commit.

### Compatibility

- `just release-pipeline` is unchanged — it already runs the
  subset of these gates as opt-in. The new `just promote`
  adds gate 7 (in-place-safety) as a HARD gate; the existing
  pipeline uses it as INFORMATIONAL only.
- `scripts/release-github.ps1` continues to handle the tag +
  draft-release mechanics. The `just promote` target calls it
  at gate 9.
- The `safe-for-in-place` / `unsafe-for-in-place` labels are
  unchanged. ADR 0007's label protocol continues to apply at
  PR time.

## Implementation notes

> **As of 2026-07-03**, the destination branch is `stable`,
> not `main`. See [ADR 0009 — Stable branch as released-code
> home](0009-stable-branch-promotion.md). The gate chain
> below is unchanged; only the destination name changes.

### Existing infra this ADR composes with

- `Makefile` — `just --list` already lists the related targets;
  add `just promote` and `just promote-dry-run`.
- `internal/appshell/cli_debug_test.go` — already covers the
  `dixiedata debug in-place-safety` walker; the gate at step
  7 is the CLI binary's contract.
- `docs/RELEASING.md` — the release procedure doc; this ADR
  expands §Release workflow with the gate chain.
- `AGENTS.md` §Branch policy — the "Promotion: dev → main"
  subsection gets a one-line cross-reference to this ADR.

### Files this ADR will touch (NOT this commit)

- `Makefile` — add `promote` and `promote-dry-run` targets
- `CONTEXT.md` §Laws — add "Promotion to main = `just promote`"
- `AGENTS.md` §Branch policy — cross-reference this ADR
- `docs/RELEASING.md` — release workflow expansion
- `scripts/release-github.ps1` — unchanged

### Open questions

1. **Tag naming.** Today's tags are `v1.1.{N}` (the
   v{MAJOR}.{U}.{N} shape shipped in commit 5a297a6 per
   issue #266 — U=1 implicit for legacy v1.2.N releases).
   Should pre-promotion tags (the base-ref for
   in-place-safety) be different, e.g. `v1.1.{N}-pre` for
   the pre-promotion state? Decide before first
   `just promote` run.
2. **Rollback after a failed in-place update.** ADR 0007 +
   ADR 0001 cover the BEFORE-update state (restore points)
   and the change-shape contracts. They don't cover the
   AFTER-update state — when a release ships, gets to users,
   and then a critical bug is found. The rollback story
   (tag a new release that re-applies the previous? user
   manually rolls back?) is its own decision; track in a
   follow-up issue.
3. **Multi-platform release artifacts.** `just archive`
   produces a Windows zip. macOS/Linux users currently have
   no in-place update path because there's no artifact for
   their platform. Out of scope for this ADR; tracked in
   the `cli-plan.md` follow-ups.

### Regression net (this ADR carries no code; the gate chain's
regression is its first run)

- After this ADR lands, the next `just promote-dry-run` is
  the proof that the gate chain is wired correctly.
- A follow-up `internal/update/updater_test.go` test should
  exercise the in-place-safety gate (compareVersions reads
  the `safe-for-in-place` label via the GitHub API at startup
  and warns if missing); not part of this ADR but mentioned
  in issue #267's body for future work.

## References

- [`docs/adr/0007-in-place-update-safety.md`](0007-in-place-update-safety.md)
  — the four rules this gate enforces
- [`docs/adr/0001-in-place-update-restore-points.md`](0001-in-place-update-restore-points.md)
  — the binary-rollback contract
- [`CONTEXT.md`](../../CONTEXT.md) §Laws — cross-cutting
  context that every release ships under
- [`AGENTS.md`](../../AGENTS.md) §Branch policy — the maintainer
  rule this ADR codifies
- [`docs/RELEASING.md`](../RELEASING.md) — release mechanics
  this ADR composes with (rewritten in commit e4f6a97 to
  cover the v{MAJOR}.{U}.{N} shape + three-counter bump
  script; per this ADR's "Files this ADR will touch"
  follow-up)
- [`docs/agents/build-protocol.md`](../agents/build-protocol.md)
  §5 — canonical build procedure
- Issue #266 — `v{MAJOR}.{U}.{N}` version split (shipped in
  commit 5a297a6; the cadence-driven promotion that this
  split unlocks is path (b) of "When does promotion happen?")
- Issue #255 — Support & Diagnostics move (unrelated; not
  this ADR; surfaced during the same sweep)
- Issue #293 — CLI + UI version emit switched to new shape
  (commit e807297)
- Issue #294 — bump-version.ps1 + release tag use new shape
  (commits 011173f + e4f6a97)
- Issue #295 — this doc update (RELEASING.md + ADR 0008 +
  CONTEXT.md)
- Issue #296 — `BackupManifest` carries U + N explicitly
  (commit forthcoming)


## Author

Jeremy Morris (@jeremymorris) — backfilled 2026-07-17
