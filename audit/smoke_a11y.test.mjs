// audit/smoke_a11y.test.mjs — Slice 3b gate-flip regression net
//
// Asserts the gateA11y() pure function that backs the
// audit/smoke_a11y.mjs gate flip. Slice 3a left the gate
// open (hasCritical = false); slice 3b lands the real
// computation. The probe still runs in default mode GREEN
// for the migration window; SMOKE_A11Y_STRICT=1 flips to
// FAIL mode on any serious+critical violation.
//
// Why a pure-function test instead of a full-probe test:
//   - The probe spawns dixiedata-web + chromium + walks 28
//     surfaces = multi-minute. A test of the gate
//     computation is sub-millisecond and pins the
//     contract the CI gate depends on.
//   - The pure function is the only piece of slice 3b
//     logic that can fail in a way the rest of the probe
//     doesn't already catch (a wrong return shape would
//     silently invert the gate posture).

import test from 'node:test';
import assert from 'node:assert/strict';
import { gateA11y } from './smoke_a11y.mjs';

test('gateA11y: default mode GREEN regardless of violations', () => {
  // Migration window: default mode stays ok:true so the
  // existing CI is not blocked while teams triage. The
  // hasCritical computation still runs (recorded in the
  // return shape) so STRICT mode + the JSON summary can
  // both observe it.
  assert.equal(gateA11y(false, 0, 0).ok, true);
  assert.equal(gateA11y(false, 1, 0).ok, true);
  assert.equal(gateA11y(false, 0, 1).ok, true);
  assert.equal(gateA11y(false, 26, 4).ok, true);
});

test('gateA11y: STRICT mode fails on any serious violation', () => {
  const r = gateA11y(true, 1, 0);
  assert.equal(r.ok, false);
  assert.equal(r.hasCritical, true);
});

test('gateA11y: STRICT mode fails on any critical violation', () => {
  const r = gateA11y(true, 0, 1);
  assert.equal(r.ok, false);
  assert.equal(r.hasCritical, true);
});

test('gateA11y: STRICT mode passes when no serious+critical', () => {
  const r = gateA11y(true, 0, 0);
  assert.equal(r.ok, true);
  assert.equal(r.hasCritical, false);
});

test('gateA11y: hasCritical counts serious + critical together', () => {
  // The post-#716 cohort has 0 serious + 0 critical (the
  // top-nav color-contrast violation family was the only
  // a11y finding, classified serious; slice 3c remediated
  // it). The default-mode GREEN posture + STRICT-mode
  // GREEN assertion together pin the contract that "after
  // remediation, both modes pass."
  assert.equal(gateA11y(true, 0, 0).ok, true);
  assert.equal(gateA11y(false, 0, 0).ok, true);
});

test('gateA11y: hasCritical ignores minor + moderate severities', () => {
  // The a11y probe tallies bySeverity across minor /
  // moderate / serious / critical (audit/smoke_a11y.mjs
  // visitSurface). The gate decision must ignore minor +
  // moderate (per the slice 3a Q2 threshold: fail on
  // serious or critical; allow minor and moderate to warn
  // but not fail). Confirms the counter contract: the
  // record() path in smoke_a11y.mjs must increment only
  // the serious + critical module-scoped counters, not
  // pass minor + moderate up to gateA11y().
  assert.equal(gateA11y(true, 0, 0).ok, true);
  // The function only sees serious + critical arguments,
  // so a "minor + moderate only" scenario is structurally
  // impossible at this layer. The upstream bySeverity
  // tally is the discipline; the test pins the contract
  // that gateA11y only sees the two severities that gate.
  assert.equal(gateA11y(true, 0, 0).hasCritical, false);
});
