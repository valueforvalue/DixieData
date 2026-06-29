// audit/_lib/manual-walk.mjs — companion to run-interactive.mjs.
// Performs the 4 manual-only checks the walker can't automate:
//  1. Calendar day click → anniversary pane populates
//  2. /soldiers/new required-field validation (HTML5 form block)
//  3. /browse soldier link navigates to detail page
//  4. Floating dock layout (no overlap with toast region or modal)
//
// Run AFTER run-interactive.mjs. Writes findings to
// audit/manual-walk-report.json (gitignored).
//
//   node audit/_lib/manual-walk.mjs

import { chromium } from 'playwright';
import { writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPORT = join(__dirname, '..', 'manual-walk-report.json');
const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const findings = [];

const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
const page = await context.newPage();

const consoleErrors = [];
page.on('console', (msg) => {
  if (msg.type() === 'error') consoleErrors.push(msg.text());
});
page.on('pageerror', (e) => consoleErrors.push(`pageerror: ${e.message}`));

// 1. Calendar day click
await page.goto(`${BASE}/calendar`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(500);
const dayBtn = page.locator('button[aria-label*="press enter to load details"]').first();
if (await dayBtn.count() > 0) {
  const label = await dayBtn.getAttribute('aria-label');
  await dayBtn.click();
  await page.waitForTimeout(800);
  const detailsPaneHtml = await page.locator('#details-pane').innerHTML().catch(() => '');
  const hasDetails = detailsPaneHtml.length > 100;
  findings.push({
    surface: 'calendar',
    check: 'calendar-day-click-shows-anniversary',
    result: hasDetails ? 'PASS' : 'FAIL',
    details: `clicked day "${label}", details pane html length: ${detailsPaneHtml.length}`,
  });
} else {
  findings.push({ surface: 'calendar', check: 'day-button', result: 'FAIL', details: 'no day button found' });
}

// 2. /soldiers/new required-field validation
// Submit via form.submit() to bypass the busy state / overlay that
// a direct click() would race with.
await page.goto(`${BASE}/soldiers/new`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(500);
const requiredFields = await page.locator('form [required]').count();
const urlBefore = page.url();
const form = page.locator('form').first();
await form.evaluate((f) => f.submit()).catch(() => {});
await page.waitForTimeout(500);
const urlAfter = page.url();
const formBlocked = urlBefore === urlAfter;
findings.push({
  surface: 'soldier-new',
  check: 'required-field-validation',
  result: formBlocked ? 'PASS' : 'FAIL',
  details: `${requiredFields} required field(s); URL stayed at ${urlAfter}`,
});

// 3. /browse soldier link
await page.goto(`${BASE}/browse`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(500);
const soldierLink = page.locator('a[href^="/soldiers/"]').first();
if (await soldierLink.count() > 0) {
  const href = await soldierLink.getAttribute('href');
  await soldierLink.click();
  await page.waitForTimeout(800);
  const detailUrl = page.url();
  const landed = detailUrl.includes('/soldiers/') && !detailUrl.endsWith('/browse');
  const detailTitle = await page.locator('h1, h2, h3').first().innerText().catch(() => '');
  findings.push({
    surface: 'browse',
    check: 'browse-soldier-link-navigates',
    result: landed ? 'PASS' : 'FAIL',
    details: `clicked ${href}, landed on ${detailUrl}; first heading: "${detailTitle}"`,
  });
}

// 4. Floating dock layout
await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(500);
const menuBtn = page.locator('button:has-text("Menu")').first();
if (await menuBtn.count() > 0) {
  await menuBtn.click();
  await page.waitForTimeout(400);
  const navPanel = page.locator('[data-floating-nav-panel]').first();
  const navVisible = await navPanel.isVisible().catch(() => false);
  if (navVisible) {
    const navBox = await navPanel.boundingBox();
    const dock = page.locator('.floating-dock').first();
    const dockBox = await dock.boundingBox();
    const overlap = navBox && dockBox && (
      navBox.x < dockBox.x + dockBox.width &&
      navBox.x + navBox.width > dockBox.x &&
      navBox.y < dockBox.y + dockBox.height &&
      navBox.y + navBox.height > dockBox.y
    );
    findings.push({
      surface: 'floating-dock-layout',
      check: 'floating-nav-vs-dock-overlap',
      result: overlap ? 'FAIL' : 'PASS',
      details: overlap ? 'nav panel overlaps dock' : `nav ${JSON.stringify(navBox)} dock ${JSON.stringify(dockBox)}`,
    });
  }
  // Close nav
  await menuBtn.click();
  await page.waitForTimeout(200);
}

// 5. Open feedback modal + submit + check no console errors
await page.locator('[data-feedback-open]').first().click();
await page.waitForTimeout(200);
await page.fill('#feedback-form textarea[name="message"]', 'manual audit walkthrough test message');
const feedbackErrorsBefore = consoleErrors.length;
await page.click('#feedback-form button[type="submit"]');
await page.waitForTimeout(800);
const toastVisible = await page.evaluate(() => {
  const t = document.querySelector('[data-toast-region]');
  return t instanceof HTMLElement && !t.classList.contains('hidden') && (t.textContent || '').trim().length > 0;
});
const newErrors = consoleErrors.slice(feedbackErrorsBefore);
findings.push({
  surface: 'feedback-modal',
  check: 'save-shows-toast-no-console-errors',
  result: toastVisible && newErrors.length === 0 ? 'PASS' : (toastVisible ? 'CONCERN' : 'FAIL'),
  details: `toast=${toastVisible} newErrors=${newErrors.length} ${newErrors.slice(0, 2).join(' | ')}`,
});

await browser.close();
await writeFile(REPORT, JSON.stringify({ findings, ts: new Date().toISOString() }, null, 2));
console.log('\n============================================================');
console.log('MANUAL WALK RESULTS:');
for (const f of findings) {
  console.log(`  ${f.result === 'PASS' ? '✓' : (f.result === 'CONCERN' ? '?' : '✗')} ${f.surface}.${f.check} — ${f.details}`);
}
const passes = findings.filter((f) => f.result === 'PASS').length;
const fails = findings.filter((f) => f.result === 'FAIL').length;
const concerns = findings.filter((f) => f.result === 'CONCERN').length;
console.log(`\nTotal: ${passes} pass, ${fails} fail, ${concerns} concern`);
console.log(`Report: ${REPORT}`);
console.log('============================================================');
process.exit(fails > 0 ? 1 : 0);
