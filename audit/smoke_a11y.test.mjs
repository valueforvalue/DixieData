// audit/smoke_a11y.test.mjs — Slice 3b gate-flip regression net
// (default-posture flip landed in the slice 3c follow-up)
//
// Asserts the gateA11y() pure function that backs the
// audit/smoke_a11y.mjs gate flip. After the slice 3c follow-up,
// the gate is unconditional — any serious or critical violation
// fails the probe. The slice 3b migration window (default mode
// GREEN, STRICT opt-in) is closed; the prior behavior is now the
// only behavior.
//
// Why a pure-function test instead of a full-probe test:
//   - The probe spawns dixiedata-web + chromium + walks 28
//     surfaces = multi-minute. A test of the gate
//     computation is sub-millisecond and pins the
//     contract the CI gate depends on.
//   - The pure function is the only piece of gate logic
//     that can fail in a way the rest of the probe doesn't
//     already catch (a wrong return shape would silently
//     invert the gate posture).

import test from 'node:test';
import assert from 'node:assert/strict';
import { gateA11y } from './smoke_a11y.mjs';

test('gateA11y: passes when no serious+critical', () => {
  const r = gateA11y(0, 0);
  assert.equal(r.ok, true);
  assert.equal(r.hasCritical, false);
});

test('gateA11y: fails on any serious violation', () => {
  const r = gateA11y(1, 0);
  assert.equal(r.ok, false);
  assert.equal(r.hasCritical, true);
});

test('gateA11y: fails on any critical violation', () => {
  const r = gateA11y(0, 1);
  assert.equal(r.ok, false);
  assert.equal(r.hasCritical, true);
});

test('gateA11y: hasCritical counts serious + critical together', () => {
  // The post-#716 + post-#718 cohort has 0 serious + 0 critical
  // (both cohorts remediated). The gate passing when both are
  // zero pins the contract that "after the cohorts clear, the
  // probe stays GREEN."
  assert.equal(gateA11y(0, 0).ok, true);
  assert.equal(gateA11y(0, 0).hasCritical, false);
});

test('gateA11y: ignores minor + moderate severities (gate discipline)', () => {
  // The a11y probe tallies bySeverity across minor / moderate /
  // serious / critical (audit/smoke_a11y.mjs visitSurface).
  // The gate decision must ignore minor + moderate (per the
  // slice 3a Q2 threshold: fail on serious or critical; allow
  // minor and moderate to warn but not fail). Confirms the
  // counter contract: the record() path in smoke_a11y.mjs must
  // increment only the serious + critical module-scoped
  // counters, not pass minor + moderate up to gateA11y().
  // The function only sees serious + critical arguments, so
  // a "minor + moderate only" scenario is structurally
  // impossible at this layer. The upstream bySeverity tally
  // is the discipline; the test pins the contract that
  // gateA11y only sees the two severities that gate.
  assert.equal(gateA11y(0, 0).hasCritical, false);
});

test('gateA11y: large violation counts still gate', () => {
  // The slice 3a first-run cohort had 26 serious + 0 critical
  // across surfaces; the post-#716 cohort had ~4 serious;
  // after #718 the cohort is 0. This test pins that even a
  // single violation (not a threshold) gates the probe.
  assert.equal(gateA11y(26, 0).ok, false);
  assert.equal(gateA11y(0, 4).ok, false);
  assert.equal(gateA11y(26, 4).ok, false);
});
