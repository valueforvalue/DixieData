// audit/smoke.mjs — live regression net for click-driven surfaces.
//
// Catches the entire class of bug where a button is rendered, looks
// clickable, but doesn't actually trigger a network request because of
// some mismatch between the templ/htmx/JS/handler stack. This file
// boots a real Chromium against the live dixiedata-web server and
// asserts that every button we ship causes the expected network round
// trip.
//
// Run after dixiedata-web is up at $BASE_URL:
//   node audit/smoke.mjs
//
// Exit code is non-zero when any smoke assertion fails.

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const results = [];
let pass = 0;
let fail = 0;

function record(name, ok, details = {}) {
  results.push({ name, ok, ...details });
  if (ok) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.log(`  ✗ ${name} — ${JSON.stringify(details)}`);
  }
}

async function clickAndWaitForRequest(page, buttonLocator, path, opts = {}) {
  // Click + wait for a network request to `path`. Returns the request
  // (or null on timeout). Some buttons are inside forms whose submit
  // triggers the request — we click the button directly so the form
  // submit handler in app.js picks it up.
  const reqPromise = page.waitForRequest(
    (req) => req.url().includes(path),
    { timeout: opts.timeout || 4000 }
  ).catch(() => null);
  try {
    await buttonLocator.click({ timeout: 1500 });
  } catch (e) {
    return null;
  }
  return await reqPromise;
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();

  page.on('pageerror', (err) => console.log(`  [pageerror] ${err.message}`));
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      // Skip the well-known SaveFileDialog 400 noise from web mode
      // — those are expected because the Wails bridge isn't present.
      if (text.includes('Response Status Error Code 400')) return;
      console.log(`  [console.error] ${text}`);
    }
  });

  // ────────────────────────────────────────────────────────────────────
  // Quick search (top-nav /soldiers page)
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[1] Quick search');
  await page.goto(`${BASE}/soldiers`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);

  const searchInput = page.locator('input[name="q"]');
  await searchInput.waitFor({ timeout: 5000 });
  const inputExists = await searchInput.count();

  const listBefore = await page.evaluate(() => {
    const el = document.querySelector('#soldier-list');
    return el ? el.innerHTML.length : -1;
  });

  const searchReqPromise = page.waitForResponse(
    (resp) => resp.url().includes('/soldiers/search') && resp.status() === 200,
    { timeout: 4000 }
  ).catch(() => null);
  await searchInput.fill('Robert');
  const searchResp = await searchReqPromise;
  await page.waitForTimeout(300);

  const listAfter = await page.evaluate(() => {
    const el = document.querySelector('#soldier-list');
    return el ? el.innerHTML.length : -1;
  });

  record('search-input-exists', inputExists === 1);
  record('search-fires-request', !!searchResp, { status: searchResp?.status() });
  record('search-results-update', listAfter !== listBefore && listAfter > 0, {
    listBefore,
    listAfter,
  });

  // ────────────────────────────────────────────────────────────────────
  // Browse Alphabetically button (soldier_card.templ)
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[2] Browse Alphabetically button');
  await page.goto(`${BASE}/soldiers`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);

  const browseAlphBtn = page.locator('button', { hasText: 'Browse Alphabetically' });
  const browseAlphExists = await browseAlphBtn.count();

  const browseListBefore = await page.evaluate(() => {
    const el = document.querySelector('#soldier-list');
    return el ? el.innerHTML.length : -1;
  });

  const browseResp = await clickAndWaitForRequest(page, browseAlphBtn, '/soldiers/search');
  await page.waitForTimeout(300);
  const browseListAfter = await page.evaluate(() => {
    const el = document.querySelector('#soldier-list');
    return el ? el.innerHTML.length : -1;
  });

  record('browse-alphabetically-button-exists', browseAlphExists === 1);
  record('browse-alphabetically-fires-request', !!browseResp, {
    method: browseResp?.method(),
    url: browseResp?.url(),
  });
  record('browse-alphabetically-swaps-content', browseListAfter !== browseListBefore, {
    before: browseListBefore,
    after: browseListAfter,
  });

  // ────────────────────────────────────────────────────────────────────
  // Browse filter form auto-submit on change
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[3] Browse filter form auto-submit');
  await page.goto(`${BASE}/browse`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);

  await page.evaluate(() => {
    const d = document.querySelector('[data-browse-filters-details]');
    if (d) d.open = true;
  });
  await page.waitForTimeout(200);

  const resultsBefore = await page.evaluate(() => {
    const el = document.querySelector('#browse-results');
    return el ? el.innerHTML.length : -1;
  });

  const browseFilterResp = await clickAndWaitForRequest(
    page,
    page.locator('form#browse-filters select[name="entry_type"]').first(),
    '/browse/results'
  );
  // The click handler change-event triggers a hx-get on the form.
  // selectOption fires a change event, so use that pattern instead.
  if (!browseFilterResp) {
    const filterReqPromise = page.waitForRequest(
      (req) => req.url().includes('/browse/results'),
      { timeout: 4000 }
    ).catch(() => null);
    await page.selectOption('form#browse-filters select[name="entry_type"]', 'widow');
    const fallback = await filterReqPromise;
    await page.waitForTimeout(300);
    record('browse-filter-fires-request', !!fallback, { method: fallback?.method() });
  } else {
    await page.waitForTimeout(300);
    record('browse-filter-fires-request', true, { method: browseFilterResp?.method() });
  }

  const resultsAfter = await page.evaluate(() => {
    const el = document.querySelector('#browse-results');
    return el ? el.innerHTML.length : -1;
  });
  record('browse-filter-swaps-results', resultsAfter !== resultsBefore, {
    before: resultsBefore,
    after: resultsAfter,
  });

  // ────────────────────────────────────────────────────────────────────
  // Insights → Export Analytics Report
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[4] Insights Export Analytics Report');
  await page.goto(`${BASE}/insights`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);

  const insightsBtn = page.locator('button', { hasText: 'Export Analytics Report' }).first();
  const insightsExists = await insightsBtn.count();
  const insightsResp = insightsExists > 0
    ? await clickAndWaitForRequest(page, insightsBtn, '/insights/report/pdf')
    : null;
  record('insights-export-button-exists', insightsExists > 0);
  record('insights-export-fires-request', !!insightsResp, {
    method: insightsResp?.method(),
    url: insightsResp?.url(),
  });

  // ────────────────────────────────────────────────────────────────────
  // Share page export buttons (correct labels)
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[5] Share page export buttons');
  const shareButtons = [
    { label: /^Export JSON/i, path: '/export/json' },
    { label: /^Export Excel/i, path: '/export/csv' },
    { label: /^Export iCalendar/i, path: '/export/ical' },
    { label: /^Export Static Web Archive/i, path: '/export/static-archive' },
    { label: /^Export Backup/i, path: '/export/backup' },
    { label: /^Export Shared Archive/i, path: '/export/shared-archive' },
  ];

  for (const btn of shareButtons) {
    await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(300);
    const loc = page.locator('button', { hasText: btn.label }).first();
    const exists = await loc.count();
    const req = exists > 0
      ? await clickAndWaitForRequest(page, loc, btn.path)
      : null;
    record(`share-${btn.path}-button-exists`, exists > 0);
    record(`share-${btn.path}-fires-request`, !!req, {
      method: req?.method(),
      url: req?.url(),
    });
  }

  // ────────────────────────────────────────────────────────────────────
  // Settings → Debug Mode toggle (enables debug console)
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[6] Settings Debug Mode toggle');
  await page.goto(`${BASE}/settings`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);

  // Debug Mode form is the one with the 🐞 emoji label. Find by the
  // form's hx-post / data-hx-post attribute.
  const debugForm = page.locator('form:has(input[name="debug_mode"])').first();
  const debugFormExists = await debugForm.count();
  // Check the checkbox first — the handler reads the checkbox value
  // and toggles accordingly. Clicking Apply without checking leaves
  // debug mode off.
  if (debugFormExists > 0) {
    const checkbox = debugForm.locator('input[name="debug_mode"]').first();
    const isChecked = await checkbox.isChecked();
    if (!isChecked) {
      await checkbox.check();
    }
  }
  const debugReq = debugFormExists > 0
    ? await clickAndWaitForRequest(page, debugForm.locator('button[type="submit"]'), '/settings/debug-mode')
    : null;
  record('debug-mode-form-exists', debugFormExists > 0);
  record('debug-mode-toggle-fires-request', !!debugReq, {
    method: debugReq?.method(),
    url: debugReq?.url(),
  });
  await page.waitForTimeout(500);

  // ────────────────────────────────────────────────────────────────────
  // Debug Console page (now that debug mode is on)
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[7] Debug Console page');
  const debugResp = await page.goto(`${BASE}/debug/console`, { waitUntil: 'domcontentloaded' }).catch((e) => ({ error: e.message }));
  const debugStatus = debugResp?.status?.();
  const debugBody = await page.content();
  record('debug-console-page-loads', debugStatus === 200 && debugBody.includes('Debug Console'), {
    status: debugStatus,
    bodyContainsTitle: debugBody.includes('Debug Console'),
  });

  // ────────────────────────────────────────────────────────────────────
  // Summary
  // ────────────────────────────────────────────────────────────────────
  console.log(`\n${'='.repeat(60)}`);
  console.log(`SMOKE RESULTS: ${pass} pass, ${fail} fail`);
  console.log('='.repeat(60));

  if (fail > 0) {
    console.log('\nFailures:');
    for (const r of results.filter((r) => !r.ok)) {
      console.log(`  - ${r.name}: ${JSON.stringify(r)}`);
    }
  }

  await browser.close();
  process.exit(fail > 0 ? 1 : 0);
}

main().catch((err) => {
  console.error('Smoke test crashed:', err);
  process.exit(2);
});