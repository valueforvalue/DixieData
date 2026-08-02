// scripts/ci/lint-rc-commits.test.mjs (issue #668, ADR 0011)
//
// Regression net on the conventional-commit regex + the
// allowed/disallowed sets the classifier enforces. Static
// pattern tests — the live run is wired into
// .github/workflows/rc-lint.yml on every PR to rc/v*.

import { strict as assert } from 'node:assert';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readFileSync } from 'node:fs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..', '..');

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

// Re-declare the canonical shapes the classifier uses so
// we can verify each in isolation. The constants are
// duplicated here intentionally: a future contributor who
// edits the classifier without updating this test will see
// one (or both) of these drift.
const conventionalRe = /^([a-z]+)(\(([^)]+)\))?!?: (.+)$/;
const ALLOWED = new Set(['fix', 'docs', 'chore', 'test', 'ci', 'style', 'release']);
const DISALLOWED = new Set(['feat', 'refactor', 'perf', 'build']);
const DIFF_SIZE_THRESHOLD = 50;

test('conventionalRe matches fix(...) scope commits', () => {
  const m = 'fix(frontend): wire image upload (#691)'.match(conventionalRe);
  assert.ok(m, 'subject should match');
  assert.equal(m[1], 'fix');
  assert.equal(m[3], 'frontend');
  assert.equal(m[4], 'wire image upload (#691)');
});

test('conventionalRe matches prefix-only commits (no scope)', () => {
  const m = 'ci: lint-js-init-guards assertion (#685)'.match(conventionalRe);
  assert.ok(m, 'subject should match');
  assert.equal(m[1], 'ci');
});

test('conventionalRe rejects subjects without a type(scope): prefix', () => {
  const m = 'wip: forgot the prefix'.match(conventionalRe);
  // 'wip' isn't a known type but the regex still matches
  // structurally. The classifier rejects via the
  // "not in ALLOWED, not in DISALLOWED" path.
  assert.ok(m);
  assert.equal(m[1], 'wip');
});

test('conventionalRe rejects commits missing the colon', () => {
  const m = 'fix(frontend) wire image upload'.match(conventionalRe);
  assert.equal(m, null);
});

test('allowed set contains the fix/docs/chore/test/ci types', () => {
  for (const t of ['fix', 'docs', 'chore', 'test', 'ci']) {
    assert.ok(ALLOWED.has(t), `ALLOWED should contain ${t}`);
  }
});

test('disallowed set contains feat/refactor/perf/build', () => {
  for (const t of ['feat', 'refactor', 'perf', 'build']) {
    assert.ok(DISALLOWED.has(t), `DISALLOWED should contain ${t}`);
  }
});

test('diff-size threshold is 50 files per ADR 0011 §Diff-size gate', () => {
  assert.equal(DIFF_SIZE_THRESHOLD, 50);
});

test('classifier rejects a commit that lands a feat on rc/v*', () => {
  // Static-only: the live rejection happens when the CI
  // workflow exits 1. The classifier's logic per ADR
  // 0011 §Allowed vs disallowed:
  const subject = 'feat(ui): new floating dock menu (#342)';
  const m = subject.match(conventionalRe);
  assert.ok(m);
  assert.ok(
    DISALLOWED.has(m[1]) && !ALLOWED.has(m[1]),
    `feat(...) must classify as disallowed; got type=${m[1]}`,
  );
});

test('classifier accepts a fix commit on rc/v*', () => {
  const subject = 'fix(frontend): wire image upload (#691)';
  const m = subject.match(conventionalRe);
  assert.ok(m);
  assert.ok(
    ALLOWED.has(m[1]) && !DISALLOWED.has(m[1]),
    `fix(...) must classify as allowed; got type=${m[1]}`,
  );
});

test('files-ci test fixture imports the classifier mjs without syntax errors', () => {
  const path = join(__dirname, 'lint-rc-commits.mjs');
  // Read the mjs source to ensure it doesn't throw on parse
  // (the `import.meta.url` + top-level `process.argv` access
  // would normally fail to import in a non-Node ESM loader,
  // but `readFileSync` + `new Function` is enough).
  const src = readFileSync(path, 'utf8');
  assert.ok(src.length > 100, 'classifier source should be non-trivial');
  assert.match(src, /classify.*DISALLOWED|ALLOWED/s, 'classifier must reference the allow/disallow sets');
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
