# Triage Labels

DixieData uses a 6-axis label taxonomy on every GitHub issue.
Each axis answers a different question; together they let
maintainers filter the backlog by component, urgency, and triage
state without re-reading every issue title.

| Axis | Question | Labels |
|---|---|---|
| **Type** | What's the work? | `bug`, `enhancement`, `documentation` |
| **Status** | Where is it in triage? | `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix` |
| **Area** | Which part of the system? | `area:backend`, `area:frontend`, `area:templates`, `area:cli`, `area:share`, `area:tags`, `area:export`, `area:import`, `area:db`, `area:docs`, `area:debug` |
| **Priority** | How urgent is it? | `priority:high`, `priority:medium`, `priority:low` |
| **Cohort** | What batch does it belong to? | `audit-fallout` |
| **Meta** | Process state, not work state | `blocked`, `deferred`, `duplicate`, `invalid`, `question`, `good first issue`, `help wanted`, `wontfix` |

The full label set with colors + descriptions is defined in
[`scripts/sync-labels.sh`](../../scripts/sync-labels.sh).
Run `./scripts/sync-labels.sh --dry-run` to see what would change;
run `./scripts/sync-labels.sh` to apply.

## Why six axes

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

## Status — the triage labels

The canonical triage roles match the labels in the mattpocock/skills
table from before:

| Role | Label | Meaning |
|---|---|---|
| `needs-triage` | `needs-triage` | Maintainer needs to evaluate this issue |
| `needs-info` | `needs-info` | Waiting on reporter for more information |
| `ready-for-agent` | `ready-for-agent` | Fully specified, ready for an AFK agent |
| `ready-for-human` | `ready-for-human` | Requires human implementation |
| `wontfix` | `wontfix` | Will not be actioned |

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
| `area:cli` | Headless subcommand in `internal/appshell/cli_*.go` | `docs/agents/cli-plan.md` |
| `area:db` | SQLite schema / migrations / queries | `docs/migrations/` (latest two) |
| `area:debug` | Debug harness, trace instrumentation | `docs/agents/dialog-guard.md`, ADR 0006 |
| `area:docs` | `CONTEXT.md`, `agents/`, `adr/`, `migrations/` | This file, `docs/agents/INDEX.md` |
| `area:export` | Export surface (PDF, JPG, JSON, CSV, iCal) | `docs/agents/dialog-guard.md` |
| `area:frontend` | Frontend JS, CSS, htmx, `templates/` | `docs/COMMON_BUGS.md` §1-3 |
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
| `blocked` | Held by another issue. Comment on the issue with a link to the blocker. |
| `deferred` | Held until a future trigger fires. Issue body must document the reopen conditions (what event would un-defer this?). Distinct from `wontfix` (decision to never action) and `blocked` (held by a specific in-flight issue). |
| `duplicate` | This issue already exists. Comment with a link to the canonical issue. |
| `good first issue` | Small enough for a newcomer to pick up. Maintainer-curated. |
| `help wanted` | Maintainer is actively looking for someone to pick this up. |
| `invalid` | Not a real issue (test post, spam, off-topic). |
| `question` | Reporter is asking, not filing. Convert to enhancement/bug if it becomes a real issue. |
| `wontfix` | Decision to not action. Must include reasoning in a comment. |

## References

- [`docs/agents/issue-tracker.md`](issue-tracker.md) §Label
  taxonomy — the per-section rationale
- [`scripts/sync-labels.sh`](../../scripts/sync-labels.sh) —
  the canonical label spec (idempotent; safe to re-run)
- [`scripts/backfill-labels.sh`](../../scripts/backfill-labels.sh) —
  applies area + priority labels to existing open issues