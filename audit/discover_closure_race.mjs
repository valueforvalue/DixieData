// audit/discover_closure_race.mjs
//
// Static-analysis probe for the closure-capture race pattern
// that issue #419 shipped as a fix:
//
//   var jobID string
//   jobID = a.jobs.Start(kind, func(ctx, p) error {
//       a.jobs.SetResultPath(jobID, path)  // <- reads outer-scope jobID
//       ...
//   })
//
// The worker goroutine spawned by a.jobs.Start reads the
// outer-scope `jobID` (or any other outer-scope variable
// assigned by the same `=` statement that captured the
// return value). Under -race the detector flags the
// unsynchronized read inside the worker vs the write in the
// outer code after Start returns. The failure mode is silent
// in production: the worker may fire before the outer
// assignment lands, call SetResultPath("", path) (no-op,
// no such job), and the per-job stats land on the wrong
// row.
//
// This probe catches the pattern before the slice merges.
// Two flavours of the pattern:
//
//   (a) closure reads outer-scope variable on the LHS of
//       `= jobs.Start(...)` directly (the #419 case)
//   (b) closure reads an outer-scope variable that was
//       declared in the same statement (e.g. `id := jobs.Start`)
//       via `var id string; id = jobs.Start(...)` — same
//       race, different shape
//
// What the probe CANNOT catch (false-negative limits):
//
//   - Channel-handoff fixes (the #419 fix shape) DO use
//     `var jobID string; jobID = jobs.Start(...)` but the
//     worker reads the value from a channel (`<-jobIDCh`),
//     not from outer-scope `jobID`. The probe sees the
//     outer-scope assignment and the worker reference to
//     `jobID` and may false-positive. Each candidate has a
//     `confidence` field; high-confidence means the worker
//     reads the outer-scope variable directly (no channel
//     handoff in sight); low-confidence means the worker
//     MIGHT be reading it through a channel and the next
//     agent should verify by reading the file.
//
//   - Workers that read outer-scope variables NOT assigned
//     from jobs.Start (e.g. a `tempDir` captured by
//     reference). These are usually safe because the
//     outer-scope variable is assigned BEFORE Start is
//     called, so the assignment happens-before the
//     goroutine spawn. The probe ignores such reads.
//
//   - Cross-file races (the probe only reads one file at a
//     time). JobID-passing between packages would not be
//     caught.
//
//   - AST-aware patterns: the probe uses regex + line-by-line
//     analysis, so a hand-rolled `func(... )` form that
//     doesn't match the standard `a.jobs.Start(kind, func(`
//     pattern would be missed. The probe's regex is broad
//     enough to catch the common Go idioms; exotic patterns
//     require code review.
//
// Run with: node audit/discover_closure_race.mjs
//           node audit/discover_closure_race.mjs --strict
// Exit code: 0 if no high-confidence candidates, 1 if any.
// Low-confidence candidates always exit 0 (informational).

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const APPSHELL_DIR = join(ROOT, 'internal/appshell');

const STRICT = process.argv.includes('--strict');

function readText(path) {
	return readFileSync(path, 'utf8');
}

function listGoFiles(dir) {
	const out = [];
	for (const name of readdirSync(dir)) {
		const full = join(dir, name);
		if (statSync(full).isFile() && name.endsWith('.go')) {
			out.push(full);
		}
	}
	return out;
}

// findWorkers returns the line ranges of every `func(...) { ... }`
// expression that is the worker argument to a jobs.Start call.
// Each range covers from `func(` through the closing `}` of
// the closure body.
function findWorkers(source) {
	const ranges = [];
	// Regex anchors on `.Start(` (jobs.Start, a.jobs.Start, etc.)
	// then locates the worker `func(` token that follows.
	const startRe = /\.\s*Start(?:Manual)?\s*\([^)]*,\s*func\s*\(/g;
	let m;
	while ((m = startRe.exec(source)) !== null) {
		// Walk forward from the `func(` matching parens to find the
		// end of the func signature, then continue matching braces
		// to find the end of the closure body.
		let i = m.index + m[0].length;  // points AT `(` of func(
		let depth = 1;  // the outer func( open paren
		let inString = false;
		let stringCh = '';
		let inLineComment = false;
		let inBlockComment = false;
		while (i < source.length && depth > 0) {
			const c = source[i];
			if (inLineComment) {
				if (c === '\n') inLineComment = false;
				i++;
				continue;
			}
			if (inBlockComment) {
				if (c === '*' && source[i + 1] === '/') { inBlockComment = false; i += 2; continue; }
				i++;
				continue;
			}
			if (inString) {
				if (c === '\\' && i + 1 < source.length) { i += 2; continue; }
				if (c === stringCh) inString = false;
				i++;
				continue;
			}
			if (c === '/' && source[i + 1] === '/') { inLineComment = true; i += 2; continue; }
			if (c === '/' && source[i + 1] === '*') { inBlockComment = true; i += 2; continue; }
			if (c === '"' || c === "'" || c === '`') { inString = true; stringCh = c; i++; continue; }
			if (c === '(') depth++;
			else if (c === ')') depth--;
			i++;
		}
		// i is now AT the `)` that closes the func signature.
		// Continue past the close-paren AND the return-type
		// identifier (e.g. `error`, `int64`, `(int64, error)`),
		// then match braces for the body.
		if (depth !== 0) continue;
		// Skip past the return type: it's either a single bare
		// identifier (`error`) or a parenthesised list
		// `(int64, error)` or a multi-word form (`context.Context`,
		// `*jobs.Progress`). Walk forward, skipping over a balanced
		// paren group if present, then over a balanced sequence of
		// identifiers + dots + asterisks.
		i++;  // step past the closing `)` of the func signature
		// Optional parenthesised return type.
		while (i < source.length && /\s/.test(source[i])) i++;
		if (source[i] === '(') {
			let pd = 1; i++;
			while (i < source.length && pd > 0) {
				if (source[i] === '(') pd++;
				else if (source[i] === ')') pd--;
				i++;
			}
		}
		// Skip past the bare return type identifier(s) and any
		// pointer asterisk.
		while (i < source.length) {
			// whitespace
			while (i < source.length && /\s/.test(source[i])) i++;
			if (/[A-Za-z_]/.test(source[i])) {
				while (i < source.length && /[A-Za-z0-9_\.]/.test(source[i])) i++;
			} else if (source[i] === '*') {
				i++;
			} else {
				break;
			}
		}
		if (source[i] !== '{') continue;  // unexpected shape, skip
		// Walk forward matching braces.
		let braceDepth = 1;
		i++;  // skip the opening `{`
		inString = false; inLineComment = false; inBlockComment = false;
		while (i < source.length && braceDepth > 0) {
			const c = source[i];
			if (inLineComment) {
				if (c === '\n') inLineComment = false;
				i++;
				continue;
			}
			if (inBlockComment) {
				if (c === '*' && source[i + 1] === '/') { inBlockComment = false; i += 2; continue; }
				i++;
				continue;
			}
			if (inString) {
				if (c === '\\' && i + 1 < source.length) { i += 2; continue; }
				if (c === stringCh) inString = false;
				i++;
				continue;
			}
			if (c === '/' && source[i + 1] === '/') { inLineComment = true; i += 2; continue; }
			if (c === '/' && source[i + 1] === '*') { inBlockComment = true; i += 2; continue; }
			if (c === '"' || c === "'" || c === '`') { inString = true; stringCh = c; i++; continue; }
			if (c === '{') braceDepth++;
			else if (c === '}') braceDepth--;
			i++;
		}
		if (braceDepth !== 0) continue;
		ranges.push({
			startLine: sourceLine(source, m.index),
			endLine: sourceLine(source, i),
			// body covers from the func signature start through the
			// closing brace so identifier extraction sees the worker
			// body too.
			body: source.slice(m.index + m[0].length, i),
			callLine: sourceLine(source, m.index),
			callStart: m.index,
		});
	}
	return ranges;
}

function sourceLine(source, idx) {
	let line = 1;
	for (let i = 0; i < idx && i < source.length; i++) {
		if (source[i] === '\n') line++;
	}
	return line;
}

// findOuterAssignments returns the set of variable names that
// were assigned by the OUTER code on the SAME line as the
// .Start( call OR on a line AFTER it (i.e. the assignment
// happens AFTER Start returns, so the goroutine spawned by
// Start could race with it). Variables assigned on lines
// BEFORE the Start line are excluded because the assignment
// happens-before the goroutine spawn.
//
// Most common race shape: `var X string\r\nX = a.jobs.Start(...)`
// — the `X = ...` is on the same line as the .Start( call (the
// LHS of the assignment is `X`, the RHS is `a.jobs.Start(...)`).
function findOuterAssignments(source, startIdx, startLine) {
	const vars = new Set();
	// 1. Same-line assignment: `X = a.jobs.Start(...)` — the LHS
	//    identifier is the racing variable.
	const sameLineStart = source.lastIndexOf('\n', startIdx - 1) + 1;
	const sameLineEnd = source.indexOf('\n', startIdx);
	const sameLine = source.slice(sameLineStart >= 0 ? sameLineStart : 0, sameLineEnd >= 0 ? sameLineEnd : source.length);
	const sameLineAssign = sameLine.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*[A-Za-z_]/);
	if (sameLineAssign) {
		vars.add(sameLineAssign[1]);
	}
	// 2. Look back over the preceding 12 lines for `var X ...`
	//    declarations whose value is assigned on this same line
	//    via `X =`. The `var` itself doesn't race; the `X =`
	//    does, but we already captured that via the same-line
	//    match above. We just need to ALSO know the worker might
	//    reference the declared variable name, so include it.
	const lookbackStart = Math.max(0, startIdx - 800);
	const window = source.slice(lookbackStart, startIdx);
	const lines = window.split(/\r?\n/);
	for (const line of lines) {
		const decl = line.match(/^\s*var\s+([A-Za-z_][A-Za-z0-9_]*)\b/);
		if (decl) {
			vars.add(decl[1]);
			continue;
		}
	}
	return vars;
}

// channelHandoffInWorker returns true if the worker body
// reads from a channel (`<-ch`) — a sign that the slice is
// using the #419 fix shape (channel handoff) and the
// outer-scope variable is no longer being read directly.
function channelHandoffInWorker(body) {
	return /<-\s*[A-Za-z_][A-Za-z0-9_]*\b/.test(body);
}

// removeStringContents removes the contents of string and
// rune literals so the regex doesn't match identifiers
// inside comments or strings.
function stripLiterals(source) {
	let out = '';
	let i = 0;
	while (i < source.length) {
		const c = source[i];
		if (c === '/' && source[i + 1] === '/') {
			const end = source.indexOf('\n', i);
			i = end < 0 ? source.length : end;
			continue;
		}
		if (c === '/' && source[i + 1] === '*') {
			const end = source.indexOf('*/', i + 2);
			i = end < 0 ? source.length : end + 2;
			continue;
		}
		if (c === '"' || c === "'" || c === '`') {
			out += '""';
			if (c === '`') {
				const end = source.indexOf('`', i + 1);
				i = end < 0 ? source.length : end + 1;
			} else {
				while (i < source.length && source[i] !== c) {
					if (source[i] === '\\') i++;
					i++;
				}
				i++;
			}
			continue;
		}
		out += c;
		i++;
	}
	return out;
}

// identifiersInBody returns the set of bare identifier
// references in the worker body (string contents stripped
// first so identifier-like text in docstrings is ignored).
function identifiersInBody(body) {
	const stripped = stripLiterals(body);
	const out = new Set();
	const re = /\b([A-Za-z_][A-Za-z0-9_]*)\b/g;
	let m;
	while ((m = re.exec(stripped)) !== null) {
		out.add(m[1]);
	}
	return out;
}

const candidates = [];

for (const file of listGoFiles(APPSHELL_DIR)) {
	const rel = file.slice(ROOT.length).replace(/\\/g, '/');
	const source = readText(file);
	const workers = findWorkers(source);
	for (const w of workers) {
		const outerVars = findOuterAssignments(source, w.callStart, w.callLine);
		const workerIdents = identifiersInBody(w.body);
		const usesChannel = channelHandoffInWorker(w.body);
		for (const v of outerVars) {
			if (!workerIdents.has(v)) continue;
			// The worker references the outer-scope variable.
			// If the worker ALSO reads from a channel, this is
			// likely the #419 fix shape (channel handoff) and
			// the channel read replaces the outer-scope read.
			// We can't be 100% sure without AST analysis; mark
			// as low-confidence.
			candidates.push({
				file: rel,
				startLine: w.callLine,
				endLine: w.endLine,
				variable: v,
				confidence: usesChannel ? 'low' : 'high',
				usesChannel,
			});
		}
	}
}

function findCallStart(source, line) {
	// find the byte index of the start of the given line number
	let cur = 1;
	for (let i = 0; i < source.length; i++) {
		if (cur === line) return i;
		if (source[i] === '\n') cur++;
	}
	return 0;
}

const highConfidence = candidates.filter((c) => c.confidence === 'high');
const lowConfidence = candidates.filter((c) => c.confidence === 'low');

console.log(`Closure-capture race probe (issue #419 pattern):`);
console.log(`  Files scanned: ${listGoFiles(APPSHELL_DIR).length}`);
console.log(`  .Start workers found: ${workers_total()}`);
console.log(`  Candidates: ${candidates.length} (high=${highConfidence.length}, low=${lowConfidence.length})`);
console.log('');

if (candidates.length === 0) {
	console.log('No candidates. ✓');
	process.exit(0);
}

console.log('=== CANDIDATES ===');
for (const c of candidates) {
	console.log(`  [${c.confidence}]  ${c.file}:${c.startLine}-${c.endLine}  reads outer-scope \`${c.variable}\`${c.usesChannel ? ' (worker also has channel reads; verify handoff shape manually)' : ''}`);
}

if (highConfidence.length > 0) {
	console.log('');
	console.log(`Found ${highConfidence.length} high-confidence candidate(s) \u2014 likely #419 closure-capture race.`);
	if (STRICT) {
		console.log('--strict: treating as a CI failure.');
		process.exit(1);
	}
} else {
	console.log('');
	console.log('Low-confidence candidates only \u2014 verify manually that the channel handoff is in place.');
}

function workers_total() {
	let total = 0;
	for (const file of listGoFiles(APPSHELL_DIR)) {
		const source = readText(file);
		total += findWorkers(source).length;
	}
	return total;
}

process.exit(0);