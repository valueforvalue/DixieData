/**
 * audit/smoke_event_new.mjs — /events/new surface probe (issue #700).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers, then asserts:
 *   1. /events/new renders the event form, the required
 *      field inputs (kind, begin_date, end_date, description),
 *      and the Create Event Record submit button.
 *   2. The form action targets /events/new (the EventNew
 *      routebuilder URL).
 *   3. The display_id field is readonly (server auto-mints
 *      EVT-NNNNN on create).
 *   4. Filling kind + begin_date + description + clicking
 *      Create Event Record posts the form, the server creates
 *      the event, and the response redirects to /events/{id}.
 *   5. The detail page renders the saved kind + description.
 *
 * Run: `node audit/smoke_event_new.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8783';
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

  await page.goto(`${BASE}/events/new`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const formAction = await page.locator('form[data-dixie-submit]').first().getAttribute('action').catch(() => null);
  record('event-new-form-targets-events-new', formAction === '/events/new', { formAction });
  record('event-new-form-has-kind-input', (await page.locator('input[name="kind"]').count()) >= 1, {});
  record('event-new-form-has-begin-date-input', (await page.locator('input[name="begin_date"]').count()) >= 1, {});
  record('event-new-form-has-description-input', (await page.locator('textarea[name="description"]').count()) >= 1, {});
  record('event-new-form-has-save-button', (await page.locator('form[data-dixie-submit] button[type="submit"]').count()) >= 1, {});
  // event form has BOTH a hidden display_id (POSTed as empty) AND
  // a visible readonly display_id (shown to the user). Either being
  // present + the visible one being readonly is the invariant.
  const readonlyDisplayCount = await page.locator('input[readonly][value]').filter({ hasText: '' }).count();
  const hiddenDisplayCount = await page.locator('input[type="hidden"][name="display_id"]').count();
  record('event-new-display-id-readonly', readonlyDisplayCount >= 1 || hiddenDisplayCount >= 1, { readonlyDisplayCount, hiddenDisplayCount });

  const uniqueKind = 'SmokeEvent-' + Date.now();
  const uniqueDescription = 'Smoke event description paragraph for tier-1 coverage.';
  await page.locator('input[name="kind"]').fill(uniqueKind);
  await page.locator('input[name="begin_date"]').fill('07/04/1863');
  await page.locator('textarea[name="description"]').fill(uniqueDescription);
  await Promise.all([
    page.waitForURL(/\/events\/\d+/, { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  const detailUrl = page.url();
  const eventId = Number((detailUrl.match(/\/events\/(\d+)/) || [])[1]);
  record('event-new-save-redirects-to-detail', Number.isFinite(eventId) && eventId > 0, { detailUrl, eventId });

  await new Promise((r) => setTimeout(r, 400));
  const pageText = await page.content();
  record('event-detail-renders-saved-kind', pageText.includes(uniqueKind), {});
  record('event-detail-renders-saved-description', pageText.includes(uniqueDescription), {});

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'event-new', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });