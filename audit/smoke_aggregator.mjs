// audit/smoke_aggregator.mjs (issue #700, ADR 0011)
//
// CLI entry point for the shared smoke runner. Walks
// audit/_lib/smoke_index.mjs::SURFACES[] and dispatches each
// entry to either:
//   kind: 'playwright'  -> dynamic-import the file,
//                            invoke the exported probeFn
//                            contract via the runner.
//   kind: 'scanner'     -> spawnSync('node', [path, '--strict'])
//
// Slice 1: no-op stub. SURFACES[] is empty; the runner prints
// "0 surfaces, 0 pass, 0 fail" and exits 0.
// Slice 2: surfaces[0] = {kind: 'playwright', file: ...}
//          populates the first Playwright entry. The
//          aggregator dynamic-imports the file at
//          AUDIT_REPO_ROOT/file and calls the exported
//          probeFn-shaped entry point.
//
// CI integration (slice 6): .github/workflows/audit.yml invokes
// `just test-smoke` after run-round3.mjs. The justfile recipe
// resolves to `node audit/smoke_aggregator.mjs`.

import { spawnSync } from 'node:child_process';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { SURFACES } from './_lib/smoke_index.mjs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { record, renderSummary, writeJson } from './_lib/smoke_reporter.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(__dirname, '..');
const STRICT = process.argv.includes('--strict');

function surfacePath(file) {
  return isAbsolute(file) ? file : resolve(REPO_ROOT, file);
}

async function dispatchSurface(surface, index) {
  if (surface.kind === 'playwright') {
    // The migrated probe file is itself a runner script: it
    // spawns its own server + chromium and calls runProbe()
    // at the top level. We invoke it as a child process so
    // the per-probe scratch-dir + chromium lifecycle stays
    // isolated. stdout carries the per-step PASS/FAIL lines
    // from the probe; the exit code is the probe's pass/fail
    // tally (0 on success, 1 on any failed step).
    //
    // PROBE_PORT is allocated per probe from the config's
    // portRangeBase (8774) + the probe's index in the SURFACES[]
    // queue. Without unique ports the smoke server's bind() can
    // fail on Windows when the previous probe's port is still in
    // TIME_WAIT.
    const port = 8774 + (index || 0);
    const result = spawnSync(
      process.execPath,
      [surfacePath(surface.file)],
      { encoding: 'utf8', cwd: REPO_ROOT, env: { ...process.env, PROBE_PORT: String(port) } },
    );
    return {
      name: surface.name,
      ok: result.status === 0,
      details: {
        exitCode: result.status,
        ...(result.stdout ? { lastStdoutLines: result.stdout.trim().split('\n').slice(-20) } : {}),
        ...(result.stderr ? { stderrTail: result.stderr.split('\n').slice(-12).join('\n') } : {}),
      },
    };
  }
  if (surface.kind === 'scanner') {
    // Slice 5: spawn the CLI linter with `--strict` and
    // aggregate its exit code. The 4 linters (issue #682
    // nested forms, #687 invoker URL drift, #685 init
    // guards, plus the orphan-handlers sweep) all honour
    // the `--strict` gate: 0 on clean, non-zero on
    // violations. Surfacing the exit code in the JSON
    // summary lets the aggregator produce a single
    // audit/smoke_summary.json that merges the live
    // Playwright probes and the static scanners.
    //
    // Output shape mirrors the playwright branch:
    // lastStdoutLines + stderrTail. The full stdout is
    // larger for scanners (lint-no-nested-forms prints
    // every .templ it scans); we capture the tail to keep
    // the JSON summary bounded.
    //
    // The aggregator's STRICT flag is threaded as a
    // second pass-through arg: today all 4 linters
    // consume `--strict` themselves, but the pattern
    // generalizes so a future strict-only scanner can
    // gate behind the aggregator's CLI flag without
    // touching this branch.
    const args = [surfacePath(surface.file), '--strict'];
    if (STRICT) args.push('--strict');
    const result = spawnSync(
      process.execPath,
      args,
      { encoding: 'utf8', cwd: REPO_ROOT },
    );
    return {
      name: surface.name,
      ok: result.status === 0,
      details: {
        exitCode: result.status,
        ...(result.stdout ? { lastStdoutLines: result.stdout.trim().split('\n').slice(-20) } : {}),
        ...(result.stderr ? { stderrTail: result.stderr.split('\n').slice(-12).join('\n') } : {}),
      },
    };
  }
  return {
    name: surface.name,
    ok: false,
    error: `unknown kind: ${surface.kind}`,
  };
}

async function main() {
  console.log(`Smoke runner: ${SURFACES.length} surface(s) queued.`);
  let index = 0;
  for (const surface of SURFACES) {
    try {
      const result = await dispatchSurface(surface, index);
      // Issue #703: forward kind + class from the SURFACES[]
      // entry so the JSON consumer can distinguish a runtime
      // regression (playwright) from a static-source regression
      // (scanner) + group by the 9-class button-bug catalog
      // (issue #681).
      const meta = { kind: surface.kind, class: surface.class };
      record(surface.name, !!result?.ok, result?.details || {}, meta);
    } catch (err) {
      record(surface.name, false, { error: err?.message || String(err) }, { kind: surface.kind, class: surface.class });
    }
    index++;
  }
  const exitCode = renderSummary();
  writeJson(resolve('audit/smoke_summary.json'));
  process.exit(exitCode);
}

main().catch((err) => {
  console.error('smoke_aggregator: uncaught', err);
  process.exit(1);
});
