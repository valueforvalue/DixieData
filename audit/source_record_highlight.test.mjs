// Source Record reorder highlight regression net
// (UX pass, see AGENTS.md Wails hazards + soldier_card.templ).
//
// The form-submit dispatcher stashes the moved record's ID in
// sessionStorage before navigating, and the
// flashLastMovedSourceRecord() helper applies
// data-just-moved="true" to the matching <li> on the next
// page load. This static-scan test pins all three legs of
// the contract:
//
//   1. The dispatcher writes to sessionStorage on a
//      successful Source reorder fetch.
//   2. The page-load helper reads sessionStorage and applies
//      the data attribute to the row.
//   3. The CSS bundle has the keyframe + selector that turn
//      the attribute into a visible highlight.

import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const APP_JS = join(ROOT, 'frontend/app.js');
const APP_CSS = join(ROOT, 'frontend/app.css');

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

const js = readFileSync(APP_JS, 'utf8');
const css = readFileSync(APP_CSS, 'utf8');

test('dispatcher stashes moved record id in sessionStorage before navigating', () => {
  // The highlight survives the post-reorder page reload
  // only if the source-record id is stashed somewhere that
  // round-trips through navigation. sessionStorage is the
  // right primitive (survives reload, cleared on tab close).
  // The key must be namespaced (dixiedata.*) so a future
  // debug tool or third-party script can read it without
  // colliding.
  const dispatcherIdx = js.indexOf('async function dispatchDixieDataForm');
  // The stash block lives ~400 lines after the function
  // declaration, just before the navigate-to-redirect
  // branch. Widen the window so the regex can see it.
  const window = js.slice(dispatcherIdx, dispatcherIdx + 30000);
  assert.ok(
    /dixiedata\.lastMovedSource/.test(window),
    'dispatcher should write to sessionStorage under key dixiedata.lastMovedSource so the next page load can flash the moved row',
  );
  assert.ok(
    /sessionStorage\.setItem/.test(window),
    'dispatcher should use sessionStorage.setItem (not localStorage, which would leak across tabs)',
  );
  // Must be scoped to Source reorder URLs only. Match the
  // path segments separately to avoid regex-escape noise
  // around the literal `(\d+)` capture group syntax the
  // dispatcher uses.
  assert.ok(
    window.indexOf("/soldiers/") >= 0
      && window.indexOf("/sources/") >= 0
      && window.indexOf("/position") >= 0
      && window.indexOf("pathname.match") >= 0,
    'highlight stash should be scoped to /soldiers/{id}/sources/{sourceId}/position URLs only — other PATCH/POST forms should not flash',
  );
});

test('flashLastMovedSourceRecord helper exists and is wired into DOMContentLoaded', () => {
  // The helper reads sessionStorage, applies data-just-moved
  // to the row, and cleans up. It must run on every page
  // load (including the post-reorder reload) so the
  // highlight fires after navigation.
  assert.ok(
    /function\s+flashLastMovedSourceRecord\s*\(/.test(js),
    'flashLastMovedSourceRecord helper function missing from frontend/app.js',
  );
  assert.ok(
    /flashLastMovedSourceRecord\s*\(\s*\)/.test(js),
    'flashLastMovedSourceRecord is not invoked from any page-load boot path; the highlight will never fire',
  );
  assert.ok(
    /dixiedata\.lastMovedSource/.test(js),
    'flashLastMovedSourceRecord should read the same sessionStorage key the dispatcher writes',
  );
  assert.ok(
    /data-just-moved/.test(js),
    'flashLastMovedSourceRecord should toggle data-just-moved on the matching row',
  );
});

test('CSS bundle includes the highlight keyframe and selector', () => {
  // The animation contract:
  //   - selector: [data-source-record-id][data-just-moved="true"]
  //     (scoped to source-record rows, not all <li>s)
  //   - keyframe: source-row-flash, ~1.5s, amber pulse
  // The minified CSS uses data-source-record-id][data-just-moved=true
  // (no quotes around true) so the assertion matches the
  // bundle shape, not the source shape.
  assert.ok(
    /\[data-source-record-id\]\[data-just-moved=true\]/.test(css),
    'app.css is missing the [data-source-record-id][data-just-moved=true] selector — the highlight will not render',
  );
  assert.ok(
    /source-row-flash/.test(css),
    'app.css is missing the source-row-flash keyframe',
  );
  assert.ok(
    /#fef3c7/.test(css),
    'app.css is missing the amber-100 background color (#fef3c7) for the highlight',
  );
});

test('flash helper cleans up the data attribute after the animation', () => {
  // If the attribute is never removed, every row that
  // matches the data-source-record-id selector on every
  // page load would pulse forever. Cleanup is required.
  const helperIdx = js.indexOf('function flashLastMovedSourceRecord');
  const helper = js.slice(helperIdx, helperIdx + 2000);
  assert.ok(
    /removeAttribute\(\s*['"]data-just-moved['"]\s*\)/.test(helper),
    'flashLastMovedSourceRecord should remove the data-just-moved attribute once the animation ends',
  );
  assert.ok(
    /animationend|transitionend/.test(helper),
    'flashLastMovedSourceRecord should listen for animationend/transitionend to time the cleanup',
  );
  assert.ok(
    /sessionStorage\.removeItem/.test(helper),
    'flashLastMovedSourceRecord should clear the sessionStorage entry so a manual deep link does not re-trigger the highlight',
  );
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);