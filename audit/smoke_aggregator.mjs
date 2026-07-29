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

async function dispatchSurface(surface) {
  if (surface.kind === 'playwright') {
    // The migrated probe file is itself a runner script: it
    // spawns its own server + chromium and calls runProbe()
    // at the top level. We invoke it as a child process so
    // the per-probe scratch-dir + chromium lifecycle stays
    // isolated. stdout carries the per-step PASS/FAIL lines
    // from the probe; the exit code is the probe's pass/fail
    // tally (0 on success, 1 on any failed step).
    const result = spawnSync(
      process.execPath,
      [surfacePath(surface.file)],
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
  if (surface.kind === 'scanner') {
    // Slice 5: spawnSync('node', [path, '--strict']) and
    // aggregate {status, stdout} per scanner. Today this is
    // a no-op stub.
    return {
      name: surface.name,
      ok: true,
      details: { stub: true, kind: surface.kind, note: 'slice 5' },
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
