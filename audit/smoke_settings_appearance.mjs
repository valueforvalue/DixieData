/**
 * audit/smoke_settings_appearance.mjs — /settings/appearance surface
 * probe (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. /settings/appearance renders the SettingsAppearancePanel
 *      (3 theme radio options + Apply Theme submit + 2 export-
 *      surface radio options + Apply submit).
 *   2. The theme form action targets /settings/theme (the
 *      handleSettingsTheme POST endpoint).
 *   3. The export-surface form action targets
 *      /settings/export-surface (the handleSettingsExportSurface
 *      POST endpoint).
 *   4. Picking "High Contrast" + clicking Apply Theme posts to
 *      /settings/theme, server stores it, response redirects
 *      to /settings/appearance, and the high-contrast radio is
 *      now checked on re-render.
 *   5. Picking "toast-only" export surface + clicking Apply
 *      posts to /settings/export-surface, server stores it,
 *      and the toast-only radio is now checked on re-render.
 *
 * Run: `node audit/smoke_settings_appearance.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';
import { loadConfig, resolveBaseUrl } from './_lib/config.mjs';
import { submitAndWait } from './_lib/smoke_form.mjs';

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

  await page.goto(`${BASE}/settings/appearance`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const themeFormAction = await page.locator('form[data-dixie-submit][action="/settings/theme"]').first().getAttribute('action').catch(() => null);
  record('settings-appearance-theme-form-targets-theme', themeFormAction === '/settings/theme', { themeFormAction });

  const exportSurfaceFormAction = await page.locator('form[data-dixie-submit][action="/settings/export-surface"]').first().getAttribute('action').catch(() => null);
  record('settings-appearance-export-surface-form-targets-endpoint', exportSurfaceFormAction === '/settings/export-surface', { exportSurfaceFormAction });

  record('settings-appearance-theme-has-three-radios', (await page.locator('input[name="theme"]').count()) === 3, {});
  record('settings-appearance-export-surface-has-two-radios', (await page.locator('input[name="export_surface"]').count()) === 2, {});

  // The theme radios are sr-only (visually hidden); .check() with
  // force:true bypasses the actionability check. Alternatively the
  // label[data-theme-option="high-contrast"] wraps the input and
  // is the user-visible click target. Use force:true to keep the
  // probe stable across theme-picker DOM refactors that change
  // label layout but not the input semantics.
  await page.locator('input[name="theme"][value="high-contrast"]').check({ force: true });
  // Issue #715: use submitAndWait helper for the form
  // submit + post-POST read. The previous
  // `Promise.all([page.waitForURL(/settings/appearance/, ...).catch(() => null), click])`
  // shape had a latent race (URL was already at
  // /settings/appearance from the initial goto, so waitForURL
  // resolved immediately; #713 was the deterministic
  // failure on the export-surface branch which had a
  // defensive second goto that raced with the dispatcher's
  // programmatic navigation). submitAndWait pins the POST
  // response BEFORE the DOM read, then waits for the
  // dispatcher's navigation to settle via networkidle.
  const themeSubmit = page.locator('form[data-dixie-submit][action="/settings/theme"] button[type="submit"]').first();
  await submitAndWait(page, {
    submit: themeSubmit,
    urlPredicate: (u) => u.endsWith('/settings/theme'),
  });
  const themeChecked = await page.locator('input[name="theme"][value="high-contrast"]').isChecked().catch(() => false);
  record('settings-appearance-theme-save-round-trip', themeChecked, { themeChecked, url: page.url() });

  await page.goto(`${BASE}/settings/appearance`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 300));
  await page.locator('input[name="export_surface"][value="toast-only"]').check({ force: true });
  // Issue #715: same submitAndWait helper as the theme step above.
  // The export-surface round-trip was the deterministic-failing case
  // (#713) before this helper existed — the previous
  // `Promise.all([waitForURL, click])` + defensive `page.goto` shape
  // raced with the dispatcher's programmatic navigation.
  const exportSubmit = page.locator('form[data-dixie-submit][action="/settings/export-surface"] button[type="submit"]').first();
  const { response: exportPostResp } = await submitAndWait(page, {
    submit: exportSubmit,
    urlPredicate: (u) => u.includes('/settings/export-surface'),
  });
  const exportChecked = await page.locator('input[name="export_surface"][value="toast-only"]').isChecked().catch(() => false);
  record('settings-appearance-export-surface-save-round-trip', exportChecked, { exportChecked, url: page.url(), exportPostStatus: exportPostResp?.status() });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'settings-appearance', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });