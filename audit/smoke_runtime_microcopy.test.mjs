// audit/smoke_runtime_microcopy.test.mjs
//
// Regression tests for the issue #581 runtime microcopy gate
// (slice 4) and its issue #582 follow-up. The probe is
// RED-on-HEAD pre-fix (the soldiers_handlers.go:708 rephrase
// is missing), GREEN-on-HEAD post-fix. This file pins both the
// RED state and the synthetic regressions for each rule family.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/smoke_runtime_microcopy.mjs');

let pass = 0;
let fail = 0;

function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  PASS ${name}`);
  } catch (err) {
    fail++;
    console.log(`  FAIL ${name}`);
    console.log(`    ${err.message}`);
  }
}

function runProbe(source, strict = false) {
  return spawnSync('node', [PROBE, ...(strict ? ['--strict'] : [])], {
    encoding: 'utf8',
    env: { ...process.env, RUNTIME_SOURCE: source },
  });
}

function withTempSource(body, fn) {
  const dir = mkdtempSync(join(tmpdir(), 'runtime-microcopy-'));
  const source = join(dir, 'synthetic.js');
  writeFileSync(source, body);
  try {
    return fn(source);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

// Canonical synthetic source represents the POST-FIX repo (every
// pinned string present, including the slice 582B R5 rephrase).
// Slice 582A RED-on-HEAD assertions check that the pre-fix repo
// missing the R5 rephrase is exactly 1 finding; the synthetic
// canonical proves the probe accepts the post-fix shape.
const CANONICAL_SOURCE = [
  '// canonical runtime producer -- every pinned string present',
  '',
  '// top-traffic toasts in app.js',
  'showToast("Saved local draft restored.", "success");',
  'showToast("Path copied.", "success");',
  'showToast("No path to copy.", "error");',
  'showToast("Could not copy the path. Long-press to select.", "error");',
  'showToast("Could not load print options.", "error");',
  'showToast("Browse refresh failed.", "error");',
  'showToast("Choose exactly two records to compare.", "error");',
  'showToast("Nothing to copy.", "error");',
  'showToast("Clipboard helper unavailable.", "error");',
  'showToast("Could not copy. Long-press to select.", "error");',
  'showToast("Copied: " + preview, "success");',
  'showToast("Preview content was not available.", "error");',
  'showToast("Open a record with a saved Record ID before launching the scratch pad.", "warning");',
  'showToast(message || "Scratch pad opened.", "success");',
  'showToast(message || "Scratch pad failed to open.", "error");',
  'showToast("Scratch pad failed to open.", "error");',
  'showToast(toastMessage || "Request failed.", "error");',
  'showToast(warnings.length + " stale filter values; click \'Show details\' for the list.", "warning");',
  '',
  '// server-side X-DixieData-Toast producer sites',
  'w.Header().Set("X-DixieData-Toast", fmt.Sprintf("Article PDF saved to %s.", filepath.Base(path)))',
  'w.Header().Set("X-DixieData-Toast", fmt.Sprintf("Event PDF saved to %s.", filepath.Base(path)))',
  'w.Header().Set("X-DixieData-Toast", "Identity saved. Loading DixieData...")',
  'w.Header().Set("X-DixieData-Toast", fmt.Sprintf("Display ID set to %s", newID))',
  '',
  '// startup placeholder',
  '<title>Loading DixieData...</title>',
  'text-2xl font-semibold text-[var(--theme-text-primary)]">Loading DixieData...</p>',
  'The local archive is still starting up. This screen will refresh automatically.',
  '',
  '// CLI help',
  'DixieData CLI \u2014 headless archive operations',
  'dixiedata <subcommand> [flags]',
  'See docs/agents/cli-plan.md for the full roadmap.',
  '',
].join('\n');

// Pre-fix canonical: the soldiers_handlers.go toast is the
// passive "Display ID recovered" phrasing that the slice 582B
// rephrase replaces. We use this for the RED-on-HEAD baseline
// assertions.
const PREFIX_CANONICAL = CANONICAL_SOURCE.replace(
  'fmt.Sprintf("Display ID set to %s", newID)',
  'fmt.Sprintf("Display ID recovered: %s", newID)',
);

// --- baseline-on-HEAD --------------------------------------------------

test('probe reports 0 baseline findings on current repo (GREEN-on-HEAD, slice 582B rephrase landed)', () => {
  // Post-fix HEAD: the slice 582B R5 rephrase replaced the
  // passive "recovered" with action-form "set to", so the
  // required pin finds its match AND the forbidden regex
  // finds no source for the passive phrasing. Both flip to
  // clean. Issue #582 closed.
  const result = runProbe(undefined, false);
  assert.equal(result.status, 0, `expected informational exit 0\nstdout: ${result.stdout}`);
  assert.match(
    result.stdout,
    /Required copy findings: 0\b/,
    `expected 0 required findings on HEAD post-fix; got:\n${result.stdout}`,
  );
  assert.match(
    result.stdout,
    /Forbidden\/verbose copy findings: 0\b/,
    `expected 0 forbidden findings on HEAD post-fix (R5 rephrase landed); got:\n${result.stdout}`,
  );
});

test('probe exits 0 under --strict on current repo (GREEN-on-HEAD post-fix)', () => {
  const result = runProbe(undefined, true);
  assert.equal(result.status, 0, `expected strict exit 0 on HEAD\nstdout: ${result.stdout}`);
  assert.match(result.stdout, /Runtime microcopy sweep: clean/);
});

// --- synthetic positive: missing-toast fixtures -----------------------

test('synthetic missing-toast fixture fails the required rule', () => {
  withTempSource(
    CANONICAL_SOURCE.replace('showToast("Path copied.", "success");', '/* removed */'),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /Path copied\./);
    },
  );
});

test('synthetic missing-startup-heading fixture fails the required rule', () => {
  const mutated = CANONICAL_SOURCE.replace(
    '\ntext-2xl font-semibold text-[var(--theme-text-primary)]">Loading DixieData...</p>\n',
    '\nLoading app...\n',
  );
  withTempSource(mutated, (source) => {
    const result = runProbe(source, true);
    assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
    assert.match(result.stdout, /startup: body heading/);
  });
});

test('synthetic missing-CLI-help-opener fixture fails the required rule', () => {
  withTempSource(
    CANONICAL_SOURCE.replace('DixieData CLI \u2014 headless archive operations', 'CLI help'),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /cli: DixieData CLI opener/);
    },
  );
});

test('synthetic missing-server-toast Article PDF fixture fails the required rule', () => {
  withTempSource(
    CANONICAL_SOURCE.replace('Article PDF saved to ', '[removed] '),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /Article PDF/);
    },
  );
});

test('synthetic missing-server-toast Display ID set fixture fails the required rule', () => {
  withTempSource(
    CANONICAL_SOURCE.replace('Display ID set to ', '[removed] '),
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /Display ID set/);
    },
  );
});

// --- synthetic positive: forbidden drift fixtures ---------------------

test('synthetic status-form-empty-toast fixture fails the forbidden rule', () => {
  withTempSource(
    CANONICAL_SOURCE + '\nshowToast("No records yet.", "info");\n',
    (source) => {
      const result = runProbe(source, true);
      assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
      assert.match(result.stdout, /status-form empty copy/);
    },
  );
});

test('synthetic passive-recovered-toast fixture fails the forbidden R5 rule', () => {
  // Synthesize the PRE-FIX repo shape (with the slice 582B
  // rephrase missing) and assert the passive "recovered"
  // forbidden rule fires. This pins the R5 fix's rephrase
  // direction: the probe must reject the passive phrasing.
  withTempSource(PREFIX_CANONICAL, (source) => {
    const result = runProbe(source, true);
    assert.equal(result.status, 1, `expected strict failure on pre-fix R5\nstdout: ${result.stdout}`);
    assert.match(result.stdout, /server-toast: passive/);
  });
});

// --- synthetic positive: full canonical passes -------------------------

test('synthetic fully-canonical fixture passes the strict gate', () => {
  withTempSource(CANONICAL_SOURCE, (source) => {
    const result = runProbe(source, true);
    assert.equal(result.status, 0, `expected strict exit 0\nstdout: ${result.stdout}`);
  });
});

// --- synthetic positive: header-only fixture (smoke) -------------------

test('synthetic header-only fixture exits 1 under --strict (catches missing required)', () => {
  withTempSource('// a near-empty file with no pinned strings\n', (source) => {
    const result = runProbe(source, true);
    assert.equal(result.status, 1, `expected strict failure\nstdout: ${result.stdout}`);
    assert.match(result.stdout, /Required copy findings: [1-9]/);
  });
});

// --- strict mode -------------------------------------------------------

test('probe exits 0 without --strict even when findings exist', () => {
  withTempSource('// empty\n', (source) => {
    const result = runProbe(source, false);
    assert.equal(result.status, 0, `expected informational exit 0\nstdout: ${result.stdout}`);
    assert.match(result.stdout, /informational findings/);
  });
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
