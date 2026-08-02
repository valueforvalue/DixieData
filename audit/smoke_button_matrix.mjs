// audit/smoke_button_matrix.mjs — Slice 1 (GREEN stub)
//
// Per-button state matrix probe (issue raised in
// .rpiv/artifacts/plans/2026-08-01_button-matrix-a11y-probes.md,
// Slice 1 of 4).
//
// Slice 1 ships a GREEN stub: the probe exists, the runner
// contract is exercised end-to-end, the aggregator surfaces
// the entry in audit/smoke_summary.json, and CI stays green.
// Slice 2 replaces the stub body with the real catalog
// walker + 10-step assertion algorithm (see plan §Slice 2).
//
// Why GREEN, not RED:
//   Per user direction (2026-08-02), CI green is
//   non-negotiable. The earlier "RED stub" framing was
//   wrong for the runner-wiring capability — proving the
//   probe exists in SURFACES[] + runs through the aggregator
//   does not require the probe to fail. The GREEN stub
//   emits a single `wiring-verified` record with `ok: true`
//   and the same JSON-summary presence a FAIL stub would
//   produce. No information lost; CI stays green.
//
// Strict mode:
//   When `SMOKE_BUTTON_MATRIX_STRICT=1`, the probe emits a
//   FAIL record and exits 1. This is the operator toggle
//   for "I want this probe to gate CI now." Defaults to
//   off (GREEN). Slice 2 will inherit the same toggle.
//
// Lifecycle: no server, no chromium, no scratch dir. The
// stub probeFn returns immediately. The aggregator's
// spawnSync exit code is the only side effect (exit 0 in
// default mode, exit 1 in strict mode).

import { runProbe } from './_lib/smoke_runner.mjs';

const STRICT = process.env.SMOKE_BUTTON_MATRIX_STRICT === '1';

async function main(ctx) {
  if (STRICT) {
    ctx.record('wiring-verified', false, {
      detail: 'SMOKE_BUTTON_MATRIX_STRICT=1 set; operator requested fail-mode before Slice 2 lands',
    });
    return { ok: false, reason: 'strict-mode' };
  }
  ctx.record('wiring-verified', true, {
    detail: 'Slice 1 GREEN stub; runner contract verified. Slice 2 ships the catalog walker + 10-step assertion algorithm.',
    slice: 1,
    strictToggle: 'SMOKE_BUTTON_MATRIX_STRICT=1',
  });
  return { ok: true, reason: 'slice-1-green-stub' };
}

const result = await runProbe({
  name: 'button-matrix',
  probeFn: main,
});
process.exit(result.ok ? 0 : 1);