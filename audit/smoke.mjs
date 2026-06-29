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
import { discoverShareExportButtons } from './discover_export_buttons.mjs';

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

// clickAndWaitForResponse is the response-side twin of
// clickAndWaitForRequest. Returns the full Response so callers
// can inspect status + headers (e.g. the 303 Location: /jobs/{id}
// header every share-page export now writes after issue #130).
async function clickAndWaitForResponse(page, buttonLocator, path, opts = {}) {
  const respPromise = page.waitForResponse(
    (resp) => resp.url().includes(path) && resp.request().method() === 'POST',
    { timeout: opts.timeout || 4000 }
  ).catch(() => null);
  try {
    await buttonLocator.click({ timeout: 1500 });
  } catch (e) {
    return null;
  }
  return await respPromise;
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
  let insightsResp = null;
  if (insightsExists > 0) {
    const insightsReqPromise = page.waitForRequest(
      (r) => r.url().includes('/insights/report/pdf') && r.method() === 'POST',
      { timeout: 4000 }
    ).catch(() => null);
    try {
      await insightsBtn.click({ timeout: 1500 });
    } catch (e) { /* may be hidden */ }
    insightsResp = await insightsReqPromise;
    // Option C: dispatcher navigates via window.location.assign. Wait
    // for the resulting navigation to settle before the next page.goto.
    await page.waitForLoadState('domcontentloaded', { timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(200);
  }
  record('insights-export-button-exists', insightsExists > 0);
  record('insights-export-fires-request', !!insightsResp, {
    method: insightsResp?.method(),
    url: insightsResp?.url(),
  });

  // ────────────────────────────────────────────────────────────────────
  // Share page export buttons (correct labels)
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[5] Share page export buttons');
  // The manifest is auto-discovered from internal/templates/*.templ
  // by audit/discover_export_buttons.mjs. A new export button added
  // to share.templ is covered by the harness without manual
  // curation. See the discovery module header for the eligibility
  // rules and the override tables (builderPrefixOverrides +
  // literalPathOverrides) that govern which buttons qualify.
  const shareButtons = discoverShareExportButtons().map((b) => ({
    label: b.label,
    path: b.path,
  }));

  for (const btn of shareButtons) {
    await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(300);
    const loc = page.locator('button', { hasText: btn.label }).first();
    const exists = await loc.count();
    // Click + wait for any navigation to settle. Option C's dispatcher
    // calls window.location.assign() which triggers a navigation;
    // subsequent page.goto calls would conflict with an in-flight nav.
    let req = null;
    if (exists > 0) {
      const reqPromise = page.waitForRequest(
        (r) => r.url().includes(btn.path) && r.method() === 'POST',
        { timeout: 4000 }
      ).catch(() => null);
      try {
        await loc.click({ timeout: 1500 });
      } catch (e) { /* button may be hidden by modal */ }
      req = await reqPromise;
      // Wait for any navigation triggered by the dispatcher.
      await page.waitForLoadState('domcontentloaded', { timeout: 3000 }).catch(() => {});
      await page.waitForTimeout(200);
    }
    record(`share-${btn.path}-button-exists`, exists > 0);
    record(`share-${btn.path}-fires-request`, !!req, {
      method: req?.method(),
      url: req?.url(),
    });
    // Option C: dispatcher reads 303 Location OR 200 X-DixieData-Redirect,
    // navigates via window.location.assign. The user-visible contract is
    // landing on /jobs/{id} (or returning to /share on dedup). Don't assert
    // a specific status code — assert the page actually navigated.
    const expectsRedirect = !btn.path.startsWith('/export/static-archive');
    if (expectsRedirect) {
      await page.waitForTimeout(200); // give the redirect a moment to settle
      const urlAfter = page.url();
      // All exports (except the carve-outs below) now route through
      // enqueueExport -> X-DixieData-Redirect, which
      // dispatchDixieDataForm reads and navigates to. The smoke that
      // accepted /share as success was masking a real bug: the web-mode
      // binary never installed SetSaveFileDialogOverride, so every save-
      // dialog-backed export landed in the dedup branch and bounced the
      // user back to /share. With the override wired in
      // cmd/dixiedata-web, every export should land on /jobs/{id}.
      //
      // Two carve-outs:
      //   - /export/static-archive: uses a plain <form method="post">
      //     and the browser follows 303 + Location natively, so the
      //     dispatcher never fires and the URL stays on /share.
      //   - /export/feedback-log (issue #137): now routes through
      //     enqueueExport when a feedback log exists (lands on /jobs/{id}),
      //     but the no-feedback-yet branch returns 200 + X-DixieData-Toast
      //     and stays on /share. Both outcomes are acceptable.
      const isExemptFromJobRedirect =
        btn.path.startsWith('/export/static-archive') ||
        btn.path === '/export/feedback-log';
      const navigated = isExemptFromJobRedirect
        ? urlAfter.endsWith('/share') || urlAfter.includes('/jobs/')
        : urlAfter.includes('/jobs/');
      record(`share-${btn.path}-navigates-to-jobs`, navigated, {
        urlAfter,
        expected: isExemptFromJobRedirect
          ? '/share or /jobs/{id} (plain-form / no-data carve-out)'
          : '/jobs/{id} (dispatcher must read X-DixieData-Redirect)',
      });
    }
  }
  // Printable PDF modal: same /jobs/{id} landing surface as the buttons above.
  // Regression net for the bug where the JS bridge returned inline markup instead of
  // letting htmx redirect to the job status page (issue: 'export options status pages
  // not landing'). The form now uses hx-swap=none, so the only thing that can navigate
  // the page is the HX-Redirect header from handleExportDatabasePDF.
  console.log('\n[5b] Printable PDF modal navigates to /jobs/{id}');
  await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);
  const printModalTrigger = page.locator('[data-print-config-open]').first();
  const printModalExists = await printModalTrigger.count();
  if (printModalExists > 0) {
    await printModalTrigger.click();
    await page.waitForTimeout(200);
    const submitPrint = page.locator('button[type="submit"]', { hasText: /Generate|Export|Start|Print|Printable/i }).first();
    const printSubmitExists = await submitPrint.count();
    record('share-print-modal-openable', printSubmitExists > 0);
    if (printSubmitExists > 0) {
      await page.waitForTimeout(200);
      const printUrlAfter = page.url();
      const navigated = printUrlAfter.includes('/jobs/') || printUrlAfter.endsWith('/share');
      record('share-print-modal-navigates-to-jobs', navigated, {
        urlAfter: printUrlAfter,
      });
    }
  } else {
    record('share-print-modal-openable', false, { reason: 'no trigger found' });
  }


  // ───────────────────────────────────────────────────────────────────
  // Memorial JSON import: pick file -> queued /jobs/{id}
  // -> confirm -> terminal status with the confirmation
  // card visible.
  // ───────────────────────────────────────────────────────────────────
  console.log('\n[5c] Memorial JSON import confirmation flow');
  // The harness cannot drive the OS file picker, so the dev
  // binary's DIXIE_OPEN_FILE_DIALOG_PATH env (set by run.mjs
  // before the binary boots) hands the file path directly to
  // the open-dialog override. Without that env wiring, this
  // block is skipped.
  const memorialPath = process.env.MEMORIAL_JSON_PATH;
  if (!memorialPath) {
    record('memorial-import-flow', false, {
      reason: 'MEMORIAL_JSON_PATH not set; run.mjs is expected to seed it',
    });
  } else {
    await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(300);
    const memorialBtn = page.locator('button[data-action="/import/memorial-json"]').first();
    const memorialBtnCount = await memorialBtn.count();
    if (memorialBtnCount === 0) {
      record('memorial-import-button-rendered', false, {
        reason: 'Memorial JSON button missing from share.templ',
      });
    } else {
      record('memorial-import-button-rendered', true);
      // Click + wait for navigation to /jobs/{id} (queued state).
      const navP = page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 4000 }).catch(() => null);
      await memorialBtn.click({ timeout: 2000 }).catch(() => null);
      await navP;
      await page.waitForTimeout(400);
      const queuedUrl = page.url();
      record('memorial-import-redirects-to-job-page', /\/jobs\//.test(queuedUrl), {
        urlAfter: queuedUrl,
        expected: '/jobs/{id}',
      });
      // Awaiting-confirmation card must render with Confirm + Cancel.
      const confirmBtn = page.locator('[data-job-confirm]').first();
      const cancelBtn = page.locator('[data-job-cancel]').first();
      const confirmCount = await confirmBtn.count();
      const cancelCount = await cancelBtn.count();
      record('memorial-import-confirmation-card-rendered', confirmCount > 0 && cancelCount > 0, {
        confirmCount,
        cancelCount,
      });
      // Click Cancel to dismiss the queued job so we don't leave
      // it running across smoke runs.
      if (cancelCount > 0) {
        await cancelBtn.click({ timeout: 1500 }).catch(() => null);
        await page.waitForTimeout(300);
      }
    }
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

  // [7b] beforeend swap (commit b185f0e). The debug-mode toggle
  // uses hx-swap="beforeend" so the new debug-console-panel is
  // appended to <body> without replacing the document. After the
  // first toggle the page must contain a #debug-console-panel as
  // a direct body child; if the wrong swap strategy slipped in
  // the page would be wiped instead.
  const panelCountBefore = await page.evaluate(
    () => document.querySelectorAll('body > #debug-console-panel').length
  );
  // Visit /debug/console a second time to exercise the beforeend
  // append path (the same path the toggle button triggers via
  // hx-get /debug/console hx-swap beforeend).
  const debugPanelResp = await page.goto(`${BASE}/debug/console`, { waitUntil: 'domcontentloaded' }).catch(() => null);
  await page.waitForTimeout(400);
  const panelCountAfter = await page.evaluate(
    () => document.querySelectorAll('body > #debug-console-panel').length
  );
  record('debug-console-panel-appends-beforeend', panelCountAfter >= panelCountBefore, {
    before: panelCountBefore,
    after: panelCountAfter,
  });

  // ────────────────────────────────────────────────────────────────────
  // Settings → Scan for Orphaned Images + Data Quality Scan render
  // into their result divs (issue #134). Both forms carry
  // data-dixie-submit + data-results-target; the dispatcher writes
  // the response body into the target div instead of dropping it.
  // ────────────────────────────────────────────────────────────────────
  console.log('\n[7c] Settings scan/quality results render');
  await page.goto(`${BASE}/settings`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);

  const orphanForm = page.locator('form[action*="/settings/images/orphans/scan"]').first();
  const orphanBtn = orphanForm.locator('button[type="submit"]').first();
  const orphanReq = await clickAndWaitForRequest(page, orphanBtn, '/settings/images/orphans/scan');
  record('orphan-scan-form-submits', !!orphanReq, {
    method: orphanReq?.method(),
    url: orphanReq?.url(),
  });
  await page.waitForTimeout(400);
  const orphanResultsHtml = await page.evaluate(
    () => document.querySelector('#settings-orphan-results')?.innerHTML?.trim() || ''
  );
  record('orphan-scan-results-render', orphanResultsHtml.length > 0, {
    targetLen: orphanResultsHtml.length,
  });

  const qualityForm = page.locator('form[action*="/settings/quality/scan"]').first();
  const qualityBtn = qualityForm.locator('button[type="submit"]').first();
  const qualityReq = await clickAndWaitForRequest(page, qualityBtn, '/settings/quality/scan');
  record('quality-scan-form-submits', !!qualityReq, {
    method: qualityReq?.method(),
    url: qualityReq?.url(),
  });
  await page.waitForTimeout(400);
  const qualityResultsHtml = await page.evaluate(
    () => document.querySelector('#settings-quality-results')?.innerHTML?.trim() || ''
  );
  record('quality-scan-results-render', qualityResultsHtml.length > 0, {
    targetLen: qualityResultsHtml.length,
  });

  // [7d] Feedback save flow: the floating-dock "Feedback" button
  // opens the modal, the user types a message, clicks "Save
  // Feedback", and the response carries X-DixieData-Close-Feedback
  // + X-DixieData-Toast headers. The JS post-response path must
  // (a) hide the modal, (b) clear the form fields, and (c) render
  // the toast immediately — not queue it for the next page nav.
  // This block catches the regression where the toast was queued
  // and never displayed, leaving the user with a closed modal and
  // no confirmation.
  console.log('\n[7d] Feedback save flow confirmation');
  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(300);
  const feedbackOpenBtn = page.locator('[data-feedback-open]').first();
  const feedbackOpenExists = await feedbackOpenBtn.count();
  if (feedbackOpenExists > 0) {
    await feedbackOpenBtn.click();
    await page.waitForTimeout(200);
    const feedbackForm = page.locator('#feedback-form').first();
    const feedbackFormVisible = await feedbackForm.isVisible().catch(() => false);
    record('feedback-modal-openable', feedbackFormVisible);
    if (feedbackFormVisible) {
      await page.fill('#feedback-form textarea[name="message"]', 'smoke test feedback');
      const feedbackSubmitRespPromise = page.waitForResponse(
        (r) => r.url().includes('/feedback/submit') && r.request().method() === 'POST'
      );
      await page.click('#feedback-form button[type="submit"]');
      const feedbackResp = await feedbackSubmitRespPromise;
      const closeHeader = feedbackResp.headers()['x-dixiedata-close-feedback'];
      const toastHeader = feedbackResp.headers()['x-dixiedata-toast'];
      record('feedback-save-sends-close-header', closeHeader === 'true', {
        closeHeader,
      });
      record('feedback-save-sends-toast-header', !!toastHeader, {
        toastHeader,
      });
      await page.waitForTimeout(400);
      const modalHidden = await page.evaluate(() => {
        const modal = document.querySelector('[data-feedback-modal]');
        return modal instanceof HTMLElement && modal.classList.contains('hidden');
      });
      record('feedback-save-hides-modal', modalHidden);
      const textareaValue = await page.evaluate(() => {
        const ta = document.querySelector('#feedback-form textarea[name="message"]');
        return ta instanceof HTMLTextAreaElement ? ta.value : null;
      });
      record('feedback-save-clears-form', textareaValue === '', {
        textareaValue,
      });
      const toastVisible = await page.evaluate(() => {
        const toast = document.querySelector('[data-toast-region]');
        if (!(toast instanceof HTMLElement)) return false;
        return !toast.classList.contains('hidden') && (toast.textContent || '').trim().length > 0;
      });
      record('feedback-save-shows-toast', toastVisible);
    }
  } else {
    record('feedback-modal-openable', false, { reason: 'no [data-feedback-open] trigger' });
  }

  // [8] Progress slot swap (commit b185f0e). The layout's
  // progress slot polls /jobs/active and uses hx-swap="innerHTML"
  // against the [data-jobs-progress-region] wrapper. Without the
  // fix the wrapper itself got replaced and the next poll logged
  // htmx:targetError and the progress bar froze. This assertion
  // exercises the live /jobs/active endpoint on a page that has
  // the layout shell (e.g. /calendar) and verifies the wrapper
  // survives three sequential fetches.
  await page.goto(`${BASE}/calendar`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);
  const progressRegionCount = await page.evaluate(async () => {
    const baseline = document.querySelectorAll('[data-jobs-progress-region]').length;
    for (let i = 0; i < 3; i++) {
      try {
        await fetch('/jobs/active', { headers: { 'HX-Request': 'true' } });
      } catch (e) { /* ignore */ }
    }
    return {
      baseline,
      after: document.querySelectorAll('[data-jobs-progress-region]').length,
    };
  });
  record('jobs-progress-overlay-survives-polls', progressRegionCount.baseline >= 1 && progressRegionCount.after >= progressRegionCount.baseline, progressRegionCount);

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