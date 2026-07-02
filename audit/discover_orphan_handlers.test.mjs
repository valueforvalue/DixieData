// audit/discover_orphan_handlers.test.mjs — unit tests for the
// orphan handler detection probe. Pins the regex + matching
// behavior so future changes don't accidentally flip
// reachable handlers to orphan (false positive) or orphan
// handlers to reachable (false negative).
//
// Run with: node audit/discover_orphan_handlers.test.mjs
// Exit code: 0 if all assertions pass, 1 otherwise.

import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
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

// Test 1: probe runs to completion with exit 0 in
// informational mode.
test('probe exits 0 in default (informational) mode', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  assert.equal(r.status, 0, `expected exit 0, got ${r.status}\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
});

// Test 2: probe output contains the standard report
// header lines.
test('probe output includes summary report', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  assert.ok(r.stdout.includes('Routes registered:'), 'missing "Routes registered:" line');
  assert.ok(r.stdout.includes('Templ invokers found:'), 'missing "Templ invokers found:" line');
  assert.ok(r.stdout.includes('Orphan handlers'), 'missing "Orphan handlers" line');
});

// Test 3: probe finds KNOWN reachable routes (no false
// positives in the report).
//
// /export/json is the canonical example: it's invoked from
// share.templ via data-action. If the probe flags it as
// orphan, the regex is broken.
test('does not flag /export/json as orphan (templ invoker exists)', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  assert.ok(!/POST\s+\/export\/json$/.test(r.stdout.split('=== CANDIDATE ORPHAN HANDLERS ===')[1] || ''),
    'probe flagged /export/json as orphan but it has a templ invoker in share.templ');
});

// Test 4: probe output format is parseable.
test('orphan list is parseable (one method+path per line)', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  const tail = r.stdout.split('=== CANDIDATE ORPHAN HANDLERS ===')[1] || '';
  const lines = tail.split('\n').filter((l) => /^\s+(GET|POST|PATCH|PUT|DELETE)\s+\//.test(l));
  for (const line of lines) {
    // Each line should match "METHOD PATH" with the method
    // being one of the standard HTTP verbs and the path
    // starting with /.
    assert.match(line, /^\s+(GET|POST|PATCH|PUT|DELETE)\s+\/\S+/, `malformed line: ${line}`);
  }
});

// Test 5: --strict mode exits 1 when orphans are found
// (this repo has 68 candidate orphans; with the small
// always-reachable list, --strict should fail).
test('--strict mode exits 1 when orphans are found', () => {
  const r = spawnSync('node', [PROBE, '--strict'], { encoding: 'utf8' });
  assert.equal(r.status, 1, `expected exit 1 in strict mode, got ${r.status}`);
  assert.ok(r.stdout.includes('--strict: treating as a CI failure.'),
    'expected --strict confirmation line in output');
});

// Test 6: the canonical "shipped but invisible" routes
// from issue #256 are flagged. The tagging feature has
// backend handlers but no templ invoker for the soldier
// detail page tag editor. The probe SHOULD flag these so
// the issue is visible from CI.
//
// Note: the GET /soldiers/{id}/tags is the autocomplete
// endpoint used by tag_picker.templ — which is never
// invoked. So even the autocomplete is orphan.
test('flags the orphan tagging routes (issue #256 regression net)', () => {
  const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
  const tail = r.stdout.split('=== CANDIDATE ORPHAN HANDLERS ===')[1] || '';
  assert.ok(/\/soldiers\/.+\/tags/.test(tail),
    'expected the /soldiers/{id}/tags routes to be flagged as orphan (issue #256)');
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);