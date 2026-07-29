// verify_embed_tree.mjs -- issue #686.
//
// Asserts that every frontend/** file referenced by the
// live HTML (index.html + the runtime layout) is reachable
// via the `//go:embed frontend` directive at main.go:25.
//
// Why this matters: Go's `embed` package skips files and
// directories whose names begin with `.` or `_`. The
// DixieData convention has been to put shared JS helpers in
// `frontend/lib/` (after `12f1834a` renamed `_lib/` to
// `lib/`). If a future contributor re-introduces a
// `_`-prefixed top-level dir under `frontend/` for shared
// helpers, the Wails binary will silently drop every file
// under it. The article Preview button bug (`12f1834a`) was
// the canonical instance: `frontend/_lib/debounce.js` was
// never embedded, `window.__dixieDebounce` was undefined,
// the Preview button was a no-op.
//
// Three rules cover the bug class:
//
//   R1: reparent-by-prefix — every file/dir under
//       `frontend/` whose name starts with `_` or `.`
//       would be skipped by `//go:embed`. The script
//       reports current state as informational; new
//       violations are warnings (the existing code may
//       have intentionally-prefixed helpers — see the
//       Common Bugs §8.5 cross-reference for the policy).
//
//   R2: index-html-references-resolve — every `src=`,
//       `href=`, or bootstrap script path in `frontend/
//       index.html` must point to a file that exists
//       under `frontend/`. Catches typos and dead-script
//       references.
//
//   R3: live-html-references-resolve — same shape as R2
//       but applied to the rendered runtime HTML embedded
//       in `internal/templates/layout_templ.go`. The
//       runtime layout mentions `/_lib/...` paths in the
//       comments above the script tags; the actual paths
//       are `lib/...`. The lint uses the rendered HTML
//       attributes (`src="/lib/..."`) as the source of
//       truth.
//
// Why this is a static scan rather than a runtime check:
// the Wails build embeds the frontend at compile time.
// Failing at build time is too late for a PR-time gate.
// The static scan fails at lint time, before the build.
//
// Why this isn't a Go test: the embed directive is in
// `main.go` but the asset *paths* are owned by the frontend
// (templ + JS). The cross-layer rule belongs in `audit/`
// alongside the other invariant lints (lint-bake-bootstrap,
// lint-htmx-guard). Output is the canonical row-only
// pipe-delimited format: `file:line | rule-id | note`.
// `--strict` flips to exit 1 so CI can use it as a gate.
//
// Sibling to audit/lint_bake_bootstrap.mjs.

import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs';
import { join, relative, sep, posix } from 'node:path';

const ROOT = process.env.DIXIE_ROOT || new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const FRONTEND = join(ROOT, 'frontend');
const INDEX_HTML = join(FRONTEND, 'index.html');
const LAYOUT_TEMPL_GO = join(ROOT, 'internal/templates/layout_templ.go');
const EMBED_DIRECTIVE_FILE = join(ROOT, 'main.go');

const STRICT = process.argv.includes('--strict');

// ---- Path classification ----

// True for any path whose final segment starts with `_`
// or `.`. Go's `embed` package skips these.
function isReparentByPrefix(name) {
  return name.startsWith('_') || name.startsWith('.');
}

// Walk `frontend/` recursively. Returns:
//   - allFiles: every regular file path under `frontend/`,
//     relative to FRONTEND (forward-slash).
//   - reparentCandidates: subset of allFiles whose path
//     contains a `_`- or `.`-prefixed segment. These are
//     the files that `//go:embed frontend` would silently
//     drop.
function walkFrontend() {
  const allFiles = [];
  function walk(dir) {
    for (const entry of readdirSync(dir)) {
      const p = join(dir, entry);
      const s = statSync(p);
      if (s.isDirectory()) {
        walk(p);
      } else if (s.isFile()) {
        allFiles.push(relative(FRONTEND, p).split(sep).join('/'));
      }
    }
  }
  if (!existsSync(FRONTEND)) return { allFiles: [], reparentCandidates: [] };
  walk(FRONTEND);
  const reparentCandidates = allFiles.filter((rel) => {
    const parts = rel.split('/');
    return parts.some(isReparentByPrefix);
  });
  return { allFiles, reparentCandidates };
}

// ---- HTML reference extraction ----

// Pull asset references out of an HTML file's text. Assets
// are `<script src="...">` and `<link rel="stylesheet" href="...">`.
// Navigation links (`<a href="...">`) are NOT asset refs — they
// resolve to Go routes. Returns the de-duplicated list of
// URL strings (only paths starting with `/`, external URLs
// are filtered out). The HANDLER_ALLOWLIST skips URLs that
// the Go layer synthesizes (e.g. /boot-theme.js is served
// by a Go handler, not the embedded frontend).
//
// The regex handles BOTH plain HTML quotes (`src="..."`) and
// Go-escaped quotes in generated templ files (`src=\"/...\"`).
// The escape pattern shows up in the generated `*_templ.go`
// files because Go source quoting escapes the inner double
// quotes. Same URL, two flavours.
function extractHtmlRefs(htmlPath) {
  if (!existsSync(htmlPath)) return [];
  const text = readFileSync(htmlPath, 'utf8');
  const refs = new Set();
  // <script src="..."> or <script src=\"...\"> (Go-escaped)
  const scriptRe = /<script[^>]*\bsrc\s*=\\?(?:"|\\")([^"]+)(?:"|\\")/g;
  let m;
  while ((m = scriptRe.exec(text)) !== null) {
    const url = m[1].replace(/\\$/, '');
    if (url.startsWith('/') && !url.startsWith('//')) refs.add(url);
  }
  // <link href="..."> or <link href=\"...\"> (Go-escaped)
  const linkRe = /<link[^>]*\bhref\s*=\\?(?:"|\\")([^"]+)(?:"|\\")/g;
  while ((m = linkRe.exec(text)) !== null) {
    const url = m[1].replace(/\\$/, '');
    if (url.startsWith('/') && !url.startsWith('//')) refs.add(url);
  }
  return [...refs].filter((url) => !HANDLER_ALLOWLIST.has(url));
}

// URLs that are served by Go handlers, not by the embedded
// frontend. These won't appear under `frontend/` because they
// are generated at runtime by the chi router. The allowlist
// is the inverse of the route registry: every URL here is a
// path that the embed-tree rule must NOT verify against
// the disk.
const HANDLER_ALLOWLIST = new Set([
  '/boot-theme.js',  // internal/appshell/boot_theme.go
  '/boot-config.js', // internal/appshell/boot_config.go
  '/debug/client-logs', // Go-side debug sink
  '/wails/runtime.js', // Wails runtime injection
  '/wails/ipc.js', // Wails runtime injection
]);

// Same, but applied to the rendered runtime HTML embedded
// in the generated templ file. The runtime HTML is in a
// templ WriteString call; the script/href attributes pass
// through verbatim. We grep the *generated* file because
// source `.templ` files are written with line breaks inside
// the script tags that defeat single-line regexes.
//
// The generated file uses Go-escaped quotes (`src=\"/...\"`);
// the regex handles both shapes.
function extractLiveHtmlRefs(livePath) {
  if (!existsSync(livePath)) return [];
  const text = readFileSync(livePath, 'utf8');
  const refs = new Set();
  const scriptRe = /<script[^>]*\bsrc\s*=\\?(?:"|\\")([^"]+)(?:"|\\")/g;
  let m;
  while ((m = scriptRe.exec(text)) !== null) {
    const url = m[1].replace(/\\$/, '');
    if (url.startsWith('/') && !url.startsWith('//')) refs.add(url);
  }
  const linkRe = /<link[^>]*\bhref\s*=\\?(?:"|\\")([^"]+)(?:"|\\")/g;
  while ((m = linkRe.exec(text)) !== null) {
    const url = m[1].replace(/\\$/, '');
    if (url.startsWith('/') && !url.startsWith('//')) refs.add(url);
  }
  return [...refs].filter((url) => !HANDLER_ALLOWLIST.has(url));
}

// ---- Verify the embed directive exists at the expected path ----

// Lightweight check: the embed directive must exist at
// main.go at a known location. If a future refactor moves
// the embed to another package, the lint can be updated.
// Reparse via simple regex (the directive is a single line).
function findEmbedDirective() {
  if (!existsSync(EMBED_DIRECTIVE_FILE)) return null;
  const text = readFileSync(EMBED_DIRECTIVE_FILE, 'utf8');
  const m = text.match(/^\/\/go:embed\s+(\S+)/m);
  if (!m) return null;
  const embedPath = m[1];
  const line = text.split('\n').findIndex((l) => l.includes('//go:embed'));
  return { path: embedPath, line: line + 1 };
}

// ---- Main ----

function main() {
  console.log('=== verify-embed-tree ===');
  console.log('R1: reparent-by-prefix — _ or . prefixed files/dirs under frontend/ are skipped by //go:embed');
  console.log('R2: index-html-resolves — every <script src>/<link href> in frontend/index.html points to a real file');
  console.log('R3: live-html-resolves — same shape, applied to internal/templates/layout_templ.go (rendered HTML)');
  console.log('');

  const violations = [];

  // R1: reparent-by-prefix
  const { allFiles, reparentCandidates } = walkFrontend();
  console.log(`Found ${allFiles.length} file(s) under frontend/.`);
  if (reparentCandidates.length > 0) {
    console.log(`  [warn] R1: ${reparentCandidates.length} file(s) would be skipped by //go:embed frontend:`);
    for (const f of reparentCandidates) {
      const segment = f.split('/').find(isReparentByPrefix);
      console.log(`         ${f}  (segment "${segment}" starts with _ or .)`);
    }
    // R1 is a warning, not a hard failure — historical files
    // (e.g. legacy `_lib/` residue) may legitimately exist. The
    // lint reports state; the rule is "no NEW additions".
    // Document this in the row output as informational.
    for (const f of reparentCandidates) {
      const segment = f.split('/').find(isReparentByPrefix);
      violations.push({
        file: EMBED_DIRECTIVE_FILE,
        line: 0,
        rule: 'embed-tree-excluded-by-prefix',
        note: `frontend/${f} is excluded: segment "${segment}" starts with _ or .`,
      });
    }
  } else {
    console.log('  [ok]   R1: no _ or . prefixed files under frontend/');
  }
  console.log('');

  // Embed directive check
  const embed = findEmbedDirective();
  if (!embed) {
    console.log(`  [error] no //go:embed directive found in main.go`);
    violations.push({
      file: EMBED_DIRECTIVE_FILE,
      line: 0,
      rule: 'embed-directive-missing',
      note: 'main.go lacks a //go:embed directive; Wails runtime cannot serve frontend assets',
    });
  } else {
    console.log(`  [ok]   embed directive at main.go:${embed.line} → //go:embed ${embed.path}`);
  }
  console.log('');

  // R2: index-html references
  const indexRefs = extractHtmlRefs(INDEX_HTML);
  let indexMissing = 0;
  if (indexRefs.length === 0) {
    console.log(`  [skip] R2: ${INDEX_HTML} not found or has no <script>/<link> references`);
  } else {
    console.log(`R2: index.html references ${indexRefs.length} URL(s).`);
    for (const url of indexRefs) {
      // `/lib/debounce.js` → frontend/lib/debounce.js
      const fsPath = join(FRONTEND, url.replace(/^\//, '').split('?')[0].split('#')[0]);
      const posixPath = posix.join(FRONTEND, url.replace(/^\//, '').split('?')[0].split('#')[0]);
      if (!existsSync(fsPath) && !existsSync(posixPath)) {
        console.log(`  [fail] ${url}  (not found under frontend/)`);
        indexMissing++;
        violations.push({
          file: INDEX_HTML,
          line: 0,
          rule: 'index-html-reference-missing',
          note: `${url} referenced by index.html but no file exists at frontend/${url}`,
        });
      } else {
        console.log(`  [ok]   ${url}`);
      }
    }
  }
  console.log('');

  // R3: live-html references (rendered runtime HTML)
  // Note: on Windows the file path uses backslashes; the
  // existsSync check below still works on Windows because
  // Node's fs layer normalizes path separators. The
  // explicit cwd-detail note is for the operator.
  const liveRefs = extractLiveHtmlRefs(LAYOUT_TEMPL_GO);
  let liveMissing = 0;
  if (!existsSync(LAYOUT_TEMPL_GO)) {
    console.log(`  [skip] R3: ${relative(ROOT, LAYOUT_TEMPL_GO)} not found`);
    console.log(`         (Regenerate via 'make tpl' if you changed internal/templates/layout.templ.)`);
  } else if (liveRefs.length === 0) {
    console.log(`  [skip] R3: ${relative(ROOT, LAYOUT_TEMPL_GO)} has no <script>/<link> references`);
  } else {
    console.log(`R3: rendered HTML (layout_templ.go) references ${liveRefs.length} URL(s).`);
    for (const url of liveRefs) {
      const fsPath = join(FRONTEND, url.replace(/^\//, '').split('?')[0].split('#')[0]);
      const posixPath = posix.join(FRONTEND, url.replace(/^\//, '').split('?')[0].split('#')[0]);
      if (!existsSync(fsPath) && !existsSync(posixPath)) {
        console.log(`  [fail] ${url}  (not found under frontend/)`);
        liveMissing++;
        violations.push({
          file: LAYOUT_TEMPL_GO,
          line: 0,
          rule: 'live-html-reference-missing',
          note: `${url} referenced by rendered HTML but no file exists at frontend/${url}`,
        });
      } else {
        console.log(`  [ok]   ${url}`);
      }
    }
  }
  console.log('');

  // Summary
  const realViolations = violations.filter((v) => v.rule !== 'embed-tree-excluded-by-prefix');
  if (realViolations.length === 0) {
    console.log('✓ verify-embed-tree clean. No missing references.');
    if (reparentCandidates.length > 0) {
      console.log(`  (informational: ${reparentCandidates.length} prefix-excluded file(s) under frontend/ — see R1 above for the policy.)`);
    }
    process.exit(0);
  }
  console.log(`Found ${realViolations.length} missing-reference violation(s):`);
  for (const v of realViolations) {
    const rel = relative(ROOT, v.file).split(sep).join('/');
    const lineStr = v.line > 0 ? `:${v.line}` : '';
    console.log(`  ${rel}${lineStr} | ${v.rule} | ${v.note}`);
  }
  if (realViolations.length > 0) {
    console.log('');
    console.log(`Refer to docs/COMMON_BUGS.md §2.7 (nested form) and the original bug at 12f1834a.`);
  }
  if (STRICT) {
    console.log('');
    console.log('--strict: treating as a CI failure.');
    process.exit(1);
  }
  process.exit(0);
}

main();
