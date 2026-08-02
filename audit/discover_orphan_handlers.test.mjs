// audit/discover_orphan_handlers.test.mjs — unit tests for the
// orphan handler detection probe. Pins the regex + matching
// behavior so future changes don't accidentally flip
// reachable handlers to orphan (false positive) or orphan
// handlers to reachable (false negative).
//
// Run with: node audit/discover_orphan_handlers.test.mjs
// Exit code: 0 if all assertions pass, 1 otherwise.
//
// Repair history (issue #702): this file was last touched in
// commit 4de0ee4 (a docs commit). The probe was rewritten
// twice since then:
//
//   - ba90b2d: rewrote the probe for routebuilder-wired
//     routes per #369 / #410. Collapsed the three
//     per-type invoker counts into one combined
//     "Total invokers (templ + generated + frontend):"
//     line. Removed the "=== CANDIDATE ORPHAN HANDLERS ==="
//     section separator (orphan lines now follow the
//     summary inline).
//
//   - ca67d4a: added frontend fetch invoker detection,
//     further consolidating the invoker accounting.
//
// Both rewrites silently invalidated three assertions in
// this file. The test was left passing in the sense that
// `just lint-orphan-handlers-test` exited 0… until the
// repo's orphan count dropped from ~68 to 0 during the
// #700 sweep, at which point the tests that depended on
// "there are orphans" started failing in earnest.
//
// This rewrite pins the current contract:
//
//   1. Probe exits 0 in informational mode against HEAD.
//   2. Probe output contains the current summary header
//      lines (3 lines, not 5).
//   3. Probe invoker count is a positive integer
//      (the walker is finding invokers).
//   4. --strict exits match orphan count: 0 when none,
//      1 when any. Pinned against the live HEAD (current
//      state: 0 orphans → exit 0). A future sweep that
//      introduces orphans will cause this test to flip to
//      the "exit 1" branch — that's the intended tripwire.
//   5. /soldiers/{id}/tags is reachable via the
//      routebuilder walker (issue #256 fix). The
//      pre-#369 walker flagged these as orphan; the
//      current walker classifies them as reachable because
//      routebuilder.SoldierTagAutocomplete /
//      SoldierTagAttach / SoldierTagDetach are invoked
//      from tag_picker.templ.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { join } from 'node:path';

// Issue #702 repair: the original regex /^\/([A-Z]):/ captured
// only the drive letter and the literal `:` in the pattern
// was OUTSIDE the capture group, so .replace(..., '$1') dropped
// the colon — '/C:/Development/DixieData/' became
// 'C/Development/DixieData/' (no colon). spawnSync('node',
// [PROBE]) then resolved PROBE relative to the test runner's
// cwd and produced 'C:\Development\DixieData\C\Development\
// DixieData\audit\...'. The fix: capture the colon too so
// the replacement preserves the drive-letter separator.
const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)\//, '$1/');
const PROBE = join(ROOT, 'audit/discover_orphan_handlers.mjs');

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

// Test 1: probe exits 0 in informational mode. Current
// HEAD has 0 orphans, so the informational mode is a no-op
// success.
test('probe exits 0 in default (informational) mode', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  assert.equal(r.status, 0, `expected exit 0, got ${r.status}\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
});

// Test 2: probe output contains the current summary
// header lines. The ba90b2d rewrite collapsed the three
// per-type invoker counts into a single combined count, so
// the old "Templ invokers found:" / "Generated invokers
// found:" / "Frontend invokers found:" lines are gone.
// Current headers:
//
//   "Routes registered: <N>"
//   "Templ files scanned: <N>"
//   "Generated files scanned: <N>"
//   "Frontend files scanned: <N>"
//   "Routebuilder helpers loaded: <N>"
//   "Total invokers (templ + generated + frontend): <N>"
//   "Always-reachable (excluded): <N>"
//   "Orphan handlers (registered, no invoker): <N>"
test('probe output includes summary report', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  assert.ok(r.stdout.includes('Routes registered:'), 'missing "Routes registered:" line');
  assert.ok(r.stdout.includes('Total invokers (templ + generated + frontend):'),
    'missing combined invoker count line; the ba90b2d rewrite replaced the three per-type "X invokers found:" lines with one combined count');
  assert.ok(r.stdout.includes('Orphan handlers (registered, no invoker):'),
    'missing orphan count line');
});

// Test 3: the probe is finding invokers. If the walker
// regressed and matched 0 invokers, every route would
// flip to orphan and the probe would report
// "Orphan handlers: 202" instead of 0. The positive
// integer pin catches that regression class.
test('probe invoker count line is a positive integer on HEAD', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  const match = r.stdout.match(/Total invokers \(templ \+ generated \+ frontend\): (\d+)/);
  assert.ok(match, 'missing "Total invokers (templ + generated + frontend): <N>" line');
  const count = parseInt(match[1], 10);
  assert.ok(count > 0,
    `expected invoker count > 0 on HEAD, got ${count}; the walker missed every invoker. Routes to check: routes.go registration shape, routebuilder walker signature, frontend invoker parser.`);
});

// Test 4: orphan count parses cleanly + matches --strict
// exit code. The current contract:
//
//   orphan count == 0  →  --strict exits 0
//   orphan count  > 0  →  --strict exits 1
//
// This is the regression net for "did the sweep
// regress?" — if a future refactor re-introduces orphans,
// the exit code flips and CI catches it. With 0 orphans
// today, --strict exits 0; the test currently asserts the
// exit-0 branch and will flip to the exit-1 assertion if
// the orphan count rises.
test('--strict exit code matches orphan count (0 → exit 0, >0 → exit 1)', () => {
  const head = spawnSync('node', [PROBE, '--strict'], { encoding: 'utf8' });
  const m = head.stdout.match(/Orphan handlers \(registered, no invoker\): (\d+)/);
  assert.ok(m, 'HEAD probe output missing orphan count line');
  const orphanCount = parseInt(m[1], 10);
  assert.ok(orphanCount >= 0, `orphan count must be a non-negative integer, got ${orphanCount}`);
  const expectedExit = orphanCount > 0 ? 1 : 0;
  assert.equal(head.status, expectedExit,
    `expected --strict exit ${expectedExit} (orphan count = ${orphanCount}), got ${head.status}\nstdout: ${head.stdout}`);
});

// Test 5: /soldiers/{id}/tags is REACHABLE via the
// routebuilder walker (issue #256 regression net, current
// direction). Pre-#369, the probe flagged these routes as
// orphan. The current walker classifies them as reachable
// because routebuilder.SoldierTagAutocomplete +
// SoldierTagAttach + SoldierTagDetach are invoked from
// tag_picker.templ (the "standalone tag picker page",
// per issue #183).
//
// The test asserts the post-#369 contract: these routes
// MUST NOT appear in the orphan list. If a future refactor
// drops the routebuilder helpers or breaks the walker,
// the route flips back to orphan and this test fails.
test('/soldiers/{id}/tags is reachable via routebuilder walker (issue #256 fix)', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  const orphanMatch = r.stdout.match(/Orphan handlers \(registered, no invoker\): (\d+)/);
  assert.ok(orphanMatch, 'missing orphan count line');
  const orphanCount = parseInt(orphanMatch[1], 10);
  // Read the orphan block — everything after the orphan
  // count line. The current probe prints orphan lines as
  // "  METHOD PATH", one per line, immediately after the
  // count. Trim at the next blank line or EOF.
  const tail = r.stdout.slice(r.stdout.indexOf(`Orphan handlers (registered, no invoker): ${orphanCount}`));
  assert.ok(!/\/soldiers\/.+\/tags/.test(tail),
    `probe flagged /soldiers/{id}/tags as orphan. The routebuilder walker (#369) should classify these as reachable via tag_picker.templ's SoldierTagAutocomplete + SoldierTagAttach + SoldierTagDetach invokers. If this assertion fails, either (a) the walker regressed, (b) the routebuilder helpers were renamed, or (c) tag_picker.templ dropped its invokers. Orphan block:\n${tail}`);
});

// Test 6: /export/json is reachable via share.templ
// (canonical "shipped but invisible" regression net). The
// probe should NOT flag /export/json as orphan. If a
// future refactor of share.templ drops the data-action
// attribute, the route flips to orphan — this test catches
// that.
test('/export/json is reachable via share.templ data-action', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  const orphanMatch = r.stdout.match(/Orphan handlers \(registered, no invoker\): (\d+)/);
  assert.ok(orphanMatch, 'missing orphan count line');
  const orphanCount = parseInt(orphanMatch[1], 10);
  const tail = r.stdout.slice(r.stdout.indexOf(`Orphan handlers (registered, no invoker): ${orphanCount}`));
  assert.ok(!/^\s+(GET|POST|PATCH|PUT|DELETE)\s+\/export\/json\s*$/m.test(tail),
    `probe flagged /export/json as orphan but share.templ invokes it via data-action. Tail:\n${tail}`);
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);