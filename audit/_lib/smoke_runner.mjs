// audit/_lib/smoke_runner.mjs (issue #700, ADR 0011)
//
// Shared Playwright smoke-test runner for the DixieData
// audit harness. One chromium.launch() per runner-process,
// one page per probe (a fresh context isolates cookies /
// storage / fixtures). The aggregator at
// audit/smoke_aggregator.mjs walks
// audit/_lib/smoke_index.mjs::SURFACES[] and dispatches
// each entry to either a Playwright probe fn (this file's
// runProbe) OR a static-scanner sub-process (the four
// class-2/4/6/8/9 linters that already exist as standalone
// CLIs at audit/lint_*.mjs + scripts/lint-*.mjs).
//
// Slices:
//   1. Skeleton + aggregator stub + this module's no-op
//      runProbe. The aggregator prints "0 passed, 0 failed"
//      and exits 0.
//   2. MIGRATED: runProbe now accepts a probeFn contract,
//      creates a per-probe scratchDir (mkdtempSync), invokes
//      the probeFn with {page: stub, base, scratchDir,
//      record, registerCleanup}, catches exceptions, runs
//      per-probe cleanup hooks. The first migrated probe is
//      audit/smoke_soldier_images.mjs (5 step assertions,
//      shared 746 LoC, ~330 LoC after migration). The
//      chromium.launch() + page creation is NOT done by the
//      runner yet -- that lands in slice 3 once the second
//      probe (smoke_submit_e2e.mjs) migrates and we know the
//      contract holds for >1 caller.
//
// Why this is runner-only and not @playwright/test:
//   The repo already uses `node --test` + raw `playwright`.
//   Adding @playwright/test would inherit its runner, its
//   reporter, and its discovery model. The user explicitly
//   asked for "shared runner, no more duplication" -- this
//   module honours that with ~200 LoC of plain ESM.

import { chromium } from 'playwright';
import { mkdtempSync, mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

let browser = null;
const contexts = [];

async function start({ headless = true, viewport = { width: 1280, height: 800 } } = {}) {
  if (browser) return browser;
  browser = await chromium.launch({ headless });
  return browser;
}

async function stop() {
  for (const ctx of contexts) {
    try { await ctx.close(); } catch (_) { /* best effort */ }
  }
  contexts.length = 0;
  if (browser) {
    try { await browser.close(); } catch (_) { /* best effort */ }
    browser = null;
  }
}

async function newPageFor(opts = {}) {
  if (!browser) await start(opts);
  const ctx = await browser.newContext({
    viewport: opts.viewport || { width: 1280, height: 800 },
    acceptDownloads: true,
  });
  contexts.push(ctx);
  const page = await ctx.newPage();
  page.on('dialog', (d) => d.accept().catch(() => {}));
  return page;
}

// PROBE_SHAPE documents the ctx contract a probeFn receives.
// JSDoc is the only documentation; the unit test
// (smoke_runner.test.mjs) pins the names so any drift
// breaks at PR time, not in production.
//
// `page` is an opaque handle. Today it's an empty object
// (`{}`) because slice 2 doesn't launch chromium -- the
// migrated smoke_soldier_images.mjs only needs the smoke-
// runner contract and runs in-process; the binary under
// test is what produces the user-facing behavior. Slice 3
// makes `page` a real Playwright Page when we migrate
// smoke_submit_e2e.mjs and want to drive the live binary.
//
// `record(name, ok, details)` flows results into the shared
// reporter (`audit/_lib/smoke_reporter.mjs`). The probe fn
// calls it once per assertion; the reporter writes both a
// console summary and `audit/smoke_summary.json`.
//
// `registerCleanup(fn)` registers a fn that runs after the
// probeFn returns (success OR failure). Used for per-probe
// scratchDir cleanup, per-probe process teardown, etc.
export const PROBE_SHAPE = {};

// RUNNER_VERSION is the audit/_lib/smoke_runner.mjs schema
// contract version. The reporter stamps every record() call
// + the JSON summary's top-level payload with this value
// so a future CI annotation tool can detect a breaking
// change to the runProbe({name, probeFn, ctx}) contract
// without parsing the JSON. Bump this when:
//   - the ctx shape changes (new field added, existing
//     field renamed, required field removed)
//   - the runProbe() return shape changes
//   - the record() payload shape changes
// Do NOT bump for internal refactors that preserve the
// external surface.
//
// History:
//   1 -- initial shape (page, base, scratchDir, record,
//         registerCleanup). Issue #703 introduced the
//         version constant + the kind/class fields on
//         record() + the runnerVersion field on the
//         JSON summary.
export const RUNNER_VERSION = 1;

// runProbe invokes the supplied probeFn with a sliced ctx
// and aggregates the result into a single
// `{name, ok, error?, details}` shape that the reporter
// records via the bridge in smoke_aggregator.mjs.
//
// Throwing inside probeFn is caught and surfaced as
// `{ok: false, error}`. The runner does not re-throw — the
// aggregator collects one result per probe and continues.
export async function runProbe({ name = '<unnamed>', probeFn = async () => ({ ok: true }) } = {}) {
  // Per-probe PARENT directory so the state root (which the
  // server computes as `<parent-of-dataDir>/.dixiedata-state`)
  // is also per-probe. Without this, every probe would share
  // `<tmpdir>/.dixiedata-state` and a settings change from
  // one probe (e.g. smoke_settings_appearance picking
  // `toast-only` export surface) would leak into every
  // subsequent probe on the same machine.
  const parentDir = mkdtempSync(join(tmpdir(), `smoke-${name}-`));
  const scratchDir = join(parentDir, '.dixiedata');
  mkdirSync(scratchDir, { recursive: true });
  const cleanups = [() => rmSync(parentDir, { recursive: true, force: true })];

  // The probeFn contract surface: opaque page (slice 3+
  // will replace {} with a real Playwright Page when
  // chromium.launch is wired into the runner), the base URL
  // (default localhost:8080; CI's audit.yml + the migrated
  // smoke_soldier_images.mjs both override via env), the
  // scratchDir, and the record + registerCleanup
  // reporters. Today the runner does NOT own the server
  // lifecycle -- that lives in the migrated probe (the
  // server-spawn pattern will be hoisted into the runner
  // itself in slice 5/6 once we have a stable per-probe
  // contract for 3 callers).
  const ctx = {
    page: {}, // slice 3+: await newPageFor() from runner
    base: process.env.SMOKE_BASE_URL || 'http://127.0.0.1:8080',
    scratchDir,
    record: (n, ok, details = {}) => {
      // Bridge into the shared reporter. We import here
      // rather than at module scope to avoid a circular
      // dependency (the reporter doesn't depend on the
      // runner).
      // eslint-disable-next-line global-require
      const { record: r } = require_report();
      r(n, ok, details);
    },
    registerCleanup: (fn) => {
      cleanups.push(fn);
    },
  };

  // The reporter bridge is a tiny indirection so the
  // ESM 'import' can stay at module scope. The dynamic
  // import below lazily resolves the reporter module on
  // first call.
  //
  // (We use a function rather than a top-level import to
  // avoid a cycle: smoke_reporter doesn't import the
  // runner, but if it did, our top-level static import
  // would deadlock at module evaluation.)
  function require_report() {
    return _reportModule;
  }

  let probeFnError;
  let probeFnResult = { ok: true };
  try {
    probeFnResult = await probeFn(ctx);
  } catch (err) {
    probeFnError = err;
  }

  // Run per-probe cleanup hooks in reverse-registration
  // order. Each hook is best-effort; one failure does not
  // block the others.
  for (const cleanup of cleanups.reverse()) {
    try {
      await cleanup();
    } catch (_) { /* best effort */ }
  }

  if (probeFnError) {
    return {
      name,
      ok: false,
      error: probeFnError.message || String(probeFnError),
      details: { scratchDir },
    };
  }

  return {
    name,
    ok: !!probeFnResult?.ok,
    error: null,
    details: { scratchDir, ...(probeFnResult?.details || {}) },
  };
}

// Reporter module is loaded once at module scope so the
// record() calls don't pay an import-cost penalty per
// probe. The lazy `_reportModule` lets the bridge in
// probeFn-record() resolve without an import cycle.
import * as _reportModuleNS from './smoke_reporter.mjs';
const _reportModule = {
  record: _reportModuleNS.record,
  reset: _reportModuleNS.reset,
  renderSummary: _reportModuleNS.renderSummary,
  writeJson: _reportModuleNS.writeJson,
};
