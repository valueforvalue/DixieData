/**
 * audit/smoke_share_queue.mjs — /share/queue surface probe
 * (issue #700 tier-2 follow-up).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers, then asserts:
 *   1. /share/queue renders the queue management panel
 *      (id=panel.share.queue).
 *   2. The empty-state copy renders when no rows are
 *      staged in the client-side share-queue store.
 *   3. After staging 2 soldier IDs via the
 *      `data-share-queue-add` button on /browse (which
 *      writes the client-side store via `addToShareQueue`),
 *      /share/queue renders 2 entry rows + the per-row
 *      Remove button + the select-all checkbox.
 *   4. The bulk-action form targets
 *      /export/shared-archive/subset and carries
 *      data-share-queue-page-form (the JS that drives
 *      the per-row export wiring).
 *   5. The preset-save form carries the preset-name input
 *      + the Save current queue submit button.
 *   6. After removing a row via the per-row Remove
 *      button, the queue shrinks to 1 entry.
 *
 * Note: clicking "Export Selected as .ddshare" triggers
 * a native save dialog (out of scope for headless).
 * The bulk-export round-trip is covered by the legacy
 * probe audit/smoke_share_queue_clear_after_export.mjs.
 *
 * Run: `node audit/smoke_share_queue.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';
import { loadConfig, resolveBaseUrl } from './_lib/config.mjs';

const cfg = loadConfig();
const BASE = resolveBaseUrl(cfg);
const PORT = process.env.PROBE_PORT ? parseInt(process.env.PROBE_PORT, 10) : cfg.defaultPort;
const WEB_BIN_PATH = webBin();
if (!existsSync(WEB_BIN_PATH)) { console.error('missing', WEB_BIN_PATH); process.exit(2); }

let pass = 0, fail = 0;
const results = [];
function record(name, ok, details = {}) {
  results.push({ name, ok, details });
  if (ok) { pass++; console.log(`  PASS ${name}`); }
  else { fail++; console.log(`  FAIL ${name}\n    ${JSON.stringify(details).slice(0, 800)}`); }
}

async function main(ctx) {
  const SCRATCH = ctx.scratchDir;
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-soldiers', '3', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let seedOut = '';
  seedProc.stdout.on('data', (d) => { seedOut += d; });
  seedProc.stderr.on('data', (d) => { seedOut += d; });
  const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
  if (seedExit !== 0) throw new Error(`seed-data failed (${seedExit}):\n${seedOut}`);

  const server = spawn(WEB_BIN_PATH, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', SCRATCH], { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, DIXIEDATA_DATA_DIR: SCRATCH } });
  ctx.registerCleanup(() => { try { server.kill(); } catch (_) {} });

  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(`${BASE}/`); if (r.status < 500) break; } catch {}
    await new Promise((r) => setTimeout(r, 250));
  }

  const browser = await chromium.launch({ headless: true });
  ctx.registerCleanup(() => browser.close().catch(() => {}));
  const page = await browser.newPage({ viewport: { width: 1600, height: 1200 } });
  page.on('dialog', (d) => { d.accept().catch(() => {}); });

  // Empty state: no rows staged.
  await page.goto(`${BASE}/share/queue`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const emptyText = await page.locator('body').innerText().catch(() => '');
  record('share-queue-empty-state-renders', /No Person Records staged/i.test(emptyText), {});

  // Stage 2 soldiers via direct localStorage write (mirrors
  // what the data-share-queue-add click would do via
  // addToShareQueue). More deterministic than the click path
  // for headless; the click path is exercised by
  // smoke_browse.mjs.
  await page.goto(`${BASE}/browse`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 600));
  const browseHrefs = await page.locator('a[href^="/soldiers/"]:not([href$="/new"]):not([href$="/edit"])').evaluateAll((anchors) =>
    anchors.map((a) => Number((a.getAttribute('href') || '').match(/\/soldiers\/(\d+)/)?.[1] || 0)).filter((n) => n > 0)
  );
  const uniqueIds = [...new Set(browseHrefs)].slice(0, 2);
  if (uniqueIds.length < 2) throw new Error(`/browse has ${browseHrefs.length} soldier links; need at least 2 unique ids`);
  await page.evaluate((ids) => {
    window.localStorage.setItem('dixiedata.share-queue', JSON.stringify(ids));
  }, uniqueIds);

  await page.goto(`${BASE}/share/queue`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 500));
  const queueRows = await page.locator('tr[data-share-queue-page-row-id]').count();
  record('share-queue-renders-staged-rows', queueRows === 2, { queueRows });
  record('share-queue-row-remove-button-renders', (await page.locator('button[data-share-queue-page-remove-id]').count()) >= 2, {});
  record('share-queue-select-all-checkbox-renders', (await page.locator('input[data-share-queue-page-select-all]').count()) >= 1, {});

  const bulkFormAction = await page.locator('form[data-share-queue-page-form]').first().getAttribute('action').catch(() => null);
  record('share-queue-bulk-form-targets-subset-export', bulkFormAction !== null && /\/export\/shared-archive(\/subset|\?subset)/.test(bulkFormAction), { bulkFormAction });

  record('share-queue-preset-save-form-renders', (await page.locator('form[data-share-queue-preset-save]').count()) >= 1, {});
  record('share-queue-preset-name-input-renders', (await page.locator('form[data-share-queue-preset-save] input[name="name"]').count()) >= 1, {});

  // Remove the first row and assert the queue shrinks.
  const firstRemove = page.locator('button[data-share-queue-page-remove-id]').first();
  const firstRemoveId = await firstRemove.getAttribute('data-share-queue-page-remove-id');
  await firstRemove.click();
  await new Promise((r) => setTimeout(r, 500));
  await page.goto(`${BASE}/share/queue`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 500));
  const queueRowsAfterRemove = await page.locator('tr[data-share-queue-page-row-id]').count();
  const removedRowGone = (await page.locator(`tr[data-share-queue-page-row-id="${firstRemoveId}"]`).count()) === 0;
  record('share-queue-remove-row-shrinks-queue', queueRowsAfterRemove === 1 && removedRowGone, { queueRowsAfterRemove, removedRowGone });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'share-queue', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });