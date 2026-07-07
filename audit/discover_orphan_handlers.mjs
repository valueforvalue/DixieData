// audit/discover_orphan_handlers.mjs
//
// Probe that cross-references every route registered in
// internal/appshell/routes.go against every URL invoker found in:
//
//   1. Templ files (the original heuristic) — HTML attribute
//      strings + fmt.Sprintf legacy patterns. RETAINED as a
//      safety net for legacy handlers that don't use routebuilder.
//   2. Generated *_templ.go files — every templ.SafeURL(arg)
//      expression where arg is a literal, fmt.Sprintf, or
//      routebuilder.X(...) call. The arg is captured and resolved
//      against a Map of routebuilder helpers parsed from
//      internal/routebuilder/routebuilder.go.
//   3. Always-reachable prefixes (front controller + partials
//      + static archive + fragments + media).
//
// Any route that survives the three passes is reported as a
// candidate orphan — a registered handler whose templ invoker
// the probe cannot see. Such handlers ARE often reachable (via
// JS dispatcher / fetch / wails runtime) but lack the templ
// shape we'd expect for a discoverable navigation. The probe's
// job is signal-only: surface the candidate, let a human verify.
//
// Issue #369 (direction #2 — generated-code scan). Slice 3
// of the locked plan published in the issue body.

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

import { parseRoutebuilder } from './_lib/routebuilder_helpers.mjs';
import { parseGeneratedInvokers } from './_lib/invokers_from_generated.mjs';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const ROUTES_FILE = join(ROOT, 'internal/appshell/routes.go');
const ROUTEBUILDER_FILE = join(ROOT, 'internal/routebuilder/routebuilder.go');
const TEMPL_DIR = join(ROOT, 'internal/templates');
const JS_DIR = join(ROOT, 'frontend');

const ALWAYS_REACHABLE = [
	/^\/$/,
	/^\/app\./,
	/^\/debug(\/|$)/,
	/^\/htmx(\/|$)/,
	/^\/index\.html$/,
	/^\/recovery$/,
	/^\/setup$/,
	/^\/layout\//,
	/^\/partials\//,
	/^\/static-archive\//,
	/^\/fragments\//,
	/^\/media\//,
];

function readText(path) {
	return readFileSync(path, 'utf8');
}

function listTempl(dir) {
	const out = [];
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) continue;
		if (name.endsWith('.templ')) out.push(p);
	}
	return out;
}

function listGenerated(dir) {
	const out = [];
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) continue;
		if (name.endsWith('_templ.go')) out.push(p);
	}
	return out;
}

const ROUTE_LINE = /\br\.(Get|Post|Patch|Put|Delete)\(\s*"([^"]+)"\s*,/g;

function parseRoutes(text) {
	const routes = [];
	for (const m of text.matchAll(ROUTE_LINE)) {
		const method = m[1].toUpperCase();
		const path = m[2];
		routes.push({ method, path });
	}
	return routes;
}

// Legacy HTML-attribute + fmt.Sprintf scan. RETAINED as a safety
// net for handlers that pre-date the routebuilder package (and
// for any future ad-hoc templ string literals that don't use
// routebuilder.X(...)).
const INVOKER_RE = /(?:"data-action"|"action"|"href"|"hx-post"|"hx-get"|"hx-patch"|"hx-delete"|"hx-put")\s*:\s*"([^"]+)"|(?:data-action|action|href|hx-post|hx-get|hx-patch|hx-delete|hx-put)\s*=\s*"([^"]+)"|fmt\.Sprintf\(\s*"([^"]+)"/g;

function parseLegacyInvokers(text) {
	const out = [];
	for (const m of text.matchAll(INVOKER_RE)) {
		const v = m[1] || m[2] || m[3];
		if (v) out.push(v);
	}
	return out;
}

function isAlwaysReachable(path) {
	return ALWAYS_REACHABLE.some((re) => re.test(path));
}

function normalizeForMatch(path) {
	return path
		.replace(/\{[^}]+\}/g, '')
		.replace(/\/\*$/, '/');
}

function routeMatches(route, invoker) {
	if (route === invoker) return true;
	const rNorm = normalizeForMatch(route);
	const iNorm = normalizeForMatch(invoker);
	if (rNorm === iNorm) return true;
	if (iNorm.startsWith(rNorm) && (rNorm.endsWith('/') || iNorm[rNorm.length] === '/')) {
		return true;
	}
	return false;
}

function findOrphans(routes, invokers) {
	const orphans = [];
	for (const r of routes) {
		if (isAlwaysReachable(r.path)) continue;
		const matched = invokers.some((inv) => routeMatches(r.path, inv));
		if (!matched) {
			orphans.push(r);
		}
	}
	return orphans;
}

function collectAllInvokers() {
	const inv = [];

	const templFiles = listTempl(TEMPL_DIR);
	for (const f of templFiles) {
		const text = readText(f);
		for (const v of parseLegacyInvokers(text)) inv.push(v);
	}

	let helperMap;
	try {
		helperMap = parseRoutebuilder(readText(ROUTEBUILDER_FILE));
	} catch {
		helperMap = new Map();
	}

	const generatedFiles = listGenerated(TEMPL_DIR);
	for (const f of generatedFiles) {
		const text = readText(f);
		for (const v of parseGeneratedInvokers(text, helperMap)) inv.push(v);
	}

	return { inv, templFiles, generatedFiles, helperMap };
}

export { parseRoutes, parseLegacyInvokers, normalizeForMatch, routeMatches, findOrphans, isAlwaysReachable };

function main() {
	const routesText = readText(ROUTES_FILE);
	const routes = parseRoutes(routesText);
	const { inv, templFiles, generatedFiles, helperMap } = collectAllInvokers();

	const orphans = findOrphans(routes, inv);

	console.log(`Routes registered: ${routes.length}`);
	console.log(`Templ invokers found (legacy HTML-attr + fmt.Sprintf): ${templFiles.length} files scanned`);
	console.log(`Routebuilder helpers loaded: ${helperMap.size}`);
	console.log(`Generated *_templ.go scanned: ${generatedFiles.length}`);
	console.log(`Total invokers (legacy + generated): ${inv.length}`);
	console.log(`Always-reachable (excluded): ${routes.filter((r) => isAlwaysReachable(r.path)).length}`);
	console.log(`Orphan handlers (registered, no invoker): ${orphans.length}`);
	console.log('');

	if (orphans.length > 0) {
		console.log('=== CANDIDATE ORPHAN HANDLERS ===');
		console.log('These handlers are registered in internal/appshell/routes.go');
		console.log('but the probe could not find a templ invoker (routebuilder call,');
		console.log('HTML attribute, or fmt.Sprintf). They may be reachable via JS');
		console.log('dispatch (app.js fetch), htmx polling, wails runtime, or a code');
		console.log('path the probe does not yet see. A human should verify each.');
		console.log('See COMMON_BUGS §4.20 for the "shipped but invisible" pattern.');
		console.log('');
		for (const o of orphans) {
			console.log(`  ${o.method.padEnd(6)} ${o.path}`);
		}

		if (process.argv.includes('--strict')) {
			console.log('');
			console.log('--strict: treating as a CI failure.');
			process.exit(1);
		}
		process.exit(0);
	} else {
		console.log('✓ No orphan handlers. Every registered route has a templ invoker.');
		process.exit(0);
	}
}

const isEntrypoint = (() => {
	if (!process.argv[1]) return false;
	return process.argv[1].replace(/\\/g, '/').endsWith('discover_orphan_handlers.mjs');
})();

if (isEntrypoint) {
	main();
}
