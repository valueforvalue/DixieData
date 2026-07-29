# Triage Labels

DixieData uses a **7-axis** label taxonomy on every GitHub issue.
Each axis answers a different question; together they let
maintainers filter the backlog by component, urgency, and triage
state without re-reading every issue title.

| Axis | Question | Labels |
|---|---|---|
| **Type** | What's the work? | `bug`, `enhancement`, `documentation`, `reference-doc` |
| **Status** | Where is it in triage? | `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human` |
| **Area** | Which part of the system? | `area:backend`, `area:frontend`, `area:templates`, `area:cli`, `area:share`, `area:tags`, `area:export`, `area:import`, `area:db`, `area:docs`, `area:debug`, `area:build`, `area:ci` |
| **Priority** | How urgent is it? | `priority:high`, `priority:medium`, `priority:low` |
| **Target** | Which branch should the fix land on? | `target:dev`, `target:rc`, `target:stable` |
| **Cohort** | What batch does it belong to? | `audit-fallout` |
| **Meta** | Process state, not work state | `deferred`, `duplicate`, `invalid`, `question`, `good first issue`, `help wanted`, `wontfix`, `safe-for-in-place`, `unsafe-for-in-place`, `release-blocker` |

The full label set with colors + descriptions is defined in
[`scripts/sync-labels.sh`](../../scripts/sync-labels.sh).
Run `./scripts/sync-labels.sh --dry-run` to see what would change;
run `./scripts/sync-labels.sh` to apply.

## Why seven axes

- **Type × Status** distinguishes bugs from features without
  losing triage routing. A bug and an enhancement can both be
  `ready-for-agent`; the agent reads the Type to know which
  template ([`docs/agents/issue-tracker.md`](issue-tracker.md))
  to apply.
- **Area** lets a maintainer filter by component
  (`area:cli`, `area:templates`) without re-reading every issue
  title. New area labels are added via `scripts/sync-labels.sh`
  when a new component emerges (max ~15 to avoid label explosion).
- **Priority** is the urgency signal for the backlog.
  `priority:high` is reserved for known regressions, lost-data
  bugs, and the issues a user is actively blocked on.
  `priority:medium` is the default. `priority:low` is polish.
- **Target** (added 2026-07-26, ADR 0011) is the branch
  routing signal. The repo carries four branches (`dev` /
  `rc/v*` / `stable` / `main`, per AGENTS.md §Four-branch
  model); the Target axis tells the agent + the operator
  which branch the fix should land on. `target:dev` is the
  default for new features + new bugs found during normal
  development. `target:rc` is for stabilization fixes that
  block a release-in-progress (the operator assigns the
  specific `rc/v*` line during triage by commenting
  “target: rc/v1.1”). `target:stable` is rare — it’s for
  urgent hotfixes on the released-code home after a promote.
  Every PR opened against `rc/v*` must carry BOTH a
  `target:rc` label AND the `release-blocker` meta label;
  the rc-lint CI workflow enforces this combination. See
  ADR 0011 for the full policy.
- **Cohort** groups issues that share a discovery context (e.g.
  `audit-fallout` for the 2026-06-24 audit sweep). Lets a
  maintainer filter the audit work without re-reading every
  audit-finding issue.
- **Meta** is process state, not work state. `blocked` is held
  by another issue. `duplicate` points to the canonical issue.
  `question` / `invalid` are triage outcomes, not work states.

## One label per axis

An issue has **exactly one label from each axis** (where the axis
applies). Two `area:*` labels on one issue means the agent doesn't
know which component to load docs for. Zero `area:*` labels means
the issue hasn't been routed.

The exception is `Meta` — an issue can carry multiple `Meta`
labels (`duplicate` + `wontfix` for "this is a dup, also we won't
fix either"). Process labels compose.

## Type — what's the work

| Label | When to apply |
|---|---|
| `bug` | Something isn't working. Includes data loss, crash, regression, and visual regressions. |
| `enhancement` | New feature or non-trivial capability change. |
| `documentation` | Documentation-only change (CHANGELOG entries that are the entire issue, docs typo fixes, ADR updates). |
| `reference-doc` | Long-lived reference document — not active work. The issue body is a maintained checklist or playbook an agent should follow when the documented trigger condition is met (e.g. "add a new canonical status"). Distinct from `documentation` (which is about adding/editing prose) and from `deferred` (which implies a wait for a specific event). Examples: #156 (confederate-home-status extension checklist), #281 (.ddbak format versioning policy), #279 (`dixiedata package` design). Status for these is almost always `needs-triage` is wrong — leave them open indefinitely; the issue IS the doc. |

## Status — the triage labels

The canonical triage roles match the labels in the mattpocock/skills
table from before:

| Role | Label | Meaning |
|---|---|---|
| `needs-triage` | `needs-triage` | Maintainer needs to evaluate this issue |
| `needs-info` | `needs-info` | Waiting on reporter for more information |
| `ready-for-agent` | `ready-for-agent` | Fully specified, ready for an AFK agent |
| `ready-for-human` | `ready-for-human` | Requires human implementation |

For **bugs**, `ready-for-agent` requires the bug template
(Symptom + Repro + Root cause + Files + Regression net) filled in
with a file:line cite for Root cause. See
[`issue-tracker.md`](issue-tracker.md) §Bug protocol.

For **features**, `ready-for-agent` requires the feature template
(User story + Locked decisions + Apply sites + Slice plan) filled
in with at least one apply-site and one testable slice. See
[`feature-protocol.md`](feature-protocol.md) and
[`issue-tracker.md`](issue-tracker.md) §Feature protocol.

## Area — the component labels

The current set, with the docs each one signals an agent to load:

| Label | Signals | Agent loads |
|---|---|---|
| `area:backend` | Go code in `internal/appshell/`, `internal/records/`, etc. | `docs/CODE_CHANGES.md`, `docs/COMMON_BUGS.md` §4 |
| `area:build` | Build pipeline (Makefile, `scripts/`, release pipeline, `versioninfo` codename mechanics) | `docs/RELEASING.md` |
| `area:ci` | GitHub Actions workflows, race gates, CI plumbing | `.github/workflows/` |
| `area:cli` | Headless subcommand in `internal/appshell/cli_*.go` | `docs/agents/cli-plan.md` |
| `area:db` | SQLite schema / migrations / queries | `docs/migrations/` (latest two) |
| `area:debug` | Debug harness, trace instrumentation | `docs/agents/dialog-guard.md`, ADR 0006 |
| `area:docs` | `CONTEXT.md`, `agents/`, `adr/` | This file, `docs/agents/INDEX.md` |
| `area:export` | Export surface (PDF, JPG, JSON, CSV, iCal) | `docs/agents/dialog-guard.md` |
| `area:frontend` | Frontend JS, CSS, htmx, `templates/`, UX/microcopy | `docs/COMMON_BUGS.md` §1-3 |
| `area:import` | Import surface (.ddbak, .ddshare, images) | `docs/agents/cli-plan.md` §Phase 5 |
| `area:share` | `/share` screen + subpages | `docs/ui-map/wireframes/08-export.md` |
| `area:tags` | Tag system (Person Record free-text labels) | Issue #183 |
| `area:templates` | Templ HTML + Typst PDF templates | `docs/agents/tune-iteration.md` |

### Adding a new area label

Add it to `scripts/sync-labels.sh` and run the script. The label
set is intentionally limited; new components should be infrequent.
If you find yourself wanting more than ~15 area labels, the
taxonomy probably needs to be split by sub-area
(`area:cli-export` instead of generic `area:cli`).

## Priority — the urgency labels

| Label | When to apply |
|---|---|
| `priority:high` | Known regression, lost-data bug, active user blocker. Examples: a crash on a common flow, a data-import path that drops records, an exported file that fails the smoke probe. |
| `priority:medium` | Default. Bugs that have a workaround, features that are on the backlog, polish that's worth doing. |
| `priority:low` | Speculative, nice-to-have, or research-only. Won't get agent attention unless the user requests it. |

`priority:high` issues should be in the current sprint.
`priority:medium` issues are the backlog. `priority:low` issues
are tracked but not actively worked.

## Target — which branch the fix should land on

| Label | When to apply |
|---|---|
| `target:dev` | New feature or bug found during normal development. Lands on `dev` for the next release. The default. |
| `target:rc` | Stabilization fix blocking a release-in-progress. The PR opens against the active `rc/v*` branch. Operator adds a comment with the specific `rc/vN.M` line (e.g. “target: rc/v1.1”) so the agent knows which RC branch to base the PR on. |
| `target:stable` | Urgent hotfix on the released-code home after a promote. Rare; the regression slipped past `make promote` and the fix can't wait for the next release. Requires a maintainer's review. |

The Target axis was added 2026-07-26 as part of [ADR 0011](../../adr/0011-rc-branch-policy.md).
The reason it exists: the repo carries four branches
(`dev` / `rc/v*` / `stable` / `main`) with very different
acceptance policies. Without an explicit target signal, an
agent reading the issue body has to guess which branch the
fix should land on — and the guess is often wrong (the
v1.1 RC1 cohort shipped 8 RCs that mixed real fixes with
new features because there was no Target axis to tell
“this is an RC blocker, not a new feature”).

**Rule:** every issue that becomes a PR must carry a Target
label before merge. A PR opened without one gets a
`needs-info` label and a bot comment asking the contributor
to set the target. The PR's `target:*` label MUST match the
PR's base branch (a `target:rc` PR opening against `dev`
gets the same `needs-info` flag).

**PR-to-RC special case:** every PR to `rc/v*` must carry
BOTH `target:rc` AND the `release-blocker` meta label. The
`lint-rc-commits` CI workflow enforces this combination on
top of the commit-message + diff-size policy. See ADR 0011
§Decision 3 for the rationale.

## Cohort — the batch labels

| Label | When to apply |
|---|---|
| `audit-fallout` | Issue discovered by a full audit sweep. The audit date + the sweep ID live in the issue body. |

The cohort axis exists so a maintainer can filter the audit
follow-up work without re-reading every audit-finding issue.
New cohort labels are rare — created when a new sweep produces
a backlog of related issues.

## Meta — the process labels

| Label | When to apply |
|---|---|
| `deferred` | Held until a future trigger fires. Issue body must document the reopen conditions (what event would un-defer this?). Distinct from `wontfix` (decision to never action). |
| `duplicate` | This issue already exists. Comment with a link to the canonical issue. |
| `good first issue` | Small enough for a newcomer to pick up. Maintainer-curated. |
| `help wanted` | Maintainer is actively looking for someone to pick this up. |
| `invalid` | Not a real issue (test post, spam, off-topic). |
| `question` | Reporter is asking, not filing. Convert to enhancement/bug if it becomes a real issue. |
| `wontfix` | Decision to not action. Must include reasoning in a comment. |

## In-place update safety (build-protocol.md §5)

| Label | When to apply |
|---|---|
| `safe-for-in-place` | Reviewed against the 4 safety rules; safe for in-place update on `main`. |
| `unsafe-for-in-place` | Intentional destructive change; ships via full re-install + restore-point only. |

## Release-process exemptions

| Label | When to apply |
|---|---|
| `release-counter-exempt` | PR may skip the `CurrentAppVersionInt` bump gate. Used rarely — pure-docs releases advancing N for tracking. The N-bump CI step (`release-counter-bumped` in `.github/workflows/test.yml`) auto-allows PRs carrying this label. Apply with a one-line comment justifying the exception. See `docs/RELEASING.md` §"N bump semantics". |

## References

- [`docs/agents/issue-tracker.md`](issue-tracker.md) §Label
  taxonomy — the per-section rationale
- [`scripts/sync-labels.sh`](../../scripts/sync-labels.sh) —
  the canonical label spec (idempotent; safe to re-run)
- [`scripts/backfill-labels.sh`](../../scripts/backfill-labels.sh) —
  applies area + priority labels to existing open issues