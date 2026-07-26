# RC branch policy: feature freeze on rc/v* branches (Zephyr-style)

## Status

Accepted 2026-07-26. Codifies the policy the v1.1 RC1 cohort surfaced as
needed: the release-candidate line must receive stabilization fixes only,
not new features.

## Context

The v1.1 RC1 cycle (issue #658 + the cohort it spawned: #660, #661, #663,
#664, #665, #666, #667) shipped **eight** RCs in quick succession, each
one mixing a real bug fix with a brand-new feature:

| RC | Real fix it shipped | New feature it also shipped |
|---|---|---|
| 1.1.5 | theme persistence | debug-mode toggle UI |
| 1.1.6 | version compare rejected RC1 | Check for Updates button visible |
| 1.1.10 | debug-mode logging | shell theme default |
| 1.1.13 | tag-delete form | memorial / config / progress bar / PDF linebreak |
| 1.1.14 | tag-delete dispatcher | (no new features — first clean RC) |
| 1.1.15 | check-for-updates polling guard | (no new features) |
| 1.1.16 | article delete affordance | (no new features) |
| 1.1.17 | (no fix — clean RC) | `--seed` in-app flag + seed-fixture.ps1 |

The middle three RCs are the model: each one closes a specific bug from
the cohort and lands zero new surface. That's the policy the user wants
codified — and the early RCs are the cautionary tale of what happens
when a release line accepts features in the middle of stabilization.

Three established engineering practices informed this decision:

1. **Zephyr Project release process** (`docs.zephyrproject.org/latest/project/release_process.html`):
   declares a *feature freeze* the moment the first RC is tagged. Only
   "stabilization-related changes" (bug fixes, doc updates, tests for
   existing functionality) are accepted on the RC branch. New features
   require an explicit TSC exception. Their discipline is the model
   this ADR copies.

2. **VisIt RC development** (`visit-sphinx-github-user-manual.readthedocs.io/en/3.4rc/dev_manual/RCDevelopment.html`):
   maintains a long-lived `3.3RC` branch in parallel with `develop`,
   requires every RC PR to be followed by a separate PR applying the
   same change to `develop`, and explicitly disallows "Changes to files
   impacting communication protocols or public APIs" without team
   approval. Their workflow is the model for the backport direction
   (RC → dev).

3. **Gitflow release branches** (per the Stack Exchange consensus in
   `softwareengineering.stackexchange.com/q/432957`): the classic
   gitflow answer to "how do I fix a bug on the RC line without
   dragging in main-line features" is to branch the fix off the RC
   branch, merge it back to the RC, then merge the same commit into
   the integration branch. Cherry-picking is a code smell because it
   duplicates commit identity.

The gap this ADR closes: DixieData's current `AGENTS.md` says direct
commits to `dev` are the default. There is no rule for what an RC
branch accepts. The cohort just shipped an RC line that alternated
"real fix + new feature" because the rule didn't exist.

## Decision

Adopt a **hard (Zephyr-style) feature freeze** on `rc/v*` branches. The
policy is encoded in three places, with enforcement at the layer that
catches it earliest:

### 1. Branch naming

- `rc/v1.1` — created from `dev` at RC1 cut, receives the next
  release's stabilization fixes.
- Future RC lines: `rc/v1.2`, `rc/v2.0`, etc.
- A long-lived `rc/v*` branch exists for every active release line.
  It is **not** deleted when the release ships — it stays as the
  patch-release maintenance line (e.g. `v1.1.1`, `v1.1.2` cherry-picks
  come from `rc/v1.1`).

### 2. Commit types allowed on `rc/v*`

| Type | Allowed? | Why |
|---|---|---|
| `fix:` | ✓ | Bug fix — the entire point of the RC line |
| `docs:` | ✓ | Doc clarifications + corrections |
| `test:` | ✓ | Regression net for existing features |
| `ci:` | ✓ | Build / CI fixes (broken workflows block release) |
| `chore:` | ✓ | Regression net for a fix (must reference the fix) |
| `feat:` | ✗ | New feature — wait for `dev` to merge into next release |
| `refactor:` | ✗ | Refactor not tied to a bug fix — wait for `dev` |
| `perf:` | ✗ | Perf improvement — wait for `dev` |
| `build:` | ✗ | Build-system change — wait for `dev` |

A commit that touches ≥ 50 files is **always** rejected, regardless
of type — large diffs are the canonical signal of a refactor or
feature masquerading as a fix.

### 3. PR requirements

- **Required label**: every PR to `rc/v*` must have a `release-blocker`
  label (or `bug` + `ready-for-agent`, see below). The label signals
  "this is a real fix to a problem the cohort surfaced."
- **Required reviewers**: 2 maintainer approvals (matches the
  existing `stable` and `main` rules per ADR 0009).
- **Required CI**: `lint-rc-commits` job must pass (the
  commit-message gate described below).
- **No direct push**: branch protection mirrors `stable` + `main`.

### 4. Commit-message gate (CI)

A new `scripts/ci/lint-rc-commits.mjs` script runs in CI on every PR
targeting `rc/v*`. It walks the PR's commit list, parses the
`type:` prefix out of each subject line, and fails the job if any
commit's type is in the disallowed list. The allowed types are
defined in the script (no env-var indirection — the rule is the rule).

This catches the failure mode in the table above: a contributor who
opens a PR to `rc/v1.1` with subject `feat: add per-row tag delete
affordance` gets a red CI status before any human review happens.

### 5. Sync direction: RC → dev (VisIt pattern)

Fixes land on `rc/v*` first, then get **merged** (not cherry-picked,
per the gitflow consensus) into `dev` as a follow-up commit. The
follow-up commit's message references the RC commit by SHA so the
audit trail is complete:

```
fix(tags): add data-method=DELETE to tag delete form (#664)

Backport of rc/v1.1 commit 8d7989c4 to dev.
```

The opposite direction (dev → RC) is allowed for the special case
where a fix is developed on `dev` first and then backported to the
RC line. The PR description must say "backport from dev" and link
the original `dev` commit SHA. This is the Zephyr "long term
enhancements are performed only on the develop branch" exception
applied in reverse.

### 6. Migration to `stable`

The promotion path (RC → stable → release tag) is unchanged from
ADR 0008 / ADR 0009. The only addition: `make promote-dry-run`
must pass against `rc/v*` HEAD before the promotion PR opens. The
promotion PR is the moment a release becomes "always releasable";
the RC branch continues to receive fixes if a regression is found
post-promotion, and those fixes trigger a follow-up promotion.

## Consequences

### Easier

- The RC cohort gets one job: close real bugs. No more "is this
  feature ready, can we sneak it into RC2?" decisions.
- `dev` keeps accepting features at full velocity. The RC line
  doesn't slow `dev` down; the two branches have orthogonal
  policies.
- Audit trail is clean: every commit on `rc/v*` has a
  release-blocker label, a 2-reviewer approval, and a backport
  commit on `dev`. The next engineer can grep `git log rc/v1.1
  --grep='^fix:'` to see the complete stabilization history.
- Release quality improves because the RC line is small
  (only `fix:` commits) and predictable. The cohort's 8-RC cycle
  would have been 3-4 RCs under this policy.

### Harder

- Two commits per fix (RC + dev) instead of one. The backport
  step adds ~10 minutes per fix. For the v1.1 cohort at 8 RCs
  with ~5 fixes each, that's ~6 hours of extra ceremony.
- The `lint-rc-commits` CI job must be added to the workflow
  before the rule takes effect. The script is small but it's a
  new failure mode for contributors to learn.
- The 50-file diff cap is a blunt instrument. A genuine 60-file
  bug fix would be rejected. The escape hatch is the same as
  the `feat:` exception: a maintainer can override the CI
  check with a comment explaining the scope.

### Locked in

- The RC branch becomes a "no new features" zone permanently.
  This is the trade-off Zephyr made and they have not regretted
  it. If DixieData ever needs a new feature on an RC line, the
  answer is "no, wait for the next release" — not "make an
  exception."
- The sync direction is RC → dev. The opposite direction
  (dev → RC) requires explicit justification per PR. This
  prevents the "fix on dev first, cherry-pick to RC" pattern
  that produces duplicate commit identity and confused
  bisects.

## References

- [Three-branch model](0009-stable-branch-promotion.md) — the
  existing `dev` / `stable` / `main` model this ADR extends with
  a fourth `rc/v*` layer.
- [Promotion protocol](0008-promotion-protocol.md) — the
  `make promote` / `make promote-dry-run` gate chain that
  migrates RC HEAD to `stable`.
- Zephyr Project release process:
  <https://docs.zephyrproject.org/latest/project/release_process.html>
  — the source of the allowed/disallowed types table and the
  feature-freeze language.
- VisIt RC development:
  <https://visit-sphinx-github-user-manual.readthedocs.io/en/3.4rc/dev_manual/RCDevelopment.html>
  — the source of the RC → dev backport discipline.
- Stack Exchange `q/432957` — the gitflow consensus that
  branching off the RC and merging back is preferable to
  cherry-picking.
- Issue #658 — the RC1 cohort that exposed the policy gap.

## Author

Jeremy Morris (@jeremymorris) — 2026-07-26
