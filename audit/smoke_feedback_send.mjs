// audit/smoke_feedback_send.mjs
// Issue #566: regression net for the "Save & Send to Support" wire-up
// in the global feedback modal. Verifies the modal renders the new
// PII disclosure (locked decision 7), that clicking the new button
// fires the /feedback/submit POST with action=send, and that the
// response carries the success toast + closes the modal. Network
// probe is a separate Node fetch (the test is Wails-free — POSTs to
// the real Formspark endpoint and asserts 200 + JSON echo).
//
// Pins:
//   - the disclosure copy names all three version fields (app version,
//     build identity, database schema version) AND Formspark
//     (third-party marker) AND the local-first invariant.
//   - the Send button submits the form with action=send (not save).
//   - the response is 2xx + the success toast "Feedback sent to
//     DixieData support. A copy is in the local log."
//   - the live Formspark endpoint returns 2xx for the exact payload
//     shape the slice-1 helper builds.
//
// Usage:
//   node audit/smoke_feedback_send.mjs              # against the live BASE
//   BASE=http://127.0.0.1:8900 node audit/smoke_feedback_send.mjs
//
// Exit code: 0 on full pass, 1 on any assertion failure.

import { chromium } from 'playwright';
import { runProbe } from './_lib/smoke_runner.mjs';
import { loadConfig, resolveBaseUrl } from './_lib/config.mjs';

// Issue #707 batch 3: replaced hardcoded `process.env.BASE ||
// 'http://127.0.0.1:8900'` with config.mjs. PROBE_PORT (set by
// the aggregator's per-probe allocation) wins via
// resolveBaseUrl; SMOKE_BASE_URL (CI mode) is also honored.
// The browser probe is opt-in via SMOKE_FEEDBACK_SEND_BROWSER=1
// (network probe always runs against the live Formspark
// endpoint).
const cfg = loadConfig();
const BASE = resolveBaseUrl(cfg).replace(/\/$/, '');
const FORMSPARK_ENDPOINT = 'https://submit-form.com/vJSONT1nB';
let pass = 0;
let fail = 0;
const failures = [];

function record(name, ok, detail) {
  const tag = ok ? 'PASS' : 'FAIL';
  if (ok) {
    pass += 1;
    console.log(`  ${tag} ${name}`);
  } else {
    fail += 1;
    failures.push({ name, detail });
    console.log(`  ${tag} ${name} :: ${JSON.stringify(detail)}`);
  }
}

async function networkProbe() {
  // Mirror the exact payload shape the slice-1 helper builds.
  // No client secret, no bundle — text-only per slice 3 plan.
  const payload = {
    subject: 'smoke · /calendar · anonymous',
    message: 'smoke probe from audit/smoke_feedback_send.mjs',
    page_path: '/calendar',
    category: 'general',
    app_version: '1.1.1',
    build_identity: 'audit-smoke',
    schema_version: 67,
    submitted_at: new Date().toISOString(),
  };
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), 15_000);
  try {
    const res = await fetch(FORMSPARK_ENDPOINT, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify(payload),
      signal: ctrl.signal,
    });
    const text = await res.text();
    return {
      status: res.status,
      contentType: res.headers.get('content-type') || '',
      body: text,
    };
  } catch (err) {
    return { status: 0, error: err.message };
  } finally {
    clearTimeout(timer);
  }
}

async function runBrowserProbe() {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  try {
    await page.goto(BASE, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(300);

    // 1. Modal opens.
    const openBtn = page.locator('[data-feedback-open]').first();
    const openCount = await openBtn.count();
    if (openCount === 0) {
      record('feedback-modal-open-button', false, { reason: 'no [data-feedback-open] trigger' });
      return;
    }
    await openBtn.click();
    await page.waitForTimeout(200);
    const formVisible = await page.locator('#feedback-form').first().isVisible().catch(() => false);
    record('feedback-modal-openable', formVisible);
    if (!formVisible) {
      return;
    }

    // 2. Disclosure copy names all three version fields + Formspark +
    //    local-first invariant. Pin every needle so a future copy
    //    change must update this regression net.
    const disclosureText = await page.evaluate(() => {
      const strongs = Array.from(document.querySelectorAll('#feedback-modal strong'));
      const sendStrong = strongs.find((s) => (s.textContent || '').trim() === 'Send to support');
      if (!sendStrong) return '';
      const p = sendStrong.closest('p');
      return p ? (p.textContent || '') : '';
    });
    for (const needle of [
      'Send to support',
      'Formspark',
      'app 1.1.1',
      'build dev', // build identity — dev mode renders "dev · commit dev"; release builds render the actual commit
      'database schema v67',
      'local copy is always saved first',
    ]) {
      record(`feedback-modal-disclosure-mentions-${needle.replace(/[^a-z0-9]+/gi, '-').toLowerCase()}`, disclosureText.includes(needle), { needle, disclosureText });
    }

    // 3. The Send button must submit with action=send (not action=save).
    // Pin the button's identity + name + value so a future templ
    // refactor can't silently swap the wire.
    const sendButton = page.locator('#feedback-form button[value="send"][name="action"]').first();
    const sendCount = await sendButton.count();
    record('feedback-modal-send-button-exists', sendCount > 0, { count: sendCount });
    if (sendCount === 0) {
      return;
    }
    const sendButtonAttrs = await sendButton.evaluate((b) => ({
      type: b.getAttribute('type'),
      name: b.getAttribute('name'),
      value: b.getAttribute('value'),
    }));
    record('feedback-modal-send-button-has-correct-attrs',
      sendButtonAttrs.type === 'submit' && sendButtonAttrs.name === 'action' && sendButtonAttrs.value === 'send',
      sendButtonAttrs);

    // 4. Click the button + wait for the fetch + assert response shape.
    // Regression net for #571: form.action IDL returns a RadioNodeList
    // in Chromium when the form has any descendant element named
    // "action" (e.g. the feedback modal's Save + Send submit buttons).
    // The dispatcher in app.js:4171 uses form.getAttribute('action')
    // to defend. The probe asserts the dispatch fires the correct URL
    // and the response shape matches the success contract.
    await page.fill('#feedback-form textarea[name="message"]', 'smoke send-to-support probe');
    const sendRespPromise = page.waitForResponse(
      (r) => r.url().includes('/feedback/submit') && r.request().method() === 'POST',
      { timeout: 5000 }
    );
    await sendButton.click();
    const sendResp = await sendRespPromise;
    const requestPostData = sendResp.request().postData() || '';
    const closeHeader = sendResp.headers()['x-dixiedata-close-feedback'];
    const toastHeader = sendResp.headers()['x-dixiedata-toast'];
    record('feedback-send-sends-close-header', closeHeader === 'true', { closeHeader });
    record('feedback-send-sends-toast-header', !!toastHeader, { toastHeader });
    record(
      'feedback-send-success-toast-text',
      (toastHeader || '').includes('Feedback sent to DixieData support'),
      { toastHeader }
    );
    record('feedback-send-payload-contains-action-send',
      // Multipart body: name="action"\r\n\r\nsend
      // urlencoded body: (^|&)action=send(&|$)
      /name="action"\r\n\r\nsend/.test(requestPostData) || /(^|&)action=send(&|$)/.test(requestPostData),
      { requestPostData: requestPostData.slice(0, 400) });

    // 5. Modal hides + form clears.
    await page.waitForTimeout(400);
    const modalHidden = await page.evaluate(() => {
      const m = document.querySelector('[data-feedback-modal]');
      return m instanceof HTMLElement && m.classList.contains('hidden');
    });
    record('feedback-send-hides-modal', modalHidden);
    const textareaValue = await page.evaluate(() => {
      const ta = document.querySelector('#feedback-form textarea[name="message"]');
      return ta instanceof HTMLTextAreaElement ? ta.value : null;
    });
    record('feedback-send-clears-form', textareaValue === '', { textareaValue });
  } finally {
    await browser.close();
  }
}

async function main() {
  // Browser probe is opt-in: requires a running BASE (e.g. via
  // the audit harness or a developer running `dixiedata-web` in
  // the background). Skipped by default so CI runs (no live
  // server) don't false-fail. Network probe always runs.
  if (process.env.SMOKE_FEEDBACK_SEND_BROWSER === '1') {
    console.log(`\n[feedback-send] Browser probe against ${BASE}`);
    await runBrowserProbe();
  } else {
    console.log(`\n[feedback-send] Browser probe SKIPPED (set SMOKE_FEEDBACK_SEND_BROWSER=1 to enable against a live BASE)`);
  }
  console.log(`\n[feedback-send] Network probe against ${FORMSPARK_ENDPOINT}`);
  const net = await networkProbe();
  record('formspark-returns-2xx', net.status >= 200 && net.status < 300, { status: net.status, error: net.error });
  record('formspark-content-type-is-json', (net.contentType || '').includes('application/json'), { contentType: net.contentType });
  console.log(`\n[feedback-send] Result: ${pass} pass / ${fail} fail`);
  if (fail > 0) {
    console.log('\nFailures:');
    for (const f of failures) {
      console.log(`  - ${f.name}: ${JSON.stringify(f.detail)}`);
    }
  }
  return { ok: fail === 0, steps: { pass, fail } };
}

// Issue #707 batch 3: this probe only runs the network probe by
// default (the browser probe is opt-in via
// SMOKE_FEEDBACK_SEND_BROWSER=1). The runner provides the
// uniform {ok, steps} return contract + per-probe cleanup.
runProbe({ name: 'feedback-send', probeFn: async () => main() })
  .then((r) => { process.exit(r.ok ? 0 : 1); })
  .catch((err) => { console.error('FATAL', err); process.exit(2); });
