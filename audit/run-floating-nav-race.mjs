// Regression test for "Menu button doesn't work on first load".
//
// Race condition hypothesis: the user can click the Menu button
// BEFORE app.js's DOMContentLoaded handler runs initializeFloatingNav().
// The current implementation only attaches the click handler inside
// that init function, so a click during the gap is silently dropped.
//
// This test reproduces the race by clicking the Menu button at various
// points AFTER the initial HTML is parsed, simulating a user that
// clicks fast. The fix is to use event delegation on document OR an
// inline onclick, so the handler is bound when the button is parsed
// (not when app.js finishes loading).

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';
const URL = process.env.TARGET_URL || `${BASE}/calendar`;

async function clickMenuAtTiming(page, delayMs) {
  await page.goto(URL, { waitUntil: 'commit' }); // network response received, DOM may not be fully parsed
  // Wait only for the toggle to exist in DOM (parse done), but NOT
  // for app.js's DOMContentLoaded handler to finish. We do this by
  // polling for the element with a very short timeout, then clicking
  // immediately.
  await page.locator('[data-floating-nav-toggle]').waitFor({ state: 'attached', timeout: 5000 });
  if (delayMs > 0) {
    await page.waitForTimeout(delayMs);
  }
  await page.locator('[data-floating-nav-toggle]').click({ timeout: 3000 });
  await page.waitForTimeout(200);
  const panelHidden = await page.locator('[data-floating-nav-panel]').evaluate((el) => el.classList.contains('hidden')).catch(() => null);
  return panelHidden;
}

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();

  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));
  page.on('console', (msg) => { if (msg.type() === 'error') errors.push(`console: ${msg.text()}`); });

  // Click immediately after toggle appears (simulates fast user).
  console.log('--- Click timing: 0ms after toggle appears ---');
  const immediate = await clickMenuAtTiming(page, 0);
  console.log(`panel hidden after click: ${immediate}`);

  // Click after a generous delay (simulates normal user).
  console.log('--- Click timing: 1000ms after toggle appears ---');
  const delayed = await clickMenuAtTiming(page, 1000);
  console.log(`panel hidden after click: ${delayed}`);

  // Click AFTER the page fully loads, BEFORE app.js has had a chance
  // to run initializeFloatingNav. Hardest case: simulates the user
  // clicking the rendered button in the tiny window between HTML
  // parse completion and app.js's DOMContentLoaded handler.
  console.log('--- Click timing: 50ms after toggle appears (race window) ---');
  const race = await clickMenuAtTiming(page, 50);
  console.log(`panel hidden after click: ${race}`);

  // Toggle back closed to test outside-click handler doesn't break
  // things.
  console.log('--- Click timing: 500ms after toggle appears (open + close) ---');
  const cycle = await clickMenuAtTiming(page, 500);
  console.log(`panel hidden after click 1: ${cycle}`);
  await page.locator('[data-floating-nav-toggle]').click({ timeout: 3000 });
  await page.waitForTimeout(200);
  const cycleAfter2 = await page.locator('[data-floating-nav-panel]').evaluate((el) => el.classList.contains('hidden')).catch(() => null);
  console.log(`panel hidden after click 2: ${cycleAfter2}`);

  await browser.close();

  console.log(`page errors: ${errors.length}`);
  errors.forEach((e) => console.log(`  - ${e}`));

  // Pass criteria: all three timings open the panel, the cycle test
  // opens then closes.
  const allImmediate = immediate === false;
  const allDelayed = delayed === false;
  const allRace = race === false;
  const cycleOk = cycle === false && cycleAfter2 === true;

  if (!allImmediate || !allDelayed || !allRace || !cycleOk) {
    console.log(`FAIL: timings — immediate=${immediate} delayed=${delayed} race=${race} cycle(open,close)=${cycle},${cycleAfter2}`);
    process.exit(1);
  }
  console.log('PASS: menu button works at all timings and toggles correctly');
}

main().catch((err) => {
  console.error('FATAL:', err);
  process.exit(2);
});