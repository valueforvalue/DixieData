# Managing Complexity in a Growing Codebase

> Pair with [`AGENTS.md`](../../AGENTS.md) §YAGNI (refined: features
> only, not interfaces) and [`feature-protocol.md`](feature-protocol.md)
> §Module discipline (deep-module). This doc carries the *why* behind
> both rules. The others carry the *what* and *how*.

A growing codebase's failure mode is not bugs. It's that **change
becomes linear in global system size instead of local module size.**
This doc collects the wisdom that keeps the cost of change proportional
to the component being changed, not to the system surrounding it.

The wisdom here is imported from the broader practitioner canon
(Ousterhout, Hickey, Naur, Brooks, McKinley, Metz, Brown, Lehman,
Majiors). The DixieData-specific applications live in
[`MISTAKES.md`](../../MISTAKES.md) (recurring-failure roll call) and
[`AGENT_ARCHITECTURE_MAP.md`](../../AGENT_ARCHITECTURE_MAP.md) (current
architecture instance). Cite this doc when the principle applies; cite
those for the codebase's actual evidence of it.

---

## 1. The reconciliation — YAGNI for features, broad interface for deep modules

YAGNI ("You Aren't Gonna Need It") forbids speculative *features*.
John Ousterhout's *A Philosophy of Software Design* argues YAGNI was
*never* about interface shape — only about behavior. The two camps
agree on one failure mode: code written only for today's feature, with
no thought for tomorrow's reader.

**The operational rule this repo adopts:**

- **Implement** the smallest set of methods/cases that today's
  acceptance criterion demands. No methods nobody calls. No branches
  nobody exercises.
- **Design the interface** slightly broader than today's needs when
  doing so *removes* complexity from callers, future tests, or future
  readers — not when it merely *anticipates* a feature.

This matches Ousterhout's "somewhat general-purpose" principle
(PoSD Ch. 6) and Kent Beck's original C3-project rationale (which
always paired YAGNI with refactoring-and-test infrastructure).

> "In order for an element to provide a net gain against complexity,
> it must eliminate some complexity that would be present in the
> absence of the design element." — Ousterhout, PoSD Ch. 1

### Net-complexity-gain test (operational)

Before adding an abstraction, hook, parameter, or extension point,
ask: *does this design element remove more complexity than it adds?*

**In DixieData:** pair this test with the **two-adapter rule** in
`feature-protocol.md` §Module discipline:

- 0 callers → pass-through, refactor into the caller.
- 1 caller → borderline; justify or drop.
- 2+ callers → earning its keep; keep the module.

**Sources.** Ousterhout, PoSD Ch. 1, 6, 19. jmoiron's
[notes](https://jmoiron.net/blog/notes-on-philosophy-of-software-design)
include the pragmatic counter — tactical when unknowns are many,
strategic when unknowns are few.

---

## 2. Tactical vs. strategic programming

> "Agile development tends to focus developers on features, not
> abstractions, and it encourages developers to put off design decisions
> in order to produce working software as soon as possible. … This
> can result in a rapid accumulation of complexity." — Ousterhout,
> PoSD Ch. 19

> "Developing incrementally is generally a good idea, but the
> increments of development should be abstractions, not features." —
> Ousterhout, PoSD, summary chapter

**In DixieData.** Each feature slice in `feature-protocol.md` ends
with a check for *prefactor opportunities* — modular shapes that make
future slices easier. That check is the strategic-programming moment.
Don't skip it because the slice "already works."

**Sources.** Ousterhout PoSD Ch. 3 ("tactical tornado"), Ch. 19.
*Refactoring* 2nd ed. names Speculative Generality as the code smell
that mirrors the opposite failure — too much strategic design up front.

---

## 3. Pull complexity downward; design interfaces once

> "The best modules are those whose interfaces are much simpler than
> their implementations." — Ousterhout, PoSD Ch. 4

> "Most modules have more users than developers, so it is better for
> the developers to suffer than the users." — Ousterhout, PoSD Ch. 4

**In DixieData.** The deep-module discipline in `feature-protocol.md`
§Module discipline is the operational form: write the public service
interface + DTO contracts before any internal code. Internal helpers
stay private. If a caller needs to reach into internals, the seam is
wrong — fix it before shipping the slice.

Concrete DixieData examples worth re-reading:
- `internal/htmxattr/` (typed `htmxattr.Mux` builder) — replaces
  raw `hx-*` string concatenation across the whole frontend. One
  module's interface removes a class of bug from N callers.
- `internal/routebuilder/` (typed URL builders) — replaces string
  route literals. Same pattern.
- `internal/uiids/` (canonical DOM ID constants) — replaces
  copy-paste string literals across goquery invariant tests.

These three are the running examples of *pull complexity downward*
in this codebase.

**Define errors out of existence** is the related trick (PoSD Ch. 7)
— redesign the interface so invalid states cannot be expressed,
instead of forcing every caller to handle them. Look at any
`internal/debug/` or `internal/htmxattr/` decision and ask "could
the type system have caught this?"

**Sources.** Ousterhout PoSD Ch. 4, 6, 8.

---

## 4. Decomplect: prefer one-dimensional abstractions

> "Simplicity is a great victory over complexity. … Complecting
> multiple concerns in one place guarantees that any later change
> ripples through unrelated logic." — Rich Hickey, "Simple Made
> Easy" (Strange Loop 2011)

**In DixieData.** Apply as a tiebreaker when two module shapes both
look reasonable: prefer the one that separates concerns along
orthogonal axes (persistence shape vs. domain shape, request vs.
response, validation vs. business rule). The deep-module discipline
already enforces this at the interface boundary; decomplecting
enforces it inside modules.

**Sources.** Hickey,
[Simple Made Easy transcript](https://github.com/matthiasn/talk-transcripts/blob/master/Hickey_Rich/SimpleMadeEasy.md) (2011).

---

## 5. Wait for the third use: the rule of three

> "Duplication is far cheaper than the wrong abstraction." — Sandi
> Metz, "The Wrong Abstraction" (2016)

**In DixieData.** When two apply-sites share logic, write the
duplication and add a comment noting the candidate for extraction.
Extract on the third. The two-adapter rule in `feature-protocol.md`
§Module discipline is the post-extraction sanity check — count
callers before keeping the abstraction.

**Sources.** Metz,
[The Wrong Abstraction](https://sandimetz.com/blog/2016/1/20/the-wrong-abstraction).
Rule of Three across
[Coding Horror](https://blog.codinghorror.com/the-big-ball-of-mud-and-other-architectural-disasters/)
and *Refactoring*.

---

## 6. Enforce boundaries, don't honour them

> "If everything is marked `public`, all four architectural
> approaches presented before are exactly the same." — Simon Brown
> (paraphrased)

**In DixieData.** Where Go allows, mark internals as lowercase.
Where the boundary crosses package edges, the budgeted next-best
is a custom lint rule or an architectural test that fails CI on a
leak. Per `feature-protocol.md` §Service is the seam: UI depends
on the service's DTOs, never on the persistence struct.

Concrete enforcement we already use:
- The `internal/` directory is the Go-stdlib enforcement that
  external packages can't import our internals.
- `MISTAKES.md` carries the recurring lessons that the language
  can't enforce.

**Sources.** Brown, *Modular Monoliths*. Ford, Parsons, Kua,
*Building Evolutionary Architectures* — fitness functions turn
architecture into a CI failure when it drifts.

---

## 7. Code is the artifact; the theory is the deliverable

> "The theory of a program … has to be possessed by [the]
> members of the programming team, since it cannot be embodied in
> the program text." — Peter Naur, "Programming as Theory Building"
> (1985)

**In DixieData.** Theory-transfer is the job of `MISTAKES.md`,
`AGENT_ARCHITECTURE_MAP.md`, `CONTEXT.md` (the glossary), and the
in-progress ADR practice. Source code alone cannot transfer a theory.
New joiners onboard by sitting with people who hold it, by reading the
*decisions* about it, and by exercising it under guidance.

**Operational rule:** the source code is the artifact, but the
*archive* (`CONTEXT.md`, the ADRs, `MISTAKES.md`, this doc) is the
deliverable. When you change a subsystem shape, update the archive
in the same slice.

**Sources.** Naur,
[*Programming as Theory Building*](https://pages.cs.wisc.edu/~remzi/Naur.pdf)
(1985).

---

## 8. Choose boring technology; spend innovation tokens deliberately

> "Innovation is costly, so you should choose standard, well-understood,
> rock-solid technologies insofar as you possibly can. You only get a
> few innovation tokens to spend, so you should spend them on technologies
> that can give you a true competitive advantage." — Dan McKinley,
> *Choose Boring Technology*

**In DixieData.** The runtime commitments are well-baked already
(SQLite, Wails, HTMX, templ, chi). New dependencies should arrive
only when they solve an enumerated bottleneck. Every line of code is
a maintenance commitment; every exotic library is future cost in
cognitive load, upgrade pain, and bus-factor risk.

**Sources.** McKinley,
[*Choose Boring Technology*](http://boringtechnology.club/). Charity
Majors, [Honeycomb blog](https://charity.wtf/).

---

## 9. Indirection is paid for only at boundaries that actually change

> "The most fundamental problem in software development is complexity.
> There is only one basic way of dealing with complexity: divide and
> conquer." — Bjarne Stroustrup

> "Any problem in computer science can be solved with another layer of
> indirection, except of course for the problem of too many
> indirections." — David Wheeler (via Stroustrup)

**In DixieData.** Add indirection at boundaries that actually change:
persistence (SQLite migrations), transport (HTML/JSON shape), third-party
integration (anything in `internal/exportcontract/`), UI framework
events (HTMX + Wails boundary). Never inside stable logic.

**Sources.** Stroustrup [quotes](https://www.stroustrup.com/quotes.html).

---

## 10. Entropy is the default; allocate complexity budget

> "Software in use must change or become progressively less useful."
> — Lehman, *Laws of Software Evolution*, summary

> "Almost every system that *works* today will be wrong tomorrow,
> because the environment in which it operates will change."

**In DixieData.** Allocate complexity budget per slice. If a slice
exceeds it (long prefactor, sprawling test matrix, broad churn in
unrelated modules), decompose before shipping — the slice is bigger
than `feature-protocol.md` §Tracer-bullets allows. The deletion test
and two-adapter rule together form the budget guard.

**The lead indicator.** A function whose `consensus_score` rises
across consecutive [`consensus-hunter`](consensus-hunter.md) runs is a
Lehman §10 warning. Treat that as a slice-budget alarm.

**Sources.** Lehman & Belady,
[*Laws of Software Evolution*](https://en.wikipedia.org/wiki/Lehman%27s_laws_of_software_evolution).

---

## How to use this doc

- **On adoption.** Read end to end once.
- **During slicing.** Cross-reference the principle the slice most
  tests (often §1 net-complexity-gain or §5 rule of three) before
  declaring done.
- **On suspicion of YAGNI violation.** Run the net-complexity-gain
  test against the proposed abstraction. If it doesn't pass, drop it.
- **On reviewing a PR.** Cite section numbers in review comments;
  don't re-argue the foundations.
- **When in doubt.** "Working code isn't enough" — but don't take
  that to the opposite extreme. Every rule has its exceptions. Match
  depth to the task (AGENTS.md Rule 5, Proportional depth).

## Cross-references

- [`AGENTS.md`](../../AGENTS.md) — Rule 3 (YAGNI, refined to read
  with this doc), Rule 5 (proportional depth), §Capturing decisions.
- [`feature-protocol.md`](feature-protocol.md) — §Module discipline,
  §Tracer-bullets, §Prefactor before slicing, §Issue template.
- [`rpci.md`](rpci.md) — Design phase enforces strategic-programming
  increments, not feature increments.
- [`MISTAKES.md`](../../MISTAKES.md) — codebase-specific evidence
  for each principle.
- [`AGENT_ARCHITECTURE_MAP.md`](../../AGENT_ARCHITECTURE_MAP.md) —
  the codebase-specific instance of "current architecture, why it
  is shaped this way."
- [`CONTEXT.md`](../../CONTEXT.md) — the glossary; the theory-of-the-
  domain in Naur's sense.

## References (consolidated)

- John Ousterhout, *A Philosophy of Software Design*, 2nd ed. (2021)
  — primary source for §1–§3.
- Kent Beck, *Extreme Programming Explained* (1999); Ron Jeffries,
  [practices/pracnotneed](http://ronjeffries.com/xprog/articles/practices/pracnotneed/)
  (1998) — YAGNI origin.
- Rich Hickey, "Simple Made Easy," Strange Loop (2011) — §4.
- Sandi Metz, "The Wrong Abstraction" (2016) — §5.
- Peter Naur, "Programming as Theory Building" (1985) — §7.
- Simon Brown, *Modular Monoliths* and *C4 Model* — §6.
- Bjarne Stroustrup, [quotes](https://www.stroustrup.com/quotes.html) — §9.
- Dan McKinley, *Choose Boring Technology* ([club](http://boringtechnology.club/)) — §8.
- Lehman & Belady, *Laws of Software Evolution* — §10.
- Fred Brooks, "No Silver Bullet" (1986)
  — [PDF](https://worrydream.com/refs/Brooks_1986_-_No_Silver_Bullet.pdf).
- Charity Majors, [charity.wtf](https://charity.wtf/).
- jmoiron, [Notes on A Philosophy of Software Design](https://jmoiron.net/blog/notes-on-philosophy-of-software-design).
