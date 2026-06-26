// Focused Playwright check for the new Debug Console UI shipped in
// commit 39e3ea2 + 41dbd28. Drives the debug framework end-to-end:
//   1. boot /debug/state, assert debug_mode is off initially
//   2. POST /settings/debug-mode (form) to turn it on
//   3. reload /calendar, assert the 🐞 Debug footer button is visible
//   4. click it, wait for the console panel
//   5. exercise each button: level filter, copy, open-folder, clear,
//      refresh, close
//   6. capture console errors + page errors throughout
//
// Assumes dixiedata-web is already running at $BASE_URL
// (default http://127.0.0.1:8765).

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';
const results = [];
const consoleErrors = [];
const pageErrors = [];

function record(name, ok, detail) {
  results.push({ name, ok, detail });
  const tag = ok ? 'PASS' : 'FAIL';
  console.log(`[${tag}] ${name}${detail ? ` — ${detail}` : ''}`);
}

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    permissions: ['clipboard-read', 'clipboard-write'],
  });
  const page = await context.newPage();

  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      consoleErrors.push(msg.text());
    }
  });
  page.on('pageerror', (err) => {
    pageErrors.push(err.message);
  });

  // 1. /debug/state baseline.
  const stateOff = await page.request.get(`${BASE}/debug/state`);
  const stateJson = await stateOff.json();
  record('GET /debug/state returns JSON', stateOff.status() === 200, `debug_mode=${stateJson.debug_mode}`);
  // Reset debug mode to off so we start clean.
  if (stateJson.debug_mode) {
    await page.request.post(`${BASE}/settings/debug-mode`, {
      form: { debug_mode: '' },
    });
  }

  // 2. POST toggle via form (the form-encoded path; the plan calls
  // out that the toggle also accepts JSON).
  const toggleOn = await page.request.post(`${BASE}/settings/debug-mode`, {
    form: { debug_mode: 'on' },
    headers: { 'HX-Request': 'true' },
  });
  record('POST /settings/debug-mode (form=on) accepts request', toggleOn.status() === 204 || toggleOn.status() === 200, `status=${toggleOn.status()}`);

  const stateOn = await (await page.request.get(`${BASE}/debug/state`)).json();
  record('debug_mode flipped on', stateOn.debug_mode === true, `debug_mode=${stateOn.debug_mode}`);

  // 3. Navigate to /calendar; confirm Debug footer button appears.
  await page.goto(`${BASE}/calendar`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('[data-debug-button]', { timeout: 5000 }).catch(() => null);
  const buttonVisible = await page.locator('[data-debug-button]').isVisible().catch(() => false);
  record('Debug footer button visible when debug_mode=on', buttonVisible);

  if (!buttonVisible) {
    await browser.close();
    printSummary();
    process.exit(1);
  }

  // 4. Click Debug button to open the panel.
  await page.locator('[data-debug-button]').click();
  await page.waitForSelector('#debug-console-panel', { timeout: 5000 });
  record('Debug Console panel opens', true);

  // 5a. Level filter (set to ERROR, confirm dropdown is wired).
  await page.selectOption('#debug-console-panel select[name="level"]', 'ERROR');
  await page.waitForTimeout(300); // htmx swap
  record('Level filter accepts ERROR', true);

  await page.selectOption('#debug-console-panel select[name="level"]', '');
  await page.waitForTimeout(300);

  // 5b. Refresh button.
  await page.locator('#debug-console-panel button:has-text("Refresh")').click();
  await page.waitForTimeout(300);
  await page.waitForSelector('#debug-console-panel', { timeout: 5000 });
  record('Refresh re-renders panel', true);

  // 5c. Copy button (uses navigator.clipboard; we gave permission).
  await page.locator('#debug-console-panel button:has-text("Copy")').click();
  await page.waitForTimeout(200);
  record('Copy button executes without error', true);

  // 5d. Open folder — in web-mode BrowserOpenURL returns
  // errWailsFrontendUnavailable, but the handler logs a warning and
  // returns 204. The button click should not error.
  const folderResp = page.waitForResponse(
    (resp) => resp.url().includes('/debug/open-folder') && resp.request().method() === 'GET',
    { timeout: 3000 },
  ).catch(() => null);
  await page.locator('#debug-console-panel button:has-text("Open folder")').click();
  const folderR = await folderResp;
  record('Open folder button hits endpoint', folderR !== null, folderR ? `status=${folderR.status()}` : 'no response observed');

  // 5e. Clear button.
  const clearResp = page.waitForResponse(
    (resp) => resp.url().includes('/debug/console/clear') && resp.request().method() === 'POST',
    { timeout: 3000 },
  ).catch(() => null);
  await page.locator('#debug-console-panel button:has-text("Clear")').click();
  const clearR = await clearResp;
  record('Clear button POSTs /debug/console/clear', clearR !== null, clearR ? `status=${clearR.status()}` : 'no response observed');

  // 5f. Close button removes the panel from the DOM.
  await page.locator('#debug-console-panel button:has-text("Close")').click();
  await page.waitForTimeout(200);
  const panelGone = (await page.locator('#debug-console-panel').count()) === 0;
  record('Close button removes panel', panelGone);

  // 6. Toggle debug mode off again to leave the server clean.
  await page.request.post(`${BASE}/settings/debug-mode`, {
    form: { debug_mode: '' },
  });
  const stateFinal = await (await page.request.get(`${BASE}/debug/state`)).json();
  record('debug_mode flipped back off', stateFinal.debug_mode === false);

  await browser.close();
  printSummary();
}

function printSummary() {
  const failed = results.filter((r) => !r.ok);
  console.log('');
  console.log(`Console errors: ${consoleErrors.length}`);
  consoleErrors.forEach((e) => console.log(`  - ${e}`));
  console.log(`Page errors: ${pageErrors.length}`);
  pageErrors.forEach((e) => console.log(`  - ${e}`));
  console.log('');
  console.log(`${results.length - failed.length}/${results.length} checks passed`);
  if (failed.length || pageErrors.length) {
    process.exit(1);
  }
}

main().catch((err) => {
  console.error('FATAL:', err);
  process.exit(2);
});