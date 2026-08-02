/**
 * Walk frontend/app.js (and lib/) for invoker-style URL strings:
 *   - fetch("...")
 *   - fetch(`/articles/${id}/images/import`, ...)
 *   - hx-get / hx-post / etc inside HTML strings (rare in .js)
 * Returns a Set of URL templates (with {argName} placeholders for
 * template-literal interpolations). Hand-rolled instead of AST so
 * the probe stays a plain ESM file with no extra deps.
 *
 * The runtime/Go probe only needs to match these against the route
 * table, so the templates can be loose -- e.g. `${id}` is recorded
 * as `{id}` and the route `/{id}` matches.
 */
export function parseFrontendInvokers(text) {
	const out = new Set();
	const fetchRe = /fetch\(\s*([`'"])([\s\S]*?)\1/g;
	for (const m of text.matchAll(fetchRe)) {
		const tpl = m[2];
		if (tpl) out.add(templateLiteral(tpl));
	}
	// Bare string URLs the dispatcher / wails bridge hardcodes, e.g.
	// return "/jobs/" + jobID; but those still end up wrapped in fetch()
	// above OR in a templ fmt.Sprintf already. Skip; the fetch pass
	// covers the surface area that matters.
	return out;
}

function templateLiteral(s) {
	// Convert `${expr}` to `{expr}` so the route matcher treats the
	// interpolation as a parameter (matches routebuilder conventions
	// and the chi path templates like `/{id:[0-9]+}/events/...`).
	return s.replace(/\$\{([^}]+)\}/g, '{$1}');
}
