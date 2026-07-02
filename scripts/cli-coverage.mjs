#!/usr/bin/env node
// scripts/cli-coverage.mjs
//
// Standalone fallback for `dixiedata debug cli-coverage`.
// Walks the same dispatcher + doc the Go binary reads, but
// works without a built binary (e.g. in CI before `make debug`).
//
// Source-of-truth for "implemented": main.go's dispatch chain
// reads:
//   if appshell.Has<Verb>Subcommand(...) { ... }
//   if appshell.Has<Verb>Flag(...) { ... }
// We extract every Has*Subcommand / Has*Flag reference from
// main.go and assume each corresponds to one top-level verb.
// (One verb per Has* function is the convention; see
// internal/appshell/cli_*.go.)
//
// Flag-style top-level verbs (--smoke, --version) are extracted
// from the same chain — they're handled by HasSmokeFlag /
// EnvRequestsSmoke in cli_smoke.go.
//
// Usage:
//   node scripts/cli-coverage.mjs                  # human-readable
//   node scripts/cli-coverage.mjs --json           # JSON envelope
//   node scripts/cli-coverage.mjs --root <path>    # repo root override

import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const args = process.argv.slice(2);
let json = false;
let rootOverride = null;
for (let i = 0; i < args.length; i++) {
  if (args[i] === '--json') json = true;
  else if (args[i] === '--root') rootOverride = args[++i];
  else if (args[i] === '--help' || args[i] === '-h') {
    process.stderr.write(`Usage: node scripts/cli-coverage.mjs [--json] [--root PATH]\n`);
    process.exit(0);
  }
}

function findRepoRoot(start) {
  let dir = start;
  for (let i = 0; i < 5; i++) {
    if (fs.existsSync(path.join(dir, 'go.mod'))) return dir;
    const parent = path.dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  return null;
}

const cwd = process.cwd();
const root = rootOverride || findRepoRoot(cwd);
if (!root) {
  process.stderr.write(`cli-coverage: could not find repo root from ${cwd}\n`);
  process.exit(2);
}

const docPath = path.join(root, 'docs', 'agents', 'cli-plan.md');
if (!fs.existsSync(docPath)) {
  process.stderr.write(`cli-coverage: ${docPath} not found\n`);
  process.exit(2);
}

// --- Implemented top-level verbs ---
//
// Parse main.go for `appshell.Has<Verb>Subcommand` /
// `appshell.Has<Verb>Flag` references. Each is one top-level
// verb. Then map the function name back to the verb string
// by reading the function body in cli_*.go and pulling the
// first quoted case statement.
const mainPath = path.join(root, 'main.go');
if (!fs.existsSync(mainPath)) {
  process.stderr.write(`cli-coverage: main.go not found at ${mainPath}\n`);
  process.exit(2);
}

const mainText = fs.readFileSync(mainPath, 'utf8');
const hasRefRe = /appshell\.Has(\w+?)(?:Subcommand|Flag)\(/g;
const funcRefs = new Set();
let m;
while ((m = hasRefRe.exec(mainText))) funcRefs.add(m[1]);

// For each function name reference, find the function in
// cli_*.go and pull its first quoted verb.
const cliFiles = fs.readdirSync(path.join(root, 'internal', 'appshell'))
  .filter(f => /^cli_.*\.go$/.test(f) || /^smoke\.go$/.test(f) || /^doctor\.go$/.test(f))
  .map(f => path.join(root, 'internal', 'appshell', f));

const implSet = new Set();

for (const funcName of funcRefs) {
  for (const f of cliFiles) {
    const txt = fs.readFileSync(f, 'utf8');
    // Match `func Has<X>Subcommand(...)` / `func Has<X>Flag(...)`
    // and capture the body up to the matching closing brace.
    const funcDefRe = new RegExp(
      `func\\s+Has${funcName}(?:Subcommand|Flag)\\([^)]*\\)\\s*bool\\s*\\{([\\s\\S]*?)\\n\\}`,
      'm'
    );
    const fd = funcDefRe.exec(txt);
    if (!fd) continue;
    // Find first `case "<verb>":` (or `case "<v1>", "<v2>":`).
    const caseRe = /case\s+"([a-z][a-z0-9_-]*)"(?:\s*,\s*"([a-z][a-z0-9_-]*)")*\s*:/;
    const cm = caseRe.exec(fd[1]);
    if (cm) {
      implSet.add(cm[1]);
      const verbRe = /"([a-z][a-z0-9_-]*)"/g;
      let v;
      const caseText = fd[1].slice(cm.index, cm.index + 200);
      while ((v = verbRe.exec(caseText))) implSet.add(v[1]);
    }
    // ALSO run the eq-based fallback (in case the function
    // uses args[0] != / == / a == rather than a switch).
    const eqRe = /args\[0\]\s*[!=]=\s*"((?:--?)?[a-z][a-z0-9_\-]*)"/g;
    let em;
    while ((em = eqRe.exec(fd[1]))) implSet.add(em[1]);
    const aeqRe = /\ba\s*==\s*"((?:--?)?[a-z][a-z0-9_\-]*)"/g;
    while ((em = aeqRe.exec(fd[1]))) implSet.add(em[1]);
  }
}

// --- Documented top-level verbs ---
//
// Match `dixiedata <verb>` at start of line in cli-plan.md.
// Also match `dixiedata --<flag>` for flag-style top-level.
const docText = fs.readFileSync(docPath, 'utf8');
const docSet = new Set();
const lineRe = /^\s*dixiedata\s+([a-z][a-z0-9_-]*)/gm;
const flagRe = /^\s*dixiedata\s+(--[a-z][a-z0-9-]*)/gm;
while ((m = lineRe.exec(docText))) docSet.add(m[1]);
while ((m = flagRe.exec(docText))) docSet.add(m[1]);

const docList = [...docSet].sort();
const implList = [...implSet].sort();

const docNotImpl = docList.filter(d => !implSet.has(d));
const implNotDoc = implList.filter(d => !docSet.has(d));

const matched = docList.filter(d => implSet.has(d)).length;
const coverage = docList.length === 0 ? 100 : Math.floor((matched * 100) / docList.length);

const report = {
  command: 'cli-coverage',
  documented: docList,
  implemented: implList,
  documented_not_implemented: docNotImpl,
  implemented_not_documented: implNotDoc,
  documented_count: docList.length,
  implemented_count: implList.length,
  coverage_percent: coverage,
  doc_path: docPath,
  generated_at: new Date().toISOString(),
};

if (json) {
  process.stdout.write(JSON.stringify(report, null, 2) + '\n');
} else {
  process.stdout.write(`dixiedata debug cli-coverage\n`);
  process.stdout.write(`================================\n`);
  process.stdout.write(`Doc:           ${report.doc_path}\n`);
  process.stdout.write(`Documented:    ${report.documented_count} subcommands\n`);
  process.stdout.write(`Implemented:   ${report.implemented_count} subcommands\n`);
  process.stdout.write(`Coverage:      ${report.coverage_percent}%\n\n`);
  if (docNotImpl.length > 0) {
    process.stdout.write(`Documented, not implemented (drift — remove from docs):\n`);
    for (const s of docNotImpl) process.stdout.write(`  - ${s}\n`);
    process.stdout.write(`\n`);
  }
  if (implNotDoc.length > 0) {
    process.stdout.write(`Implemented, not documented (drift — add to docs OR are leaf verbs under a parent):\n`);
    for (const s of implNotDoc) process.stdout.write(`  - ${s}\n`);
    process.stdout.write(`\nNote: leaf verbs (e.g. 'pdf' under 'export') appear here because they're\n`);
    process.stdout.write(`switch-case literals in the dispatcher but only documented as\n`);
    process.stdout.write(`'dixiedata export pdf'. They are real, reachable verbs. Add a top-level\n`);
    process.stdout.write(`'dixiedata <verb>' line in cli-plan.md ONLY if the verb is reachable\n`);
    process.stdout.write(`directly (e.g. via 'dixiedata migrate status', not 'dixiedata migrate up status').\n\n`);
  }
  if (docNotImpl.length === 0 && implNotDoc.length === 0) {
    process.stdout.write(`Clean: every documented subcommand is implemented and vice versa.\n`);
  }
}

if (docNotImpl.length > 0 || implNotDoc.length > 0) process.exit(1);
process.exit(0);