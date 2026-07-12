/**
 * audit/smoke_archive_soft_only.mjs — RED-first regression net for
 * issue #494 (Soft = new default; Default renamed "Classic"; HTML
 * archive ships Soft-only).
 *
 * Source-scan probe — no live server needed. Reads
 * `internal/archive/static_archive.go` and asserts the archive's
 * HTML template carries the Soft palette at :root, has NO theme
 * picker UI, NO per-archive localStorage theme key, NO
 * high-contrast or classic override blocks, and NO legacy
 * Default-palette tokens.
 *
 * The static archive's HTML is rendered via a Go html/template
 * constant string; the same file is the source of truth. A
 * regression that re-adds the picker or strips the Soft palette
 * fails the corresponding test. The companion Go test in
 * `internal/archive/static_archive_theme_test.go` exercises the
 * actual `renderStaticArchiveIndex` function and pins the same
 * surface; this probe provides a lighter-weight assertion for
 * the CI audit step.
 */

import { readFileSync, existsSync } from 'node:fs';
import { strict as assert } from 'node:assert';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');

const STATIC_ARCHIVE_GO = join(ROOT, 'internal', 'archive', 'static_archive.go');

let pass = 0;
let fail = 0;

function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  PASS ${name}`);
  } catch (err) {
    fail++;
    console.log(`  FAIL ${name}`);
    console.log(`    ${err.message}`);
  }
}

if (!existsSync(STATIC_ARCHIVE_GO)) {
  console.error(`fatal: missing ${STATIC_ARCHIVE_GO}`);
  process.exit(2);
}

const src = readFileSync(STATIC_ARCHIVE_GO, 'utf8');

test('soft-src-01 archive bakes data-theme="soft" on <html>', () => {
  assert.ok(
    src.includes('data-theme="soft"'),
    'static_archive.go must stamp data-theme="soft" on <html>; the legacy "default" stamp was removed in issue #494',
  );
});

test('soft-src-02 archive carries NO theme picker UI', () => {
  for (const needle of [
    'data-theme-pick="default"',
    'data-theme-pick="high-contrast"',
    'data-theme-pick="soft"',
    'class="theme-picker"',
    'class="theme-pick"',
  ]) {
    assert.ok(
      !src.includes(needle),
      `static_archive.go must NOT carry ${needle} (theme picker removed in #494)`,
    );
  }
});

test('soft-src-03 archive carries NO per-archive localStorage theme key', () => {
  assert.ok(
    !src.includes('dixiedata.static.theme:'),
    'static_archive.go must NOT reference dixiedata.static.theme: (localStorage key removed in #494)',
  );
});

test('soft-src-04 archive has Soft palette tokens at :root', () => {
  for (const needle of [
    ':root {',
    '--paper: #f4ecd8;',
    '--gold: #8d7440;',
    '--ink: #3b2a1a;',
  ]) {
    assert.ok(
      src.includes(needle),
      `static_archive.go must carry Soft palette token ${needle} at :root`,
    );
  }
});

test('soft-src-05 archive carries NO html[data-theme="high-contrast"] block', () => {
  assert.ok(
    !src.includes('html[data-theme="high-contrast"]'),
    'static_archive.go must NOT carry html[data-theme="high-contrast"] block (Soft-only bundle since #494)',
  );
});

test('soft-src-06 archive carries NO html[data-theme="default"] block', () => {
  assert.ok(
    !src.includes('html[data-theme="default"]'),
    'static_archive.go must NOT carry html[data-theme="default"] block (Soft-only bundle since #494)',
  );
});

test('soft-src-07 archive carries NO legacy Default-palette tokens', () => {
  for (const needle of ['--paper: #d7d2c9;', '--gold: #a88a46;']) {
    assert.ok(
      !src.includes(needle),
      `static_archive.go must NOT carry legacy Default-palette token ${needle} (Soft-only since #494)`,
    );
  }
});

test('soft-src-08 archive carries NO theme-picker JS (no buttons.forEach, no aria-pressed setter, no localStorage.setItem)', () => {
  for (const needle of [
    'document.querySelectorAll(\'[data-theme-pick]\')',
    'localStorage.setItem(storageKey,',
    'aria-pressed',
  ]) {
    assert.ok(
      !src.includes(needle),
      `static_archive.go must NOT carry ${needle} (theme picker JS removed in #494)`,
    );
  }
});

test('soft-src-09 live app Settings picker shows "Classic" label (not "Default")', () => {
  // Read the Settings appearance panel to confirm the user-facing
  // label was renamed. The radio value stays "default" so the
  // CSS attribute semantics are preserved.
  const TEMPL = join(ROOT, 'internal', 'templates', 'entry_form.templ');
  const templ = readFileSync(TEMPL, 'utf8');
  // The picker slice should have an entry with label "Classic".
  assert.ok(
    /\{ "default", "Classic",/.test(templ),
    'entry_form.templ picker must rename the "Default" label to "Classic"',
  );
});

test('soft-src-10 live app ThemeDefault constant is renamed to ThemeClassic', () => {
  const SETTINGS = join(ROOT, 'internal', 'records', 'local_settings.go');
  const ls = readFileSync(SETTINGS, 'utf8');
  assert.ok(
    /ThemeClassic\s*=\s*"default"/.test(ls),
    'local_settings.go must declare ThemeClassic = "default" (constant rename, value preserved)',
  );
  assert.ok(
    !ls.includes('ThemeDefault'),
    'local_settings.go must NOT declare ThemeDefault anymore (rename complete)',
  );
});

test('soft-src-11 live app empty-string fallback resolves to Soft', () => {
  const SETTINGS = join(ROOT, 'internal', 'records', 'local_settings.go');
  const ls = readFileSync(SETTINGS, 'utf8');
  // The ResolvedTheme empty-string branch should return ThemeSoft.
  assert.ok(
    /if s\.Theme == "" \{[\s\S]*?return ThemeSoft/.test(ls),
    'local_settings.go ResolvedTheme empty-string branch must return ThemeSoft (Soft is the new default for fresh installs)',
  );
});

console.log(`\nResults: ${pass} pass, ${fail} fail`);
if (fail > 0) {
  process.exit(1);
}
process.exit(0);