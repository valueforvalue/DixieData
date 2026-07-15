# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues. Use the `gh` CLI for all operations.

## Conventions

- **Create an issue**: `gh issue create --title "..." --body "..."`. Use a heredoc for multi-line bodies.
- **Read an issue**: `gh issue view <number> --comments`, filtering comments by `jq` and also fetching labels.
- **List issues**: `gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'` with appropriate `--label` and `--state` filters.
- **Comment on an issue**: `gh issue comment <number> --body "..."`
- **Apply / remove labels**: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **Close**: `gh issue close <number> --comment "..."`

Infer the repo from `git remote -v` - `gh` does this automatically when run inside a clone.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill says "fetch the relevant ticket"

Run `gh issue view <number> --comments`.

## Bug protocol

Every bug must be filed as a GitHub issue with the RPCI
research attached in the body. The issue is the durable
record; the chat is not. A bug filed without research
attached is incomplete and will be bounced back for
investigation.

The acronym: **RPCI** = Research, Plan, Critique, Implement.
Full procedure in `docs/agents/rpci.md`. This section covers
just the **R (Research) phase** that the issue must contain.

### Required sections in the issue body

Use this template when filing a bug. Replace placeholders;
do not ship an issue with `<!-- ... -->` comments still in
the body.

```markdown
## Symptom
<What the user sees. Verbatim error text if any. One
paragraph, no solutions, no speculation about cause.>

## Repro
<Numbered steps a maintainer can follow to reproduce.
Include the URL, button name, expected vs actual. If the
bug requires seeded data, name the seed command.>

## Root cause
<One paragraph, max 5 sentences. File:line of the buggy
code. Why it produces the symptom. Link the spec /
contract the code violated, if any.>

## Call sites / blast radius
<List every place the same code path runs. For
"document.addEventListener(\"click\", ...)" the blast
radius is every page that contains the matching
selector. For a templ helper, list every templ that
imports it.>

## Proposed fix
<One paragraph: the smallest change that resolves the
root cause. If the fix has more than one reasonable
shape, list the options and recommend one.>

## Files
<Bulleted list of every file that will be touched. If
this is unknown, file the issue with just Symptom +
Repro and label `needs-triage`.>

## Regression net
<Bulleted list: unit test name(s), audit smoke probe
filename(s), or a manual smoke step. If the regression
net is unknown, the issue stays `needs-triage` until
the Plan phase writes it.>

## Related
<Issue numbers, ADR numbers, or `docs/COMMON_BUGS.md`
section references that overlap with this bug.>
```

### What goes in each section

**Symptom** is what the user told you, not what you
diagnosed. If the user said "the buttons don't work",
write "the buttons don't work" — don't paraphrase to
"the click handler is detached from the DOM". The
diagnosis lives in Root cause.

**Repro** is for the next maintainer. If the bug is in
production, write the steps the user followed. If it's
caught by a probe, write the probe invocation. If it
needs a fresh archive, say so and name the seed command.

**Root cause** must cite a file:line. If you can't, the
issue is not ready — you haven't done enough research.
Label it `needs-triage` and route it through the
diagnose skill.

**Call sites / blast radius** distinguishes "the bug" from
"the bug class". The Fix addresses both when the blast
radius is small; for big blast radius, the Fix may need
to be a refactor + the targeted fix.

**Proposed fix** is one paragraph, not a plan. The plan
with slices and success criteria lives in the RPCI Plan
phase output, which goes in the PR description or
`.rpiv/artifacts/plans/` — not in the issue body. Issues
should be small enough to read in 30 seconds.

**Regression net** names the test or probe that will
catch the bug if it regresses. A regression net that
just "confirms the fix works" is not enough; it must
catch the same shape of bug from a different entry
point.

### Labels

| Label | When to apply |
|---|---|
| `bug` | Always. Every bug gets this. |
| `needs-triage` | Root cause or files unknown. The research didn't pin the bug down. |
| `needs-info` | Symptom clear but the repro or root cause needs the reporter to clarify. |
| `ready-for-agent` | Full template filled in, root cause cited, fix proposed. An AFK agent can implement. |
| `ready-for-human` | Bug is straightforward but needs human judgment (e.g. UX decision). |
| `wontfix` | Decision to not fix. Must include reasoning in a comment. |

Apply `ready-for-agent` only when the R is complete. If
the issue has Symptom + Repro but no Root cause, leave
it at `needs-triage` and let the triage skill route it.

### Anti-patterns

- **Filing a bug with just the title.** Title + Symptom
  is fine for a `wontfix` candidate; for a real bug,
  every section is required.
- **Filing the fix in the issue body.** The issue is
  the research, the PR is the fix. Don't paste code
  into the issue; reference the files it will touch.
- **Skipping Repro because "it's obvious".** The next
  maintainer is not you. A 3-step repro that takes 30
  seconds to verify is worth more than a 1-paragraph
  diagnosis.
- **Filing during the chat session without writing it
  to the repo's actual issue tracker.** Issues live on
  GitHub. "I noted it in CHANGELOG" is not the same
  thing — CHANGELOG is for shipped changes, not for
  pending bugs.
- **Filing a "feature" as a "bug".** If the proposed
  fix is to add new behavior, label it `enhancement`
  and the template shifts: Symptom becomes "Current
  behavior", Repro becomes "Use case", Root cause
  becomes "Why this is missing", and the rest of the
  template still applies. The triage labels distinguish
  bugs (root cause exists, fix is restoration) from
  features (no root cause, fix is addition).

## Feature protocol

Enhancements use a parallel template that mirrors the bug
template's rigor. The full procedure — 3-tier commit rule,
module discipline, pipeline phasing, slice plan template —
lives in [`feature-protocol.md`](feature-protocol.md).
This section is the issue-filing companion only.

### Feature issue template

Every section is required unless marked optional:

```markdown
## Summary
<One sentence: what the feature does and who it serves.>

## User story
<As a <role>, I want <capability>, so that <outcome>.>

## Locked decisions
<Numbered list of decisions settled during recon. Each cites
the source: "Decided in <PR/issue/chat on YYYY-MM-DD>". Locked
decisions are NOT re-opened during the Critique phase unless
the user explicitly says so.>

## Proposed UX
<Per apply-site, what the user sees. References the screen by
name and surface by ID.>

## Apply sites (v1 checklist)
- [ ] <Surface 1>
- [ ] <Surface 2>

The feature is not "shipped" until every box is checked.
Backend-only landings require a tracked follow-up issue for
each missing UI surface; see Backend-First Law in CONTEXT.md.

## Glossary changes (if any)
<Quote the new entry exactly as it should appear in CONTEXT.md.
If the feature uses existing terms only, write "None.">

## Schema sketch (if any)
<Migration SQL + version bump + seed data.>

## Acceptance criteria
- [ ] <Observable, testable in 5 min>

## Slice plan
### Slice 1: <name>
- Files: <paths>
- Success criteria: <observable>
- Regression net: <test / probe / manual step>

## Test plan
- Unit: <file: TestXxx>
- Handler: <file: TestXxx>
- Migration: <file: TestXxx>
- Smoke probe: audit/smoke_<feature>.mjs (per UI apply-site)

## Files
- <bulleted list of every file that will be touched>

## Regression net
- <unit test names + audit/smoke probe filenames>

## Related
- <issue numbers, ADR numbers, docs/COMMON_BUGS.md refs>
```

### What shifts vs the bug template

| Bug section | Feature section | Why |
|---|---|---|
| Symptom | User story | Bug has a wrong-behavior; feature has a missing-capability |
| Repro | (skip — UI walk is in the apply-sites list) | Features don't repro; they get exercised by the user story |
| Root cause | Locked decisions | Bug has a why-broken; feature has a why-this-shape |
| Proposed fix | Proposed UX | Bug fixes are scoped to code; features span code + UI |
| Files | Apply sites + Files | Bug touches specific files; feature touches a surface area |
| Regression net | Test plan | Bug needs one net; feature needs per-apply-site nets |

### Labels

| Label | When to apply |
|---|---|
| `enhancement` | Always. Every feature gets this. |
| `needs-triage` | Locked decisions unknown or apply-sites list empty. The recon didn't pin the feature down. |
| `needs-info` | User story clear but the locked decisions or apply-sites need the reporter to clarify. |
| `ready-for-agent` | Full template filled in, locked decisions cited, apply-sites checklist complete, slice plan testable. An AFK agent can implement. |
| `ready-for-human` | Feature is straightforward but needs human judgment (UX decision, design call). |
| `wontfix` | Decision to not implement. Must include reasoning in a comment. |

Apply `ready-for-agent` only when the P (Plan) phase is complete.
If the issue has User story + Apply sites but no Locked decisions
or Slice plan, leave it at `needs-triage`.

### Anti-patterns

- **Filing a feature without apply-sites.** The checklist is the
  contract; an empty list signals "I haven't thought about where
  this lives in the UI." Don't file it until the list is concrete.
- **One mega-slice.** A slice plan with a single "Slice 1: ship
  the whole thing" line is no plan. Decompose until each slice is
  Tier 1 / Tier 2 / Tier 3 per the 3-tier rule.
- **Filing the design in the issue body.** The issue is the
  research + acceptance + slice plan. Architecture decisions
  live in `.rpiv/artifacts/designs/<slug>.md`. Don't paste code
  into the issue.
- **Locking decisions the user hasn't settled.** Locked decisions
  cite their source. If a decision is "I think we should X", it
  isn't locked — list it under "Open questions" instead.
- **Skipping the smoke probe.** Every UI apply-site needs an
  `audit/smoke_<feature>.mjs` assertion that checks both the
  response shape AND `page.url()` after the click.

## Label taxonomy

Issues carry labels from six axes. Each axis answers a different
question:

| Axis | Question | Values |
|---|---|---|
| **Type** | What's the work? | `bug`, `enhancement`, `documentation` |
| **Status** | Where is it in triage? | `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix` |
| **Area** | Which part of the system? | `area:backend`, `area:frontend`, `area:templates`, `area:cli`, `area:share`, `area:tags`, `area:export`, `area:import`, `area:db`, `area:docs`, `area:debug`, ... |
| **Priority** | How urgent is it? | `priority:high`, `priority:medium`, `priority:low` |
| **Cohort** | What batch does it belong to? | `audit-fallout`, ... |
| **Meta** | Process state, not work state | `duplicate`, `invalid`, `question`, `good first issue`, `help wanted`, `blocked` |

### Why six axes

- **Type × Status** distinguishes bugs from features without losing
  triage routing. A bug and an enhancement can both be
  `ready-for-agent`; the agent knows which template to read by
  the Type.
- **Area** lets a maintainer filter by component (`area:cli`,
  `area:templates`) without re-reading every issue title. New
  area labels are added via `scripts/sync-labels.sh` when a new
  component emerges (max ~15 to avoid label explosion).
- **Priority** is the urgency signal for the backlog. `priority:high`
  is reserved for known regressions, lost-data bugs, and the
  issues a user is actively blocked on. `priority:medium` is the
  default. `priority:low` is polish.
- **Cohort** groups issues that share a discovery context (e.g.
  `audit-fallout` for the 2026-06-24 audit sweep). Lets a
  maintainer filter the audit work without re-reading every
  audit-finding issue.
- **Meta** is process state, not work state. `blocked` is held by
  another issue. `duplicate` points to the canonical issue.
  `question` / `invalid` are triage outcomes, not work states.

### One label per axis

An issue has **exactly one label from each axis** (where the axis
applies). Two `area:*` labels on one issue means the agent doesn't
know which component to load docs for. Zero `area:*` labels means
the issue hasn't been routed.

The exception is `Meta` — an issue can carry multiple `Meta`
labels (`duplicate` + `wontfix` for "this is a dup, also we won't
fix either"). Process labels compose.

### Triage workflow

1. New issue lands with no labels (or just `enhancement`).
2. Maintainer applies the Type label (`bug` / `enhancement` /
   `documentation`).
3. Maintainer applies the Status label based on what's missing:
   `needs-triage` if Root cause / Locked decisions unknown,
   `needs-info` if the reporter owes detail, `ready-for-agent`
   when the template is complete.
4. Maintainer applies the Area label based on the affected
   component.
5. Maintainer applies the Priority label based on impact.
6. Agent picks up `ready-for-agent` issues, drops the Status label,
   works the slice, applies the next Status label (`needs-info` if
   blocked on the user, no label on merge).

The `scripts/backfill-labels.sh` script applies Area + Priority
labels to existing open issues by parsing titles + bodies. Idempotent.

## Closing issues when the work ships

Because this repo's branch policy (AGENTS.md §Branch policy) lands
work as direct commits to `dev` rather than via merged PRs, GitHub's
auto-close-via-keyword path (`fixes #N` in a PR body) does not
fire. **All 500 closed issues in this repo were closed manually.**
That is the dangling-issues trap: a slice ships, the agent moves on,
the issue stays open forever.

**The discipline:** the commit subject that lands the final slice of
an issue MUST close the issue. The closing step is part of the slice,
not a follow-up. Three equivalent paths:

### Path A: Include the issue number in the commit subject

If the issue number is in the commit subject, the closing step is:

```bash
gh issue close <N> --comment "Closed — landed in <short-sha>: <subject>"
```

The subject format `<area>: <imperative> (#N)` (the AGENTS.md
convention used by `ae86f1e`, `4198951`, etc.) makes the issue number
easy to grep for. **The `(#N)` at the end of the subject is a
breadcrumb, not a GitHub auto-close keyword.** Wrap it in parens or
append it after a colon and GitHub will NOT auto-close — that's why
agents must close manually.

### Path B: `Closes #N` in the commit body

If the subject doesn't have the issue number, add `Closes #N` to the
commit body (last bullet, separated by a blank line so GitHub picks
it up). Still requires `gh issue close <N>` after the commit lands,
since this repo doesn't open PRs — the `Closes` keyword only fires
on merged PR bodies.

### Path C: Periodic sweep

For issues that slipped through the cracks (work shipped but no
`(#N)` breadcrumb, no close comment), run a sweep:

```bash
# Find open issues whose number appears in any merged-or-on-dev commit
# subject across the last 90 days:
gh issue list --state open --json number,title --jq '.[] | .number' \
  | while read n; do
      if git log --since="90 days ago" --format='%s' \
         | grep -qE "(#${n}\b|fixes? #${n}\b)"; then
        echo "#$n likely shipped; review and close"
      fi
    done
```

The sweep is a fallback. Path A is the primary discipline.

### Anti-patterns

- **Ship the slice, leave the issue open "for the user to close".**
  The user is not always watching; the issue rots.
- **Trust `fixes #N` in the commit body to auto-close.** It won't,
  because there is no PR. The keyword only fires on merged PR bodies.
- **Trust `(#N)` at the end of the subject to auto-close.** Same
  reason — GitHub's parser sees the parens and ignores it. It's a
  breadcrumb for humans, not a closing keyword.
- **Close without a comment.** A close-with-no-comment loses the
  ship evidence. Future agents reading the issue need to know which
  commit landed the fix.

