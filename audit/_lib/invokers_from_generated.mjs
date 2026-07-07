// audit/_lib/invokers_from_generated.mjs
//
// Walk a generated *_templ.go file text and extract every URL-shaped
// string that ends up in an HTML attribute. Three patterns:
//
//   1. templ.SafeURL("/literal/path")             — bare literal.
//   2. templ.SafeURL(fmt.Sprintf("..%d..%d", a, b)) — positional format.
//   3. templ.SafeURL(routebuilder.X(args))        — helper call,
//      resolved via the caller-supplied helper map (parsed by
//      routebuilder_helpers.mjs). Multi-line arg lists supported.
//
// Returns a Set of URL templates (each using {argName} placeholders
// that the probe then compares against the route registrations'
// {id}-style path segments).
//
// Issue #369 Slice 2 (direction #2 — generated-code scan).

/**
 * Walk templ-generated Go code and pull out every SafeURL arg.
 * @param {string} text Generated *_templ.go file source.
 * @param {Map<string,string>} helperMap routebuilder name -> URL template.
 * @returns {Set<string>} URL templates (with {argName} placeholders).
 */
export function parseGeneratedInvokers(text, helperMap) {
	const out = new Set();
	// Match every `templ.SafeURL(<arg>)` — the arg can span multiple
	// lines because Go formatter allows it. We capture by balanced
	// paren tracking instead of a regex tail so multi-line args work.
	let i = 0;
	const marker = 'templ.SafeURL(';
	while ((i = text.indexOf(marker, i)) !== -1) {
		const argStart = i + marker.length;
		const argEnd = findMatchingParen(text, argStart);
		if (argEnd === -1) { i = argStart; continue; }
		const arg = text.slice(argStart, argEnd);
		const url = parseSingleArg(arg.trim(), helperMap);
		if (url) out.add(url);
		i = argEnd + 1;
	}
	return out;
}

/**
 * Find the index of the `)` that closes the `(`. Returns -1 if not
 * balanced. Handles string literals + nested parens.
 * @param {string} text
 * @param {number} start Index immediately after the opening `(`.
 * @returns {number}
 */
function findMatchingParen(text, start) {
	let depth = 1;
	let i = start;
	let inStr = false;
	let esc = false;
	while (i < text.length) {
		const c = text[i];
		if (esc) { esc = false; i++; continue; }
		if (inStr) {
			if (c === '\\') { esc = true; i++; continue; }
			if (c === '"') inStr = false;
			i++;
			continue;
		}
		if (c === '"') { inStr = true; i++; continue; }
		if (c === '(') { depth++; i++; continue; }
		if (c === ')') {
			depth--;
			i++;
			if (depth === 0) return i - 1;
			continue;
		}
		i++;
	}
	return -1;
}

/**
 * Parse one captured SafeURL arg expression into a URL template.
 * Returns null for unparseable expressions (caller's signal-only
 * false-positive prevention: don't make the probe louder than the
 * existing HTML-attr scan).
 * @param {string} expr
 * @param {Map<string,string>} helperMap
 * @returns {string | null}
 */
function parseSingleArg(expr, helperMap) {
	// Pattern 1: bare string literal "/path".
	const lit = expr.match(/^"((?:\\.|[^"\\])*)"$/);
	if (lit) return lit[1];

	// Pattern 2: fmt.Sprintf("..%d..%s..", a, b, c, ...).
	const sp = expr.match(/^fmt\.Sprintf\(\s*"((?:\\.|[^"\\])*)"\s*(?:,\s*([\s\S]+))?\)$/);
	if (sp) {
		const format = sp[1];
		const argList = sp[2] ? splitTopLevelCommas(sp[2]).map((s) => s.trim()) : [];
		let j = 0;
		return format.replace(/%(?:[-+ 0#]*\d*(?:\.\d+)?)([sdvqboOxuXcCfFp])/g, () => {
			const a = argList[j++] ?? '?';
			return `{${a}}`;
		});
	}

	// Pattern 3: routebuilder.X(arg0, arg1) — multi-line or single-line args.
	const call = expr.match(/^routebuilder\.([A-Z]\w*)\(([\s\S]*)\)$/);
	if (call) {
		return _resolveHelperCall(call[1], call[2].trim(), helperMap);
	}

	return null;
}

/**
 * Resolve a routebuilder.X(arg) call into a URL template.
 * For helper template `/path/{id}/sub` + arg `s.ID`, returns
 * `/path/{id}/sub` (helper's template placeholders pass through
 * verbatim — they line up with the route registration's
 * `{id}` segment for the matching step).
 *
 * Exported so tests can pin the single-arg + field-access behaviour
 * independently of the paren-walker.
 * @param {string} helperName
 * @param {string} argListExpr Args text (already trimmed).
 * @param {Map<string,string>} helperMap
 * @returns {string | null}
 */
export function _resolveHelperCall(helperName, argListExpr, helperMap) {
	const tmpl = helperMap.get(helperName);
	if (!tmpl) return null;
	// Single-arg case: emit helper template verbatim. Multi-arg with
	// consistent argN->{argN} substitution: helper template already
	// uses placeholders shaped like {name}; we trust the call site
	// because routebuilder.go + handler route lines were authored
	// together (per the package contract).
	return tmpl;
}

/**
 * Split the comma-separated fmt.Sprintf arg list. Tolerates the
 * Go formatter wrapping across lines.
 * @param {string} s
 * @returns {string[]}
 */
function splitTopLevelCommas(s) {
	const out = [];
	let depth = 0;
	let inStr = false;
	let esc = false;
	let last = 0;
	for (let i = 0; i < s.length; i++) {
		const c = s[i];
		if (esc) { esc = false; continue; }
		if (inStr) {
			if (c === '\\') esc = true;
			else if (c === '"') inStr = false;
			continue;
		}
		if (c === '"') { inStr = true; continue; }
		if (c === '(' || c === '[' || c === '{') { depth++; continue; }
		if (c === ')' || c === ']' || c === '}') { depth--; continue; }
		if (c === ',' && depth === 0) {
			out.push(s.slice(last, i));
			last = i + 1;
		}
	}
	out.push(s.slice(last));
	return out;
}
