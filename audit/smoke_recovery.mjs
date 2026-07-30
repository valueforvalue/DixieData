/**
 * audit/smoke_recovery.mjs — /recovery surface probe
 * (issue #700 tier-3).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. GET /recovery when no pendingRecovery is set
 *      (the default state on a healthy archive) returns
 *      303 → /calendar -- the handleRecovery guard
 *      (`a.pendingRecovery == nil` returns http.Redirect
 *      to /calendar with StatusSeeOther).
 *   2. The /recovery page DOES render (200 with the
 *      Update Recovery markup) ONLY when the Wails app
 *      has activated a pending restore point. This probe
 *      does NOT exercise that path (would require
 *      triggering an actual failed update via
 *      `activatePendingRecovery` -- out of scope for
 *      an integration probe). Coverage for the
 *      restore-point rendering lives in
 *      internal/appshell/app_recovery_test.go and the
 *      server-side tests in internal/update.
 *
 * Note: when the pending-recovery state IS active in a
 * real install, /recovery renders the "Restore previous
 * build and Local Archive" button which POSTs to /recovery
 * and triggers the rollback script (PowerShell; runs
 * outside the probe process). The probe does NOT click
 * that button in either state.
 *
 * Run: `node audit/smoke_recovery.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8794';
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

  // GET /recovery on a healthy archive returns 303 -> /calendar.
  // The browser follows the redirect and lands on /calendar.
  await page.goto(`${BASE}/recovery`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  record('recovery-healthy-state-redirects-to-calendar', /\/calendar/.test(page.url()), { url: page.url() });

  // Negative assertion: /recovery did NOT render the recovery
  // markup (the heading "The last update did not finish a
  // healthy first launch." is only present when
  // a.pendingRecovery != nil).
  const recoveryMarkupPresent = await page.locator('text=The last update did not finish').count();
  record('recovery-healthy-state-does-not-render-markup', recoveryMarkupPresent === 0, { recoveryMarkupPresent });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'recovery', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });