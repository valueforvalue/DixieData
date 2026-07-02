# RPCI — Research, Plan, Critique, Implement

The default workflow for non-trivial work on this repo. Use the
acronym "RPCI" to invoke it ("do RPCI on this", "run RPCI before
implementing") — see the [Invocation](#invocation) section at the
bottom for the surface forms the LLM agent should recognize.

The flow is procedural, not architectural. It governs **how the
agent works until the code ships**; architectural shape comes
from `CONTEXT.md` and `AGENT_ARCHITECTURE_MAP.md`.

## When to use

| Task shape | Flow |
|---|---|
| One-line bug fix, clear repro | **Skip RPCI.** Patch + regression test + commit. |
| Multi-file bug, root cause unknown | **R1 + I.** Research enough to confirm the fix, then implement. Skip explicit Plan / Critique. |
| Feature / refactor touching 3+ files | **Full RPCI.** All four phases, even if Plan is one paragraph. |
| Subsystem redesign, 6+ files across layers | **Full RPCI + design artifact.** Write the artifact under `.rpiv/artifacts/designs/`. The RPCI phases still gate the work; the artifact is the durable output. |

The "Bias toward action" rule from `AGENTS.md` still applies:
don't over-procedure a small task. The RPCI overhead is paid
once; under-using it on a big task costs more than over-using
it on a small one.

## The four phases

### R — Research

**Goal:** Understand the problem space well enough to write a
plan that doesn't re-ask the questions you should have answered
in recon.

**Activities:**
- Read the affected files. For Go backend, start with the
  handler; for templ/htmx, start with the rendered HTML.
- Read `docs/COMMON_BUGS.md` §layer for the area you're touching.
  Recurring bug patterns are documented there with `Find it:`
  greps.
- Read related issues on GitHub (use `gh issue view N` for
  context, `--json comments` for prior discussions).
- Read prior commits on the same files (`git log --oneline
  -- <path>`). Recurring failure: 200-line "fix export buttons"
  commits that bundle templ + handler + audit test + regression
  net + CHANGELOG. Each was a separate logical change.
- Bench / measure when the claim is performance. "It's slow" is
  not a plan; "5k records takes 261ms because `listAllSoldiers`
  is 80% of the cost" is.

**Output:** A short prose summary — what's broken (or to be
built), the call sites, the constraints, the bench numbers if
any. Don't write a plan yet.

**Stop when:** You can articulate the root cause (or, for a
feature, the surface area) in two sentences. If you can't,
you need more recon, not more plan.

### P — Plan

**Goal:** Lay out the change as a sequence of atomic, testable
slices that map to commits.

**Activities:**
- Decompose the change into vertical slices. A slice is
  end-to-end across layers (templ + handler + JS + test) and
  produces something a user (or a probe) can verify.
- For each slice, list: files touched, success criteria, the
  regression net (test name, smoke probe, manual step).
- Identify the commit/PR boundary. One commit per logical
  change, per `AGENTS.md`. If a slice bundles two logical
  changes, split it.
- Surface decisions that need user input. **Don't hide them
  in the body of a slice** — list them as `## Decisions` at
  the top so the Critique phase catches them.

**Output:** A plan document. Format:

```markdown
# Plan: <title>

## Decisions to confirm
- Q1: ...
- Q2: ...

## Slices
### Slice 1: <name>
- Files: <paths>
- Success criteria: <observable>
- Regression net: <test/probe>
### Slice 2: ...
```

**Stop when:** Each slice can be implemented by a focused
agent in one sitting, the success criteria are testable, and
the decisions are surfaced.

### C — Critique

**Goal:** Stress-test the plan before any code moves.

**Activities:**
- Use the `grilling` skill (or its surface form) to interview
  the user about the plan. Aim for 3-5 focused questions, not
  20. The questions should be about decisions and trade-offs,
  not about things the plan already nailed.
- For each decision in `## Decisions`, present the trade-off
  and ask for the call. Provide a recommended option with
  "(Recommended)" appended.
- For each slice, ask: "Is this slice's success criterion
  testable in 5 minutes? Does the regression net catch a
  future regression, or does it just confirm the fix?"
- Check: does the plan match `AGENTS.md`? (One commit per
  slice? Bug-pattern greps run? CHANGELOG updated? smoke
  probe in the right format?)
- If the critique surfaces a missing slice, a wrong commit
  boundary, or a decision the user wants to revisit, revise
  the plan and re-critique. Don't move to Implement with
  open questions.

**Output:** A revised plan, approved by the user. The user's
explicit "Approve, start" or "looks good, go" is the gate.

**Stop when:** The user has signed off and the only remaining
work is mechanical execution.

### I — Implement

**Goal:** Execute the plan slice by slice, with regression
tests confirming each slice before the next starts.

**Activities per slice:**
1. Read the files you'll touch (recon is cheap, do it again
   even after the plan is written — things move).
2. Make the change. Smallest diff that satisfies the success
   criterion. No drive-by refactors.
3. Add the regression net. For Go: a test in
   `internal/<layer>/<file>_test.go`. For UI: an
   `audit/smoke_<feature>.mjs` probe (live binary + headless
   browser).
4. Run the targeted test + the package's full short suite.
   `go test -count=1 -short ./internal/<layer>/...`.
5. Run smoke probes that touch the area. `node
   audit/smoke_<feature>.mjs` against the debug build.
6. Update CHANGELOG. One bullet per logical change, under the
   right `### Added/Changed/Fixed/Maintenance` heading.
7. Commit. Subject `<area>: <imperative summary>` ≤72 chars;
   body explains *why* and references the issue.
8. Push branch + open PR (target `dev`, not `main`).
9. Watch CI (audit + test + build all green).
10. Hand the merge back to the user; don't auto-merge.

**Stop when:** Every slice is committed, all PRs are merged
to `dev`, CHANGELOG `[Unreleased]` is current.

## Invocation

The user invokes the flow with a phrase that includes
"RPCI" or its full name. The LLM agent must:

1. Confirm it understood the scope (one sentence, not a
   re-ask of the full task).
2. Run RPCI, gating the Implement phase on explicit approval.
3. Surface the slice breakdown before writing any code.

Surface forms the user might use:
- "do RPCI on the click delay fix"
- "research plan critique implement the import dedupe"
- "RPCI this"
- "run RPCI before coding"
- "let's RPCI the new export template UI"

## Anti-patterns

- **Skipping Critique.** The user explicitly asked for this
  flow because past sessions shipped plans that the user
  would have rejected if asked. Don't let urgency skip the
  gate.
- **Critique as a monologue.** The plan needs real questions,
  not a summary of what the agent already decided. If the
  user can answer "yes" to all questions in five seconds,
  the plan didn't surface real decisions.
- **Plan with 10+ slices.** If the change needs that many
  atomic units, decompose into a design artifact first
  (`.rpiv/artifacts/designs/`) and reference it from the
  plan. The plan's job is to sequence, not to enumerate
  every micro-step.
- **Implementing during Plan.** Recon, asking questions, and
  writing a plan are all fine. Running `make` targets that
  mutate state, writing files, or scaffolding handlers is
  not, until the user approves.
- **One commit per slice is not a rule.** Some slices are
  pure refactors with no user-visible effect and belong in
  `### Maintenance` with their own commit; some are tightly
  coupled and land as one commit. Use the rule of thumb: if
  you can write a one-sentence commit message for the slice
  that doesn't need "and", it's atomic.
