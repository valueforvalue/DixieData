// Regression net for the slice-3 TypeScript-baseline work.
//
// Slice 3 introduced frontend/global.d.ts as a typed-augmentation
// file that:
//   1. Adds ~15 install-once markers hung off `window` (e.g.
//      window.__foldoutInstallN, window.__dixieBrowseFilterTimer,
//      window.htmx, window.DIXIEDATA_DEVTOOLS, window.dixie).
//   2. Adds per-element markers on HTMLInputElement
//      (__dixieLiveCountHandler), Element (__copyPathBound) and
//      HTMLElement (same).
//   3. Provides the htmx CustomEvent detail shape used by the
//      afterRequest handler (xhr.getResponseHeader, xhr.status).
//
// These augmentations turn what was previously 168 TS2339 / TS2304
// errors into 0 — but they are load-bearing: removing the file (or
// deleting an interface block from it) would push the typechecker
// from clean back to a noisy baseline.
//
// This test pins the augmentation file's presence + shape so a
// future cleanup/refactor can't silently drop it. The file
// patterns checked here are stable: augmentation interfaces have
// a specific shape that the next slice will match when growing
// the marker list.
//
// What it asserts (against `frontend/global.d.ts`):
//   1. File exists and declares the DixieDataWindow interface.
//   2. The 12+ marker names are still declared (each pinned by
//      individual lookup so a deletion shows clearly).
//   3. The interface HTMLInputElement / Element / HTMLElement
//      augmentations are still present (per-element markers).
//   4. The htmx augmentation is still in place (the shape that
//      lets the afterRequest handler call xhr.getResponseHeader).
//
// What it asserts (against `frontend/app.js`):
//   5. The `eventTargetElement` helper introduced in slice 3 is
//      still wired in (the function reads `event.target` safely).
//      Loss of the helper would re-emerge ~53 TS2339 sites in the
//      DOMContentLoaded click block, all reading .closest on a raw
//      EventTarget.
//
// Static source-scan is the right shape here because:
//   - It mirrors the dispatcher_tdz_fix.test.mjs pattern.
//   - The augmentations are textual invariants; their effect on
//     tsc is verified by `just lint-typecheck` separately.
//   - Adding a runtime JSDOM harness for these checks would add
//     zero signal: the .d.ts file is consumed only at type-check
//     time, never at runtime.

import { strict as assert } from 'node:assert';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const GLOBAL_DTS = join(ROOT, 'frontend/global.d.ts');
const APP_JS = join(ROOT, 'frontend/app.js');

const dts = existsSync(GLOBAL_DTS)
  ? readFileSync(GLOBAL_DTS, 'utf8')
  : '';
const app = readFileSync(APP_JS, 'utf8');

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

test('frontend/global.d.ts exists', () => {
  if (!existsSync(GLOBAL_DTS)) {
    throw new Error('frontend/global.d.ts missing — slice-3 augmentation file was deleted. tsc would re-emit 168 errors.');
  }
});

test('DixieDataWindow interface is declared with all install-once markers', () => {
  if (!dts.includes('interface DixieDataWindow')) {
    throw new Error('`interface DixieDataWindow` missing from global.d.ts — the Window augmentation that resolves 25+ TS2339 sites has been removed.');
  }
  const required = [
    '__foldoutInstallN',
    '__foldoutProbeReinit',
    '__foldoutDocHandlerBound',
    '__foldoutBoundTriggers',
    '__megaMenuInstallN',
    '__megaMenuDocHandlerBound',
    '__megaMenuBoundTriggers',
    // Issue #476 floating-nav panel markers (installFloatingNavPanel).
    '__floatingNavInstallN',
    '__floatingNavBoundTriggers',
    '__dixieBrowseFilterDebounce',
    '__dixieDebug',
    '__dixieDebugDisabled',
    '__dixieGoogleSettings',
    '__dixieLocalSettings',
    '__dixieUpdateSettings',
    'htmx',
    'DIXIEDATA_DEVTOOLS',
    'dixie',
  ];
  for (const marker of required) {
    // Each marker must be declared inside the DixieDataWindow
    // interface block. Use a generous substring check: marker
    // appears at line start with `?:` or as a property name. If
    // a future refactor renames a marker, this test catches it
    // and forces a deliberate update.
    if (!dts.includes(marker)) {
      throw new Error(`marker \`${marker}\` missing from DixieDataWindow — rest of the app expects it; restoring this single interface member re-clears the related TS2339.`);
    }
  }
});

test('per-element marker interfaces are still declared', () => {
  // Each per-element sentinel hung off an HTML element (any
  // combination of HTMLInputElement, HTMLTextAreaElement,
  // HTMLFormElement, Element, HTMLElement) must have a
  // matching declaration in frontend/global.d.ts. The marker
  // name list is discovered by scanning app.js for
  // `<elem>.__<name> = ...` assignments and `<elem>.__<name> ===`
  // comparisons. If a new sentinel is added in app.js without
  // the corresponding augmentation, the discoverer's named-
  // capture group pulls it into the assertion set, the test
  // fails on the missing marker, and the developer is forced
  // to update global.d.ts before the tsc gate goes green.
  //
  // Origin: issue #711 — the `__shareIncludeTagsWired` sentinel
  // (from #705) shipped in app.js but never made it into the
  // .d.ts. The earlier version of this test only pinned three
  // markers by name (`__dixieLiveCountHandler`, `__copyPathBound`,
  // `__inventoryChartPainted`) and missed the new one.
  const sentinelRegex = /\.__([a-zA-Z][a-zA-Z0-9_]*)\s*(?:=|[!=]==)/g;
  const appMarkers = new Set();
  for (const match of app.matchAll(sentinelRegex)) {
    appMarkers.add(match[1]);
  }
  if (appMarkers.size === 0) {
    throw new Error('discoverer failed to find any `__<name> = / __<name> ===` patterns in app.js — regex is stale, fix the discoverer before relying on this test.');
  }
  // Required interface declarations (Element / HTMLElement are the
  // base augmentations; HTMLInputElement / HTMLTextAreaElement /
  // HTMLFormElement extend them).
  for (const iface of ['interface HTMLInputElement', 'interface HTMLTextAreaElement', 'interface HTMLFormElement', 'interface Element', 'interface HTMLElement']) {
    if (!dts.includes(iface)) {
      throw new Error(`\`${iface}\` augmentation missing — per-element markers can no longer compile.`);
    }
  }
  // Every sentinel written/read in app.js must appear in global.d.ts.
  // Skip the window-scoped helpers that live on DixieDataWindow
  // (and are pinned by the earlier test). Discovered by the
  // regex but not per-element sentinels: `window.__<name>`-style
  // installs of WeakSets, install counters, debounce wrappers,
  // and helper functions.
  const windowHelpers = new Set([
    '__dixieBrowseFilterDebounce',
    '__dixieDebounce',
    '__dixieInsertTextAtCursor',
    '__floatingNavBoundTriggers',
    '__floatingNavInstallN',
    '__foldoutBoundTriggers',
    '__foldoutDocHandlerBound',
    '__foldoutInstallN',
    '__foldoutProbeReinit',
    '__megaMenuBoundTriggers',
    '__megaMenuDocHandlerBound',
    '__megaMenuInstallN',
  ]);
  const missing = [];
  for (const marker of appMarkers) {
    if (windowHelpers.has(marker)) continue;
    if (!dts.includes(marker)) {
      missing.push(marker);
    }
  }
  if (missing.length > 0) {
    throw new Error(
      `per-element marker(s) declared in app.js but missing from frontend/global.d.ts: ${missing.join(', ')}. ` +
      `Add the corresponding \`__${missing[0]}?: boolean;\` (or typed signature) under the appropriate \`interface HTML…Element\` / \`interface Element\` / \`interface HTMLElement\` block. ` +
      `See issue #711 for the canonical example.`,
    );
  }
});

test('htmx augmentation carries xhr.getResponseHeader()', () => {
  // The afterRequest handler reads xhr.getResponseHeader() for
  // the X-DixieData-Redirect fragment-204 contract (issue #316,
  // blocked-state recovery). The augmentation must expose the
  // shape. If a future refactor narrows the htmx type, this
  // assertion catches the downgrade before the runtime helper
  // is silenced.
  if (!dts.includes('getResponseHeader')) {
    throw new Error('`getResponseHeader` missing from the htmx Window augmentation — afterRequest handler at app.js:5541+ silently loses the redirect-follow behavior.');
  }
});

test('eventTargetElement helper is still wired in app.js', () => {
  // The helper introduced in slice 3 narrows Document / Window
  // event handlers' event.target to Element. Loss of the helper
  // re-emits 53+ TS2339 sites in the DOMContentLoaded click block
  // (every `event.target.closest(...)` becomes
  // `event.target.closest does not exist on EventTarget`). Pin
  // the helper definition + at least one use site.
  if (!app.includes('const eventTargetElement = ')) {
    throw new Error('`eventTargetElement` helper missing from app.js — see comment block at the top of the IIFE for the rationale.');
  }
  if (!app.includes('eventTargetElement(event)')) {
    throw new Error('`eventTargetElement(event)` not invoked anywhere — helper was defined but never wired.');
  }
});

test('app.js still uses the type-narrowed form / input checks introduced in slice 2 + 3', () => {
  // Spot-check 3 narrowing invariants introduced since slice 1.
  // These are the highest-signal slice-3 changes: if a future
  // refactor relaxes them, the runtime re-gains the latent
  // Element-not-narrow bug class.
  const anchors = [
    'first instanceof HTMLInputElement',           // formIsNewSoldierWithEmptyNames helper
    'el instanceof HTMLInputElement',              // share-queue page-select forEach
    'select instanceof HTMLSelectElement',         // export-template modal load-template path
  ];
  for (const anchor of anchors) {
    if (!app.includes(anchor)) {
      throw new Error(`expected narrowing \`${anchor}\` missing from app.js — slice-3 narrowing was relaxed and the type-checker error count would climb back up.`);
    }
  }
});

console.log(`\n  typecheck_augmentations regression net: ${pass} passed, ${fail} failed`);
if (fail > 0) {
  process.exit(1);
}
