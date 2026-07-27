// Regression net for issue #428: PATCH form method attribute
// fell back to GET via form.method IDL — Source Record and event
// source reorder forms silently GET'd instead of PATCH'ing.
//
// Browser quirk pinned by this test: HTMLFormElement.method
// IDL getter normalizes unsupported values (PATCH, PUT, DELETE)
// to "get" per the HTML spec. The fix in dispatchDixieDataForm
// (frontend/app.js ~line 3426) and formIsNewSoldierWithEmptyNames
// (~line 3287) reads the method via getAttribute() instead of the
// IDL getter.
//
// This test scans frontend/app.js and asserts:
//   1. The string "form.method.toUpperCase()" is GONE (the bug).
//      If a future refactor reintroduces it, this test fails.
//   2. The dispatcher uses "explicitMethod.toUpperCase()" so
//      PATCH/PUT/DELETE forms submit with the right HTTP verb.
//   3. The synthetic form path (data-action + data-method on a
//      bare button) is still wired to the same dispatcher — i.e.
//      the fix didn't accidentally orphan the data-action flow.
//
// Static source-scan is the right shape here because:
//   - Adding JSDOM as a devDep to verify the IDL behavior in
//     isolation is overkill; the browser quirk is well-documented.
//   - The repo's existing audit probes (discover_htmx_guard.mjs,
//     discover_orphan_handlers.mjs) follow the same source-scan
//     pattern.
//   - The probe protects against regression at the editor level,
//     which is where the bug originally shipped.
//
// If we ever add JSDOM (or vitest with happy-dom), this test
// can grow a runtime sibling that asserts form.method === "get"
// on a real <form method="patch">.

import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const APP_JS = join(ROOT, 'frontend/app.js');

let pass = 0;
let fail = 0;
function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  ✓ ${name}`);
  } catch (err) {
    fail++;
    console.log(`  ✗ ${name}`);
    console.log(`    ${err.message}`);
  }
}

const src = readFileSync(APP_JS, 'utf8');

test('frontend/app.js no longer reads method via form.method IDL', () => {
  // The bug: form.method.toUpperCase() returns "GET" for any
  // unsupported method attribute (PATCH/PUT/DELETE) because
  // HTMLFormElement.method normalizes to "get" per the HTML spec.
  // Anywhere in the file that uses form.method.toUpperCase() to
  // drive fetch is wrong. Allow form.method in other contexts
  // (e.g. form.method as an IDL property reference is fine if it's
  // not being called with toUpperCase to compute a fetch verb).
  const offenders = src.match(/form\.method\.toUpperCase\s*\(\s*\)/g) || [];
  assert.equal(
    offenders.length,
    0,
    `found ${offenders.length} "form.method.toUpperCase()" call(s); ` +
      `the IDL getter normalizes PATCH/PUT/DELETE to "get". ` +
      `Use getAttribute("method").toUpperCase() instead. ` +
      `Offenders: ${JSON.stringify(offenders)}`,
  );
});

test('dispatcher reads method via getAttribute not IDL', () => {
  assert.ok(
    /getAttribute\(\s*['"]method['"]\s*\)/.test(src),
    'expected method read via getAttribute("method") in frontend/app.js — the #428 fix reads the method via the HTML attribute, not the IDL getter',
  );
});

test('synthetic-form data-action path still wired through dispatcher', () => {
  // The fix at line ~3426 covers both real forms and the
  // synthetic <form> created when a bare button has data-action.
  // Sanity check that the synthetic form code path still exists
  // and the dispatcher decision logic hasn't been gutted.
  assert.ok(
    /synthetic\.method\s*=\s*method/.test(src),
    'synthetic form method assignment (data-action + data-method path) is missing — dispatcher may be broken for bare-button flows',
  );
  assert.ok(
    /data-action/.test(src),
    'data-action attribute handler missing from dispatcher',
  );
});

test('formIsNewSoldierWithEmptyNames uses getAttribute, not IDL', () => {
  // The same browser quirk was lurking in the empty-name guard.
  // Pin that we now read via getAttribute there too. Match
  // a generous window after the function declaration so the
  // assertion isn't tripped by unrelated later code (the
  // function body itself is ~25 lines).
  const fnIdx = src.indexOf('function formIsNewSoldierWithEmptyNames');
  assert.ok(fnIdx >= 0, 'formIsNewSoldierWithEmptyNames function not found in frontend/app.js');
  // Scan the next 40 lines / 2000 chars for the patterns of interest.
  const window = src.slice(fnIdx, fnIdx + 2000);
  assert.ok(
    !/form\.method\.toUpperCase/.test(window),
    'formIsNewSoldierWithEmptyNames still uses form.method.toUpperCase() — same #428 IDL bug',
  );
  assert.ok(
    /getAttribute\(\s*['"]method['"]\s*\)/.test(window),
    'formIsNewSoldierWithEmptyNames should read method via getAttribute (see #428 fix)',
  );
});

test('dispatcher bails when submitter button is disabled', () => {
  // Defense-in-depth net for the Source Record ▲/▼ buttons
  // (and any future form with a disabled submit button):
  // when the submitter's `disabled` is true, the HTML spec
  // says FormData(form, submitter) produces an empty entry
  // list, so the fetch would go out with no body and the
  // server would 400. Native HTML form submission ignores
  // disabled buttons, but the JS dispatcher can be invoked
  // via dispatchEvent() or by WebView2 quirks. Bail before
  // the fetch so the user doesn't see a misleading
  // validation toast.
  const dispatcherIdx = src.indexOf('async function dispatchDixieDataForm');
  assert.ok(dispatcherIdx >= 0, 'dispatchDixieDataForm function not found in frontend/app.js');
  const window = src.slice(dispatcherIdx, dispatcherIdx + 5000);
  assert.ok(
    /submitter\s+instanceof\s+HTMLButtonElement\s*&&\s*submitter\.disabled/.test(window),
    'dispatchDixieDataForm should check submitter.disabled and bail; otherwise FormData(form, submitter) returns empty body and server 400s',
  );
});

test('dispatcher rewrites PATCH/PUT/DELETE to POST + X-HTTP-Method-Override inside Wails', () => {
  // Wails v2.12.0's wails.localhost custom protocol does
  // not deliver the body for non-GET/POST requests to the
  // Go handler (confirmed in TDM65-00514 / soldier 525:
  // every PATCH reorder request reached the server with an
  // empty body, surfacing as a 'Position must be a positive
  // integer' validation toast). The server already accepts
  // X-HTTP-Method-Override (internal/appshell/app.go:487
  // requestMethodOverride) so the dispatcher rewrites the
  // method to POST and sets the override header when the
  // request is going to wails.localhost. Plain-Chromium
  // (audit harness) requests keep the real PATCH so
  // Playwright assertions still see the genuine method.
  const dispatcherIdx = src.indexOf('async function dispatchDixieDataForm');
  // The Wails-PATCH block + FormData-to-URLSearchParams
  // conversion both live in the dispatcher. Widen the window
  // enough to reach both (the URLSearchParams block sits
  // over ~10000 chars to reach both workarounds.
  const window = src.slice(dispatcherIdx, dispatcherIdx + 12000);
  assert.ok(
    /X-HTTP-Method-Override/.test(window),
    'dispatchDixieDataForm should set X-HTTP-Method-Override when the request is PATCH/PUT/DELETE and the URL is wails.localhost; otherwise Wails strips the body and the server returns 400',
  );
  assert.ok(
    /wails\.localhost/.test(window),
    'Wails-PATCH workaround should be scoped to wails.localhost so the audit harness keeps using real PATCH',
  );
  assert.ok(
    /new\s+URLSearchParams\s*\(\s*\)/.test(window),
    'dispatchDixieDataForm should construct a URLSearchParams to convert FormData entries for the wails.localhost path',
  );
  assert.ok(
    /application\/x-www-form-urlencoded/.test(window),
    'dispatcher should set Content-Type: application/x-www-form-urlencoded after FormData → URLSearchParams conversion',
  );
  assert.ok(
    /params\.toString\s*\(\s*\)/.test(window),
    'URLSearchParams must be serialized to a string before being assigned to fetchOptions.body',
  );
  // Issue #674: the wails.localhost gate originally only checked
  // requestUrl, but that's a relative path for most forms (e.g.
  // /soldiers/42/sources/5/position). The fallback to
  // window.location.hostname catches relative-action forms.
  assert.ok(
    /window\.location\.hostname\s*===?\s*['"]wails\.localhost['"]/.test(window),
    'Wails-PATCH + Wails-FormData gates should also check window.location.hostname; relative form actions (most forms) never carry wails.localhost in the path',
  );
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);