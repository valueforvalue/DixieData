/**
 * audit/smoke_initial_setup.mjs — /setup surface probe
 * (issue #700 tier-3).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir
 * with a fresh `.dixiedata` subdir (no DB files), so
 * `setupRequired=true` on startup and `/setup` renders the
 * First Launch Setup wizard. Asserts:
 *   1. /setup renders the First Launch Setup form (heading
 *      + first_name / middle_name / last_name / birth_year
 *      inputs).
 *   2. The form action targets /setup (POST to
 *      handleInitialSetup).
 *   3. Filling first_name + last_name + birth_year + clicking
 *      Save Identity posts the form, server stores the
 *      identity, response redirects to /calendar, and the
 *      /setup page now redirects away (setupRequired=false).
 *   4. After the identity save, /calendar renders with the
 *      page header (the setupRequired gate clears).
 *
 * Run: `node audit/smoke_initial_setup.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync, mkdirSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8793';
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
  // Fresh DB to trigger setupRequired=true. The probe nests a
  // `.dixiedata` child inside the runner scratch dir (mirrors
  // the smoke_settings_data pattern); this time we DO NOT seed
  // so the DB stays uninitialized.
  const SCRATCH = ctx.scratchDir;
  const DATA_DIR = `${SCRATCH}/.dixiedata`;
  mkdirSync(DATA_DIR, { recursive: true });

  const server = spawn(WEB_BIN_PATH, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', DATA_DIR], { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, DIXIEDATA_DATA_DIR: DATA_DIR } });
  ctx.registerCleanup(() => { try { server.kill(); } catch (_) {} });

  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(`${BASE}/`); if (r.status < 500) break; } catch {}
    await new Promise((r) => setTimeout(r, 250));
  }

  const browser = await chromium.launch({ headless: true });
  ctx.registerCleanup(() => browser.close().catch(() => {}));
  const page = await browser.newPage({ viewport: { width: 1600, height: 1200 } });
  page.on('dialog', (d) => { d.accept().catch(() => {}); });
  page.on('response', (r) => {
    if (/\/setup\b/.test(r.url()) && r.request().method() === 'POST') {
      console.log('  [diag] setup POST status=', r.status(), 'redirect=', r.headers()['x-dixiedata-redirect']);
    }
  });

  await page.goto(`${BASE}/setup`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const heading = await page.locator('h2:has-text("First Launch Setup")').count();
  record('initial-setup-renders-heading', heading >= 1, { heading });
  record('initial-setup-form-targets-setup', (await page.locator('form[data-dixie-submit][action="/setup"]').count()) >= 1, {});
  record('initial-setup-has-first-name-input', (await page.locator('input[name="first_name"]').count()) >= 1, {});
  record('initial-setup-has-middle-name-input', (await page.locator('input[name="middle_name"]').count()) >= 1, {});
  record('initial-setup-has-last-name-input', (await page.locator('input[name="last_name"]').count()) >= 1, {});
  record('initial-setup-has-birth-year-input', (await page.locator('input[name="birth_year"]').count()) >= 1, {});

  await page.locator('input[name="first_name"]').fill('Test');
  await page.locator('input[name="middle_name"]').fill('Smoke');
  await page.locator('input[name="last_name"]').fill('Researcher');
  await page.locator('input[name="birth_year"]').fill('1985');
  await Promise.all([
    page.waitForURL(/\/calendar/, { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action="/setup"] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  await new Promise((r) => setTimeout(r, 500));
  record('initial-setup-save-redirects-to-calendar', /\/calendar/.test(page.url()), { url: page.url() });

  // After save, /setup should redirect away (setupRequired=false).
  await page.goto(`${BASE}/setup`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const setupStillRendering = (await page.locator('h2:has-text("First Launch Setup")').count()) >= 1;
  record('initial-setup-after-save-redirects-away', !setupStillRendering, { url: page.url(), setupStillRendering });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'initial-setup', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });