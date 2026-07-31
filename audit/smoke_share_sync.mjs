/**
 * audit/smoke_share_sync.mjs — /share/sync surface probe
 * (issue #700 tier-2 follow-up).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. /share/sync renders the Google Integration panel
 *      with the 4 data-action button groups (Connect,
 *      Disconnect, Backup, Sheets Export, Calendar
 *      Use/Sync/Unsync, Test Use/Sync/Unsync).
 *   2. Each button carries the expected data-action URL
 *      (regression net for button-URL drift per #687).
 *   3. The Unsync button (destructive) carries
 *      data-confirm.
 *   4. The status region shows "Not connected" (the
 *      default state on a fresh archive without Google
 *      credentials).
 *   5. The DixieData Calendar ID + Test Calendar ID
 *      are rendered (the form surfaces them for the
 *      user to copy).
 *
 * Note: clicking the buttons fires Google OAuth flows
 * (out of scope for headless). Coverage for the Google
 * integration handlers lives in
 * internal/appshell/integrations_google_*.go + the
 * smoke_icalendar_microcopy probe.
 *
 * Run: `node audit/smoke_share_sync.mjs`
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

const EXPECTED_ACTIONS = [
  '/integrations/google/connect',
  '/integrations/google/disconnect',
  '/integrations/google/backup',
  '/integrations/google/sheets/export',
  '/integrations/google/calendar/use-managed',
  '/integrations/google/calendar/sync-managed',
  '/integrations/google/calendar/unsync-managed',
  '/integrations/google/calendar/use-test',
  '/integrations/google/calendar/sync-test',
  '/integrations/google/calendar/unsync-test',
];

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

  await page.goto(`${BASE}/share/sync`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  record('share-sync-renders-page', (await page.locator('h2:has-text("Share Sync")').count()) >= 1, {});

  let allActionsPresent = true;
  const actionDetails = {};
  for (const action of EXPECTED_ACTIONS) {
    const attr = await page.locator(`[data-action="${action}"]`).first().getAttribute('data-action').catch(() => null);
    const key = action.replace(/[^a-z0-9]+/gi, '-').toLowerCase();
    record(`share-sync-${key}-data-action-attr`, attr === action, { action, attr });
    if (attr !== action) allActionsPresent = false;
    actionDetails[action] = attr;
  }

  const unsyncConfirm = await page.locator('[data-action="/integrations/google/calendar/unsync-managed"][data-confirm]').first().getAttribute('data-confirm').catch(() => null);
  record('share-sync-unsync-managed-has-confirm', !!unsyncConfirm && unsyncConfirm.length > 0, { unsyncConfirm });

  const bodyText = await page.locator('body').innerText().catch(() => '');
  record('share-sync-default-state-not-connected', /Not connected/i.test(bodyText), {});
  record('share-sync-managed-calendar-id-renders', /DixieData Calendar ID/i.test(bodyText), {});
  record('share-sync-test-calendar-id-renders', /DixieData Test Calendar ID/i.test(bodyText), {});

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'share-sync', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });