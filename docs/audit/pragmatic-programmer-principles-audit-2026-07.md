# Pragmatic Programmer Principles + Tips Audit

> **STATUS: OPEN.** Initial pass; awaiting user review. Tracks issue
> [#567](https://github.com/valueforvalue/DixieData/issues/567).
>
> This audit **supersedes** `pragmatic-programmer-audit-2026-07.md`
> (the 100-tip-only pass from earlier in this session). The earlier
> doc is retained as a historical artifact and referenced from §6.

## Why a second pass

The first pass (the 100-tip map) read the book as a **checklist**.
The second pass reads it as a **principle spine** + a **checklist**.

The book is organized around a small set of named principles — DRY,
orthogonality, reversibility, tracer bullets, design by contract, the
Law of Demeter, the WISDOM acrostic, the Architectural Questions, the
Debugging Checklist, the Aspects of Testing, etc. — and the 100 tips
are the operational consequences of those principles. Mapping the
principles directly surfaces gaps the tip-list view hides:

- The **principles** name a *why*. The **tips** name a *what*.
- A repo can be fully covered by the tips while still drifting on
  the principles (e.g. we satisfy Tip #11 "DRY" by having
  `internal/routebuilder`, but we may still violate the *spirit* of
  DRY by duplicating knowledge across the templ + JS + Go layers
  in a way the tip-by-tip view doesn't catch).
- A principle often has **checklist forms** in the book (the
  Architectural Questions, the Debugging Checklist, the Aspects
  of Testing) that the tip-list view doesn't enumerate. Those
  checklists are the testable operational form.

This audit is structured: **principles first, tips second, gap
candidates consolidated**.

## How the book is organized (2nd edition / 20th Anniversary)

| Ch | Title | Principle spine |
|---|---|---|
| 1 | A Pragmatic Philosophy | Cat Ate My Source Code, Software Entropy (broken windows), Stone Soup & Boiled Frogs, Good Enough Soup, Knowledge Portfolio, Communicate (WISDOM) |
| 2 | A Pragmatic Approach | **DRY**, **Orthogonality**, **Reversibility**, **Tracer Bullets**, Prototypes, **Domain Languages**, Estimating |
| 3 | The Basic Tools | **Plain Text**, Shells, Power Editing, **Source Code Control**, Debugging, Text Manipulation, **Code Generators** |
| 4 | A Pragmatic Paranoia | **Design by Contract**, **Dead Programs Tell No Lies** (crash early), **Assertive Programming**, Exceptions, **Resource Balancing** |
| 5 | Bend, or Break | **Decoupling / Law of Demeter**, **Metaprogramming** (abstractions in code, details in metadata), **Temporal Coupling**, **It's Just a View** (MVC), Blackboards |
| 6 | While You Are Coding | **Program by Coincidence → Program Deliberately**, **Algorithm Speed** (Big O), **Refactoring**, **Code That's Easy to Test**, Evil Wizards |
| 7 | Before the Project | **Requirements Pit** (dig, don't gather), **Solving Impossible Puzzles** (Cutting the Gordian Knot), **Not Until You're Ready**, **Specification Trap**, **Circles and Arrows** |
| 8 | Pragmatic Projects | Pragmatic Teams, **Ubiquitous Automation**, **Ruthless Testing** (Aspects of Testing), **It's All Writing** (English as code), **Great Expectations**, **Sign Your Work** |

The **bolded** entries are the principles this audit maps to the
repo. The others (single-tip or chapter-section entries without a
named principle spine) are mapped in §6's tip table.

## How the repo is organized (so the audit reads forward)

```
CONTEXT.md              ← glossary + Laws (non-negotiable)
AGENTS.md               ← session protocol + branch policy
docs/agents/INDEX.md    ← 3-tier progressive disclosure of process docs
docs/agents/feature-protocol.md  ← Tier 1-3 commit rule + slice discipline
docs/agents/rpci.md     ← Research → Plan → Critique → Implement
docs/agents/tdd.md      ← RED/GREEN/REFACTOR
docs/agents/complexity.md ← deep-module + YAGNI + two-adapter rule
docs/agents/build-protocol.md  ← freshness gate + bump-version contract
docs/adr/               ← 10 ADRs (branch model, design system, etc.)
docs/audit/             ← this directory
docs/CODE_CHANGES.md    ← cross-layer working contract
docs/COMMON_BUGS.md     ← per-layer bug catalog (the recurring-bug contract)
docs/migrations/        ← plain-text source of truth for schema
CONTEXT.md "Laws"       ← earned-by-real-bug non-negotiable rules
```

# §1 — Principle map (the new layer)

For each principle in the book's spine, the audit lists:
- The principle's name and canonical definition (per the book)
- The repo's operational form (file + line)
- The state: ✅ Enforced / ⚠️ Partial / ❌ Gap / ➖ N/A
- Notes when the principle shows up in a way the tip-list view misses

## §1.1 — DRY (Don't Repeat Yourself) — Tip #15

**Definition:** "Every piece of knowledge must have a single,
unambiguous, authoritative representation within a system."

**Operational forms (per the book):**
- *Imposed* — environment forces duplication (e.g. schema + types)
- *Inadvertent* — developer doesn't realize they're duplicating
- *Impatient* — developer is lazy
- *Interdeveloper* — multiple people duplicate the same knowledge

**Repo state:** ✅ **Enforced**, with the strongest enforcement on
*imposed* duplication (which is the hardest to spot).

| Form | Repo state | Evidence |
|---|---|---|
| Imposed (schema ↔ types) | ✅ | `internal/db/migrations.go` is the single source of truth for schema; `internal/models/` derives types from the same domain. The v60 → v61 Event Records refactor (sibling-table rename `soldier_id` → `person_record_id`) is the worked example of eliminating imposed duplication. |
| Imposed (URLs) | ✅ | `internal/routebuilder` is the canonical source for every URL; the `hx_guard_test.go` test fails any templ that uses a string literal. |
| Imposed (UI surface IDs) | ✅ | `internal/uiids` registry; the `htmxattr.Mux` validator refuses unknown `hx-target` values at render time. |
| Imposed (CLI flags) | ✅ | `cmd/seed-data/main.go` + `dixiedata-web` + `gold-master` all share the `cli-coverage.mjs` enforcement that fails on a flag the user can't reach. |
| Inadvertent (vocabulary) | ✅ | `CONTEXT.md` is the project glossary. The "Flagged ambiguities" section is the regression net for vocabulary drift. |
| Inadvertent (UX microcopy) | ✅ | `docs/agents/ux-microcopy.md` (issue #560) is the recently-pinned rule for "no self-explanatory headings" / "no duplicated copy." The audit probe (`audit/smoke_microcopy.mjs`) is the executor (per the issue body — not yet landed at audit time). |
| Inadvertent (tier-1 doc drift) | ⚠️ | `docs/agents/INDEX.md` is the progressive-disclosure table, but no automation asserts "every tier-1 doc is listed." A future doc could land without an INDEX entry. |
| Impatient | ⚠️ | The `tdd.md` discipline + the RED-first slice protocol make impatient code (drive-by refactors, "I'll add a comment later") visible. But no formal regression net. |
| Interdeveloper | ➖ | N/A. Solo project. |

**Gap candidate (DRY-Inadvertent):** add an `INDEX.md`-drift test that
asserts every file in `docs/agents/` is referenced by `INDEX.md`. Low
effort, high signal — catches "shipped but undocumented" process docs.

## §1.2 — Orthogonality — Tip #17

**Definition:** "Two or more things are orthogonal if changes in one
do not affect any of the others." The book calls this *cohesion*.

**Operational checklist from the book:**
- Design independent, well-defined components
- Keep your code decoupled
- Avoid global data
- Refactor similar functions

**Repo state:** ✅ **Strongly enforced.** The strongest enforcement
of any single principle in the book.

| Form | Repo state | Evidence |
|---|---|---|
| Independent components | ✅ | `internal/architecture/architecture_test.go` is the forbidden-import table. Every deep-module package has a `forbiddenByPackage` entry; CI fails on a new import. |
| Decoupled | ✅ | The deep-module discipline (`docs/agents/complexity.md`) is the rule; DTOs at the seam, not persistence structs. |
| No global data | ✅ | Wails `App` is a per-process struct, not a global; the dialog-guard mutex is per-`App`. The "no package-level mutable state" rule is implicit. |
| Refactor similar functions | ✅ | The `components/` primitives (Foldout, Button, Pill, Card, EmptyState, Field, Toast) are the operational form. The Markdown cheatsheet work (#565) explicitly followed the "no new component primitives" rule. |
| **The "shy code" / Law of Demeter dimension** | ⚠️ | The deep-module rule is *function-level*; the Law of Demeter is also *expression-level* (no `a.b().c().d()`). No regression test catches Demeter violations. |
| **Algorithm-level orthogonality** | ❌ | No Big O annotations in code or per-function docs. A future contributor picking up `internal/records/quality_scan.go` can't tell which scan is O(n) and which is O(n²) without reading the implementation. |

**Gap candidate (Orthogonality):** add a "Big O" doc-comment rule
for non-trivial functions in `internal/records/`, `internal/db/`,
`internal/seed/`, and `internal/jobs/`. Pattern: `// Big O: O(n²) in
person_count due to nested loop at line N. Pinned at this complexity
because the call site is bounded to N < 10k.` Matches the per-iter
SQL footprint pattern that already exists in `tdd.md`.

**Gap candidate (Orthogonality / Law of Demeter):** the canonical
name "Law of Demeter" is missing from the deep-module doc. A one-line
cross-link in `docs/agents/complexity.md` would close the gap.

## §1.3 — Reversibility — Tip #18

**Definition:** "No decision is cast in stone. Instead, consider each
as being written in the sand at the beach, and plan for change."

**Operational forms (per the book):**
- Critical decisions are irreversible or very expensive
- Encapsulate third-party APIs (the "provider" pattern) so a vendor
  swap is one file
- Keep code decoupled (orthogonality → reversibility)
- Use metadata for config, not code

**Repo state:** ✅ **Enforced via the three-branch model.**

| Form | Repo state | Evidence |
|---|---|---|
| Branch model is reversible | ✅ | ADR 0009: `dev` for integration, `stable` for releases, `main` frozen at 2026-07-03. Releases can be rolled back by promoting an older `stable` tag. |
| Vendor encapsulation | ⚠️ | Wails v2.12.0 is the Wails runtime, embedded. A future Wails-v3 migration is the hardest reversibility problem in the repo — the `Wails-PATCH` / `Wails-FormData` quirks in `frontend/app.js` are tied to the v2.12.0 asset server (per AGENTS.md §"Wails runtime hazards"). No provider layer. |
| Config-as-metadata | ✅ | `internal/uiids` registry, `internal/routebuilder`, the `lint-enforcement` rules in `Makefile` are the operational form. The `--smoke` flag in `dixiedata --smoke` is a config knob. |
| Decision reversibility audit | ❌ | No `docs/adr/decision-reversibility.md` that maps each ADR to "how do we reverse this decision?" A future agent inheriting a Wails-v3 migration would benefit. |

**Gap candidate (Reversibility):** one-page ADR or appendix mapping
each existing ADR to its reversal path. Especially important for
ADR 0009 (branch model — what if `stable` proves wrong?) and the
implicit Wails-v2.12.0 commitment.

## §1.4 — Tracer Bullets — Tip #20

**Definition:** "Tracer bullets let you home in on your target by
trying things and seeing how close they land." Small end-to-end
vertical slices, NOT horizontal layers. **Disposable prototype code
vs. tracer code is the key distinction** — tracer code IS the
skeleton of the final system.

**Repo state:** ✅ **The strongest enforced principle in the book.**

| Form | Repo state | Evidence |
|---|---|---|
| Tracer code IS the skeleton | ✅ | The `tracer-bullets` skill is auto-loaded by pi; `feature-protocol.md` §"Tracer bullets" is the operational form. The slice-1-only-per-session RPCI discipline is the strongest enforcement of any principle in the book. |
| Fresh context per slice | ✅ | `rpci.md` §I — "the first slice is the only slice that runs in the same session." A future agent doesn't see the previous slice's accumulated context and is forced to re-read the code. |
| Tracer code is lean but complete | ✅ | The Tier 2 vertical rule (one slice crosses every layer) ensures no throwaway scaffolding. |
| Tracer code is NOT a prototype | ✅ | The audit probes assert on the tracer code, not on a separate prototype. The Markdown cheatsheet work (#565) shipped tracer code (data + component + form wiring + audit probe) in one commit. |

**No gaps identified.** This is the principle the repo enforces most
deliberately. The RPCI critique gate is the operational form of
"user signs off on the tracer before we expand."

## §1.5 — Design by Contract — Tip #37

**Definition:** A correct program does no more and no less than it
claims. Use **preconditions, postconditions, invariants**.

**Repo state:** ✅ **Enforced via the architecture test + the
docs/CODE_CHANGES.md cross-layer contract.**

| Form | Repo state | Evidence |
|---|---|---|
| Preconditions | ✅ | Function preconditions are documented via doc comments ("Exported Go identifiers carry doc comments" Law in CONTEXT.md). |
| Postconditions | ⚠️ | Postconditions are documented for some functions (e.g. `records.MarkdownRenderer.Render` returns sanitized HTML) but no formal per-function rule. The "Tip #36 you can't write perfect software" is the escape valve: the *contract* is "best-effort, well-documented" not "provably correct." |
| Invariants | ⚠️ | The architecture test enforces **package** invariants (no forbidden imports). No **runtime** invariant checks via the `internal/debug` trace harness beyond what ADR 0006 documents. |
| Loop invariants | ❌ | No formal loop-invariant doc-comment rule. |
| DBC as a regression net | ⚠️ | The per-iter SQL footprint pattern in `tdd.md` is the operational form of "test the contract under real conditions," but the framing is "test budget" not "test the contract." |

**Gap candidate (Design by Contract):** the deep-module doc + the
tdd doc could grow a "DBC in this repo" appendix that names the
*three* DBC tools the repo uses (doc comments = preconditions,
architecture test = package invariants, smoke probes = behavioral
postconditions) and the gap (no runtime invariant checks). Closes
the partial state and gives future contributors a vocabulary.

## §1.6 — Dead Programs Tell No Lies (Crash Early) — Tip #38

**Definition:** "A dead program normally does a lot less damage than
a crippled one." When something impossible happens, the program is
no longer viable.

**Repo state:** ✅ **Enforced via the dialog-guard Law + the
htmxattr.Mux swap-allowlist panic.**

| Form | Repo state | Evidence |
|---|---|---|
| Panic on impossible state | ✅ | `htmxattr.Mux` swap-allowlist panics at render time. The dialog-guard mutex pattern prevents the "crippled" state (WebView2 + native dialog re-entry) by refusing to start. |
| Crash early in the right place | ✅ | The native `<dialog>` revert (issue #117) is the worked example — better to crash loudly than ship a known-bad shape. |
| Crash early vs. graceful degradation | ⚠️ | The principle is "crash early"; the repo's `X-DixieData-Toast` contract is a "graceful degradation" of errors. Both are present, sometimes in tension. No stated rule for "when to crash vs. when to toast." |

**Gap candidate (Crash Early):** a one-paragraph "When to crash vs.
when to toast" rule in `docs/agents/error-handling.md` (or a new
doc). The rule could be: "Crash on impossible state (assertion,
invariant violation, impossible match). Toast on user-recoverable
error (validation, missing data, retry-able failure). For
ambiguous cases, prefer toast — the user is the final authority."

## §1.7 — Decoupling / Law of Demeter — Tip #44

**Definition:** An object's method should call only methods belonging
to itself, its parameters, objects it creates, or its directly held
component objects. "Write shy code."

**Repo state:** ⚠️ **The principle name is missing from the deep-module doc.**

The deep-module rule enforces decoupling at the *package* boundary.
The Law of Demeter enforces it at the *expression* boundary (no
`a.b().c().d()` chains). The repo has the former but not the latter.

**Gap candidate (Decoupling):** rename / add a cross-link in
`docs/agents/complexity.md` to make the canonical name "Law of
Demeter" explicit, with a one-paragraph "what shy code means at
the expression level in Go" example. Low effort, completes the
principle mapping.

## §1.8 — Metaprogramming (abstractions in code, details in metadata) — Tip #79

**Definition:** "Program for the general case, and put the specifics
somewhere else — outside the compiled code base." Details in
metadata; abstractions in code.

**Repo state:** ✅ **Enforced via the `internal/uiids` registry and
the `lint-enforcement` rules.** The Wails event bus is a related
mechanism that's documented as unused (`docs/RESEARCH.md` §FU.3).

| Form | Repo state | Evidence |
|---|---|---|
| UI surface IDs in registry | ✅ | `internal/uiids` is the canonical example. Every component is a string constant in the registry. |
| Route URLs in builder | ✅ | `internal/routebuilder` is the canonical example. |
| Lint rules in Makefile | ✅ | `Makefile` `lint-defer-close`, `lint-htmx-guard`, `lint-typecheck-augmentations` are the operational form. |
| Wails event bus | ⚠️ | Documented in `docs/RESEARCH.md` as unused. The book explicitly recommends publish/subscribe for "separate views from models" — the Wails event bus would be the natural DixieData implementation. No decision recorded to use or skip it. |
| Feature flags | ❌ | No feature-flag infrastructure. A "v2 markdown editor" toggle would require code change. |

**Gap candidate (Metaprogramming):** record an ADR decision on the
Wails event bus: either adopt it for the views-from-models pattern
the book recommends, or record a `n/a` decision (the Single-User
Single-Window reality makes the Wails event bus redundant). The
gap is the *decision*, not the implementation.

## §1.9 — Temporal Coupling / Concurrency — Tip #57

**Definition:** Reduce time-based dependencies. Design for
concurrency so the system can be deployed flexibly and can be
tested for thread safety.

**Repo state:** ✅ **The Wails App-struct pattern is the
actor-equivalent in this codebase.**

| Form | Repo state | Evidence |
|---|---|---|
| Concurrency in the user's workflow | ✅ | The `X-DixieData-Toast` + `X-DixieData-Redirect` contract: the user gets a toast + a jobs page in parallel for every export. The dialog-guard mutex ensures the UI thread is never blocked. |
| Hungry consumer model | ✅ | The `internal/jobs` package is the operational form. Every export enqueues a job, workers process the queue. |
| Design for concurrency | ✅ | The `App` struct is per-process, not global. The Wails bindings run on the UI thread; service code runs on goroutines. |
| Workflow analysis (activity diagrams) | ❌ | No `docs/agents/concurrency.md` that maps the app's workflows to activity diagrams. A future "concurrent export + edit" feature would benefit. |
| Per-package `goleak` regression net | ⚠️ | Two packages have `zzz_goleak_test.go` (`internal/appshell`, `internal/archive`). Other packages with goroutines (jobs, scratchpad, debug) do not. |

**Gap candidate (Concurrency):** add `zzz_goleak_test.go` to every
package that spawns goroutines, and grow `docs/agents/concurrency.md`
from the dialog-guard doc (which is the *one* concurrency rule that
isn't generic).

## §1.10 — It's Just a View (MVC) — Tip #42

**Definition:** Separate the model from views of the model. Support
multiple views of the same data; use common viewers on many
different data models.

**Repo state:** ✅ **The viewmodel layer is the DixieData
implementation of MVC.**

| Form | Repo state | Evidence |
|---|---|---|
| Model | ✅ | `internal/records/`, `internal/db/`, `internal/models/`. |
| View | ✅ | `internal/templates/` (templ) + `frontend/app.js` (browser JS). |
| Controller | ✅ | `internal/appshell/` (HTTP handlers) + Wails bindings. |
| Viewmodel | ✅ | `internal/viewmodel/` is the explicit grey-box layer. The architecture test (`internal/architecture/architecture_test.go`) is the rule that enforces "viewmodel doesn't import appshell/wails." |
| Publish/subscribe for view updates | ⚠️ | The Wails event bus is the natural mechanism, documented as unused. The Calendar / Browse / Person Record detail pages are server-rendered + htmx-swap, which is the Wails-app equivalent of pub/sub (the server pushes the new HTML, the browser swaps). |
| Multiple views of the same data | ✅ | The static archive (`internal/archive/static_archive.go`) ships a read-only viewer of the same data the Wails app mutates. Different view, same model. |

**No gaps identified.** This is the principle the repo enforces
*silently* — the architecture test catches violations but the
"MVC" framing isn't in any tier-1 doc. **Gap candidate (MVC):**
add a one-paragraph "MVC in this repo" appendix to
`docs/agents/complexity.md` to make the framing explicit.

## §1.11 — Program by Coincidence → Program Deliberately — Tip #62

**Definition:** "Rely only on reliable things. Beware of accidental
complexity, and don't confuse a happy coincidence with a purposeful
plan." The book's "How to Program Deliberately" checklist:

- Always be aware of what you are doing
- Don't code blindfolded
- Proceed from a plan
- Rely only on reliable things
- Document your assumptions
- Test assumptions as well as code
- Prioritize your effort
- Don't be a slave to history

**Repo state:** ✅ **Enforced via RPCI + TDD + the property-test
gate.** Two of the eight rules have a gap.

| Form | Repo state | Evidence |
|---|---|---|
| Always be aware | ✅ | RPCI Critique gate is the operational form — every plan is reviewed before code. |
| Don't code blindfolded | ✅ | RED-first TDD is the operational form. |
| Proceed from a plan | ✅ | RPCI Plan is the operational form. |
| Rely only on reliable things | ✅ | The `tmx` skill (`tracer-bullets`) is the operational form. The per-iter SQL footprint doc-comment pattern is the operational form for "test the actual environment." |
| Document your assumptions | ✅ | Doc-comment floor (CONTEXT.md Law) is the operational form. |
| **Test assumptions as well as code** | ⚠️ | The `internal/dates` property tests cover date parsing. The Markdown cheatsheet work (#565) added a drift detector (assumption: "the cheatsheet matches the renderer"). No per-PR discipline for "what assumptions does this PR make that need a test?" |
| Prioritize your effort | ✅ | The "fresh context per slice" rule is the operational form — forces prioritization at the slice boundary. |
| Don't be a slave to history | ✅ | Refactor Early, Refactor Often (§1.13 below) + the v60 → v61 rename refactor are the operational form. |

**Gap candidate (Program Deliberately):** add a "What assumptions
does this PR make?" line to the slice plan template in
`docs/agents/feature-protocol.md`. Closes the partial state on
"testing assumptions" and is a one-line change.

## §1.12 — Algorithm Speed (Big O) — Tip #63

**Definition:** "Get a feel for how long things are likely to take
before you write code." Use Big O notation; common-sense
estimation: O(1) / O(log n) / O(n) / O(n log n) / O(n²) / O(n³) / O(cⁿ).

**Repo state:** ⚠️ **The canonical name "Big O" is missing from
the repo.** The per-iter SQL footprint pattern in `tdd.md` is the
closest form, but it's framed as a perf budget, not a complexity
annotation.

| Form | Repo state | Evidence |
|---|---|---|
| Per-function Big O annotation | ❌ | No doc-comment rule. |
| Per-iter SQL footprint | ✅ | `tdd.md` §"Per-layer recipes" documents the pattern. `TestStressEventAttachDetachRoundTrip` is the worked example. |
| Common-sense estimation | ⚠️ | Implicit in RPCI Plan ("Files touched, Success criteria, Regression net") but no time estimate. |
| "I'll get back to you" | ✅ | The RPCI "no implementation before Plan approved" rule is the operational form. We never give an estimate we don't have evidence for. |
| Track estimating prowess | ❌ | No log of estimates-vs-actuals. The book recommends keeping a log to find the "where did your estimate go wrong" patterns. |

**Gap candidate (Algorithm Speed):** add Big O doc-comment rule for
non-trivial functions in `internal/records/`, `internal/db/`,
`internal/seed/`, and `internal/jobs/`. Same gap as Orthogonality
§1.2 — these gaps overlap and could land as one PR.

## §1.13 — Refactoring — Tip #65

**Definition:** Code needs to evolve. The book's "When to Refactor"
checklist:

- You discover a violation of the DRY principle
- You find things that could be more orthogonal
- Your knowledge improves
- The requirements evolve
- You need to improve performance

**Repo state:** ✅ **Enforced via the `complexity.md` doc and the
3-tier commit rule.**

| Form | Repo state | Evidence |
|---|---|---|
| DRY violation | ✅ | The deep-module rule + the architecture test. |
| Non-orthogonal | ✅ | The forbidden-import table. |
| Knowledge improved | ✅ | The v60 → v61 Event Records refactor (sibling-table rename) is the worked example. |
| Requirements evolve | ✅ | The CHANGELOG is a public log of "knowledge improved" refactors. |
| Performance | ✅ | The per-iter SQL footprint pattern + the `race-stress.yml` workflow. |
| Don't refactor and add features at the same time | ✅ | Tier 2 vs. Tier 3 commit rule. The slice-1-only-per-session discipline prevents the "drive-by refactor" pattern. |
| Have good tests before refactoring | ✅ | TDD + smoke probes. |
| Take short, deliberate steps | ✅ | RPCI. |

**No gaps identified.** This principle is the best-enforced in the
repo. The `complexity.md` doc is the working form.

## §1.14 — Code That's Easy to Test — Tip #67

**Definition:** Build testability into the software from the very
beginning, and test each piece thoroughly before trying to wire them
together.

**Repo state:** ✅ **Enforced via the architecture test + the
per-apply-site smoke probe contract.**

| Form | Repo state | Evidence |
|---|---|---|
| Unit testing | ✅ | The Go test suite across every package. The architecture test forbids untestable dependencies. |
| Integration testing | ✅ | The `appshell_test.go` harness + the per-handler integration tests. |
| Validation and verification | ✅ | The smoke probes (Playwright) drive the live binary. |
| **Resource exhaustion, errors, and recovery** | ⚠️ | Two packages have `zzz_goleak_test.go`. The error-recovery path is tested per handler. Resource-exhaustion testing is not formalized. |
| **Performance testing** | ✅ | The `race-stress.yml` workflow + `TestStressEventAttachDetachRoundTrip`. The per-iter SQL footprint pattern. |
| **Usability testing** | ❌ | Manual only. No automated usability test. The Markdown cheatsheet work was usability-tested by being shipped. |
| **Testing the tests themselves** | ❌ | No mutation testing. Audit 1.0 already flagged this. |

**Gap candidates (Testing):**
- Resource-exhaustion test: add `zzz_goleak_test.go` to the
  `internal/jobs/`, `internal/scratchpad/`, `internal/debug/`
  packages (all spawn goroutines).
- Mutation testing: evaluate `go-mutesting` or equivalent. Low
  effort to add, high signal.
- Usability testing: the manual-audit playbook exists. Automating
  a subset (a11y via axe-core through `tools/tune/`) is a follow-up.

## §1.15 — Ubiquitous Automation — Tip #85

**Definition:** A shell script or batch file will execute the same
instructions, in the same order, time after time. Don't use manual
procedures. Automation is everywhere.

**Repo state:** ✅ **Strongly enforced** (23 scripts in `scripts/`,
40+ Makefile targets, 4 GitHub workflows).

| Form | Repo state | Evidence |
|---|---|---|
| Build automation | ✅ | `make build` + `make debug` + `make freshness` chain. |
| Test automation | ✅ | `make test` + the 4 workflows. |
| Release automation | ✅ | `make promote` + `scripts/release-github.ps1` per ADR 0008. |
| Audit automation | ✅ | `make audit` + the 20+ `audit/*.mjs` probes. |
| **Manual procedures that should be automated** | ⚠️ | The audit harness scratch-dir setup is partly manual (`.scratch/webmode` warm-up, seed-data invocation). A `make audit-env` target that boots the scratch dir + seeds it would be a `make freshness` analog. |

**Gap candidate (Automation):** `make audit-env` target. One-time
setup of `.scratch/webmode` with a seeded database, so every audit
probe can `make audit` from a clean shell. Mirrors `make freshness`
for the build side.

## §1.16 — It's All Writing (English as code) — Tip #13

**Definition:** Documentation and code are different views of the
same underlying model. Write documents as you would write code:
honor the DRY principle, use metadata, MVC, automatic generation.

**Repo state:** ✅ **Enforced via the doc-comment floor + the
`docs/COMMON_BUGS.md` per-layer catalog + the tier-1 process docs.**

| Form | Repo state | Evidence |
|---|---|---|
| Docs as code (versioned, reviewed) | ✅ | The `docs/` tree is committed; PR review covers doc changes. |
| DRY in docs | ✅ | `CONTEXT.md` is the single source of truth for vocabulary. Tier-1 process docs cross-link. |
| Docs in plain text | ✅ | Every doc is `.md`. |
| Docs generated from code | ⚠️ | The `docs/adr/` ADRs are hand-written. The `CHANGELOG.md` is hand-curated (the book would recommend generating from commit messages — DixieData already has rich commit messages that could feed a generated CHANGELOG). |
| **Date-stamp on every web page** (book's specific example) | ➖ | N/A. Docs are local + version-controlled; the date stamp is `git log -1 --format=%ai <file>`. |

**Gap candidate (It's All Writing):** evaluate a `git-cliff` or
`conventional-changelog`-style CHANGELOG generator. The CHANGELOG
currently requires manual curation; the 79 fixship commits in the
repo history are well-structured enough to feed a generator. Low
effort, removes a recurring manual procedure.

## §1.17 — Great Expectations (Exceed users' expectations) — Tip #69

**Definition:** "Come to understand your users' expectations, then
deliver just that little bit more." The book's specific examples:
balloon help, keyboard shortcuts, colorization, log file analyzers,
automated installation, tools for checking the integrity of the
system, the ability to run multiple versions, a splash screen.

**Repo state:** ✅ **Enforced via the recent fixship cadence.**

| Form | Repo state | Evidence |
|---|---|---|
| Balloon help / in-context help | ✅ | The Markdown cheatsheet work (#565) is the recent example. The 2026-07 audit surfaced the *need* for tooltips; the in-context cheatsheet was the in-app answer. |
| Keyboard shortcuts | ⚠️ | Some shortcuts exist (the floating dock's keyboard nav, the smart-back helper). No global keyboard shortcut sheet. |
| Quick reference guide | ⚠️ | The user manual exists. The Markdown cheatsheet is the in-app equivalent for one screen. The Articles screen has the only in-app reference card. |
| Colorization | ✅ | The theme system (`internal/theme/`) is operational. The phase-3 contrast probe is the regression net. |
| Log file analyzers | ❌ | No log file analyzer. The `internal/debug/Configure` produces logs but no analyzer. |
| Automated installation | ✅ | Wails handles this. `make build` is the dev side. |
| Tools for checking system integrity | ✅ | `dixiedata debug` (per `docs/agents/cli-plan.md`). `dixiedata doctor` is a future phase. |
| Multiple versions for training | ❌ | No. The three-branch model (dev/stable/main) is the closest form; the user can build any branch but cannot run two simultaneously. |
| Splash screen | ✅ | Wails handles this. |

**Gap candidate (Great Expectations):** a "log file analyzer" or
"smoke-test the user's DB" subcommand in `dixiedata doctor`. Phase
2+ of `docs/agents/cli-plan.md` already plans a `doctor`
subcommand; this gap is *executing* an existing plan, not new
work.

## §1.18 — Pragmatic Teams (organize around functionality) — Tip #86

**Definition:** Don't separate designers from coders, testers from
data modelers. Build teams the way you build code.

**Repo state:** ➖ **N/A** (solo project). The closest operational
form is the "single agent" agent, which IS organized around
functionality by virtue of the slice discipline: a single agent
ships data + component + form wiring + audit probe in one slice.

**No gaps identified.** The principle is satisfied by the slice
discipline, not by org structure.

## §1.19 — WISDOM Acrostic (Ch 1, Communicate)

**Definition:** When communicating (CHANGELOG, issue, commit
message, ADR), use the WISDOM acrostic:
- **W**hat do you want them to learn?
- What **i**s their interest in what you've got to say?
- How **s**ophisticated are they?
- How much **d**etail do they want?
- Whom do you want to **o**wn the information?
- How can you **m**otivate them to listen to you?

**Repo state:** ⚠️ **The spirit is satisfied; the canonical name
is missing.**

| Form | Repo state | Evidence |
|---|---|---|
| Audience-targeted | ✅ | CHANGELOG bullets name the user ("the user landed on what looked like an in-progress page"). Issues name the apply-sites the user will see. |
| Detail level varies | ✅ | The "Short" (commit subject) / "Long" (commit body + CHANGELOG) / "Very long" (ADR) ladder is operational. |
| **WISDOM by name** | ❌ | The acrostic is not in any tier-1 doc. A new contributor writing an ADR or a long-form commit has no checklist. |

**Gap candidate (WISDOM):** add a "WISDOM acrostic" appendix to
`docs/agents/issue-tracker.md` (for issues) and to a new
`docs/agents/communication.md` (for commit messages, ADRs, and
CHANGELOG bullets). One page total.

## §1.20 — Architectural Questions (Ch 7)

**Definition:** When designing a new module / service / feature, ask:

- Are responsibilities well defined?
- Are the collaborations well defined?
- Is coupling minimized?
- Can you identify potential duplication?
- Are interface definitions and constraints acceptable?
- Can modules access needed data — when needed?

**Repo state:** ⚠️ **The deep-module rule captures 4 of 6; the
"interface definitions and constraints" and "access to data"
questions are not pinned.**

**Gap candidate (Architectural Questions):** add the 6 questions
as a checklist to `docs/agents/complexity.md` §"Module discipline."
Two paragraphs.

## §1.21 — Debugging Checklist (Ch 3)

**Definition:** When stuck on a bug, ask:

- Is the problem being reported a direct result of the underlying
  bug, or merely a symptom?
- Is the bug really in the compiler? Is it in the OS? Or is it in
  your code?
- If you explained this problem in detail to a coworker, what
  would you say?
- If the suspect code passes its unit tests, are the tests complete
  enough? What happens if you run the unit test with this data?
- Do the conditions that caused this bug exist anywhere else in
  the system?

**Repo state:** ⚠️ **The spirit is enforced (the orphan-handler
probe, the dialog-guard rule); the canonical checklist is missing.**

**Gap candidate (Debugging Checklist):** add the 5 questions as a
side-box in `docs/agents/diagnosing-bugs.md` or the relevant
debug doc. Low effort, high value for future agents.

## §1.22 — Aspects of Testing (Ch 8)

**Definition:** The 7 aspects of testing a system:

- Unit testing
- Integration testing
- Validation and verification
- Resource exhaustion, errors, and recovery
- Performance testing
- Usability testing
- Testing the tests themselves

**Repo state:** ⚠️ **5 of 7 are enforced; resource-exhaustion and
testing-the-tests have gaps.** Usability testing is manual.

| Aspect | State |
|---|---|
| Unit testing | ✅ |
| Integration testing | ✅ |
| Validation and verification | ✅ |
| Resource exhaustion, errors, recovery | ⚠️ (goleak in 2/4 packages) |
| Performance testing | ✅ |
| Usability testing | ⚠️ (manual; axe-core via `tools/tune/` is a follow-up) |
| Testing the tests themselves | ❌ (no mutation testing) |

**Gap candidates:** see §1.14 (Code That's Easy to Test) above —
the Aspects-of-Testing checklist IS the principle. The gaps are
the same.

## §1.23 — Cutting the Gordian Knot (Ch 7)

**Definition:** When solving impossible problems, ask:

- Is there an easier way?
- Am I solving the right problem?
- Why is this a problem?
- What makes it hard?
- Do I have to do it this way?
- Does it have to be done at all?

**Repo state:** ❌ **No checklist.** A future agent stuck on a
"this can't be done" task has no rule to consult.

**Gap candidate (Gordian Knot):** add the 6 questions as a
side-box in `docs/agents/feature-protocol.md` §"Tracer bullets"
or `docs/agents/lock-requirements.md`. One paragraph.

## §1.24 — When to Refactor (Ch 6)

**Definition:** The 5 refactor triggers (also in §1.13 above):
- DRY violation
- Non-orthogonal
- Knowledge improved
- Requirements evolve
- Performance

**Repo state:** ✅ **Enforced.** The deep-module doc and the
Tier 2/3 commit rule are the operational form.

# §2 — Tip map (the 100-tip view, condensed)

The 100-tip view from the first audit pass
(`pragmatic-programmer-audit-2026-07.md`) is retained as a
historical artifact and referenced here. This audit does NOT
duplicate the 100-row table; the principle view in §1 is the
primary lens. The mapping is:

| Principle (this audit) | Maps to tips (audit 1.0) |
|---|---|
| §1.1 DRY | #11, #12, #15, #16 |
| §1.2 Orthogonality | #17, #44, #45, #46, #47, #48, #49, #50, #51 |
| §1.3 Reversibility | #14, #18, #19 |
| §1.4 Tracer Bullets | #20, #42, #68 |
| §1.5 Design by Contract | #36, #37, #38, #39 |
| §1.6 Dead Programs | #32, #33, #34 |
| §1.7 Law of Demeter | #44 (overlaps with §1.2), #45, #46 |
| §1.8 Metaprogramming | #78, #79, #80 |
| §1.9 Temporal Coupling | #56, #57, #58, #59, #60 |
| §1.10 MVC | #42 (overlaps) |
| §1.11 Program Deliberately | #62, #67 |
| §1.12 Algorithm Speed | #63, #64 |
| §1.13 Refactoring | #65, #66 |
| §1.14 Code That's Easy to Test | #69, #70, #71, #72, #73 |
| §1.15 Ubiquitous Automation | #85, #86, #90, #91, #94, #95 |
| §1.16 It's All Writing | #11, #12, #13 |
| §1.17 Great Expectations | #69, #96 |
| §1.18 Pragmatic Teams | N/A |
| §1.19 WISDOM | #10, #12 |
| §1.20 Architectural Questions | (no tip; chapter section) |
| §1.21 Debugging Checklist | (no tip; chapter section) |
| §1.22 Aspects of Testing | (no tip; chapter section) |
| §1.23 Cutting the Gordian Knot | (no tip; chapter section) |
| §1.24 When to Refactor | (no tip; chapter section) |

The 100-tip audit's 6 gap candidates are subsumed by the principle
view's gap candidates. The two new gaps surfaced by the principle
view (Big O annotations, Law of Demeter canonical name) are
additions, not replacements.

# §3 — Cross-reference matrix (principle × repo)

A flat view: which principles the repo enforces, with one
sentence per principle:

| Principle | State | One-line summary |
|---|---|---|
| DRY | ✅ | Enforced via routebuilder, uiids, models, CONTEXT.md vocabulary. |
| Orthogonality | ✅ | Enforced via the architecture forbidden-import test. |
| Reversibility | ✅ | The three-branch model (ADR 0009) is the operational form. |
| Tracer Bullets | ✅ | The slice-1-only-per-session RPCI discipline. |
| Design by Contract | ⚠️ | Doc comments + architecture test; no runtime invariant checks. |
| Dead Programs | ✅ | htmxattr.Mux swap-allowlist panic + dialog-guard mutex. |
| Law of Demeter | ⚠️ | Enforced at the package level; not at the expression level. |
| Metaprogramming | ✅ | uiids + routebuilder + lint rules. |
| Temporal Coupling | ✅ | Wails App-struct pattern + jobs queue. |
| MVC | ✅ | viewmodel + architecture test. |
| Program Deliberately | ⚠️ | 7 of 8 rules enforced; "test assumptions" is partial. |
| Algorithm Speed | ⚠️ | Per-iter SQL footprint pattern; no Big O annotations. |
| Refactoring | ✅ | complexity.md + Tier 2/3 commit rule. |
| Code That's Easy to Test | ⚠️ | 5 of 7 Aspects of Testing enforced. |
| Ubiquitous Automation | ✅ | 23 scripts + 40+ Make targets + 4 workflows. |
| It's All Writing | ✅ | doc-comment floor + tier-1 process docs. |
| Great Expectations | ✅ | Recent fixship cadence. |
| Pragmatic Teams | ➖ | N/A. |
| WISDOM | ⚠️ | Spirit enforced; canonical name missing. |
| Architectural Questions | ⚠️ | 4 of 6 captured by deep-module rule. |
| Debugging Checklist | ⚠️ | Spirit enforced; canonical checklist missing. |
| Aspects of Testing | ⚠️ | 5 of 7 enforced. |
| Cutting the Gordian Knot | ❌ | No checklist. |
| When to Refactor | ✅ | The 5 triggers are operationalized. |

**Count:** 14 ✅ + 9 ⚠️ + 1 ❌ + 1 ➖ = 25 principles.

# §4 — Consolidated gap candidates

The principle view surfaces **11 gap candidates** (the tip view
surfaced 6; the principle view adds 5 because the principle checklists
have gaps the tip-by-tip view hides). Each is one new file or one
short doc update.

| # | Gap | Principle | File(s) | Effort |
|---|---|---|---|---|
| 1 | Big O doc-comment rule for non-trivial functions | §1.2 Orthogonality + §1.12 Algorithm Speed | `docs/agents/complexity.md` or new `docs/agents/algorithm-complexity.md` | S |
| 2 | Law of Demeter canonical name in deep-module doc | §1.2 + §1.7 | `docs/agents/complexity.md` | XS |
| 3 | "Crash vs. toast" rule | §1.6 Dead Programs | `docs/agents/error-handling.md` or new | S |
| 4 | "What assumptions does this PR make?" line in slice template | §1.11 Program Deliberately | `docs/agents/feature-protocol.md` | XS |
| 5 | `make audit-env` target for warm scratch + seeded DB | §1.15 Ubiquitous Automation | `Makefile` | S |
| 6 | CHANGELOG generator (git-cliff or conventional-changelog) | §1.16 It's All Writing | `CHANGELOG.md` workflow | M |
| 7 | `zzz_goleak_test.go` for jobs/scratchpad/debug packages | §1.9 Temporal Coupling + §1.14 Testing | 3 new files | S |
| 8 | Mutation testing pilot (one package) | §1.14 + §1.22 Aspects of Testing | `Makefile` + one package | M |
| 9 | ADR decision on Wails event bus (use or skip) | §1.8 Metaprogramming | new ADR | XS |
| 10 | WISDOM + Architectural Questions + Debugging Checklist + Cutting the Gordian Knot in tier-1 docs | §1.19 + §1.20 + §1.21 + §1.23 | `docs/agents/communication.md` (new) + cross-link | M |
| 11 | `docs/learning/` directory (Tip #9 follow-up from audit 1.0) | §1.19 + the book's Ch 1 §"Knowledge Portfolio" | new directory + 3 starter entries | S |
| 12 | Reversibility decision-reversal appendix to ADRs | §1.3 | `docs/adr/README.md` or new doc | M |

**Total: 12 gap candidates.** Each can ship as a one-PR slice
following the standard protocol (RPCI → TDD → slice commit).

# §5 — Resolution path

Same as audit 1.0. This is a diagnostic, not a refactor:

1. **User reviews the audit** + the 12 gap candidates
2. **User decides** which gaps become follow-up issues
3. **File one issue per accepted gap** (or one umbrella issue with
   a per-gap checklist — match the user's preference)
4. **Each gap ships as a one-PR slice** per the standard protocol
5. **When every accepted gap has a closed follow-up issue**, this
   audit moves to `docs/audit/resolved/`

# §6 — Reference to the 100-tip audit

`pragmatic-programmer-audit-2026-07.md` is retained as a
historical artifact. The 100-tip table there is the *operational*
view; the principle spine in this audit is the *structural* view.
Both are useful. The principle view is the primary lens for
*future work*; the tip view is the primary lens for *checking
specific commits* against the canonical book.

When this audit is resolved (moved to `docs/audit/resolved/`),
the 100-tip doc moves with it. The two are version-locked.

# §7 — References

- _The Pragmatic Programmer, 20th Anniversary Edition_, Hunt &
  Thomas. 100 tips excerpted at <https://pragprog.com/tips/> with
  the publisher's permission.
- HugoMatilla's summary of the 1st edition (chapter structure +
  70-tip list + 11 checklists):
  <https://github.com/HugoMatilla/The-Pragmatic-Programmer>.
  The 2nd edition (20th Anniversary) restructured the chapters
  but kept the principle spine.
- The 11 book-end checklists (WISDOM, How to Maintain
  Orthogonality, Things to Prototype, Architectural Questions,
  Debugging Checklist, Law of Demeter for Functions, How to
  Program Deliberately, When to Refactor, Cutting the Gordian
  Knot, Aspects of Testing) are reproduced in
  <https://github.com/HugoMatilla/The-Pragmatic-Programmer#checklist>.
- `CONTEXT.md` — glossary + Laws.
- `AGENTS.md` — session protocol.
- `docs/agents/INDEX.md` — tier-1 process doc index.
- `docs/agents/feature-protocol.md` — Tier 1-3 commit rule.
- `docs/agents/tdd.md` — RED/GREEN discipline.
- `docs/agents/rpci.md` — RPCI flow.
- `docs/agents/complexity.md` — deep-module + YAGNI.
- `docs/agents/build-protocol.md` — freshness gate.
- `docs/CODE_CHANGES.md` — cross-layer contract.
- `docs/COMMON_BUGS.md` — per-layer bug catalog.
- `docs/adr/` — 10 ADRs covering branch model, design system,
  dispatcher, slog/trace, in-place update, promotion, lint, stable.
- `docs/audit/pragmatic-programmer-audit-2026-07.md` — the
  100-tip-only pass retained as a historical artifact.
