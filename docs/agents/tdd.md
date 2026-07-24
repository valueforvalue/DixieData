# Test-Driven Development — DixieData Discipline

TDD is the **truth anchor** for every slice that lands on this
repo. It does not replace the vertical-slice discipline in
[`feature-protocol.md`](feature-protocol.md); it sits *inside*
it, in the gap between "the slice is well-shaped" and "the
slice ships what we said it would."

The repo already enforces: compile green, lint green,
orphan-handler probe green, doc-coverage floor green, smoke
probes green on its own surface. TDD adds the test that
**pins the slice's user-facing acceptance criterion BEFORE
the slice's code lands**, so the slice can't quietly
regress to "compiles but doesn't do the thing."

This doc is the DixieData port of the `tdd` skill (user-level,
auto-loaded by pi on TDD-related triggers). Read the skill
for the generic red-green-refactor loop; read this doc for
the DixieData-specific anchors, per-layer recipes, and
anti-patterns extracted from the recent `fix:` commits
below.

**Scope of application:** this protocol applies to all new
Tier-2 vertical slices whose slice plan is written **after
the commit that introduces this doc** (commit `260e1f1`,
`docs(agents): TDD discipline anchored in vertical-slice
protocol`, 2026-07-04). Slices that landed before that
commit (e.g. the v60 Event Records slots #322-#338, the
v61 fix for #340) are not retroactively required to comply
— rewriting them to add the RED step would be
drive-by churn, not TDD. The protocol's regression net is
the **forward** slice commits, not the historical ones.

## The DixieData failure modes TDD prevents

Three classes of bug have shipped to `dev` in 2026-07 alone.
Each was caught by an after-the-fact `fix:` commit, not by
the slice that introduced it. Each is preventable by a
single test written *before* the slice lands.

### #4 — Ship-and-claim without live repro (newest, July 2026)

**Symptom:** A slice ships + closes an issue + claims the
bug is fixed. The agent never actually opened a browser,
clicked the reported trigger, or asserted the end state.
The user's screenshot or a Playwright probe proves the fix
addressed a *different* layer of the bug than the one filed.

**Real example:** the `markdown.png` + #607 + #609
sequence. A user reported "Preview button on
/articles/{id}/edit does nothing." An agent shipped an
`internal/templates/soldier_card.templ`-shaped fix (moved
the install into `initializeDynamicContent` + added an
idempotency guard) and closed the issue. The user re-pinged
because the button still did nothing. A live Playwright
probe against `dixiedata-web` revealed the real bug — the
chi mux + the `<head>` lacked any route for `/_lib/*.js`,
so `window.__dixieDebounce` was undefined and the
`initializeArticlePreview` function bailed early *before*
the fix had a chance to do anything. The agent's fix was
correct in isolation, but neither it nor the verification
ever touched the actual repo-root `.dixiedata/` archive.

**Prevention:** `CONTEXT.md` §"No slice ships until a live
probe confirms" + `manual-audit-playbook.md` "Where the
archive lives" codify the rule end-to-end: a slice ships
when a live probe (Playwright OR a binary smoke against
the canonical `<repo-root>/.dixiedata/`) shows the
user-reported trigger now produces the expected end
state. The git comment + CHANGELOG bullet cite the probe
file path so the verification is replayable.

### #1 — Modal invoker wiring (most common)

**Symptom:** UI button calls a JS helper that queries the
DOM for a modal element, gets `null`, and silently
early-returns. The feature is fully implemented server-side
(handler + route + state) but the user sees no feedback.

**Real example:** commit `d5541a7` — Build Share Archive
button + Share Queue pill opened the modal on click. The
modal was never rendered into the page after issue #284 split
`/share` into three subpages; the JS queried for it, got
`null`, and no-op'd.

**RED test that catches it:** `audit/smoke_<feature>.mjs`
that drives the live binary, clicks the button, and asserts
the modal is in the DOM (`document.querySelector('#modal')`
is not `null`) AND has `aria-modal="true"` AND the
`hidden` class is absent. This test fails BEFORE the slice
because the modal doesn't exist. It goes green when the
slice wires the lazy-load (the `d5541a7` fix shape).

### #2 — Adjacent race / state-machine collision

**Symptom:** Slice's own click handler does the right thing,
but a different handler attached to a higher-priority event
(document, window, ancestor element) reacts to the same event
and undoes the work. The slice passes its own smoke probe
because the probe only checks the slice's own outcome.

**Real example:** commit `d8f73b7` — outside-click handler
closed the foldout panel the trigger just opened. The
trigger's bubble-phase click fired first and called `open()`,
then the click bubbled to the document-level handler that
iterated *all* foldouts and closed the one the trigger owned.

**RED test that catches it:** smoke probe that clicks the
trigger, waits 50–200ms (enough for the bubble to land), then
asserts the panel is still visible AND `aria-expanded="true"`.
For DixieData's Wails runtime, the 50ms window is the right
delay — WebView2 event loop is slower than headless Chromium.
This test fails on the first click of a fresh load (matches
the bug report verbatim).

### #3 — Orphan handler / fragment-as-redirect

**Symptom:** A `feat(http):` slice adds a `r.Post("/x", ...)`
handler. The response sets a header (`X-DixieData-Redirect`)
that the JS dispatcher reads and treats as a navigation
target, but the URL is a fragment endpoint that returns raw
HTML. The browser navigates to the fragment URL and renders
raw HTML as a full page.

**Real example:** commit `d3e0a02` — Sources/Tags panels
returned fragment HTML on POST via `X-DixieData-Redirect`,
which `dispatchUtilitySubmit` followed as a navigation. The
orphan-handler probe flagged it after merge.

**RED test that catches it:** handler test in
`internal/appshell/<handler>_test.go` that asserts the POST
response does NOT set `X-DixieData-Redirect` AND the response
body contains the swap-target HTML (the panel fragment).
Plus: the existing `audit/discover_orphan_handlers.mjs` probe
moved into the pre-commit hook (see Wiring §3 below) so it
fires *before* the slice merges, not just in CI.

## The DixieData red-green loop

This is the slice-internal protocol. It runs **inside** one
Tier-2 vertical slice, not across the whole feature.

### Step 0 — Read the slice's acceptance criterion

The slice plan (in the issue body or `.rpiv/artifacts/plans/`)
states "the user can X" or "the user sees Y when Z." That
criterion is what the test will pin. If the slice plan
doesn't have one, the plan is incomplete — go back and add
it. An implementation criterion ("the service exposes method
`Foo(int64)`") is not an acceptance criterion; it's an
implementation step.

### Step 1 — RED: write the failing test first

Write the test that **pins the acceptance criterion.** Do
not write the slice code yet. Do not write other tests yet.

- **Go backend slice:** add a handler test in
  `internal/appshell/<handler>_test.go` (testify table-driven)
  or a service test in `internal/<layer>/<svc>_test.go`. If
  the slice touches `internal/db/`, the test uses a real
  SQLite test DB (the repo already has the `pgregory.net/rapid`
  property-test pattern from commit `c89ed25`; reach for
  it when the slice has interesting input domains).
- **templ slice:** add a render test in
  `internal/templates/<view>_test.go` that calls `Render`
  into a `bytes.Buffer` and asserts the rendered HTML
  contains (or doesn't contain) specific surface IDs from
  `internal/uiids/`. The repo's pattern is `bytes.Buffer` +
  `Render` + `strings.Contains`, not `goquery`.
- **htmx/JS slice:** add (or extend)
  `audit/smoke_<feature>.mjs` that drives the live binary
  via Playwright and asserts the response shape AND the
  post-navigation URL AND the relevant DOM state. The
  response-only assertion is insufficient (that's how the
  htmx `hx-swap="none"` + 303 silent-swallow bug shipped —
  `70878ac` → `3612dab`).
- **Cross-layer slice (Tier 2 vertical):** both kinds of
  test. The handler test pins the server contract; the
  smoke probe pins the invoker wiring (#1) and adjacent
  state (#2). They commit together in the same slice commit.

The test fails for the **right reason.** If the test fails
for an unrelated reason (missing import, test setup
mistake), fix the test, not the code. A passing test that
wasn't actually pinning the criterion is worse than no test.

### Contract touch

TDD is DixieData's executable form of Design by Contract, but it
is not a formal Meyer-style contract system. Tests express and
verify caller obligations, observable guarantees, and invariants;
Go source does not gain a generic `Require`, `Ensure`, or
`Invariant` framework.

Apply a contract touch when a slice:

- adds behavior at a public seam;
- materially changes behavior at an existing public seam; or
- fixes a bug caused by an implicit or violated seam contract.

A public seam includes an exported service method or helper, HTTP
handler, DTO, typed builder, persistence boundary, Wails/native
adapter, and user-visible DOM interaction. Apply the rule to
existing code when the slice materially touches that seam. Do not
start a backlog-wide retrofit or expand a slice into unrelated code.
Formatting, generated output, mechanical renames, and other
behavior-preserving edits do not trigger contract churn.

For each affected seam, document and test whichever dimensions
apply:

- **Preconditions** — what callers must provide or establish.
- **Postconditions** — observable result after success.
- **Failure modes** — typed error, status/header/body, toast, or
  other caller-visible failure behavior.
- **State effects** — what changes, what remains unchanged on
  failure, and whether the operation is atomic.
- **Idempotency and concurrency** — whether retries, duplicate
  actions, or simultaneous calls are safe.

Omit dimensions that do not apply; do not write boilerplate such as
"no side effects" on every pure helper. Exported Go doc comments
state the source-level contract. The RED test proves relevant claims
at the seam. For cross-layer slices, the handler test proves the
server contract and the smoke probe proves the user-visible
postcondition.

Choose the strongest enforcement site that removes invalid states
with the least runtime risk:

1. Go types, enums, and typed builders prevent invalid
   representation.
2. SQLite constraints protect durable data invariants.
3. Service validation and typed errors reject user or caller input.
4. Architecture tests protect package boundaries.
5. Handler, service, property, render, and smoke tests prove
   observable postconditions.
6. Panics protect developer-created impossible states only. Never
   panic for ordinary user input, recoverable I/O, or third-party
   failures.

The contract is complete enough when a caller can answer "what must
I provide, what may change, and what can fail?" and a failing test
identifies which promise broke. More prose or assertions after that
point add noise, not safety.

Example:

```go
// AttachSourceRecord attaches sourceID to personID.
//
// personID and sourceID must identify existing records. On success,
// the attachment is persisted exactly once. Duplicate attachment is
// idempotent. ErrNotFound identifies either missing record; all errors
// leave archive state unchanged.
func (s *Service) AttachSourceRecord(personID, sourceID int64) error
```

Tests then cover success, either missing record, duplicate calls, and
unchanged state after failure. They do not assert private SQL call
order unless that order is itself a documented performance contract.

### Step 2 — GREEN: write the minimum slice code

Implement the smallest diff that satisfies the RED test.
No drive-by refactors. No "while I'm here" cleanup. No
speculative surface.

- `go test -count=1 -short ./internal/<layer>/...` — green
- `node audit/smoke_<feature>.mjs` — green
- `just test` — green across the whole short suite

If a slice makes another test in the same package turn red,
**stop.** The slice is touching code outside its seam.
Either the seam is wrong (and the slice needs to escalate
back to Plan) or the test you broke was pinning adjacent
behavior — and now is the time to fix it, not to "make the
test more lenient."

### Step 3 — Adjacent behavior sweep (the TDD-specific step)

Before the commit, run the smoke probes and handler tests
for **adjacent surfaces** — anything in the same screen
family, anything that shares the JS dispatcher, anything
that the slice's render touches.

The screen family is keyed off `docs/ui-map/INDEX.md` —
look up the slice's screen row and pick the 2-4 adjacent
rows (same screen family, sibling tabs, screens that share
the same JS dispatcher like `dispatchUtilitySubmit`,
screens that share the same handler cluster). Then run
the smoke probes + handler tests for those rows only.
Don't blanket-run every probe (slow) — the slice's
`feature-protocol.md` checklist + uiids registry should
give you the exact 2-4 probe names. **Caveat:** as of
this doc, `docs/ui-map/INDEX.md` is stale for the v60 Event
Records surface + the Share foldout + the floating-dock
Menu (issue #342). For surfaces not yet in INDEX.md, fall
back to grep'ing the slice's templ + handler files for
shared `data-dixie-submit` / `dispatchUtilitySubmit`
callers and pick the smoke probes that touch those.

Concretely:

```bash
# Run the named 2-4 smoke probes for the slice's screen
# family (NOT a blanket ls | xargs — that runs every probe
# and is too slow to be the per-slice gate).
node audit/smoke_<feature_a>.mjs
node audit/smoke_<feature_b>.mjs

# Run every test in the slice's package + sibling packages
# that share a service or handler.
go test -count=1 -short ./internal/<slice-pkg>/...
go test -count=1 -short ./internal/<sibling-pkg>/...
```

If any turn red, the slice has crossed a seam it shouldn't
have. **Fix or escalate; do not silence.** A common
shortcut to avoid here is `// nolint` / `t.Skip` / deleting
the adjacent test — those are exactly the moves that
produced the three bug classes above.

### Step 4 — REFACTOR (only after green)

Existing rule from `feature-protocol.md`. Refactor candidates:

- Extract duplication the slice revealed (e.g. two
  near-identical handler tests → table-driven)
- Deepen modules (move new logic behind a service seam)
- Two-adapter check before adding any new public method

**Never refactor while RED.** Get to GREEN first.

### Step 5 — Commit the RED test + GREEN slice together

The RED test lives in the same commit as the slice code it
pins. Inline, atomic, reviewable. Standard Go convention;
matches the repo's "one commit = one reviewable unit" rule
from `AGENTS.md`.

## What's the seam?

Per `feature-protocol.md` §"Module discipline", the seam
is the **service interface + DTO**. Tests cross the seam.
The persistence layer (`internal/db/`) is private to the
service; the test does not import it.

For slices that don't add a service (pure templ + JS), the
seam is the **DOM surface ID** from `internal/uiids/`. The
templ test asserts the surface ID is present (or absent)
in the rendered HTML; the smoke probe asserts the same ID
is reachable from a click.

If a test needs to mock the service, the test is testing
the wrong thing. The service is the seam; mocking it
hides the failure mode (#1, d5541a7) where the JS calls a
real method that quietly early-returns.

Exception: when the service itself depends on a hard-to-
fake boundary (Wails runtime, native dialog, WebView2),
mock the boundary, not the service. The
`appshell_test.go` files already do this with the
`fakeWailsRuntime` pattern; reach for it but don't
over-mock.

## Per-layer recipes

### Go backend (handler + service)

- **Handler test:** `internal/appshell/<handler>_test.go`,
  testify suite + `appTestHarness` (already in the repo).
  Assert status code, response shape, headers (especially
  `X-DixieData-Redirect` — if it's set, assert where it
  points and what the URL renders to).
- **Service test:** `internal/<layer>/<svc>_test.go`, real
  SQLite test DB (the `internal/db/testdb.go` helper).
  No mocks. The repo uses `pgregory.net/rapid` for property
  tests when the input domain is interesting.
- **Migration test:** if the slice ships a schema change,
  the slice's RED step extends the existing reversibility
  catalogue test in `internal/db/migrations_test.go`: add
  the new block ID to the `want` map (with its
  `Reversibility` classification) and bump the
  `Migrations() length` expectation. The map-driven test
  fails for the slice's RED step because the new block
  isn't in the map yet; the GREEN step adds it. Downgrade
  tests in `internal/db/migrate_down_test.go` may also
  need the boundary updated if the new block is
  Reversible/PartiallyReversible (the test asserts which
  targets the applyDownSchema refuses). A separate
  fresh-DB applySchema test (`TestApplySchemaF*`) does
  not exist in the repo as of this doc — a future slice
  could add one as a `pgregory.net/rapid`-shaped
  characterization test, but it's not required by this
  protocol today.
- **Per-iter SQL footprint, when the test has a perf budget.**
  Any test that asserts a wall-clock budget on a service
  call MUST document the actual per-iter SQL footprint in
  the test's doc-comment: count the SELECTs, INSERTs,
  UPDATEs, DELETEs, transactions, and any Go-side work
  (UUID mint, marshal, fsync) that the call performs. The
  budget must be sized against the slowest supported runner
  (currently Windows CI with SQLite fsync + disk-pressure
  overhead), not the dev box. The pre-#420 budget on
  `TestStressEventAttachDetachRoundTrip` was 5ms based on a
  "1 INSERT + 1 DELETE" assumption that didn't match the
  real per-iter footprint (5 SQL ops + 2 fsync-flushed
  statements); the resulting 5880us/iter regression on CI
  went unflagged for 24h because the budget was tuned to
  the dev box. The correct shape is "real footprint in
  comments, budget at slowest-runner observation + 70%
  headroom, the test is a regression detector not a perf
  SLA." `git log --grep='per-iter SQL footprint'` finds the
  doc-comment pattern future tests should mirror.

### templ

- **Render test:** `internal/templates/<view>_test.go`,
  `bytes.Buffer` + `Render` + `strings.Contains`. Assert
  the surface IDs the slice adds (and assert the ones it
  removes are gone — that's how `TestShareLandingHasNoModals`
  pinned the #284 subpage split).
- **htmx attribute test:** the repo's `htmxattr` package
  validates targets against the `internal/uiids` registry.
  If the slice introduces a new `hx-target` value, the
  validator fails fast in dev builds. The slice's RED
  step is to add the target to the registry + a
  validator-passing test.

### htmx + frontend JS

- **Smoke probe:** `audit/smoke_<feature>.mjs`, Playwright
  driving the live binary (`dixiedata-web.exe` on the
  scratch dir). Assert response shape AND `page.url()` AND
  the relevant DOM state. See existing
  `audit/smoke_share_queue_page.mjs` for the shape (5
  state transitions covered in one probe — the slice plan
  tells you which transitions to cover).
- **JS unit test:** only when the slice adds a pure-JS
  helper with no DOM dependency. Reach for Vitest only
  if the slice already adds Vitest infrastructure; don't
  add it for one helper.

### Audit probe changes

If the slice adds or modifies a `discover_*.mjs` probe,
the RED step is to run the probe against the slice's
expected pre-state and confirm it produces the right
output (the orphan, the lint failure, the contrast drift).
The GREEN step is the slice's fix; the probe should now
pass.

## Anti-patterns

The `tdd` skill calls these out generically; the DixieData-
specific instances are below.

- **Horizontal slicing.** Writing all the tests first
  (the slice plan's full test list), then all the
  implementation. This is the AI-slop failure mode the
  skill is built to prevent. DixieData version: an LLM
  agent reads the slice plan, writes 8 tests across 4
  files, then writes the 8 implementations in order. Half
  the tests pass for the wrong reason; the slice ships.
  **Right move:** one test → one impl → repeat, even
  within a single slice commit.
- **Test-after-the-slice-is-done.** The slice lands, then
  a regression test is added "to make sure it doesn't
  break." This is the existing regression-net discipline.
  It catches *future* regressions but not the slice's own
  drift. TDD adds the test that pins the slice's
  acceptance criterion *before* the slice lands. The two
  are complementary, not interchangeable.
- **Mocking the seam.** Writing a test that mocks the
  service to assert "the handler called `Foo(int64)` with
  X." This pins the implementation, not the behavior. The
  slice can satisfy the test without satisfying the
  acceptance criterion (#1 above). The handler test should
  hit a real (test-DB-backed) service and assert the
  observable outcome (status, headers, body shape).
- **Pinning the response shape but not the post-click
  URL.** The htmx silent-swallow bug (`70878ac` →
  `3612dab`) shipped because every smoke probe at the time
  only asserted the response. Smoke probes MUST assert
  `page.url()` after every click that expects navigation.
  This is in `feature-protocol.md` already; the TDD layer
  is "write the URL assertion FIRST, so the slice can't
  skip it."
- **Speculative tests.** Writing a test for behavior the
  acceptance criterion doesn't mention. "While I'm writing
  the test, let me also assert that the modal is keyboard-
  navigable" — only if the criterion says it should be. If
  it doesn't, the test is YAGNI and a future maintenance
  burden.

## Wiring

The protocol is anchored in three small edits to the
existing agent docs:

1. **`docs/agents/feature-protocol.md` §Pre-flight checklist**
   — add "TDD anchor" as the 6th checklist item, pointing
   to this doc. Forces the agent to read this doc before
   recon.
2. **`docs/agents/rpci.md` §I — Implement** — between step 1
   (read the files) and step 2 (make the change), insert
   "step 1.5: write the RED test for the slice's acceptance
   criterion." Per `rpci.md`'s autonomous clause, when the
   user replies "all recommended" to the Plan or Critique
   decisions, this becomes an active implementation step.
3. **`docs/agents/INDEX.md`** — add `tdd.md` to Tier 0
   (cross-cuts every feature work, every bug fix).

### Failure-mode-#3 probe status (as of this commit)

The orphan-handler probe that catches failure mode #3 is
**not yet wired to a pre-commit hook or CI step.** As of
this doc's commit it runs **manually** per `AGENTS.md`'s
"Before pushing" checklist (`node
audit/discover_orphan_handlers.mjs`), exits 0 even when
orphans are found (informational mode), and produces an
~80-line list that the agent reads rather than a binary
pass / fail. The probe's value is the audit signal — both
#340 and #341 in this audit round were caught because the
probe's output read \"GET /events/{id}/sources\" /
\"GET /events/{id}/tags\" as orphan handlers, which the
agent then connected to the X-DixieData-Redirect
fragment-as-page nav failure mode.

A future slice should move the probe into pre-commit (or a
Husky hook if the repo adopts one) and exit non-zero on
orphan growth. Until that lands, the manual run is the
gate; agents must read the output rather than rely on an
exit code.

## When NOT to TDD

- **One-line bug fix with clear repro.** Bug protocol in
  `issue-tracker.md` applies: patch + regression test +
  commit. The regression test IS the TDD step, but the
  protocol overhead is unnecessary.
- **Pure doc change.** No code, no test.
- **Pure build/CI change.** Tests for build behavior are
  brittle and slow; the existing `just freshness` gate is
  the anchor.
- **Refactor with characterization tests.** When the slice
  preserves behavior but rearranges code (per `feature-
  protocol.md` §3-tier commit rule Tier 2 verticals without
  a user-visible surface), the RED step is a characterization
  test — pin the existing behavior before the refactor.
  This is the same loop; the criterion is "behavior
  unchanged," not "new capability added."

## References

- `tdd` skill (user-level, auto-loaded) — generic
  red-green-refactor protocol (read first)
- [`feature-protocol.md`](feature-protocol.md) — slice
  discipline this protocol sits inside
- [`rpci.md`](rpci.md) §I — Implement phase wiring
- [`COMMON_BUGS.md`](../COMMON_BUGS.md) — per-layer bug
  patterns (read alongside the per-layer recipes above)
- Commit `d5541a7` — modal invoker wiring failure mode #1
- Commit `d8f73b7` — adjacent race failure mode #2
- Commit `d3e0a02` — orphan handler / fragment-as-redirect
  failure mode #3
- Commit `c89ed25` — `pgregory.net/rapid` property-test
  pattern
- Commit `4682557` — `goleak` leak detector pattern
- `audit/smoke_share_queue_page.mjs` — the canonical smoke
  probe shape (5 state transitions in one probe)