import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const src = readFileSync(join(root, 'frontend/app.js'), 'utf8');
let pass = 0;
let fail = 0;
function test(name, fn) {
  try { fn(); pass++; console.log(`  ✓ ${name}`); }
  catch (err) { fail++; console.log(`  ✗ ${name}\n    ${err.message}`); }
}

test('dispatcher reads method from attribute', () => {
  assert.doesNotMatch(src, /form\.method\.toUpperCase\s*\(\s*\)/);
  assert.match(src, /getAttribute\(\s*["']method["']\s*\)/);
});

test('dispatcher retains Wails method and FormData workarounds', () => {
  const idx = src.indexOf('async function dispatchDixieDataForm');
  assert.ok(idx >= 0);
  const window = src.slice(idx);
  assert.match(window, /X-HTTP-Method-Override/);
  assert.match(window, /wails\.localhost/);
  assert.match(window, /new\s+URLSearchParams\s*\(/);
  assert.match(window, /application\/x-www-form-urlencoded/);
  assert.match(window, /params\.toString\s*\(\s*\)/);
  assert.match(window, /window\.location\.hostname\s*===?\s*["']wails\.localhost["']/);
});

test('dispatcher bails disabled submitter', () => {
  const idx = src.indexOf('async function dispatchDixieDataForm');
  assert.match(src.slice(idx, idx + 5000), /submitter\s+instanceof\s+HTMLButtonElement\s*&&\s*submitter\.disabled/);
});

test('Wails image upload uses native picker trigger and reloads', () => {
  const idx = src.indexOf('function handleImageUpload');
  assert.ok(idx >= 0);
  const full = src;
  assert.match(full, /wailsNativeUpload/);
  assert.match(full, /preventDefault|addEventListener\(["']click["']/);
  assert.match(full, /new URLSearchParams\(\)/);
  assert.match(full, /window\.location\.reload\(\)/);
  assert.match(full, /Image import failed:/);
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
