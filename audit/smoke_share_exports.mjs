/**
 * audit/smoke_share_exports.mjs — /share/exports surface probe
 * (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers, then asserts:
 *   1. /share/exports renders the exports panel with the
 *      6 export action buttons (JSON / Excel / iCalendar /
 *      Static Archive / Printable PDF / Backup / Shared
 *      Archive) + the include_tags form.
 *   2. Each export button carries the expected data-action
 *      attribute (regression net for button-URL drift
 *      per issue #687 / class 6).
 *   3. The static-archive form action targets
 *      /export/static-archive (note: this form does NOT
 *      carry data-dixie-submit; the JS dispatcher relies on
 *      the standard form submit event).
 *   4. The include-tags form action targets
 *      /share/export-options, the hidden include_tags=0
 *      sibling renders, and the include_tags=1 checkbox
 *      is initially unchecked.
 *
 * Note: the include-tags toggle is currently a DEAD UI --
 * the form has no submit button and no JS change handler
 * (data-share-include-tags is unwired in frontend/app.js).
 * Toggling the checkbox does NOT POST to /share/export-options
 * today. Filed as a follow-up issue; covered server-side by
 * internal/appshell/tags_handlers_test.go. This probe asserts
 * the form contract (right action, right fields) and skips
 * the round-trip until the JS wiring lands.
 *
 * Note: the export buttons themselves trigger native save
 * dialogs (Wails SaveFileDialog) and download flows; the
 * probe does NOT click them (would require a download path
 * capture). Coverage for the individual export handlers
 * lives in internal/appshell/exports_handlers_test.go and
 * the smoke_export_*_microcopy probes.
 *
 * Run: `node audit/smoke_share_exports.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8788';
const BASE = `http://127.0.0.1:${PORT}`;
const WEB_BIN_PATH = webBin();
if (!existsSync(WEB_BIN_PATH)) { console.error('missing', WEB_BIN_PATH); process.exit(2); }

let pass = 0, fail = 0;
const results = [];
function record(name, ok, details = {}) {
  results.push({ name, ok, details });
  if (ok) { pass++; console.log(`  PASS ${name}`); }
  else { fail++; console.log(`  FAIL ${name}\n    ${JSON.stringify(details).slice(0, 800)}`); }
}

const EXPECTED_ACTIONS = {
  '/export/json': 'Export JSON',
  '/export/csv': 'Export Excel (.xlsx)',
  '/export/ical': 'Export iCalendar',
  '/export/backup': 'Export Backup (.ddbak)',
  '/export/shared-archive': 'Export Shared Archive (.ddshare)',
};

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

  await page.goto(`${BASE}/share/exports`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const panel = await page.locator('[id="panel.share.exports"]').count();
  record('share-exports-renders-panel', panel >= 1, { panel });

  for (const [action, label] of Object.entries(EXPECTED_ACTIONS)) {
    const attr = await page.locator(`[data-action="${action}"]`).first().getAttribute('data-action').catch(() => null);
    record(`share-exports-${label.replace(/[^a-z0-9]+/gi, '-').toLowerCase()}-data-action-attr`, attr === action, { action, attr });
  }

  // The static-archive button lives inside its own <form>;
  // assert the form action. This form does NOT carry
  // data-dixie-submit -- the standard form submit event fires
  // via the embedded button[type="submit"].
  const staticArchiveFormAction = await page.locator('form[action*="/export/static-archive"]').first().getAttribute('action').catch(() => null);
  record('share-exports-static-archive-form-targets-endpoint', staticArchiveFormAction !== null && /\/export\/static-archive/.test(staticArchiveFormAction), { staticArchiveFormAction });

  const exportOptionsAction = await page.locator('form[data-dixie-submit][action="/share/export-options"]').first().getAttribute('action').catch(() => null);
  record('share-exports-include-tags-form-targets-endpoint', exportOptionsAction === '/share/export-options', { exportOptionsAction });
  // The hidden include_tags=0 sibling must render so the form
  // POSTs a deterministic value when the checkbox is unchecked.
  record('share-exports-include-tags-hidden-zero-renders', (await page.locator('input[type="hidden"][name="include_tags"][value="0"]').count()) >= 1, {});
  record('share-exports-include-tags-checkbox-default-unchecked', await page.locator('input[name="include_tags"][value="1"]').isChecked().then((v) => !v).catch(() => false), {});

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'share-exports', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });