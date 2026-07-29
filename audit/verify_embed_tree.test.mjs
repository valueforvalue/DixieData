// verify_embed_tree.test.mjs -- issue #686 regression net.
//
// Pins the verify-embed-tree probe's behaviour against the
// DixieData module + synthetic fixtures. Sibling to
// audit/lint_bake_bootstrap.test.mjs; uses the same
// hand-rolled test() / pass-fail accounting convention
// rather than node:test so the suite runs identically under
// `node --test` and `node audit/...test.mjs`.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync, mkdirSync, existsSync, readFileSync } from 'node:fs';
import { join, sep as PATH_SEP } from 'node:path';
import { tmpdir } from 'node:os';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/verify_embed_tree.mjs');

let pass = 0;
let fail = 0;
function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  ${name}`);
  } catch (err) {
    fail++;
    console.log(`  ${name}`);
    console.log(`    ${err.message}`);
  }
}

function runProbe(args = []) {
  return spawnSync('node', [PROBE, ...args], {
    encoding: 'utf8',
    cwd: ROOT,
  });
}

function withTempDir(fn) {
  const dir = mkdtempSync(join(tmpdir(), 'verify-embed-tree-'));
  try {
    return fn(dir);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

function runProbeIn(dir, args = []) {
  // Run the probe against a synthetic temp dir that
  // mimics the DixieData layout. The probe walks paths
  // relative to DIXIE_ROOT (an env var); we set cwd to the
  // temp dir and pre-create the index.html + layout_templ.go
  // + frontend/ structure.
  return spawnSync('node', [PROBE, ...args], {
    encoding: 'utf8',
    cwd: dir,
    env: { ...process.env, DIXIE_ROOT: dir },
  });
}

// ---- Baseline against current dev HEAD ----

test('probe runs cleanly against the live repository', () => {
  const result = runProbe();
  assert.equal(result.status, 0, `expected exit 0, got ${result.status}\n${result.stdout}\n${result.stderr}`);
  // The DixieData repo has frontend/ populated and references
  // resolve. The probe should report zero missing references.
  assert.match(result.stdout, /verify-embed-tree clean/);
});

test('probe --strict exits 0 on a clean tree', () => {
  const result = runProbe(['--strict']);
  assert.equal(result.status, 0, `expected exit 0 with --strict on a clean tree, got ${result.status}\n${result.stdout}`);
});

// ---- Synthetic fixtures: bug shapes ----

// Catch: an index.html reference that doesn't resolve to a
// file under frontend/. The probe should flag it as a
// missing reference.
test('flags a missing index.html reference', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'frontend'), { recursive: true });
    writeFileSync(join(dir, 'frontend/index.html'),
      '<html><head><script src="/missing.js"></script></head></html>');
    writeFileSync(join(dir, 'main.go'),
      'package main\nimport "embed"\n//go:embed frontend\nvar assets embed.FS\n');
    const result = runProbeIn(dir);
    assert.equal(result.status, 0, `expected exit 0 (informational), got ${result.status}`);
    assert.match(result.stdout, /index-html-reference-missing/);
    assert.match(result.stdout, /\/missing\.js/);
  });
});

// Catch: a reparent-by-prefix file under frontend/. The probe
// should report R1 as a warning. The fix is to rename the
// dir to a non-underscore prefix.
test('flags a _-prefixed dir under frontend/', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'frontend/_helpers'), { recursive: true });
    writeFileSync(join(dir, 'frontend/_helpers/x.js'), '// x');
    writeFileSync(join(dir, 'frontend/index.html'),
      '<html><head></head></html>');
    writeFileSync(join(dir, 'main.go'),
      'package main\nimport "embed"\n//go:embed frontend\nvar assets embed.FS\n');
    const result = runProbeIn(dir);
    assert.equal(result.status, 0, `expected exit 0 (R1 is informational), got ${result.status}`);
    assert.match(result.stdout, /R1: reparent-by-prefix/, 'R1 header missing');
    assert.match(result.stdout, /_helpers\/x\.js/, 'R1 file path not reported');
  });
});

// Catch: a missing embed directive. The probe should fail
// in --strict mode.
test('flags a missing //go:embed directive', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'frontend'), { recursive: true });
    writeFileSync(join(dir, 'frontend/index.html'),
      '<html><head></head></html>');
    writeFileSync(join(dir, 'main.go'),
      'package main\n// no embed directive here\n');
    const result = runProbeIn(dir, ['--strict']);
    assert.equal(result.status, 1, `expected exit 1 with --strict, got ${result.status}\n${result.stdout}`);
    assert.match(result.stdout, /embed-directive-missing/);
  });
});

// Catch: the boot-theme.js allowlist. The probe should NOT
// flag /boot-theme.js as a missing reference even though it
// does not exist under frontend/ (it's served by a Go handler).
test('allowlists /boot-theme.js (Go handler, not embedded asset)', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'frontend'), { recursive: true });
    writeFileSync(join(dir, 'frontend/index.html'),
      '<html><head><script src="/boot-theme.js"></script></head></html>');
    writeFileSync(join(dir, 'main.go'),
      'package main\nimport "embed"\n//go:embed frontend\nvar assets embed.FS\n');
    const result = runProbeIn(dir);
    assert.equal(result.status, 0);
    assert.doesNotMatch(result.stdout, /index-html-reference-missing/, 'boot-theme.js was incorrectly flagged');
  });
});

// Catch: Go-escaped quotes in generated templ files. The
// probe should resolve /lib/debounce.js whether the file
// uses `src="/lib/debounce.js"` (plain HTML) or
// `src=\"/lib/debounce.js\"` (Go-escaped).
test('resolves Go-escaped quotes in generated templ files', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'frontend/lib'), { recursive: true });
    writeFileSync(join(dir, 'frontend/lib/debounce.js'), '// debounce');
    writeFileSync(join(dir, 'frontend/index.html'),
      '<html><head></head></html>');
    writeFileSync(join(dir, 'main.go'),
      'package main\nimport "embed"\n//go:embed frontend\nvar assets embed.FS\n');
    mkdirSync(join(dir, 'internal/templates'), { recursive: true });
    writeFileSync(join(dir, 'internal/templates/layout_templ.go'),
      'package templates\n// <script src=\\"/lib/debounce.js\\"></script>\n');
    const result = runProbeIn(dir);
    assert.equal(result.status, 0);
    assert.match(result.stdout, /live-html-resolves/);
    assert.match(result.stdout, /\/lib\/debounce\.js/);
  });
});

// ---- Summary ----

console.log('');
console.log(`${pass} passed, ${fail} failed`);
if (fail > 0) process.exit(1);
