// audit/smoke_microcopy.test.mjs
//
// Regression test for audit/smoke_microcopy.mjs. Mirrors the
// discover_htmx_guard.test.mjs pattern: positive (current HEAD
// must include the canonical violation per issue #561) +
// synthetic-regression subtests that redirect the probe at a
// temp directory and assert the four rules fire / stay silent
// as expected.
//
// Why a probe test exists: the rules in smoke_microcopy.mjs are
// regex-heavy and easy to over- or under-fit. Pinning them with
// synthetic inputs (and pinning the current-HEAD baseline
// count) means a future regex tweak that silently degrades the
// probe fails this test instead of rotting the count silently.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/smoke_microcopy.mjs');

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
	const dir = mkdtempSync(join(tmpdir(), 'microcopy-'));
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
});

test('probe output includes the three-rule summary on current HEAD', () => {
	const r = runProbe();
	assert.ok(r.stdout.includes('R1'), 'missing R1 rule summary line');
	assert.ok(r.stdout.includes('R2'), 'missing R2 rule summary line');
	assert.ok(r.stdout.includes('R3'), 'missing R3 rule summary line');
});

test('probe flags the canonical Rotating Local Archive Quote violation on synthetic input (issue #561 finding 1, regression net)', () => {
	// Slice 2 of #561 removed the eyebrow from calendar.templ, so
	// the canonical violation no longer exists on HEAD. The probe
	// must still fire when the eyebrow is reintroduced (regression
	// net for the fix). Pin via synthetic fixture.
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'canonical.templ'), `package templates
templ QuotePanel() {
	<div>
		<p class="text-xs font-semibold uppercase tracking-[0.28em]">Rotating Local Archive Quote</p>
		<blockquote>"x"</blockquote>
	</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		assert.ok(
			/rotating local archive quote/i.test(r.stdout),
			`expected canonical violation in synthetic fixture\nstdout: ${r.stdout}`,
		);
	});
});

test('probe does NOT flag calendar.templ on current HEAD (slice 2 fix landed)', () => {
	const r = runProbe();
	assert.ok(
		!/rotating local archive quote/i.test(r.stdout),
		`canonical violation should be absent on post-slice-2 HEAD\nstdout: ${r.stdout}`,
	);
});

test('probe flags the share_exports.templ exact-duplicate body sentence on synthetic input (issue #561 finding 53, regression net)', () => {
	// Slice 3 removed the duplicated body sentence from share_exports.templ,
	// so the violation no longer exists on HEAD. Pin via synthetic fixture.
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'duplicate.templ'), `package templates
templ Dedupe() {
	<div>
		<h2>Share Exports</h2>
		<p>Generate portable exports, replacement backups, and merge-ready shared archives.</p>
		<section>
			<h3>Create files</h3>
			<p>Generate portable exports, replacement backups, and merge-ready shared archives.</p>
		</section>
	</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		assert.ok(
			/generate portable exports, replacement backups/i.test(r.stdout),
			`expected exact-duplicate body to be flagged in synthetic fixture\nstdout: ${r.stdout}`,
		);
	});
});

test('probe does NOT flag share_exports.templ exact-duplicate on current HEAD (slice 3 fix landed)', () => {
	const r = runProbe();
	assert.ok(
		!/generate portable exports, replacement backups/i.test(r.stdout),
		`share_exports duplicate should be absent on post-slice-3 HEAD\nstdout: ${r.stdout}`,
	);
});

test('probe flags the share_sync.templ exact-duplicate body sentence (issue #561 finding 58)', () => {
	const r = runProbe();
	assert.ok(
		/connect a google account to upload backups/i.test(r.stdout),
		`expected share_sync duplicate body to be flagged\nstdout: ${r.stdout}`,
	);
});

test('probe --strict exits 1 when violations exist on current HEAD', () => {
	const r = runProbe();
	const strict = spawnSync('node', [PROBE, '--strict'], {
		encoding: 'utf8',
		env: { ...process.env },
	});
	// We don't assert a specific exit code here because some passes
	// may have all 3 findings fixed; the probe should still report
	// the R1/R2/R3 summary either way. The important contract is
	// that --strict exits 0 ONLY when zero findings.
	if (r.stdout.includes('Total findings: 0')) {
		assert.equal(strict.status, 0, 'expected --strict to exit 0 when zero findings');
	} else {
		assert.equal(strict.status, 1, `expected --strict to exit 1 when findings exist\nstdout: ${strict.stdout}`);
	}
});

// ---- R1 synthetic regression ----

test('R1 flags eyebrow above a single blockquote with no form controls', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r1.templ'), `package templates
templ QuotePanel() {
	<div>
		<p class="text-xs font-semibold uppercase tracking-[0.2em]">Rotating Quote</p>
		<blockquote>"SYNTHETIC QUOTE"</blockquote>
	</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		assert.ok(/Rotating Quote/i.test(r.stdout), `expected R1 to flag eyebrow\nstdout: ${r.stdout}`);
		assert.ok(r.stdout.includes('r1.templ'), `expected r1.templ in output\nstdout: ${r.stdout}`);
	});
});

test('R1 does NOT flag eyebrow that heads a form section (form controls present)', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r1_form.templ'), `package templates
templ FormLabel() {
	<div>
		<p class="text-xs font-semibold uppercase tracking-[0.2em]">Form Label</p>
		<form><label><input type="text" name="x"/></label></form>
	</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		assert.ok(!/Form Label/i.test(r.stdout), `eyebrow above form must not fire R1\nstdout: ${r.stdout}`);
	});
});

// ---- R2 synthetic regression ----

test('R2 flags a heading whose text equals an adjacent button label', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r2.templ'), `package templates
templ Duplicate() {
	<div>
		<h3 class="text-xl">Edit Event</h3>
		<button type="button">Edit Event</button>
	</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		assert.ok(/Edit Event/i.test(r.stdout), `expected R2 to flag heading-button duplicate\nstdout: ${r.stdout}`);
	});
});

test('R2 does NOT flag a heading whose adjacent button has different text', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r2_clean.templ'), `package templates
templ Distinct() {
	<div>
		<h3 class="text-xl">Soldier Profile</h3>
		<button type="button">Edit</button>
	</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		// R2 finding rows look like `    R2  L<N>  <text>` (indented,
		// followed by line number). The summary line `R2 (...): 0`
		// does not match that shape.
		const r2FindingRows = r.stdout.split('\n').filter((l) => /^\s+R2\s+L\d+/.test(l));
		assert.equal(r2FindingRows.length, 0, `R2 should not flag distinct heading + button\nfinding rows: ${r2FindingRows.join('\n')}`);
	});
});

// ---- R3 synthetic regression ----

test('R3 flags a long paragraph that appears twice within 15 lines', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r3.templ'), `package templates
templ DuplicateSentence() {
	<div>
		<p>Generate portable exports, replacement backups, and merge-ready shared archives.</p>
		<button>Do thing</button>
		<p>Generate portable exports, replacement backups, and merge-ready shared archives.</p>
	</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		assert.ok(
			/generate portable exports, replacement backups/i.test(r.stdout),
			`expected R3 to flag exact duplicate body\nstdout: ${r.stdout}`,
		);
	});
});

test('R3 does NOT flag identical Tailwind class strings across rows', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r3_table.templ'), `package templates
templ TableRows() {
	<table>
		<tr class="px-4 py-3 align-top text-slate-600"><td>A</td></tr>
		<tr class="px-4 py-3 align-top text-slate-600"><td>B</td></tr>
		<tr class="px-4 py-3 align-top text-slate-600"><td>C</td></tr>
	</table>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		const r3FindingRows = r.stdout.split('\n').filter((l) => /^\s+R3\s+L\d+/.test(l));
		assert.equal(r3FindingRows.length, 0, `R3 must not flag class-list repetition\nfinding rows: ${r3FindingRows.join('\n')}`);
	});
});

test('R3 does NOT flag identical templ component invocations across rows', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r3_comp.templ'), `package templates
templ CompReps() {
	<div>
		@soldierRow(s, 1)
		@soldierRow(s, 2)
		@soldierRow(s, 3)
	</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		const r3FindingRows = r.stdout.split('\n').filter((l) => /^\s+R3\s+L\d+/.test(l));
		assert.equal(r3FindingRows.length, 0, `R3 must not flag @component() repetition\nfinding rows: ${r3FindingRows.join('\n')}`);
	});
});

test('R3 does NOT flag repeated JSON-shaped attribute strings', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r3_json.templ'), `package templates
templ JsonAttrs() {
	<div data-merge-review-action="" data-busy-group="google-calendar-actions">A</div>
	<div data-merge-review-action="" data-busy-group="google-calendar-actions">B</div>
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		const r3FindingRows = r.stdout.split('\n').filter((l) => /^\s+R3\s+L\d+/.test(l));
		assert.equal(r3FindingRows.length, 0, `R3 must not flag JSON-attr repetition\nfinding rows: ${r3FindingRows.join('\n')}`);
	});
});

test('R3 does NOT flag templ control-flow lines (Go code inside @if/@for)', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r3_goflow.templ'), `package templates
templ GoFlow() {
	if total <= 7 {
		window := make([]Page, 0, total)
		for i := 1; i <= total; i++ {
			window = append(window, Page{Number: i})
		}
		return window
	}
}
`);
		const r = runProbe({ MICROCOPY_TEMPL_DIR: dir });
		const r3FindingRows = r.stdout.split('\n').filter((l) => /^\s+R3\s+L\d+/.test(l));
		assert.equal(r3FindingRows.length, 0, `R3 must not flag pure Go code\nfinding rows: ${r3FindingRows.join('\n')}`);
	});
});

// ---- Make target integration ----

test('--strict exits 1 on a synthetic R1 violation', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'r1_strict.templ'), `package templates
templ StrictCheck() {
	<div>
		<p class="uppercase tracking-wide">Strict Eyebrow</p>
		<blockquote>"x"</blockquote>
	</div>
}
`);
		const r = spawnSync('node', [PROBE, '--strict'], {
			encoding: 'utf8',
			env: { ...process.env, MICROCOPY_TEMPL_DIR: dir },
		});
		assert.equal(r.status, 1, `expected --strict to exit 1 with violation\nstdout: ${r.stdout}`);
	});
});

test('--strict exits 0 on a clean fixture', () => {
	withTempDir((dir) => {
		mkdirSync(dir, { recursive: true });
		writeFileSync(join(dir, 'clean.templ'), `package templates
templ Clean() {
	<div>
		<h2>Profile</h2>
		<p>This is a short paragraph.</p>
		<button>Save</button>
	</div>
}
`);
		const r = spawnSync('node', [PROBE, '--strict'], {
			encoding: 'utf8',
			env: { ...process.env, MICROCOPY_TEMPL_DIR: dir },
		});
		assert.equal(r.status, 0, `expected --strict to exit 0 on clean fixture\nstdout: ${r.stdout}`);
	});
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);