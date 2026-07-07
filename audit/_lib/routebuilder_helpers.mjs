// audit/_lib/routebuilder_helpers.mjs
//
// Parse a routebuilder.go source file and return a Map<name, urlTemplate>
// where each urlTemplate is the helper's return value rewritten as a
// string with {argName} placeholders for parameters.
//
// Issue #369 (direction #2): the orphan-handler probe needs to know
// every URL that `routebuilder.X(...)` can produce so it can match
// registered routes against templ invokers written as
// `routebuilder.X(arg0, arg1)` rather than bare URL strings.
//
// Patterns supported:
//   1. `func F() string { return "/literal/path" }` — pass through.
//   2. `func F(a, b int) string { return fmt.Sprintf("/p/%d/%d", a, b) }`
//      — rewrite every %d (and %s / %v / %q) at the matching argName
//      position in the helper's signature.
//   3. `func F(id string) string { return "/p/" + url.PathEscape(strings.TrimSpace(id)) + "/sub" }`
//      — replace the helper-call expression with `{argName}`.
//   4. `func F(id string) string { return OtherHelper(id) + "?q=1" }`
//      — recurse into OtherHelper first to resolve the chain.
//
// Return-type gating: the helper ignores any function whose declared
// return type is NOT exactly `string`. Probe-side caller can decide
// how to surface that (currently: silently drop + warn once).

import { readFileSync } from 'node:fs';

/**
 * Parse a routebuilder.go source file text into a Map<name, urlTemplate>.
 * @param {string} text Source of the routebuilder.go file.
 * @returns {Map<string, string>} Each entry maps helper name to the
 *   rewritten URL template using `{argName}` placeholders.
 */
export function parseRoutebuilder(text) {
	const map = new Map();
	// Walk every `func NAME(<params>) <returnType> { <body> }` block.
	// A non-greedy match on the body keeps nested braces out of the
	// helper's signature/body but tolerates multi-line declarations.
	// `[\s\S]*?` matches across newlines; matching is bounded by the
	// outer `}` so chained-call helpers (single return expr) work.
	const re = /func\s+([A-Z]\w*)\s*\(([^)]*)\)\s*(?:\([^)]*\)\s*)?([*\[\]A-Za-z\w]+)\s*\{([\s\S]*?)\}/g;
	for (const m of text.matchAll(re)) {
		const name = m[1];
		const paramsSrc = m[2];
		const returnType = m[3];
		const body = m[4];
		if (returnType !== 'string') continue;
		const args = splitParams(paramsSrc);
		const urlTemplate = resolveBody(body.trim(), args, map);
		if (urlTemplate != null) map.set(name, urlTemplate);
	}
	return map;
}

/**
 * Resolve a helper's body to a URL template.
 * Body is the inside of `{ ... }`. We expect EXACTLY ONE return
 * statement — extract its argument and resolve it.
 * @param {string} body
 * @param {string[]} args Helper parameter names in declaration order.
 * @param {Map<string,string>} map Already-parsed helpers (for recursion).
 * @returns {string | null}
 */
function resolveBody(body, args, map) {
	const ret = body.match(/^\s*return\s+([\s\S]+?);?\s*$/);
	if (!ret) return null;
	return resolveExpr(ret[1].replace(/;\s*$/, '').trim(), args, map, new Set());
}

/**
 * Resolve a single Go expression to a URL template string.
 * Supported terminals: string literal, integer/variable identifier,
 * helper call, fmt.Sprintf(format, args...), binary + concatenation
 * with `/` or any other string-literal-stringifiable operand.
 * @param {string} expr
 * @param {string[]} args
 * @param {Map<string,string>} map
 * @param {Set<string>} visiting Cycle guard for recursion.
 * @returns {string | null}
 */
function resolveExpr(expr, args, map, visiting) {
	// Imported-helper calls used by routebuilder.go: url.PathEscape + strings.TrimSpace.
	// Both stringify their arg, so they're semantically `{argName}` for our purposes.
	const pathEscapeCall = expr.match(/^url\.PathEscape\((?:strings\.TrimSpace\(([A-Za-z_]\w*)\)|([A-Za-z_]\w*))\)$/);
	if (pathEscapeCall) {
		const argName = pathEscapeCall[1] || pathEscapeCall[2];
		if (args.includes(argName)) return `{${argName}}`;
		return null;
	}

	// fmt.Sprintf("/path/%d/%d", arg0, arg1, ...) — rewrite each format verb
	// to the corresponding positional argName (or {argN} for index >= args.length).
	const sprintfCall = expr.match(/^fmt\.Sprintf\(\s*"((?:\\.|[^"\\])*)"\s*(?:,\s*([^)]+))?\)$/);
	if (sprintfCall) {
		const format = sprintfCall[1];
		const argList = sprintfCall[2] ? sprintfCall[2].split(',').map((s) => s.trim()) : [];
		let i = 0;
		return format.replace(/%(?:[-+ 0#]*\d*(?:\.\d+)?)([sdvqboOxuXcCfFp])/g, () => {
			// %d%s%v%q etc all stringify; capture positional arg name.
			const a = argList[i++];
			return `{${a}}`;
		});
	}

	// Bare string literal: "/path".
	const lit = expr.match(/^"((?:\\.|[^"\\])*)"$/);
	if (lit) return lit[1];

	// Chained helper call: OtherHelper(arg) or OtherHelper(arg) + suffix.
	// We resolve the inner first; if it's an identifier in our param list,
	// wrap as `{name}`.
	const call = expr.match(/^([A-Z]\w*)\((.*)\)$/);
	if (call && map.has(call[1]) && !visiting.has(call[1])) {
		visiting.add(call[1]);
		const inner = resolveExpr(call[2], args, map, visiting);
		visiting.delete(call[1]);
		if (inner == null) return null;
		return map.get(call[1]).replace(/\{[A-Za-z_]\w*\}/g, (placeholder) => {
			// Single-arg case: replace all placeholders with the arg.
			return inner;
		});
	}

	// Bare identifier that's a parameter — `{name}`.
	if (/^[A-Za-z_]\w*$/.test(expr) && args.includes(expr)) {
		return `{${expr}}`;
	}

	// String concatenation with +. Each operand must itself resolve.
	const plusParts = splitTopLevelPlus(expr);
	if (plusParts && plusParts.length > 1) {
		const resolved = plusParts.map((p) => resolveExpr(p.trim(), args, map, visiting));
		if (resolved.some((r) => r == null)) return null;
		return resolved.join('');
	}

	return null;
}

/**
 * Split a Go expression at top-level `+` operators (those NOT inside
 * parens or string literals). Returns null if the expression doesn't
 * contain a top-level `+`.
 * @param {string} expr
 * @returns {string[] | null}
 */
function splitTopLevelPlus(expr) {
	const parts = [];
	let depth = 0;
	let inStr = false;
	let esc = false;
	let last = 0;
	for (let i = 0; i < expr.length; i++) {
		const c = expr[i];
		if (esc) { esc = false; continue; }
		if (inStr) {
			if (c === '\\') esc = true;
			else if (c === '"') inStr = false;
			continue;
		}
		if (c === '"') { inStr = true; continue; }
		if (c === '(') { depth++; continue; }
		if (c === ')') { depth--; continue; }
		if (c === '+' && depth === 0) {
			parts.push(expr.slice(last, i));
			last = i + 1;
		}
	}
	parts.push(expr.slice(last));
	if (parts.length === 1) return null;
	return parts;
}

/**
 * Split a Go parameter list by commas (no nested generics in
 * routebuilder signatures — params are primitives like `int`, `int64`,
 * `string`, `bool`).
 * @param {string} paramsSrc
 * @returns {string[]} Parameter names in declaration order.
 */
function splitParams(paramsSrc) {
	if (!paramsSrc.trim()) return [];
	return paramsSrc
		.split(',')
		.map((s) => s.trim().split(/\s+/))
		.filter((parts) => parts.length === 2)
		.map((parts) => parts[0]);
}

/**
 * Convenience: read a routebuilder.go file by path and return the map.
 * @param {string} path Absolute path to routebuilder.go.
 * @returns {Map<string,string>}
 */
export function parseRoutebuilderFile(path) {
	return parseRoutebuilder(readFileSync(path, 'utf8'));
}
