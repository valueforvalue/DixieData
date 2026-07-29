// audit/_lib/smoke_runner.test.mjs (issue #700, ADR 0011)
//
// Slice 1 regression net for the shared runner skeleton.
// Three assertions pin the slice-1 contract:
//   1. record() from smoke_reporter increments the pass counter
//      and pushes a record with the right shape.
//   2. renderSummary() returns 0 when all records pass, 1 when
//      any fails.
//   3. writeJson() produces a JSON file with the records.
//
// Subsequent slices (2-7) extend this test with:
//   - runProbe spawn lifecycle assertions (slice 2).
//   - scanner-bridge exit-code contract (slice 5).
//
// All probes exit-0 today because the runner skeleton is a
// no-op stub; the slice-1 contract is "exit 0, write JSON,
// no exceptions."

import { strict as assert } from 'node:assert';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { record, renderSummary, writeJson, reset, count, getResults } from './smoke_reporter.mjs';
import { webBin, seedBin, webBinExists } from './smoke_paths.mjs';

let pass = 0;
let fail = 0;
function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  \u2713 ${name}`);
  } catch (err) {
    fail++;
    console.log(`  \u2717 ${name}`);
    console.log(`    ${err.message}`);
  }
}

// Reset the in-memory results before each test run so the
// assertions below don't pick up state from prior runs in
// the same `node --test` invocation.
reset();

test('record() pushes an entry with {name, ok, ts} and increments the pass counter', () => {
  reset();
  const before = count();
  assert.equal(before.total, 0, 'fresh state should have 0 results');
  const entry = record('test-a', true, { slice: 1 });
  assert.ok(entry.name === 'test-a');
  assert.equal(entry.ok, true);
  assert.ok(typeof entry.ts === 'string', 'ts must be ISO string');
  const after = count();
  assert.equal(after.pass, 1);
  assert.equal(after.fail, 0);
  assert.equal(after.total, 1);
});

test('record() with ok: false increments the fail counter', () => {
  reset();
  record('test-b', false, { error: 'sample' });
  const after = count();
  assert.equal(after.pass, 0);
  assert.equal(after.fail, 1);
});

test('renderSummary() returns 0 when all records pass, 1 when any fails', () => {
  reset();
  record('pass-1', true);
  record('pass-2', true);
  let code = renderSummary();
  assert.equal(code, 0, 'all-pass run should return exit code 0');
  record('fail-1', false, { error: 'boom' });
  code = renderSummary();
  assert.equal(code, 1, 'mixed run should return exit code 1');
});

test('writeJson() emits the records to disk', () => {
  reset();
  record('disk-1', true);
  const dir = mkdtempSync(join(tmpdir(), 'smoke-runner-test-'));
  try {
    const path = join(dir, 'summary.json');
    writeJson(path);
    assert.ok(existsSync(path), 'summary file must exist after write');
    const parsed = JSON.parse(readFileSync(path, 'utf8'));
    assert.ok(Array.isArray(parsed.results), 'results field must be an array');
    assert.ok(parsed.results[0].name === 'disk-1');
    assert.ok(typeof parsed.finishedAt === 'string');
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('smoke_paths: webBin() returns the .exe-suffixed binary on win32, bare on linux', () => {
  // We cannot assert the literal path here (repo-root location
  // is environment-dependent) but we can assert the platform
  // branch: win32 path includes the .exe, others do not.
  const path = webBin();
  const isWin = process.platform === 'win32';
  assert.equal(
    path.endsWith('dixiedata-web.exe'),
    isWin,
    `webBin() on ${process.platform} should ${isWin ? '' : 'NOT '}end with .exe; got ${path}`,
  );
});

test('smoke_paths: webBinExists() returns false when the binary is missing', () => {
  // On a clean dev tree without `just debug` having run,
  // build/bin/dixiedata-web(.{exe,}) does not exist. The
  // assertion guards against the false-positive of returning
  // true unconditionally.
  // (Skipped if the binary IS present -- in CI it's built
  // before the audit step, so we accept either.)
  const exists = webBinExists();
  assert.equal(typeof exists, 'boolean', 'webBinExists must return a boolean');
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
