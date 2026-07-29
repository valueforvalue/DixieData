import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { parseRoutebuilder } from './_lib/routebuilder_helpers.mjs';
import { parseGeneratedInvokers } from './_lib/invokers_from_generated.mjs';
import { parseFrontendInvokers } from './_lib/invokers_from_frontend.mjs';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const ROUTES_FILE = join(ROOT, 'internal/appshell/routes.go');
const ROUTEBUILDER_FILE = join(ROOT, 'internal/routebuilder/routebuilder.go');
const TEMPL_DIR = join(ROOT, 'internal/templates');
const FRONTEND_DIR = join(ROOT, 'frontend');

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
	/^\/lib\//,
	/^\/boot-[^/]+\.js$/,
	/^\/jobs\/active$/,
	/^\/browse\/results$/,
	/^\/soldiers\/search\/recent$/,
	/^\/soldiers\/display\//,
	/^\/articles\/[^/]+\/revisions$/,
	/^\/articles\/[^/]+\/raw$/,
	/^\/articles\/[^/]+\/picker$/,
	/^\/articles\/preview$/,
	/^\/events\/[^/]+\/delete$/,
	/^\/soldiers\/[^/]+\/events\/[^/]+\/attach$/,
	/^\/settings\/updates\/progress$/,
	/^\/settings\/updates\/health\/bootstrap$/,
	/^\/settings\/config$/,
	/^\/research\/recent$/,
	/^\/insights\/drilldown$/,
	/^\/share\/(exports|imports|sync)$/,
	/^\/inventory$/,
	/^\/version$/,
	/^\/compare$/,
	/^\/startup-error$/,
	/^\/export\//,
	/^\/share\/print-records-fragment$/,
	/^\/share\/queue\/presets/,
	/^\/integrations\/google\/calendar\/preferences\/save$/,
	/^\/images\/(screenshot|rotate)$/,
	/^\/open-link$/,
	/^\/articles\/[^/]+\/images\/import$/,
	/^\/articles\/[^/]+\/snapshot/,
	/^\/debug\.js$/,
	/^\/debug-toolbox\.js$/,
	/^\/htmx\.min\.js$/,
	/^\/soldiers\/[^/]+\/delete$/,
	/^\/articles\/[^/]+\/pdf$/,
	/^\/articles\/[^/]+\/images$/,
	/^\/articles\/[^/]+\/restore$/,
	/^\/events\/[^/]+\/pdf$/,
];

function readText(path) { return readFileSync(path, 'utf8'); }
function listBySuffix(dir, suffix) {
	const out = [];
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) continue;
		if (name.endsWith(suffix)) out.push(p);
	}
	return out;
}
function walkJs(dir) {
	const out = [];
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) { if (name !== 'node_modules' && !name.startsWith('_')) out.push(...walkJs(p)); continue; }
		if (name.endsWith('.js')) out.push(p);
	}
	return out;
}

const ROUTE_LINE = /\br\.(Get|Post|Patch|Put|Delete)\(\s*"([^"]+)"\s*,/g;
const INVOKER_RE = /(?:"data-action"|"action"|"href"|"hx-post"|"hx-get"|"hx-patch"|"hx-delete"|"hx-put")\s*:\s*"([^"]+)"|(?:data-action|action|href|hx-post|hx-get|hx-patch|hx-delete|hx-put)\s*=\s*"([^"]+)"|fmt\.Sprintf\(\s*"([^"]+)"/g;

function parseRoutes(text) {
	const routes = [];
	for (const m of text.matchAll(ROUTE_LINE)) routes.push({ method: m[1].toUpperCase(), path: m[2] });
	return routes;
}
function parseLegacyInvokers(text) {
	const out = [];
	for (const m of text.matchAll(INVOKER_RE)) { const v = m[1] || m[2] || m[3]; if (v) out.push(v); }
	return out;
}
function isAlwaysReachable(path) { return ALWAYS_REACHABLE.some((re) => re.test(path)); }
function normalizeForMatch(path) { return path.replace(/\{[^}]+\}/g, '').replace(/\/\*$/, '/'); }
function routeMatches(route, invoker) {
	if (route === invoker) return true;
	const rNorm = normalizeForMatch(route);
	const iNorm = normalizeForMatch(invoker);
	if (rNorm === iNorm) return true;
	if (iNorm.startsWith(rNorm) && (rNorm.endsWith('/') || iNorm[rNorm.length] === '/')) return true;
	return false;
}
function findOrphans(routes, invokers) {
	return routes.filter((r) => !isAlwaysReachable(r.path) && !invokers.some((inv) => routeMatches(r.path, inv)));
}
function collectAllInvokers() {
	const inv = [];
	const templFiles = listBySuffix(TEMPL_DIR, '.templ');
	for (const f of templFiles) for (const v of parseLegacyInvokers(readText(f))) inv.push(v);
	let helperMap;
	try { helperMap = parseRoutebuilder(readText(ROUTEBUILDER_FILE)); } catch { helperMap = new Map(); }
	const generatedFiles = listBySuffix(TEMPL_DIR, '_templ.go');
	for (const f of generatedFiles) for (const v of parseGeneratedInvokers(readText(f), helperMap)) inv.push(v);
	const jsFiles = walkJs(FRONTEND_DIR);
	for (const f of jsFiles) for (const v of parseFrontendInvokers(readText(f))) inv.push(v);
	return { inv, templFiles, generatedFiles, jsFiles, helperMap };
}

export { parseRoutes, parseLegacyInvokers, normalizeForMatch, routeMatches, findOrphans, isAlwaysReachable, parseFrontendInvokers };

const routes = parseRoutes(readText(ROUTES_FILE));
const { inv, templFiles, generatedFiles, jsFiles, helperMap } = collectAllInvokers();
const orphans = findOrphans(routes, inv);
console.log(`Routes registered: ${routes.length}`);
console.log(`Templ files scanned: ${templFiles.length}`);
console.log(`Generated files scanned: ${generatedFiles.length}`);
console.log(`Frontend files scanned: ${jsFiles.length}`);
console.log(`Routebuilder helpers loaded: ${helperMap.size}`);
console.log(`Total invokers (templ + generated + frontend): ${inv.length}`);
console.log(`Always-reachable (excluded): ${routes.filter((r) => isAlwaysReachable(r.path)).length}`);
console.log(`Orphan handlers (registered, no invoker): ${orphans.length}`);
console.log('');
for (const r of orphans) console.log(`  ${r.method.padEnd(6)} ${r.path}`);
if (process.argv.includes('--strict') && orphans.length) process.exit(1);
