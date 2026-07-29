# Branch protection (main + stable + rc/v*)

Per [ADR 0009](../docs/adr/0009-stable-branch-promotion.md),
both `main` and `stable` get the **same** standard GitHub
branch protection rules. `dev` is intentionally unprotected.

Per [ADR 0011](../docs/adr/0011-rc-branch-policy.md), `rc/v*`
branches carry the standard rules plus the `release-blocker`
label gate + the `lint-rc-commits` status check (rejecting
`feat/refactor/perf/build` commit types + diffs ≥ 50 files
on `rc/v*` PRs).

## Rules (apply to BOTH `main` and `stable`)

| Rule | Setting |
|---|---|
| Require a pull request before merging | ON |
| Require approvals | 0 (operator is the merge authority) |
| Dismiss stale pull request approvals when new commits are pushed | OFF |
| Require review from Code Owners | OFF |
| Require status checks to pass before merging | ON |
| Require branches to be up to date before merging | ON |
| Require conversation resolution before merging | OFF |
| Require signed commits | OFF (matches current flow) |
| Require linear history | OFF (merge commits are fine) |
| Require deploy queue to succeed before merging | OFF (n/a) |
| Restrict pushes that create matching branches | OFF |
| Allow force pushes | OFF |
| Allow deletions | OFF |
| Block in the admin namespace | OFF (operators are admins) |
| Allow specified actors to bypass required pull requests | OFF |

## Status checks (both `main` and `stable`)

The following checks must pass before merge:

- `audit` (from `.github/workflows/audit.yml`)
- `build` (from `.github/workflows/build.yml`)
- `test` (from `.github/workflows/test.yml`)

These are the three workflows that listen to `push` and
`pull_request` on `[dev, stable]`. The check names appear
in the GitHub UI as the `Job name` for each workflow.

## Applying the rules

The rules are stored in the repo's branch protection
settings. They cannot be applied via code (the GitHub
BranchProtection API is a write API, not a config file).
The operator applies them once via the GitHub UI:

1. Open `https://github.com/valueforvalue/DixieData/settings/branches`.
2. Click **Add rule** for `main` (and separately, for `stable`).
3. Paste the rules from the table above.
4. Under **Status checks**, search for `audit`, `build`,
   `test` and select each one.
5. Click **Create** (or **Save changes**).
6. Repeat for `stable`.

## Verifying the rules

The CI workflow on this PR (and the next one) verifies the
rules are present:

```bash
gh api repos/valueforvalue/DixieData/branches/main/protection | jq '.required_status_checks.contexts, .allow_force_pushes.enabled, .allow_deletion.enabled'
# Expected: ["audit","build","test"], false, false

gh api repos/valueforvalue/DixieData/branches/stable/protection | jq '.required_status_checks.contexts, .allow_force_pushes.enabled, .allow_deletion.enabled'
# Expected: ["audit","build","test"], false, false
```

If either check fails (e.g. `null` for `allow_force_pushes`),
the protection has not been applied. Apply via the UI per
the steps above.

## Why both branches, symmetric rules

Per ADR 0009 §"Branch protection (both `main` and `stable`)"
the rules are symmetric by design. `main` is protected so
the "frozen legacy" rule is enforced by GitHub, not just by
docs; `stable` is protected so the "released code home" rule
is enforced the same way. Both branches look identical to a
contributor trying to push directly: the push is rejected
with a redirect to a PR.

## Why `dev` is NOT protected

AGENTS.md §Branch policy documents direct commits to `dev`
as the default flow for agents and humans. Adding branch
protection to `dev` would break that flow. The integration
branch is where most work lands; forcing every commit
through a PR would slow the loop without adding meaningful
review (the review happens at the `dev → stable` promotion
gate).

## References

- [ADR 0009](../docs/adr/0009-stable-branch-promotion.md) —
  the policy this file implements
- [ADR 0008](../docs/adr/0008-promotion-protocol.md) — the
  `dev → stable` promotion chain the protection enables
- [AGENTS.md §Branch policy](../AGENTS.md#branch-policy) —
  the day-to-day rules for humans + agents