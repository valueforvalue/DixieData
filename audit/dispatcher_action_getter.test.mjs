// audit/dispatcher_action_getter.test.mjs
// Regression net for #571: Chromium's HTMLFormElement.action IDL
// returns a RadioNodeList (the named-form-element collection)
// when a form contains any descendant element with name="action".
// The dispatcher in frontend/app.js:4171 must use
// form.getAttribute('action') to read the raw attribute instead.
//
// This probe asserts the engine quirk is real (sanity check on
// the build's Chrome) and that the dispatcher's chosen accessor
// (form.getAttribute('action')) returns the expected string
// regardless of the IDL getter's behaviour. A future refactor of
// the dispatcher line that drops back to form.action would
// silently break every form with a name="action" submit button.

import { chromium } from 'playwright';

const BASE = process.env.BASE || 'http://127.0.0.1:8900';
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

async function run() {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  try {
    await page.goto(BASE, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(500);

    // 1. The feedback form has two submit buttons both named
    //    "action" (Save + Send). Construct a synthetic assertion
    //    on the live form so a future templ change that drops
    //    the second button is caught here.
    const hasTwoActionButtons = await page.evaluate(() => {
      const f = document.getElementById('feedback-form');
      if (!f) return null;
      return f.querySelectorAll('button[name="action"]').length;
    });
    record('feedback-form-has-2-name-action-buttons', hasTwoActionButtons === 2, { hasTwoActionButtons });

    // 2. Sanity: the engine quirk must be real. form.action must
    //    NOT be the action attribute string (i.e. must be a Node
    //    or RadioNodeList, not a string). If this fails, the engine
    //    has been fixed and the dispatcher's defensive code is no
    //    longer needed — but the dispatcher should still prefer
    //    getAttribute for portability.
    const idlQuirk = await page.evaluate(() => {
      const f = document.getElementById('feedback-form');
      if (!f) return { exists: false };
      return {
        exists: true,
        actionAttr: f.getAttribute('action'),
        actionIdlType: typeof f.action,
        actionIdlStringified: String(f.action).slice(0, 60),
        actionIdlMatchesAttr: f.action === f.getAttribute('action'),
      };
    });
    if (!idlQuirk.exists) {
      record('feedback-form-rendered', false, { reason: 'no #feedback-form' });
    } else {
      // In a buggy Chromium build, actionIdlType === 'object' and
      // actionIdlMatchesAttr === false. In a fixed build, both are
      // 'string' and true. We assert the SAFE ACCESSOR (getAttribute)
      // returns the expected URL string regardless.
      record('feedback-form-getAttribute-action-returns-string',
        idlQuirk.actionAttr === '/feedback/submit' && typeof idlQuirk.actionAttr === 'string',
        idlQuirk);
    }

    // 3. Construct a synthetic form with the same shape and
    //    assert the dispatcher-style URL read returns the action
    //    attribute string. This is the test that pins the
    //    dispatcher's defensive code: form.getAttribute('action')
    //    must work even when form.action is a RadioNodeList.
    const syntheticResult = await page.evaluate(() => {
      const f = document.createElement('form');
      f.setAttribute('action', '/synthetic/test');
      f.method = 'post';
      const b1 = document.createElement('button');
      b1.setAttribute('type', 'submit');
      b1.setAttribute('name', 'action');
      b1.setAttribute('value', 'save');
      f.appendChild(b1);
      const b2 = document.createElement('button');
      b2.setAttribute('type', 'submit');
      b2.setAttribute('name', 'action');
      b2.setAttribute('value', 'send');
      f.appendChild(b2);
      document.body.appendChild(f);
      try {
        return {
          attrAccessor: f.getAttribute('action'),
          idlAccessor: String(f.action).slice(0, 60),
          idlType: typeof f.action,
        };
      } finally {
        f.remove();
      }
    });
    record('synthetic-form-getAttribute-action-returns-action-attr',
      syntheticResult.attrAccessor === '/synthetic/test',
      syntheticResult);
    record('synthetic-form-idl-action-quirk-is-observed',
      syntheticResult.idlType !== 'string' || syntheticResult.idlAccessor !== '/synthetic/test',
      { note: 'if this fails, Chromium has been fixed and the dispatcher no longer needs the getAttribute defence; consider dropping the workaround', ...syntheticResult });

    // 4. End-to-end: fire the dispatcher against the live feedback
    //    modal and assert the fetch URL matches the form's
    //    `action` attribute (not the RadioNodeList). This is the
    //    end-user-facing regression net.
    await page.locator('[data-feedback-open]').first().click();
    await page.waitForTimeout(300);
    await page.fill('#feedback-form textarea[name="message"]', 'dispatcher action getter probe');
    const respPromise = page.waitForResponse(
      (r) => r.url().includes('/feedback/submit') && r.request().method() === 'POST',
      { timeout: 5000 }
    );
    await page.locator('#feedback-form button[value="save"]').first().click();
    const resp = await respPromise;
    record('dispatcher-uses-getAttribute-action-not-radionodelist',
      resp.url().endsWith('/feedback/submit') && !resp.url().includes('[object'),
      { url: resp.url() });
  } finally {
    await browser.close();
  }
}

async function main() {
  console.log(`\n[dispatcher-action-getter] Browser probe against ${BASE}`);
  await run();
  console.log(`\n[dispatcher-action-getter] Result: ${pass} pass / ${fail} fail`);
  if (fail > 0) {
    console.log('\nFailures:');
    for (const f of failures) {
      console.log(`  - ${f.name}: ${JSON.stringify(f.detail)}`);
    }
    process.exit(1);
  }
}

main().catch((err) => {
  console.error('dispatcher_action_getter.test.mjs crashed:', err);
  process.exit(2);
});
