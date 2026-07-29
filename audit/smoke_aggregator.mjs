// audit/smoke_aggregator.mjs (issue #700, ADR 0011)
//
// CLI entry point for the shared smoke runner. Walks
// audit/_lib/smoke_index.mjs::SURFACES[] and dispatches each
// entry to either:
//   kind: 'playwright'  -> audit/_lib/smoke_runner.mjs::runProbe
//   kind: 'scanner'     -> spawnSync('node', [path, '--strict'])
//
// Slice 1: no-op stub. SURFACES[] is empty; the runner prints
// "0 surfaces, 0 pass, 0 fail" and exits 0. Slice 2 (smoke_
// soldier_images migration) populates the first Playwright
// entry; slice 5 populates the scanner entries.
//
// CI integration (slice 6): .github/workflows/audit.yml invokes
// `just test-smoke` after run-round3.mjs. The justfile recipe
// resolves to `node audit/smoke_aggregator.mjs`.

import { resolve } from 'node:path';
import { SURFACES } from './_lib/smoke_index.mjs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { record, renderSummary, writeJson } from './_lib/smoke_reporter.mjs';

async function dispatchSurface(surface) {
  if (surface.kind === 'playwright') {
    return runProbe(surface);
  }
  // Slice 1 stub ignores non-playwright entries. Slice 5 will
  // add the spawnSync bridge.
  return { name: surface.name, ok: true, details: { stub: true, kind: surface.kind } };
}

async function main() {
  console.log(`Smoke runner: ${SURFACES.length} surface(s) queued.`);
  for (const surface of SURFACES) {
    try {
      const result = await dispatchSurface(surface);
      record(surface.name, !!result?.ok, result?.details || {});
    } catch (err) {
      record(surface.name, false, { error: err?.message || String(err) });
    }
  }
  const exitCode = renderSummary();
  writeJson(resolve('audit/smoke_summary.json'));
  process.exit(exitCode);
}

main().catch((err) => {
  console.error('smoke_aggregator: uncaught', err);
  process.exit(1);
});
