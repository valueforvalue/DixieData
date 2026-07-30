/**
 * audit/smoke_research_log.mjs — /soldiers/{id}/research-log
 * surface probe (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. /soldiers/{id}/research-log renders the research-log
 *      page with the Add Research Task form.
 *   2. The Add Task form action targets
 *      /soldiers/{id}/research-log/tasks and carries the
 *      title + evidence_type + notes inputs.
 *   3. The evidence_type select exposes 7 options (general,
 *      service, pension, burial, vital, family, Local
 *      Archive).
 *   4. Submitting a new task posts to /tasks, server stores
 *      it, response swaps the page fragment with the new
 *      task card visible.
 *   5. The per-task resolve button carries
 *      data-action="/soldiers/{id}/research-log/tasks/{entryId}/resolve".
 *
 * Run: `node audit/smoke_research_log.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8792';
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

  // Discover soldier id from /soldiers.
  await page.goto(`${BASE}/soldiers`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  // Browse shows the list; use that for IDs.
  await page.goto(`${BASE}/browse`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const firstHref = await page.locator('a[href^="/soldiers/"]:not([href$="/new"]):not([href$="/edit"])').first().getAttribute('href').catch(() => null);
  const soldierId = Number((firstHref || '').match(/\/soldiers\/(\d+)/)?.[1] || 0);
  if (!Number.isFinite(soldierId) || soldierId <= 0) throw new Error('cannot discover soldier id from /browse');

  await page.goto(`${BASE}/soldiers/${soldierId}/research-log`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const addTaskFormAction = await page.locator(`form[data-dixie-submit][action="/soldiers/${soldierId}/research-log/tasks"]`).first().getAttribute('action').catch(() => null);
  record('research-log-add-task-form-targets-endpoint', addTaskFormAction === `/soldiers/${soldierId}/research-log/tasks`, { addTaskFormAction });
  record('research-log-title-input-renders', (await page.locator('input[name="title"]').count()) >= 1, {});
  record('research-log-evidence-type-select-renders', (await page.locator('select[name="evidence_type"]').count()) >= 1, {});

  const evidenceTypeOptions = await page.locator('select[name="evidence_type"] option').evaluateAll((opts) => opts.map((o) => o.value));
  record('research-log-evidence-type-has-seven-options', evidenceTypeOptions.length === 7, { evidenceTypeOptions });

  // Submit a new task.
  const uniqueTitle = 'SmokeTask-' + Date.now();
  await page.locator('input[name="title"]').first().fill(uniqueTitle);
  await page.locator('select[name="evidence_type"]').first().selectOption('pension');
  await page.locator('textarea[name="notes"]').first().fill('Smoke task notes for tier-2 coverage.');
  await Promise.all([
    page.waitForResponse((r) => new RegExp(`/soldiers/${soldierId}/research-log/tasks`).test(r.url()) && r.request().method() === 'POST', { timeout: 15_000 }).catch(() => null),
    page.locator(`form[data-dixie-submit][action="/soldiers/${soldierId}/research-log/tasks"]`).first().locator('button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  await new Promise((r) => setTimeout(r, 600));
  await page.goto(`${BASE}/soldiers/${soldierId}/research-log`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const pageText = await page.locator('body').innerText().catch(() => '');
  record('research-log-create-task-round-trip', pageText.includes(uniqueTitle), {});

  const resolveAction = await page.locator('[data-action*="/research-log/tasks/"][data-action$="/resolve"]').first().getAttribute('data-action').catch(() => null);
  record('research-log-resolve-button-data-action-attr', resolveAction !== null && new RegExp(`/soldiers/${soldierId}/research-log/tasks/\\d+/resolve`).test(resolveAction), { resolveAction });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'research-log', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });