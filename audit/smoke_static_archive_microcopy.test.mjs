// audit/smoke_static_archive_microcopy.test.mjs
//
// Regression tests for the issue #581 Static Archive microcopy gate.
// The probe scans the rendered HTML/JS raw string inside
// internal/archive/static_archive.go. Keep this test source-level: the
// format is self-contained and does not need a running server.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/smoke_static_archive_microcopy.mjs');
const SOURCE = join(ROOT, 'internal/archive/static_archive.go');

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

function runProbe(source = SOURCE, strict = false) {
  return spawnSync('node', [PROBE, ...(strict ? ['--strict'] : [])], {
    encoding: 'utf8',
    env: { ...process.env, STATIC_ARCHIVE_SOURCE: source },
  });
}

function withTempSource(mutator, fn) {
  const dir = mkdtempSync(join(tmpdir(), 'static-archive-microcopy-'));
  const source = join(dir, 'static_archive.go');
  try {
    const original = readFileSync(SOURCE, 'utf8');
    writeFileSync(source, mutator(original));
    return fn(source);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

test('probe passes current Static Archive copy in strict mode', () => {
  const result = runProbe(SOURCE, true);
  assert.equal(
    result.status,
    0,
    `expected clean Static Archive copy\nstdout: ${result.stdout}\nstderr: ${result.stderr}`,
  );
});

test('probe catches reintroduced verbose hero copy', () => {
  withTempSource(
    (source) => source.replace(
      'Read-only Static Archive export.',
      'Browse this standalone DixieData archive as a read-only mirror of the DixieData app. The Calendar landing shows every anniversary and event day in the archive; use the nav to jump to Filter (filterable Person Record list), Insights (analytics snapshot), or the Event / Article tabs.',
    ),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /hero description/i);
    },
  );
});

test('probe catches duplicated report instructions', () => {
  withTempSource(
    (source) => source.replace(
      'title="Print this report"',
      'title="Open the printable report in a new tab. Use your browser\'s Print → PDF to save it."',
    ),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /duplicated report instructions/i);
    },
  );
});

test('probe catches terminology drift in Static Archive output', () => {
  withTempSource(
    (source) => source.replace('Source Records</h4>', 'Records</h4>'),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /source records heading/i);
    },
  );
});

test('probe catches removal of required concise copy', () => {
  withTempSource(
    (source) => source.replace('Read-only Static Archive export.', 'Static Archive export.'),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /required copy/i);
    },
  );
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
