// Repro for "Menu button in bottom right doesn't work on first load".
// Loads /calendar (or any page) fresh, asserts the floating nav toggle
// button has a click handler attached. If it does NOT, the bug
// reproduces — the early-return at app.js:2226 fired because
// document.querySelector("[data-floating-nav-toggle]") returned null.

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';
const URL = process.env.TARGET_URL || `${BASE}/calendar`;

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();

  const pageErrors = [];
  const consoleErrors = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));
  page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });

  // Load the target page fresh — no prior navigation.
  await page.goto(URL, { waitUntil: 'domcontentloaded' });
  // Give DOMContentLoaded handlers a beat.
  await page.waitForTimeout(300);

  // First assertion: was the toggle click handler attached at all?
  // Tap into the button: capture whether a click event fires any handler.
  const handlerState = await page.evaluate(() => {
    const btn = document.querySelector('[data-floating-nav-toggle]');
    if (!btn) return { exists: false };
    // Try clicking immediately after page load (before any prior nav).
    const panel = document.querySelector('[data-floating-nav-panel]');
    return {
      exists: true,
      tag: btn.tagName,
      panelExists: !!panel,
      panelHidden: panel ? panel.classList.contains('hidden') : null,
      // count any 'click' listeners attached via addEventListener — not directly
      // observable, but we can dispatch a click and see if the panel responds.
    };
  });
  console.log('pre-click state:', JSON.stringify(handlerState));

  // Confirm the toggle button + panel are actually in the DOM.
  const toggleCount = await page.locator('[data-floating-nav-toggle]').count();
  const panelCount = await page.locator('[data-floating-nav-panel]').count();

  // Click the toggle and see if the panel becomes visible (loses .hidden).
  const panelHiddenBefore = await page.locator('[data-floating-nav-panel]').evaluate((el) => el.classList.contains('hidden')).catch(() => null);

  await page.locator('[data-floating-nav-toggle]').click({ timeout: 3000 }).catch((e) => {
    console.error('click failed:', e.message);
  });

  await page.waitForTimeout(200);
  const panelHiddenAfter = await page.locator('[data-floating-nav-panel]').evaluate((el) => el.classList.contains('hidden')).catch(() => null);

  console.log(`URL:               ${URL}`);
  console.log(`toggle buttons:    ${toggleCount}`);
  console.log(`panels:            ${panelCount}`);
  console.log(`panel hidden before click: ${panelHiddenBefore}`);
  console.log(`panel hidden after  click: ${panelHiddenAfter}`);
  console.log(`console errors:    ${consoleErrors.length}`);
  consoleErrors.forEach((e) => console.log(`  - ${e}`));
  console.log(`page errors:       ${pageErrors.length}`);
  pageErrors.forEach((e) => console.log(`  - ${e}`));

  await browser.close();

  // The bug manifests as: toggle exists in DOM but click does nothing,
  // i.e. panel remains hidden after click.
  if (toggleCount === 0 || panelCount === 0) {
    console.log('FAIL: toggle or panel missing from DOM');
    process.exit(1);
  }
  if (panelHiddenBefore === true && panelHiddenAfter === true) {
    console.log('FAIL: bug reproduced — menu button click did not open the panel on first load');
    process.exit(1);
  }
  console.log('PASS: menu button works on first load');
}

main().catch((err) => {
  console.error('FATAL:', err);
  process.exit(2);
});