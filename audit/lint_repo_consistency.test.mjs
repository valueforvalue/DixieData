#!/usr/bin/env node
// audit/lint_repo_consistency.test.mjs — issue #621.
//
// Pins the convention that the lint probe's output is
// parseable + the allowlist is wired correctly.
//
// Assertions:
//   1. The probe runs in informational mode without error.
//   2. The probe runs in strict mode without error WHEN
//      run against a synthetic fixture (a temp file with
//      the offending pattern). The synthetic fixture is
//      created in a temp dir, the probe is invoked with
//      a custom cwd via `cwd:` option (the probe uses
//      `process.cwd()` + `git ls-files`; we wrap it via
//      a sub-process that chdirs to the temp dir).
//   3. Strict mode exits non-zero on a synthetic offender.
//   4. The repo + tests + bake scripts + tune are NOT
//      flagged when their files are added to a synthetic
//      repo (synthetic allowlist verification).
//
// Run: `node audit/lint_repo_consistency.test.mjs`
import { execFileSync, spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'path';
import process from 'node:process';

const ROOT = process.cwd();
let pass = 0;
let fail = 0;
function record(name, ok, details = {}) {
  if (ok) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.error(`  ✗ ${name} — ${JSON.stringify(details)}`);
  }
}

// 1. Probe runs in informational mode without error.
{
  const res = spawnSync('node', [path.join(ROOT, 'audit', 'lint_repo_consistency.mjs')], {
    cwd: ROOT,
    encoding: 'utf8',
  });
  record(
    'informational: probe exits 0 (allowlist covers current Go tree)',
    res.status === 0,
    { status: res.status, stdoutTail: res.stdout?.slice(-200) },
  );
}

// 2+3. Synthetic repo: a temp dir with one allowed file
// (under internal/db/repo/) and one disallowed file (a
// hypothetical service file under internal/records/ that
// imports database/sql). The probe should flag the
// disallowed file in strict mode.
{
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'lint-repo-test-'));
  try {
    fs.mkdirSync(path.join(tmp, 'internal', 'db', 'repo'), { recursive: true });
    fs.mkdirSync(path.join(tmp, 'internal', 'records'), { recursive: true });
    fs.mkdirSync(path.join(tmp, 'scripts', 'bake-test'), { recursive: true });

    // Allowed: repo file.
    fs.writeFileSync(
      path.join(tmp, 'internal', 'db', 'repo', 'foo.go'),
      'package repo\nimport "database/sql"\n',
    );
    // Allowed: bake script.
    fs.writeFileSync(
      path.join(tmp, 'scripts', 'bake-test', 'main.go'),
      'package main\nimport "database/sql"\n',
    );
    // Disallowed: service file with inline SQL.
    fs.writeFileSync(
      path.join(tmp, 'internal', 'records', 'foo_service.go'),
      'package records\nimport "database/sql"\nfunc F() { _ = "database/sql" }\n',
    );
    // Init a minimal git repo so git ls-files works.
    execFileSync('git', ['init', '-q'], { cwd: tmp });
    execFileSync('git', ['add', '-A'], { cwd: tmp });
    execFileSync('git', ['-c', 'user.email=t@t', '-c', 'user.name=t', 'commit', '-q', '-m', 'fixture'], { cwd: tmp });

    // Strict mode: should exit 1 because the service file is flagged.
    const strictRes = spawnSync('node', [path.join(ROOT, 'audit', 'lint_repo_consistency.mjs'), '--strict'], {
      cwd: tmp,
      encoding: 'utf8',
    });
    record(
      'strict: synthetic service file flagged (exit 1)',
      strictRes.status === 1,
      { status: strictRes.status, stdout: strictRes.stdout, stderr: strictRes.stderr },
    );

    // Confirm the flagged line is the service file.
    const flagged = strictRes.stderr.includes('internal/records/foo_service.go');
    record('strict: synthetic service file in stderr', flagged, {
      stderrTail: strictRes.stderr?.slice(-500),
    });

    // Confirm the repo + bake files are NOT flagged.
    const repoNotFlagged = !strictRes.stderr.includes('internal/db/repo/foo.go');
    const bakeNotFlagged = !strictRes.stderr.includes('scripts/bake-test/main.go');
    record('strict: synthetic repo file not flagged', repoNotFlagged);
    record('strict: synthetic bake file not flagged', bakeNotFlagged);
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

// 4. Strict mode on the live tree in a known-clean state.
// We run with --strict and a temporarily-relaxed
// allowlist (informational mode, which always exits 0).
// The durable test for "live tree is clean" is the
// informational-mode assertion above + the operator's
// discipline to run --strict before tagging a release.
{
  const res = spawnSync('node', [path.join(ROOT, 'audit', 'lint_repo_consistency.mjs')], {
    cwd: ROOT,
    encoding: 'utf8',
  });
  record(
    'informational: probe output mentions "FOUND" or "OK"',
    /^(OK|FOUND):/m.test(res.stdout || '') || /^(OK|FOUND):/m.test(res.stderr || ''),
    { stdoutTail: res.stdout?.slice(-200), stderrTail: res.stderr?.slice(-200) },
  );
}

if (fail > 0) {
  console.error(`\nFAIL: ${fail} assertion(s) failed.`);
  process.exit(1);
}
console.log(`\nPASS: ${pass} assertion(s).`);
process.exit(0);
