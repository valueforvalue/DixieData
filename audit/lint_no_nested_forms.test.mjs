// lint_no_nested_forms.test.mjs -- issue #682 regression net.
//
// Pins the lint-no-nested-forms probe's behaviour against the
// DixieData module + synthetic fixtures. Sibling to
// audit/lint_bake_bootstrap.test.mjs.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/lint_no_nested_forms.mjs');

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

function runProbe(dir) {
  return spawnSync('node', [PROBE, '--strict'], {
    encoding: 'utf8',
    cwd: dir,
    env: { ...process.env, DIXIE_ROOT: dir },
  });
}

function withTempDir(fn) {
  const dir = mkdtempSync(join(tmpdir(), 'lint-no-nested-forms-'));
  try {
    return fn(dir);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

// ---- Baseline against current dev HEAD ----

test('probe runs cleanly against the live repository', () => {
  const result = spawnSync('node', [PROBE], { encoding: 'utf8', cwd: ROOT });
  assert.equal(result.status, 0, `expected exit 0, got ${result.status}\n${result.stdout}\n${result.stderr}`);
  assert.match(result.stdout, /lint-no-nested-forms clean/);
});

test('probe --strict exits 0 on a clean tree', () => {
  const result = spawnSync('node', [PROBE, '--strict'], { encoding: 'utf8', cwd: ROOT });
  assert.equal(result.status, 0, `expected exit 0 with --strict on a clean tree, got ${result.status}\n${result.stdout}`);
});

// ---- Synthetic fixtures: bug shapes ----

// Flat single form — passes.
test('flat single form is clean', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'internal/templates'), { recursive: true });
    writeFileSync(join(dir, 'internal/templates/example.templ'),
      'package templates\ntempl Foo() {\n\t<form action="/x" method="post">\n\t\t<input name="x">\n\t</form>\n}\n');
    const result = runProbe(dir);
    assert.equal(result.status, 0, `expected exit 0, got ${result.status}\n${result.stdout}`);
    assert.match(result.stdout, /lint-no-nested-forms clean/);
  });
});

// Nested form — fails.
test('flags a nested <form> tag', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'internal/templates'), { recursive: true });
    writeFileSync(join(dir, 'internal/templates/bad.templ'),
      'package templates\ntempl Bad() {\n\t<form action="/x" method="post">\n\t\t<form action="/y" method="post">\n\t\t\t<input name="y">\n\t\t</form>\n\t</form>\n}\n');
    const result = runProbe(dir);
    assert.equal(result.status, 1, `expected exit 1 with --strict on nested form, got ${result.status}\n${result.stdout}`);
    assert.match(result.stdout, /nested-form-open/);
  });
});

// Multiline form tag — must be recognized.
test('recognizes multi-line <form open tags', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'internal/templates'), { recursive: true });
    writeFileSync(join(dir, 'internal/templates/multiline.templ'),
      'package templates\ntempl Foo() {\n\t<form\n\t\taction="/x"\n\t\tmethod="post"\n\t>\n\t\t<input name="x">\n\t</form>\n}\n');
    const result = runProbe(dir);
    assert.equal(result.status, 0);
    assert.match(result.stdout, /lint-no-nested-forms clean/);
  });
});

// Comments containing <form> must not trigger false positives.
test('ignores <form> in // comments', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'internal/templates'), { recursive: true });
    writeFileSync(join(dir, 'internal/templates/commenty.templ'),
      'package templates\ntempl Foo() {\n\t<form action="/x" method="post">\n\t\t// the old shape was <form> inside another <form></form>; we removed it.\n\t\t<input name="x">\n\t</form>\n}\n');
    const result = runProbe(dir);
    assert.equal(result.status, 0);
    assert.match(result.stdout, /lint-no-nested-forms clean/);
  });
});

// Non-<form> tags must not confuse the scan.
test('ignores non-form tags like <formaldehyde> (false match edge case)', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'internal/templates'), { recursive: true });
    writeFileSync(join(dir, 'internal/templates/chem.templ'),
      'package templates\ntempl Foo() {\n\t<div>formaldehyde is a chemical</div>\n\t<form action="/x" method="post">\n\t\t<input name="x">\n\t</form>\n}\n');
    const result = runProbe(dir);
    assert.equal(result.status, 0);
    assert.match(result.stdout, /lint-no-nested-forms clean/);
  });
});

// Sibling templ blocks must not be flagged as nested forms.
test('treats sibling templ blocks as independent', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'internal/templates'), { recursive: true });
    writeFileSync(join(dir, 'internal/templates/siblings.templ'),
      'package templates\ntempl A() {\n\t<form action="/a" method="post">\n\t\t<input name="a">\n\t</form>\n}\ntempl B() {\n\t<form action="/b" method="post">\n\t\t<input name="b">\n\t</form>\n}\n');
    const result = runProbe(dir);
    assert.equal(result.status, 0);
    assert.match(result.stdout, /lint-no-nested-forms clean/);
  });
});

// Self-closing <form/> — invalid in real HTML but should be handled.
test('handles self-closing <form/> tag', () => {
  withTempDir((dir) => {
    mkdirSync(join(dir, 'internal/templates'), { recursive: true });
    writeFileSync(join(dir, 'internal/templates/selfclose.templ'),
      'package templates\ntempl Foo() {\n\t<form action="/x" method="post" />\n\t<form action="/y" method="post">\n\t\t<input name="y">\n\t</form>\n}\n');
    const result = runProbe(dir);
    assert.equal(result.status, 0, `expected exit 0, got ${result.status}\n${result.stdout}`);
    assert.match(result.stdout, /lint-no-nested-forms clean/);
  });
});

// ---- Summary ----

console.log('');
console.log(`${pass} passed, ${fail} failed`);
if (fail > 0) process.exit(1);
