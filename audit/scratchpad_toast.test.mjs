// Regression net for issue #535: the floating-dock "Scratch Pad"
// button's no-record-open error must surface as a toast, not as
// a near-invisible bottom-dock status pill. Pre-#535 the helper
// `setScratchpadStatus` at frontend/app.js:1762 wrote into
// `<p data-floating-scratchpad-status>` in light slate-300 at the
// bottom of the screen. Post-#535 the helper is gone; the three
// call sites in `openScratchpad` call `showToast` directly; the
// `<p>` element is removed from internal/templates/layout.templ.
//
// This file source-scans frontend/app.js + internal/templates
// (the generated layout_templ.go, since the templ source is
// compiled to Go for the runtime render) for the post-#535
// contract. Source-scanning (not Playwright) is the cheapest
// regression net that pins the source-level contract; a live
// Playwright probe in audit/smoke_scratchpad.mjs would also
// exercise the actual toast-on-click path but needs a booted
// dixiedata-web server. The two complement each other.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const appJSPath = resolve(here, '..', 'frontend', 'app.js');
const appJS = readFileSync(appJSPath, 'utf8');

// The generated layout_go is what templ compiles layout.templ
// into. Both files are in internal/templates; either can carry
// the literal string `data-floating-scratchpad-status`. We scan
// every internal/templates/*.go file so a refactor that moves
// the element to a different layout helper doesn't slip past.
const templatesDir = resolve(here, '..', 'internal', 'templates');
const templatesGo = readdirSync(templatesDir)
  .filter((f) => f.endsWith('.go'))
  .map((f) => join(templatesDir, f));
const templatesContent = templatesGo
  .map((p) => readFileSync(p, 'utf8'))
  .join('\n');

test('app.js no longer defines setScratchpadStatus (issue #535)', () => {
  assert.doesNotMatch(
    appJS,
    /function\s+setScratchpadStatus\b/,
    'setScratchpadStatus helper must be removed; the 3 call sites in openScratchpad now call showToast directly (issue #535)',
  );
});

test('app.js no longer defines scratchpadStatusTarget (issue #535)', () => {
  assert.doesNotMatch(
    appJS,
    /function\s+scratchpadStatusTarget\b/,
    'scratchpadStatusTarget lookup helper must be removed alongside setScratchpadStatus (issue #535)',
  );
});

test('app.js no longer queries data-floating-scratchpad-status at runtime (issue #535)', () => {
  // The string "data-floating-scratchpad-status" may still
  // appear in comments (documenting what was removed) -- this
  // test pins the *code* contract that no querySelector or
  // dataset read targets the removed element at runtime.
  assert.doesNotMatch(
    appJS,
    /(?:querySelector|querySelectorAll|getElementById|closest)\s*\(\s*["'`][^"'`]*data-floating-scratchpad-status/,
    'app.js must not query data-floating-scratchpad-status at runtime; the <p> element is removed (issue #535)',
  );
});

test('openScratchpad no-record branch calls showToast with the warning message (issue #535)', () => {
  // The 3 call sites in openScratchpad must each call showToast.
  // We assert by looking for the warning message text near a
  // showToast call inside openScratchpad.
  const openIdx = appJS.indexOf('async function openScratchpad(');
  assert.ok(openIdx > 0, 'openScratchpad function must exist');
  // Find the end of the function (the next 'async function' or
  // top-level closing brace heuristic).
  let end = openIdx + 1;
  let depth = 0;
  let started = false;
  for (let i = openIdx; i < appJS.length; i++) {
    const c = appJS[i];
    if (c === '{') { depth++; started = true; }
    else if (c === '}') { depth--; if (started && depth === 0) { end = i + 1; break; } }
  }
  const body = appJS.slice(openIdx, end);
  assert.match(
    body,
    /Open a record with a saved Record ID/,
    'openScratchpad no-record branch must surface the user-facing message (issue #535)',
  );
  assert.match(
    body,
    /showToast\s*\(/,
    'openScratchpad no-record branch must call showToast (issue #535)',
  );
});

test('layout.templ render no longer carries data-floating-scratchpad-status element (issue #535)', () => {
  assert.doesNotMatch(
    templatesContent,
    /data-floating-scratchpad-status/,
    'generated layout_templ.go must not render data-floating-scratchpad-status anywhere; the <p> element is removed (issue #535)',
  );
});