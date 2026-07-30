/**
 * audit/smoke_jobs_status.mjs — /jobs/{id} surface probe
 * (issue #700 tier-3).
 *
 * Self-spawns `dixiedata-web` against a private scratch dir,
 * seeds 1 soldier, then asserts:
 *   1. /jobs/active on a quiet archive returns 204 (the
 *      renderActiveJob guard returns StatusNoContent when
 *      no active job exists -- the layout progress slot
 *      polls this every 3s and expects an empty 204 in
 *      the quiet state).
 *   2. GET /jobs/1 (a non-existent job ID) renders the
 *      page-level error/empty state without crashing
 *      (the handleJobStatus handler renders the
 *      JobStatusView which handles missing jobs).
 *   3. GET /jobs/foo (a malformed job ID) returns a
 *      sensible error (not 500).
 *
 * Note: starting a real background job requires
 * triggering an export pipeline (which fires the Wails
 * native SaveFileDialog -- out of scope for headless).
 * Coverage for the active-job rendering lives in
 * internal/appshell/jobs_handlers_test.go and the
 * smoke_update_progress.mjs source-scan probe for the
 * issue #661 download-progress contract.
 *
 * Run: `node audit/smoke_jobs_status.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8795';
const BASE = `http://127.0.0.1:${PORT}`;
const WEB_BIN_PATH = webBin();
if (!existsSync(WEB_BIN_PATH)) { console.error('missing', WEB_BIN_PATH); process.exit(2); }

let pass = 0, fail = 0;
const results = [];
function record(name, ok, details = {}) {
  results.push({ name, ok, details });
  if (ok) { pass++; console.log(`  PASS ${name}`); }
  else { fail++; console.log(`  FAIL ${name}\n    ${JSON.stringify(details).slice(0, 800)}`); }
}

async function main(ctx) {
  const SCRATCH = ctx.scratchDir;
  const seedProc = spawn('go', ['run', './cmd/seed-data', '-data-dir', SCRATCH, '-soldiers', '1', '-reset'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let seedOut = '';
  seedProc.stdout.on('data', (d) => { seedOut += d; });
  seedProc.stderr.on('data', (d) => { seedOut += d; });
  const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
  if (seedExit !== 0) throw new Error(`seed-data failed (${seedExit}):\n${seedOut}`);

  const server = spawn(WEB_BIN_PATH, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', SCRATCH], { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, DIXIEDATA_DATA_DIR: SCRATCH } });
  ctx.registerCleanup(() => { try { server.kill(); } catch (_) {} });

  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(`${BASE}/`); if (r.status < 500) break; } catch {}
    await new Promise((r) => setTimeout(r, 250));
  }

  // /jobs/active returns 204 when no active job exists.
  const activeRes = await fetch(`${BASE}/jobs/active`);
  record('jobs-active-empty-state-returns-204', activeRes.status === 204, { status: activeRes.status });

  // /jobs/1 -- no such job; the handler renders an empty/error
  // state. Assert it does NOT crash (no 500) and renders
  // SOMETHING (status 200 or 4xx, but not 5xx).
  const nonExistent = await fetch(`${BASE}/jobs/1`);
  record('jobs-non-existent-id-does-not-500', nonExistent.status < 500, { status: nonExistent.status });

  // /jobs/foo -- malformed ID; same expectation (no 500).
  const malformed = await fetch(`${BASE}/jobs/foo`);
  record('jobs-malformed-id-does-not-500', malformed.status < 500, { status: malformed.status });

  const failed = results.filter((r) => !r.ok);
  if (failed.length > 0) {
    console.log(`\nFAIL: ${failed.length} assertion(s) failed.`);
    return { ok: false, steps: { pass, fail, failed } };
  }
  console.log(`\nPASS: ${results.length} assertion(s).`);
  return { ok: true, steps: { pass, fail } };
}

// Explicit process.exit -- libuv on Windows crashes during
  // shutdown when the probe spawns+tears-down multiple
  // sub-processes in quick succession (server + seed + nothing
  // else, but the libuv handle-closing assertion still fires).
  // Setting the exit code and letting the event loop drain
  // cleanly avoids the crash.
  runProbe({ name: 'jobs-status', probeFn: main })
  .then((r) => { process.exitCode = r.ok ? 0 : 1; });