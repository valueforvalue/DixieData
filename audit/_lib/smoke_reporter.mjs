// audit/_lib/smoke_reporter.mjs (issue #700, ADR 0011, #703)
//
// Result accumulator + summary renderer for the shared
// Playwright smoke runner. Each probe (smoke_*) calls
// record() with a name + status + details; this module
// collects them, renders a console summary, and emits
// a machine-readable audit/smoke_summary.json so the CI
// workflow can surface the artifact.
//
// Shape parity with audit/findings.json (the dialog-guard
// probe emits the same `{name, ok, details}` shape).
//
// Issue #703 extended record() to accept {kind, class}
// from the SURFACE entry (passed by smoke_aggregator.mjs
// when it invokes record() on each probe result) so the
// JSON consumer can distinguish playwright (runtime
// regression) from scanner (static-source regression)
// failures + group by the 9-class button-bug catalog
// (issue #681). The runnerVersion is sourced from
// audit/_lib/smoke_runner.mjs::RUNNER_VERSION + stamped
// on every entry + the JSON summary's top-level payload
// so a future breaking change to the runProbe() contract
// can be detected without parsing the JSON shape.

import { mkdirSync, renameSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { RUNNER_VERSION } from './smoke_runner.mjs';

const results = [];

export function record(name, ok, details = {}, meta = {}) {
  // meta carries the SURFACE-level fields (kind, class) that
  // smoke_aggregator.mjs forwards from the SURFACES[] entry.
  // The runner-stamped fields (runnerVersion, ts) are added
  // here so every record() call produces the same shape
  // regardless of whether the caller passed meta.
  const entry = {
    name,
    ok,
    ...details,
    ...meta, // {kind, class} when called from the aggregator
    runnerVersion: RUNNER_VERSION,
    ts: new Date().toISOString(),
  };
  results.push(entry);
  // Print per-result line at the call site: the caller
  // decides whether to print immediately or batch. The
  // runner prints on each record so a long probe sequence
  // still streams its progress.
  return entry;
}

export function getResults() {
  return results.slice();
}

export function reset() {
  results.length = 0;
}

export function count() {
  let pass = 0;
  let fail = 0;
  for (const r of results) (r.ok ? pass++ : fail++);
  return { pass, fail, total: results.length };
}

// renderSummary prints one line per result + a totals footer.
// Returns the process exit code (0 if all pass, 1 otherwise).
// The caller (smoke_aggregator.mjs top-level) propagates this
// to process.exit.
export function renderSummary() {
  const { pass, fail, total } = count();
  // Print in insertion order; the caller (aggregator) inserts
  // via record() during probe execution.
  for (const r of results) {
    const mark = r.ok ? '\u2713' : '\u2717';
    const errSuffix = r.ok ? '' : (r.error ? ` -- ${r.error}` : '');
    console.log(`  ${mark} ${r.name}${errSuffix}`);
  }
  console.log(`\n${pass} passed, ${fail} failed (${total} total)`);
  return fail === 0 ? 0 : 1;
}

// writeJson emits the machine-readable summary that the CI
// workflow uploads as an artifact. Atomic write: temp-file
// + rename so a crash mid-write leaves no artifact (and the
// next run replaces it cleanly).
//
// Top-level payload shape (issue #703):
//   {
//     schemaVersion: 1,            // smoke_summary.json shape contract
//     runnerVersion: <from smoke_runner.mjs::RUNNER_VERSION>,
//     finishedAt: <ISO timestamp>,
//     results: [{name, ok, kind, class, runnerVersion, ts, ...details}],
//   }
//
// `schemaVersion` is the smoke_summary.json shape itself
// (separate from runnerVersion, which is the runProbe()
// contract). Bump schemaVersion when the JSON structure
// changes (new top-level field, renamed field, removed
// field). Do NOT bump for individual record() payload
// changes -- those are pinned by the test instead.
export const SCHEMA_VERSION = 1;

export function writeJson(jsonPath = resolve('audit/smoke_summary.json')) {
  mkdirSync(dirname(jsonPath), { recursive: true });
  const tmp = `${jsonPath}.tmp`;
  const payload = {
    schemaVersion: SCHEMA_VERSION,
    runnerVersion: RUNNER_VERSION,
    finishedAt: new Date().toISOString(),
    results: results.slice(),
  };
  writeFileSync(tmp, JSON.stringify(payload, null, 2));
  renameSync(tmp, jsonPath);
}
