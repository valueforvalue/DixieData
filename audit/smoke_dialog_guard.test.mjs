// audit/smoke_dialog_guard.test.mjs
//
// Regression net for audit/smoke_dialog_guard.mjs.
//
// Pins the post-#615 contract: every native dialog call site in
// internal/appshell/ uses a recognised guard pattern. The probe
// must exit 0 and report 0 unguarded sites. The test re-reads
// the source so a new native dialog call cannot sneak in
// without surfacing here.

import { strict as assert } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const PROBE = join(ROOT, 'audit/smoke_dialog_guard.mjs');
const APPSHELL = join(ROOT, 'internal/appshell');

const DIALOG_RE =
	/\ba\.(Open|Save)(File|Directory|MultipleFiles)?Dialog\s*\(/g;

let pass = 0;
let fail = 0;
function test(name, fn) {
	try {
		fn();
		pass++;
		console.log(`  \u2713 ${name}`);
	} catch (err) {
		fail++;
		console.log(`  \u2717 ${name}`);
		console.log(`    ${err.message}`);
	}
}

function runProbe() {
	return spawnSync('node', [PROBE], { encoding: 'utf8' });
}

function* walkGoFiles(dir) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) {
			yield* walkGoFiles(p);
			continue;
		}
		if (!name.endsWith('.go')) continue;
		if (name.endsWith('_test.go')) continue;
		yield p;
	}
}

function countNativeDialogCallSites() {
	let count = 0;
	for (const file of walkGoFiles(APPSHELL)) {
		const src = readFileSync(file, 'utf8');
		const matches = src.match(DIALOG_RE);
		if (matches) count += matches.length;
	}
	return count;
}

test('probe exits 0 (no unguarded sites)', () => {
	const res = runProbe();
	if (res.status !== 0) {
		throw new Error(`probe exited ${res.status}\n${res.stdout}\n${res.stderr}`);
	}
});

test('probe report includes the success line', () => {
	const res = runProbe();
	if (!/every native dialog call is guarded/.test(res.stdout)) {
		throw new Error(`missing success line in probe output:\n${res.stdout}`);
	}
});

test('every native dialog call site is guarded', () => {
	// Count native dialog calls in the source tree (matches the
	// probe's regex). The probe's "guarded" count must equal the
	// dialog-call count we see in the source.
	const dialogCount = countNativeDialogCallSites();
	const res = runProbe();
	const m = res.stdout.match(/guarded:\s+(\d+)/);
	if (!m) throw new Error(`could not parse guarded count:\n${res.stdout}`);
	const guarded = Number(m[1]);
	if (guarded !== dialogCount) {
		throw new Error(`guarded=${guarded} != dialog calls=${dialogCount}`);
	}
});

test('probe scans internal/appshell/*.go (non-test files only)', () => {
	const res = runProbe();
	const m = res.stdout.match(/Files scanned:\s+(\d+)/);
	if (!m) throw new Error(`could not parse files scanned:\n${res.stdout}`);
	const files = Number(m[1]);
	if (files < 10) {
		// Sanity: the dialog-guard sweep must reach the bulk of
		// appshell. If this drops unexpectedly, the probe's
		// walk broke.
		throw new Error(`only ${files} files scanned; expected 10+`);
	}
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
