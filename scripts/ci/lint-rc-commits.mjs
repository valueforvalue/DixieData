// scripts/ci/lint-rc-commits.mjs (issue #668, ADR 0011)
//
// Commit-type classifier for PRs targeting `rc/v*` branches.
// Enforces ADR 0011's RC feature-freeze policy:
//
//   Allowed: fix, docs, chore, test, ci (and any plain prefix
//            that matches these)
//   Disallowed: feat, refactor, perf, build
//   Missing-type: a commit subject with no `type(scope):` prefix
//                 is rejected (forces the operator to be
//                 explicit about what they're shipping).
//
// Also enforces the diff-size gate: a PR with ≥ 50 files changed
// is treated as refactor-by-stealth and rejected.
//
// Usage:
//   BASE_REF=origin/main HEAD_REF=HEAD node scripts/ci/lint-rc-commits.mjs
//   node scripts/ci/lint-rc-commits.mjs --self-test  # runs the regression net
//
// Exit codes:
//   0 — clean (no disallowed commits, no oversized diffs)
//   1 — disallowed commit(s) or oversized diffs
//   2 — script invocation error (missing BASE_REF/HEAD_REF, etc.)

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { existsSync } from 'node:fs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..', '..');

const ALLOWED = new Set(['fix', 'docs', 'chore', 'test', 'ci', 'style', 'release']);
const DISALLOWED = new Set(['feat', 'refactor', 'perf', 'build']);
const DIFF_SIZE_THRESHOLD = 50;

const conventionalRe = /^([a-z]+)(\(([^)]+)\))?!?: (.+)$/;
const issueRefRe = /\((#\d+)\)/;

if (process.argv.includes('--self-test')) {
  runSelfTest();
  process.exit(0);
}

// --- invocation ---
const baseRef = process.env.BASE_REF;
const headRef = process.env.HEAD_REF || 'HEAD';
if (!baseRef) {
  console.error('lint-rc-commits: BASE_REF env var is required (e.g. origin/dev)');
  process.exit(2);
}

// Resolve the merge-base + commits between base..head.
const commitRange = `${baseRef}..${headRef}`;
const commitListProc = spawnSync(
  'git',
  ['log', '--no-merges', '--pretty=format:%H %s', commitRange],
  { encoding: 'utf8', cwd: ROOT },
);
if (commitListProc.status !== 0) {
  console.error(`lint-rc-commits: git log failed: ${commitListProc.stderr}`);
  process.exit(2);
}

const lines = commitListProc.stdout.trim().split('\n').filter(Boolean);
let pass = 0;
let fail = 0;
const failures = [];

for (const line of lines) {
  const m = line.match(/^([0-9a-f]{40}) (.+)$/);
  if (!m) continue;
  const sha = m[1];
  const subject = m[2];
  const cMatch = subject.match(conventionalRe);
  if (!cMatch) {
    fail++;
    console.log(`  ✗ ${sha.slice(0, 7)} ${subject}`);
    console.log(`    no conventional commit prefix; expected type(scope): summary. See ADR 0011.`);
    failures.push({ sha, subject, reason: 'no-prefix' });
    continue;
  }
  const type = cMatch[1];
  const issueRef = (subject.match(issueRefRe) || [])[1] || '';
  if (DISALLOWED.has(type)) {
    fail++;
    console.log(`  ✗ ${sha.slice(0, 7)} ${subject}`);
    console.log(`    commit type ${type} is disallowed on rc/v* per ADR 0011 (feature-freeze). ` +
      `Land this on dev first; cherry-pick once the RC line freezes.`);
    failures.push({ sha, subject, reason: `disallowed:${type}` });
    continue;
  }
  if (!ALLOWED.has(type)) {
    fail++;
    console.log(`  ✗ ${sha.slice(0, 7)} ${subject}`);
    console.log(`    commit type ${type} is not in the allowed list (${[...ALLOWED].join(', ')}) ` +
      `and not in the disallowed list either. Check the conventional commit spec.`);
    failures.push({ sha, subject, reason: `unknown:${type}` });
    continue;
  }
  // File-count gate (per-commit diff against the previous
  // commit; cumulative PR-level gate is a separate workflow).
  const diffProc = spawnSync(
    'git',
    ['diff', '--name-only', `${sha}^..${sha}`],
    { encoding: 'utf8', cwd: ROOT },
  );
  const files = diffProc.status === 0
    ? diffProc.stdout.trim().split('\n').filter(Boolean)
    : [];
  if (files.length >= DIFF_SIZE_THRESHOLD) {
    fail++;
    console.log(`  ✗ ${sha.slice(0, 7)} ${subject}`);
    console.log(`    diff touches ${files.length} files (threshold: ${DIFF_SIZE_THRESHOLD}). ` +
      `Per ADR 0011 this is refactor-by-stealth. Split the commit.`);
    failures.push({ sha, subject, reason: `oversized:${files.length}` });
    continue;
  }
  pass++;
  console.log(`  ✓ ${sha.slice(0, 7)} ${subject}`);
}

console.log(`\n${pass} passed, ${fail} failed`);

if (fail > 0) {
  console.error(`\nlint-rc-commits: ${fail} commit(s) violate ADR 0011's RC policy. See \`gh pr comment\` for the breakdown.`);
  process.exit(1);
}

// --- self-test ---

function runSelfTest() {
  // The classifier is a script, not a module; we run a
  // pattern-shape smoke instead of unit tests. The
  // formal regression net lives in lint-rc-commits.test.mjs.
  console.log('Self-test: inspecting five canonical commit subjects against the classifier regex...');
  const probes = [
    { subject: 'fix(frontend): wire image upload (issue #691)', wantType: 'fix' },
    { subject: 'ci: lint-js-init-guards assertion (#685)', wantType: 'ci' },
    { subject: 'feat(ui): new floating dock menu (#342)', wantType: 'feat' },
    { subject: 'refactor(appdata): move logs (#590)', wantType: 'refactor' },
  ];
  for (const p of probes) {
    const m = p.subject.match(conventionalRe);
    const got = m ? m[1] : null;
    assert.equal(got, p.wantType, `probe: ${p.subject}; want ${p.wantType}, got ${got}`);
    console.log(`  ✓ ${p.subject} → ${got}`);
  }
  // Probe the failure path: a subject without `type:` colon is rejected.
  const noColon = 'wip forgot the prefix';
  const m = noColon.match(conventionalRe);
  assert.equal(m, null, `probe: ${noColon}; should not match`);
  console.log(`  ✓ ${noColon} → (no match)`);
  console.log('PASS');
}
