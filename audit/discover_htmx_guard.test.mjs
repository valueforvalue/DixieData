import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { sep as PATH_SEP } from 'node:path';

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

// ---- Slice 2: templ target walker ----

function writeTemplFixture(dir, files) {
  // files: { 'relative/path.templ': 'content', ... }
  for (const [rel, content] of Object.entries(files)) {
    const full = join(dir, rel);
    const idx = full.lastIndexOf(PATH_SEP);
    if (idx > 0) mkdirSync(full.substring(0, idx), { recursive: true });
    writeFileSync(full, content);
  }
}

test('templ walker exits 0 on clean templ dir (no orphan targets)', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'clean.templ'), `package templates
templ Clean() {
	<div id="results" hx-get="/x" hx-target="#results">OK</div>
}
`);
    const r = runProbe({ HTMX_GUARD_TEMPL_DIR: dir });
    assert.ok(!/TEMPL ORPHAN TARGET VIOLATIONS/.test(r.stdout),
      `clean templ dir should NOT have violations\nstdout: ${r.stdout}`);
    assert.ok(/Templ orphan target violations: 0/.test(r.stdout),
      `expected zero-count summary line\nstdout: ${r.stdout}`);
  });
});

test('templ walker flags a #X target with no matching id', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'orphan.templ'), `package templates
templ Broken() {
	<div hx-get="/y" hx-target="#nonexistent">Bug</div>
}
`);
    const r = runProbe({ HTMX_GUARD_TEMPL_DIR: dir });
    assert.ok(/TEMPL ORPHAN TARGET VIOLATIONS/.test(r.stdout),
      `expected templ violation section\nstdout: ${r.stdout}`);
    assert.ok(/hx-target="#nonexistent"/.test(r.stdout),
      `expected #nonexistent selector in output\nstdout: ${r.stdout}`);
  });
});

test('templ walker ignores hx-target="this" (htmx self pseudo)', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'self.templ'), `package templates
templ Self() {
	<span hx-get="/x" hx-target="this">OK</span>
}
`);
    const r = runProbe({ HTMX_GUARD_TEMPL_DIR: dir });
    assert.ok(!/TEMPL ORPHAN TARGET VIOLATIONS/.test(r.stdout),
      `this pseudo must not be flagged\nstdout: ${r.stdout}`);
  });
});

test('templ walker ignores non-# selectors ([data-...], body, .cls)', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'mixed.templ'), `package templates
templ Mixed() {
	<div hx-get="/x" hx-target="body"></div>
	<div hx-get="/y" hx-target="[data-bar]"></div>
	<div hx-get="/z" hx-target=".cls"></div>
}
`);
    const r = runProbe({ HTMX_GUARD_TEMPL_DIR: dir });
    assert.ok(!/TEMPL ORPHAN TARGET VIOLATIONS/.test(r.stdout),
      `non-# selectors must not be flagged\nstdout: ${r.stdout}`);
  });
});

test('templ walker catches data-results-target orphan', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'results.templ'), `package templates
templ Results() {
	<form data-results-target="#missing-result-region">x</form>
}
`);
    const r = runProbe({ HTMX_GUARD_TEMPL_DIR: dir });
    assert.ok(/data-results-target="#missing-result-region"/.test(r.stdout),
      `data-results-target orphan should be flagged\nstdout: ${r.stdout}`);
  });
});

test('templ walker accepts data-results-target with matching id', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'results.templ'), `package templates
templ Results() {
	<div id="present-region"></div>
	<form data-results-target="#present-region">x</form>
}
`);
    const r = runProbe({ HTMX_GUARD_TEMPL_DIR: dir });
    assert.ok(!/TEMPL ORPHAN TARGET VIOLATIONS/.test(r.stdout),
      `data-results-target with matching id should pass\nstdout: ${r.stdout}`);
  });
});

test('templ walker recurses into subdirectories (partials/)', () => {
  withTempDir((dir) => {
    writeTemplFixture(dir, {
      'partials/modal.templ': `package templates
templ Modal() {
	<div hx-get="/x" hx-target="#nonexistent-subdir">x</div>
}
`,
    });
    const r = runProbe({ HTMX_GUARD_TEMPL_DIR: dir });
    assert.ok(/TEMPL ORPHAN TARGET VIOLATIONS/.test(r.stdout),
      `subdirectory orphan should be flagged\nstdout: ${r.stdout}`);
    assert.ok(r.stdout.includes('partials/modal.templ'),
      `expected partials/ path in output\nstdout: ${r.stdout}`);
  });
});

test('templ walker accepts an id defined in a sibling file', () => {
  withTempDir((dir) => {
    writeTemplFixture(dir, {
      'a.templ': `package templates
templ Define() { <div id="shared-id"></div> }
`,
      'b.templ': `package templates
templ Use() { <div hx-get="/x" hx-target="#shared-id">y</div> }
`,
    });
    const r = runProbe({ HTMX_GUARD_TEMPL_DIR: dir });
    assert.ok(!/TEMPL ORPHAN TARGET VIOLATIONS/.test(r.stdout),
      `id declared in sibling file should satisfy target reference\nstdout: ${r.stdout}`);
  });
});

test('--strict exits 1 when templ orphan target exists', () => {
  withTempDir((dir) => {
    writeFileSync(join(dir, 'orphan.templ'), `package templates
templ Broken() { <div hx-get="/y" hx-target="#nonexistent">x</div> }
`);
    const r = spawnSync('node', [PROBE, '--strict'], {
      encoding: 'utf8',
      env: { ...process.env, HTMX_GUARD_TEMPL_DIR: dir },
    });
    assert.equal(r.status, 1, `expected exit 1 in --strict mode, got ${r.status}`);
  });
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
