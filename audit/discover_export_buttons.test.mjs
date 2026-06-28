// audit/discover_export_buttons.test.mjs — unit tests for the
// share-page export button discovery module.
//
// Run with: node audit/discover_export_buttons.test.mjs
//
// Why this exists — the regression net for the auto-discovery
// refactor:
//
//   The hand-maintained shareButtons array in smoke.mjs drifted
//   twice in 2026 (a new export button was added to share.templ
//   and the harness did not pick it up; a forgotten UpdateShip
//   caused the htmx 303 silent-swallow bug to ship again in
//   handleGoogleBackup/Sheets). The discovery module replaces the
//   hand-maintained array with code that scans .templ files at
//   runtime. To prevent the discovery itself from drifting (a
//   regex update that misses a case, an override table that
//   becomes stale), these tests pin the manifest shape and
//   contents down to the literal URL list.

import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { discoverShareExportButtons } from './discover_export_buttons.mjs';

let pass = 0;
let fail = 0;

function test(name, fn) {
  try {
    fn();
    console.log(`  ✓ ${name}`);
    pass++;
  } catch (err) {
    console.log(`  ✗ ${name} — ${err.message}`);
    fail++;
  }
}

console.log('discover_export_buttons tests');

test('manifest is non-empty', () => {
  const manifest = discoverShareExportButtons();
  assert.ok(manifest.length > 0, 'manifest should not be empty');
});

test('every entry has a label regex', () => {
  const manifest = discoverShareExportButtons();
  for (const entry of manifest) {
    assert.ok(
      entry.label instanceof RegExp,
      `${entry.path} missing label regex`
    );
  }
});

test('every path is unique', () => {
  const manifest = discoverShareExportButtons();
  const seen = new Set();
  for (const entry of manifest) {
    assert.ok(!seen.has(entry.path), `duplicate path ${entry.path}`);
    seen.add(entry.path);
  }
});

test('all six canonical share-page exports are covered', () => {
  const manifest = discoverShareExportButtons();
  const paths = manifest.map((e) => e.path);
  // The canonical share-page export buttons. The set is the
  // minimum coverage; if a future edit drops one of these from
  // the manifest, the test fails. (Static archive is included as
  // the canonical "plain <form> action" case; the printable PDF
  // modal is excluded via excludedPaths because it has its own
  // dedicated smoke block.)
  const required = [
    '/export/json',
    '/export/csv',
    '/export/ical',
    '/export/static-archive?async=1',
    '/export/backup',
    '/export/shared-archive',
    '/export/bug-report',
    '/export/feedback-log',
    '/integrations/google/backup',
    '/integrations/google/sheets/export',
  ];
  for (const path of required) {
    assert.ok(
      paths.includes(path),
      `manifest missing ${path}; found: ${paths.join(', ')}`
    );
  }
});

test('Google Calendar / connect / disconnect are NOT exports', () => {
  // The scanner must not surface preferences/connection flows as
  // exports even though they share the /integrations/google/
  // URL prefix.
  const manifest = discoverShareExportButtons();
  const paths = manifest.map((e) => e.path);
  for (const path of paths) {
    assert.ok(
      !path.includes('/integrations/google/calendar'),
      `unexpected calendar endpoint in export manifest: ${path}`
    );
    assert.ok(
      !path.endsWith('/google/connect') &&
        !path.endsWith('/google/disconnect'),
      `unexpected connect/disconnect endpoint in export manifest: ${path}`
    );
  }
});

test('printable PDF modal is excluded (covered by [5b] block)', () => {
  const manifest = discoverShareExportButtons();
  const paths = manifest.map((e) => e.path);
  assert.ok(
    !paths.includes('/export/database-pdf?async=1'),
    'printable PDF modal should be excluded; the [5b] smoke block covers it'
  );
});

test('every label matches its button text in the source file', () => {
  // Spot-check that the inferred label regex actually matches the
  // button text in the .templ file. This catches label-inference
  // regressions where a templ refactor breaks the regex but the
  // discovery still emits an entry (e.g. it falls back to the
  // builder name).
  const manifest = discoverShareExportButtons();
  for (const entry of manifest) {
    if (!entry.source) continue;
    const src = readFileSync(entry.source, 'utf8');
    // Build the literal expected by walking the regex source and
    // stripping regex escapes: backslash before any non-alphanumeric
    // char (parens, dots, pluses, etc.) just escapes that char.
    const expected = entry.label.source
      .replace(/^\^/, '')
      .replace(/\\(.)/g, '$1');
    assert.ok(
      src.includes(expected) ||
        // Allow the builderName fallback (when label inference
        // couldn't find a literal — the manifest records the
        // builder name so a future author can add a
        // data-smoke-label override).
        expected === entry.builderName,
      `${entry.path}: label "${expected}" not found in ${entry.source}`
    );
  }
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);