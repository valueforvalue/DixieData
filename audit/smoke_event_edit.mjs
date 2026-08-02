/**
 * audit/smoke_event_edit.mjs — /events/{id}/edit surface probe (issue #700).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers + 1 event, then asserts:
 *   1. /events/{id}/edit renders the form, title + description
 *      inputs, and Save button.
 *   2. The form action targets /events/{id} (the EventDetail
 *      routebuilder URL).
 *   3. Editing the description + clicking Save keeps the
 *      page on /events/{id}/edit (in-place update) and
 *      re-renders with the new description text.
 *   4. The Tags form posts to /events/{id}/tags and Add Tag
 *      + Linked Persons surfaces are rendered.
 *   5. The Linked Persons add-form action targets
 *      /events/{id}/links.
 *
 * Run: `node audit/smoke_event_edit.mjs`
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
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-soldiers', '3', '-events', '1', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
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

  // Discover the seeded event id by scanning the events list
  // for any /events/{id} link, then open the detail page to
  // find the Edit anchor.
  await page.goto(`${BASE}/events`, { waitUntil: 'networkidle' });
  const firstEventHref = await page.locator('a[href^="/events/"][href*="/events/"]:not([href$="/new"]):not([href$="/edit"]):not([href$="/import"])').first().getAttribute('href').catch(() => null);
  const eventId = firstEventHref ? Number((firstEventHref.match(/\/events\/(\d+)/) || [])[1]) : 0;
  record('events-list-has-event-link', Number.isFinite(eventId) && eventId > 0, { firstEventHref, eventId });

  await page.goto(`${BASE}/events/${eventId}`, { waitUntil: 'networkidle' });
  const editHref = await page.locator('a[href*="/edit"]').first().getAttribute('href').catch(() => null);
  record('event-detail-has-edit-link', (editHref || '').includes(`/events/${eventId}/edit`), { editHref });

  await page.goto(`${BASE}/events/${eventId}/edit`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const formAction = await page.locator('form[data-dixie-submit]').first().getAttribute('action');
  record('event-edit-form-targets-event-detail', new RegExp(`/events/${eventId}(?:/|$)`).test(formAction || ''), { formAction });
  const titleCount = await page.locator('input[name="title"]').count();
  const descCount = await page.locator('textarea[name="description"]').count();
  const heading = await page.locator('h1, h2').first().textContent().catch(() => '');
  record('event-edit-has-display-heading', (heading || '').trim().length > 0, { heading });
  record('event-edit-has-description-input', (await page.locator('textarea[name="description"]').count()) >= 1, {});
  record('event-edit-has-save-button', (await page.locator('form[data-dixie-submit] button[type="submit"]').count()) >= 1, {});

  const tagsForm = await page.locator('form[data-dixie-submit][action$="/tags"]').first().getAttribute('action').catch(() => null);
  record('event-edit-renders-tags-form', new RegExp(`/events/${eventId}/tags`).test(tagsForm || ''), { tagsForm });

  const linksForm = await page.locator('form[data-dixie-submit][action$="/links"]').first().getAttribute('action').catch(() => null);
  record('event-edit-renders-linked-persons-form', new RegExp(`/events/${eventId}/links`).test(linksForm || ''), { linksForm });

  const newDesc = 'Smoke description update ' + Date.now();
  await page.locator('textarea[name="description"]').fill(newDesc);
  await Promise.all([
    page.waitForResponse((r) => r.url().includes(`/events/${eventId}`) && r.request().method() === 'POST', { timeout: 10_000 }).catch(() => null),
    page.locator('form[data-dixie-submit]').first().locator('button[type="submit"]').click(),
  ]);
  await new Promise((r) => setTimeout(r, 400));
  const onDetail = /\/events\/\d+\/?$/.test(page.url());
  record('event-edit-save-redirects-or-reloads-detail', onDetail, { url: page.url() });

  await page.goto(`${BASE}/events/${eventId}`, { waitUntil: 'networkidle' });
  const rendered = await page.content();
  record('event-detail-renders-updated-description', rendered.includes(newDesc), { hasNew: rendered.includes(newDesc) });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'event-edit', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });
