// audit/smoke_fragment_redirect.mjs — regression net for the
// fragment-204 contract.
//
// Pins the client-side bridge that the fragment-204 contract relies
// on: any htmx request that returns 204 + X-DixieData-Redirect must
// trigger a full browser navigation to the redirect target. Without
// this listener, blocked states (pre-mux, setup-required, recovery,
// startupErr) leave the <body> empty and the user sees a white
// screen.
//
// Reproduces the exact path that shipped the original bug:
// 1. browser loads /index.html (the empty shell)
// 2. <body hx-get="/calendar" hx-trigger="load"> fires
// 3. server returns 204 + X-DixieData-Redirect: /setup
// 4. WITHOUT listener: body stays empty, white screen
//    WITH listener: window.location.assign("/setup") → full nav
//
// Run after dixiedata-web is up at $BASE_URL on a scratch dir where
// setupRequired is true (no .dixiedata/dixiedata.db):
//
//   scratch_dir=... node audit/smoke_fragment_redirect.mjs
//
// Exit code is non-zero when the assertion fails.

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

let pass = 0;
let fail = 0;
const results = [];

function record(name, ok, details = {}) {
  results.push({ name, ok, ...details });
  if (ok) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.log(`  ✗ ${name}`);
    console.log('    ', JSON.stringify(details, null, 2).replace(/\n/g, '\n     '));
  }
}

const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function run() {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  // Mock /index.html to return the empty body shell directly. This
  // bypasses the server's 303 redirect for / (which works as a full
  // browser nav) and forces the bug path: empty shell + htmx fires +
  // 204 response.
  await page.route(`${BASE}/index.html`, async (route) => {
    const html = `<!DOCTYPE html><html><head><title>DixieData</title>
      <link rel="stylesheet" href="${BASE}/app.css">
      <script src="${BASE}/htmx.min.js" defer></script>
      <script src="${BASE}/app.js" defer></script>
    </head><body hx-get="${BASE}/calendar" hx-trigger="load"></body></html>`;
    await route.fulfill({ status: 200, contentType: 'text/html', body: html });
  });

  const events = [];
  page.on('response', (r) => {
    if (r.url().startsWith(BASE)) {
      const xdr = r.headers()['x-dixiedata-redirect'];
      events.push({
        status: r.status(),
        method: r.request().method(),
        url: r.url(),
        xdr: xdr || null,
      });
    }
  });

  console.log('\nfragment-204 contract regression');
  console.log('--------------------------------');

  await page.goto(`${BASE}/index.html`, { waitUntil: 'domcontentloaded' });

  // Wait for the listener to fire + new page to render (max 6s).
  const deadline = Date.now() + 6000;
  while (Date.now() < deadline && page.url() === `${BASE}/index.html`) {
    await wait(150);
  }
  // Wait for the destination page's load event so document.body exists
  // and is populated.
  try {
    await page.waitForLoadState('domcontentloaded', { timeout: 5000 });
  } catch {}
  await wait(500);

  const finalUrl = page.url();
  let bodyInner = '';
  try {
    bodyInner = await page.evaluate(() => document.body.innerHTML);
  } catch {
    bodyInner = '(navigation in progress)';
  }

  record(
    'fragment-204-redirect-target',
    finalUrl === `${BASE}/setup`,
    { finalUrl, expected: `${BASE}/setup` },
  );
  record(
    'fragment-204-body-not-empty',
    bodyInner.length > 500,
    { bodyInnerLength: bodyInner.length },
  );

  // Pin the response shape: htmx's load-time GET /calendar must return
  // 204 + X-DixieData-Redirect: /setup. If this regresses (e.g. someone
  // changes blockIfFragment to return 200 + full HTML, or strips the
  // header), the listener would have nothing to follow.
  const htmxLoad = events.find(
    (e) => e.method === 'GET' && e.url === `${BASE}/calendar`,
  );
  record(
    'fragment-204-response-shape',
    htmxLoad && htmxLoad.status === 204 && htmxLoad.xdr === '/setup',
    { htmxLoad: htmxLoad || null },
  );

  await browser.close();

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail === 0 ? 0 : 1);
}

run().catch((e) => {
  console.error('FATAL', e);
  process.exit(2);
});