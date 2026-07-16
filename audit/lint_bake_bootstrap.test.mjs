// lint_bake_bootstrap.test.mjs -- issue #591 regression net.
//
// Pins the lint-bake-bootstrap probe's behaviour against the
// DixieData module + synthetic fixtures. Sibling to
// audit/discover_htmx_guard.test.mjs; uses the same
// hand-rolled test() / pass-fail accounting convention
// rather than node:test so the suite runs identically under
// `node --test` and `node audit/...test.mjs`.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync, mkdirSync, readFileSync } from 'node:fs';
import { join, sep as PATH_SEP } from 'node:path';
import { tmpdir } from 'node:os';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/lint_bake_bootstrap.mjs');

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
  const dir = mkdtempSync(join(tmpdir(), 'lint-bake-bootstrap-'));
  try {
    return fn(dir);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

// ---- Baseline against current dev HEAD ----

test('probe exits 0 against current HEAD (no bake-script-imports-target)', () => {
  const r = runProbe();
  assert.equal(r.status, 0, `expected exit 0, got ${r.status}\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
  assert.ok(/lint-bake-bootstrap clean/.test(r.stdout),
    `expected clean summary line on current HEAD\nstdout: ${r.stdout}`);
});

test('probe output lists both bake scripts on HEAD', () => {
  const r = runProbe();
  assert.ok(r.stdout.includes('bake-release-notes'),
    `expected bake-release-notes in output\nstdout: ${r.stdout}`);
  assert.ok(r.stdout.includes('bake-activity'),
    `expected bake-activity in output\nstdout: ${r.stdout}`);
  assert.ok(/Found 2 bake script/.test(r.stdout),
    `expected 'Found 2 bake script(s)' line\nstdout: ${r.stdout}`);
});

test('probe marks each existing bake script [ok] on HEAD', () => {
  const r = runProbe();
  assert.ok(/\[ok\].*bake-release-notes.*target internal\/releasehistory/.test(r.stdout),
    `expected bake-release-notes ok line\nstdout: ${r.stdout}`);
  assert.ok(/\[ok\].*bake-activity.*target internal\/activityhistory/.test(r.stdout),
    `expected bake-activity ok line\nstdout: ${r.stdout}`);
});

// ---- Synthetic fixtures: bake-script-imports-target ----
//
// End-to-end coverage: stage a fake repo (with the probe
// copied into audit/) + a synthetic scripts/bake-*/main.go
// that violates the invariant, then assert the probe flags
// it. The fake-repo approach lets us redirect ROOT (the probe
// computes it from import.meta.url, which is now relative to
// the temp probe copy).

// ---- End-to-end via fake repo root ----
//
// Strategy: copy the probe to a temp dir + fake ROOT via env so
// the probe scans a fake scripts/. The probe hard-codes ROOT from
// import.meta.url; the cleanest way to test it end-to-end is to
// stage a fake repo with scripts/bake-<name>/main.go + minimal
// supporting files, then run the probe with `--strict` and assert
// the exit code + output.

// We add a minimal env hook the probe doesn't yet read. The probe
// was kept simple (no env override) to mirror the repo's lint
// probe convention. For end-to-end coverage we instead stage a
// fake repo by copying the probe + scripts/ scaffold, run it
// in-place, and parse its output.

function stageFakeRepo(dir, scripts) {
  mkdirSync(join(dir, 'scripts'), { recursive: true });
  mkdirSync(join(dir, 'audit'), { recursive: true });
  // Copy the probe into the fake repo so its import.meta.url
  // resolves ROOT to the temp dir.
  // (Node will resolve the specifier relative to the probe file.)
  // The probe reads scripts/bake-*/main.go relative to ROOT.
  for (const [name, content] of Object.entries(scripts)) {
    const d = join(dir, 'scripts', `bake-${name}`);
    mkdirSync(d, { recursive: true });
    writeFileSync(join(d, 'main.go'), content);
  }
}

function copyProbe(dir) {
  // Read the probe + write to <dir>/audit/lint_bake_bootstrap.mjs.
  // The audit/ dir must exist; stageFakeRepo creates it for the
  // other tests; this helper ensures it for tests that copyProbe
  // alone.
  mkdirSync(join(dir, 'audit'), { recursive: true });
  const src = readFileSync(PROBE, 'utf8');
  writeFileSync(join(dir, 'audit', 'lint_bake_bootstrap.mjs'), src);
}

test('end-to-end: clean fake repo with no scripts/ entries → exit 0', () => {
  withTempDir((dir) => {
    copyProbe(dir);
    const probePath = join(dir, 'audit', 'lint_bake_bootstrap.mjs');
    const r = spawnSync('node', [probePath], { encoding: 'utf8' });
    assert.equal(r.status, 0, `expected exit 0\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
    assert.ok(/Found 0 bake script/.test(r.stdout),
      `expected zero count\nstdout: ${r.stdout}`);
  });
});

test('end-to-end: clean fake repo with parse-only import → exit 0', () => {
  withTempDir((dir) => {
    copyProbe(dir);
    stageFakeRepo(dir, {
      'clean': `package main

import (
	"fmt"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/foo/parse"
)

func main() {
	root, _ := findRoot()
	out := filepath.Join(root, "internal/foo/baked.go")
	_ = parse.ParseFile
	fmt.Println(out)
}

func findRoot() (string, error) { return ".", nil }
`,
    });
    const probePath = join(dir, 'audit', 'lint_bake_bootstrap.mjs');
    const r = spawnSync('node', [probePath], { encoding: 'utf8' });
    assert.equal(r.status, 0, `expected exit 0 for parse-only import\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
    assert.ok(/\[ok\].*target internal\/foo/.test(r.stdout),
      `expected [ok] line\nstdout: ${r.stdout}`);
  });
});

test('end-to-end: violating fake repo (imports target package) → exit 0 informational / 1 --strict', () => {
  withTempDir((dir) => {
    copyProbe(dir);
    stageFakeRepo(dir, {
      'foo': `package main

import (
	"fmt"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/foo"
)

func main() {
	root, _ := findRoot()
	out := filepath.Join(root, "internal/foo/baked.go")
	_ = foo.Baked
	fmt.Println(out)
}

func findRoot() (string, error) { return ".", nil }
`,
    });
    const probePath = join(dir, 'audit', 'lint_bake_bootstrap.mjs');

    // Default (informational) → exit 0 + violation line printed.
    const r = spawnSync('node', [probePath], { encoding: 'utf8' });
    assert.equal(r.status, 0,
      `informational mode should exit 0\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
    assert.ok(/bake-script-imports-target/.test(r.stdout),
      `expected violation rule id\nstdout: ${r.stdout}`);
    assert.ok(/internal\/foo.*should import internal\/foo\/parse/.test(r.stdout),
      `expected fix-hint note\nstdout: ${r.stdout}`);
    assert.ok(/Found 1 violation/.test(r.stdout),
      `expected violation count\nstdout: ${r.stdout}`);

    // --strict → exit 1.
    const rs = spawnSync('node', [probePath, '--strict'], { encoding: 'utf8' });
    assert.equal(rs.status, 1,
      `--strict should exit 1 on violations\nstdout: ${rs.stdout}\nstderr: ${rs.stderr}`);
    assert.ok(/--strict: treating as a CI failure/.test(rs.stdout),
      `expected strict confirmation line\nstdout: ${rs.stdout}`);
  });
});

test('end-to-end: violating script with multiple import paths reports the right target', () => {
  withTempDir((dir) => {
    copyProbe(dir);
    // Script imports BOTH parse (legitimate) AND the target
    // package (violation). The probe must flag the target
    // import specifically and NOT mis-attribute to parse.
    stageFakeRepo(dir, {
      'bar': `package main

import (
	"fmt"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/bar"
	"github.com/valueforvalue/DixieData/internal/bar/parse"
)

func main() {
	root, _ := findRoot()
	out := filepath.Join(root, "internal/bar/baked.go")
	_ = bar.Baked
	_ = parse.Entry
	fmt.Println(out)
}

func findRoot() (string, error) { return ".", nil }
`,
    });
    const probePath = join(dir, 'audit', 'lint_bake_bootstrap.mjs');
    const r = spawnSync('node', [probePath, '--strict'], { encoding: 'utf8' });
    assert.equal(r.status, 1, `expected --strict exit 1\nstdout: ${r.stdout}`);
    assert.ok(/imports github\.com\/valueforvalue\/DixieData\/internal\/bar/.test(r.stdout),
      `expected target import in output\nstdout: ${r.stdout}`);
    assert.ok(/writes to internal\/bar/.test(r.stdout),
      `expected target dir note\nstdout: ${r.stdout}`);
  });
});

test('end-to-end: script with no filepath.Join target is skipped (not a violation)', () => {
  withTempDir((dir) => {
    copyProbe(dir);
    // A bake-style tool that writes elsewhere (e.g. a static
    // archive, not a Go file) should not be flagged. The probe
    // matches filepath.Join(root, "internal/<pkg>/...") — a
    // script that writes to docs/ or build/ has no match and
    // is skipped.
    stageFakeRepo(dir, {
      'docs': `package main

import (
	"fmt"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/releasehistory"
)

func main() {
	out := filepath.Join(".", "docs/release-notes.md")
	_ = releasehistory.Baked
	fmt.Println(out)
}
`,
    });
    const probePath = join(dir, 'audit', 'lint_bake_bootstrap.mjs');
    const r = spawnSync('node', [probePath, '--strict'], { encoding: 'utf8' });
    assert.equal(r.status, 0,
      `script with non-internal target should not be flagged\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
    assert.ok(/\[skip\]/.test(r.stdout),
      `expected skip marker\nstdout: ${r.stdout}`);
  });
});

test('end-to-end: real bake-activity on HEAD does NOT import activityhistory (regression net)', () => {
  // Pins the fix from #588: bake-activity must import parse +
  // releasehistory, never activityhistory directly. The probe
  // would flag it if a future contributor re-introduced the
  // direct import.
  const r = runProbe();
  assert.ok(!/scripts.bake-activity.*bake-script-imports-target/.test(r.stdout),
    `bake-activity must not import activityhistory\nstdout: ${r.stdout}`);
});

test('end-to-end: real bake-release-notes on HEAD does NOT import releasehistory', () => {
  const r = runProbe();
  assert.ok(!/scripts.bake-release-notes.*bake-script-imports-target/.test(r.stdout),
    `bake-release-notes must not import releasehistory\nstdout: ${r.stdout}`);
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);