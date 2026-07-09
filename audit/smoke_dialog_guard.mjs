// audit/smoke_dialog_guard.mjs
//
// Regression net for issue #445 — extend the dialog-guard sweep
// (issue #158 was the first 5 handlers; the doc table in
// docs/agents/dialog-guard.md enumerates 17+ today and new ones
// keep landing: event import, article snapshot, settings images,
// settings quality apply, etc.). This probe ensures every native
// dialog call is guarded so a Wails v2.12.0 re-entry race can't
// crash the frontend.
//
// Why a static scan, not a runtime probe:
//   - Wails' os.Exit(1) on contention kills the harness before we
//     could observe the crash from a test
//   - The guard is a code-shape invariant; if a new call site
//     doesn't sit inside an enterInFlight / LoadOrStore / sentinel
//     block, it's wrong regardless of test data
//   - Mirrors the discover_htmx_guard / discover_orphan_handlers
//     pattern already in audit/
//
// Patterns accepted as "guarded" (any one of these in the
// enclosing function body is sufficient):
//
//   1. enterInFlight(...)               — Pattern B (app.go inline)
//   2. a.inFlight.LoadOrStore(...)      — raw sync.Map guard
//   3. guardedSaveFileDialog(...)
//      guardedOpenFileDialog(...)
//      guardedOpenDirectoryDialog(...)
//      guardedOpenMultipleFilesDialog(...)
//      — Pattern A via exports_handlers.go helpers
//   4. errExportInFlight sentinel return — Pattern C (helpers
//      like exportFullDatabasePDFPath that callers map back to
//      a friendly response)
//
// Sites NOT requiring a guard (intentionally unguarded):
//   - *_test.go files (these mock the dialog at the boundary)
//   - app_facades.go — production facade for cli/web harness that
//     itself has no native dialog (the dialog is behind the facade);
//     any dialog in there is a helper that tests use, not user-facing
//   - cli_export.go / cli_import.go — CLI subcommands bypass the
//     native dialog per docs/agents/cli-plan.md
//
// Fail mode:
//   - 0 unguarded sites → exit 0
//   - ≥ 1 unguarded site → print file:line:func + exit 1
//
// Adding a new native-dialog call site:
//   1. Put it inside one of the four guard patterns above.
//   2. If you can't, add it to the user's escape hatch in this
//      file with a // intentional-no-guard comment explaining why
//      and reference an issue that documents the decision.
//   3. Run `node audit/smoke_dialog_guard.mjs` before committing.

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const APPSHELL = join(ROOT, 'internal/appshell');

// Files where dialog calls are intentionally unguarded. Each entry
// must have a // justification comment in this file AND a
// referenced issue. PRs that grow this list get a stern look.
const EXEMPT_FILES = new Set([
	// cli_export.go + cli_import.go: --export / --import subcommands
	// route through inline save/load paths, not the Wails dialog.
	// Per docs/agents/cli-plan.md, every CLI command bypasses
	// native dialogs. No guard needed because there's no UI thread
	// on the CLI. Issue #281 documents the deferred bundle work
	// that may eventually consolidate these, but for now they are
	// correctly dialog-free at the call site.
	'cli_export.go',
	'cli_import.go',
]);

const DIALOG_RE =
	/\ba\.(Open|Save)(File|Directory|MultipleFiles)?Dialog\s*\(/g;

const GUARD_PATTERNS = [
	/\benterInFlight\s*\(/,
	/\binFlight\.LoadOrStore\s*\(/,
	/\bguarded(?:Save|Open)(?:File|Directory|MultipleFiles)Dialog\s*\(/,
	/\berrExportInFlight\b/,
];

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

// Build a list of `{` indexes that open a `func` body. We tokenize
// the file once (handling strings + comments) to walk a brace stack.
// The top of the brace stack at any text index points to whatever
// `{` block we're inside — including control-flow `if`/`for`/
// `switch` braces. For each `{`, peek left to see if it is the
// opening brace of a `func NAME(` declaration (with optional
// receiver). If yes, push to a function stack AND the brace stack;
// if no, push only to the brace stack. On `}`, pop both.
//
// Crucial: every `{` and every `}` is matched 1:1. Control-flow
// blocks don't carry function metadata, but they still pair with
// their closing brace so the brace stack stays balanced.
function indexFunctions(src) {
	const indices = []; // array of { openBrace, closeBrace, name }
	const braceStack = []; // mixed: func entries + anon placeholders, every `{` pushes
	let i = 0;
	let inString = false;
	let stringCh = '';
	let inLineComment = false;
	let inBlockComment = false;
	const funcNameAt = (braceIdx) => {
		// Find the most recent `func` keyword at or before this brace.
		// When we land on a `{` for a function declaration, scanning
		// backwards finds the `func` token (allowing receiver and
		// name between them). When we land on a `{` for a control-flow
		// construct (if/for/switch/struct-literal), no `func` exists
		// before this brace, so return null.
		let k = braceIdx - 1;
		while (k >= 0 && /\s/.test(src[k])) k--;
		// If the chunk immediately before `{` doesn't look like the
		// end of a parameter list (closing `)` followed by whitespace
		// and `{`), this brace is unlikely to be a function header.
		// Quick rejection: any non-identifier, non-`)` ending here is
		// almost certainly not a function declaration.
		if (k < 0) return null;
		if (src[k] !== ')') return null;
		// Slice from `k+1` (the closing `)`) up to `braceIdx` (just
		// before `{`). Include enough text on the left to capture
		// `func (recv) NAME(`. We do that by walking further left until
		// we see `func` (4 ASCII chars) at a word boundary.
		let lookback = 200;
		let start = Math.max(0, braceIdx - lookback);
		const head = src.slice(start, braceIdx);
		// Match a function declaration: optional receiver, function
		// name, parameter list, optional return-type clause (single
		// parenthesised form on the same line), end of slice. The
		// regex tolerates arbitrary single-pair `(...)` for the param
		// list and ONE trailing `(...)` for the return type because
		// DixieData handlers never use bare return-type tokens like
		// `error`; they always parenthesise. Simpler: scan for the
		// right-most `func NAME(` in the slice.
		const matches = [...head.matchAll(/func(?:\s+\([^)]*\))?\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(/g)];
		if (matches.length === 0) return null;
		// Return the LAST match (closest to brace).
		return matches[matches.length - 1][1];
	};
	while (i < src.length) {
		const ch = src[i];
		const next = src[i + 1];
		if (inLineComment) {
			if (ch === '\n') inLineComment = false;
			i++;
			continue;
		}
		if (inBlockComment) {
			if (ch === '*' && next === '/') {
				inBlockComment = false;
				i += 2;
				continue;
			}
			i++;
			continue;
		}
		if (inString) {
			if (ch === '\\') {
				i += 2;
				continue;
			}
			if (ch === stringCh) inString = false;
			i++;
			continue;
		}
		if (ch === '/' && next === '/') {
			inLineComment = true;
			i += 2;
			continue;
		}
		if (ch === '/' && next === '*') {
			inBlockComment = true;
			i += 2;
			continue;
		}
		if (ch === '"' || ch === '`') {
			inString = true;
			stringCh = ch;
			i++;
			continue;
		}
		if (ch === '{') {
			const name = funcNameAt(i);
			const entry = name ? { name, openBrace: i } : { name: null, openBrace: i };
			braceStack.push(entry);
			i++;
			continue;
		}
		if (ch === '}') {
			const popped = braceStack.pop();
			if (popped && popped.name) {
				indices.push({ openBrace: popped.openBrace, closeBrace: i, name: popped.name });
			}
			i++;
			continue;
		}
		i++;
	}
	return indices;
}

// Walk the brace stack manually for a single text index so we know
// which function we're inside, even when nested control-flow braces
// are in between. Re-uses the same tokenizer as indexFunctions.
function enclosingFunctionAt(src, funcs, idx) {
	const braceStack = [];
	let inString = false;
	let stringCh = '';
	let inLineComment = false;
	let inBlockComment = false;
	let i = 0;
	while (i <= idx) {
		if (i === idx) {
			// Find the most recent func entry on the brace stack.
			for (let j = braceStack.length - 1; j >= 0; j--) {
				if (braceStack[j].name) {
					const fn = braceStack[j];
					return { openBrace: fn.openBrace, closeBrace: -1, name: fn.name };
				}
			}
			return null;
		}
		const ch = src[i];
		const next = src[i + 1];
		if (inLineComment) {
			if (ch === '\n') inLineComment = false;
			i++;
			continue;
		}
		if (inBlockComment) {
			if (ch === '*' && next === '/') {
				inBlockComment = false;
				i += 2;
				continue;
			}
			i++;
			continue;
		}
		if (inString) {
			if (ch === '\\') {
				i += 2;
				continue;
			}
			if (ch === stringCh) inString = false;
			i++;
			continue;
		}
		if (ch === '/' && next === '/') {
			inLineComment = true;
			i += 2;
			continue;
		}
		if (ch === '/' && next === '*') {
			inBlockComment = true;
			i += 2;
			continue;
		}
		if (ch === '"' || ch === '`') {
			inString = true;
			stringCh = ch;
			i++;
			continue;
		}
		if (ch === '{') {
			// We need the function name on the stack too. Use
			// precomputed funcs if possible — but the funcs list
			// only has top-level entries. Since control-flow braces
			// are not in funcs, we approximate by using the most
			// recent funcs entry whose openBrace <= i.
			let name = null;
			for (let j = funcs.length - 1; j >= 0; j--) {
				if (funcs[j].openBrace <= i) {
					name = funcs[j].name;
					break;
				}
			}
			braceStack.push({ name, openBrace: i });
			i++;
			continue;
		}
		if (ch === '}') {
			braceStack.pop();
			i++;
			continue;
		}
		i++;
	}
	return null;
}

// Given the function open-brace index, walk forward to find the
// matching close brace. Counts nested braces. Skips over string
// literals and line comments to avoid counting braces inside them.
function lineColOf(src, idx) {
	let line = 1;
	let col = 1;
	for (let i = 0; i < idx; i++) {
		if (src[i] === '\n') {
			line++;
			col = 1;
		} else col++;
	}
	return { line, col };
}

function guardPresentIn(body) {
	return GUARD_PATTERNS.some((re) => re.test(body));
}

function main() {
	const sites = [];
	let filesScanned = 0;
	for (const file of walkGoFiles(APPSHELL)) {
		filesScanned++;
		const basename = file.slice(file.lastIndexOf('/') + 1).replace(/\\/g, '/').split('/').pop();
		if (EXEMPT_FILES.has(basename)) continue;
		const src = readFileSync(file, 'utf8');
		const funcs = indexFunctions(src);
		// Build name -> closeBrace lookup. For short files this is
		// fine even though it scans by name (handlers are uniquely
		// named enough that collisions don't happen).
		const closeBraceByNameAndOpen = new Map();
		for (const f of funcs) {
			closeBraceByNameAndOpen.set(f.openBrace, f.closeBrace);
		}
		for (const m of src.matchAll(DIALOG_RE)) {
			const idx = m.index;
			const lineInfo = lineColOf(src, idx);
			const enclosing = enclosingFunctionAt(src, funcs, idx);
			if (!enclosing) {
				sites.push({
					file,
					line: lineInfo.line,
					func: '<unknown>',
					guarded: false,
					reason: 'could not locate enclosing function',
				});
				continue;
			}
			const closeBrace = closeBraceByNameAndOpen.get(enclosing.openBrace) ?? -1;
			const bodySrc = closeBrace > 0 ? src.slice(enclosing.openBrace, closeBrace + 1) : src.slice(enclosing.openBrace);
			sites.push({
				file,
				line: lineInfo.line,
				func: enclosing.name,
				guarded: guardPresentIn(bodySrc),
				reason: guardPresentIn(bodySrc) ? '' : 'no enterInFlight / LoadOrStore / guarded*Dialog / errExportInFlight in function body',
			});
		}
	}

	const unguarded = sites.filter((s) => !s.guarded);
	const guarded = sites.filter((s) => s.guarded);

	console.log(`Files scanned: ${filesScanned}`);
	console.log(`Native dialog call sites: ${sites.length}`);
	console.log(`  guarded:     ${guarded.length}`);
	console.log(`  unguarded:   ${unguarded.length}`);
	if (sites.length > 0) {
		console.log('');
		console.log('=== guarded sites ===');
		for (const s of guarded) {
			const rel = s.file.slice(ROOT.length).replace(/^\//, '');
			console.log(`  ${rel}:${s.line}  ${s.func}`);
		}
	}

	if (unguarded.length === 0) {
		console.log('');
		console.log('✓ every native dialog call is guarded per docs/agents/dialog-guard.md');
		process.exit(0);
	}

	console.log('');
	console.log('=== UNGUARDED SITES ===');
	console.log(
		'Each entry is a native dialog call whose enclosing function has no enterInFlight / LoadOrStore /',
	);
	console.log(
		'guarded*Dialog helper / errExportInFlight sentinel. Add a guard per docs/agents/dialog-guard.md',
	);
	console.log('or move the call into an exempt file with a // justification comment.');
	console.log('');
	for (const s of unguarded) {
		const rel = s.file.slice(ROOT.length).replace(/^\//, '');
		console.log(`  ${rel}:${s.line}  ${s.func}`);
		console.log(`    reason: ${s.reason}`);
	}

	if (process.argv.includes('--strict')) {
		console.log('');
		console.log('--strict: treating as a CI failure.');
	}
	process.exit(1);
}

main();
