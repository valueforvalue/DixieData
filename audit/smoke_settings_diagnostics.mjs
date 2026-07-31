/**
 * audit/smoke_settings_diagnostics.mjs — /settings/diagnostics surface
 * probe (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. /settings/diagnostics renders the diagnostics panel with
 *      the Debug Mode toggle + Support & Diagnostics bundle.
 *   2. The debug-mode form action targets /settings/debug-mode
 *      and exposes the debug_mode checkbox.
 *   3. Toggling debug mode ON + Save persists the toggle, the
 *      response reloads the page, and the checkbox is checked
 *      on re-render (debugMode.Load() returns true).
 *   4. Toggling debug mode OFF + Save persists the toggle and
 *      the checkbox is unchecked on re-render.
 *   5. The bug-report form targets /export/bug-report and
 *      carries the include_images checkbox (defaults to checked
 *      per issue #545 slice 3).
 *   6. The Export Feedback Log button carries data-action
 *      pointing at /export/feedback-log.
 *
 * Note: clicking the Export Feedback Log or Export Bug Report
 * Bundle triggers native download dialogs on the OS — the
 * probe does NOT click them (would require a download path
 * capture). Coverage for the bug-report export lives in
 * internal/appshell/app_bug_report_test.go (server side).
 *
 * Run: `node audit/smoke_settings_diagnostics.mjs`
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
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-soldiers', '1', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
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

  await page.goto(`${BASE}/settings/diagnostics`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const pageRoot = await page.locator('[id="page.settings.diagnostics"]').count();
  record('settings-diagnostics-renders-page', pageRoot >= 1, { pageRoot });

  const debugModeAction = await page.locator('form[data-dixie-submit][action="/settings/debug-mode"]').first().getAttribute('action').catch(() => null);
  record('settings-diagnostics-debug-mode-form-targets-endpoint', debugModeAction === '/settings/debug-mode', { debugModeAction });
  record('settings-diagnostics-debug-mode-checkbox-renders', (await page.locator('input[name="debug_mode"]').count()) === 1, {});

  // Toggle ON + save: form has data-reload-on-success="true";
  // the JS dispatcher reloads the page on a 204. Wait for the
  // post-response navigation to settle, then verify the checkbox
  // reflects the new state.
  await page.locator('input[name="debug_mode"]').check();
  await Promise.all([
    page.waitForResponse((r) => /\/settings\/debug-mode/.test(r.url()) && r.request().method() === 'POST', { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action="/settings/debug-mode"] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  // Wait for the reload that data-reload-on-success triggers.
  await page.waitForLoadState('domcontentloaded').catch(() => null);
  await new Promise((r) => setTimeout(r, 400));
  const debugOnChecked = await page.locator('input[name="debug_mode"]').isChecked().catch(() => false);
  record('settings-diagnostics-debug-mode-toggle-on-round-trip', debugOnChecked, { debugOnChecked });

  // Toggle OFF + save.
  await page.locator('input[name="debug_mode"]').uncheck();
  await Promise.all([
    page.waitForResponse((r) => /\/settings\/debug-mode/.test(r.url()) && r.request().method() === 'POST', { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action="/settings/debug-mode"] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  await page.waitForLoadState('domcontentloaded').catch(() => null);
  await new Promise((r) => setTimeout(r, 400));
  const debugOffChecked = await page.locator('input[name="debug_mode"]').isChecked().catch(() => true);
  record('settings-diagnostics-debug-mode-toggle-off-round-trip', !debugOffChecked, { debugOffChecked });

  const bugReportAction = await page.locator('form[data-dixie-submit][action="/export/bug-report"]').first().getAttribute('action').catch(() => null);
  record('settings-diagnostics-bug-report-form-targets-endpoint', bugReportAction === '/export/bug-report', { bugReportAction });
  record('settings-diagnostics-bug-report-include-images-checkbox-defaults-checked', await page.locator('input[name="include_images"]').isChecked().catch(() => false), {});

  const feedbackLogAction = await page.locator('[data-action="/export/feedback-log"]').first().getAttribute('data-action').catch(() => null);
  record('settings-diagnostics-feedback-log-data-action-attr', feedbackLogAction === '/export/feedback-log', { feedbackLogAction });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'settings-diagnostics', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });