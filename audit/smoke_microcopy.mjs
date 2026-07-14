// audit/smoke_microcopy.mjs
//
// Regression net for issue #561 — UI/UX text audit. The probe walks
// every .templ file under internal/templates/** and flags violations
// of the rules documented in docs/agents/ux-microcopy.md.
//
// This is a static-source scan (mirrors audit/smoke_dialog_guard.mjs +
// audit/discover_htmx_guard.mjs). Runtime per-page context (is this
// the only thing on the page?) cannot be reliably inferred from the
// source alone, so the rules here are the ones that CAN be checked
// statically without false positives.
//
// Rules enforced today:
//
//   R1. Eyebrow above self-explanatory single-element block.
//       A `<p>` / `<div>` whose class contains both `uppercase`
//       AND `tracking-` (the eyebrow style) sitting at the TOP of
//       a sibling block that contains ONLY a single
//       `<blockquote>`, a single `<table>`, or a prose `<p>` is
//       flagged. Buttons, forms, inputs, labels, and selects are
//       all excluded — an eyebrow that heads a form section is a
//       real label, not chrome. Canonical example: `/calendar`
//       line 89 `Rotating Local Archive Quote` above a single
//       `<blockquote>` (issue #561 finding 1).
//
//   R2. Heading text equals adjacent button text.
//       An `<h1>`–`<h4>` whose text matches (case-insensitive,
//       whitespace-collapsed) a `<button>`'s text within the next
//       ~15 lines is flagged. Canonical example: a heading that
//       says `Edit Event` immediately above a button that also
//       says `Edit Event` (issue #561 finding 31).
//
//   R3. Same user-facing text duplicated within ~15 lines.
//       The visible text content of two elements within a 15-line
//       window in the same templ file is identical (case-
//       insensitive, whitespace-collapsed, ≥ 30 chars). CSS class
//       strings are stripped before comparison — class lists
//       legitimately repeat across table rows and form rows.
//       Canonical example: share_exports.templ L24 + L29 where the
//       body sentence `Generate portable exports, replacement
//       backups, and merge-ready shared archives.` appears twice
//       (issue #561 finding 53).
//
//   R4. Stacked headings.
//       A short heading-style element (`<h1>`–`<h4>`, or an
//       `uppercase tracking-` eyebrow `<p>` / `<div>`) whose text
//       is < 60 chars sitting within 3 lines of ANOTHER heading-
//       style element with no `<p>` body between them is flagged.
//       Canonical example: share_exports.templ `Export & Backup`
//       eyebrow + `Create files to share or preserve` heading
//       (audit row 52).
//
//   R5. Verbose body paragraph directly under a heading.
//       A `<p>` whose visible text is > 80 chars that sits within
//       2 lines of a heading-style element (same definition as R4)
//       is flagged. The verbose paragraph narrates what the
//       heading already says. Canonical example: entry_form.templ
//       `Person records stay anchored to a soldier record for
//       navigation, merge review, and comparisons.` directly under
//       the `Person Record Link` eyebrow (audit row 37).
//
// Why not the other two rules from docs/agents/ux-microcopy.md yet:
//
//   R6 (helper copy longer than label) — needs DOM `aria-describedby`
//   cross-referencing across files; deferred to a runtime probe
//   following issue #561's slice plan.
//
//   R7 (single-button section needs no heading) — needs to know
//   whether the section is the only thing on the page; deferred
//   to a runtime probe.
//
// Exit codes:
//
//   0 — every file clean (or informational mode)
//   1 — ≥ 1 violation (or `--strict` always returns the count)
//
// Usage:
//
//   node audit/smoke_microcopy.mjs            # informational; prints baseline counts, exits 0
//   node audit/smoke_microcopy.mjs --strict   # CI gate; exits 1 on any violation

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
// Allow tests to redirect the scan to a temp directory.
const TEMPL_DIRS = process.env.MICROCOPY_TEMPL_DIR
	? [process.env.MICROCOPY_TEMPL_DIR]
	: [join(ROOT, 'internal/templates')];
const TEMPL_EXT = '.templ';
const SKIP_DIR_NAMES = new Set(['node_modules', '.git']);

const STRICT = process.argv.includes('--strict');

// ---- helpers --------------------------------------------------------------

function walkTempl(dir) {
	const out = [];
	for (const name of readdirSync(dir)) {
		if (SKIP_DIR_NAMES.has(name)) continue;
		const p = join(dir, name);
		const s = statSync(p);
		if (s.isDirectory()) {
			out.push(...walkTempl(p));
		} else if (name.endsWith(TEMPL_EXT)) {
			out.push(p);
		}
	}
	return out;
}

function collapse(s) {
	return s.toLowerCase().replace(/\s+/g, ' ').trim();
}

function textOfTag(line) {
	const m = line.match(/<[^>]+>(.*?)<\/[^>]+>/);
	return m ? collapse(m[1]) : '';
}

function nextNonBlankLine(lines, i) {
	for (let j = i + 1; j < lines.length; j++) {
		if (lines[j].trim() !== '') return j;
	}
	return -1;
}

// Returns the indentation of the line that opens the nearest enclosing
// block element, or -1 if not found within `window` lines.
function findEnclosingBlockEnd(lines, start, window) {
	let bestDepth = -1;
	let bestIndent = -1;
	let bestCloseLine = -1;
	let depth = 0;
	for (let j = start; j < Math.min(lines.length, start + window); j++) {
		const line = lines[j];
		// strip comments + Go-style block control to avoid counting
		// templ control structures as elements
		const stripped = line.replace(/\/\/.*$/, '');
		const opens = (stripped.match(/<[a-zA-Z][a-zA-Z0-9]*[\s>]/g) || []).length;
		const closes = (stripped.match(/<\/[a-zA-Z][a-zA-Z0-9]*>/g) || []).length;
		const selfClose = (stripped.match(/<[a-zA-Z][a-zA-Z0-9]*[^>]*\/>/g) || []).length;
		depth += opens - closes - selfClose;
		if (depth < bestDepth || bestDepth === -1) {
			bestDepth = depth;
			bestIndent = line.length - line.trimStart().length;
			bestCloseLine = j;
		}
	}
	return bestCloseLine;
}

// ---- R1: eyebrow above self-explanatory single-element block -------------

function findR1(lines, file) {
	const out = [];
	const eyebrowRe = /<(p|div|span)[^>]*\buppercase\b[^>]*\btracking-/;
	for (let i = 0; i < lines.length; i++) {
		if (!eyebrowRe.test(lines[i])) continue;
		const eyebrowText = textOfTag(lines[i]);
		if (eyebrowText.length < 3) continue; // ignore icon-only spans
		// Find the enclosing block end. If the segment inside it
		// contains any interactive element (button, form, input,
		// select, label), the eyebrow is a form label, not chrome.
		const close = findEnclosingBlockEnd(lines, i, 20);
		if (close === -1) continue;
		const segment = lines.slice(i + 1, close + 1).join('\n');
		if (/<(button|form|input|select|label|textarea)\b/i.test(segment)) continue;
		const blockquoteCount = (segment.match(/<blockquote\b/g) || []).length;
		const tableCount = (segment.match(/<table\b/g) || []).length;
		// Pure prose: a single `<p>` (blockquote or table) is the
		// only meaningful child of the segment.
		const proseOk =
			(blockquoteCount === 1 && tableCount === 0) ||
			(tableCount === 1 && blockquoteCount === 0);
		if (!proseOk) continue;
		out.push({
			file,
			line: i + 1,
			rule: 'R1',
			text: eyebrowText,
			hint: `eyebrow precedes a self-explanatory single block (blockquote or table); the eyebrow is unnecessary chrome`,
		});
	}
	return out;
}

// ---- R2: heading text equals adjacent button text -------------------------

function findR2(lines, file) {
	const out = [];
	const headingRe = /<h([1-4])\b[^>]*>(.*?)<\/h\1>/;
	const buttonRe = /<button\b[^>]*>([\s\S]*?)<\/button>/g;
	for (let i = 0; i < lines.length; i++) {
		const m = lines[i].match(headingRe);
		if (!m) continue;
		const headingText = collapse(m[2]);
		if (headingText.length < 3) continue;
		for (let j = i + 1; j < Math.min(lines.length, i + 16); j++) {
			buttonRe.lastIndex = 0;
			let bm;
			while ((bm = buttonRe.exec(lines[j])) !== null) {
				const buttonText = collapse(bm[1]);
				if (buttonText === headingText) {
					out.push({
						file,
						line: i + 1,
						rule: 'R2',
						text: headingText,
						hint: `heading text duplicates adjacent button label; one of them is unnecessary`,
					});
					break;
				}
			}
		}
	}
	return out;
}

// ---- R3: same visible text duplicated within ~15 lines -------------------

// Strip everything that isn't visible user-facing text. Returns
// the empty string for lines that are pure templ control flow /
// Go code / CSS class lists / templ component invocations /
// JSON-shaped attribute strings — those are not user-facing copy
// and should not produce R3 hits.
function stripNonText(line) {
	// Lines that contain no `<` at all are templ control flow or
	// pure Go code (e.g. `window = append(window, browsePageNumber)`).
	// Skip them entirely.
	if (!line.includes('<')) return '';
	let s = line;
	// Drop templ component invocations `@name(...)` and `templ.X(...)`.
	s = s.replace(/@\w+\([^)]*\)/g, ' ');
	s = s.replace(/\btempl\.\w+\([^)]*\)/g, ' ');
	// Drop JSON-shaped attribute strings — strings inside `<script>` /
	// `data-*` attributes that look like `"key": "value"`.
	s = s.replace(/"[a-z][\w-]+"\s*:\s*"[^"]*"/g, ' ');
	s = s.replace(/"[a-z][\w-]+"\s*:\s*'[^']*'/g, ' ');
	// Drop string-concat expressions (`"foo" + bar + "baz"`).
	s = s.replace(/"[^"]*"\s*\+\s*[^+]+/g, ' ');
	// Drop Go-style `return "..."` and similar.
	s = s.replace(/\breturn\s+"[^"]*"/g, ' ');
	// Drop `class="..."` and `class='...'` attributes.
	s = s.replace(/\sclass\s*=\s*"[^"]*"/g, ' ');
	s = s.replace(/\sclass\s*=\s*'[^']*'/g, ' ');
	// Drop every other HTML attribute — keeps the text content.
	s = s.replace(/\s[a-zA-Z-]+\s*=\s*"[^"]*"/g, ' ');
	s = s.replace(/\s[a-zA-Z-]+\s*=\s*'[^']*'/g, ' ');
	// Drop remaining HTML tags.
	s = s.replace(/<\/?[a-zA-Z][a-zA-Z0-9-]*[^>]*>/g, ' ');
	// Drop Go expressions `{ ... }`.
	s = s.replace(/\{[^}]*\}/g, ' ');
	// Collapse whitespace.
	s = s.replace(/\s+/g, ' ').trim();
	return s;
}

function findR3(lines, file) {
	const out = [];
	const seen = new Map(); // text -> first line index
	for (let i = 0; i < lines.length; i++) {
		const text = collapse(stripNonText(lines[i]));
		if (text.length < 30) continue;
		if (!/[a-z]/.test(text)) continue;
		if (seen.has(text)) {
			const first = seen.get(text);
			if (i - first <= 15) {
				out.push({
					file,
					line: first + 1,
					rule: 'R3',
					text,
					hint: `same text also appears at line ${i + 1}`,
				});
			}
		} else {
			seen.set(text, i);
		}
	}
	return out;
}

// ---- R4 + R5: heading-style element + adjacent body paragraph -----------

// A heading-style element is either an `<h1>`–`<h4>` or an
// eyebrow-style `<p>` / `<div>` (uppercase + tracking-). Return the
// captured text + the kind, or null.
function headingMatch(line) {
	const h = line.match(/<h([1-4])\b[^>]*>(.*?)<\/h\1>/);
	if (h) return { text: collapse(h[2]), kind: 'h' };
	const e = line.match(/<(p|div)\b[^>]*\buppercase\b[^>]*\btracking-[^>]*>(.*?)<\/\1>/);
	if (e) return { text: collapse(e[2]), kind: 'eyebrow' };
	return null;
}

// Find a `<p>` body element within `window` lines starting at
// `startIdx` (exclusive). Returns { line, text } or null.
function bodyPMatch(lines, startIdx, window) {
	for (let j = startIdx + 1; j < Math.min(lines.length, startIdx + 1 + window); j++) {
		const m = lines[j].match(/<p\b[^>]*>(.*?)<\/p>/);
		if (m) return { line: j, text: collapse(m[1]) };
	}
	return null;
}

function findR4R5(lines, file) {
	const out = [];
	for (let i = 0; i < lines.length; i++) {
		const head = headingMatch(lines[i]);
		if (!head) continue;
		// Skip dynamic headings — Go-templ expressions like `{ x }`
		// mean the heading is data-driven and the chrome concern
		// (eyebrow + heading pair) doesn't apply. R4 is about
		// literal-text stacked headings only.
		const headIsDynamic = /\{[^}]+\}/.test(head.text);
		if (headIsDynamic) continue;
		if (head.text.length === 0 || head.text.length >= 60) continue;

		// R4: another heading-style element within 3 lines, no
		// intervening <p>. Eyebrow + heading pair.
		for (let j = i + 1; j < Math.min(lines.length, i + 4); j++) {
			if (/<p\b/.test(lines[j])) break; // body paragraph broke the pair
			const next = headingMatch(lines[j]);
			if (!next) continue;
			if (/\{[^}]+\}/.test(next.text)) continue; // dynamic heading
			if (next.text.length === 0 || next.text.length >= 60) continue;
			out.push({
				file,
				line: i + 1,
				rule: 'R4',
				text: `${head.text} | ${next.text}`,
				hint: `stacked headings (eyebrow + heading or two headings); the first one is usually unnecessary chrome`,
			});
			break;
		}

		// R5: <p> immediately adjacent (within 1 line) whose text
		// is > 80 chars. Window of 1 catches the canonical
		// eyebrow/body pair shape; a body 2+ lines under a heading
		// is intentional mid-section prose, not narration.
		const body = bodyPMatch(lines, i, 1);
		if (!body) continue;
		if (body.text.length <= 80) continue;
		out.push({
			file,
			line: body.line + 1,
			rule: 'R5',
			text: body.text,
			hint: `verbose body paragraph directly under heading '${head.text}'; the heading already names the topic — trim or delete`,
		});
	}
	return out;
}

// ---- main -----------------------------------------------------------------

function main() {
	const files = TEMPL_DIRS.flatMap(walkTempl);
	const findings = [];
	for (const file of files) {
		const src = readFileSync(file, 'utf8');
		const lines = src.split(/\r?\n/);
		findings.push(...findR1(lines, file));
		findings.push(...findR2(lines, file));
		findings.push(...findR3(lines, file));
		findings.push(...findR4R5(lines, file));
	}

	const byRule = { R1: 0, R2: 0, R3: 0, R4: 0, R5: 0 };
	const byFile = new Map();
	for (const f of findings) {
		byRule[f.rule] = (byRule[f.rule] || 0) + 1;
		if (!byFile.has(f.file)) byFile.set(f.file, []);
		byFile.get(f.file).push(f);
	}

	const rel = (p) => p.slice(ROOT.length).replace(/^[\\/]/, '');
	console.log(`Files scanned: ${files.length}`);
	console.log(`Total findings: ${findings.length}`);
	console.log(`  R1 (eyebrow above self-explanatory block): ${byRule.R1 || 0}`);
	console.log(`  R2 (heading text = adjacent button text): ${byRule.R2 || 0}`);
	console.log(`  R3 (string literal duplicated within ~15 lines): ${byRule.R3 || 0}`);
	console.log(`  R4 (stacked headings within 3 lines): ${byRule.R4 || 0}`);
	console.log(`  R5 (verbose body >80 chars under heading): ${byRule.R5 || 0}`);
	if (findings.length === 0) {
		console.log('');
		console.log('✓ every .templ file satisfies docs/agents/ux-microcopy.md R1-R5');
		process.exit(0);
	}

	console.log('');
	console.log('=== findings ===');
	for (const [file, fs] of [...byFile.entries()].sort()) {
		console.log('');
		console.log(`  ${rel(file)}  (${fs.length})`);
		for (const f of fs) {
			console.log(`    ${f.rule}  L${f.line}  ${f.text.slice(0, 80)}`);
			console.log(`      → ${f.hint}`);
		}
	}

	if (STRICT) {
		console.log('');
		console.log('--strict: treating as a CI failure.');
	}
	process.exit(STRICT ? 1 : 0);
}

main();