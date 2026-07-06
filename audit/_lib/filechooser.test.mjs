// audit/_lib/filechooser.test.mjs — unit-style assertions for
// setFileChooserFixture. Runs without spinning up a live server;
// uses a fake Page that records listener attachments and lets the
// test simulate a `filechooser` event by invoking the listener
// directly. This is the RED-first regression net for the helper
// itself; the end-to-end smoke coverage lives in
// `audit/smoke_events.mjs` step-12 (issue #385 + #348b).

import { strict as assert } from 'node:assert';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { setFileChooserFixture } from './filechooser.mjs';

let pass = 0;
let fail = 0;
function test(name, fn) {
  return Promise.resolve()
    .then(fn)
    .then(() => {
      pass++;
      console.log(`  ✓ ${name}`);
    })
    .catch((err) => {
      fail++;
      console.log(`  ✗ ${name}`);
      console.log(`    ${err.stack || err.message}`);
    });
}

/**
 * Minimal fake of the Playwright Page surface the helper actually
 * touches: `on`, `off`, and an emit helper. The helper only ever
 * calls `page.on('filechooser', handler)` and `page.off(...)`,
 * so we don't need the full Page contract — just enough to
 * observe registration and to drive the listener from the test.
 */
function makeFakePage() {
  const listeners = new Map();
  return {
    listeners,
    on(event, fn) {
      if (!listeners.has(event)) listeners.set(event, []);
      listeners.get(event).push(fn);
      return this;
    },
    off(event, fn) {
      const list = listeners.get(event);
      if (!list) return this;
      const i = list.indexOf(fn);
      if (i >= 0) list.splice(i, 1);
    },
    /** Test helper: pretend the page emitted a `filechooser`. */
    emitFileChooser(paths) {
      const list = listeners.get('filechooser') || [];
      const fc = {
        async setFiles(p) {
          fc.lastSetFiles = p;
        },
      };
      // Fire-and-forget the handlers so an async setFiles
      // resolves before the test asserts.
      return Promise.all(list.map((fn) => fn(fc))).then(() => fc);
    },
  };
}

// Fixtures on disk so the helper's "normalize to string[]" path
// sees real strings (not just any-typed JS values).
const tmp = mkdtempSync(join(tmpdir(), 'filechooser-test-'));
const fixtureA = join(tmp, 'a.png');
const fixtureB = join(tmp, 'b.jpg');
writeFileSync(fixtureA, 'fake-png');
writeFileSync(fixtureB, 'fake-jpg');

try {
  await Promise.all([
    test('attaches a filechooser listener exactly once', () => {
      const page = makeFakePage();
      setFileChooserFixture(page, fixtureA);
      assert.equal(page.listeners.get('filechooser').length, 1);
    }),

    test('single string path is normalized to a one-element array', async () => {
      const page = makeFakePage();
      setFileChooserFixture(page, fixtureA);
      const fc = await page.emitFileChooser();
      assert.deepEqual(fc.lastSetFiles, [fixtureA]);
    }),

    test('array of paths is passed through unchanged', async () => {
      const page = makeFakePage();
      setFileChooserFixture(page, [fixtureA, fixtureB]);
      const fc = await page.emitFileChooser();
      assert.deepEqual(fc.lastSetFiles, [fixtureA, fixtureB]);
    }),

    test('is idempotent: second call replaces, does not stack', async () => {
      const page = makeFakePage();
      const off1 = setFileChooserFixture(page, fixtureA);
      setFileChooserFixture(page, [fixtureA, fixtureB]);
      assert.equal(page.listeners.get('filechooser').length, 1);
      const fc = await page.emitFileChooser();
      assert.deepEqual(fc.lastSetFiles, [fixtureA, fixtureB]);
      // off1 is now a no-op because the handler it detached was
      // replaced; calling it must not throw and must not detach
      // the current handler.
      off1();
      assert.equal(page.listeners.get('filechooser').length, 1);
    }),

    test('unsubscribe detaches the handler', () => {
      const page = makeFakePage();
      const off = setFileChooserFixture(page, fixtureA);
      assert.equal(page.listeners.get('filechooser').length, 1);
      off();
      assert.equal(page.listeners.get('filechooser').length, 0);
      // Idempotent unsubscribe: calling off() again is a no-op.
      off();
      assert.equal(page.listeners.get('filechooser').length, 0);
    }),

    test('two pages do not share handlers', async () => {
      const pageA = makeFakePage();
      const pageB = makeFakePage();
      setFileChooserFixture(pageA, fixtureA);
      setFileChooserFixture(pageB, fixtureB);
      const fcA = await pageA.emitFileChooser();
      const fcB = await pageB.emitFileChooser();
      assert.deepEqual(fcA.lastSetFiles, [fixtureA]);
      assert.deepEqual(fcB.lastSetFiles, [fixtureB]);
    }),

    test('returns a function (unsubscribe)', () => {
      const page = makeFakePage();
      const ret = setFileChooserFixture(page, fixtureA);
      assert.equal(typeof ret, 'function');
      ret();
    }),
  ]);
} finally {
  try { rmSync(tmp, { recursive: true, force: true }); } catch (_) { /* best effort */ }
}

console.log(`\nfilechooser helper: ${pass} passed, ${fail} failed`);
if (fail > 0) process.exit(1);

// Sanity check: barrel re-export still resolves and matches the
// helper's direct export. Catches a future refactor that breaks
// the barrel without breaking the helper itself.
const barrel = await import('./index.mjs');
assert.equal(typeof barrel.setFileChooserFixture, 'function');
assert.equal(barrel.setFileChooserFixture, setFileChooserFixture);
process.exit(0);