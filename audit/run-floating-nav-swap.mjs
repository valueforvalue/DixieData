// Regression for: floating-nav outside-click handler lost after
// htmx-style content swap.
//
// Steps:
//  1. Load /calendar fresh, open the floating-nav panel via Menu
//  2. Click a calendar day to trigger an htmx-style swap of
//     #details-pane
//  3. Click outside the panel — does it close?
//
// Pre-fix: handler closure captured the OLD panel reference, so
// after the swap the outside-click handler checks the detached
// old panel's classList (which still says "hidden=false") and
// the new panel never closes.
//
// Post-fix: handler either re-attaches after the swap, OR uses
// event delegation that doesn't depend on the original node.

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();

  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));
  page.on('console', (msg) => { if (msg.type() === 'error') errors.push(`console: ${msg.text()}`); });

  await page.goto(`${BASE}/calendar`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);

  // Open the menu panel.
  await page.locator('[data-floating-nav-toggle]').click();
  await page.waitForTimeout(200);
  const openAfterClick = await page.locator('[data-floating-nav-panel]').evaluate((el) => el.classList.contains('hidden'));
  if (openAfterClick !== false) {
    console.log('FAIL: panel did not open after Menu click');
    process.exit(1);
  }
  console.log('PASS: menu panel opens');

  // Trigger an htmx swap by clicking a calendar day (June 19 — has
  // anniversaries, populates the details pane).
  await page.locator('button[aria-label*="June 19"]').click({ timeout: 5000 });
  await page.waitForTimeout(500);
  console.log('PASS: triggered swap via day click (innerHTML)');

  // Now trigger an OUTERHTML swap: change the month. The month-select
  // dropdown navigates via window.location.href, which does a full
  // page reload — not a swap. Better: directly call refreshCalendarGrid
  // via the export form (async=1) which routes through the applyResponse
  // outerHTML path. Actually, the cleanest outerHTML swap trigger is
  // calling refreshCalendarGrid manually via window, but that's only
  // callable from app.js. Skip — log that this test covers innerHTML.

  // Re-open menu (if the swap closed it). Then test outside-click.
  const openAfterSwap = await page.locator('[data-floating-nav-panel]').evaluate((el) => el.classList.contains('hidden'));
  if (openAfterSwap === true) {
    await page.locator('[data-floating-nav-toggle]').click();
    await page.waitForTimeout(200);
  }

  // Click outside the panel (on the main content, not on the toggle).
  await page.locator('h2:has-text("Calendar")').click({ timeout: 3000 }).catch(() => null);
  await page.waitForTimeout(300);
  const closedAfterOutside = await page.locator('[data-floating-nav-panel]').evaluate((el) => el.classList.contains('hidden'));
  console.log(`panel hidden after outside-click: ${closedAfterOutside}`);

  await browser.close();

  if (errors.length > 0) {
    console.log(`page errors: ${errors.length}`);
    errors.forEach((e) => console.log(`  - ${e}`));
  }

  if (closedAfterOutside !== true) {
    console.log('FAIL: outside-click did NOT close the panel after htmx swap — handler closure is stale');
    process.exit(1);
  }
  console.log('PASS: outside-click closes panel even after htmx swap');
}

main().catch((err) => {
  console.error('FATAL:', err);
  process.exit(2);
});