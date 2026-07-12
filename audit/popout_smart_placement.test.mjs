import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const APP_JS = join(ROOT, 'frontend/app.js');

const content = readFileSync(APP_JS, 'utf8');

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

// Issue #476: top-nav foldouts/megamenus/dock panel get cut off
// at the left edge on narrow windows. The fix centralizes popout
// placement in a single shared helper so every popout surface
// follows the same smart-placement rules. The pre-#476 behavior
// had clampPopoutPanels only targeting [data-popout-panel] and
// the open() handlers in installFoldouts + installMegaMenus never
// invoked any placement helper — so a panel extending past the
// left edge on a narrow window had no chance to flip or shift.

test('a shared popout placement helper exists in app.js', () => {
  // The new helper computes position via getBoundingClientRect on
  // the trigger, picks the right placement based on available
  // viewport space, and applies inline styles so the panel stays
  // fully inside the viewport no matter where the trigger sits.
  // We accept either placePopoutPanel or placePopout as the name;
  // the test pins the existence + placement logic, not the exact
  // spelling.
  const placePopoutPanel = content.match(/function\s+placePopoutPanel\s*\(/);
  const placePopout = content.match(/function\s+placePopout\s*\(/);
  if (!placePopoutPanel && !placePopout) {
    throw new Error(
      'expected a `placePopoutPanel(trigger, panel)` (or `placePopout`) helper in app.js — ' +
      'centralized popover placement is the fix for #476. If you intentionally chose a ' +
      'different name, update this test to match.',
    );
  }
});

test('placePopoutPanel reads getBoundingClientRect on the trigger', () => {
  // The helper must measure the trigger so it knows where the
  // panel would overflow.
  const fnMatch = content.match(/function\s+(placePopoutPanel|placePopout)\s*\([\s\S]*?\n\}/);
  if (!fnMatch) {
    throw new Error('placePopoutPanel function body not found — check the function is properly closed');
  }
  const body = fnMatch[0];
  if (!body.includes('getBoundingClientRect')) {
    throw new Error('placePopoutPanel must call getBoundingClientRect on the trigger to measure available viewport space');
  }
});

test('placePopoutPanel applies a horizontal transform when the panel would overflow the left edge', () => {
  // The core smart-placement rule: if the panel's right edge is
  // past the trigger's left edge (panel would clip the viewport's
  // left edge), shift the panel right via transform so it stays
  // inside the viewport. We accept either translateX (simple) or
  // translate3d (preferred — keeps the transform on its own
  // compositing layer).
  const fnMatch = content.match(/function\s+(placePopoutPanel|placePopout)\s*\([\s\S]*?\n\}/);
  if (!fnMatch) {
    throw new Error('placePopoutPanel function body not found');
  }
  const body = fnMatch[0];
  if (!body.includes('transform') || !/translate(?:X|3d)/i.test(body)) {
    throw new Error('placePopoutPanel must apply a translateX or translate3d transform when the panel would overflow horizontally');
  }
});

test('foldout open() invokes the placement helper', () => {
  // installFoldouts' open() function must call placePopoutPanel
  // after panel.classList.remove("hidden") so the panel is
  // measured + clamped as soon as it becomes visible.
  const fnMatch = content.match(/function\s+installFoldouts\s*\([\s\S]*?\n  function\s+/);
  if (!fnMatch) {
    throw new Error('installFoldouts function not found');
  }
  const fn = fnMatch[0];
  // The open() helper inside installFoldouts sets
  // panel.classList.remove("hidden"); look for the placement call
  // after that line.
  const openIdx = fn.indexOf('panel.classList.remove("hidden")');
  if (openIdx < 0) {
    throw new Error('installFoldouts.open() does not show the panel — refactor must have changed the structure');
  }
  const tail = fn.slice(openIdx);
  if (!/(placePopoutPanel|placePopout)\s*\(\s*trigger\s*,\s*panel\s*\)/.test(tail)) {
    throw new Error(
      'installFoldouts.open() must invoke placePopoutPanel(trigger, panel) (or equivalent) ' +
      'after panel.classList.remove("hidden") so the panel is measured + clamped on open',
    );
  }
});

test('megamenu open() invokes the placement helper', () => {
  // Same rule for installMegaMenus.open().
  const fnMatch = content.match(/function\s+installMegaMenus\s*\([\s\S]*?\n  function\s+/);
  if (!fnMatch) {
    throw new Error('installMegaMenus function not found');
  }
  const fn = fnMatch[0];
  const openIdx = fn.indexOf('panel.classList.remove("hidden")');
  if (openIdx < 0) {
    throw new Error('installMegaMenus.open() does not show the panel — refactor must have changed the structure');
  }
  const tail = fn.slice(openIdx);
  if (!/(placePopoutPanel|placePopout)\s*\(\s*trigger\s*,\s*panel\s*\)/.test(tail)) {
    throw new Error(
      'installMegaMenus.open() must invoke placePopoutPanel(trigger, panel) (or equivalent) ' +
      'after panel.classList.remove("hidden") so the panel is measured + clamped on open',
    );
  }
});

console.log(`\n  popout_smart_placement regression net: ${pass} passed, ${fail} failed`);
if (fail > 0) {
  process.exit(1);
}