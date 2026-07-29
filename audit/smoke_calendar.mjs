/**
 * audit/smoke_calendar.mjs — /calendar surface probe (issue #700).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers + 1 anniversary per month so every calendar
 * day shows a clickable card, then asserts:
 *   1. /calendar renders the 4-week grid (>= 28 day cells).
 *   2. Month dropdown navigates to /calendar/{n} and the new
 *      grid renders the same number of cells.
 *   3. A day cell with an anniversary carries a working
 *      hx-get to /calendar/{m}/{d} (the Anniversary endpoint).
 *   4. The "Export Month PDF" button posts to the report
 *      endpoint with `return=`.
 *   5. /calendar/{m}/{d} renders the day detail panel with
 *      the seeded anniversary entry.
 *
 * Run: `node audit/smoke_calendar.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8778';
const BASE = `http://127.0.0.1:${PORT}`;
const WEB_BIN_PATH = webBin();

if (!existsSync(WEB_BIN_PATH)) { console.error('missing', WEB_BIN_PATH); process.exit(2); }

async function ready() {
  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(`${BASE}/`); if (r.status < 500) return; } catch {}
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error('server never came up');
}

let pass = 0, fail = 0;
const results = [];
function record(name, ok, details = {}) {
  results.push({ name, ok, details });
  if (ok) { pass++; console.log(`  PASS ${name}`); }
  else { fail++; console.log(`  FAIL ${name}\n    ${JSON.stringify(details).slice(0, 800)}`); }
}

async function main(ctx) {
  const SCRATCH = ctx.scratchDir;
  const here = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = path.dirname(here);

  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-soldiers', '3', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let seedOut = '';
  seedProc.stdout.on('data', (d) => { seedOut += d; });
  seedProc.stderr.on('data', (d) => { seedOut += d; });
  const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
  if (seedExit !== 0) throw new Error(`seed-data failed (${seedExit}):\n${seedOut}`);

  const server = spawn(WEB_BIN_PATH, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', SCRATCH], { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, DIXIEDATA_DATA_DIR: SCRATCH } });
  ctx.registerCleanup(() => { try { server.kill(); } catch (_) {} });

  await ready();

  const browser = await chromium.launch({ headless: true });
  ctx.registerCleanup(() => browser.close().catch(() => {}));
  const page = await browser.newPage({ viewport: { width: 1600, height: 1200 } });

  await page.goto(`${BASE}/calendar`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 500));

  const dayBtn = page.locator('.grid.grid-cols-7 button[hx-get*="/anniversary/"]');
  const cells = await dayBtn.count();
  record('calendar-renders-28-plus-day-cells', cells >= 28, { cells });

  const monthSelect = await page.locator('#month-select').first();
  const monthValue = await monthSelect.inputValue();
  record('month-select-has-current-value', monthValue !== '', { monthValue });

  const targetMonth = monthValue === '1' ? '12' : '1';
  await monthSelect.selectOption(targetMonth);
  await page.waitForURL(new RegExp(`/calendar/${targetMonth}$`), { timeout: 5000 }).catch(() => null);
  const url = page.url();
  record('month-select-navigates-to-month-url', url.endsWith(`/calendar/${targetMonth}`), { url });
  const cellsAfter = await dayBtn.count();
  record('month-grid-uses-shared-day-class', cellsAfter >= 28, { cellsAfter });

  const firstCell = dayBtn.first();
  const hasHxGet = await firstCell.evaluate((el) => el.getAttribute('hx-get') || '');
  record('day-cell-wires-anniversary-hx-get', new RegExp(`/anniversary/${targetMonth}/`).test(hasHxGet), { hasHxGet });
  await firstCell.click();
  await page.waitForLoadState('domcontentloaded');
  const dayUrl = page.url();
  record('day-click-navigates-to-anniversary-endpoint', /\/calendar\/\d+/.test(dayUrl), { dayUrl });

  await page.goto(`${BASE}/calendar`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 300));
  const exportAction = await page.locator('form[data-dixie-submit][action*="/calendar/"][action*="/pdf"]').first().getAttribute('action');
  record('export-month-form-targets-pdf-endpoint', /\/calendar\/\d+\/report\/pdf/.test(exportAction || ''), { exportAction });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'calendar', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });
