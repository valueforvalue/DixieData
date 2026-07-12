// Regression net for the slice-2 TypeScript-baseline fix.
//
// While wiring jsconfig.json + tsc against frontend/app.js,
// the type checker flagged app.js:3584-3585 with TS2304
// ("Cannot find name 'fetchOptions'") — a USE-BEFORE-DECLARE
// inside dispatchDixieDataForm. The empty-name confirm branch
// ran BEFORE the `const fetchOptions = { ... }` declaration,
// so on the empty-name path the early `fetchOptions.body
// instanceof FormData` read would have hit the temporal dead
// zone at runtime (ReferenceError). This is the latent bug
// the user wanted the typecheck work to catch.
//
// The fix reordered dispatchDixieDataForm so the FormData
// construction + fetchOptions assembly happen FIRST, then
// the empty-name confirm check appends "confirm_empty_name"
// to the already-built body. This test pins the order so
// the regression can't return.
//
// What it asserts:
//   1. The line `const fetchOptions = { method: explicitMethod ?...
//      (or any const fetchOptions = { method: ... } declaration)
//      appears BEFORE the
//      `if (formIsNewSoldierWithEmptyNames(button, form))` call.
//   2. The `fetchOptions.body instanceof FormData` read sits
//      AFTER the declaration, not before.
//   3. The `confirm_empty_name` append is still wired in the
//      empty-name path.
//
// Static source-scan is the right shape here because:
//   - It mirrors the existing dispatcher_patch_method.test.mjs
//     pattern (same file, same DOMContentLoaded install, same
//     fail-loud messaging).
//   - The order is a textual invariant. Adding JSDOM to verify
//     TDZ behavior would not add signal beyond the line-order
//     check; the spec for `let`/`const` TDZ is deterministic.
//   - Future refactors of dispatchDixieDataForm are the only
//     plausible regression vector; a source-scan catches the
//     regression at the editor level.

import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const APP_JS = join(ROOT, 'frontend/app.js');

const content = readFileSync(APP_JS, 'utf8');
const lines = content.split('\n');

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

test('const fetchOptions = { method: ... } is declared in dispatchDixieDataForm', () => {
  const declMatch = content.match(/const fetchOptions = \{ method:/);
  if (!declMatch) {
    throw new Error('expected `const fetchOptions = { method: ... }` somewhere in app.js — dispatcher was refactored in a way that dropped the namespace, please rerun the slice-2 review');
  }
});

test('formIsNewSoldierWithEmptyNames is still wired in dispatchDixieDataForm', () => {
  // Find both the call site and the declaration line. Both must
  // exist; the order check below is the actual regression pin.
  const callIdx = content.indexOf('if (formIsNewSoldierWithEmptyNames(');
  if (callIdx < 0) {
    throw new Error('formIsNewSoldierWithEmptyNames call site missing — dispatcher was refactored in a way that dropped the empty-name flow');
  }
  const declIdx = content.indexOf('const fetchOptions = { method:');
  if (declIdx < 0) {
    throw new Error('fetchOptions declaration missing — see test above');
  }
  // Convert char offsets to line numbers for human-readable failures.
  const lineOf = (offset) => content.slice(0, offset).split('\n').length;
  if (declIdx >= callIdx) {
    throw new Error(
      `fetchOptions (line ${lineOf(declIdx)}) must be declared BEFORE ` +
      `the formIsNewSoldierWithEmptyNames branch (line ${lineOf(callIdx)}). ` +
      `Original slice-2 fix reordered the dispatcher so the empty-name confirm ` +
      `runs AFTER the body is constructed; if you intentionally re-ordered ` +
      `dispatchDixieDataForm, append ` +
      `"// intentional: <reason> reordering preserves fetchOptions.body access" ` +
      `to the branch header and update this test.`,
    );
  }
});

test('confirm_empty_name is still appended to FormData in the empty-name branch', () => {
  // After the slice-2 fix, the body append reads:
  //   if (fetchOptions.body instanceof FormData) {
  //     fetchOptions.body.append("confirm_empty_name", "1");
  //   }
  // and lives AFTER the fetchOptions declaration. This pins that
  // pattern; the substring check is strict to catch even a single-
  // char rename of the flag (the server reads "confirm_empty_name"
  // literally, see internal/templates/entry_form_save.go).
  const appendIdx = content.indexOf('fetchOptions.body.append("confirm_empty_name"');
  if (appendIdx < 0) {
    throw new Error('expected `fetchOptions.body.append("confirm_empty_name"` in app.js — empty-name flow was rewired without preserving the server-expected key');
  }
  const declIdx = content.indexOf('const fetchOptions = { method:');
  if (declIdx < 0) {
    throw new Error('fetchOptions declaration missing — see earlier test');
  }
  if (appendIdx <= declIdx) {
    throw new Error(
      `fetchOptions.body.append("confirm_empty_name", ...) sits at line ` +
      `${content.slice(0, appendIdx).split('\n').length}, which is at or before ` +
      `the fetchOptions declaration at line ` +
      `${content.slice(0, declIdx).split('\n').length}. ` +
      `This is the original TS2304 bug — fetchOptions is in the temporal dead ` +
      `zone on the empty-name path and would throw ReferenceError at runtime.`,
    );
  }
});

test('typeof FormData narrowing still gates the URLSearchParams workaround', () => {
  // Adjacent regression: the Wails-FormData workaround at app.js:3680+
  // converts FormData → URLSearchParams only when requestUrl contains
  // the wails.localhost or wails.:// prefix. If a future refactor
  // drops the FormData check, every form in Wails stops including
  // confirm_empty_name in the converted urlencoded body. Pin both
  // substrings.
  if (!content.includes('typeof FormData')) {
    throw new Error('expected `typeof FormData` narrowing in app.js — Wails-FormData workaround lost its type guard');
  }
  if (!content.includes('"wails.localhost"')) {
    throw new Error('expected `"wails.localhost"` URL guard in app.js — Wails method-override workaround lost its runtime gate');
  }
});

console.log(`\n  dispatcher_tdz_fix regression net: ${pass} passed, ${fail} failed`);
if (fail > 0) {
  process.exit(1);
}
