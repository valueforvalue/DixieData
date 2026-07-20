# Pragmatic Programmer Tip Index (100 tips)

> **Externalized from [`../agents/pragmatic-principles.md`](../agents/pragmatic-principles.md).**
> The principle spine in §1–§5 of that doc is what agents load
> when designing or reviewing a feature. This file holds the
> **per-tip reference**: every row in §6 (Tip index), §7
> (Summary counts), and §9 (Book chapter → tip cross-reference).
>
> **When to load:** on demand, when checking whether a specific
> commit, behavior, or new pattern is in compliance with the
> canonical book. Not part of the Tier-0 / Tier-1 load.
>
> **Maintenance:** the source audit at
> [`pragmatic-programmer-audit-2026-07.md`](pragmatic-programmer-audit-2026-07.md)
> is the per-tip *evidence* (historical artifact, retained).

---

## §6 — Tip index (the 100 tips, by number)

This index is the **operational counterpart** to the principle
spine in §1. Each tip is a concrete behavior; the principle
is the *why* behind the behavior. The principle spine is what
an agent reads when designing a new feature; this index is what
an agent reads when checking whether a specific commit or
behavior is in compliance.

### When to consult this table

- **Slice planning** (per `feature-protocol.md`) — before
  shipping a Tier 2 vertical slice, scan the rows for any
  tip whose Principle column matches the slice's primary
  concern. If any row is ⚠️ or ❌ in that area, the slice
  plan needs a regression-test bullet that closes the gap.
- **PR review** — for each modified file, the reviewer
  reads the rows for the spine principles the file
  touches. A file under `internal/db/` triggers §1.1 (DRY)
  + §1.5 (Design by Contract) + §1.11 (Program Deliberately).
- **Retrospective** (post-merge audit cadence, per issue
  #561) — when a bug lands in `main`, find the tip row that
  should have caught it, mark the row ❌ or ⚠️, and file a
  follow-up issue to close the gap.
- **Onboarding a new contributor** — the table is the *single
  document* that names every behavior the repo enforces.
  Reading §1 first (the spine) gives the *why*; reading §6
  second (the index) gives the *what*.

**State legend:**
- ✅ **Enforced** — a `CONTEXT.md` Law, a tier-1 `../docs/agents/` doc, a regression test, an ADR, or a workflow file pins the tip.
- ⚠️ **Partial** — the spirit is satisfied but no written rule, OR the rule exists but a recent commit violated it. Cited.
- ❌ **Gap** — the tip is not addressed and would be high-leverage to add. A candidate follow-up issue exists.
- ➖ **N/A** — the tip applies to contexts DixieData does not inhabit.

**Principle column** maps the tip to the §1.x section in this doc
that explains the *why*. Tips with a "philosophy / technique"
note in the Principle column are *attitudes* (Ch 1 of the book)
or *techniques* (Ch 3+), not principles per se — they inform
behavior but are not operationalized as a deep-module rule.

**Evidence / how to apply** is the *concrete recipe* column. It
names the repo surface that enforces the tip AND the action the
agent takes. Where the recipe fits in one line, it fits in one
line; where it needs two or three lines (a code snippet, a
step-by-step), the row carries it inline. The full evidence per
tip lives in the source audit at
`../docs/audit/pragmatic-programmer-audit-2026-07.md` (the
100-tip map retained as a historical artifact).

**Action / when violated** is the *trigger column*. It names the
*observable signal* that the tip has been violated — the thing
the agent should look for in a PR or in a code path before
shipping. "If you see X, the tip is violated; the fix is Y."
Where the trigger fits inline (most rows), it lives in the
evidence cell. Where it needs a long-form trigger (rare; one
or two rows), the multi-line content follows the row in a
"Per-tip notes" block.

The full evidence per tip lives in the source audit at
`../docs/audit/pragmatic-programmer-audit-2026-07.md` (the
100-tip map retained as a historical artifact).

| # | Tip | State | Principle | Evidence / how to apply |
|---|---|---|---|---|
| 1 | Care About Your Craft | ✅ | — (philosophy / technique, not a principle) | AGENTS.md "Bias toward action" + the entire quality regime: test floor per package, smoke probes per apply-site, doc-comment floor on every exported Go identifier, the `make freshness` gate before any push, the CHANGELOG discipline. *How to apply:* read AGENTS.md first thing in every session; if a session ended without updating AGENTS.md when a new pattern emerged, the next session has lost the rule. |
| 2 | Think! About Your Work | ✅ | — (philosophy / technique, not a principle) | RPCI flow (`../docs/agents/rpci.md`) explicitly turns off the autopilot: every plan has a Critique phase the user signs off on before code lands. *How to apply:* before writing the first line of a slice, write the slice plan (files touched, success criteria, regression net) into the issue body. If you can't write those three things, you haven't thought yet. |
| 3 | You Have Agency | ✅ | — (philosophy / technique, not a principle) | RPCI Critique gate is the operational form. The user has explicit "approve, start" authority over every plan; the agent never starts work without it. *How to apply:* if a slice feels blocked by a missing requirement, surface the decision in `ask_user_question` with 2-4 options, not by stalling. |
| 4 | Provide Options, Don't Make Lame Excuses | ✅ | — (philosophy / technique, not a principle) | RPCI surfaces "decisions to confirm" with options; the user picks, the agent never says "can't be done." *How to apply:* every blocker you hit becomes a 2-4 option question, not a refusal. "Can't do X" → "Can do A, B, or C; which?" |
| 5 | Don't Live with Broken Windows | ✅ | — (philosophy / technique, not a principle) | CONTEXT.md Laws are earned-by-real-bug rules. The Backend-First Law explicitly names the four "shipped but invisible" bug classes (per AGENTS.md §Backend-First Law). *How to apply:* when a slice ships a backend surface without a UI apply-site, the slice is *not done*. Open a follow-up issue the same commit. |
| 6 | Be a Catalyst for Change | ⚠️ | — (philosophy / technique, not a principle) | The repo does this implicitly via the audit cadence (issue #561 → #560) but has no written rule. Process is implicit. *When violated:* a stale doc, an undocumented pattern, or a workaround that nobody proposed a fix for. *How to apply:* when you find a broken window, file the follow-up issue + propose the slice in the same message; don't wait for someone else. |
| 7 | Remember the Big Picture | ✅ | — (philosophy / technique, not a principle) | The `../docs/agents/INDEX.md` progressive-disclosure table is the operational form. Agents know which tier to load for the task. *How to apply:* every session starts by reading `../docs/agents/INDEX.md`; the 3-tier table tells you whether to load a doc, just skim it, or skip it entirely. |
| 8 | Make Quality a Requirements Issue | ⚠️ | — (philosophy / technique, not a principle) | Every feature issue carries an "Acceptance criteria" section, but quality requirements are implicit (test-first, doc-comment floor, smoke probe). *When violated:* a slice ships without the test bullet or the smoke probe bullet in the plan. *How to apply:* the slice plan template (`feature-protocol.md`) makes test + smoke probe mandatory bullets; a plan without them is not a valid plan. |
| 9 | Invest Regularly in Your Knowledge Portfolio | ❌ | — (philosophy / technique, not a principle) | No `../docs/learning/` or per-agent reading list. The audit (this doc) is a one-shot, not a habit. *Open gap:* the 5 ❌ rows in this table identify the high-leverage follow-ups; until each lands, the audit doc itself is the closest the repo has to a learning artifact. *How to apply:* when a new book or framework is added to the stack, add a tip row that points at it. |
| 10 | Critically Analyze What You Read and Hear | ⚠️ | ⵏ1 WISDOM | The repo applies this in the "forgo following fads" framing (per the canonical pragmatics). *When violated:* a "let's rewrite in X" pitch that arrives without a stated problem to solve. *How to apply:* every architecture choice ships with a 1-line problem statement; a pitch without the problem is a red flag. Ask "what bug does this fix?" before adopting. |
| 11 | English is Just Another Programming Language | ✅ | ⵏ1.1 DRY | CONTEXT.md Laws are written in plain English with `_Avoid_` lists; treated as code. Every commit is reviewed for doc accuracy. *How to apply:* if a term is used in two places, one of them is wrong. Either rename to match the glossary or update the glossary + file the rename in the same commit. |
| 12 | It's Both What You Say and the Way You Say It | ✅ | ⵏ2.1 WISDOM | The CHANGELOG is exemplary: long-form bullets explain the *why*, the regression net, and the out-of-scope. *How to apply:* a commit message without the regression net line is incomplete; a CHANGELOG bullet without the *why* is too thin. |
| 13 | Build Documentation In, Don't Bolt It On | ✅ | ᵉfʳᵖˣʳ Writing | Doc-comment floor (CONTEXT.md "Exported Go identifiers carry doc comments" Law) + `internal/uiids` registry of surface IDs + the `internal/routebuilder` typed URL builder. Every exported identifier ships with a doc comment; every URL is a typed call. *When violated:* a new top-level URL string in a templ file (the architecture test fails the build). |
| 14 | Good Design Is Easier to Change Than Bad Design | ✅ | ⵏ1.3 Reversibility | The deep-module discipline (`../docs/agents/complexity.md`) + the architecture forbidden-import test = the rule. *How to apply:* a new module lands behind a small facade (`internal/<pkg>/` exports a single struct or interface); the architecture test refuses cross-package deep imports. If your slice needs to reach across 3 packages, the design is wrong; refactor first. |
| 15 | DRY—Don't Repeat Yourself | ✅ | ⵏ1.1 DRY | The routebuilder is the canonical example: single source of truth for every URL. `internal/routebuilder` + `internal/uiids` + `CONTEXT.md` + the components/ primitives are the DRY seams. *When violated:* a URL literal in a templ file (caught by `hx_guard_test.go`); a duplicated kind label across two switch statements (caught by `kind_label_test.go`); a vocabulary term with two spellings (caught by the glossary). *How to apply:* before writing the second copy, grep for the first; if the first exists, refactor before duplicating. |
| 16 | Make It Easy to Reuse | ✅ | ⵏ1.1 DRY | The `components/` design-system primitives (Foldout, Button, Card, Pill, EmptyState, Field, Toast) are the single source for visual patterns; per-component snapshot tests pin byte-stable output. *How to apply:* new visual pattern → new primitive in `internal/templates/components/` + per-component snapshot test. A templ file with a hand-rolled class string is a *new primitive waiting to be extracted*. |
| 17 | Eliminate Effects Between Unrelated Things | ✅ | ⵏ1.2 Orthogonality | `internal/architecture/architecture_test.go` forbidden-import table is the regression net. Deep-module list (`../docs/agents/complexity.md`) defines the package boundaries. *How to apply:* a new import that crosses a boundary fails the architecture test. Fix: move the type to a shared package or expose a facade method. |
| 18 | There Are No Final Decisions | ✅ | ⵏ1.3 Reversibility | The three-branch model (ADR 0009) is the operational form: `dev` for integration, `stable` for release, `main` is the frozen 2026-07-03 anchor. Releases can be rolled back by promoting an older `stable` tag. *When violated:* a direct push to `main` or `stable` (rejected by branch protection); a non-reversible migration without a downgrade SQL path (rejected by the migrations review). *How to apply:* every migration is classified Reversible / Partially / Non-Reversible in `../docs/migrations/reversibility.md`. |
| 19 | Forgo Following Fads | ⚠️ | ⵏ1.3 Reversibility | Implicit in the architecture choices (templ + chi + goldmark) but no written policy. *When violated:* a "rewrite in [framework]" pitch that arrives without a 1-line problem statement (per Tip #10). *How to apply:* the architecture choices are listed in `../docs/adr/`; a new framework needs its own ADR with the alternatives considered and the rejected option's rationale. |
| 20 | Use Tracer Bullets to Find the Target | ✅ | ⵏ1.4 Tracer Bullets | `tracer-bullets` skill (auto-loaded) + `feature-protocol.md` §"Tracer bullets" + the 3-tier commit rule. *How to apply:* every Tier 2 vertical slice is a tracer bullet. The slice's RED test is the "fires" check. If the bullet misses (the test is hard to write or passes for the wrong reason), the next slice adjusts the aim. |
| 21 | Prototype to Learn | ⚠️ | — (philosophy / technique, not a principle) | `.scratch/` is the prototype playground (Python scripts, MCP probes) and `repl` skill is a scratch tool. *When violated:* a 200-line prototype landed in `internal/` because nobody created the `.scratch/` shim first. *How to apply:* if the slice's success criteria includes "learn whether X is feasible," the prototype lives in `.scratch/`; only the proven mechanism ports to `internal/`. |
| 22 | Program Close to the Problem Domain | ✅ | — (philosophy / technique, not a principle) | The `articles` package (just shipped in #565) is the exemplar: domain types `CheatSheetRow`, not infrastructure types `MarkdownRow` + `CopyAction` + `Tokenizer`. *When violated:* an `internal/articles/article_infra.go` file with no domain terms in the type names. *How to apply:* the type name should read like the requirement ("Person Record", "Display ID", "Article Snapshot"), not like the implementation. |
| 23 | Estimate to Avoid Surprises | ⚠️ | — (philosophy / technique, not a principle) | RPCI Plan includes "files touched, success criteria, regression net" but no time estimate. The "Bias toward action" rule absorbs this. *When violated:* a slice ships that touches 15+ files when the plan estimated 3. *How to apply:* if a slice's actual footprint is 5x the estimate, the slice is doing two things; split it. |
| 24 | Iterate the Schedule with the Code | ➖ | — (philosophy / technique, not a principle) | N/A. Single-binary single-user project; no schedule to iterate. (See Tip #85 — the cadence is operational, not scheduled.) |
| 25 | Keep Knowledge in Plain Text | ✅ | — (philosophy / technique, not a principle) | Every config is plain text (Makefile, .json, .md, .ps1, .yml). No binary configs. `../docs/migrations/v{N}.md` is the changelog for schema changes. *When violated:* a binary blob in the repo (git LFS exception is the only allowed case, and there are none). |
| 26 | Use the Power of Command Shells | ✅ | — (philosophy / technique, not a principle) | `audit/_lib/`, `../scripts/*.ps1`, `tools/tune/cli-coverage.mjs`, `make` targets. The `probe-clean.ps1` + `run-crash-dump.ps1` are the operational form. *How to apply:* if a step needs to run twice, it belongs in a script + a `make` target. Manual repetition is the violation. |
| 27 | Achieve Editor Fluency | ➖ | — (philosophy / technique, not a principle) | Agent-context; not a code/doc concern. |
| 28 | Always Use Version Control | ✅ | — (philosophy / technique, not a principle) | The entire branching model (ADR 0009): dev for integration, stable for release, main frozen 2026-07-03. *How to apply:* every change goes through git; every commit has a message; every PR has a body. A change outside git is not a change. |
| 29 | Fix the Problem, Not the Blame | ⚠️ | — (philosophy / technique, not a principle) | Implicit in CHANGELOG tone ("two compounding bugs in X" — no person named) but no stated rule. *When violated:* a commit message names a person ("@alice broke this"). *How to apply:* a fix commit names the *bug*, the *root cause*, and the *regression net*. Person names never appear in commit messages. |
| 30 | Don't Panic | ➖ | — (philosophy / technique, not a principle) | N/A. No incident response. |
| 31 | Failing Test Before Fixing Code | ✅ | — (philosophy / technique, not a principle) | `../docs/agents/tdd.md` red-green-refactor is the operational form. *How to apply:* the first commit in a bug-fix slice is the failing test; the second is the fix that turns it green. A fix without a preceding failing test is a Tier 3 maintenance commit, not a Tier 2 vertical. |
| 32 | Read the Damn Error Message | ✅ | ⵏ1.6 Dead Programs | The audit probes assert the response shape AND the post-click URL AND the DOM state (not "we got an error somewhere"). *How to apply:* a panic with a clear message is the signal; a swallowed panic with a generic toast is the violation. The dialog-guard mutex, the `htmxattr.Mux` panic on invalid swap, the deep-module doc-comment floor all enforce "say what failed." |
| 33 | "select" Isn't Broken | ⚠️ | ⵏ1.6 Dead Programs | Implicit (no recent commit blamed SQLite or WebView2 without evidence) but no stated rule. *When violated:* a commit message like "SQLite is broken" or "WebView2 ate my bytes" without a stack trace, a reproduction, or a minimal test. *How to apply:* the bug is in our code until proven otherwise; the proof requires the failing test first. |
| 34 | Don't Assume It—Prove It | ✅ | ⵏ1.6 Dead Programs | The `tools/tune/snapshot_test.go` per-iter SQL footprint doc-comment pattern is the operational form: every non-trivial function documents the count of SELECTs, INSERTs, and transactions the test actually performs. *When violated:* a function's doc-comment has no SQL footprint (the test is incomplete). |
| 35 | Learn a Text Manipulation Language | ➖ | — (philosophy / technique, not a principle) | N/A at the repo level. |
| 36 | You Can't Write Perfect Software | ✅ | ⵏ1.5 Design by Contract | The dialog-guard Law (CONTEXT.md) is the operational form. The "Fail loud, no silent fallback" decision in #117 is the worked example. *How to apply:* every error path either crashes early or surfaces a clear toast; a silent fallback (returning `nil, nil` on error, swallowing a panic) is the violation. |
| 37 | Design with Contracts | ✅ | ⵏ1.5 Design by Contract | `../docs/agents/tdd.md` §Contract touch is the prospective function- and boundary-contract rule; `internal/architecture/architecture_test.go` enforces package contracts. *How to apply:* every new or materially changed public seam documents relevant caller obligations, observable guarantees, failure/state semantics, and retry/concurrency behavior; its RED test proves those claims. Skip mechanical edits and untouched code. Prefer types, constraints, typed errors, and tests over generic runtime assertion helpers. |
| 38 | Crash Early | ✅ | ⵏ1.5 Design by Contract | `htmxattr.Mux` swap-allowlist panic at render time on an invalid swap value — the panic is the signal. The native `<dialog>` revert (#117) is the worked example. *How to apply:* an unreachable code path should panic, not silently return. The panic is the agent's "the world is broken" signal; swallowing it is the violation. |
| 39 | Use Assertions to Prevent the Impossible | ✅ | ⵏ1.5 Design by Contract | `templ.Attributes` typed spread + `routebuilder` typed URL builder + `internal/uiids` registry IDs prevent invalid states at construction time. The `htmxattr.Mux` builder pre-validates its values. *How to apply:* prefer types, builders, database constraints, validation, and typed errors. Use a runtime panic only for a developer-created impossible state; never panic for ordinary user input, recoverable I/O, or third-party failure. |
| 40 | Finish What You Start | ✅ | — (philosophy / technique, not a principle) | Go's `defer` for resource close + the defer-close lint rule. *How to apply:* every `Open()` / `Lock()` / goroutine launch is paired with `defer Close()` / `defer Unlock()` / a context-aware wait. A resource opened without a defer is the violation. |
| 41 | Act Locally | ✅ | — (philosophy / technique, not a principle) | Function-scope variables; the Wails v2.12.0 dialog-guard mutex is a per-handler-scope guard. *When violated:* a package-level mutable variable (the architecture test flags it). *How to apply:* a `var foo = ...` at the package level is the violation; pass the value into the function or hold it on the `App` struct. |
| 42 | Take Small Steps—Always | ✅ | ⵏ1.10 Take Small Steps | RPCI is the operational form. Slice 1 is always the tracer bullet. *How to apply:* a slice plan that lists >5 files touched is doing two things; split it. The fresh-context-per-slice rule in `rpci.md` makes "ship the smallest useful unit" the default. |
| 43 | Avoid Fortune-Telling | ✅ | — (philosophy / technique, not a principle) | The "YAGNI" rule in `../docs/agents/feature-protocol.md` §"Module discipline" + the two-adapter rule are the operational form. *When violated:* a slice adds an interface or a config field "for the future." *How to apply:* the rule is "no feature PR ships a parameter that no caller passes." Delete the parameter or file the follow-up issue. |
| 44 | Decoupled Code Is Easier to Change | ✅ | ⵏ1.7 Law of Demeter | The deep-module discipline + architecture test. The v60 → v61 Event Records refactor (sibling-table rename) is the worked example. *How to apply:* a chain like `result := a.B().C().D()` is a Demeter violation; pass the value or expose a facade method. |
| 45 | Tell, Don't Ask | ⚠️ | ⵏ1.7 Law of Demeter | The DTO discipline (UI depends on service DTOs, never on persistence structs) is the closest form. The rule is "the handler asks the service for the data, not the model." *When violated:* a templ file imports `internal/records` directly. *How to apply:* the `internal/viewmodel/` package is the seam; expose a DTO, not the persistence struct. |
| 46 | Don't Chain Method Calls | ➖ | ⵏ1.7 Law of Demeter | N/A in Go. The law of demeter is implicit in the deep-module rule. |
| 47 | Avoid Global Data | ✅ | ⵏ1.2 Orthogonality | The "no package-level mutable state" rule (implicit in the deep-module discipline) + Wails' `App` struct is the single source of shared state. *When violated:* a `var foo = make(...)` at package level. *How to apply:* hold the value on the `App` struct; pass the `App` pointer into every consumer. |
| 48 | If It's Important Enough To Be Global, Wrap It in an API | ✅ | ⵏ1.2 Orthogonality | The `a.guardedSaveFileDialog` wrapper for native dialogs is the canonical example: the dangerous thing (the OS dialog) is wrapped in an API that adds re-entry protection. *How to apply:* if the global is unavoidable (a system handle, a process-wide config), wrap it in a struct method that adds the safety invariant. |
| 49 | Programming Is About Code, But Programs Are About Data | ✅ | ⵏ1.2 Orthogonality | The viewmodel layer is the operational form: every cross-boundary value is a DTO. The audit probes assert on data shape, not on code paths. *How to apply:* a handler that returns a service struct is the violation; the handler returns a viewmodel DTO, the viewmodel maps the service struct. |
| 50 | Don't Hoard State; Pass It Around | ✅ | ⵏ1.2 Orthogonality | Function arguments over package-level state. The Markdown cheatsheet work (#565) is the recent example — the cheatsheet is constructed from explicit args, not read from package globals. *When violated:* a function reads from a package-level variable instead of taking the value as a parameter. |
| 51 | Don't Pay Inheritance Tax | ➖ | ⵏ1.2 Orthogonality | N/A in Go (no inheritance). Composition is the only path; the rule is implicit. |
| 52 | Prefer Interfaces to Express Polymorphism | ✅ | ⵏ1.2 Orthogonality | Go interfaces are the operational form: services expose behavior through interface types, not concrete structs. The `internal/diagnostics` + `internal/archive` boundaries are interface seams. *How to apply:* a consumer that imports the concrete service struct is the violation; the consumer imports an interface, the Wails binding wires the concrete. |
| 53 | Delegate to Services: Has-A Trumps Is-A | ➖ | — (philosophy / technique, not a principle) | N/A in Go. Composition is the only path; the rule is implicit and trivially satisfied by every Wails binding. |
| 54 | Use Mixins to Share Functionality | ➖ | — (philosophy / technique, not a principle) | N/A in Go (no mixins). Embedding structs is the closest analog; the dialog-guard mutex and the per-handler `inFlight` pattern are embedded once and reused. *When violated:* a struct copy-pastes a method instead of embedding the type that owns it. |
| 55 | Parameterize Your App Using External Configuration | ✅ | — (philosophy / technique, not a principle) | `internal/local_settings` is the external config store (user-tunable settings); `Makefile` + `.github/workflows/*.yml` are the build-time config. No value the app depends on is hardcoded. *How to apply:* a constant in Go code that the user might want to change is the violation; move it to `local_settings` or to the `Makefile`. |
| 56 | Analyze Workflow to Improve Concurrency | ✅ | ⵏ1.9 Temporal Coupling | The export flow (PDF / JSON / archive) is jobs-based; the user gets a toast + the jobs page updates in parallel. The `X-DixieData-Toast` contract decouples the click from the work. *How to apply:* a handler that does the work inline (blocks the response for >500ms) is the violation; move the work to a job, return a 202 with a job ID. |
| 57 | Shared State Is Incorrect State | ✅ | — (philosophy / technique, not a principle) | The dialog-guard mutex + the Wails `App` struct pattern + the per-job worker context. `internal/jobs/jobs.go` is the canonical example of state ownership. *How to apply:* a goroutine that reads/writes a package-level variable is the violation; pass a `*App` pointer, the goroutine owns no state. |
| 58 | Random Failures Are Often Concurrency Issues | ✅ | — (philosophy / technique, not a principle) | `audit/race-stress.yml` workflow + the `internal/dates` property-test gate. The issue #479 advisory-downgrade race was caught by the workflow, not the unit tests. *How to apply:* a flaky test is concurrency; run with `-race` before assuming the test is broken. |
| 59 | Use Actors For Concurrency Without Shared State | ⚠️ | — (philosophy / technique, not a principle) | The Wails `App` is a struct passed by reference; not technically an actor. The "jobs.Start" pattern in `internal/jobs` is the closest operational form. *When violated:* a goroutine that mutates a value owned by the caller. *How to apply:* the rule is "the goroutine owns its state; the caller passes inputs and reads outputs through channels or a return-only interface." |
| 60 | Use Blackboards to Coordinate Workflow | ➖ | ⵏ1.9 Temporal Coupling | N/A. Single-user app; no blackboard pattern. |
| 61 | Listen to Your Inner Lizard | ⚠️ | ⵏ1.9 Temporal Coupling | Implicit. The "agent inner lizard" surfaced in the 6-commit over-decomposition anti-pattern (AGENTS.md "Commits and branches"). *When violated:* the slice plan says "I'll figure out the shape as I go." *How to apply:* if a plan feels wrong, the plan is wrong; surface the concern in the slice plan's "Principle warnings" block. |
| 62 | Don't Program by Coincidence | ✅ | ⵏ1.9 Temporal Coupling | The `internal/dates` property-test gate is the operational form. The CHANGELOG entry tone is "root cause: X; fix: Y" — never "happened to work." *When violated:* a commit message that doesn't explain *why* the fix works. *How to apply:* the slice plan's "regression net" line names the test that proves the fix; a fix without that line is coincidence. |
| 63 | Estimate the Order of Your Algorithms | ⚠️ | ⵏ1.12 Algorithm Speed | The `TestStressEventAttachDetachRoundTrip` per-iter SQL footprint is the closest form. Not a stated rule for non-DB code paths. *When violated:* a function in `internal/records/` or `internal/db/` whose doc-comment has no SQL footprint. *How to apply:* the doc-comment lists "O(N) reads, O(1) writes"; a doc-comment without the footprint is incomplete. |
| 64 | Test Your Estimates | ✅ | ⵏ1.12 Algorithm Speed | `tools/tune/stress/` + `.github/workflows/race-stress.yml`. The race-detector step is the live regression net that catches drift between the estimate and reality. *How to apply:* every slice that touches `internal/records/` or `internal/db/` runs the stress suite locally before the PR opens. |
| 65 | Refactor Early, Refactor Often | ✅ | ⵏ1.13 Refactoring | `../docs/agents/complexity.md` is the operational form. The v60 → v61 Event Records refactor is the worked example. *How to apply:* the slice plan's "When to refactor" checklist (5 triggers: DRY violation, non-orthogonal, knowledge improved, requirements evolve, performance) — a slice that hits one trigger files the refactor as a sibling issue. |
| 66 | Testing Is Not About Finding Bugs | ✅ | ⵏ1.13 Refactoring | The audit cadence is the operational form. Issues #531, #539, #540, #542 each surfaced a *class* of bug the tests themselves couldn't catch. *How to apply:* the audit probe (Playwright) is the second test surface; a slice that passes unit tests but fails the smoke probe is incomplete. |
| 67 | A Test Is the First User of Your Code | ✅ | ⵏ1.13 Refactoring | `tdd.md` Step 1 is "RED: write the failing test first." The Markdown cheatsheet work (#565) shipped RED + GREEN as a single reviewable commit. *How to apply:* the slice plan's first commit is the failing test; the second is the fix. A slice with no RED commit is not TDD. |
| 68 | Build End-To-End, Not Top-Down or Bottom Up | ✅ | ⵏ1.4 Tracer Bullets | RPCI tracer-bullet slice 1 is the operational form. Every Tier 2 vertical slice crosses every layer (templ + htmx + JS + Go handler + DB). *When violated:* a "backend slice" that ships the handler but no UI, or a "frontend slice" that ships the templ but no handler. *How to apply:* the slice plan template requires both the apply-site URL and the smoke probe bullet; a plan without both is not Tier 2. |
| 69 | Design to Test | ✅ | ⵏ1.11 Program Deliberately | `internal/architecture/architecture_test.go` + `audit/discover_orphan_handlers.mjs` + the smoke-probe per-apply-site contract — three independent test surfaces. *How to apply:* a new handler without an audit probe entry is the violation; the audit probe is the test that catches the "handler returns 200 but renders nothing" bug class. |
| 70 | Test Your Software, or Your Users Will | ✅ | ⵏ1.4 Tracer Bullets | The smoke-probe per-apply-site contract (per `feature-protocol.md` "Backend-First Law" + `tdd.md` "Per-layer recipes") catches the "shipped but invisible" bug class. *When violated:* a feature ships without the smoke probe bullet; the user finds the bug. *How to apply:* the slice plan template lists smoke probes per apply-site; a missing probe is the violation. |
| 71 | Use Property-Based Tests to Validate Your Assumptions | ✅ | ⵏ1.17 Great Expectations | `internal/dates/dates_property_test.go` uses `pgregory.net/rapid`. Documented in `tdd.md` §"Per-layer recipes." *How to apply:* a function whose doc-comment claims "handles edge cases X, Y, Z" without a property test is the violation; the property test enumerates the input space. |
| 72 | Keep It Simple and Minimize Attack Surfaces | ✅ | — (philosophy / technique, not a principle) | The "no new component primitives" rule from the Markdown cheatsheet work (#565) is the recent example. The deep-module discipline is the standing operational form. *When violated:* a slice adds a new templ primitive when an existing one (Foldout, Pill, EmptyState) would carry the case. *How to apply:* before adding a new primitive, grep for existing primitives + read the components/conventions.md doc. |
| 73 | Apply Security Patches Quickly | ❌ | — (philosophy / technique, not a principle) | No `govulncheck` or `gosec` in `Makefile` / `.github/workflows/`. Deps are updated when a PR forces it, not on a cadence. *Open gap:* add `govulncheck ./...` to a weekly workflow; add `gosec ./...` to the audit chain. *How to apply until then:* `go list -m -u all` every Friday; file a follow-up issue per dependency that has a CVE. |
| 74 | Name Well; Rename When Needed | ⚠️ | — (philosophy / technique, not a principle) | The glossary tier-2 rename (issue #97) is the canonical example. *When violated:* a new type name that's a synonym for an existing one ("Worker" + "JobRunner" + "TaskProcessor" in the same package). *How to apply:* the slice plan's "files touched" line lists the new name; if the name is a synonym, rename one of them in the same slice. |
| 75 | No One Knows Exactly What They Want | ✅ | — (philosophy / technique, not a principle) | RPCI: "Plan → Critique" with explicit user gate. The "the user is the chat" capture rule in AGENTS.md. *How to apply:* a slice plan that ships without a user sign-off is the violation; the Plan phase ends only when the user types "approved". |
| 76 | Programmers Help People Understand What They Want | ✅ | — (philosophy / technique, not a principle) | The slice plan + apply-sites checklist in every feature issue is the operational form. The user sees the *full shape* of the change before code lands. *How to apply:* the slice plan template lists the apply-sites (which pages the change touches) and the smoke probe; if either is missing, the user can't visualize the change. |
| 77 | Requirements Are Learned in a Feedback Loop | ✅ | — (philosophy / technique, not a principle) | RPCI's "fresh context per slice" is the strongest form. The next session picks up the new state and refines. *When violated:* a slice ships with a "complete" spec that was never re-validated. *How to apply:* every slice ends with a `make freshness` + a `make audit` run; the next session's slice plan re-reads the previous slice's CHANGELOG bullet and asks "does this still match what the user said?" |
| 78 | Work with a User to Think Like a User | ⚠️ | — (philosophy / technique, not a principle) | Implicit in the user's "go ahead and tackle 565" pattern, but no stated cadence. The audit cycle (issue #561 → #560) is the closest scheduled form. *When violated:* a slice ships without the user manually clicking through the change on a real archive. *How to apply:* the slice plan's "Verification" line lists the manual click-through; a slice without it is incomplete. |
| 79 | Policy Is Metadata | ✅ | ⵏ1.8 Metaprogramming | The `internal/uiids` registry + the architecture test forbidden-import map + the linter rule files are the operational form. Policy lives in data, not code. *When violated:* a hardcoded list of valid swap targets in the templ layer (the data should live in `internal/uiids`). *How to apply:* the rule is "if a value appears in 3+ places, it lives in a registry." |
| 80 | Use a Project Glossary | ✅ | ⵏ1.8 Metaprogramming | `CONTEXT.md` is the project glossary. The "Flagged ambiguities" section is the regression net for vocabulary drift. *How to apply:* a new term in a doc or comment that isn't in `CONTEXT.md` is the violation; add the term + the `_Avoid_` list to `CONTEXT.md` in the same commit. |
| 81 | Don't Think Outside the Box—Find the Box | ✅ | — (philosophy / technique, not a principle) | The slice-3.6 → slice-4 markdown editor + preview modal rework (issue #526) is the worked example — instead of escaping the "no WYSIWYG" box, the team found the smaller box the tool could ship. *How to apply:* §2.4 Cutting the Gordian Knot is the 6-question checklist; a slice that answers "yes" on Q6 ("don't have to do it at all") files the abandonment as a sibling issue. |
| 82 | Don't Go into the Code Alone | ⚠️ | — (philosophy / technique, not a principle) | Implicit (the user + agent pairing) but no stated rule. The `.rpiv/artifacts/issues/` + `.scratch/issues/` ad-hoc review threads are the closest form. *How to apply:* a slice that ships without a PR description is the violation; the PR body is the conversation that catches the "you missed X" finding. |
| 83 | Agile Is Not a Noun; Agile Is How You Do Things | ➖ | — (philosophy / technique, not a principle) | N/A. (Solo project; no team-level agile ceremony. The *spirit* is satisfied by the slice cadence + the "ship the smallest useful unit" discipline.) |
| 84 | Maintain Small Stable Teams | ➖ | — (philosophy / technique, not a principle) | N/A. Solo project. |
| 85 | Schedule It to Make It Happen | ⚠️ | ⵏ1.15 Ubiquitous Automation | The `make freshness` gate + the daily `make test` + the CHANGELOG cadence are the closest form. But "scheduled reflection / learning" is not yet on the calendar. *When violated:* a quality improvement ships behind a feature because nobody scheduled the work. *How to apply:* the slice plan template lists a "Cadence" line; a cadence without a calendar entry is the violation. |
| 86 | Organize Fully Functional Teams | ➖ | ⵏ1.15 Ubiquitous Automation | N/A. |
| 87 | Do What Works, Not What's Fashionable | ⚠️ | — (philosophy / technique, not a principle) | Implicit in the architecture choices (no React, no ORM, no microservices). No stated policy. *When violated:* a "rewrite in [framework]" pitch without a 1-line problem statement. *How to apply:* the architecture choices are listed in `../docs/adr/`; a new framework needs its own ADR with the alternatives considered and the rejected option's rationale. |
| 88 | Deliver When Users Need It | ✅ | — (philosophy / technique, not a principle) | The Tier 3 apply-site rule + the slice-1-only-per-session rule = ship the smallest useful unit as fast as possible. *When violated:* a 2-week polish cycle that ships nothing. *How to apply:* the slice plan's "Verification" line lists the manual click-through; the slice ships when the click-through is green. |
| 89 | Use Version Control to Drive Builds, Tests, and Releases | ✅ | — (philosophy / technique, not a principle) | `.github/workflows/{build,test,audit,race-stress}.yml` are the operational form. Every PR triggers the full chain. *When violated:* a manual build step that runs outside CI. *How to apply:* the workflow YAML files are the source of truth; if a step isn't in a workflow, it doesn't run. |
| 90 | Test Early, Test Often, Test Automatically | ✅ | ⵏ1.15 Ubiquitous Automation | Per-PR + per-push + weekly race-stress + per-promotion freshness. *How to apply:* a slice that ships without a CI-green check is the violation; the PR is not mergeable until the green check is in the PR body's status line. |
| 91 | Coding Ain't Done 'Til All the Tests Run | ✅ | ⵏ1.15 Ubiquitous Automation | The "RED + GREEN + adjacent-behavior sweep" in `tdd.md` + the package-floor + the smoke-probe per-apply-site contract. *When violated:* a slice ships with one passing test + one skipped test + one failing test. *How to apply:* the slice's PR description lists every test status; a slice without the full matrix is not mergeable. |
| 92 | Use Saboteurs to Test Your Testing | ❌ | — (philosophy / technique, not a principle) | No mutation testing. The `tools/tune/` golden-snapshot tests catch regressions but not silent test-skipping. *Open gap:* add a mutation testing step to the audit chain (e.g., `go-mutesting` for Go, `stryker` for JS). *How to apply until then:* every PR review asks "does this test actually catch the bug it's named for?"; if the test is "expect sum of [1,2] == 3," the test is too trivial to trust. |
| 93 | Test State Coverage, Not Code Coverage | ⚠️ | — (philosophy / technique, not a principle) | The smoke probes assert state (response shape, URL, DOM) not just code paths. But there is no coverage metric on the state surface yet. *When violated:* a test that asserts the handler returns 200 but never reads the response body. *How to apply:* the audit probe template (`audit/_lib/`) asserts the full page state; a probe that asserts only the status code is incomplete. |
| 94 | Find Bugs Once | ✅ | ⵏ1.15 Ubiquitous Automation | Every `fix:` commit in CHANGELOG grows a regression test. The 79 `fix:` commits with regression nets in `../docs/COMMON_BUGS.md` are the audit trail. *How to apply:* the slice plan's "regression net" line names the test; a fix commit without a regression test name is the violation. |
| 95 | Don't Use Manual Procedures | ✅ | ⵏ1.15 Ubiquitous Automation | `make freshness` automates the build + probe chain. `make audit` automates the audit sweep. `make promote` automates the gate chain. *When violated:* a step in the README that says "run X then Y then Z." *How to apply:* every multi-step procedure belongs in a script + a `make` target; a README without a script is the violation. |
| 96 | Delight Users, Don't Just Deliver Code | ✅ | ⵏ1.17 Great Expectations | The Markdown cheatsheet work (#565), the feedback modal (#544), the bug-report bundle (#545) are the recent examples — each ships a *surprise* the user didn't ask for. *When violated:* a slice ships the minimum required by the spec and nothing else. *How to apply:* the slice plan's "Verification" line lists the manual click-through; a click-through that ends with "works as specified" but no extra good moment is the violation. |
| 97 | Sign Your Work | ✅ | — (philosophy / technique, not a principle) | The CHANGELOG fixship-by-fixship attribution is the operational form. The "Ticket close-out law" in `feature-protocol.md` requires the commit hash + verification output + acceptance criteria on every closing commit. *How to apply:* every closing commit ends with a `Verification:` block that pastes the `make test` output; a closing commit without the block is unsigned. |
| 98 | First, Do No Harm | ✅ | — (philosophy / technique, not a principle) | The dialog-guard Law + the "no feature PR ships a backend surface without a UI apply-site" Law + the per-iter SQL footprint pattern. *When violated:* a slice that breaks a non-target surface (e.g., the new Inventory Metrics breaks the Articles headline count). *How to apply:* the slice plan's "regression net" line lists every adjacent surface the change could touch; a slice without that list is a harm candidate. |
| 99 | Don't Enable Scumbags | ➖ | — (philosophy / technique, not a principle) | N/A. |
| 100 | It's Your Life. Share it. Celebrate it. Build it. AND HAVE FUN! | ✅ | — (philosophy / technique, not a principle) | The CHANGELOG tone + the "Bias toward action" rule + the user's "go ahead and tackle X" cadence. The repo is built by a human who cares. *How to apply:* when the work feels like a slog, the slog is the signal; surface the concern, ship a smaller slice, take a break. |


## §7 — Summary: the 100 tips in numbers

| State | Count | What it means |
|---|---|---|
| ✅ Enforced | 66 | The repo is in compliance. The principle spine in §1 names the operational form. |
| ⚠️ Partial | 18 | The spirit is satisfied but no written rule. A future commit could regress without the agent noticing. |
| ❌ Gap | 3 | Not addressed. Each gap is a candidate follow-up issue; see §1 in this doc for the principle-level gaps. |
| ➖ N/A | 13 | Doesn't apply (multi-team management, hiring, incident response, etc.). |

The 100-tip audit is a **check**, not a refactor. The principle
spine (§1) is the primary lens for future work; the tip index
(§6, this section) is the primary lens for checking specific
commits against the canonical book.

When a new tip is added to a future edition of the book, append
a row to this index. When a tip's state changes (a new
commit pushes a ⚠️ to ✅ or a ✅ to ⚠️), update the State column.
The index is the **source of truth** for the per-tip state; the
audit doc is the per-tip *evidence*.


## §8 — References

- _The Pragmatic Programmer, 20th Anniversary Edition_, Hunt &
  Thomas. The 100 tips are excerpted at
  <https://pragprog.com/tips/>.
- The principle spine + 11 book-end checklists are summarized
  at
  <https://github.com/HugoMatilla/The-Pragmatic-Programmer#checklist>.
- The 2-pass audit that produced this doc:
  `../docs/audit/pragmatic-programmer-audit-2026-07.md` (100-tip
  view, retained as a historical artifact) +
  `../docs/audit/pragmatic-programmer-principles-audit-2026-07.md`
  (principle spine, the primary source).
- This doc is self-contained: the 100-tip index lives in §6
  (the *what*) and the 4-state summary in §7. An agent reading
  the principle spine can check specific commits against the
  tip index without leaving this file.
- The tier-1 process docs this doc cross-references:
  - `feature-protocol.md` — slice discipline + Tier 1-3 commit rule
  - `tdd.md` — RED/GREEN/REFACTOR
  - `rpci.md` — RPCI flow
  - `complexity.md` — deep-module + YAGNI
  - `build-protocol.md` — freshness gate
  - `architecture/architecture_test.go` — forbidden-import test
  - `CONTEXT.md` §"Laws" — the earned-by-real-bug operational form

---

## §9 — Book chapter → tip cross-reference

The principle spine (§1) is *principle-organized*; the tip index
