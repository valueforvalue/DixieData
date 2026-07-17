# Stable branch as released-code home; main frozen as legacy production

## Status

Accepted 2026-07-03. Introduces a third named branch (`stable`)
as the destination for `dev → released-code` promotion, and
freezes `main` at its current HEAD (`56e31f0`) as the legacy
production record.

## Context

The current state at the time of this ADR:

- `main` sits at commit `56e31f0` (the version actively in
  production use by users).
- `dev` carries ongoing integration work (e.g. the
  schema-downgrade work shipped in PRs #298, #299, #300).
- ADR 0008 documents the promotion chain as `dev → main`,
  gated by `make promote`.
- AGENTS.md §Branch policy says "`main` is always stable," but
  the name `main` collides in conversation with Go's `main`
  package (`cmd/dixiedata/main.go`); the docs conflate the two
  regularly.
- There is no current name that distinguishes "what users have
  right now" from "what we're shipping next."

Three problems this creates:

1. **No clear home for releases.** Once the schema-downgrade
   work and any subsequent dev commits land, "what gets
   shipped" is ambiguous between `main` (frozen, in production)
   and `dev` (where new work lands).
2. **No path to flip the default branch.** The natural moment
   to flip the GitHub default branch from `main` to something
   else is when that something else becomes the actual
   production version. Today, there is no such ref.
3. **`main` collides with the Go entry-point vocabulary.**
   The audit at `docs/agents/cli-plan.md` and the PR template
   refer to "main" as both a branch and a function; the
   collision is benign today but grows worse as the codebase
   does.

## Decision

Introduce `stable` as the released-code branch. Freeze `main` at
its current HEAD. Update the promotion chain documented in ADR
0008 from `dev → main` to `dev → stable`.

### Branch protection (both `main` and `stable`)

Both `main` and `stable` get the **same** standard GitHub branch
protection rules:

- No direct pushes (require a PR).
- No force-pushes.
- No branch deletion.
- Require CI green (`build`, `test`, `audit` workflows) before
  merge.

This is symmetric by design. `main` is protected so the
'frozen legacy' rule is enforced by GitHub, not just by docs;
`stable` is protected so the 'released code home' rule is
enforced the same way. Both branches look identical to a
contributor trying to push directly: the push is rejected with
a redirect to a PR.

`dev` is **not** protected. AGENTS.md §Branch policy already
documents direct commits to `dev` as the default flow; adding
protection would break the agent + human commit pattern that
ships most work.

This is a change from the prior state (`main` was unprotected
before this ADR). The protection is added in the same PR that
introduces `stable` so the three-branch model ships as a
coherent unit. The protection rule is documented in
`.github/BRANCH_PROTECTION.md` (a new file in this PR) so
future agents have a checklist to apply the rules via `gh api`
or the GitHub UI.

### The chain

```
dev          — integration branch (unchanged)
  │
  │  make promote (gate chain per ADR 0008)
  ▼
stable       — released-code branch (NEW; promotion destination)
  │
  │  tag (v{MAJOR}.{U}.{N}), release draft, release-github.ps1
  ▼
GitHub release artifact (consumed by in-place update flow)
```

`main` is **not** in this chain. It is a frozen legacy
record — the commit at which this ADR was written (`56e31f0`)
is its last commit. It accepts no new work, ever.

### Why `main` stays in place (not deleted)

- The commit graph is unchanged. Every tag, every fork, every
  clone that pins `origin/main` keeps working.
- `main` becomes the immutable audit anchor for "what users had
  on 2026-07-03." Future agents that need to reconstruct the
  production state of the app at that date point at `main`.
- Deletion would force every fork + every CI matrix to update
  before they could fetch. The cost is not justified: the freeze
  achieves the goal (no new work) without the destruction.

### Branch protection

`main` and `stable` both get GitHub branch protection:

- No direct pushes (require a PR).
- Require CI green before merge.
- No force-pushes.
- No branch deletion.

`dev` is **not** protected. AGENTS.md §Branch policy already
documents direct commits to `dev` as the default flow; adding
protection would break the agent + human commit pattern that
ships most work.

### When does the GitHub default branch flip from `main` to `stable`?

**Not as part of this ADR.** The default branch flips in a
future, separate change once `stable` carries the actual
production version — i.e. after the first `make promote` lands
a release on `stable` AND that release has been in users'
hands long enough to verify it. The user's instruction (issue
follow-up, 2026-07-03): "When we pull the changes from dev
into stable then itll be flipped to the default."

This makes the default-branch flip a one-way decision driven by
operational evidence (a release has shipped from `stable` and
proven itself), not a documentation flip tied to this ADR.

### How `dev → stable` differs from `dev → main`

The gate chain in ADR 0008 (the `make promote` target, gates
1-9) is unchanged. Only the destination branch changes. The
user-driven path documented in ADR 0008 §"When does promotion
happen?" (path a) becomes:

```bash
git checkout stable
git merge dev
git tag v1.1.{N} && git push --tags
scripts/release-github.ps1
```

(Replacing `main` with `stable` in the ADR 0008
example. Everything else is identical.)

### The promote flow (PR via GitHub UI)

The user picks **PR via GitHub UI** as the promotion flow
(2026-07-03 issue follow-up). The chain is:

1. **Operator runs `make promote-dry-run`** — runs the gate
   chain (gates 1-9 from ADR 0008) and prints the result.
   No push, no tag, no PR. The operator reads the output to
   decide whether to proceed.
2. **Operator runs `make promote`** — runs the gate chain
   (halts on failure). On success, it opens a PR
   `dev → stable` via `gh pr create` with the gate-chain
   output embedded in the PR body. The PR title is
   `promote: dev → stable (v{MAJOR}.{U}.{N})` where the
   version comes from `versioninfo.go`. No code is merged
   yet; the PR is the proposal.
3. **Operator reviews the PR in the GitHub UI** — reads the
   diff, the gate-chain output, the commit log
   (`git log <last-tag>..HEAD --oneline`), and any CI
   annotations. The operator is the merge authority; no
   auto-merge is configured.
4. **Operator merges the PR via the GitHub UI** — the merge
   button is the promotion. CI re-runs as part of the merge
   branch protection rules (require CI green before merge).
5. **Operator runs `scripts/release-github.ps1`** — tags the
   merge commit as `v{MAJOR}.{U}.{N}`, pushes the tag,
   opens a draft GitHub release. The release artifact is
   what users get via the in-place update flow.

### Conflict policy (dev diverges from stable)

The user picks **merge dev into stable** (2026-07-03 issue
follow-up). When `dev` has commits `stable` doesn't have:

- `make promote` aborts with a clear message: "dev has
  commits stable doesn't have. Run `make promote-prep` to
  sync." The dry-run variant (`make promote-dry-run`)
  detects the same divergence and prints it.
- `make promote-prep` runs `git fetch origin`, then
  `git checkout stable && git pull origin stable`, then
  `git checkout dev && git pull origin dev`, then prints
  the divergence as a `git log <stable>..<dev> --oneline`
  summary. The operator reads the summary and decides.
- If the divergence is **non-conflicting** (e.g. dev added
  new files that stable doesn't touch), the operator runs
  `git checkout stable && git merge --no-ff origin/dev` and
  resolves any conflicts locally, then re-runs
  `make promote` to open the PR.
- If the divergence is **conflicting** (e.g. the same line
  was edited on both branches), the operator resolves
  conflicts locally per ADR 0008 §"Pre-promotion
  checklist." The resolution is committed to `stable`
  directly via a hot-fix PR (not via `make promote`); then
  `make promote` runs to open the standard promotion PR
  with the conflict already resolved.

Cherry-pick and rebase were considered (see
§"Alternatives considered"). Merge wins because it matches
the existing `dev → main` flow that operators are familiar
with, and it preserves the commit graph (no rewrites).

## Alternatives considered

### Alt 1: Rename `main` → `stable`

`git branch -m main stable` locally + force-push delete remote
`main`. Rejected because:

- Destructive: every existing fork + clone breaks until they
  fetch + sync.
- The "main" name is meaningful as the production-history
  anchor; losing it loses the audit trail for "what users had
  on 2026-07-03."
- The freeze achieves the same goal (no new work) without
  forcing every downstream consumer to update.

### Alt 2: Delete `main` entirely after `stable` is the default

The same destructiveness as Alt 1, plus a second decision
point (when is `stable` ready to be the default?). Rejected
for the same reasons; the freeze is sufficient.

### Alt 3: Keep the two-branch flow (`dev` + `main`); no `stable`

Status quo. Rejected because the GitHub-default-branch flip
becomes impossible without data loss (Alt 1 is required to
move "main" to a different name, and Alt 1 is rejected).

## Consequences

### Positive

- The promotion chain has a destination name (`stable`) that
  does not collide with Go's `main` package. Future docs,
  scripts, and CLI output can reference the branch by its
  actual purpose.
- The GitHub default branch can flip from `main` to `stable`
  in a future, planned PR (one-line change in repo settings).
- `main` is preserved as the immutable production-history
  anchor. Future agents that need to know "what was the app
  state on 2026-07-03" point at `main` HEAD = `56e31f0`.
- Branch protection on `main` and `stable` is symmetric.
  Both are read-only-ish from the agent's perspective.
- The CI workflows (test, build, audit) update from
  `branches: [dev, main]` to `branches: [dev, stable]` in the
  same PR; the change is one-line per file.

### Negative

- Three named branches is more surface than two. Future
  contributors have to learn which branch accepts what.
  Mitigation: AGENTS.md §Branch policy is amended to spell out
  the three roles; the new wording is in this PR.
- The PULL_REQUEST_TEMPLATE safety-label warning fires for PRs
  to `stable`, not `main`. Anyone with muscle memory for
  "PR to main" gets a moment of friction. Mitigation: the
  template change is part of this PR; the warning text is
  explicit about the rename.
- `main` stays in the branch list forever. Branch listings
  show three entries (`dev`, `main`, `stable`) instead of two.
  Acceptable cost for the audit anchor.

### Compatibility

- Existing forks that pin `origin/main` keep working. The
  branch is frozen (no new commits) but the ref exists, the
  commit graph is intact, and all tags still resolve.
- Existing CI matrices that listen to `push to main` get
  switched to `push to stable` in the workflow files. The
  push triggers themselves are unchanged; only the branch
  name in the trigger filter changes.
- `scripts/release-github.ps1` is unchanged in behavior. The
  inline comments + the docstring at the top of the script
  get updated to say "stable" instead of "main." No call
  sites change.
- ADR 0008 is not modified. Its gate chain is unchanged; only
  the destination branch changes. ADR 0008 §"Implementation
  notes" gets a one-line cross-reference to this ADR at the
  top.

## Implementation notes

### Files this ADR will touch

- `AGENTS.md` §Branch policy — rewrite "Promotion: dev → main"
  to "Promotion: dev → stable; main is legacy freeze."
- `CONTEXT.md` §Laws — add the line "Released code lands on
  `stable`. `main` is frozen at `56e31f0`."
- `CHANGELOG.md` [Unreleased] — Maintenance entry.
- `docs/RELEASING.md` §Release workflow — replace "merge to
  `main`" with "merge to `stable`." Add the promote-flow
  steps (`make promote-dry-run`, `make promote`, PR review,
  merge via UI, `scripts/release-github.ps1`).
- `docs/adr/0008-promotion-protocol.md` — single-line
  cross-reference at the top of §"Implementation notes":
  "**As of 2026-07-03**, the destination branch is `stable`,
  not `main`. See ADR 0009."
- `docs/adr/0009-stable-branch-promotion.md` — this file.
- `.github/PULL_REQUEST_TEMPLATE.md` — update "every PR to
  `dev` or `main`" to "every PR to `dev` or `stable`."
- `.github/workflows/test.yml` — replace `branches: [dev, main]`
  with `branches: [dev, stable]` in `push:` and
  `pull_request:` triggers. Move the "Stern warning if PR
  targets main without in-place safety label" check to a
  check for "PR targets stable."
- `.github/workflows/build.yml` — same `branches:` change.
- `.github/workflows/audit.yml` — same.
- `.github/BRANCH_PROTECTION.md` — **new file**. Documents
  the standard protection rules applied to both `main` and
  `stable` so future agents have a checklist to apply them
  via `gh api` or the GitHub UI.
- `Makefile` — add `make promote`, `make promote-dry-run`,
  `make promote-prep` targets per ADR 0008 §"The promotion
  command." Targets call into the existing gate chain
  (`release-pipeline` minus the final `release-github` gate)
  and open the PR via `gh pr create`. Add `STABLE_BRANCH
  ?= stable` constant for any future code that needs the
  destination name.
- `scripts/release-github.ps1` — comments + docstring only;
  no behavior change. Update inline references from "main" to
  "stable" so future maintainers see the correct destination.
- `scripts/promote-prep.sh` — **new file**. Implements the
  `make promote-prep` step: fetches origin, prints the
  divergence between `dev` and `stable`, and instructs the
  operator on conflict resolution per the conflict policy
  above.

### Branch operations

- `git push origin main:stable` — creates `stable` from
  `main` HEAD (`56e31f0`). Same commit; no rewrite.
- `git branch -d old-main 2>/dev/null || true` — no-op; we
  keep `main`.
- GitHub branch protection rules — set in repo settings
  (Settings → Branches → Add rule for `main` and `stable`).
  Manual step; PR body links to the settings page.

### Open questions

1. **First `make promote` run.** When the first promotion
   from `dev` to `stable` lands, it will produce a release
   that is not the version in production (that's still
   `56e31f0`). The release notes for that first promotion
   must say so explicitly. Tracked as a follow-up to the
   first promote run.
2. **GitHub default branch flip.** When does it happen? Per
   the user's instruction: "When we pull the changes from dev
   into stable then itll be flipped to the default." That
   sentence implies the flip happens after the first
   `dev → stable` promotion. The flip itself is a one-line
   repo settings change + a one-line update to the README
   badges. Tracked as a follow-up to the first promote run.
3. **Cadence-driven promotion.** Path (b) in ADR 0008 §"When
   does promotion happen?" — after every schema bump, a CI
   job opens a draft promotion PR. Unchanged; out of scope for
   this ADR.

### Regression net

- All existing tests + CI workflows continue to pass after the
  workflow files are updated. The change is a one-line
  `branches:` filter update per workflow.
- The PULL_REQUEST_TEMPLATE safety-label warning continues to
  fire; the target branch name in the warning text changes.
- Existing forks that pin `origin/main` continue to work.
  After the next sync, the rename is visible as a `stale`
  remote-branch + `new branch: stable` in `git fetch` output.

## References

- [`docs/adr/0008-promotion-protocol.md`](0008-promotion-protocol.md)
  — the gate chain this ADR composes with. ADR 0008 §Implementation
  notes gets a one-line cross-reference to this ADR.
- [`docs/adr/0007-in-place-update-safety.md`](0007-in-place-update-safety.md)
  — the four rules the promotion gate enforces. Unchanged.
- [`docs/adr/0001-in-place-update-restore-points.md`](0001-in-place-update-restore-points.md)
  — the binary-rollback contract. Unchanged.
- [`AGENTS.md`](../../AGENTS.md) §Branch policy — the maintainer
  rule this ADR codifies.
- [`docs/RELEASING.md`](../RELEASING.md) — release mechanics
  this ADR composes with.
- [`CONTEXT.md`](../../CONTEXT.md) §Laws — cross-cutting
  context that every release ships under.
- Issue #273 — schema-downgrade (shipped in PRs #298, #299,
  #300). The first wave of work that will land on `dev` and
  become the first `dev → stable` promotion.
- Commit `56e31f0` — the current `main` HEAD. Frozen by this
  ADR as the legacy production record.

## Author

Jeremy Morris (@jeremymorris) — backfilled 2026-07-17
