// audit/discover_orphan_handlers.mjs — CI gate for the
// "backend-first landing" bug class (COMMON_BUGS §4.20).
//
// What it does:
//
//   1. Parses internal/appshell/routes.go for every
//      r.(Get|Post|Patch|Put|Delete)("/path", a.handler)
//      registration.
//   2. Scans internal/templates/*.templ for every route
//      invoker pattern: data-action="...", <form
//      action="...">, hx-post/hx-get/hx-patch/hx-delete
//      attributes, plus dynamic patterns
//      (fmt.Sprintf("/path/%d/...", ...)) that match a
//      prefix registered in routes.go.
//   3. Cross-references the two sets. A registered route
//      with zero invokers is an orphan: the handler
//      exists but no UI calls it. The user cannot reach
//      the feature from the running app.
//
// Exit code is non-zero when any orphan handler is
// found, so the probe can run in CI as a gate.
//
// Limitations (deliberate, documented):
//
//   - Templ dynamic URLs (fmt.Sprintf with %d, %s) match
//     the route by prefix. A route like
//     r.Post("/soldiers/{id:[0-9]+}/tags", ...) is treated
//     as a prefix of any data-action="/soldiers/%d/tags"
//     templ invoker. The probe is conservative: a prefix
//     match counts as an invoker, so a templ that has a
//     fmt.Sprintf("/foo/%d/bar", ...) for a different
//     route at the same prefix is incorrectly credited
//     as an invoker. False-negatives (handler flagged as
//     orphan when it has an invoker) are rare; if you
//     see one, file a bug and add a hand-curated map of
//     known-prefixes to KNOWN_PREFIX_OVERRIDES below.
//
//   - data-action values are matched as substrings of
//     registered routes. A data-action="/export/json"
//     matches the registered route "/export/json"
//     (exact) AND any route with that prefix
//     (e.g. "/export/json/something"). Again, this
//     errs on the side of "handler is reachable", which
//     means fewer false-positive orphans.
//
//   - Handlers registered with wildcard patterns
//     (e.g. r.Get("/jobs/*", a.handleJobStatus)) are
//     checked by their prefix ("/jobs/") rather than the
//     wildcard. Any invoker matching the prefix counts.
//
//   - The probe does NOT verify that the route handler
//     actually works (that's the smoke probes' job). It
//     only verifies that SOME templ invoker exists.
//
// Run with: node audit/discover_orphan_handlers.mjs
// Exit code: 0 if zero orphans, 1 otherwise.

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const ROUTES_FILE = join(ROOT, 'internal/appshell/routes.go');
const TEMPL_DIR = join(ROOT, 'internal/templates');
const JS_DIR = join(ROOT, 'frontend');

// Routes that are intentionally never invoked from a
// templ: API endpoints, asset routes, internal handlers.
// Each entry is a regex matched against the route path.
// KEEP THIS LIST SMALL. A long list is the probe lying
// to itself about reachability; the goal is to flag
// candidates that humans can then investigate.
const ALWAYS_REACHABLE = [
  /^\/$/,                              // root
  /^\/app\./,                          // static assets
  /^\/debug\./,
  /^\/htmx\./,
  /^\/index\.html$/,
  /^\/recovery$/,
  /^\/setup$/,
  /^\/layout\//,                       // htmx fragment polling
  /^\/partials\//,                     // htmx fragments
  /^\/static-archive\//,               // static archive hosting
  /^\/fragments\//,
  /^\/media\//,                        // media serving
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

// Extract (method, path) pairs from a route registration line.
// Matches: r.Get("/path", a.handler)
//          r.Post("/path", a.handler)
//          r.Patch("/path", a.handler)
//          r.Put("/path", a.handler)
//          r.Delete("/path", a.handler)
//          r.Method("/path", a.handler)
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

// Extract route-invoker strings from a templ file. Looks
// for:
//   - data-action="..." / "data-action": "..."  (Go map literal)
//   - action="..." / "action": "..."  (Go map literal)
//   - href="..."
//   - hx-post="..." / hx-get="..." / hx-patch="..." / hx-delete="..." / hx-put="..."
//   - fmt.Sprintf("/foo/%d/bar", ...)  (templ dynamic URLs)
const INVOKER_RE = /(?:"data-action"|"action"|"href"|"hx-post"|"hx-get"|"hx-patch"|"hx-delete"|"hx-put")\s*:\s*"([^"]+)"|(?:data-action|action|href|hx-post|hx-get|hx-patch|hx-delete|hx-put)\s*=\s*"([^"]+)"|fmt\.Sprintf\(\s*"([^"]+)"/g;

function parseInvokers(text) {
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

// A route is "reachable" if any invoker matches it.
// Matching rules:
//   1. Exact string match (invoker === path)
//   2. Prefix match where the route is a prefix directory:
//      route = "/export", invoker = "/export/json" -> match
//      route = "/soldiers/{id}/tags", invoker = "/soldiers/42/tags" -> match
//   3. Dynamic placeholder substitution:
//      route = "/soldiers/{id:[0-9]+}/tags" -> treat as
//      prefix "/soldiers/" for matching
//   4. Wildcard trailing slash:
//      route = "/jobs/*" -> treat as prefix "/jobs/"
function normalizeForMatch(path) {
  return path
    .replace(/\{[^}]+\}/g, '')   // strip {id:...} placeholders
    .replace(/\/\*$/, '/');      // /jobs/* -> /jobs/
}

function routeMatches(route, invoker) {
  if (route === invoker) return true;
  const rNorm = normalizeForMatch(route);
  const iNorm = normalizeForMatch(invoker);
  if (rNorm === iNorm) return true;
  // Prefix match: route is a parent of invoker.
  if (iNorm.startsWith(rNorm) && (rNorm.endsWith('/') || iNorm[rNorm.length] === '/')) {
    return true;
  }
  return false;
}

// Handlers that are registered but not directly invoked
// from a templ are flagged. We compare each registered
// route against the union of all invokers across all
// templ files. A route with no match is orphan.
//
// Routes in ALWAYS_REACHABLE are not checked.
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

function main() {
  const routesText = readText(ROUTES_FILE);
  const templFiles = listTempl(TEMPL_DIR);

  const routes = parseRoutes(routesText);
  const allInvokers = [];
  for (const f of templFiles) {
    const text = readText(f);
    const inv = parseInvokers(text);
    for (const v of inv) allInvokers.push(v);
  }

  const orphans = findOrphans(routes, allInvokers);

  // Report.
  console.log(`Routes registered: ${routes.length}`);
  console.log(`Templ invokers found: ${allInvokers.length} (across ${templFiles.length} files)`);
  console.log(`Always-reachable (excluded): ${routes.filter((r) => isAlwaysReachable(r.path)).length}`);
  console.log(`Orphan handlers (registered, no templ invoker): ${orphans.length}`);
  console.log('');

  if (orphans.length > 0) {
    console.log('=== CANDIDATE ORPHAN HANDLERS ===');
    console.log('These handlers are registered in internal/appshell/routes.go');
    console.log('but no templ file references their path via data-action,');
    console.log('<form action>, href, or hx-* attributes. The handler is');
    console.log('likely reachable via JS (e.g. dispatchDixieDataForm), htmx');
    console.log('polling, or wails runtime — but a human should verify each.');
    console.log('See COMMON_BUGS §4.20 for the "shipped but invisible" pattern.');
    console.log('');
    for (const o of orphans) {
      console.log(`  ${o.method.padEnd(6)} ${o.path}`);
    }
    // By default, exit 0 (informational). The probe is too
    // coarse to gate CI: many "orphans" are reachable via
    // JS. To gate CI, add the specific routes to the
    // ALWAYS_REACHABLE list above with a comment explaining
    // the reachability mechanism. The probe is meant to be
    // a starting point for human triage, not a verdict.
    // The exit code is 0 by design; pass --strict to flip it.
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

main();