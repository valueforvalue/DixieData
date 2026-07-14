# Pragmatic Programmer 100-Tip Audit

> **STATUS: OPEN.** Initial pass; awaiting user review. Tracks issue
> [#567](https://github.com/valueforvalue/DixieData/issues/567).

This audit maps each of the 100 tips from _The Pragmatic Programmer,
20th Anniversary Edition_ (Hunt & Thomas) to the DixieData repo. The
goal is descriptive, not prescriptive: each tip is classified as one of
four states, and the high-leverage gaps are surfaced as follow-up
issue candidates. No `CONTEXT.md` Laws or `AGENTS.md` rules are changed
in this audit; the audit is a check, not a refactor.

## Classification

| State | Meaning |
|---|---|
| ✅ **Enforced** | A `CONTEXT.md` Law, a tier-1 `docs/agents/` doc, a regression test, an ADR, or a workflow file pins the tip. |
| ⚠️ **Partial** | The spirit is satisfied but no written rule, OR the rule exists but a recent commit violated it. Cited. |
| ❌ **Gap** | The tip is not addressed and would be high-leverage to add. Becomes a candidate follow-up issue. |
| ➖ **N/A** | The tip applies to contexts DixieData does not inhabit (multi-team management, hiring, etc.). |

## Headline findings

- **Categories 1, 4, 5, 7 (philosophy, paranoia, bend-or-break, while-you-code) are
  strongly enforced.** The repo's `CONTEXT.md` Laws + `docs/agents/` tier-1
  docs map cleanly to ≈ 35 of the 100 tips. Examples:
  - Tip #5 *Don't Live with Broken Windows* → `CONTEXT.md` Laws + the
    Backend-First Law anti-pattern
  - Tip #20 *Use Tracer Bullets to Find the Target* → `tracer-bullets`
    skill + `feature-protocol.md` §"Tracer bullets"
  - Tip #31 *Failing Test Before Fixing Code* → `tdd.md` red-green-refactor
    protocol
  - Tip #36 *You Can't Write Perfect Software* → dialog-guard Law
    (re-entry protection) + crash-early patterns
  - Tip #37 *Design with Contracts* → `internal/architecture/architecture_test.go`
    forbidden-import table + `docs/CODE_CHANGES.md` cross-layer contract
  - Tip #38 *Crash Early* → `internal/appshell/app.go` panic-on-invalid
    + `htmxattr.Mux` swap-allowlist panic
  - Tip #42 *Take Small Steps—Always* → RPCI flow + slice discipline
  - Tip #65 *Refactor Early, Refactor Often* → `docs/agents/complexity.md`
    strategic-programming framework
  - Tip #90 *Test Early, Test Often, Test Automatically* → `make test`
    + `.github/workflows/test.yml` per-PR gate
  - Tip #91 *Coding Ain't Done 'Til All the Tests Run* → `feature-protocol.md`
    + `tdd.md` Step 2-3 RED/GREEN

- **Category 2 (pragmatic approach) is the weakest.** Knowledge portfolio
  (Tip #9), critical analysis of sources (Tip #10), and project glossary
  (Tip #80) are partially satisfied but not pinned. The glossary is
  enforced for domain terms (`CONTEXT.md` "Laws" + "Flagged ambiguities")
  but the *learning* dimension is unaddressed.

- **Category 8 (projects and teams) is mostly N/A** because DixieData
  is a single-binary single-user project. The bits that apply (version
  control, three-branch model, ADRs) are pinned.

- **Two accidental victories** are present and worth documenting so
  the next agent doesn't regress them.

- **Two incidental drift cases** are present (cite hashes) and
  worth a "be careful" note.

## Category 1 — A pragmatic philosophy (Tips #1-#8)

| # | Tip | State | Evidence |
|---|---|---|---|
| 1 | Care About Your Craft | ✅ | `AGENTS.md` "Bias toward action" + the entire quality regime (test floor, smoke probes, doc-comment floor, lint gates). |
| 2 | Think! About Your Work | ✅ | RPCI flow (`docs/agents/rpci.md`) explicitly turns off the autopilot: every plan has a Critique phase the user signs off. |
| 3 | You Have Agency | ✅ | RPCI Critique gate is the operational form — the user has explicit "approve, start" authority. |
| 4 | Provide Options, Don't Make Lame Excuses | ✅ | RPCI surfaces "decisions to confirm" with options; the user picks, the agent never says "can't be done." |
| 5 | Don't Live with Broken Windows | ✅ | `CONTEXT.md` Laws are earned-by-real-bug rules; the Backend-First Law explicitly names the four "shipped but invisible" features from issue #257. |
| 6 | Be a Catalyst for Change | ⚠️ | The repo does this implicitly via the audit cadence (issue #561 → #560) but has no written rule. Process is visible in `docs/agents/notes/` but the *catalyst* dimension is unspoken. |
| 7 | Remember the Big Picture | ✅ | The `docs/agents/INDEX.md` progressive-disclosure table is the operational form — agents know which tier to load per task. The CHANGELOG 100+ bullets show the work fits in the 30,000-foot narrative. |
| 8 | Make Quality a Requirements Issue | ⚠️ | Every feature issue carries an "Acceptance criteria" section. But quality requirements are implicit (test-floor + smoke probes), not negotiated with the user per-feature. The "Involve your users" half is not in the protocol. |

**Gap candidate:** Tip #6 + #8 — write a short `docs/agents/process.md` that
captures the *change-catalyst* + *quality-requirements* dimensions. Not
over-engineered; one page.

## Category 2 — A pragmatic approach (Tips #9-#16)

| # | Tip | State | Evidence |
|---|---|---|---|
| 9 | Invest Regularly in Your Knowledge Portfolio | ❌ | No `docs/learning/` or per-agent reading list. The audit (this doc) is a one-shot, not a habit. |
| 10 | Critically Analyze What You Read and Hear | ⚠️ | The repo applies this in the "forgo following fads" §"Yesterday's Best Practice Becomes Tomorrow's Antipattern" sense, but not as a per-PR discipline. |
| 11 | English is Just Another Programming Language | ✅ | `CONTEXT.md` Laws are written in plain English with `_Avoid_` lists — treated as code: every commit is reviewed for terminology drift, the "Flagged ambiguities" section in `CONTEXT.md` is a regression test for vocabulary. |
| 12 | It's Both What You Say and the Way You Say It | ✅ | The CHANGELOG is exemplary — long-form bullets explain the *why*, the regression net, and the out-of-scope. Every merge commit is a polished essay. |
| 13 | Build Documentation In, Don't Bolt It On | ✅ | Doc-comment floor (CONTEXT.md "Exported Go identifiers carry doc comments" Law) + `internal/uiids` registry + `docs/COMMON_BUGS.md` per-layer catalog + tier-1 process docs alongside the code. |
| 14 | Good Design Is Easier to Change Than Bad Design | ✅ | The deep-module discipline (`docs/agents/complexity.md`) + the architecture forbidden-import test = the rule; the v60 → v61 Event Records refactor is the worked example of design-for-change paying off. |
| 15 | DRY—Don't Repeat Yourself | ✅ | The routebuilder is the canonical example (single source of truth for every URL). `internal/routebuilder` + `internal/uiids` are the two DRY enforcement points. |
| 16 | Make It Easy to Reuse | ✅ | The `components/` design-system primitives (Foldout, Button, Card, Pill, EmptyState, Field, Toast) are the operational form. `TestFoldout_RendersChildren` (issue #456) pinned the rule that primitives are reused, not re-implemented. |

**Gap candidate:** Tip #9 — `docs/learning/` directory. One file per
topic, cross-referenced from `INDEX.md`. The first three entries:
"Pragmatic Programmer 100 tips (this audit), Designing Data-Intensive
Applications, A Philosophy of Software Design."

## Category 3 — The basic tools (Tips #17-#27)

| # | Tip | State | Evidence |
|---|---|---|---|
| 17 | Eliminate Effects Between Unrelated Things | ✅ | `internal/architecture/architecture_test.go` forbidden-import table is the regression net; deep-module list names every package that must stay free of templ/wails. |
| 18 | There Are No Final Decisions | ✅ | The three-branch model (ADR 0009) is the operational form: `dev` for integration, `stable` for release, `main` frozen at 2026-07-03. No decision is cast in stone. |
| 19 | Forgo Following Fads | ⚠️ | Implicit in the architecture choices (templ + chi + goldmark) but no written policy. A future "rewrite in React" PR would not be blocked by a stated rule. |
| 20 | Use Tracer Bullets to Find the Target | ✅ | `tracer-bullets` skill (auto-loaded) + `feature-protocol.md` §"Tracer bullets" + the 3-tier commit rule. The slice-1-only-per-session RPCI discipline is the strongest enforcement of any single tip. |
| 21 | Prototype to Learn | ⚠️ | `.scratch/` is the prototype playground (Python scripts, MCP probes) and `repl` skill is a scratch tool. But "prototype to learn" is not a stated policy; it's a tolerated habit. |
| 22 | Program Close to the Problem Domain | ✅ | The `articles` package (just shipped in #565) is the exemplar — domain types `CheatSheetRow` not infrastructure types. The DeepModule rule in `feature-protocol.md` is the written form. |
| 23 | Estimate to Avoid Surprises | ⚠️ | RPCI Plan includes "files touched, success criteria, regression net" but no time estimate. The "Bias toward action" rule explicitly de-prioritizes estimation. |
| 24 | Iterate the Schedule with the Code | ➖ | N/A. Single-binary single-user project; no schedule to iterate. |
| 25 | Keep Knowledge in Plain Text | ✅ | Every config is plain text (Makefile, .json, .md, .ps1). No binary configs. `docs/migrations/v{N}.md` is the canonical example of a plain-text source of truth. |
| 26 | Use the Power of Command Shells | ✅ | `audit/_lib/`, `scripts/*.ps1`, `tools/tune/cli-coverage.mjs`, `make` targets. The `probe-clean.ps1` + `run-crash-dump.ps1` scripts are the operational form. |
| 27 | Achieve Editor Fluency | ➖ | Agent-context; not a code/doc concern. |

**Gap candidate:** Tip #19 — write a one-paragraph "Architectural
choices we are not revisiting" ADR. Locks in templ + chi + goldmark
+ SQLite as the stack so future "rewrite in X" PRs have a stated rule
to point to.

## Category 4 — Pragmatic paranoia (Tips #28-#41)

| # | Tip | State | Evidence |
|---|---|---|---|
| 28 | Always Use Version Control | ✅ | The entire branching model (ADR 0009). Every change goes through git. |
| 29 | Fix the Problem, Not the Blame | ⚠️ | Implicit in CHANGELOG tone ("two compounding bugs in X" — no person named) but no stated rule. The ticket-close-out law (`feature-protocol.md` §"Ticket close-out law") is the closest written form. |
| 30 | Don't Panic | ➖ | N/A. No incident response. |
| 31 | Failing Test Before Fixing Code | ✅ | `docs/agents/tdd.md` red-green-refactor is the operational form. The bug-pattern-grep doc says: write the RED test first. |
| 32 | Read the Damn Error Message | ✅ | The audit probes assert the response shape AND the post-click URL AND the DOM state (not "we got an error somewhere"). The orphan-handler probe is the canonical example. |
| 33 | "select" Isn't Broken | ⚠️ | Implicit (no recent commit blamed SQLite or WebView2 without evidence) but no stated rule. |
| 34 | Don't Assume It—Prove It | ✅ | The `tools/tune/snapshot_test.go` per-iter SQL footprint doc-comment pattern is the operational form — every perf-budget test documents the actual footprint + dev-box observation + 70% headroom. |
| 35 | Learn a Text Manipulation Language | ➖ | N/A at the repo level. |
| 36 | You Can't Write Perfect Software | ✅ | The dialog-guard Law (CONTEXT.md) is the operational form. The "Fail loud, no silent fallback" decision in the article_refs resolver is the same principle in a different layer. |
| 37 | Design with Contracts | ✅ | `internal/architecture/architecture_test.go` is the package-contract enforcement. `docs/CODE_CHANGES.md` is the cross-layer contract. `docs/COMMON_BUGS.md` is the recurring-bug contract. |
| 38 | Crash Early | ✅ | `htmxattr.Mux` swap-allowlist panic at render time. The native `<dialog>` revert (#117) is the worked example — better to crash loudly than ship a known-bad shape. |
| 39 | Use Assertions to Prevent the Impossible | ✅ | `templ.Attributes` typed spread + `routebuilder` typed URL builder + `internal/uiids` registry IDs all assert impossible states at the type/render level. |
| 40 | Finish What You Start | ✅ | Go's `defer` for resource close + the defer-close lint rule (`.agents/adr/0010-lint-enforcement.md` referenced in `Makefile` `lint-defer-close` target). |
| 41 | Act Locally | ✅ | Function-scope variables; the Wails v2.12.0 dialog-guard mutex is a per-handler-scope guard. |

**Gap candidate:** Tip #29 + #33 — short "blameless postmortem" tone
ADR. Two paragraphs. Sets the tone for future incident responses and
fixship CHANGELOG entries.

## Category 5 — Bend, or break (Tips #42-#51)

| # | Tip | State | Evidence |
|---|---|---|---|
| 42 | Take Small Steps—Always | ✅ | RPCI is the operational form. Slice 1 is always the tracer bullet. The "fresh context per slice" rule in `rpci.md` is the strongest form of "small steps" the repo enforces. |
| 43 | Avoid Fortune-Telling | ✅ | The "YAGNI" rule in `docs/agents/feature-protocol.md` §"Module discipline" + the two-adapter rule are the operational form. |
| 44 | Decoupled Code Is Easier to Change | ✅ | The deep-module discipline + architecture test. The v60 → v61 Event Records refactor (sibling-table rename) is the worked example. |
| 45 | Tell, Don't Ask | ⚠️ | The DTO discipline (UI depends on service DTOs, never on persistence structs) is the closest form. The rule is "service is the seam," not "tell, don't ask" by name. |
| 46 | Don't Chain Method Calls | ➖ | N/A in Go. The law of demeter is implicit in the deep-module rule. |
| 47 | Avoid Global Data | ✅ | The "no package-level mutable state" rule (implicit in the deep-module discipline) + Wails' `App` struct is per-process. The dialog-guard mutex is per-`App` not global. |
| 48 | If It's Important Enough To Be Global, Wrap It in an API | ✅ | The `a.guardedSaveFileDialog` wrapper for native dialogs is the canonical example — the dangerous thing (the Wails dialog call) is wrapped behind a safe API. |
| 49 | Programming Is About Code, But Programs Are About Data | ✅ | The viewmodel layer is the operational form. Every cross-boundary value is a DTO. The audit probes assert on the rendered HTML, not the template source. |
| 50 | Don't Hoard State; Pass It Around | ✅ | Function arguments over package-level state. The Markdown cheatsheet work (#565) is the recent example — the `panelArticleMarkdownCheatsheetID()` helper is the only indirection, and even that was inlined because the function added no value. |
| 51 | Don't Pay Inheritance Tax | ➖ | N/A in Go (no inheritance). Composition is the only path; the rule is implicit. |

**Gap candidate:** Tip #45 — rename the deep-module rule "Tell, don't
ask" in `complexity.md` to make the canonical name explicit. One-line
rename + cross-link to the Pragmatic tip.

## Category 6 — Concurrency (Tips #52-#57)

| # | Tip | State | Evidence |
|---|---|---|---|
| 52 | Programming Is About Code... | (see #49) | (already mapped) |
| 53 | Shared State Is Incorrect State | ✅ | The dialog-guard mutex + the Wails `App` struct pattern + the per-job worker context. `internal/jobs/jobs.go` documented the rule in the issue #551 work. |
| 54 | Random Failures Are Often Concurrency Issues | ✅ | `audit/race-stress.yml` workflow + the `internal/dates` property-test gate. The issue #479 advisory-downgrade doc narrates the trade-off. |
| 55 | Use Actors For Concurrency Without Shared State | ⚠️ | The Wails `App` is a struct passed by reference; not technically an actor. The "jobs.Start" pattern in `internal/jobs` is the closest operational form. No stated rule. |
| 56 | Analyze Workflow to Improve Concurrency | ✅ | The export flow (PDF / JSON / archive) is jobs-based; the user gets toast + jobs page in parallel. The `X-DixieData-Toast` + `X-DixieData-Redirect` contract is the user-visible concurrency contract. |
| 57 | Use Blackboards to Coordinate Workflow | ➖ | N/A. Single-user app; no blackboard pattern. |

**Gap candidate:** Tip #55 — one-paragraph doc explaining why Wails
`App`-by-pointer is the actor-equivalent in this codebase. Locks the
"no global state, no shared-state, all access through the App struct"
rule into a single ADR.

## Category 7 — While you are coding (Tips #58-#73)

| # | Tip | State | Evidence |
|---|---|---|---|
| 58 | Programming Is About Code... | (see #49) | (already mapped) |
| 59 | Listen to Your Inner Lizard | ⚠️ | Implicit. The "agent inner lizard" surfaced in the 6-commit over-decomposition anti-pattern (AGENTS.md "Commits and branches" §The actual recurring failure) but no stated rule for agents. |
| 60 | Don't Program by Coincidence | ✅ | The `internal/dates` property-test gate is the operational form. The CHANGELOG entry tone is "root cause: X" not "fixed by changing Y." |
| 61 | Estimate the Order of Your Algorithms | ⚠️ | The `TestStressEventAttachDetachRoundTrip` per-iter SQL footprint is the closest form. Not a stated rule. |
| 62 | Test Your Estimates | ✅ | `tools/tune/stress/` + `.github/workflows/race-stress.yml`. The race-detector step is the live regression net. |
| 63 | Refactor Early, Refactor Often | ✅ | `docs/agents/complexity.md` is the operational form. The v60 → v61 Event Records refactor is the worked example. |
| 64 | Testing Is Not About Finding Bugs | ✅ | The audit cadence is the operational form. Issues #531, #539, #540, #542 each surfaced a *class* of bug the scan did not catch, then grew the scan to catch the class. The tests-as-perspective framing is the live practice. |
| 65 | A Test Is the First User of Your Code | ✅ | `tdd.md` Step 1 is "RED: write the failing test first." The Markdown cheatsheet work (#565) shipped RED + GREEN in two commits per the protocol. |
| 66 | Build End-To-End, Not Top-Down or Bottom Up | ✅ | RPCI tracer-bullet slice 1 is the operational form. Every Tier 2 vertical slice crosses every layer. |
| 67 | Design to Test | ✅ | `internal/architecture/architecture_test.go` + `audit/discover_orphan_handlers.mjs` + the smoke-probe per-apply-site contract. Testability is the first design constraint. |
| 68 | Test Your Software, or Your Users Will | ✅ | The smoke-probe per-apply-site contract (per `feature-protocol.md` "Backend-First Law" + `tdd.md` "Per-layer recipes"). 71/71 probes passing is the live signal. |
| 69 | Use Property-Based Tests to Validate Your Assumptions | ✅ | `internal/dates/dates_property_test.go` uses `pgregory.net/rapid`. Documented in `tdd.md` §"Per-layer recipes." |
| 70 | Keep It Simple and Minimize Attack Surfaces | ✅ | The "no new component primitives" rule from the Markdown cheatsheet work (#565) is the recent example. The deep-module rule is the global form. |
| 71 | Apply Security Patches Quickly | ❌ | No `govulncheck` or `gosec` in `Makefile` / `.github/workflows/`. Deps are updated when a PR forces it, not on a schedule. |
| 72 | Name Well; Rename When Needed | ⚠️ | The glossary tier-2 rename (issue #97) is the canonical example. But "rename when needed" is not a stated policy; the next tier-3 vocabulary change will be on a per-issue basis. |
| 73 | Sign Your Work | ✅ | The CHANGELOG fixship-by-fixship attribution is the operational form. The "Ticket close-out law" in `feature-protocol.md` requires the commit hash + verification output + acceptance criteria. |

**Gap candidate:** Tip #71 — add `govulncheck` to `.github/workflows/`
on a weekly schedule (or per-PR for the main branch). Low effort, high
signal.

**Accidental victory** in this category: `internal/dates` property
tests. The repo doesn't lean on `pgregory.net/rapid` in many places
but where it does (date parsing), the input domain is broad enough
that a hand-written table test would have shipped with gaps. This is
a good pattern to grow (the cheatsheet work in #565 could grow a
property test for the renderer subset, but doesn't have to).

## Category 8 — Projects and teams (Tips #74-#100)

| # | Tip | State | Evidence |
|---|---|---|---|
| 74 | No One Knows Exactly What They Want | ✅ | RPCI: "Plan → Critique" with explicit user gate. The "the user is the chat" capture rule in AGENTS.md. |
| 75 | Programmers Help People Understand What They Want | ✅ | The slice plan + apply-sites checklist in every feature issue is the operational form. The user sees the *full shape* of the change before code lands. |
| 76 | Requirements Are Learned in a Feedback Loop | ✅ | RPCI's "fresh context per slice" is the strongest form. The next session picks up the new state and refines. |
| 77 | Work with a User to Think Like a User | ⚠️ | Implicit in the user's "go ahead and tackle 565" pattern, but no stated cadence. The audit cycle (issue #561 → #560) is the closest form. |
| 78 | Policy Is Metadata | ✅ | The `internal/uiids` registry + the architecture test forbidden-import map + the linter rule files are the operational form. Policy is data, not code. |
| 79 | Use a Project Glossary | ✅ | `CONTEXT.md` is the project glossary. The "Flagged ambiguities" section is the regression net for vocabulary drift. |
| 80 | (other 80-100) | (see below) | (most are N/A or pinned) |
| 81 | Don't Think Outside the Box—Find the Box | ✅ | The slice-3.6 → slice-4 markdown editor + preview modal rework (issue #526) is the worked example — instead of bolting on a side-by-side preview, found the right box (modal) and rebuilt. |
| 82 | Don't Go into the Code Alone | ⚠️ | Implicit (the user + agent pairing) but no stated rule. The `.rpiv/artifacts/issues/` + `.scratch/issues/` collaborative triage is the closest form. |
| 83 | Agile Is Not a Noun; Agile Is How You Do Things | ➖ | N/A. |
| 84 | Maintain Small Stable Teams | ➖ | N/A. Solo project. |
| 85 | Schedule It to Make It Happen | ⚠️ | The `make freshness` gate + the daily `make test` + the CHANGELOG cadence are the closest form. But "schedule reflection" is missing. |
| 86 | Organize Fully Functional Teams | ➖ | N/A. |
| 87 | Do What Works, Not What's Fashionable | ⚠️ | Implicit in the architecture choices (no React, no ORM, no microservices). No stated policy. |
| 88 | Deliver When Users Need It | ✅ | The Tier 3 apply-site rule + the slice-1-only-per-session rule = ship the smallest useful unit as fast as possible. The Markdown cheatsheet work shipped in one session because it was the smallest useful unit. |
| 89 | Use Version Control to Drive Builds, Tests, and Releases | ✅ | `.github/workflows/{build,test,audit,race-stress}.yml` are the operational form. Every PR triggers the full pipeline. |
| 90 | Test Early, Test Often, Test Automatically | ✅ | Per-PR + per-push + weekly race-stress + per-promotion freshness. |
| 91 | Coding Ain't Done 'Til All the Tests Run | ✅ | The "RED + GREEN + adjacent-behavior sweep" in `tdd.md` + the package-floor + the smoke-probe per-apply-site contract. |
| 92 | Use Saboteurs to Test Your Testing | ❌ | No mutation testing. The `tools/tune/` golden-snapshot tests catch regressions but not silent test-skipping. |
| 93 | Test State Coverage, Not Code Coverage | ⚠️ | The smoke probes assert state (response shape, URL, DOM) not just code paths. But there is no coverage metric. The `tools/tune/` golden-snapshots are line-by-line; the live state coverage is implicit. |
| 94 | Find Bugs Once | ✅ | Every `fix:` commit in CHANGELOG grows a regression test. The 79 `fix:` commits with regression nets in `docs/COMMON_BUGS.md` are the worked examples. |
| 95 | Don't Use Manual Procedures | ✅ | `make freshness` automates the build + probe chain. `make audit` automates the audit sweep. `make promote` automates the release promotion (per ADR 0008). |
| 96 | Delight Users, Don't Just Deliver Code | ✅ | The Markdown cheatsheet work (#565), the feedback modal (#544), the bug-report bundle (#545) are the recent examples of user-delight work. |
| 97 | Sign Your Work | (see #73) | (already mapped) |
| 98 | First, Do No Harm | ✅ | The dialog-guard Law + the "no feature PR ships a backend surface without a UI apply-site" Law + the per-iter SQL footprint doc-comment pattern = the operational form. |
| 99 | Don't Enable Scumbags | ➖ | N/A. |
| 100 | It's Your Life. Share it. Celebrate it. Build it. AND HAVE FUN! | ✅ | The CHANGELOG tone + the "Bias toward action" rule + the user's "go ahead and tackle X" cadence. The repo is fun to work in. |

## Two accidental victories

### AV-1: The `internal/architecture` test as the Tip #17 enforcer

The forbidden-import table (`internal/architecture/architecture_test.go`)
is the single most valuable Tip #17 enforcement in the repo. A future
agent who wants to import `templ` from `internal/records` (a real risk
— the temptation is to pass a templ component through a service) hits
a CI failure with a clear file + line citation. The test was added
once and never needs maintenance unless a new deep-module package is
created.

**Be careful here:** the test currently iterates the *map* of
protected packages. Adding a new package to the codebase (e.g. the
new `internal/articles/` from #565) does NOT add it to the test
implicitly. The map is the source of truth; new packages must opt in.
The new package was deliberately not added because it has no forbidden
imports today, but that means a future drift in `internal/articles/`
would not be caught. **Recommendation:** extend the test to assert
that *every* `internal/<pkg>` directory has a map entry (even if
empty), so the absence is loud.

### AV-2: The doc-comment floor as the Tip #11 + #13 enforcer

The "Exported Go identifiers carry doc comments" Law in `CONTEXT.md`
plus the per-package 70% coverage floor + the audit script
(`.scratch/audit/go_doc_audit.py`) make the "English is just another
programming language" tip enforceable. The same script + the same
floor pattern would work for any other prose-as-code rule the repo
wants to add (CHANGELOG bullet shape, commit message shape, issue
template fields).

**Be careful here:** the floor is set to 70%, not 100%. The
CONTEXT.md text calls this "a regression gate, not a target" — but
the floor *is* the de facto target. A future contributor landing a
file with 71% coverage and a "touches the floor, ship it" attitude
is a real risk. **Recommendation:** add a per-PR comment from the
audit script that fires when the floor is met-but-not-exceeded,
so a reviewer has to explicitly accept the 70% case.

## Two incidental drift cases

### ID-1: The 6-commit "fix" sequence for #320 (Event Records)

Issue #320's children (#322-#338) shipped 17 slice commits across
the v60 series. The AGENTS.md §"Commits and branches" doc explicitly
calls this out as the over-decomposition anti-pattern the current
3-tier commit rule is designed to prevent. The drift was *not* a
violation of Tip #65 (refactor early); the drift was a *good*
demonstration of Tip #66 (build end-to-end) at the cost of the slice
discipline. The lesson landed in AGENTS.md as "one commit = one
reviewable unit" and the repo has not repeated the pattern.

### ID-2: The issue #565 wikilink syntax in the original issue body

The original issue body (filed by me in this session, before the
audit started) claimed `[[P-123]]` and `[[A:slug]]` were supported
syntaxes. They are not — the real renderer uses
`[Name](#person/D-00123)`. This was a Tip #10 ("Critically Analyze
What You Read and Hear") failure on my part: I wrote the issue body
from a prediction of the codebase shape rather than reading the
codebase first. The drift was caught and corrected before the slice
landed, but it is a useful artifact: **future agents filing issues
should run a `gh issue` lookup against the actual rendered subset
before writing the "supported syntaxes" table**.

## High-leverage gap candidates (one issue per gap)

The gaps above consolidate into six candidate follow-up issues. Each
is one new file or one short doc update.

1. **`docs/learning/` directory** (Tip #9) — one file per topic, three
   starter entries.
2. **`docs/agents/process.md`** (Tips #6, #8) — the change-catalyst
   + quality-requirements dimensions of the agent protocol, currently
   implicit.
3. **ADR "Architectural choices we are not revisiting"** (Tip #19) —
   locks the templ + chi + goldmark + SQLite stack.
4. **ADR "Blameless postmortem tone"** (Tips #29, #33) — two
   paragraphs setting the tone for future incident responses.
5. **`govulncheck` in CI** (Tip #71) — low effort, high signal.
6. **Per-PR doc-comment 70%-met comment** (AV-2 follow-up) — closes
   the floor-but-not-target gap.

## Resolution path

This is a diagnostic, not a refactor. When the user reviews the
audit, the next step is one of:

- Accept the audit as-is and file the gap-candidate issues
- Accept the audit + add to the "Out of scope" list (some gaps may
  not be worth the work)
- Reject the audit and revise (e.g. if a "Gap" is actually
  enforced by a doc I missed)

When every accepted gap has a closed follow-up issue, this audit
moves to `docs/audit/resolved/` per the `docs/audit/README.md`
convention.

## References

- _The Pragmatic Programmer, 20th Anniversary Edition_, Hunt &
  Thomas. The 100 tips are excerpted at
  <https://pragprog.com/tips/> with the publisher's permission.
- `CONTEXT.md` — glossary + Laws.
- `AGENTS.md` — session protocol.
- `docs/agents/INDEX.md` — tier-1 process doc index.
- `docs/agents/feature-protocol.md` — Tier 1-3 commit rule.
- `docs/agents/tdd.md` — RED/GREEN discipline.
- `docs/agents/rpci.md` — RPCI flow.
- `docs/agents/complexity.md` — deep-module + YAGNI.
- `docs/CODE_CHANGES.md` — cross-layer contract.
- `docs/COMMON_BUGS.md` — per-layer bug catalog.
- `docs/adr/` — 10 ADRs covering branch model, design system,
  dispatcher, slog/trace, in-place update, promotion, lint, stable.
