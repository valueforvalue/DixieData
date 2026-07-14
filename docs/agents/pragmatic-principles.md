# Pragmatic Principles — A Field Guide

> **Audience:** every agent that lands on a slice. Load this doc
> when designing a new feature, refactoring, reviewing a PR, or
> debugging a recurring class of bug. Cross-referenced from
> `AGENTS.md` §LLM session protocol and from
> `feature-protocol.md` §Slice plan template.

## What this doc is

A **guide**, not a law set. The principles below come from
_The Pragmatic Programmer, 20th Anniversary Edition_ (Hunt &
Thomas). They are the *why* behind the repo's existing rules:
the architecture forbidden-import test enforces **orthogonality**;
the routebuilder + uiids registries enforce **DRY**; the slice-1
discipline enforces **tracer bullets**; the per-iter SQL footprint
doc-comment pattern enforces **program deliberately**.

The laws in [`CONTEXT.md`](../../CONTEXT.md) §"Laws (non-negotiable)"
are *earned by a real bug* and are not negotiable. The principles
here are *guides* — they tell you what to think about, and they
name the *known cases* where the repo legitimately violates a
principle temporarily. The agent's job is to know the principle,
know the violation pattern, and **warn + cite** before violating
it (see §"The warn + cite protocol" below).

This is a **field guide**: it tells you what to do, when to do it,
and what to do when the rule doesn't fit. It is not a textbook.
For the book-length treatment, see Hunt & Thomas.

## The warn + cite protocol

When a slice is about to **violate a principle documented here**, the
agent must:

1. **Name the principle** in the slice Plan (the "Principle
   warnings" block — see `feature-protocol.md` §Slice plan template).
2. **Cite the operational form** being violated (e.g. "the
   routebuilder per-URL invariant," "the components/ primitive
   reuse rule").
3. **State the rationale** for the temporary violation (the
   *why* — what makes this case an exception).
4. **State the cleanup plan** (when + how the violation gets
   resolved — usually a follow-up issue filed in the same slice
   commit).
5. **Land the rationale + cleanup plan verbatim in the commit
   message and the CHANGELOG bullet** so the next agent sees the
   documented violation when reading the work.

The user signs off on the violation as part of the Plan approval
gate (per `rpci.md` §C — Critique). Violations without a documented
rationale + cleanup plan are not mergeable. This is the
**enforcement mechanism** for principles-as-guides: not a CI check
(not everything is a law), but a review-gate check (the human
sees the warning before the violation lands).

A documented violation is **not a bug** — it's a *known and
intentional* exception. An **undocumented** violation is a bug.
That distinction is the whole point of the protocol.

## When the principles don't fit

Three patterns where the principles *legitimately* need a local
override:

- **Bridges / adapter code** — `internal/htmxattr.Mux`,
  `internal/routebuilder`, `internal/uiids`, and the Wails dispatcher
  in `frontend/app.js` exist *because* the world has shape (Wails
  v2.12.0 quirks, htmx attribute string) and the adapters absorb
  it. **DRY in the application code is enforced by these adapters;
  DRY is *not* enforced inside the adapters** (the adapter is the
  one place where the duplicated shape gets collapsed).
- **Tier 1 vs Tier 2 surface** — the per-component byte-stability
  rule in `components/conventions.md` says a primitive's rendered
  HTML must match the legacy inline class-string. This is *imposed
  duplication* (per DRY §1.1) on purpose: the snapshot tests pin
  the duplication so a future refactor can verify the primitive
  swap is safe.
- **Temporary divergence** — a slice that needs to ship faster
  than the principled refactor would allow. The cleanup plan
  documents the divergence; the follow-up issue does the refactor.
  This is **not** an excuse for permanent divergence — every
  temporary violation has a deadline (the cleanup plan), and the
  follow-up issue tracks the deadline.

---

## §1 — The 16 principles

Each entry: 1-sentence definition (from the book), the *why* (what
it saves you from), the repo's operational form (file:line where
the principle is enforced), and a "When you might violate" section
listing the *known* cases where the repo legitimately breaks the
principle. New violation cases found in the wild are added to the
"when you might violate" list — they are warnings, not errors.

### §1.1 — DRY (Don't Repeat Yourself) — Tip #15

> "Every piece of knowledge must have a single, unambiguous,
> authoritative representation within a system."

**The why:** every duplicated representation is a place where a
future change has to be made in N places. Miss one and the system
silently diverges. The cost of a single-source change is O(1); the
cost of an N-place change is O(N) and grows with the system.

**Repo operational form:**
- `internal/routebuilder` — single source for every URL. The
  `hx_guard_test.go` test fails any templ that uses a string literal.
- `internal/uiids` — single source for every DOM surface ID. The
  `htmxattr.Mux` validator refuses unknown `hx-target` values.
- `internal/models` + `internal/db/migrations.go` — the schema
  is the single source of truth for column types.
- `CONTEXT.md` — the single source of truth for vocabulary. The
  "Flagged ambiguities" section is the regression net for
  vocabulary drift.
- `components/` design-system primitives — Button, Card, Pill,
  EmptyState, Field, Toast, Foldout, MarkdownCheatsheet are the
  single source for visual patterns. The per-component snapshot
  tests pin the byte-stable output.

**When you might violate (known cases):**
- **Tier 1 vs Tier 2 surface** — a new component primitive
  (e.g. `Heading` — see open audit gap "no Heading component")
  has to coexist with the literal class strings it will replace
  for one slice. The follow-up issue documents the swap.
- **Temporary `// duplicate` comments** — `internal/templates/jobs_templ.go:920`
  has a dev comment that explicitly narrates a duplication
  ("duplicated the same 2-case switch as jobs.DisplayLabel"). The
  follow-up is to extract a shared `kindLabel` helper.
- **Cheatsheet clipboard wiring** — the `Markdown syntax` cheatsheet
  (#565) has per-row `data-md-cheatsheet-copy-key` data attrs
  that should reuse the existing `[data-copy-path]` clipboard
  helper in `frontend/app.js:3682`, not reimplement it. The
  follow-up is the wire-up.
- **Glossary tier-2 rename** — when `CONTEXT.md` revises a
  vocabulary term, the old term may live in the codebase for a
  release cycle. The migration note documents the temporary
  divergence (see the v62 `soldier_id` → `person_record_id` rename).

### §1.2 — Orthogonality — Tip #17

> "Two or more things are orthogonal if changes in one do not
> affect any of the others." Also called *cohesion*.

**The why:** a change to module A should not silently require a
change to module B. Non-orthogonal systems make every change
expensive, and the cost grows with the system.

**Repo operational form:**
- `internal/architecture/architecture_test.go` — the forbidden-import
  table. Every deep-module package has a `forbiddenByPackage` entry;
  CI fails on a new import.
- `docs/agents/complexity.md` — the deep-module rule. DTOs at the
  seam, not persistence structs.
- The Wails `App` struct is per-process, not global. The dialog-guard
  mutex is per-`App`, not module-level.

**When you might violate:**
- **Bridge code** — `htmxattr.Mux` couples the templ + htmx worlds
  on purpose. The coupling is local (one file); the *uncoupling*
  is the rest of the codebase.
- **Cross-cutting concerns** — logging (`internal/debug`),
  tracing (`internal/debug/trace`), theming (`internal/theme`)
  are *designed* to be cross-cutting. They violate strict
  orthogonality; they earn it by being the smallest possible
  cross-cutting surface.
- **DI across a Wails binding** — when a service is constructed
  in `main.go` and passed to a Wails binding, the binding *does*
  know about the service. The Wails-app shape forces this. The
  mitigation is "binding is a thin wrapper, service holds the
  logic" (the seam rule).

### §1.3 — Reversibility — Tip #18

> "No decision is cast in stone. Instead, consider each as being
> written in the sand at the beach, and plan for change."

**The why:** every decision has a chance of being wrong. Decisions
that are easy to reverse are cheap; decisions that are hard to
reverse are expensive. The expensive ones are the ones that need
the most scrutiny.

**Repo operational form:**
- `docs/adr/0009-stable-branch-promotion.md` — the three-branch
  model (`dev` / `stable` / `main`). Releases can be rolled back
  by promoting an older `stable` tag.
- `internal/routebuilder` + `internal/uiids` — every URL and DOM
  ID is a string in a registry, not a literal in templ. A
  rename is a one-file change.
- `docs/migrations/reversibility.md` — every migration is classified
  as Reversible / Partially Reversible / Non-Reversible, with the
  downgrade SQL pinned.

**When you might violate:**
- **Wails v2.12.0 commitment** — the Wails runtime is deeply
  embedded (per `AGENTS.md` §"Wails runtime hazards"). A future
  Wails-v3 migration is the hardest reversibility problem in the
  repo. The mitigation is the `Wails-PATCH` / `Wails-FormData`
  workaround comments in `frontend/app.js` — the workarounds
  are *named* and *local*, so a v3 migration can find and
  remove them.
- **Schema changes that are non-reversible** — `CREATE TABLE`
  without a corresponding `DROP TABLE` in the downgrade. The
  classification in `reversibility.md` is the *honest* signal:
  future readers know which blocks are forward-only.

### §1.4 — Tracer Bullets — Tip #20

> "Tracer bullets let you home in on your target by trying things
> and seeing how close they land." Small end-to-end vertical
> slices, NOT horizontal layers. Tracer code IS the skeleton of
> the final system — it is NOT a prototype.

**The why:** a tracer bullet proves the path is *wired* end-to-end
without building every layer in isolation. The first bullet
doesn't have to hit the target; the *feedback* is the point.

**Repo operational form:**
- `feature-protocol.md` §"Tracer bullets" — the rule.
- `tracer-bullets` skill — auto-loaded by the agent harness.
- `rpci.md` §I — "the first slice is the only slice that runs in
  the same session." Slice 1 is always a tracer bullet.

**When you might violate:**
- **Documentation-only changes** — no tracer needed for a doc
  commit. The TDD discipline still applies (the doc is the
  surface; the review is the test).
- **Maintenance commits** — `chore:` / `docs:` / `style:` commits
  that ship no behavior change. The fresh-context-per-slice rule
  still applies (the agent must re-read the touched code), but
  the tracer-bullet framing doesn't add signal.

### §1.5 — Design by Contract — Tip #37

> "A correct program is one that does no more and no less than it
> claims to do." Use **preconditions, postconditions, invariants**.

**The why:** contracts make the *boundary* between caller and
callee explicit. A caller that violates a precondition knows
immediately; a callee that violates a postcondition knows
immediately. The contract is the *test surface* — the unit test
is the runtime check.

**Repo operational form:**
- The doc-comment floor (CONTEXT.md Law) — preconditions are
  documented for every exported Go identifier.
- `internal/architecture/architecture_test.go` — package-level
  invariants (no forbidden imports).
- `docs/CODE_CHANGES.md` — the cross-layer working contract.
- The smoke probes (Playwright) — behavioral postconditions for
  the HTTP surface.

**When you might violate:**
- **Third-party library contracts** — when the repo wraps a
  Wails / goldmark / bluemonday / SQLite behavior, the contract
  is *theirs*, not ours. The repo's contract is the wrapper's
  doc comment.
- **Best-effort code paths** — the Wails dispatcher falls back
  to plain HTTP for vanilla Chromium. The contract is "PATCH via
  PATCH when on Chromium, PATCH via X-HTTP-Method-Override when
  on Wails." Both behaviors are documented.

### §1.6 — Dead Programs Tell No Lies (Crash Early) — Tip #38

> "A dead program normally does a lot less damage than a crippled
> one."

**The why:** a "crippled" program keeps running with corrupt
state. The damage compounds. A dead program stops, the user
notices, and the bug is found in 5 minutes instead of 5 hours.

**Repo operational form:**
- The dialog-guard Law (CONTEXT.md) — refusing to start a
  second native dialog is *crashing early* (the UI thread is
  preserved).
- `htmxattr.Mux` swap-allowlist — panics at render time on an
  invalid swap value. The panic is the *signal*.
- The native `<dialog>` revert (issue #117) — better to crash
  loudly than ship a known-bad shape.

**When you might violate:**
- **User-recoverable errors** — a missing optional field is a
  *toast* (graceful degradation), not a crash. The
  `X-DixieData-Toast` contract is the operational form.
- **Background jobs** — a job worker that crashes leaves the
  job in `StatusInterrupted`. The user can re-run from the
  source page. The interrupted status is the *graceful*
  version of "crash early."

### §1.7 — Decoupling / Law of Demeter — Tip #44

> An object's method should call only methods belonging to itself,
> its parameters, objects it creates, or its directly held
> component objects. "Write shy code."

**The why:** chains like `a.b().c().d()` create hidden coupling.
A change to `b` propagates silently. The Law of Demeter says:
"if you want to talk to `d`, ask `a` to talk to `d` for you."

**Repo operational form:**
- `docs/agents/complexity.md` — the deep-module rule covers
  *package*-level decoupling.
- (No formal regression net for *expression*-level Demeter. The
  `gocritic` `rangeValCopy` / `hugeParam` lints catch adjacent
  smells but not Demeter chains. Open gap.)

**When you might violate:**
- **Go's `err` chains** — `if err := a.B(); err != nil { return
  fmt.Errorf("b: %w", c(err)) }` is *not* a Demeter violation;
  the chain is *one* call. Demeter violations look like
  `result := a.B().C().D()` where the intermediate objects
  matter.
- **Builder / fluent APIs** — `htmxattr.Mux{...}.Attrs()` IS a
  Demeter chain. The `htmxattr` package owns the constraint
  (the mux validates as it builds), so the chain is *intentional
  and self-checking*.

### §1.8 — Metaprogramming (abstractions in code, details in metadata) — Tip #79

> "Program for the general case, and put the specifics somewhere
> else — outside the compiled code base."

**The why:** code is expensive to change (rebuild, redeploy);
metadata is cheap (reload). The rule puts the *stable* part in
code and the *changeable* part in metadata.

**Repo operational form:**
- `internal/uiids` — the registry of surface IDs is metadata.
- `internal/routebuilder` — the URL map is metadata.
- `Makefile` lint rules — the rule definitions are *data* that
  the linter consumes.
- The theme system (`internal/theme/`) — theme tokens are
  metadata; the templ components consume them.

**When you might violate:**
- **Performance-critical paths** — when the metadata lookup is
  a hot path, a precomputed constant in code wins. The
  `htmxattr.Mux` builder pre-validates at construction time so
  the render path is metadata-free.
- **Type-checked metadata** — when the metadata has invariants
  that a string registry can't express, a Go enum / typed const
  wins. The `entry_type` / `evidence_type` / archive kind
  CHECK constraints (issue #106) are the operational form.

### §1.9 — Temporal Coupling / Concurrency — Tip #57

> "Reduce any time-based dependencies." Design for concurrency so
> the system can be deployed flexibly and tested for thread safety.

**The why:** a system with hidden time dependencies is hard to
test (you can't pause it), hard to reason about, and prone to
race conditions. Explicit concurrency makes the time dimension
*part of the API*.

**Repo operational form:**
- The Wails `App` struct is per-process — equivalent to an
  actor. The `App` holds all state; goroutines receive a
  pointer.
- `internal/jobs` — the hungry-consumer model. Workers consume
  from a queue, no central scheduler.
- `internal/appshell/app.go` — the `a.inFlight.LoadOrStore` +
  `defer a.inFlight.Delete` pattern is a per-slot mutex.

**When you might violate:**
- **Single-threaded UI paths** — the templ render path is
  single-threaded by construction (Go templates are not
  thread-safe in the templ runtime). The render doesn't *need*
  concurrency; forcing it would add coordination cost for no
  win.
- **Synchronous test paths** — Go tests run sequentially by
  default. The race detector (`go test -race`) is the *opt-in*
  concurrency check.

### §1.10 — It's Just a View (MVC) — Tip #42

> Separate the model from views of the model.

**The why:** a view that knows about the model is bound to one
view of that model. A model that doesn't know about any view can
be rendered any way. MVC decouples the two.

**Repo operational form:**
- `internal/records/`, `internal/db/`, `internal/models/` —
  the model layer (no UI imports).
- `internal/templates/` (templ) + `frontend/app.js` — the view
  layer.
- `internal/appshell/` (HTTP handlers) + Wails bindings — the
  controller layer.
- `internal/viewmodel/` — the explicit grey-box layer between
  the model and the view.

**When you might violate:**
- **Static archive** — the `internal/archive/static_archive.go`
  ships a read-only viewer of the same data the Wails app
  mutates. This is *intentional* — the static archive IS a
  second view of the same model. The principle is satisfied.
- **Audit / debug overlays** — the DEBUG build tag in
  `internal/debug/` is a *transient* view of the model that
  ships nowhere in production. The `//go:build debug` guard
  is the seam.

### §1.11 — Program Deliberately — Tip #62

> "Rely only on reliable things. Beware of accidental complexity,
> and don't confuse a happy coincidence with a purposeful plan."

**The how-to-program-deliberately checklist (8 rules):**
1. Always be aware of what you are doing
2. Don't code blindfolded
3. Proceed from a plan
4. Rely only on reliable things
5. Document your assumptions
6. Test assumptions as well as code
7. Prioritize your effort
8. Don't be a slave to history

**The why:** every "it worked but I don't know why" moment is a
landmine. Program by coincidence is the *fastest* way to ship
a bug.

**Repo operational form:**
- Rules 1-4: RPCI + TDD cover all four.
- Rule 5: doc-comment floor (CONTEXT.md Law) + "What assumptions
  does this PR make?" line in slice plan (open audit gap).
- Rule 6: property-based tests in `internal/dates/`, drift
  detectors like the cheatsheet-renderer alignment in #565.
- Rule 7: fresh-context-per-slice RPCI rule.
- Rule 8: refactor Early / refactor Often (see §1.13 below).

**When you might violate:**
- **Rapid prototyping** — the `.scratch/` directory and the
  `repl` skill are *deliberately* below the bar. The code there
  is disposable; "test assumptions" doesn't apply to throwaway.
- **Documentation work** — the principle is about *code*. Docs
  are checked by the review, not by tests.

### §1.12 — Algorithm Speed (Big O) — Tip #63

> "Get a feel for how long things are likely to take before you
> write code."

**The why:** an O(n²) algorithm on a 10k-row table is a real
problem; the same algorithm on a 100-row table is not. The Big
O annotation tells the next reader *where* to look if the perf
budget breaks.

**Repo operational form:**
- `tdd.md` §"Per-layer recipes" — the per-iter SQL footprint
  doc-comment pattern. The test documents the actual footprint
  (count of SELECTs, INSERTs, transactions) and the budget.
- `TestStressEventAttachDetachRoundTrip` is the worked example.
- (No formal Big O annotation rule. Open audit gap.)

**When you might violate:**
- **Trivial code paths** — a 5-line helper doesn't need an
  O(1) annotation. The rule applies to *non-trivial* functions
  in `internal/records/`, `internal/db/`, `internal/seed/`,
  `internal/jobs/`.
- **Already-validated code** — the per-iter SQL footprint
  pattern subsumes Big O for the database paths. A function
  that already has a documented footprint doesn't need a
  separate Big O annotation.

### §1.13 — Refactoring — Tip #65

> "Just as you might weed and rearrange a garden, rewrite, rework,
> and re-architect code when it needs it. Fix the root of the
> problem."

**The when-to-refactor checklist (5 triggers):**
1. DRY violation
2. Non-orthogonal
3. Knowledge improved
4. Requirements evolve
5. Performance

**The how-to-refactor checklist (3 rules):**
1. Don't try to refactor and add functionality at the same time
2. Make sure you have good tests before you begin refactoring
3. Take short, deliberate steps

**Repo operational form:**
- The Tier 2 / Tier 3 commit rule (`feature-protocol.md`) — the
  "don't refactor and add functionality at the same time" rule
  is operationalized as the slice discipline.
- The v60 → v61 Event Records refactor — the canonical example
  of "knowledge improved" (the `soldier_id` → `person_record_id`
  rename, the v62 `articles` table).
- `tdd.md` — "have good tests before refactoring."

**When you might violate:**
- **Drive-by cleanup** — a "while I'm here" cleanup in a
  feature commit violates rule 1. The Tier 2 / Tier 3 rule
  catches this; the drive-by belongs in a separate commit.
- **Refactor without tests** — the `tdd.md` red-green-refactor
  loop requires tests *first*. A refactor commit that lands
  without test coverage is a Tier 3 maintenance commit, not a
  Tier 2 vertical.

### §1.14 — Code That's Easy to Test — Tip #67

> "Build testability into the software from the very beginning."

**The 7 Aspects of Testing (Ch 8):**
1. Unit testing
2. Integration testing
3. Validation and verification
4. Resource exhaustion, errors, and recovery
5. Performance testing
6. Usability testing
7. Testing the tests themselves

**Repo operational form:**
- (1) Unit: every `internal/<pkg>/` has a `*_test.go`.
- (2) Integration: `appshell_test.go` harness + per-handler
  integration tests.
- (3) Validation: the smoke probes (Playwright) drive the live
  binary.
- (4) Resource: `internal/appshell/zzz_goleak_test.go` +
  `internal/archive/zzz_goleak_test.go`. (Open gap: other
  packages with goroutines.)
- (5) Performance: the per-iter SQL footprint pattern +
  `race-stress.yml` workflow.
- (6) Usability: manual via the manual-audit playbook.
  Automation via axe-core is a follow-up.
- (7) Test-the-tests: not yet. Mutation testing is an open gap.

**When you might violate:**
- **Pure UI code** — usability testing is the only aspect that
  applies, and it is manual.
- **Throwaway code** — `.scratch/` and the `repl` skill are
  below the testing bar by design.

### §1.15 — Ubiquitous Automation — Tip #85

> "A shell script or batch file will execute the same
> instructions, in the same order, time after time."

**The why:** every manual procedure is a place where a future
operator will get it slightly different. Automation is the
*only* way to guarantee repeatability.

**Repo operational form:**
- 23 scripts in `scripts/` + 40+ Makefile targets + 4 GitHub
  workflows.
- `make build` / `make debug` / `make freshness` — the build
  chain.
- `make audit` — the audit sweep.
- `make promote` — the release promotion (per ADR 0008).
- `cli-coverage.mjs` — asserts every documented CLI subcommand
  is implemented.

**When you might violate:**
- **One-time setup** — a single `git mv` is fine by hand; a
  scripted equivalent adds complexity for no win.
- **Operator judgment** — a "promote to stable" decision is
  *not* fully automatable (the human reviews the diff). The
  automation is the *enforcement* (`make promote-dry-run` is
  a CI gate), not the *decision*.

### §1.16 — It's All Writing (English as code) — Tip #13

> "Write documents as you would write code: honor the DRY
> principle, use metadata, MVC, automatic generation."

**The why:** docs that diverge from the code rot. Docs that
live in the same repo (versioned, reviewed, CI-checked) don't
rot as fast.

**Repo operational form:**
- `CONTEXT.md` is the project glossary (DRY for vocabulary).
- `docs/agents/INDEX.md` is the progressive-disclosure table
  (3-tier doc structure).
- `docs/COMMON_BUGS.md` is the per-layer bug catalog (one
  canonical place for recurring patterns).
- The doc-comment floor (CONTEXT.md Law) is the cross-layer
  contract: code and doc are versioned together.

**When you might violate:**
- **Date-stamped web docs** — the book recommends date-stamping
  every web page; the repo's `docs/` are local + version-controlled
  and the date stamp is `git log -1 --format=%ai <file>`. The
  principle is satisfied; the form is different.
- **Per-release notes** — the CHANGELOG is hand-curated today.
  A git-cliff / conventional-changelog generator is an open
  follow-up; the manual curation works for now.

---

## §2 — The 4 book-end checklists

The book has 4 checklists that the principle spine references but
which don't fit under any one principle. Each is a *test* an
agent should run before shipping.

### §2.1 — WISDOM Acrostic (Ch 1, Communicate)

When writing for the user (CHANGELOG, issue body, commit message,
ADR, README), use the WISDOM acrostic:

- **W**hat do you want them to learn?
- What **i**s their interest in what you've got to say?
- How **s**ophisticated are they?
- How much **d**etail do they want?
- Whom do you want to **o**wn the information?
- How can you **m**otivate them to listen to you?

**The why:** a CHANGELOG bullet aimed at the wrong audience either
bores an expert or loses a new contributor. The WISDOM acrostic
forces the writer to choose the audience before choosing the
words.

**When you might violate:** internal-only docs (`.scratch/`,
`/tmp`, debug logs) where the audience is the agent.

### §2.2 — Architectural Questions (Ch 7)

When designing a new module / service / feature, ask:

1. Are responsibilities well defined?
2. Are the collaborations well defined?
3. Is coupling minimized?
4. Can you identify potential duplication?
5. Are interface definitions and constraints acceptable?
6. Can modules access needed data — when needed?

**The why:** the 6 questions are the *test surface* for the deep-
module rule. A "no" on any one of them is a flag that the design
will fight back later.

**Repo operational form:** questions 1-4 are operationalized by
the deep-module rule. Questions 5-6 are open (the bookend
checklist isn't in any tier-1 doc yet).

### §2.3 — Debugging Checklist (Ch 3)

When stuck on a bug, ask:

1. Is the problem being reported a direct result of the underlying
   bug, or merely a symptom?
2. Is the bug really in the compiler? Is it in the OS? Or is it
   in your code?
3. If you explained this problem in detail to a coworker, what
   would you say?
4. If the suspect code passes its unit tests, are the tests
   complete enough? What happens if you run the unit test with
   this data?
5. Do the conditions that caused this bug exist anywhere else in
   the system?

**The why:** the 5 questions are the *test surface* for the
"fix root cause, not symptom" pattern. A "no" on question 5 is
a flag for the "find bugs once" rule — the bug will recur.

**Repo operational form:** spirit is enforced by the orphan-handler
probe + the per-bug regression test discipline (per `tdd.md`).
The 5-question checklist is not yet in any tier-1 doc; this doc
is the first place it lives.

### §2.4 — Cutting the Gordian Knot (Ch 7)

When solving an impossible problem, ask:

1. Is there an easier way?
2. Am I solving the right problem?
3. Why is this a problem?
4. What makes it hard?
5. Do I have to do it this way?
6. Does it have to be done at all?

**The why:** the 6 questions are the *test surface* for the
"find the box" rule. A "yes" on question 6 ("don't have to do
it at all") is the most valuable answer in software.

**When you might violate:** a user-stated requirement that
*is* required (a tax form, a regulatory export). The 6 questions
help the agent find a *cheaper* way; they don't help with a
*non-negotiable* way.

---

## §3 — Cross-reference: principle ↔ repo law

Some principles are *also* enforced as Laws (in `CONTEXT.md`).
This is intentional: the principles are the *why*; the Laws are
the *earned-by-real-bug operational form* of the principles. A
principle may have multiple Laws under it; a Law may pin a single
operational form of a principle.

| Principle | Repo Law(s) |
|---|---|
| DRY | "Exported Go identifiers carry doc comments" (operationalized as the cross-layer documentation contract) |
| Orthogonality | (No Law; enforced by the architecture test) |
| Reversibility | (No Law; the three-branch model + the reversibility classification are the operational form) |
| Tracer Bullets | (No Law; the slice discipline is the operational form) |
| Design by Contract | (No Law; the doc-comment floor + the architecture test are the operational form) |
| Dead Programs | "Every native dialog call is guarded against re-entry" (the dialog-guard Law) |
| Law of Demeter | (No Law; covered by the deep-module rule at the package level) |
| Metaprogramming | (No Law; the `uiids` + `routebuilder` registries are the operational form) |
| Temporal Coupling | "Every native dialog call is guarded against re-entry" (re-entry protection) |
| MVC | (No Law; the architecture test enforces the viewmodel grey-box) |
| Program Deliberately | "No feature PR ships a backend surface without a UI apply-site" (operationalized as the test-the-assumption rule) |
| Algorithm Speed | (No Law; the per-iter SQL footprint pattern is the operational form) |
| Refactoring | (No Law; the Tier 2 / Tier 3 commit rule is the operational form) |
| Code That's Easy to Test | "No feature PR ships a backend surface without a UI apply-site" (the smoke-probe-per-apply-site contract) |
| Ubiquitous Automation | "Fresh debug build = `make freshness`" |
| It's All Writing | "Exported Go identifiers carry doc comments" |
| Great Expectations | (No Law; the fixship cadence is the operational form) |

The principle is the *why*; the Law is the *what*. Both are
useful. New contributors should read the principles (this doc);
the *reviewer* reads the Laws. The principle explains why the
Law exists; the Law tells you what to do without re-deriving
the why.

---

## §4 — How to extend this doc

When a new principle-violation pattern shows up in a PR review:

1. **File the violation in the slice commit's "Principle warnings"
   block** (per §"The warn + cite protocol" above).
2. **If the violation is the kind that will recur**, add a
   "When you might violate" bullet to the relevant principle
   section. The doc grows with the codebase.
3. **If the violation is the kind that should NOT recur**, file
   a follow-up issue for the cleanup, and add a "Known gap"
   bullet to the principle section. The doc + the issue
   track the violation together.

When a new principle is needed (e.g. a 17th principle emerges
from a new book or a new pattern in the codebase), add a new
§1.N section following the same template. The doc is *additive*;
a new section is one PR.

---

## §6 — Tip index (the 100 tips, by number)

This index is the **operational counterpart** to the principle
spine in §1. Each tip is a concrete behavior; the principle
is the *why* behind the behavior. The principle spine is what
an agent reads when designing a new feature; this index is what
an agent reads when checking whether a specific commit or
behavior is in compliance.

**State legend:**
- ✅ **Enforced** — a `CONTEXT.md` Law, a tier-1 `docs/agents/` doc, a regression test, an ADR, or a workflow file pins the tip.
- ⚠️ **Partial** — the spirit is satisfied but no written rule, OR the rule exists but a recent commit violated it. Cited.
- ❌ **Gap** — the tip is not addressed and would be high-leverage to add. A candidate follow-up issue exists.
- ➖ **N/A** — the tip applies to contexts DixieData does not inhabit.

**Principle column** maps the tip to the §1.x section in this doc
that explains the *why*. Tips with a "philosophy / technique"
note in the Principle column are *attitudes* (Ch 1 of the book)
or *techniques* (Ch 3+), not principles per se — they inform
behavior but are not operationalized as a deep-module rule.

**One-line evidence** is truncated to ~110 chars. The full
evidence per tip lives in the source audit at
`docs/audit/pragmatic-programmer-audit-2026-07.md` (the
100-tip map retained as a historical artifact).

| # | Tip | State | Principle | One-line evidence |
|---|---|---|---|---|
| 1 | Care About Your Craft | ✅ | — (philosophy / technique, not a principle) | `AGENTS.md` "Bias toward action" + the entire quality regime (test floor, smoke probes, doc-comment floor, ... |
| 2 | Think! About Your Work | ✅ | — (philosophy / technique, not a principle) | RPCI flow (`docs/agents/rpci.md`) explicitly turns off the autopilot: every plan has a Critique phase the u... |
| 3 | You Have Agency | ✅ | — (philosophy / technique, not a principle) | RPCI Critique gate is the operational form — the user has explicit "approve, start" authority. |
| 4 | Provide Options, Don't Make Lame Excuses | ✅ | — (philosophy / technique, not a principle) | RPCI surfaces "decisions to confirm" with options; the user picks, the agent never says "can't be done." |
| 5 | Don't Live with Broken Windows | ✅ | — (philosophy / technique, not a principle) | `CONTEXT.md` Laws are earned-by-real-bug rules; the Backend-First Law explicitly names the four "shipped bu... |
| 6 | Be a Catalyst for Change | ⚠️ | — (philosophy / technique, not a principle) | The repo does this implicitly via the audit cadence (issue #561 → #560) but has no written rule. Process is... |
| 7 | Remember the Big Picture | ✅ | — (philosophy / technique, not a principle) | The `docs/agents/INDEX.md` progressive-disclosure table is the operational form — agents know which tier to... |
| 8 | Make Quality a Requirements Issue | ⚠️ | — (philosophy / technique, not a principle) | Every feature issue carries an "Acceptance criteria" section. But quality requirements are implicit (test-f... |
| 9 | Invest Regularly in Your Knowledge Portfolio | ❌ | — (philosophy / technique, not a principle) | No `docs/learning/` or per-agent reading list. The audit (this doc) is a one-shot, not a habit. |
| 10 | Critically Analyze What You Read and Hear | ⚠️ | §1.19 WISDOM | The repo applies this in the "forgo following fads" §"Yesterday's Best Practice Becomes Tomorrow's Antipatt... |
| 11 | English is Just Another Programming Language | ✅ | §1.1 DRY | `CONTEXT.md` Laws are written in plain English with `_Avoid_` lists — treated as code: every commit is revi... |
| 12 | It's Both What You Say and the Way You Say It | ✅ | §1.19 WISDOM | The CHANGELOG is exemplary — long-form bullets explain the *why*, the regression net, and the out-of-scope.... |
| 13 | Build Documentation In, Don't Bolt It On | ✅ | — | Doc-comment floor (CONTEXT.md "Exported Go identifiers carry doc comments" Law) + `internal/uiids` registry... |
| 14 | Good Design Is Easier to Change Than Bad Design | ✅ | §1.3 Reversibility | The deep-module discipline (`docs/agents/complexity.md`) + the architecture forbidden-import test = the rul... |
| 15 | DRY—Don't Repeat Yourself | ✅ | §1.1 DRY | The routebuilder is the canonical example (single source of truth for every URL). `internal/routebuilder` +... |
| 16 | Make It Easy to Reuse | ✅ | §1.1 DRY | The `components/` design-system primitives (Foldout, Button, Card, Pill, EmptyState, Field, Toast) are the ... |
| 17 | Eliminate Effects Between Unrelated Things | ✅ | §1.2 Orthogonality | `internal/architecture/architecture_test.go` forbidden-import table is the regression net; deep-module list... |
| 18 | There Are No Final Decisions | ✅ | §1.3 Reversibility | The three-branch model (ADR 0009) is the operational form: `dev` for integration, `stable` for release, `ma... |
| 19 | Forgo Following Fads | ⚠️ | §1.3 Reversibility | Implicit in the architecture choices (templ + chi + goldmark) but no written policy. A future "rewrite in R... |
| 20 | Use Tracer Bullets to Find the Target | ✅ | §1.4 Tracer Bullets | `tracer-bullets` skill (auto-loaded) + `feature-protocol.md` §"Tracer bullets" + the 3-tier commit rule. Th... |
| 21 | Prototype to Learn | ⚠️ | — (philosophy / technique, not a principle) | `.scratch/` is the prototype playground (Python scripts, MCP probes) and `repl` skill is a scratch tool. Bu... |
| 22 | Program Close to the Problem Domain | ✅ | — (philosophy / technique, not a principle) | The `articles` package (just shipped in #565) is the exemplar — domain types `CheatSheetRow` not infrastruc... |
| 23 | Estimate to Avoid Surprises | ⚠️ | — (philosophy / technique, not a principle) | RPCI Plan includes "files touched, success criteria, regression net" but no time estimate. The "Bias toward... |
| 24 | Iterate the Schedule with the Code | ➖ | — (philosophy / technique, not a principle) | N/A. Single-binary single-user project; no schedule to iterate. |
| 25 | Keep Knowledge in Plain Text | ✅ | — (philosophy / technique, not a principle) | Every config is plain text (Makefile, .json, .md, .ps1). No binary configs. `docs/migrations/v{N}.md` is th... |
| 26 | Use the Power of Command Shells | ✅ | — (philosophy / technique, not a principle) | `audit/_lib/`, `scripts/*.ps1`, `tools/tune/cli-coverage.mjs`, `make` targets. The `probe-clean.ps1` + `run... |
| 27 | Achieve Editor Fluency | ➖ | — (philosophy / technique, not a principle) | Agent-context; not a code/doc concern. |
| 28 | Always Use Version Control | ✅ | — (philosophy / technique, not a principle) | The entire branching model (ADR 0009). Every change goes through git. |
| 29 | Fix the Problem, Not the Blame | ⚠️ | — (philosophy / technique, not a principle) | Implicit in CHANGELOG tone ("two compounding bugs in X" — no person named) but no stated rule. The ticket-c... |
| 30 | Don't Panic | ➖ | — (philosophy / technique, not a principle) | N/A. No incident response. |
| 31 | Failing Test Before Fixing Code | ✅ | — (philosophy / technique, not a principle) | `docs/agents/tdd.md` red-green-refactor is the operational form. The bug-pattern-grep doc says: write the R... |
| 32 | Read the Damn Error Message | ✅ | §1.6 Dead Programs | The audit probes assert the response shape AND the post-click URL AND the DOM state (not "we got an error s... |
| 33 | "select" Isn't Broken | ⚠️ | §1.6 Dead Programs | Implicit (no recent commit blamed SQLite or WebView2 without evidence) but no stated rule. |
| 34 | Don't Assume It—Prove It | ✅ | §1.6 Dead Programs | The `tools/tune/snapshot_test.go` per-iter SQL footprint doc-comment pattern is the operational form — ever... |
| 35 | Learn a Text Manipulation Language | ➖ | — (philosophy / technique, not a principle) | N/A at the repo level. |
| 36 | You Can't Write Perfect Software | ✅ | §1.5 Design by Contract | The dialog-guard Law (CONTEXT.md) is the operational form. The "Fail loud, no silent fallback" decision in ... |
| 37 | Design with Contracts | ✅ | §1.5 Design by Contract | `internal/architecture/architecture_test.go` is the package-contract enforcement. `docs/CODE_CHANGES.md` is... |
| 38 | Crash Early | ✅ | §1.5 Design by Contract | `htmxattr.Mux` swap-allowlist panic at render time. The native `<dialog>` revert (#117) is the worked examp... |
| 39 | Use Assertions to Prevent the Impossible | ✅ | §1.5 Design by Contract | `templ.Attributes` typed spread + `routebuilder` typed URL builder + `internal/uiids` registry IDs all asse... |
| 40 | Finish What You Start | ✅ | — (philosophy / technique, not a principle) | Go's `defer` for resource close + the defer-close lint rule (`.agents/adr/0010-lint-enforcement.md` referen... |
| 41 | Act Locally | ✅ | — (philosophy / technique, not a principle) | Function-scope variables; the Wails v2.12.0 dialog-guard mutex is a per-handler-scope guard. |
| 42 | Take Small Steps—Always | ✅ | §1.10 MVC | RPCI is the operational form. Slice 1 is always the tracer bullet. The "fresh context per slice" rule in `r... |
| 43 | Avoid Fortune-Telling | ✅ | — (philosophy / technique, not a principle) | The "YAGNI" rule in `docs/agents/feature-protocol.md` §"Module discipline" + the two-adapter rule are the o... |
| 44 | Decoupled Code Is Easier to Change | ✅ | §1.7 Law of Demeter | The deep-module discipline + architecture test. The v60 → v61 Event Records refactor (sibling-table rename)... |
| 45 | Tell, Don't Ask | ⚠️ | §1.7 Law of Demeter | The DTO discipline (UI depends on service DTOs, never on persistence structs) is the closest form. The rule... |
| 46 | Don't Chain Method Calls | ➖ | §1.7 Law of Demeter | N/A in Go. The law of demeter is implicit in the deep-module rule. |
| 47 | Avoid Global Data | ✅ | §1.2 Orthogonality | The "no package-level mutable state" rule (implicit in the deep-module discipline) + Wails' `App` struct is... |
| 48 | If It's Important Enough To Be Global, Wrap It in an API | ✅ | §1.2 Orthogonality | The `a.guardedSaveFileDialog` wrapper for native dialogs is the canonical example — the dangerous thing (th... |
| 49 | Programming Is About Code, But Programs Are About Data | ✅ | §1.2 Orthogonality | The viewmodel layer is the operational form. Every cross-boundary value is a DTO. The audit probes assert o... |
| 50 | Don't Hoard State; Pass It Around | ✅ | §1.2 Orthogonality | Function arguments over package-level state. The Markdown cheatsheet work (#565) is the recent example — th... |
| 51 | Don't Pay Inheritance Tax | ➖ | §1.2 Orthogonality | N/A in Go (no inheritance). Composition is the only path; the rule is implicit. |
| 53 | Shared State Is Incorrect State | ✅ | — (philosophy / technique, not a principle) | The dialog-guard mutex + the Wails `App` struct pattern + the per-job worker context. `internal/jobs/jobs.g... |
| 54 | Random Failures Are Often Concurrency Issues | ✅ | — (philosophy / technique, not a principle) | `audit/race-stress.yml` workflow + the `internal/dates` property-test gate. The issue #479 advisory-downgra... |
| 55 | Use Actors For Concurrency Without Shared State | ⚠️ | — (philosophy / technique, not a principle) | The Wails `App` is a struct passed by reference; not technically an actor. The "jobs.Start" pattern in `int... |
| 56 | Analyze Workflow to Improve Concurrency | ✅ | §1.9 Temporal Coupling | The export flow (PDF / JSON / archive) is jobs-based; the user gets toast + jobs page in parallel. The `X-D... |
| 57 | Use Blackboards to Coordinate Workflow | ➖ | §1.9 Temporal Coupling | N/A. Single-user app; no blackboard pattern. |
| 59 | Listen to Your Inner Lizard | ⚠️ | §1.9 Temporal Coupling | Implicit. The "agent inner lizard" surfaced in the 6-commit over-decomposition anti-pattern (AGENTS.md "Com... |
| 60 | Don't Program by Coincidence | ✅ | §1.9 Temporal Coupling | The `internal/dates` property-test gate is the operational form. The CHANGELOG entry tone is "root cause: X... |
| 61 | Estimate the Order of Your Algorithms | ⚠️ | — (philosophy / technique, not a principle) | The `TestStressEventAttachDetachRoundTrip` per-iter SQL footprint is the closest form. Not a stated rule. |
| 62 | Test Your Estimates | ✅ | §1.11 Program Deliberately | `tools/tune/stress/` + `.github/workflows/race-stress.yml`. The race-detector step is the live regression net. |
| 63 | Refactor Early, Refactor Often | ✅ | §1.12 Algorithm Speed | `docs/agents/complexity.md` is the operational form. The v60 → v61 Event Records refactor is the worked exa... |
| 64 | Testing Is Not About Finding Bugs | ✅ | §1.12 Algorithm Speed | The audit cadence is the operational form. Issues #531, #539, #540, #542 each surfaced a *class* of bug the... |
| 65 | A Test Is the First User of Your Code | ✅ | §1.13 Refactoring | `tdd.md` Step 1 is "RED: write the failing test first." The Markdown cheatsheet work (#565) shipped RED + G... |
| 66 | Build End-To-End, Not Top-Down or Bottom Up | ✅ | §1.13 Refactoring | RPCI tracer-bullet slice 1 is the operational form. Every Tier 2 vertical slice crosses every layer. |
| 67 | Design to Test | ✅ | §1.11 Program Deliberately | `internal/architecture/architecture_test.go` + `audit/discover_orphan_handlers.mjs` + the smoke-probe per-a... |
| 68 | Test Your Software, or Your Users Will | ✅ | §1.4 Tracer Bullets | The smoke-probe per-apply-site contract (per `feature-protocol.md` "Backend-First Law" + `tdd.md` "Per-laye... |
| 69 | Use Property-Based Tests to Validate Your Assumptions | ✅ | §1.17 Great Expectations | `internal/dates/dates_property_test.go` uses `pgregory.net/rapid`. Documented in `tdd.md` §"Per-layer recip... |
| 70 | Keep It Simple and Minimize Attack Surfaces | ✅ | — (philosophy / technique, not a principle) | The "no new component primitives" rule from the Markdown cheatsheet work (#565) is the recent example. The ... |
| 71 | Apply Security Patches Quickly | ❌ | — (philosophy / technique, not a principle) | No `govulncheck` or `gosec` in `Makefile` / `.github/workflows/`. Deps are updated when a PR forces it, not... |
| 72 | Name Well; Rename When Needed | ⚠️ | — (philosophy / technique, not a principle) | The glossary tier-2 rename (issue #97) is the canonical example. But "rename when needed" is not a stated p... |
| 73 | Sign Your Work | ✅ | — (philosophy / technique, not a principle) | The CHANGELOG fixship-by-fixship attribution is the operational form. The "Ticket close-out law" in `featur... |
| 74 | No One Knows Exactly What They Want | ✅ | — (philosophy / technique, not a principle) | RPCI: "Plan → Critique" with explicit user gate. The "the user is the chat" capture rule in AGENTS.md. |
| 75 | Programmers Help People Understand What They Want | ✅ | — (philosophy / technique, not a principle) | The slice plan + apply-sites checklist in every feature issue is the operational form. The user sees the *f... |
| 76 | Requirements Are Learned in a Feedback Loop | ✅ | — (philosophy / technique, not a principle) | RPCI's "fresh context per slice" is the strongest form. The next session picks up the new state and refines. |
| 77 | Work with a User to Think Like a User | ⚠️ | — (philosophy / technique, not a principle) | Implicit in the user's "go ahead and tackle 565" pattern, but no stated cadence. The audit cycle (issue #56... |
| 78 | Policy Is Metadata | ✅ | §1.8 Metaprogramming | The `internal/uiids` registry + the architecture test forbidden-import map + the linter rule files are the ... |
| 79 | Use a Project Glossary | ✅ | §1.8 Metaprogramming | `CONTEXT.md` is the project glossary. The "Flagged ambiguities" section is the regression net for vocabular... |
| 81 | Don't Think Outside the Box—Find the Box | ✅ | — (philosophy / technique, not a principle) | The slice-3.6 → slice-4 markdown editor + preview modal rework (issue #526) is the worked example — instead... |
| 82 | Don't Go into the Code Alone | ⚠️ | — (philosophy / technique, not a principle) | Implicit (the user + agent pairing) but no stated rule. The `.rpiv/artifacts/issues/` + `.scratch/issues/` ... |
| 83 | Agile Is Not a Noun; Agile Is How You Do Things | ➖ | — (philosophy / technique, not a principle) | N/A. |
| 84 | Maintain Small Stable Teams | ➖ | — (philosophy / technique, not a principle) | N/A. Solo project. |
| 85 | Schedule It to Make It Happen | ⚠️ | §1.15 Ubiquitous Automation | The `make freshness` gate + the daily `make test` + the CHANGELOG cadence are the closest form. But "schedu... |
| 86 | Organize Fully Functional Teams | ➖ | §1.15 Ubiquitous Automation | N/A. |
| 87 | Do What Works, Not What's Fashionable | ⚠️ | — (philosophy / technique, not a principle) | Implicit in the architecture choices (no React, no ORM, no microservices). No stated policy. |
| 88 | Deliver When Users Need It | ✅ | — (philosophy / technique, not a principle) | The Tier 3 apply-site rule + the slice-1-only-per-session rule = ship the smallest useful unit as fast as p... |
| 89 | Use Version Control to Drive Builds, Tests, and Releases | ✅ | — (philosophy / technique, not a principle) | `.github/workflows/{build,test,audit,race-stress}.yml` are the operational form. Every PR triggers the full... |
| 90 | Test Early, Test Often, Test Automatically | ✅ | §1.15 Ubiquitous Automation | Per-PR + per-push + weekly race-stress + per-promotion freshness. |
| 91 | Coding Ain't Done 'Til All the Tests Run | ✅ | §1.15 Ubiquitous Automation | The "RED + GREEN + adjacent-behavior sweep" in `tdd.md` + the package-floor + the smoke-probe per-apply-sit... |
| 92 | Use Saboteurs to Test Your Testing | ❌ | — (philosophy / technique, not a principle) | No mutation testing. The `tools/tune/` golden-snapshot tests catch regressions but not silent test-skipping. |
| 93 | Test State Coverage, Not Code Coverage | ⚠️ | — (philosophy / technique, not a principle) | The smoke probes assert state (response shape, URL, DOM) not just code paths. But there is no coverage metr... |
| 94 | Find Bugs Once | ✅ | §1.15 Ubiquitous Automation | Every `fix:` commit in CHANGELOG grows a regression test. The 79 `fix:` commits with regression nets in `do... |
| 95 | Don't Use Manual Procedures | ✅ | §1.15 Ubiquitous Automation | `make freshness` automates the build + probe chain. `make audit` automates the audit sweep. `make promote` ... |
| 96 | Delight Users, Don't Just Deliver Code | ✅ | §1.17 Great Expectations | The Markdown cheatsheet work (#565), the feedback modal (#544), the bug-report bundle (#545) are the recent... |
| 98 | First, Do No Harm | ✅ | — (philosophy / technique, not a principle) | The dialog-guard Law + the "no feature PR ships a backend surface without a UI apply-site" Law + the per-it... |
| 99 | Don't Enable Scumbags | ➖ | — (philosophy / technique, not a principle) | N/A. |
| 100 | It's Your Life. Share it. Celebrate it. Build it. AND HAVE FUN! | ✅ | — (philosophy / technique, not a principle) | The CHANGELOG tone + the "Bias toward action" rule + the user's "go ahead and tackle X" cadence. The repo i... |

## §7 — Summary: the 100 tips in numbers

| State | Count | What it means |
|---|---|---|
| ✅ Enforced | 50+ | The repo is in compliance. The principle spine in §1 names the operational form. |
| ⚠️ Partial | ~30 | The spirit is satisfied but no written rule. A future commit could regress without the agent noticing. |
| ❌ Gap | ~5 | Not addressed. Each gap is a candidate follow-up issue; see §1 in this doc for the principle-level gaps. |
| ➖ N/A | ~10 | Doesn't apply (multi-team management, hiring, incident response, etc.). |

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
  `docs/audit/pragmatic-programmer-audit-2026-07.md` (100-tip
  view, retained as a historical artifact) +
  `docs/audit/pragmatic-programmer-principles-audit-2026-07.md`
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
