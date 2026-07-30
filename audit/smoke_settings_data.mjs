/**
 * audit/smoke_settings_data.mjs — /settings/data surface probe
 * (issue #700 tier-2).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 3 soldiers, then asserts:
 *   1. /settings/data renders the data panel with the Initialize
 *      Local Archive form (destructive; requires typed
 *      confirmation word).
 *   2. The init form action targets /settings/initialize.
 *   3. The confirmation_word input renders and is marked
 *      required (browser-level gate; server is the source of
 *      truth).
 *   4. Submitting with the WRONG confirmation word leaves the
 *      local archive intact (the seeded soldier list still
 *      renders -- the form's data-reload-on-success triggers
 *      a reload that re-renders the form, swallowing the
 *      cancellation body text).
 *   5. Submitting with the CORRECT confirmation word wipes
 *      the archive (verified via /browse showing zero rows
 *      after the navigation) and navigates away from
 *      /settings/data (typically to /calendar per the
 *      X-DixieData-Redirect header; the fresh database then
 *      triggers a /setup redirect via App.ServeHTTP because
 *      no setup has been run on the empty archive).
 *
 * Note: the wrong-confirmation path does NOT render a visible
 * "cancelled" toast -- the form has data-reload-on-success=true
 * which the dispatcher honors when responseOk && !redirectTo,
 * reloading the page (and discarding the cancellation body).
 * The server still rejects the wipe (archive intact), which
 * is the observable side effect we assert on.
 *
 * Note: the data dir for this probe must end in `/.dixiedata`
 * -- initializeLocalData refuses to operate on a path whose
 * base is anything else (safety check against accidental
 * wipes). The probe nests a `.dixiedata` child inside the
 * runner-provided scratchDir to satisfy the check.
 *
 * Run: `node audit/smoke_settings_data.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8787';
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
  // initializeLocalData refuses to operate unless the data dir
  // is named `.dixiedata` (its base-dir safety check). The
  // runner-provided scratchDir is a per-probe mkdtemp under
  // os.tmpdir(); nest a `.dixiedata` child inside it so the
  // init handler accepts the path.
  const SCRATCH = ctx.scratchDir;
  const DATA_DIR = `${SCRATCH}/.dixiedata`;
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', DATA_DIR, '-soldiers', '3', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let seedOut = '';
  seedProc.stdout.on('data', (d) => { seedOut += d; });
  seedProc.stderr.on('data', (d) => { seedOut += d; });
  const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
  if (seedExit !== 0) throw new Error(`seed-data failed (${seedExit}):\n${seedOut}`);

  const server = spawn(WEB_BIN_PATH, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', DATA_DIR], { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, DIXIEDATA_DATA_DIR: DATA_DIR } });
  let serverErr = '';
  server.stderr.on('data', (d) => { serverErr += d; });
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
    if (/\/settings\/initialize/.test(r.url())) {
      console.log('  [diag] initialize response status=', r.status(), 'redirect=', r.headers()['x-dixiedata-redirect'] || r.headers()['location']);
    }
  });

  await page.goto(`${BASE}/settings/data`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const pageRoot = await page.locator('[id="page.settings.data"]').count();
  record('settings-data-renders-page', pageRoot >= 1, { pageRoot });

  const initAction = await page.locator('form[data-dixie-submit][action="/settings/initialize"]').first().getAttribute('action').catch(() => null);
  record('settings-data-init-form-targets-endpoint', initAction === '/settings/initialize', { initAction });

  const confirmInput = page.locator('input[name="confirmation_word"]').first();
  record('settings-data-confirmation-word-input-renders', (await page.locator('input[name="confirmation_word"]').count()) === 1, {});
  record('settings-data-confirmation-word-input-is-required', await confirmInput.getAttribute('required') !== null || await confirmInput.getAttribute('aria-required') === 'true', {});

  // Path A: wrong confirmation word keeps the archive intact.
  // The form has data-reload-on-success=true; the dispatcher
  // reloads after a 200 with no redirect header, swallowing the
  // cancellation body text. We assert the SIDE EFFECT (archive
  // intact) instead of the transient toast text.
  await confirmInput.fill('wrongword');
  await Promise.all([
    page.waitForResponse((r) => /\/settings\/initialize/.test(r.url()) && r.request().method() === 'POST', { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action="/settings/initialize"] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  // Wait for the data-reload-on-success page reload to settle.
  await page.waitForLoadState('domcontentloaded').catch(() => null);
  await new Promise((r) => setTimeout(r, 400));

  // Verify the archive is intact: /browse shows all seeded
  // soldiers as cards (the /soldiers landing lazy-loads via
  // htmx search so an empty list there is normal, even with
  // seeded data).
  await page.goto(`${BASE}/browse`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const soldierRowsAfterCancel = await page.locator('a[href^="/soldiers/"]:not([href$="/new"]):not([href$="/edit"])').count();
  record('settings-data-wrong-confirmation-keeps-archive-intact', soldierRowsAfterCancel >= 3, { soldierRowsAfterCancel });

  // Path B: correct confirmation word wipes the archive and
  // redirects to /calendar. WaitForURL fires when URL matches;
  // also wait briefly so the post-navigation page settles
  // before we sample page.url().
  await page.goto(`${BASE}/settings/data`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  await page.locator('input[name="confirmation_word"]').fill('INITIALIZE');
  await Promise.all([
    page.waitForURL(/\/calendar/, { timeout: 15_000 }).catch(() => null),
    page.locator('form[data-dixie-submit][action="/settings/initialize"] button[type="submit"]').first().click({ timeout: 5_000 }),
  ]);
  await new Promise((r) => setTimeout(r, 600));
  // After init the database is fresh (no users, no soldiers);
  // App.ServeHTTP intercepts the /calendar GET and redirects
  // to /setup (setupRequired=true). The dispatcher honors the
  // X-DixieData-Redirect: /calendar header by navigating to
  // /calendar, but the server then bounces to /setup. Either
  // destination is a valid post-init landing -- the observable
  // contract is "navigated away from /settings/data".
  const postInitUrl = page.url();
  const initRedirected = !/\/settings\/data/.test(postInitUrl);
  record('settings-data-correct-confirmation-navigates-away', initRedirected, { url: postInitUrl });

  // After init the archive should be wiped -- browse list empty.
  await page.goto(`${BASE}/browse`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const soldierRowsAfterInit = await page.locator('a[href^="/soldiers/"]:not([href$="/new"]):not([href$="/edit"])').count();
  record('settings-data-correct-confirmation-wipes-archive', soldierRowsAfterInit === 0, { soldierRowsAfterInit });

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    if (serverErr) console.log(`\n--- server stderr tail ---\n${serverErr.split('\n').slice(-30).join('\n')}`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'settings-data', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });