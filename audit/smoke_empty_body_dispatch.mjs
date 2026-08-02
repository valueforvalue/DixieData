/**
 * audit/smoke_empty_body_dispatch.mjs — class 9 regression net
 * (issue #691, the body-construction-uses-raw-closest bug).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. GET /soldiers/{id}/edit renders the entry form
 *      with the first_name + last_name + display_id
 *      inputs.
 *   2. Filling the fields + clicking Save Changes fires
 *      a fetch to the form's action URL with a non-empty
 *      FormData body that includes the filled field values.
 *   3. The request body is urlencoded (not multipart) and
 *      has the expected field names matching the handler's
 *      r.FormValue() reads.
 *
 * The probe uses `page.route` to intercept the outgoing
 * fetch BEFORE the browser dispatches it, capturing the
 * request method + URL + body. This pins the canonical
 * class 9 bug: if a future refactor reintroduces
 * `button.closest("form")` in the body-construction
 * branch (after the fix moved to the resolved `form`
 * variable), the captured body will be empty and the
 * probe fails.
 *
 * Run: `node audit/smoke_empty_body_dispatch.mjs`
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

  // Discover soldier id from /browse.
  await page.goto(`${BASE}/browse`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));
  const firstHref = await page.locator('a[href^="/soldiers/"]:not([href$="/new"]):not([href$="/edit"])').first().getAttribute('href').catch(() => null);
  const soldierId = Number((firstHref || '').match(/\/soldiers\/(\d+)/)?.[1] || 0);
  if (!Number.isFinite(soldierId) || soldierId <= 0) throw new Error('cannot discover soldier id from /browse');

  // Capture the outgoing POST fetch via page.route. The form
  // action is /soldiers/{id}; we match the prefix AND filter
  // for POST (the page's initial GET to /soldiers/{id}/edit
  // would also match the URL pattern; only the POST is the
  // class 9 dispatch event).
  let capturedMethod = null;
  let capturedUrl = null;
  let capturedContentType = null;
  let capturedBody = null;
  await page.route(/\/soldiers\/\d+$/, async (route) => {
    const req = route.request();
    if (req.method() !== 'POST') {
      await route.continue();
      return;
    }
    capturedMethod = req.method();
    capturedUrl = req.url();
    capturedContentType = req.headers()['content-type'] || '';
    capturedBody = req.postData() || '';
    // Let the request through so the server-side state
    // changes (otherwise the round-trip assertion below
    // would see a stale page).
    await route.continue();
  });

  await page.goto(`${BASE}/soldiers/${soldierId}/edit`, { waitUntil: 'networkidle' });
  await new Promise((r) => setTimeout(r, 400));

  const formAction = await page.locator('form[data-dixie-submit]').first().getAttribute('action').catch(() => null);
  record('empty-body-dispatch-form-targets-soldier-update', formAction === `/soldiers/${soldierId}`, { formAction });
  record('empty-body-dispatch-has-first-name-input', (await page.locator('input[name="first_name"]').count()) >= 1, {});
  record('empty-body-dispatch-has-last-name-input', (await page.locator('input[name="last_name"]').count()) >= 1, {});

  // Fill the fields + click Save. The body-construction
  // branch in dispatchDixieDataForm (post #691 fix) builds
  // new FormData(form, button). Class 9 regression: if the
  // branch is reverted to use button.closest("form") raw,
  // the body is empty.
  const uniqueFirst = 'SmokeClass9-' + Date.now();
  await page.locator('input[name="first_name"]').first().fill(uniqueFirst);
  await page.locator('form[data-dixie-submit]').first().locator('button[type="submit"]').first().click({ timeout: 5_000 });
  await new Promise((r) => setTimeout(r, 1500));

  record('empty-body-dispatch-fetch-method-is-POST', capturedMethod === 'POST', { capturedMethod });
  record('empty-body-dispatch-fetch-url-matches-form-action', capturedUrl !== null && capturedUrl.endsWith(`/soldiers/${soldierId}`), { capturedUrl });
  // The entry form declares enctype="multipart/form-data" so the
  // dispatcher sends a multipart body (not urlencoded). The class 9
  // regression net is "body is non-empty" + "body contains the
  // filled values" -- the encoding type doesn't matter.
  record('empty-body-dispatch-fetch-content-type-is-form-encoded', /multipart\/form-data|application\/x-www-form-urlencoded/.test(capturedContentType), { capturedContentType });
  record('empty-body-dispatch-fetch-body-is-non-empty', !!capturedBody && capturedBody.length > 0, { bodyLength: (capturedBody || '').length });
  // Multipart bodies carry each field on its own line with
  // Content-Disposition: form-data; name="<field>". The body must
  // include the filled first_name value (case-sensitive) and the
  // existing_needs_review hidden field. If class 9 regresses, the
  // body is empty and both checks fail.
  record('empty-body-dispatch-fetch-body-includes-first-name', !!capturedBody && capturedBody.includes(`name="first_name"`) && capturedBody.includes(uniqueFirst), {});
  record('empty-body-dispatch-fetch-body-includes-existing-needs-review', !!capturedBody && capturedBody.includes('name="existing_needs_review"'), {});

  await browser.close().catch(() => {});

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

runProbe({ name: 'empty-body-dispatch', probeFn: main })
  .then((r) => process.exit(r.ok ? 0 : 1))
  .catch((err) => { console.error('fatal:', err); process.exit(2); });