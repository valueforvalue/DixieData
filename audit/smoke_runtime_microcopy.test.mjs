// audit/smoke_runtime_microcopy.test.mjs
//
// Regression tests for the issue #581 runtime microcopy gate
// (slice 4, audit-only). The probe is GREEN-on-HEAD; this
// test file pins the GREEN state plus synthetic regressions
// for each rule family.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/smoke_runtime_microcopy.mjs');

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

function runProbe(source, strict = false) {
  return spawnSync('node', [PROBE, ...(strict ? ['--strict'] : [])], {
    encoding: 'utf8',
    env: { ...process.env, RUNTIME_SOURCE: source },
  });
}

function withTempSource(body, fn) {
  const dir = mkdtempSync(join(tmpdir(), 'runtime-microcopy-'));
  const source = join(dir, 'synthetic.js');
  writeFileSync(source, body);
  try {
    return fn(source);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

const CANONICAL_SOURCE = [
  '// canonical runtime producer -- every pinned string present',
  '',
  '// top-traffic toasts',
  'showToast("Saved local draft restored.", "success");',
  'showToast("Path copied.", "success");',
  'showToast("No path to copy.", "error");',
  'showToast("Could not load print options.", "error");',
  'showToast("Browse refresh failed.", "error");',
  'showToast("Choose exactly two records to compare.", "error");',
  'showToast("Nothing to copy.", "error");',
  'showToast("Clipboard helper unavailable.", "error");',
  'showToast("Preview content was not available.", "error");',
  '',
  '// startup placeholder',
  '<title>Loading DixieData...</title>',
  'text-2xl font-semibold text-[var(--theme-text-primary)]">Loading DixieData...</p>',
  'The local archive is still starting up. This screen will refresh automatically.',
  '',
  '// CLI help',
  'DixieData CLI \u2014 headless archive operations',
  'dixiedata <subcommand> [flags]',
  'See docs/agents/cli-plan.md for the full roadmap.',
  '',
].join('\n');

// --- baseline-on-HEAD --------------------------------------------------

test('probe reports 0 baseline findings on current repo (GREEN-on-HEAD)', () => {
  const result = runProbe(undefined, false);
  // When RUNTIME_SOURCE is unset the probe scans the real
  // repo. Use --strict to make sure the real repo passes too.
  assert.equal(result.status, 0, `expected informational exit 0\nstdout: ${result.stdout}`);
  assert.match(
    result.stdout,
    /Required copy findings: 0/,
    `expected 0 required findings on HEAD; got:\n${result.stdout}`,
  );
  assert.match(
    result.stdout,
    /Forbidden\/verbose copy findings: 0/,
    `expected 0 forbidden findings on HEAD; got:\n${result.stdout}`,
  );
});

test('probe exits 0 under --strict on current repo (GREEN-on-HEAD)', () => {
  const result = runProbe(undefined, true);
  assert.equal(result.status, 0, `expected strict exit 0 on HEAD\nstdout: ${result.stdout}`);
  assert.match(result.stdout, /Runtime microcopy sweep: clean/);
});

// --- synthetic positive: missing-toast fixture ------------------------

test('synthetic missing-toast fixture fails the required rule', () => {
  withTempSource(
    CANONICAL_SOURCE.replace('showToast("Path copied.", "success");', '/* removed */'),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /Path copied\./);
    },
  );
});

test('synthetic missing-startup-heading fixture fails the required rule', () => {
  // Strip only the full markup line so the title-required
  // stays green and the body-heading-required fires on its
  // own.
  const mutated = CANONICAL_SOURCE.replace(
    '\ntext-2xl font-semibold text-[var(--theme-text-primary)]">Loading DixieData...</p>\n',
    '\nLoading app...\n',
  );
  withTempSource(mutated, (source) => {
    const result = runProbe(source, true);
    assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
    assert.match(result.stdout, /startup: body heading/);
  });
});

test('synthetic missing-CLI-help-opener fixture fails the required rule', () => {
  withTempSource(
    CANONICAL_SOURCE.replace('DixieData CLI \u2014 headless archive operations', 'CLI help'),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /cli: DixieData CLI opener/);
    },
  );
});

// --- synthetic positive: forbidden drift fixtures ---------------------

test('synthetic status-form-empty-toast fixture fails the forbidden rule', () => {
  withTempSource(
    CANONICAL_SOURCE + '\nshowToast("No records yet.", "info");\n',
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /status-form empty copy/);
    },
  );
});

// --- synthetic positive: full canonical passes -------------------------

test('synthetic fully-canonical fixture passes the strict gate', () => {
  withTempSource(CANONICAL_SOURCE, (source) => {
    const result = runProbe(source, true);
    assert.equal(result.status, 0, `expected strict exit 0\nstdout: ${result.stdout}`);
  });
});

// --- synthetic positive: header-only fixture (smoke) -------------------

test('synthetic header-only fixture exits 1 under --strict (catches missing required)', () => {
  withTempSource('// a near-empty file with no pinned strings\n', (source) => {
    const result = runProbe(source, true);
    assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
    assert.match(result.stdout, /Required copy findings: [1-9]/);
  });
});

// --- strict mode -------------------------------------------------------

test('probe exits 0 without --strict even when findings exist', () => {
  withTempSource('// empty\n', (source) => {
    const result = runProbe(source, false);
    assert.equal(result.status, 0, `expected informational exit 0\nstdout: ${result.stdout}`);
    assert.match(result.stdout, /informational findings/);
  });
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
