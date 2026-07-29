// audit/_lib/smoke_reporter.mjs (issue #700, ADR 0011)
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

import { mkdirSync, renameSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

const results = [];

export function record(name, ok, details = {}) {
  const entry = { name, ok, ...details, ts: new Date().toISOString() };
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
export function writeJson(jsonPath = resolve('audit/smoke_summary.json')) {
  mkdirSync(dirname(jsonPath), { recursive: true });
  const tmp = `${jsonPath}.tmp`;
  const payload = {
    finishedAt: new Date().toISOString(),
    results: results.slice(),
  };
  writeFileSync(tmp, JSON.stringify(payload, null, 2));
  renameSync(tmp, jsonPath);
}
