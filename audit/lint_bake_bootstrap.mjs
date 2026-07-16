// lint_bake_bootstrap.mjs -- issue #591.
//
// Walks scripts/bake-*/main.go and asserts the bake script
// never imports the package it is responsible for generating.
// This is the bootstrap-ordering invariant from issue #588:
// when a bake script imports its target package, the script's
// compile depends on the generated file it is supposed to
// produce — chicken-and-egg on a fresh checkout.
//
// Why this single rule is sufficient (and not a broader
// "any reference to a gitignored symbol" check):
//
//   The intended design IS for hand-written packages to
//   reference the gitignored `baked` symbol (the accessor
//   pattern: `func Baked() X { return baked }`). Those
//   references are IN-PACKAGE and compile cleanly once the
//   bake runs. The bug shape in #588 was specifically the
//   bake script's import — a script that imports the
//   package it generates cannot compile to produce the
//   package's baked.go. R2 catches that shape directly;
//   a wider R1 would flag the accessor pattern as a false
//   positive.
//
// Informational by default; --strict flips to exit 1 so CI
// can use it as a gate. Output is the canonical row-only
// pipe-delimited format: `file:line | rule-id | note`.
// Sibling to audit/discover_htmx_guard.mjs.

import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { join, relative, sep } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const SCRIPTS_DIR = join(ROOT, 'scripts');

const STRICT = process.argv.includes('--strict');

// ---- Bake-script walker (R2) ----

// Return every bake-script file path. Convention:
// scripts/bake-<name>/main.go.
function listBakeScripts() {
  if (!existsSync(SCRIPTS_DIR)) return [];
  const out = [];
  for (const name of readdirSync(SCRIPTS_DIR)) {
    const p = join(SCRIPTS_DIR, name, 'main.go');
    if (existsSync(p)) out.push(p);
  }
  return out;
}

// Parse the import block. Returns the set of import paths
// (the `"github.com/.../internal/foo"` strings, NOT the
// local alias names).
function parseImports(text) {
  const out = new Set();
  const singleRe = /^import\s+"([^"]+)"$/gm;
  let m;
  while ((m = singleRe.exec(text)) !== null) out.add(m[1]);
  const blockRe = /import\s*\(([\s\S]*?)\)/g;
  let g;
  while ((g = blockRe.exec(text)) !== null) {
    const pathRe = /"([^"]+)"/g;
    let p;
    while ((p = pathRe.exec(g[1])) !== null) out.add(p[1]);
  }
  return out;
}

// Walk the script's source for the `out := filepath.Join(root,
// "internal/<pkg>/...")` assignment that names the target
// package. Returns the package path (e.g. `internal/foo`) or
// null if no such assignment is found.
function findBakeTargetPackage(text) {
  const m = text.match(/filepath\.Join\(\s*root\s*,\s*"(internal\/[^/"]+)\//);
  return m ? m[1] : null;
}

function main() {
  console.log('=== lint-bake-bootstrap ===');
  console.log('R2: bake-script-imports-target — scripts/bake-*/main.go imports the package it writes into');
  console.log('');

  const bakeScripts = listBakeScripts();
  console.log(`Found ${bakeScripts.length} bake script(s) under scripts/bake-*/.`);
  const violations = [];

  for (const f of bakeScripts) {
    const text = readFileSync(f, 'utf8');
    const target = findBakeTargetPackage(text);
    if (!target) {
      console.log(`  [skip] ${relative(ROOT, f)} (no internal/<pkg>/ filepath.Join target)`);
      continue;
    }
    const imports = parseImports(text);
    // DixieData import path = `github.com/valueforvalue/DixieData/<target>`.
    const modulePath = `github.com/valueforvalue/DixieData/${target}`;
    if (imports.has(modulePath)) {
      // Find the line of the import statement for the report.
      const importLine = text.split('\n').findIndex((l) => l.includes(`"${modulePath}"`));
      violations.push({
        file: f,
        line: importLine + 1,
        rule: 'bake-script-imports-target',
        note: `imports ${modulePath} (writes to ${target}/) — should import ${target}/parse instead`,
      });
    } else {
      console.log(`  [ok]   ${relative(ROOT, f)} → target ${target}/, no direct import`);
    }
  }
  console.log('');

  if (violations.length === 0) {
    console.log('✓ lint-bake-bootstrap clean. No drift detected.');
    process.exit(0);
  }
  console.log(`Found ${violations.length} violation(s):`);
  for (const v of violations) {
    const rel = relative(ROOT, v.file).split(sep).join('/');
    console.log(`  ${rel}:${v.line} | ${v.rule} | ${v.note}`);
  }
  if (STRICT) {
    console.log('');
    console.log('--strict: treating as a CI failure.');
    process.exit(1);
  }
  process.exit(0);
}

main();