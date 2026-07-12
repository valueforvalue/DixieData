// audit/smoke_recover_display_id.mjs — round-trip click test for the
// per-row "Generate Display ID" affordance on the data-quality scan
// identity-missing group (issue #493 follow-up to #416).
//
// Bug surface: shipped in commit 487f5f5 (v1.2.59, issue #416), the
// button rendered but did not visibly correct the row's display_id
// because (a) the inner form was nested inside the outer Move
// Selected form (HTML5 invalid; Chromium auto-closed it and submit
// behavior became renderer-dependent), and (b) on success the toast
// was queued for the next nav and never surfaced because no reload
// fired.
//
// This probe:
//   1. Boots against a live dixiedata-web server.
//   2. Navigates to /settings, runs the data-quality scan.
//   3. Locates the first row with data-recover-display-id (only
//      identity-missing rows have it).
//   4. Clicks the button, asserts a POST fires to
//      /soldiers/{id}/display-id/recover.
//   5. Asserts the response carries the X-DixieData-Toast success
//      header carrying the new DXDID.
//   6. Asserts the page reloads (data-reload-on-success=true on the
//      button triggers window.location.reload() after a 200).
//   7. After reload, re-runs the scan and asserts the recovered row
//      is NO LONGER in the identity-missing group.
//
// The probe is conditional: if no identity-missing row exists in the
// live archive, the probe skips with a "skipped" record rather than
// failing — operators with healthy archives won't see this surface.
//
// Run after dixiedata-web is up at $BASE_URL:
//   node audit/smoke_recover_display_id.mjs

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const results = [];
let pass = 0;
let fail = 0;
let skipped = 0;

function record(name, ok, details = {}) {
  results.push({ name, ok, ...details });
  if (ok === 'skipped') {
    skipped++;
    console.log(`  ⊘ ${name} — ${JSON.stringify(details)}`);
  } else if (ok) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.log(`  ✗ ${name} — ${JSON.stringify(details)}`);
  }
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();

  page.on('pageerror', (err) => console.log(`  [pageerror] ${err.message}`));

  // Step 1: /settings renders the data-quality card.
  console.log('\n[1] /settings loads and quality scan form submits');
  await page.goto(`${BASE}/settings`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);

  const qualityForm = page.locator('form[action*="/settings/quality/scan"]').first();
  const qualityBtn = qualityForm.locator('button[type="submit"]').first();
  const qualityReqPromise = page
    .waitForRequest(
      (req) => req.url().includes('/settings/quality/scan') && req.method() === 'POST',
      { timeout: 4000 }
    )
    .catch(() => null);
  await qualityBtn.click({ timeout: 1500 });
  const qualityReq = await qualityReqPromise;
  record('quality-scan-form-submits', !!qualityReq, {
    method: qualityReq?.method(),
    url: qualityReq?.url(),
  });
  await page.waitForTimeout(800);

  // Step 2: scan results render. If no identity-missing rows in the
  // live archive, the probe skips the rest (nothing to recover).
  console.log('\n[2] locate identity-missing recover button');
  const recoverBtn = page.locator('button[data-recover-display-id]').first();
  const recoverBtnCount = await recoverBtn.count();
  if (recoverBtnCount === 0) {
    record('recover-button-renders', 'skipped', {
      why: 'no identity-missing rows in live archive; nothing to recover',
    });
    await browser.close();
    process.exit(fail > 0 ? 1 : 0);
  }
  record('recover-button-renders', true, { count: recoverBtnCount });

  // Pin the target id before clicking — we need it to verify the
  // POST URL and to assert the row is gone after reload.
  const targetID = await recoverBtn.getAttribute('data-recover-display-id');
  record('recover-button-has-target-id', !!targetID, { targetID });

  // Step 3: click the button, wait for the POST + 200 + reload.
  // data-reload-on-success triggers window.location.reload() after
  // a successful response — capture the post-reload page.url() so
  // we can assert the reload actually happened.
  console.log('\n[3] click Generate Display ID, assert POST + toast + reload');
  const urlBeforeClick = page.url();
  const postPromise = page
    .waitForResponse(
      (resp) =>
        resp.url().includes(`/soldiers/${targetID}/display-id/recover`) &&
        resp.request().method() === 'POST',
      { timeout: 5000 }
    )
    .catch(() => null);
  const navPromise = page
    .waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 6000 })
    .catch(() => null);
  await recoverBtn.click({ timeout: 2000 });
  const [postResp, navOk] = await Promise.all([postPromise, navPromise]);
  record('recover-post-fires', postResp !== null, {
    why: postResp ? `status=${postResp.status()} url=${postResp.url()}` : 'no POST observed',
  });
  if (postResp) {
    record('recover-post-status-200', postResp.status() === 200, {
      status: postResp.status(),
    });
    const toastHeader = postResp.headers()['x-dixiedata-toast'] || '';
    record('recover-post-carries-toast', toastHeader.includes('Display ID recovered'), {
      toastHeader,
    });
  }
  record('recover-click-triggers-reload', navOk !== null, {
    urlBefore: urlBeforeClick,
    urlAfter: page.url(),
  });

  // Step 4: after reload, re-run the scan and assert the recovered
  // row is no longer in identity-missing.
  console.log('\n[4] re-run scan, assert recovered row is gone');
  await page.goto(`${BASE}/settings`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);
  const qualityBtn2 = page.locator('form[action*="/settings/quality/scan"] button[type="submit"]').first();
  await qualityBtn2.click({ timeout: 1500 });
  await page.waitForTimeout(800);

  const recoveredRowExists = await page.evaluate((id) => {
    return document.querySelector(`button[data-recover-display-id="${id}"]`) !== null;
  }, targetID);
  record('recovered-row-removed-from-identity-missing', !recoveredRowExists, {
    targetID,
    recoveredRowExists,
  });

  await browser.close();
  process.exit(fail > 0 ? 1 : 0);
}

main().catch((err) => {
  console.error('smoke_recover_display_id.mjs crashed:', err);
  process.exit(1);
});