// audit/_lib/smoke_runner.mjs (issue #700, ADR 0011)
//
// Shared Playwright smoke-test runner for the DixieData
// audit harness. One chromium.launch() per run, one page
// per probe (a fresh context isolates cookies/storage).
// The aggregator at audit/smoke_aggregator.mjs walks
// audit/_lib/smoke_index.mjs::SURFACES[] and dispatches
// each entry to either a Playwright probe fn (this file's
// `runProbe`) OR a static-scanner sub-process (the four
// class-2/4/6/8/9 linters that already exist as standalone
// CLIs at audit/lint_*.mjs + scripts/lint-*.mjs).
//
// Slice 1 commits this file as the SKIN: `runProbe` is a
// no-op stub that records a single "ok: true" entry so the
// aggregator can already be wired. Subsequent slices
// (per the plan in CHANGELOG) populate the real browser
// lifecycle here + migrate the existing 3 Playwright
// smokes onto it.
//
// Why this is runner-only and not @playwright/test:
//   The repo already uses `node --test` + raw `playwright`.
//   Adding @playwright/test would inherit its runner, its
//   reporter, and its discovery model. The user explicitly
//   asked for "shared runner, no more duplication" — this
//   module honours that with ~200 LoC of plain ESM.

import { chromium } from 'playwright';

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
  // Native dialogs (the Wails-backed confirm() prompts in the
  // seed-data + share exports surfaces) must be auto-accepted
  // so the probe doesn't hang. Mirrors the existing smokes.
  page.on('dialog', (d) => d.accept().catch(() => {}));
  return page;
}

// probeShape is the contract every Playwright probe fn
// must conform to. This docblock is the only place the
// contract is documented — when slice 2 migrates the first
// probe onto runProbe, the contract gets its first real
// caller and any drift gets caught immediately.
export const PROBE_SHAPE = {
  /** @param {{ page: import('playwright').Page, base: string, scratchDir: string, record: (name:string, ok:boolean, details?:object)=>void, registerCleanup: (cleanup: () => Promise<void>|void) => void }} ctx */
  fn: async (_ctx) => { /* no-op stub */ },
};

// runProbe is the thunk every migrated smoke fn will call.
// For slice 1 the body is a no-op that records "ok: true".
// Subsequent slices will swap in the real browser + page
// lifecycle.
export async function runProbe({ name, probeFn = PROBE_SHAPE.fn, ctx = {} } = {}) {
  // Slice 1 stub: no browser, no server lifecycle. Just record
  // the result so the aggregator can already be wired end-to-end.
  return { name, ok: true, details: { slice: 1, stub: true, ...ctx } };
}
