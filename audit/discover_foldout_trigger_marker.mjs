// audit/discover_foldout_trigger_marker.mjs (issue #704)
//
// Static source-scan probe that catches the stale
// `data-article-md-cheatsheet-open` trigger marker that an
// early-#565 wireframe had documented. The literal marker
// does not exist in the rendered DOM (the cheatsheet
// uses the Foldout primitive's `data-foldout-trigger="<menuID>"`
// contract) — any code or docs that reference the literal
// marker is a leftover from the early-#565 typo.
//
// The probe walks frontend/, internal/, and audit/ for any
// `data-article-md-cheatsheet-open` literal in:
//   - .go files
//   - .js / .mjs / .ts / .tsx / .jsx files
//   - .templ files
//   - .md files
//   - .css files
// and exits 1 with a list of offenders when --strict is passed.
//
// For tests, the probe supports `--root <path>` so the
// synthetic-fixture tests can point at a mkdtemp without
// monkey-patching process.cwd() (which would also affect
// the test runner itself). Production invocations omit
// --root; the probe defaults to the repo root derived from
// import.meta.url.
//
// Comment-only mentions in code are allowed (the regex
// allows the marker inside /* ... */ block comments and //
// line comments) — a future agent that needs to document
// the bug (e.g. in a release note) should be able to write
// the marker name in prose without the probe flagging it.
//
// Usage:
//   node audit/discover_foldout_trigger_marker.mjs         # report
//   node audit/discover_foldout_trigger_marker.mjs --strict # exit 1 on any offender
//   node audit/discover_foldout_trigger_marker.mjs --root /tmp/fixture # scan a custom tree
//
// Static source-scan probe that catches the stale
// `data-article-md-cheatsheet-open` trigger marker that an
// early-#565 wireframe had documented. The literal marker
// does not exist in the rendered DOM (the cheatsheet
// uses the Foldout primitive's `data-foldout-trigger="<menuID>"`
// contract) — any code or docs that reference the literal
// marker is a leftover from the early-#565 typo.
//
// The probe walks frontend/, internal/, and audit/ for any
// `data-article-md-cheatsheet-open` literal in:
//   - .go files
//   - .js / .mjs / .ts / .tsx / .jsx files
//   - .templ files
//   - .md files
//   - .css files
// and exits 1 with a list of offenders when --strict is passed.
//
// Comment-only mentions in code are allowed (the regex
// allows the marker inside /* ... */ block comments and //
// line comments) — a future agent that needs to document
// the bug (e.g. in a release note) should be able to write
// the marker name in prose without the probe flagging it.
//
// Usage:
//   node audit/discover_foldout_trigger_marker.mjs         # report
//   node audit/discover_foldout_trigger_marker.mjs --strict # exit 1 on any offender

import { strict as assert } from 'node:assert';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const STRICT = process.argv.includes('--strict');

// --root <path> overrides the scan root. Default is the
// repo root (the parent of audit/). Production invocations
// omit --root; the test suite passes a mkdtemp fixture
// path so the test runs against a controlled tree without
// touching the real repo.
function parseArgs(argv) {
	const args = { root: ROOT, strict: false };
	for (let i = 0; i < argv.length; i++) {
		const a = argv[i];
		if (a === '--strict') {
			args.strict = true;
		} else if (a === '--root') {
			i++;
			args.root = argv[i];
		}
	}
	return args;
}
const args = parseArgs(process.argv.slice(2));
const SCAN_ROOT = args.root;
const STRICT_LOCAL = args.strict;

const SCAN_DIRS = [
	'frontend',
	'internal',
	'audit',
	'docs',
];
const SCAN_EXTS = new Set([
	'.go', '.js', '.mjs', '.cjs', '.ts', '.tsx', '.jsx', '.templ', '.md', '.css',
]);
const SKIP_DIRS = new Set([
	'node_modules', '.git', 'build', 'dist', '.dixiedata', '.dixiedata-state', '.scratch',
]);

// Path-based exclusions: files where the stale marker is
// INTENTIONALLY present (historical record, the probe's own
// source, the probe's tests, etc.). Each exclusion is a
// repo-relative path substring match — any file whose
// repo-relative path contains the substring is skipped.
//
// To find new offenders: run the probe without the
// exclusions and add any non-prod / non-test / non-doc
// matches to this list. The exclusions exist because the
// stale marker is a useful historical reference in comments
// + tests + the corrected wireframe row.
const EXCLUDED_PATH_SUBSTRINGS = [
	// The probe itself: defines the marker as a string
	// constant + in the diagnostic output.
	'audit/discover_foldout_trigger_marker.mjs',
	// The probe's test: references the marker to verify
	// the probe flags real-world offenders + ignores
	// comment-only / canonical-marker mentions.
	'audit/discover_foldout_trigger_marker.test.mjs',
	// The corrected wireframe: documents the stale marker
	// as a DELETED entry + the canonical trigger. Per issue
	// #704, the wireframe's correction row is the historical
	// record of the typo.
	'docs/ui-map/wireframes/15a-articles-edit.md',
	// The Foldout unit test (TestFoldout_TriggerMenuIDContract):
	// asserts the primitive does NOT emit the stale marker.
	// The marker literal is the negative-control assertion
	// that the probe exists to defend against; excluding it
	// here is a documented exemption, not a loophole.
	'internal/templates/components/foldout_test.go',
];

// The literal marker the probe flags. Wrapped in quotes so
// the test fixture (in discover_foldout_trigger_marker.test.mjs)
// can include the same literal in its own source without
// triggering the probe recursively.
const STALE_MARKER = 'data-article-md-cheatsheet-open';

function* walk(dir) {
	for (const name of readdirSync(dir)) {
		if (SKIP_DIRS.has(name)) continue;
		const full = join(dir, name);
		const st = statSync(full);
		if (st.isDirectory()) {
			yield* walk(full);
		} else {
			yield full;
		}
	}
}

// stripComments removes /* ... */ block comments and //
// line comments from a Go/JS/Templ source so the regex
// doesn't match the marker name when it appears in
// documentation prose (a release note, an explanatory
// comment, etc.). The comment-stripped source is what
// gets searched.
function stripComments(source) {
	// Strip /* ... */ block comments (non-nested, sufficient
	// for our content).
	let out = source.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '));
	// Strip // line comments. Replace with same-length
	// whitespace so byte offsets stay aligned (we don't use
	// them, but it's a tidy transformation).
	out = out.replace(/\/\/[^\n]*/g, (m) => ' '.repeat(m.length));
	return out;
}

const offenders = [];
let filesScanned = 0;

for (const dir of SCAN_DIRS) {
	const abs = join(SCAN_ROOT, dir);
	if (!statSync(abs, { throwIfNoEntry: false })) continue;
	for (const file of walk(abs)) {
		const ext = file.slice(file.lastIndexOf('.'));
		if (!SCAN_EXTS.has(ext)) continue;
		const rel = relative(SCAN_ROOT, file).replace(/\\/g, '/');
		// Skip path-based exclusions (probe's own files,
		// historical wireframe record, etc.).
		if (EXCLUDED_PATH_SUBSTRINGS.some((sub) => rel.includes(sub))) continue;
		filesScanned++;
		const raw = readFileSync(file, 'utf8');
		const stripped = stripComments(raw);
		if (!stripped.includes(STALE_MARKER)) continue;
		offenders.push({ file: rel });
	}
}

console.log('Foldout trigger marker probe (issue #704):');
console.log(`  Files scanned: ${filesScanned}`);
console.log(`  Stale marker: ${STALE_MARKER}`);
console.log(`  Offenders: ${offenders.length}`);
console.log('');

if (offenders.length === 0) {
	console.log('No offenders. ✓');
	process.exit(0);
}

console.log('=== OFFENDERS ===');
for (const o of offenders) {
	console.log(`  ${o.file}`);
}

console.log('');
console.log('The data-article-md-cheatsheet-open marker does not exist in the rendered DOM.');
console.log('The cheatsheet uses the Foldout primitive\'s canonical trigger marker:');
console.log('  data-foldout-trigger="panel.article.markdown-cheatsheet"');
console.log('(see internal/templates/components/foldout.templ:54 + :96 for the contract).');
console.log('Update the offender above to use the canonical marker.');

if (STRICT_LOCAL) {
	console.log('');
	console.log('--strict: treating as a CI failure.');
	process.exit(1);
}

process.exit(0);
