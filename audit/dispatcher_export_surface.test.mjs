// Regression net for issue #534: the dispatcher consults the
// per-page <html data-export-surface="..."> attribute before
// calling window.location.assign(redirectTo). When the user
// preference is "toast-only" the dispatcher suppresses the
// post-export navigation; the toast still fires.
//
// This test scans frontend/app.js and asserts:
//   1. The dispatcher reads document.documentElement.dataset.exportSurface
//      (the contract that the layout's <html data-export-surface>
//      attribute makes visible).
//   2. The dispatcher suppresses window.location.assign when the
//      value is "toast-only".
//   3. The dispatcher's surface read happens BEFORE the
//      window.location.assign call (order matters).
//
// Like dispatcher_patch_method.test.mjs, this is a source-scan
// test rather than a Playwright eval against a live server
// (the Wails harness can't easily drive a form submission
// across the X-DixieData-Redirect boundary). The scan pins
// the source-level contract; a future browser-harness test
// could exercise the same paths against a live server.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const dispatcherPath = resolve(here, '..', 'frontend', 'app.js');
const dispatcher = readFileSync(dispatcherPath, 'utf8');

test('dispatcher reads document.documentElement.dataset.exportSurface (issue #534)', () => {
  assert.match(
    dispatcher,
    /document\.documentElement\.dataset\.exportSurface/,
    'dispatcher must read the <html data-export-surface="..."> attribute so the JS consults the same source the layout writes (issue #534)',
  );
});

test('dispatcher suppresses window.location.assign when surface is toast-only (issue #534)', () => {
  // Find the redirect-suppression block by looking for the
  // exportSurface read and the suppressNav boolean together.
  assert.match(
    dispatcher,
    /exportSurface\s*===\s*['"]toast-only['"]/,
    'dispatcher must compare exportSurface against the literal "toast-only" string (issue #534)',
  );
  assert.match(
    dispatcher,
    /suppressNav/,
    'dispatcher must compute a suppressNav boolean before deciding whether to assign the redirect (issue #534)',
  );
});

test('dispatcher reads exportSurface BEFORE window.location.assign (order matters for issue #534)', () => {
  const readAt = dispatcher.indexOf('document.documentElement.dataset.exportSurface');
  const assignAt = dispatcher.indexOf('window.location.assign(redirectTo)');
  assert.ok(readAt > 0, 'exportSurface read must exist in dispatcher');
  assert.ok(assignAt > 0, 'window.location.assign(redirectTo) must exist in dispatcher');
  assert.ok(
    readAt < assignAt,
    `exportSurface read at ${readAt} must precede window.location.assign at ${assignAt} (issue #534)`,
  );
});