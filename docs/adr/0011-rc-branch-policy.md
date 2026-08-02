# RC branch feature-freeze policy

## Status

Accepted 2026-07-29. Codifies the policy the v1.1 RC1 cohort
surfaced as needed (issues #665, #666, #667, #668). Closes
the loop on issue #668.

## Context

The v1.1 RC1 cohort shipped eight RCs before reaching a
clean state. The early RCs mixed real bug fixes with new
features in the same commit (per the AGENTS.md "what lands in
this slice" sections of #658, #660, #661, #665, #666):

| RC | Real fix | New feature |
|---|---|---|
| 1.1.5 | theme persistence | debug-mode toggle UI |
| 1.1.6 | version compare | Check for Updates button |
| 1.1.10 | debug-mode logging | shell theme default |
| 1.1.13 | tag-delete form | memorial / config / progress bar / PDF |
| 1.1.14 | tag-delete dispatcher | (clean) |
| 1.1.15 | check-for-updates polling | (clean) |
| 1.1.16 | article delete | (clean) |
| 1.1.17 | (clean) | `--seed` flag + `seed-fixture.ps1` |

The last three RCs were clean — real fixes only. The user
wants the clean pattern codified so the next RC line (1.2,
2.0) starts disciplined from RC1 rather than learning the
pattern mid-cohort.

The v1.1 RC1 cohort also surfaced that the existing CI
gates are designed for the `dev → main` flow (ADR 0008) and
the three-branch model (ADR 0009). Neither covers the
`rc/v*` lifecycle. The chain today is:

```
dev (integration) → rc/v* (release candidate) → stable (released)
                                              → main (legacy anchor)
```

The `rc/v*` branch currently accepts anything because no
gate is wired against it. A `feat(...)` commit that lands
on `rc/v1.1` mid-cohort can carry user-visible new behavior
into a release candidate without the user being able to
distinguish "fix" from "feature" at review time.

This ADR adopts a [Zephyr-style
feature-freeze](https://docs.zephyrproject.org/latest/project/release_process.html)
policy on `rc/v*` branches so the discipline the cohort
surfaced is codified at the branch level.

## Decision

### Allowed vs disallowed commit types on `rc/v*`

| Type | Allowed? | Rationale |
|---|---|---|
| `fix(...)` | yes | Stabilization fixes are the RC purpose |
| `docs(...)` | yes | Doc changes cannot break the binary |
| `chore(...)` | yes | Tooling that ships in the release is in scope |
| `test(...)` | yes | Regression nets are stabilization |
| `ci(...)` | yes | Lint + test gates ARE RC stabilization |
| `feat(...)` | **no** | New features belong on `dev` first |
| `refactor(...)` | **no** | Refactors belong on `dev` first |
| `perf(...)` | **no** | Performance work belongs on `dev` first |
| `build(...)` | **no** | Build-system changes belong on `dev` first |

**Bypass:** a `release-blocker` label on the PR (set by
the operator after triage) exempts the PR from the gate,
with a comment in the PR body explaining why. The label
is informational in this ADR (a future slice may gate the
label application; this ADR does not).

### Migration story (RC → stable → tag) on top of ADR 0008 / 0009

ADR 0008 codifies `dev → stable` + tag with a 4-gate
chain. ADR 0009 codifies the three-branch model (`dev`,
`stable`, `main`). This ADR adds `rc/v*` in front of
`stable`. The promotion chain becomes:

```
dev (integration) ──┐
                    ├──> rc/v* (release candidate) ──> stable (released)
                    │    │                              │
                    │    └─ feat/refactor/perf/build ── backport to dev
                    │                                      before next RC line
                    │
                    └──> main (frozen legacy anchor)
```

Once `rc/v*` reaches a clean state, the operator merges
`rc/v*` into `stable` per ADR 0008's gate chain. The
backport flow is: any `feat/refactor/perf/build` commit
that landed on `rc/v*` (despite the gate) must be
backported to `dev` before the next RC line begins, so
`dev` stays authoritative.

### Diff-size gate

A PR with ≥ 50 files changed is treated as
refactor-by-stealth and rejected at lint time. The threshold
matches ADR 0008's promotion-gate scale; the gate lives in
the same `scripts/ci/` directory and runs from the same CI
workflow.

### CI integration

- `scripts/ci/lint-rc-commits.mjs` + `.test.mjs` —
  commit-type classifier. Walks every commit on a PR
  targeting `rc/v*`, fails if any has a disallowed type
  (`feat/refactor/perf/build`) or no type prefix. Also
  rejects diffs ≥ 50 files as refactor-by-stealth.
- `.github/workflows/rc-lint.yml` — runs the gate on
  every PR targeting `rc/v*`. Mirrors the
  `lint-bake-bootstrap` pattern in `test.yml`: a
  lightweight job that fails fast.
- `AGENTS.md` — three-branch model updated to four-branch
  (`dev` / `rc/v*` / `stable` / `main`), with the RC
  policy section + the updated promotion flow.
- `.github/BRANCH_PROTECTION.md` — the `rc/v*` rules
  (require `release-blocker` label + `lint-rc-commits`
  status check on top of the standard rules).

### Sources

- [Zephyr Project release process](https://docs.zephyrproject.org/latest/project/release_process.html)
  — the model for the allowed/disallowed types table.
- [VisIt RC development](https://visit-sphinx-github-user-manual.readthedocs.io/en/3.4rc/dev_manual/RCDevelopment.html)
  — the model for the RC → dev backport discipline.
- [Stack Exchange q/432957](https://softwareengineering.stackexchange.com/questions/432957)
  — the gitflow consensus on branching off the RC vs
  cherry-picking.

## Consequences

Positive:

- The next RC line (1.2, 2.0, ...) starts disciplined
  from RC1: every commit is either a real fix or a
  stabilization gate, never a new feature. Reviewers can
  trust that a green RC is shippable.
- Mid-cohort feature changes are forced onto `dev` where
  they belong, eliminating the "RC shipped a feature
  unexpectedly" surprise from the v1.1 RC1 history.
- The backport-to-dev discipline on close keeps `dev`
  authoritative and `rc/v*` short-lived.

Negative:

- A legitimate `feat(...)` discovered during the RC cohort
  cannot land on `rc/v*` directly. The operator must
  either revert it, cherry-pick to `dev` (slow), or apply
  the `release-blocker` label (requires triage). The
  `release-blocker` label bypass is the operational
  pressure-release valve; the gate is intentionally not
  ironclad.
- The CI workflow (`rc-lint.yml`) duplicates the shape of
  `test.yml` and `audit.yml`. A future slice could
  collapse the three workflows into one matrix-driven
  workflow; this ADR does not.
- `git log --grep=feat rc/v1.1` will return zero results
  in the future RC lines. A maintainer searching for "what
  feature landed in v1.2.0" must grep `dev` instead.

## References

- [ADR 0008 — promotion protocol](0008-promotion-protocol.md)
- [ADR 0009 — stable-branch promotion](0009-stable-branch-promotion.md)
- [ADR 0010 — lint enforcement](0010-lint-enforcement.md)
- [AGENTS.md §Three-branch model](../AGENTS.md) — updated to four-branch
- [.github/BRANCH_PROTECTION.md](../../.github/BRANCH_PROTECTION.md)
  — updated with `rc/v*` rules
- `scripts/ci/lint-rc-commits.mjs` — the classifier
- `.github/workflows/rc-lint.yml` — the CI workflow
- Issue #668 — the originating issue
- Issues #658, #660, #661, #665, #666, #667 — the v1.1
  RC1 cohort that surfaced the discipline this ADR codifies

## Author

Jeremy Morris (@jeremymorris) — 2026-07-29
