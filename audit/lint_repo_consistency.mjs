#!/usr/bin/env node
// audit/lint_repo_consistency.mjs — issue #621.
//
// Pins the convention that new service code uses the
// repository seam (internal/db/repo/) instead of writing
// inline SQL. Pattern follows audit/lint_bake_bootstrap.mjs
// (issues #589/#591) — the same allowlist + strict-mode
// pattern.
//
// What it greps for:
//   - `import "database/sql"` or `import "github.com/.../database/sql"`
//   - `.db.Conn().Query|Exec|QueryRow|QueryContext|ExecContext`
//
// What is allowed (allowlist):
//   - The repo files themselves under internal/db/repo/
//   - The build-time bake scripts under scripts/bake-*/
//   - The tune CLI under tools/tune/
//   - The web entry under cmd/dixiedata-web/
//   - Test files (*_test.go)
//
// Run:
//   node audit/lint_repo_consistency.mjs            (informational)
//   node audit/lint_repo_consistency.mjs --strict   (exit 1 on any offender)
//
// Exit 0 = every match is allowed.
// Exit 1 = at least one offender.
// Exit 2 = fatal (file IO / grep error).

import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'path';
import process from 'node:process';

const STRICT = process.argv.includes('--strict');
const ROOT = process.cwd();

// Patterns that count as "inline SQL outside the repo."
// These are the patterns that the seam is supposed to
// replace; a new service file that matches any of them
// defeats the convention.
const PATTERNS = [
  { name: 'database/sql import', re: /import\s+["']database\/sql["']/ },
  { name: 'database/sql group import', re: /"database\/sql"/ },
  { name: 'db.Conn().Query', re: /\.db\.Conn\(\)\.Query\b/ },
  { name: 'db.Conn().QueryRow', re: /\.db\.Conn\(\)\.QueryRow\b/ },
  { name: 'db.Conn().QueryContext', re: /\.db\.Conn\(\)\.QueryContext\b/ },
  { name: 'db.Conn().Exec', re: /\.db\.Conn\(\)\.Exec\b/ },
  { name: 'db.Conn().ExecContext', re: /\.db\.Conn\(\)\.ExecContext\b/ },
];

// Paths that are allowed to use inline SQL. Each entry is
// a regex matched against the relative path from the repo
// root. Add new entries here only with a justifying comment
// — the allowlist is the durable contract.
const ALLOWLIST = [
  // The repo itself owns all SQL.
  { re: /^internal\/db\/repo\//, reason: 'repo layer owns SQL' },
  // Build-time bake scripts run before the repo exists at
  // compile time; they can't import it (chicken-and-egg).
  { re: /^scripts\/bake-.*\/main\.go$/, reason: 'bake script — runs before repo compile' },
  // Read-only tune utilities (the operator's local CLI).
  { re: /^tools\/tune\//, reason: 'tune CLI — read-only utility' },
  // The web entry's main.go is the smoke binary; the
  // command-line args + smoke setup use direct DB access.
  { re: /^cmd\/dixiedata-web\/main\.go$/, reason: 'web entry — smoke binary setup' },
  // The Wails entry's main.go is the GUI binary; it also
  // wires the appshell + DB at startup.
  { re: /^main\.go$/, reason: 'Wails entry — GUI binary setup' },
  // Test files are allowed to use inline SQL (e.g. the
  // slice-1 contract tests under internal/db/repo/sqlite/
  // use sql.Open directly to set up an in-memory DB).
  { re: /_test\.go$/, reason: 'test file' },
  // internal/db itself is the seam; it owns *sql.DB.
  { re: /^internal\/db\/db\.go$/, reason: 'db package owns *sql.DB' },
  { re: /^internal\/db\/migrations\.go$/, reason: 'migrations use *sql.Tx directly' },
];

function isAllowed(relPath) {
  for (const a of ALLOWLIST) {
    if (a.re.test(relPath)) return a.reason;
  }
  return null;
}

function gitLsFiles() {
  try {
    return execFileSync('git', ['ls-files'], { cwd: ROOT, encoding: 'utf8' })
      .split('\n')
      .filter(Boolean);
  } catch (err) {
    console.error(`git ls-files failed: ${err.message}`);
    process.exit(2);
  }
}

function main() {
  const files = gitLsFiles();
  const goFiles = files.filter((f) => f.endsWith('.go'));
  const offenders = [];
  for (const f of goFiles) {
    const reason = isAllowed(f);
    if (reason) continue;
    let text;
    try {
      text = fs.readFileSync(path.join(ROOT, f), 'utf8');
    } catch (err) {
      console.error(`read ${f}: ${err.message}`);
      process.exit(2);
    }
    const lines = text.split('\n');
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      for (const p of PATTERNS) {
        if (p.re.test(line)) {
          offenders.push({ file: f, line: i + 1, pattern: p.name, text: line.trim() });
        }
      }
    }
  }

  if (offenders.length === 0) {
    console.log(`OK: ${goFiles.length} Go files scanned; no inline-SQL offenders outside the repo seam.`);
    process.exit(0);
  }

  console.error(`FOUND: ${offenders.length} inline-SQL offender(s) outside the repo seam:`);
  for (const o of offenders) {
    console.error(`  ${o.file}:${o.line} [${o.pattern}]`);
    console.error(`    ${o.text}`);
  }

  if (!STRICT) {
    console.error(`\nRun with --strict to exit non-zero (CI gate mode).`);
    process.exit(0);
  }
  process.exit(1);
}

main();
