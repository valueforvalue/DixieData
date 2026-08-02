// lint-button-actions-resolve.mjs (issue #687)
//
// Source-scan probe that walks every .templ file
// under internal/templates/**/*.templ and asserts every
// invoker URL (form action="...", data-action="...",
// hx-get="...", hx-post="...") resolves to a route
// registered in internal/appshell/routes.go OR matches
// one of the documented allowlists:
//   1. templ.SafeURL(routebuilder.X(...)) — the canonical
//      typed builder (resolves at code-review time).
//   2. External URLs (http://, https://, mailto:, #).
//   3. Dev-only or asset-paths allowlist: /debug/client-logs,
//      /_lib/..., /wails/...
//
// The two historical #687 instances were template-split
// button URL drift (69eb735f, 266db08c — settings
// subpage split invented fictional /share/feedback-log +
// /share/report-bug URLs instead of /export/*). The probe
// catches that bug class at editor time.
//
// Usage:
//   node scripts/lint-button-actions-resolve.mjs        # report
//   node scripts/lint-button-actions-resolve.mjs --strict  # exit 1 on FAIL

import { strict as assert } from 'node:assert';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const TEMPL_DIR = join(ROOT, 'internal/templates');
const ROUTES_FILE = join(ROOT, 'internal/appshell/routes.go');
const ROUTEBUILDER_FILE = join(ROOT, 'internal/routebuilder/routebuilder.go');

const STRICT = process.argv.includes('--strict');

const HANDLER_ALLOWLIST = new Set([
  '/debug/client-logs',
  '/boot-theme.js',
  '/boot-config.js',
  '/wails/runtime.js',
  '/wails/ipc.js',
  '/recovery',
  '/version',
  '/app.js',
  '/app.css',
]);

const EXTERNAL_PREFIXES = ['http://', 'https://', 'mailto:', 'tel:', '#'];

// attrs that carry an invoker URL — match the templ
// and the rendered HTML shapes. Captures three shapes:
//
//   1. Literal URL in a Go-string-quoted attribute:
//        action="/soldiers/42"
//        data-action="/foo"
//        hx-get="/bar"
//   2. Templ SafeURL-wrapped invoker (whitelisted by the
//      routebuilder call inside, no need to lint):
//        action={ templ.SafeURL(routebuilder.X(...)) }
//   3. The fmt.Sprintf call-site shape used by data-action
//        data-action={ fmt.Sprintf("/soldiers/%d/delete", s.ID) }
//      This shape IS a lint target — the literal "/soldiers"
//      prefix must match a registered route.
const INVOKER_REGEX = /\b(action|data-action|hx-get|hx-post|hx-put|hx-patch|hx-delete)\s*=\s*(?:"([^"]+)"|\{[^}]*(?:SafeURL[^}]*|fmt\.Sprintf\(\s*"([^"]+)"|Sprintf\(\s*"([^"]+)")[^}]*\})/g;


function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    const st = statSync(full);
    if (st.isDirectory()) {
      out.push(...walk(full));
    } else if (entry.endsWith('.templ')) {
      out.push(full);
    }
  }
  return out;
}

// Pull the registered routes from routes.go by reading
// every `r.Get("...", ...)`, `r.Post("...", ...)`, etc.
// call site. We also pull `Route(...)` patterns from the
// route registry if any.
function registeredRoutes() {
  const src = readFileSync(ROUTES_FILE, 'utf8');
  const routes = new Set();
  // Top-level handler registrations: r.Get, r.Post, etc.
  const re = /\br\.(Get|Post|Put|Patch|Delete|Head|Route)\s*\(\s*"([^"]+)"/g;
  for (;;) {
    const m = re.exec(src);
    if (!m) break;
    routes.add(m[2]);
  }
  // Sub-routes inside chi Router groups via `r.Route("/prefix", func(r chi.Router) { ... })`.
  // Walk the full file once and track the open `r.Route("/.../{wildcard}", ...)`
  // envelope, then attribute every inner registration to the
  // joined path.
  const lines = src.split('\n');
  const routeStack = []; // stack of prefix strings
  let inRouteGroup = false;
  let groupDepth = 0;
  for (const line of lines) {
    const trimmed = line.trim();
    // Track `r.Route(".../prefix", func(r chi.Router) {`.
    const routeMatch = trimmed.match(/^\s*r\.Route\(\s*"([^"]+)"\s*,\s*func\s*\(\s*\w+\s+chi\.Router\s*\)\s*\{/);
    if (routeMatch) {
      routeStack.push(routeMatch[1]);
      // Walk back through earlier lines on the same source line
      // to find the indentation depth — we use a heuristic on
      // the leading-whitespace count of the matched line.
      const indentMatch = line.match(/^(\s*)/);
      if (indentMatch) {
        routeStack.__depth = indentMatch[1].length;
      }
    }
    // Inner handler registrations: r.Get / r.Post etc. that
    // belong to a route group inherit the prefix.
    const innerMatch = trimmed.match(/\br\.(Get|Post|Put|Patch|Delete|Head|Route)\s*\(\s*"([^"]+)"/);
    if (innerMatch && routeStack.length > 0) {
      for (const prefix of routeStack) {
        routes.add(prefix.replace(/\/$/, '') + innerMatch[2]);
      }
    }
  }
  return routes;
}

// Pull the canonical routebuilder function names so we
// can whitelist templ.SafeURL(routebuilder.X(...)) call
// sites without forcing every form action to expand the
// URL.
function routebuilderFunctionNames() {
  const src = readFileSync(ROUTEBUILDER_FILE, 'utf8');
  const names = new Set();
  const re = /^func\s+([A-Z][A-Za-z0-9_]*)\s*\(/gm;
  for (;;) {
    const m = re.exec(src);
    if (!m) break;
    names.add(m[1]);
  }
  return names;
}

function urlLooksExternal(u) {
  return EXTERNAL_PREFIXES.some((p) => u.startsWith(p));
}

function isAllowlisted(u) {
  if (HANDLER_ALLOWLIST.has(u)) return true;
  for (const p of EXTERNAL_PREFIXES) {
    if (u.startsWith(p)) return true;
  }
  // /boot-*, /wails/*, /_lib/* etc. are dev-only carve-outs.
  if (/^\/(?:boot-|wails\/|_lib\/)/.test(u)) return true;
  // /assets/* paths used by templ.Static().
  if (u.startsWith('/assets/')) return true;
  return false;
}

// normalizeRoute strips Go chi placeholders to a canonical
// shape so route templates from chi ({id:[0-9]+}, *, etc.)
// compare cleanly against probe URLs. The probe replaces
// %d / %s with the same canonical token.
function normalizeRoute(r) {
  return r
    .replace(/\{[^}]+\}/g, ':p') // chi-style {id:[0-9]+} etc.
    .replace(/\*/g, ':p')        // chi * (catch-all)
    .replace(/\?.*$/, '');       // query strings
}

function classify(url, registered, builderNames) {
  if (urlLooksExternal(url)) return 'external';
  if (isAllowlisted(url)) return 'allowlisted';
  // fmt.Sprintf template strings (e.g. /soldiers/%d/delete)
  // can't be matched exactly; instead substitute the %d
  // with a placeholder and then compare.
  const probe = normalizeRoute(url.replace(/%[sdif]/g, ':p'));
  if (registered.has(probe)) return 'registered';
  // Already-normalized registered set (built once).
  const normalizedRegistered = classify.__reg || (classify.__reg = (() => {
    const set = new Set();
    for (const r of registered) set.add(normalizeRoute(r));
    return set;
  })());
  if (normalizedRegistered.has(probe)) return 'registered';
  // Check the registered routes for path-parameter matches
  // (handles cases where the probe still has literal segments).
  const pathSegments = probe.split('/').filter(Boolean);
  for (const route of normalizedRegistered) {
    const routeSegments = route.split('/').filter(Boolean);
    if (routeSegments.length !== pathSegments.length) continue;
    let match = true;
    for (let i = 0; i < routeSegments.length; i++) {
      const rs = routeSegments[i];
      const ps = pathSegments[i];
      if (rs.startsWith(':')) continue;
      if (rs !== ps) {
        match = false;
        break;
      }
    }
    if (match) return 'registered';
  }
  return 'unregistered';
}

let pass = 0;
let fail = 0;
const failures = [];

const templFiles = walk(TEMPL_DIR);
const registered = registeredRoutes();
const builderNames = routebuilderFunctionNames();

for (const file of templFiles) {
  const src = readFileSync(file, 'utf8');
  const relFile = file.replace(ROOT + '\\', '').replace(ROOT + '/', '');
  for (const m of src.matchAll(INVOKER_REGEX)) {
    const line = src.slice(0, m.index).split('\n').length;
    const attr = m[1];
    const url = m[2] || m[3] || m[4];
    // Group 2 — literal Go-quoted URL (the form we want to lint).
    // Groups 3 + 4 — fmt.Sprintf / Sprintf literal template strings.
    // If both groups 2-4 are empty, the match caught a SafeURL
    // wrap (routebuilder whitelisted, skip).
    if (!url) continue;
    const status = classify(url, registered, builderNames);
    const name = `${relFile}:${line} ${attr}=${url}`;
    if (status === 'registered' || status === 'external' || status === 'allowlisted') {
      pass++;
    } else {
      fail++;
      const detail =
        `invoker resolves to ${url} which is not registered in routes.go. ` +
        `Add the route or use templ.SafeURL(routebuilder.X(...)) — ` +
        `see issue #687.`;
      console.log(`  ✗ ${name}`);
      console.log(`    ${detail}`);
      failures.push({ file: relFile, line, attr, url, detail });
    }
  }
}

console.log(`\n${pass} invoker(s) passed, ${fail} unregistered`);
if (STRICT && fail > 0) {
  console.error(`\nlint-button-actions-resolve: ${fail} invoker(s) resolve to unregistered URLs`);
  process.exit(1);
}
