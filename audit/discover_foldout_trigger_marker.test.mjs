// audit/discover_foldout_trigger_marker.test.mjs (issue #704)
//
// Unit tests for the stale-trigger-marker probe. Pins the
// scan surface + the comment-stripping behaviour so future
// refactors of the probe don't accidentally stop catching
// the `data-article-md-cheatsheet-open` typo.
//
// Run with: node audit/discover_foldout_trigger_marker.test.mjs
// Exit code: 0 if all assertions pass, 1 otherwise.

import { strict as assert } from 'node:assert';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/discover_foldout_trigger_marker.mjs');

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
		console.log(`    ${err && err.message ? err.message : err}`);
	}
}

// Test 1: probe runs to completion with exit 0 in the
// canonical repo (no stale marker present anywhere).
test('probe exits 0 in the canonical repo (no stale marker)', () => {
	const r = spawnSync('node', [PROBE], { encoding: 'utf8' });
	assert.equal(r.status, 0, `expected exit 0, got ${r.status}\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
	assert.ok(r.stdout.includes('Foldout trigger marker probe (issue #704)'), 'missing probe header');
	assert.ok(r.stdout.includes('Files scanned:'), 'missing Files scanned line');
	assert.ok(r.stdout.includes('Offenders: 0'), 'expected zero offenders; got non-zero count');
});

// Test 2: probe flags the stale marker in synthetic
// fixtures. Uses a mkdtemp fixture tree structured like
// the real repo (frontend/, internal/, audit/, docs/ subdirs)
// so the probe's SCAN_DIRS walk finds the test files. The
// probe accepts --root to scan a custom tree (the test passes
// the mkdtemp path as --root, NOT cwd, so the test runner's
// cwd doesn't have to be inside the fixture).
test('probe flags the stale marker in a synthetic fixture', () => {
	const fixture = mkdtempSync(join(tmpdir(), 'foldout-trigger-fixture-'));
	try {
		// Re-create the SCAN_DIRS layout so the probe's walk()
		// finds the files. (The probe walks <root>/{frontend,internal,
		// audit,docs}, not cwd.)
		mkdirSync(join(fixture, 'frontend'), { recursive: true });
		mkdirSync(join(fixture, 'internal'), { recursive: true });
		mkdirSync(join(fixture, 'audit'), { recursive: true });
		mkdirSync(join(fixture, 'docs'), { recursive: true });
		// One .js file with the stale marker in a real
		// attribute (not a comment).
		const jsWithMarker = `document.querySelector('[data-article-md-cheatsheet-open]');`;
		writeFileSync(join(fixture, 'frontend', 'marker.js'), jsWithMarker);
		// One .templ file with the marker.
		const templWithMarker = `<button data-article-md-cheatsheet-open>Open</button>`;
		writeFileSync(join(fixture, 'internal', 'marker.templ'), templWithMarker);
		// One .go file with the marker. Placed under internal/
		// because the probe's SCAN_DIRS walks the 4 subdirs
		// only -- files at the fixture root would be missed.
		const goWithMarker = `const marker = "data-article-md-cheatsheet-open";`;
		writeFileSync(join(fixture, 'internal', 'app-marker.go'), goWithMarker);
		// One file with the marker ONLY in a comment -- should
		// NOT be flagged (the probe strips comments).
		const jsCommentOnly = `// Reference: data-article-md-cheatsheet-open was the old marker\nvar x = 1;`;
		writeFileSync(join(fixture, 'frontend', 'comment.js'), jsCommentOnly);
		// One file with the canonical marker -- should NOT be
		// flagged.
		const jsCorrect = `document.querySelector('[data-foldout-trigger="panel.article.markdown-cheatsheet"]');`;
		writeFileSync(join(fixture, 'frontend', 'correct.js'), jsCorrect);

		const r = spawnSync('node', [PROBE, '--root', fixture], { encoding: 'utf8' });
		assert.equal(r.status, 0, `expected exit 0 in default mode; got ${r.status}\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
		// All three real files flagged, the comment-only file
		// NOT flagged, the correct-marker file NOT flagged.
		assert.ok(r.stdout.includes('frontend/marker.js'), 'frontend/marker.js should be flagged');
		assert.ok(r.stdout.includes('internal/marker.templ'), 'internal/marker.templ should be flagged');
		assert.ok(r.stdout.includes('internal/app-marker.go'), 'internal/app-marker.go should be flagged');
		assert.ok(!r.stdout.includes('frontend/comment.js'), 'frontend/comment.js should NOT be flagged (marker only in comment)');
		assert.ok(!r.stdout.includes('frontend/correct.js'), 'frontend/correct.js should NOT be flagged (canonical marker)');
	} finally {
		rmSync(fixture, { recursive: true, force: true });
	}
});

// Test 3: --strict mode exits 1 when the synthetic
// fixture tree contains an offender.
test('--strict mode exits 1 when offenders are present', () => {
	const fixture = mkdtempSync(join(tmpdir(), 'foldout-trigger-strict-'));
	try {
		mkdirSync(join(fixture, 'audit'), { recursive: true });
		writeFileSync(join(fixture, 'audit', 'app-with-marker.go'), `const m = "data-article-md-cheatsheet-open";`);
		const r = spawnSync('node', [PROBE, '--root', fixture, '--strict'], { encoding: 'utf8' });
		assert.equal(r.status, 1, `expected --strict exit 1, got ${r.status}\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
	} finally {
		rmSync(fixture, { recursive: true, force: true });
	}
});

// Test 4: --strict mode exits 0 when the synthetic
// fixture tree has no offenders (only canonical marker + comment).
test('--strict mode exits 0 with no offenders', () => {
	const fixture = mkdtempSync(join(tmpdir(), 'foldout-trigger-clean-'));
	try {
		mkdirSync(join(fixture, 'audit'), { recursive: true });
		writeFileSync(join(fixture, 'audit', 'app-clean.go'), `const m = "data-foldout-trigger=panel.article.markdown-cheatsheet";`);
		writeFileSync(join(fixture, 'audit', 'app-comment.go'), `// data-article-md-cheatsheet-open (old marker, do not reintroduce)`);
		const r = spawnSync('node', [PROBE, '--root', fixture, '--strict'], { encoding: 'utf8' });
		assert.equal(r.status, 0, `expected --strict exit 0, got ${r.status}\nstdout: ${r.stdout}\nstderr: ${r.stderr}`);
	} finally {
		rmSync(fixture, { recursive: true, force: true });
	}
});

// Test 5: probe is idempotent across runs (state-free).
test('probe produces deterministic output across runs', () => {
	const r1 = spawnSync('node', [PROBE], { encoding: 'utf8' });
	const r2 = spawnSync('node', [PROBE], { encoding: 'utf8' });
	// Both runs must include the same header + scanned-file
	// count (timing-independent structural lines).
	const header1 = r1.stdout.match(/Files scanned: \d+/)?.[0];
	const header2 = r2.stdout.match(/Files scanned: \d+/)?.[0];
	assert.ok(header1, 'first run missing Files scanned header');
	assert.ok(header2, 'second run missing Files scanned header');
	assert.equal(header1, header2, 'Files scanned count differs between runs');
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
