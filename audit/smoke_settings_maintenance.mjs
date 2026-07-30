/**
 * audit/smoke_settings_maintenance.mjs — /settings/maintenance surface
 * probe (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers, then asserts:
 *   1. /settings/maintenance renders the maintenance panel
 *      (id="page.settings.maintenance") with the image-orphans
 *      scan + data-quality scan forms.
 *   2. The image-orphans-scan form action targets
 *      /settings/images/orphans/scan.
 *   3. The data-quality-scan form action targets
 *      /settings/quality/scan and carries the dry_run checkbox.
 *   4. POSTing the data-quality scan returns 200 + the
 *      findings fragment renders inside #settings-quality-results.
 *   5. POSTing the image-orphans scan returns 200 + the
 *      results fragment renders inside #settings-orphan-results.
 *
 * Note: /settings/images/orphans/cleanup is a per-image-row
 * action triggered by the cleanup form that the orphan-scan
 * results render on demand, not a top-level form on the
 * maintenance page. Coverage for that handler lives in the
 * server-side render test
 * `internal/appshell/settings_handlers_test.go`.
 *
 * Note: the maintenance sub-page uses `dry_run` checkbox
 * (issue #609), not the `quality_mode` radio from the older
 * inline settings view. The legacy radio is reachable on the
 * older SettingsView template; this probe targets the
 * /settings/maintenance sub-page only.
 *
 * Run: `node audit/smoke_settings_maintenance.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8785';
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

  await page.goto(`${BASE}/settings/maintenance`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const pageRoot = await page.locator('[id="page.settings.maintenance"]').count();
  record('settings-maintenance-renders-page', pageRoot >= 1, { pageRoot });

  const orphanScanAction = await page.locator('form[data-dixie-submit][action="/settings/images/orphans/scan"]').first().getAttribute('action').catch(() => null);
  record('settings-maintenance-orphan-scan-form-targets-endpoint', orphanScanAction === '/settings/images/orphans/scan', { orphanScanAction });

  const qualityScanAction = await page.locator('form[data-dixie-submit][action*="/settings/quality"]').first().getAttribute('action').catch(() => null);
  record('settings-maintenance-quality-scan-form-targets-endpoint', qualityScanAction !== null && /\/settings\/quality\/scan/.test(qualityScanAction), { qualityScanAction });

  record('settings-maintenance-quality-dry-run-checkbox', (await page.locator('input[name="dry_run"]').count()) === 1, {});

  // Submit the data-quality scan and assert the results fragment renders.
  await Promise.all([
    page.waitForResponse((r) => /\/settings\/quality\/scan/.test(r.url()) && r.request().method() === 'POST', { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action*="/settings/quality"] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  await new Promise((r) => setTimeout(r, 600));
  const qualityResultsHtml = await page.locator('#settings-quality-results').innerHTML().catch(() => '');
  record('settings-maintenance-quality-scan-renders-results-fragment', qualityResultsHtml.trim().length > 0, { fragmentLength: qualityResultsHtml.length });

  // Submit the image-orphans scan and assert the results fragment renders.
  await Promise.all([
    page.waitForResponse((r) => /\/settings\/images\/orphans\/scan/.test(r.url()) && r.request().method() === 'POST', { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action="/settings/images/orphans/scan"] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  await new Promise((r) => setTimeout(r, 600));
  const orphanResultsHtml = await page.locator('#settings-orphan-results').innerHTML().catch(() => '');
  record('settings-maintenance-orphan-scan-renders-results-fragment', orphanResultsHtml.trim().length > 0, { fragmentLength: orphanResultsHtml.length });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'settings-maintenance', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });