/**
 * audit/smoke_share_imports.mjs — /share/imports surface probe
 * (issue #700 tier-2 follow-up).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. /share/imports renders the imports panel
 *      (id=panel.share.imports) with the 3 import modes
 *      (Collaborative Merge / Memorial JSON / Replace
 *      Local Archive).
 *   2. The Collaborative Merge button carries
 *      data-action="/import/shared-archive".
 *   3. The Memorial JSON button carries
 *      data-action="/import/memorial-json".
 *   4. The Replace Local Archive button carries
 *      data-action="/import/backup" + data-confirm.
 *
 * Note: clicking the import buttons triggers a native
 * file-picker dialog (out of scope for headless). Coverage
 * for the individual import handlers lives in
 * internal/appshell/imports_handlers_test.go + the
 * smoke_static_archive_revamp.mjs probes.
 *
 * Run: `node audit/smoke_share_imports.mjs`
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

  await page.goto(`${BASE}/share/imports`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const panel = await page.locator('[id="panel.share.imports"]').count();
  record('share-imports-renders-panel', panel >= 1, { panel });

  const sharedArchiveAction = await page.locator('[data-action="/import/shared-archive"][data-dixie-submit="true"]').first().getAttribute('data-action').catch(() => null);
  record('share-imports-shared-archive-button-data-action-attr', sharedArchiveAction === '/import/shared-archive', { sharedArchiveAction });

  const memorialJsonAction = await page.locator('[data-action="/import/memorial-json"][data-dixie-submit="true"]').first().getAttribute('data-action').catch(() => null);
  record('share-imports-memorial-json-button-data-action-attr', memorialJsonAction === '/import/memorial-json', { memorialJsonAction });

  const backupAction = await page.locator('[data-action="/import/backup"][data-dixie-submit="true"]').first().getAttribute('data-action').catch(() => null);
  record('share-imports-backup-button-data-action-attr', backupAction === '/import/backup', { backupAction });
  const backupConfirm = await page.locator('[data-action="/import/backup"][data-dixie-submit="true"]').first().getAttribute('data-confirm').catch(() => null);
  record('share-imports-backup-button-has-confirm', !!backupConfirm && backupConfirm.length > 0, { backupConfirm });

  record('share-imports-memorial-preview-target-renders', (await page.locator('#memorial-preview-target').count()) >= 1, {});

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'share-imports', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });