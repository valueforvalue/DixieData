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
- `.github/workflows/audit.yml` — CI integration (slice 6).
- `justfile` — `test-smoke` / `test-smoke-strict` / `test-smoke-test` recipes.
- `CHANGELOG.md` — per-slice landing notes (slice 1-6 entries).
- `docs/agents/INDEX.md` — Tier 1 cross-reference.
- `docs/CODE_CHANGES.md` — the 9-class button-bug catalog.
- `MISTAKES.md` — the git-show-vs-read staleness lesson that
  cost ~25 minutes during slice 2 (when extracting a code
  block for surgical migration, use `git show HEAD:path` not
  the `read` tool).
