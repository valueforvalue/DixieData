// audit/_lib/smoke_runner.test.mjs (issue #700, ADR 0011)
//
// Slice 1 regression net for the shared runner skeleton;
// slice 2 extends with the runProbe contract (probeFn
// invocation, ctx shape, error catching, scratchDir, and
// per-probe cleanup hooks).
//
// Slice 1 contract:
//   1. record() from smoke_reporter increments the pass
//      counter and pushes a record with the right shape.
//   2. renderSummary() returns 0 when all records pass,
//      1 when any fails.
//   3. writeJson() produces a JSON file with the records.
//   4. webBin() returns the platform-appropriate path.
//   5. webBinExists() returns a boolean.
//
// Slice 2 contract (new):
//   6. runProbe invokes probeFn with {page, base, scratchDir,
//      record, registerCleanup}.
//   7. probeFn exceptions yield result {ok: false, error}.
//   8. scratchDir is a real (mkdtemp) directory that exists
//      when probeFn runs AND after it returns.
//   9. registerCleanup(fn) runs the fn after probeFn returns.
//
// All probe exits-0 because the runner skeleton either runs
// the probe fn (slice 2) or no-ops (slice 1). Slice-1 stubs
// recorded "ok:true"; slice 2 actually invokes probeFn.

import { strict as assert } from 'node:assert';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { record, renderSummary, writeJson, reset, count, getResults } from './smoke_reporter.mjs';
import { webBin, seedBin, webBinExists } from './smoke_paths.mjs';
import { runProbe } from './smoke_runner.mjs';

let pass = 0;
let fail = 0;
const failures = [];

function recordTest(name, ok, err) {
  if (ok) {
    pass++;
    console.log(`  \u2713 ${name}`);
  } else {
    fail++;
    console.log(`  \u2717 ${name}`);
    console.log(`    ${err && err.message ? err.message : err}`);
    failures.push(name);
  }
}

async function test(name, fn) {
  try {
    await fn();
    recordTest(name, true);
  } catch (err) {
    recordTest(name, false, err);
  }
}

// --- slice 1 cases ---

await test('record() pushes an entry with {name, ok, ts} and increments the pass counter', async () => {
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

await test('record() with ok: false increments the fail counter', async () => {
  reset();
  record('test-b', false, { error: 'sample' });
  const after = count();
  assert.equal(after.pass, 0);
  assert.equal(after.fail, 1);
});

await test('renderSummary() returns 0 when all records pass, 1 when any fails', async () => {
  reset();
  record('pass-1', true);
  record('pass-2', true);
  let code = renderSummary();
  assert.equal(code, 0, 'all-pass run should return exit code 0');
  record('fail-1', false, { error: 'boom' });
  code = renderSummary();
  assert.equal(code, 1, 'mixed run should return exit code 1');
});

await test('writeJson() emits the records to disk', async () => {
  reset();
  record('disk-1', true);
  const dir = mkdtempSync(join(tmpdir(), 'smoke-runner-test-'));
  try {
    const jsonPath = join(dir, 'summary.json');
    writeJson(jsonPath);
    assert.ok(existsSync(jsonPath), 'summary file must exist after write');
    const parsed = JSON.parse(readFileSync(jsonPath, 'utf8'));
    assert.ok(Array.isArray(parsed.results), 'results field must be an array');
    assert.ok(parsed.results[0].name === 'disk-1');
    assert.ok(typeof parsed.finishedAt === 'string');
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

await test('smoke_paths: webBin() returns the .exe-suffixed binary on win32, bare on linux', async () => {
  const binPath = webBin();
  const isWin = process.platform === 'win32';
  assert.equal(
    binPath.endsWith('dixiedata-web.exe'),
    isWin,
    `webBin() on ${process.platform} should ${isWin ? '' : 'NOT '}end with .exe; got ${binPath}`,
  );
});

await test('smoke_paths: webBinExists() returns a boolean', async () => {
  const exists = webBinExists();
  assert.equal(typeof exists, 'boolean', 'webBinExists must return a boolean');
});

// --- slice 2 cases: runProbe contract ---

await test('runProbe invokes probeFn and threads {page, base, scratchDir, record, registerCleanup}', async () => {
  reset();
  let probeFnEntered = false;
  let observedCtx = null;

  const result = await runProbe({
    name: 'unit-no-browser',
    probeFn: async (ctx) => {
      probeFnEntered = true;
      observedCtx = ctx;
      // Every migrated probe destructures these names; assert
      // they are present and well-typed.
      assert.ok(typeof ctx === 'object', 'ctx must be an object');
      assert.ok('base' in ctx, 'ctx must carry base');
      assert.ok('scratchDir' in ctx, 'ctx must carry scratchDir');
      assert.ok('record' in ctx, 'ctx must carry record');
      assert.ok('registerCleanup' in ctx, 'ctx must carry registerCleanup');
      assert.ok('page' in ctx, 'ctx must carry page');
      ctx.record('unit-step-1', true, { note: 'first' });
      ctx.record('unit-step-2', true, { note: 'second' });
      return { ok: true, steps: 2 };
    },
  });

  assert.equal(probeFnEntered, true, 'probeFn MUST be invoked by runProbe (slice-2 contract)');
  assert.equal(result.ok, true, 'probe completion must yield ok: true');
  assert.ok(observedCtx, 'ctx must have been observed by probeFn');
  const all = getResults();
  assert.ok(all.some((r) => r.name === 'unit-step-1' && r.ok === true), 'step-1 must be recorded');
  assert.ok(all.some((r) => r.name === 'unit-step-2' && r.ok === true), 'step-2 must be recorded');
});

await test('runProbe catches probeFn exceptions and surfaces { ok: false, error }', async () => {
  reset();
  let entered = false;

  const result = await runProbe({
    name: 'unit-thrower',
    probeFn: async () => {
      entered = true;
      throw new Error('synthetic failure');
    },
  });

  assert.equal(entered, true, 'probeFn must run (slice-2 contract)');
  assert.equal(result.ok, false, 'thrown probeFn yields ok: false');
  assert.ok(
    result.error && String(result.error).includes('synthetic failure'),
    'error message must propagate',
  );
  assert.equal(result.name, 'unit-thrower');
});

await test('runProbe passes scratchDir that is a real (mkdtemp) directory', async () => {
  reset();
  let observed = null;
  await runProbe({
    name: 'unit-scratch',
    probeFn: async (ctx) => {
      observed = ctx.scratchDir;
      assert.ok(existsSync(ctx.scratchDir), 'scratchDir must exist when probeFn runs');
      return { ok: true };
    },
  });
  assert.ok(observed, 'probeFn observed a scratchDir');
  assert.ok(existsSync(observed), 'scratchDir must persist after probeFn returns');
});

await test('registerCleanup hook runs after probeFn returns', async () => {
  reset();
  let hookCalled = false;
  await runProbe({
    name: 'unit-cleanup',
    probeFn: async ({ registerCleanup }) => {
      registerCleanup(() => { hookCalled = true; });
      return { ok: true };
    },
  });
  assert.equal(hookCalled, true, 'per-probe cleanup hook must run after probeFn completes');
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
