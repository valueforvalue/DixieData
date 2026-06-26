// Feedback loop for "no status shown when a backup is loaded".
//
// The Load Backup button on /share has `hx-target="this" hx-swap="none"`
// while the OTHER import buttons (memorial-json, shared-archive) target
// `#share-status`. The shared panel `<div id="share-status">` is the
// in-page status display; "this" + "none" puts the result in the toast
// header only (which the user reports as missing or hard to see).
//
// Loop:
//  1. Load /share
//  2. Verify that the Load Backup button targets `#share-status`
//    (NOT "this"), matching the other import buttons
//  3. Verify the `#share-status` panel exists and is visible
//
// Once the button targets the shared panel, htmx will swap the
// handler's response text into `#share-status` (which already
// happens for the other two import buttons).

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();

  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));
  page.on('console', (msg) => { if (msg.type() === 'error') errors.push(`console: ${msg.text()}`); });

  await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);

  // The Load Backup button is inside the "Replace Local Archive" card.
  const loadBackupButton = page.locator('button.danger-button:has-text("Load Backup")');
  const buttonCount = await loadBackupButton.count();
  if (buttonCount !== 1) {
    console.log(`FAIL: expected 1 Load Backup button, found ${buttonCount}`);
    process.exit(1);
  }

  // Read the button's hx-target attribute.
  const hxTarget = await loadBackupButton.first().getAttribute('hx-target');
  const hxSwap = await loadBackupButton.first().getAttribute('hx-swap');
  console.log(`Load Backup: hx-target="${hxTarget}", hx-swap="${hxSwap}"`);

  // The other import buttons in the same panel must target #share-status.
  const memorialButton = page.locator('div:has(p:has-text("Memorial JSON Import")) >> button:has-text("Preview Memorial JSON Import")');
  const memorialTarget = await memorialButton.first().getAttribute('hx-target').catch(() => null);
  console.log(`Memorial JSON Preview: hx-target="${memorialTarget}"`);

  // Check that #share-status panel exists and is visible.
  const statusPanelVisible = await page.locator('#share-status').isVisible().catch(() => false);
  console.log(`#share-status panel visible: ${statusPanelVisible}`);

  await browser.close();

  if (errors.length > 0) {
    console.log(`page errors: ${errors.length}`);
    errors.forEach((e) => console.log(`  - ${e}`));
  }

  // Pass: Load Backup targets #share-status (the shared panel).
  if (hxTarget !== '#share-status') {
    console.log(`FAIL: Load Backup targets "${hxTarget}" — should target "#share-status" like the other imports`);
    process.exit(1);
  }
  if (hxSwap === 'none') {
    console.log(`FAIL: Load Backup uses hx-swap="none" — would suppress the response; other imports don't use swap=none`);
    process.exit(1);
  }
  if (memorialTarget !== '#share-status') {
    console.log(`WARN: Memorial JSON Preview targets "${memorialTarget}" (expected "#share-status") — sibling inconsistency`);
  }
  if (!statusPanelVisible) {
    console.log('FAIL: #share-status panel missing or invisible');
    process.exit(1);
  }
  console.log('PASS: Load Backup targets #share-status, the shared status panel exists');
  console.log('NOTE: end-to-end backup-load test requires manual file picker — covered by Go e2e (TestBackupService_ImportSeededArchiveRoundTrip) instead.');

main().catch((err) => {
  console.error('FATAL:', err);
  process.exit(2);
});