// audit/smoke_button_matrix.mjs — Slice 1 (RED stub)
//
// Per-button state matrix probe (issue raised in
// .rpiv/artifacts/plans/2026-08-01_button-matrix-a11y-probes.md,
// Slice 1 of 4).
//
// Slice 1 is a RED stub: the probe exists, the runner
// contract is exercised, but the catalog + per-state
// assertions land in Slice 2. The stub's failure is the
// signal that the runner wired correctly — it produces a
// clear "no surface scanned yet" record that the
// aggregator surfaces in audit/smoke_summary.json.
//
// Slice 2 will replace the stub with a probe that:
//   - spawns dixiedata-web against a private scratch dir
//   - seeds via cmd/seed-data
//   - walks the 33 surfaces in audit/_lib/smoke_index.mjs
//   - caps at 8 surfaces/run × 4-run rotation for full
//     coverage across CI cycles
//   - for each surface, scans [role="button"], button,
//     a[href], and asserts the 4-state matrix:
//       visible, enabled, focused, click-changes-DOM
//
// Why this lives outside runner invariants:
//   - Per docs/agents/smoke-runner.md invariant #1, every
//     Playwright probe spawns its OWN server. Slice 2 will
//     do that; the stub does not need to.
//   - Per docs/agents/smoke-runner.md invariant #4,
//     audit/smoke_summary.json is gitignored. The stub does
//     not write it; the aggregator does.
//
// Lifecycle: no server, no chromium, no scratch dir. The
// stub probeFn returns immediately with a single failed
// record. The aggregator's spawnSync exit code is the only
// side effect (exit 1).

import { runProbe } from './_lib/smoke_runner.mjs';

async function main(ctx) {
  ctx.record('catalog-loaded', false, {
    detail: 'no surface scanned yet (Slice 1 RED stub; Slice 2 ships the catalog walker)',
  });
  return { ok: false, reason: 'slice-1-red-stub' };
}

const result = await runProbe({
  name: 'button-matrix',
  probeFn: main,
});
process.exit(result.ok ? 0 : 1);