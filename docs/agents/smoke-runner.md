# smoke-runner conventions

This doc describes the shared smoke runner shape used by the
DixieData audit harness. The runner is the executor; this doc
is the spec. Keep them in sync.

## Why this exists

Issue #700 ships a single entry point for the post-#700 smoke
probes (Playwright + static scanners) so the audit workflow
gets one PASS/FAIL signal instead of N. The runner lives at
`audit/_lib/smoke_runner.mjs`; the aggregator at
`audit/smoke_aggregator.mjs`; the canonical surface registry
at `audit/_lib/smoke_index.mjs::SURFACES[]`.

The pre-#700 world had one probe file per surface, each with
its own pass/fail accumulator, its own server-spawn path, and
its own exit code. The runner collapses all of that into a
single contract that every probe honors. Adding a new probe is
"append one line to SURFACES[]" — the runner does the rest.

## The runner contract

The runner exports a single function:

```js
import { runProbe } from './_lib/smoke_runner.mjs';

const result = await runProbe({
  name: 'my-probe',
  probeFn: async (ctx) => {
    // ctx = { page, base, scratchDir, record, registerCleanup }
    ctx.record('step-1', true, { detail: 'optional' });
    return { ok: true };
  },
});

process.exit(result.ok ? 0 : 1);
```

The contract:

- `name` — human-readable label. Becomes the JSON key in
  `audit/smoke_summary.json`.
- `probeFn(ctx)` — the probe body. Receives `ctx`, returns
  `{ok: true|false}` or throws. Exceptions are caught by the
  runner and surfaced as `{ok: false, error}`.
- `ctx.page` — Playwright `Page` handle. Slice 2 ships a stub
  (`{}`); the migrated probes today do their own
  `chromium.launch()` because each probe owns its own server
  lifecycle. Future slices may hoist the launch into the
  runner.
- `ctx.base` — `http://127.0.0.1:8080` by default, overridable
  via `SMOKE_BASE_URL` env. When the probe spawns its own
  server, it points at a different port. When the probe runs
  against an externally-started server (`BASE_URL` env),
  `ctx.base` is the canonical value.
- `ctx.scratchDir` — per-probe `mkdtempSync` directory under
  `os.tmpdir()`. The runner keeps it around after `probeFn`
  returns so the probe can inspect state on failure; clean
  it up via `ctx.registerCleanup(() => fs.rmSync(scratchDir, { recursive: true, force: true }))`.
- `ctx.record(name, ok, details)` — bridge into the shared
  reporter (`audit/_lib/smoke_reporter.mjs`). Every assertion
  flows through this. The probeFn's local `pass`/`fail`
  counters (when present) feed the runner's `{ok}` return.
- `ctx.registerCleanup(fn)` — registers a `fn` that runs after
  `probeFn` returns (success OR failure). LIFO order,
  best-effort. Used for spawned-server cleanup, scratchDir
  teardown, etc.

## How to add a probe

There are two surfaces: Playwright (interactive browser
assertion) and scanner (static-source scan). The contribution
contract is the same for both — append one entry to the
`SURFACES[]` array at `audit/_lib/smoke_index.mjs`.

### Playwright probe

Write a probe file that wraps the boilerplate around a
`runProbe({name, probeFn})` call:

```js
// audit/smoke_my_button.mjs
import { runProbe } from './_lib/smoke_runner.mjs';

async function main(ctx) {
  const { page } = await import('playwright');
  let pass = 0, fail = 0;
  const record = (n, ok, d = {}) => {
    if (ok) pass++; else fail++;
    ctx.record(n, ok, d);
  };

  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage();
    await page.goto(ctx.base + '/my-route');
    record('my-button-exists', await page.locator('button[id="my-button"]').count() > 0);
  } finally {
    await browser.close();
  }
  return { ok: fail === 0 };
}

const result = await runProbe({ name: 'my-button', probeFn: main });
process.exit(result.ok ? 0 : 1);
```

Then add the entry to `SURFACES[]`:

```js
{
  name: 'my-button',
  file: 'audit/smoke_my_button.mjs',
  kind: 'playwright',
  class: 4, // 9-class button-bug catalog from #681
  note: 'issue #NN -- short description',
},
```

### Scanner entry

A scanner reuses an existing CLI linter that already has the
`--strict` exit-code contract (`audit/lint_*.mjs` +
`scripts/lint-*.mjs`). The aggregator spawns it for you:

```js
{
  name: 'scanner-my-lint',
  file: 'audit/lint_my_lint.mjs',
  kind: 'scanner',
  class: 6,
  note: 'issue #NN -- short description',
},
```

The class number maps to the 9-class button-bug catalog from
issue #681. See `docs/CODE_CHANGES.md` for the canonical class
assignment per bug class.

### Verifying the addition

```sh
# Run the probe in isolation
node audit/smoke_my_button.mjs

# Run the full aggregator
just test-smoke

# Run the strict mode (CI posture)
just test-smoke-strict
```

All three should exit `0` once the probe is green. CI
(`.github/workflows/audit.yml`) runs `just test-smoke-strict`
after the audit-harness step; the upload-artifact step ships
`audit/smoke_summary.json` alongside the audit reports.

## The surface registry

Today the `SURFACES[]` array lists the registered surfaces
(see `audit/_lib/smoke_index.mjs` for the canonical list --
the table here is illustrative, not authoritative):

| name | kind | class | issue |
|---|---|---|---|
| `soldier-images` | playwright | 4 | #392 |
| `submit-e2e` | playwright | 1 | #618 |
| `mega-menu-nav` | playwright | 4 | #380 |
| `scanner-nested-forms` | scanner | 4 | #682 |
| `scanner-invoker-resolve` | scanner | 6 | #687 |
| `scanner-init-guards` | scanner | 9 | #685 |
| `scanner-orphan-handlers` | scanner | 2 | discover_orphan_handlers |

The `kind` field is `'playwright'` for runtime regression
probes (Playwright drives a real browser against a real
server) or `'scanner'` for static-source probes (the
aggregator spawns the scanner binary with `--strict` and
records the exit code). A future CI annotation tool uses
`kind` to decide whether a failure is a runtime regression
or a static-source regression -- different teams may own
each, with different triage paths.

The `class` field is the 9-class button-bug catalog from
issue #681. The mapping per probe is set when the probe is
added; see `docs/CODE_CHANGES.md` for the canonical class
assignment per bug class. The aggregator surfaces `class`
in the JSON summary so a future PR-comment poster can group
failures by class (e.g. "3 class-4 nested-form failures
this run").

**Adding a probe is append one line.** Removing a probe (e.g.
when a screen is retired) is delete one line. Renaming a probe
is edit the `name` field — the JSON key in
`audit/smoke_summary.json` follows the `name`. Renaming
without changing the issue is a smell; rename only when the
issue number also changes.

## The JSON summary schema

`audit/smoke_summary.json` is the machine-readable artifact
that the CI workflow uploads after `just test-smoke-strict`
(see `.github/workflows/audit.yml`). The shape (issue #703):

```jsonc
{
  "schemaVersion": 1,            // smoke_summary.json shape contract
  "runnerVersion": 1,            // runProbe() shape contract (from smoke_runner.mjs::RUNNER_VERSION)
  "finishedAt": "2026-07-31T...", // ISO timestamp of writeJson() call
  "results": [                   // one entry per SURFACE, in registration order
    {
      "name": "soldier-images",  // mirrors SURFACES[].name
      "ok": true,                // exit-0 = pass, non-0 = fail
      "kind": "playwright",      // mirrors SURFACES[].kind ('playwright' | 'scanner')
      "class": 4,                // mirrors SURFACES[].class (9-class button-bug catalog)
      "runnerVersion": 1,        // stamped from smoke_runner.mjs::RUNNER_VERSION on every record()
      "ts": "2026-07-31T...",    // ISO timestamp of record() call
      "...details": "..."         // probe-specific (lastResponses, bodySnippet, etc.)
    }
  ]
}
```

`schemaVersion` is the smoke_summary.json shape itself
(bumped when the JSON structure changes). `runnerVersion`
is the `runProbe({name, probeFn, ctx})` contract (bumped
when the ctx shape, return shape, or record() payload
changes). Both are pinned by `audit/_lib/smoke_runner.test.mjs`
so a future breaking change fails the test at PR time, not
in production.

## Lifecycle: spawn, register cleanup, exit

The runner's hardest-to-get-right bit is the cleanup hook
ordering. The pattern every migrated probe follows:

```js
// 1. If you spawn a server (or any child process), capture
//    the handle and register cleanup BEFORE doing any work.
const server = spawn(WEB_BIN, [...], { stdio: ['ignore', 'pipe', 'pipe'] });
server.stderr.on('data', () => {});
ctx.registerCleanup(() => { try { server.kill(); } catch (_) {} });

// 2. If you create temp files, register cleanup for those too.
ctx.registerCleanup(() => { try { fs.rmSync(tempDir, { recursive: true, force: true }); } catch (_) {} });

// 3. Do your work. Call ctx.record() for every assertion.

// 4. Return { ok: true|false }. The runner fires cleanups in
//    reverse-registration order (LIFO), best-effort.
```

The pre-migration probes had a structural leak: `let server;`
was declared but never assigned, so the spawned WEB_BIN
process was orphaned on Windows when local-dev mode
auto-spawned against `BASE_URL=''`. The runner fixes this by
making the cleanup hook mandatory — `ctx.registerCleanup` is
the canonical place to kill the spawned process. All migrated
probes follow this pattern; do not change it back to a
try/finally `process.exit` — the runner's cleanup hooks are
the contract.

## Invariants (do not violate)

1. **Every Playwright probe today spawns its OWN server.** The
   runner's `ctx.page = {}` is a stub; the migration plan
   deliberately stopped short of wiring `chromium.launch()`
   into the runner until the second probe migrates and the
   contract holds for >1 caller. Slice 5+ may revisit;
   individual probes do NOT change their lifecycle except
   when the slice is explicitly about that.

2. **Migrated probes preserve their step assertions
   byte-for-byte.** The runner wraps the probeFn; the body
   is unchanged. The migration is plumbing, not a refactor.

3. **All `--strict` flags are honored by the consumer.** The
   aggregator's `STRICT` flag threads through to scanners
   that already accept `--strict` (the 4 linters). Adding a
   scanner that uses a different flag requires extending the
   pass-through in `dispatchSurface()`.

4. **`audit/smoke_summary.json` is gitignored.** The
   aggregator writes it on every run; CI ships it as a
   workflow artifact only. Local commits should never include
   it.

5. **The smoke-runner does NOT replace the existing
   `audit/smoke_*.mjs` per-probe lint recipes.** The 4
   `audit/smoke_dialog_guard.mjs` /
   `audit/smoke_microcopy.mjs` /
   `audit/smoke_pdf_microcopy.mjs` /
   `audit/smoke_icalendar_microcopy.mjs` probes stay parallel
   to the new `test-smoke` triplet; they exercise the older
   per-probe lint-X / lint-X-strict / lint-X-test recipe
   shapes. Do not consolidate them.

## Running the runner

```sh
# Local-dev (spawns own server, exits 1 for known-failing probes)
just test-smoke

# CI mode (assumes server on :8080, strict scanners)
just test-smoke-strict

# Unit tests for the runner itself
just test-smoke-test
```

The aggregator's `STRICT` flag is forwarded to the scanner
entries. Playwright probes do not consume `--strict` (their
pass/fail is internal to the probeFn).

## Probe source

- `audit/_lib/smoke_runner.mjs` — the `runProbe` contract.
- `audit/_lib/smoke_reporter.mjs` — `record()/renderSummary()/writeJson()`.
- `audit/_lib/smoke_paths.mjs` — `webBin()/seedBin()` cross-platform helpers.
- `audit/_lib/smoke_index.mjs` — `SURFACES[]` registry.
- `audit/smoke_aggregator.mjs` — CLI entry point.
- `audit/_lib/smoke_runner.test.mjs` — 10-assertion unit test.

## Related

- Issue #700 — the umbrella issue tracking the slice-by-slice
  rollout.
- `.rpiv/artifacts/plans/2026-08-01_button-matrix-a11y-probes.md` — the
  matrix + a11y probe rollout plan (slices 1, 2, 3a, 3b, 4).
- `audit/smoke_button_matrix.mjs` — slice 1+2 GREEN stub expanded to
  the real per-button 5-state matrix probe.
- `audit/smoke_a11y.mjs` — slice 3a WARN-only WCAG 2 AA sweep via
  `audit/harness.mjs::runAxe()`.
- `.github/workflows/audit.yml` — CI integration (slice 6) +
  `timeout-minutes: 30` raise for the matrix + a11y probe budgets
  (slice 4, Q5 option A).
- `justfile` — `test-smoke` / `test-smoke-strict` / `test-smoke-test` recipes.
- `CHANGELOG.md` — per-slice landing notes (slice 1-6 entries).
- `docs/agents/INDEX.md` — Tier 1 cross-reference.
- `docs/CODE_CHANGES.md` — the 9-class button-bug catalog.
- `MISTAKES.md` — the git-show-vs-read staleness lesson that
  cost ~25 minutes during slice 2 (when extracting a code
  block for surgical migration, use `git show HEAD:path` not
  the `read` tool).

## The matrix probe (slice 1 + 2)

`audit/smoke_button_matrix.mjs` walks the 28 non-detail surfaces
in `SURFACES[]` and asserts a 5-state matrix per `button`,
`[role="button"]`, and `a[href]`. The 10-step algorithm (per the
plan, Q1):

1. **Spawn the server** against a per-probe `ctx.scratchDir`,
   seed via `cmd/seed-data --reset --soldiers 5 --articles 2
   --events 2 --tags 5` (per Q6).
2. **Visit the surface** with `page.goto(targetUrl, {waitUntil:
   'domcontentloaded'})`. Surface URLs are derived from the 28-
   surface `SURFACE_URLS` array (mirrors `SURFACES[]` minus the
   5 detail surfaces that dedicated per-feature probes cover).
3. **Pre-expand `<details>`** so buttons inside collapsed panels
   are visible to the matrix walker.
4. **Dismiss any auto-opened modals** (feedback-modal, print-
   config-modal, google-calendar-preferences-modal) so they
   don't intercept subsequent clicks.
5. **Install a `MutationObserver`** on `document.body` (one
   observer per surface, reset before every click) that
   increments `window.__matrixMutationCount` on every mutation
   the observer fires (childList + subtree + attributes).
6. **Enumerate clickables** in source order: `button,
   [role="button"]` first, then `a[href]`. Cap at 50 per surface
   (buttons prioritized; anchor budget backfilled from the
   top of the list). This is the 5-state matrix's "what
   buttons exist" step.
7. **For each clickable**, snapshot the pre-click uiids set
   (every `[id^="page."], [id^="panel."], [id^="tab."]` id).
   Assert `isVisible()` (per-element `cs.display !==
   'none'` + `cs.visibility !== 'hidden'` + non-zero
   `getBoundingClientRect()`) and `isEnabled()` (skip when
   the button is inside `<fieldset disabled>`). For anchors
   only, also check `inClosedMegaMenu` (skip if inside
   `[data-mega-menu-panel].hidden`) and `selfAnchor` (click
   on a self-link `href === currentUrl` is a PASS by design).
8. **Focus + click** the element. Anchor clicks get a 3s
   poll loop reading `location.href` every 150ms (replaces
   the 100ms-after-click URL read in slice 2's first cut;
   the original `Promise.all([page.waitForURL, click])` race
   missed real navigations on `/insights/drilldown` anchors).
9. **Assert click did something**: anchors expect URL change
   (or self-anchor pass); buttons expect MutationObserver
   delta OR URL change. The observer's mutation count and
   the post-click `location.href` are batched into one
   `page.evaluate()` round-trip for speed.
10. **Re-snapshot uiids** (the silent-DOM-wipe assertion,
    same shape as issue #691). Post-click set must be a
    superset of pre-click set; any vanished id is a FAIL
    with the missing ids in the detail.

**5-state matrix per plan Q1:**
- `isVisible` — assertion 1, skip otherwise.
- `isEnabled` — assertion 2, skip when inside `<fieldset
  disabled>`.
- `focus + document.activeElement === el` — assertion 3,
  soft-warn on foldout-pattern focus stealing.
- `click → mutation OR URL change OR x-dixiedata-submit
  response` within the click-poll window — assertion 4.
- `post-click uiids ⊇ pre-click uiids` — assertion 5
  (silent-DOM-wipe class).

**4 defensive filters** prevent the probe from triggering
real-world side effects:
- External `href` (`https?://`, `mailto:`, `//`) skipped — no
  GitHub commit links visited (the `/about` page links to
  `github.com/valueforvalue/DixieData/commit/<hash>`).
- `NATIVE_DIALOG_PREFIXES` (`/import/backup`,
  `/import/shared-archive`, `/import/memorial-json`) skipped
  — native `OpenFileDialog` would block headless Chromium.
- `/integrations/google/*` skipped — `google_service.go::Connect()`
  calls `pkg/browser::OpenURL(authURL)` to `accounts.google.com`,
  which pops the developer's real Chrome on every probe click.
- Modals auto-hidden at the start of every surface visit so
  they don't intercept subsequent clicks.

**Deterministic rotation (per Q5):** `SMOKE_ROTATION` env:
`ci` (default) = day-of-epoch mod 4, take 7 surfaces
(28 ÷ 7 = 4-day full-coverage cycle); `local` = first 7;
`full` = all 28.

**Default mode:** GREEN. 1,143 PASS / 4 candidate FAIL per
7-surface run (~0.4%). The 4 slice-2 candidate FAILs (the
2 `/insights/drilldown?scope=...` anchors + the 2
mega-menu `/insights` / `/about` anchors) were triaged in
real headed Chromium and confirmed as probe artifacts. The
probe-side fixes (closed-mega-menu skip + self-anchor pass
+ 3s nav poll) land in `9a8dd494` and eliminate all 4.

**Strict-mode toggle:** `SMOKE_BUTTON_MATRIX_STRICT=1` flips
to FAIL-mode + exit 1 (preserved from slice 1 for slice 2's
gate-flip).

## The a11y probe (slice 3a)

`audit/smoke_a11y.mjs` is the WARN-only counterpart to the
matrix probe. Walks the same 28 non-detail surfaces,
reuses `audit/harness.mjs::runAxe()` (which handles WCAG
2 AA + fragment detection in one call). Per-surface
`ctx.record('a11y:<name>', ok, { violations: [...] })`
emits axe findings into the JSON summary.

**Default mode is always GREEN.** The probe is
informational, not a gate. Each per-surface record is
emitted with `ok: true` regardless of violation count; the
`warn: true` flag tells the aggregator this surface had
a11y findings worth surfacing. The JSON summary's
per-surface `violations` array carries the structured form
(per-rule id + impact + node count + sample target).

**Strict-mode toggle:** `SMOKE_A11Y_STRICT=1` is wired in
but currently no-ops (the `hasCritical` check in
`smoke_a11y.mjs::main()` is hardcoded to `false`).
This is the slice 3b invariant: the hardcoded `false` is
the one-line change that flips the probe to a real gate
once the issue cohort from slice 3a's first run is
remediated (current cohort: issue #716, the top-nav
color-contrast family).

**First-run cohort (the slice 3a deliverable):** 1 violation
family — `color-contrast` (serious) — across 26 of 28
non-detail surfaces. Affected region: top-nav
(`.top-brand-title` brand title + nav links +
`.primary-button.top-nav-primary` CTA + breadcrumb
separators). All fail WCAG 2 AA contrast against the
dark-navy top-nav background (`rgba(31,43,56,0.92)`).
The 2 fragments (`/jobs`, `/research-log`) are skipped by
`runAxe`'s fragment detection.

**One tracking issue per family** (per plan Q4): issue
#716 covers the entire top-nav. When the fix lands
(lighten the top-nav text tokens — sepia 141,116,64 →
≥180,150,90 — until the 4.5 ratio threshold is met),
the probe's WARN count drops to 0 and slice 3b can
gate.

**Surfaces covered:** same 28 non-detail surfaces as the
matrix probe (per plan Q1; detail-page a11y is covered by
the per-feature detail probes). The probe uses the
8-surface day-of-epoch mod 4 rotation (Q5), not the
7-surface matrix rotation — 28 surfaces ÷ 8 = 3.5-day
cycle (the last surface of the 4th day is the 7-surface
+ 1 leftover; the next 4-day cycle starts at surface 0
with 1 day of overlap on the 4th day's leftover).

**Why this probe shape:** the plan splits a11y into
3a (WARN-only, file the issues) and 3b (gate). Shipping
the WARN-only probe first surfaces the actual violation
list — the threshold (`serious`+`critical`) was
predicated on assumption; the data shows the threshold
catches the top-nav contrast family. Slice 3b is a
one-line change after the issue is fixed.
