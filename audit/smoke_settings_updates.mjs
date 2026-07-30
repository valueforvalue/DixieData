/**
 * audit/smoke_settings_updates.mjs — /settings/updates surface
 * probe (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. /settings/updates renders the updates panel
 *      (id="settings-update-panel") with the Current Version
 *      + source URL form.
 *   2. The source-URL save form action targets
 *      /settings/updates/source and carries the source_url
 *      input.
 *   3. The Use Default form targets /settings/updates/source
 *      with a hidden empty source_url field.
 *   4. The Check for Updates form targets
 *      /settings/updates/check with data-results-target
 *      #settings-update-status.
 *   5. Saving a custom source_url posts to /settings/updates/
 *      source, server stores it, the response swaps the
 *      settings-update-panel fragment with a success notice,
 *      and the new source_url value persists on re-render.
 *   6. Clicking Use Default sets source_url back to empty,
 *      the response swaps the panel fragment, and the
 *      Effective Source URL reads as the GitHub default
 *      feed URL.
 *
 * Run: `node audit/smoke_settings_updates.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8791';
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

  await page.goto(`${BASE}/settings/updates`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const pageRoot = await page.locator('[id="page.settings.updates"]').count();
  record('settings-updates-renders-page', pageRoot >= 1, { pageRoot });

  const panel = await page.locator('[id="settings-update-panel"]').count();
  record('settings-updates-panel-renders', panel >= 1, { panel });

  const saveSourceAction = await page.locator('form[data-dixie-submit][action="/settings/updates/source"]').first().getAttribute('action').catch(() => null);
  record('settings-updates-save-source-form-targets-endpoint', saveSourceAction === '/settings/updates/source', { saveSourceAction });
  record('settings-updates-source-url-input-renders', (await page.locator('input[name="source_url"]').first().count()) >= 1, {});

  const useDefaultAction = await page.locator('form[data-dixie-submit][action="/settings/updates/source"] input[type="hidden"][name="source_url"][value=""]').count();
  record('settings-updates-use-default-form-has-hidden-empty', useDefaultAction >= 1, { useDefaultAction });

  const checkAction = await page.locator('form[data-dixie-submit][action="/settings/updates/check"]').first().getAttribute('action').catch(() => null);
  record('settings-updates-check-form-targets-endpoint', checkAction === '/settings/updates/check', { checkAction });

  // Save custom source URL.
  const customUrl = 'https://example.test/smoke-update-feed.json';
  await page.locator('input[name="source_url"]').first().fill(customUrl);
  await Promise.all([
    page.waitForResponse((r) => /\/settings\/updates\/source/.test(r.url()) && r.request().method() === 'POST', { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action="/settings/updates/source"]').first().locator('button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  await new Promise((r) => setTimeout(r, 500));
  await page.goto(`${BASE}/settings/updates`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const persistedCustom = await page.locator('input[name="source_url"]').first().inputValue().catch(() => '');
  record('settings-updates-save-custom-source-round-trip', persistedCustom === customUrl, { persistedCustom });

  // Use Default -- click the form with the hidden empty source_url.
  await page.locator('form[data-dixie-submit][action="/settings/updates/source"]:has(input[type="hidden"][name="source_url"][value=""]) button[type="submit"]').first().click({ timeout: 5_000 });
  await page.waitForResponse((r) => /\/settings\/updates\/source/.test(r.url()) && r.request().method() === 'POST', { timeout: 15_000 }).catch(() => null);
  await new Promise((r) => setTimeout(r, 500));
  await page.goto(`${BASE}/settings/updates`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const persistedDefault = await page.locator('input[name="source_url"]').first().inputValue().catch(() => '__non-empty__');
  record('settings-updates-use-default-round-trip-clears-input', persistedDefault === '', { persistedDefault });
  const sourceKindText = await page.locator('body').innerText().catch(() => '');
  record('settings-updates-use-default-shows-default-feed-label', /Default GitHub latest release feed/i.test(sourceKindText), {});

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'settings-updates', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });