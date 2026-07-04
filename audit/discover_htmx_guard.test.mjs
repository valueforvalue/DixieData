import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/discover_htmx_guard.mjs');

let pass = 0;
let fail = 0;
function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  ✓ ${name}`);
  } catch (err) {
    fail++;
    console.log(`  ✗ ${name}`);
    console.log(`    ${err.message}`);
  }
}

function runProbe(env = {}) {
  return spawnSync('node', [PROBE], {
    encoding: 'utf8',
    env: { ...process.env, ...env },
  });
}

function withTempDir(fn) {
  const dir = mkdtempSync(join(tmpdir(), 'htmx-guard-'));
  try {
    return fn(dir);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

// ---- Probe baseline behavior (against current dev HEAD) ----

test('probe exits 0 in default (informational) mode on current HEAD', () => {
  const r = runProbe();
  assert.equal(r.status, 0, `expected exit 0, got ${r.status}\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
  assert.ok(r.stdout.includes('htmx-guard clean'),
    'expected htmx-guard clean summary line on current HEAD');
});

test('probe output includes both walker sections', () => {
  const r = runProbe();
  assert.ok(r.stdout.includes('Toast-no-redirect violations:'),
    'missing toast walker summary');
  assert.ok(r.stdout.includes('JS submit coexistence violations:'),
    'missing JS submit walker summary');
});

// ---- Toast walker: synthetic regression ----

test('toast walker flags a Go handler that calls setInfoToastHeader without redirect', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'regression.go'), `package appshell

import "net/http"

// SYNTHETIC REGRESSION
func SyntheticRegression(w http.ResponseWriter, r *http.Request) {
	setInfoToastHeader(w, "Saved!")
	_ = r
}
`);
    const r = runProbe({ HTMX_GUARD_GO_DIR: dir });
    assert.ok(/[^/\\]+[/\\]regression\.go:.*SyntheticRegression/.test(r.stdout),
      `expected synthetic regression file:line in output\nstdout: ${r.stdout}`);
  });
});

test('toast walker does NOT flag a Go handler that has both toast and redirect', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'clean.go'), `package appshell

import "net/http"

func CleanHandler(w http.ResponseWriter, r *http.Request) {
	setInfoToastHeader(w, "Imported.")
	writeExportRedirect(w, "/jobs/123")
	_ = r
}
`);
    const r = runProbe({ HTMX_GUARD_GO_DIR: dir });
    assert.ok(!r.stdout.includes('regression.go'),
      `unexpected synthetic regression in output\nstdout: ${r.stdout}`);
  });
});

test('toast walker skips _test.go files', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'regression_test.go'), `package appshell

func TestSynthetic(t *testing.T) {
	setInfoToastHeader(w, "test")
}
`);
    const r = runProbe({ HTMX_GUARD_GO_DIR: dir });
    assert.ok(!r.stdout.includes('regression_test.go'),
      `expected _test.go files to be skipped\nstdout: ${r.stdout}`);
  });
});

test('toast walker ignores the setInfoToastHeader definition itself', () => {
  // The walker should not flag the function whose definition names the
  // helper. A regression that added setInfoToastHeader calls inside the
  // definition (which would itself be a bug) would still be flagged by
  // the body-content check.
  const r = runProbe();
  const text = r.stdout;
  assert.ok(!text.includes('setInfoToastHeader():') ||
            !/func\s+setInfoToastHeader/.test(text),
    'toast walker should skip the definition site, not double-flag it');
});

// ---- JS submit walker: synthetic regression ----

test('JS submit walker flags an addEventListener("submit" without marker or data-dixie-submit', () => {
  const tmp = join(tmpdir(), `_probe_js_submit_${Date.now()}.js`);
  writeFileSync(tmp, `document.addEventListener("submit", (event) => {
  const form = event.target;
  if (form instanceof HTMLFormElement) {
    handleFormWithoutMarker(form);
  }
});
`);
  try {
    const r = runProbe({ HTMX_GUARD_JS_FILE: tmp });
    assert.ok(/JS SUBMIT COEXISTENCE VIOLATIONS/.test(r.stdout),
      `expected JS submit violation section\nstdout: ${r.stdout}`);
    assert.ok(r.stdout.includes('addEventListener("submit"'),
      `expected excerpt to be quoted\nstdout: ${r.stdout}`);
  } finally {
    rmSync(tmp, { force: true });
  }
});

test('JS submit walker does NOT flag a listener that branches on data-dixie-submit', () => {
  const tmp = join(tmpdir(), `_probe_js_submit_clean_${Date.now()}.js`);
  writeFileSync(tmp, `document.addEventListener("submit", (event) => {
  const form = event.target;
  if (!(form instanceof HTMLFormElement)) return;
  if (!form.matches("[data-dixie-submit]")) return;
  event.preventDefault();
  dispatchDixieDataForm(form);
});
`);
  try {
    const r = runProbe({ HTMX_GUARD_JS_FILE: tmp });
    assert.ok(!/JS SUBMIT COEXISTENCE VIOLATIONS/.test(r.stdout),
      `data-dixie-submit delegate should NOT be flagged\nstdout: ${r.stdout}`);
  } finally {
    rmSync(tmp, { force: true });
  }
});

test('JS submit walker accepts the utility-submit marker on the preceding line', () => {
  const tmp = join(tmpdir(), `_probe_js_submit_marked_${Date.now()}.js`);
  writeFileSync(tmp, `const someForm = document.querySelector("form");
if (someForm instanceof HTMLFormElement) {
  // htmx-guard: utility-submit
  someForm.addEventListener("submit", (ev) => {
    ev.preventDefault();
    saveSomeUtilityForm(someForm);
  });
}
`);
  try {
    const r = runProbe({ HTMX_GUARD_JS_FILE: tmp });
    assert.ok(!/JS SUBMIT COEXISTENCE VIOLATIONS/.test(r.stdout),
      `marker on preceding line should suppress violation\nstdout: ${r.stdout}`);
  } finally {
    rmSync(tmp, { force: true });
  }
});

// ---- Strict mode ----

test('--strict exits 1 when toast violations exist', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'bad.go'), `package appshell
import "net/http"
func BadHandler(w http.ResponseWriter, r *http.Request) {
	setInfoToastHeader(w, "x")
	_ = r
}
`);
    const r = spawnSync('node', [PROBE, '--strict'], {
      encoding: 'utf8',
      env: { ...process.env, HTMX_GUARD_GO_DIR: dir },
    });
    assert.equal(r.status, 1, `expected exit 1 in --strict mode, got ${r.status}`);
    assert.ok(r.stdout.includes('--strict: treating as a CI failure.'),
      `expected strict confirmation line\nstdout: ${r.stdout}`);
  });
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
