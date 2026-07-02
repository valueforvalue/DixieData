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

