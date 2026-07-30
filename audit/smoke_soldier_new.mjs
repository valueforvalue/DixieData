/**
 * audit/smoke_soldier_new.mjs — /soldiers/new surface probe (issue #700).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * then asserts:
 *   1. /soldiers/new renders the entry form, all required
 *      field inputs (first_name, last_name, display_id), and
 *      the Save Changes submit button.
 *   2. The form action targets /soldiers (the SoldierCreate
 *      routebuilder URL).
 *   3. Filling first_name + last_name + clicking Save Changes
 *      posts the form, the server creates the soldier with an
 *      auto-minted DisplayID, and the response redirects to
 *      /soldiers/{id}.
 *   4. The detail page renders the saved first_name + last_name.
 *   5. The display_id field is readonly on the new form (server
 *      auto-mints; users do not pick their own).
 *
 * Note: a duplicate-display_id probe was drafted in an earlier
 * revision of this file but the server auto-mints the field on
 * create, so the duplicate path is not exercisable via this
 * surface. Coverage for duplicate handling lives in
 * internal/appshell/soldier_create_required_test.go and the
 * review-queue UI surface.
 *
 * Run: `node audit/smoke_soldier_new.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8782';
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
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
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

  await page.goto(`${BASE}/soldiers/new`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const formAction = await page.locator('form[data-dixie-submit][action="/soldiers"]').first().getAttribute('action').catch(() => null);
  record('soldier-new-form-targets-soldiers', formAction === '/soldiers', { formAction });
  record('soldier-new-form-has-first-name-input', (await page.locator('input[name="first_name"]').count()) >= 1, {});
  record('soldier-new-form-has-last-name-input', (await page.locator('input[name="last_name"]').count()) >= 1, {});
  record('soldier-new-form-has-display-id-input', (await page.locator('input[name="display_id"]').count()) >= 1, {});
  record('soldier-new-form-has-save-button', (await page.locator('form[data-dixie-submit][action="/soldiers"] button[type="submit"]').count()) >= 1, {});

  const uniqueId = 'SMK-' + Date.now();
  await page.locator('input[name="first_name"]').fill('Smoke');
  await page.locator('input[name="last_name"]').fill('Soldier');
  // display_id is readonly on the new form (server auto-mints); skip fill.
  // The duplicate-display-id assertion below uses URL/heading text instead.
  await Promise.all([
    page.waitForURL(/\/soldiers\/\d+/, { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action="/soldiers"] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  const detailUrl = page.url();
  const soldierId = Number((detailUrl.match(/\/soldiers\/(\d+)/) || [])[1]);
  record('soldier-new-save-redirects-to-detail', Number.isFinite(soldierId) && soldierId > 0, { detailUrl, soldierId });

  const heading = await page.locator('h1, h2').first().textContent().catch(() => '');
  const pageText = await page.content();
  // Server auto-mints the DisplayID on create, so we cannot
  // assert uniqueId appears; instead assert the saved name
  // (first_name + last_name) renders on the detail page.
  record('soldier-detail-renders-saved-name', (heading || '').includes('Smoke Soldier') || pageText.includes('Smoke Soldier'), { heading });

  // Duplicate display_id should fail validation.
  await page.goto(`${BASE}/soldiers/new`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 300));
  await page.locator('input[name="first_name"]').fill('Dup');
  await page.locator('input[name="last_name"]').fill('Entry');
  // display_id is readonly on the new form; duplicate-display-id
  // assertion (the prior probe generation) is not exercisable
  // via this surface. Drop it; coverage for duplicate handling
  // lives in internal/appshell/soldier_create_required_test.go
  // (server-side guard) and the review-queue UI surface.
  record('soldier-new-display-id-readonly', (await page.locator('input[name="display_id"][readonly]').count()) >= 1, {});

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'soldier-new', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });
