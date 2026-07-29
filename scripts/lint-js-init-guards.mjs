// lint-js-init-guards.mjs (issue #685)
//
// Source-scan probe that asserts every JS initializer called
// from initializeDynamicContent() (frontend/app.js) has the
// per-feature `__<thing>Wired` / `__<thing>Bound` idempotency
// guard inside its first ~40 lines.
//
// The pattern was established by fixes 87645011 and c0d89681
// (Preview button + cheatsheet copy buttons — broken after
// back/forward navigation because the init was wired in the
// cold-start block only). The fix shape: every initializer
// gets a sentinel check at the top so htmx:load swaps don't
// double-install the click handler.
//
// Without this audit gate, a future initializer added without
// the guard will regress to the broken-after-swap shape.
//
// Usage:
//   node scripts/lint-js-init-guards.mjs           # report
//   node scripts/lint-js-init-guards.mjs --strict  # exit 1 on any FAIL
//
// The probe is informational by default (matches the
// audit-harness convention in audit/_lib/cleanup.mjs) and
// flips to strict once the codebase has zero offenders.

import { strict as assert } from 'node:assert';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const APP_JS = join(ROOT, 'frontend/app.js');
const FRONTEND_DIR = join(ROOT, 'frontend');

const STRICT = process.argv.includes('--strict');

// Initializers that initializeDynamicContent() dispatches to.
// Each entry: { name, file, fnDecl }.
// Sourced from grep against frontend/app.js line 4265..4300
// (the dispatch block). Kept literal here so the probe
// catches a future rename of the function header.
const INITIALIZERS = [
  { name: 'initializeTabs', file: 'frontend/app.js', fnDecl: 'function initializeTabs' },
  { name: 'initializeEntryTypeForms', file: 'frontend/app.js', fnDecl: 'function initializeEntryTypeForms' },
  { name: 'initializeBrowseView', file: 'frontend/app.js', fnDecl: 'function initializeBrowseView' },
  { name: 'initializeCopyPathButtons', file: 'frontend/app.js', fnDecl: 'function initializeCopyPathButtons' },
  { name: 'initializeInventoryMetricsChart', file: 'frontend/app.js', fnDecl: 'function initializeInventoryMetricsChart' },
  { name: 'initializePersonRecordPicker', file: 'frontend/app.js', fnDecl: 'function initializePersonRecordPicker' },
  { name: 'initializeMarkdownCheatsheet', file: 'frontend/app.js', fnDecl: 'function initializeMarkdownCheatsheet' },
  { name: 'initializeEditorToolbar', file: 'frontend/app.js', fnDecl: 'function initializeEditorToolbar' },
  { name: 'initializeTableBuilder', file: 'frontend/app.js', fnDecl: 'function initializeTableBuilder' },
  { name: 'initializeImagePicker', file: 'frontend/app.js', fnDecl: 'function initializeImagePicker' },
  { name: 'initializeArticleImagePasteDrop', file: 'frontend/app.js', fnDecl: 'function initializeArticleImagePasteDrop' },
  { name: 'initializeImageUpload', file: 'frontend/app.js', fnDecl: 'function initializeImageUpload' },
  { name: 'initializeArticlePreview', file: 'frontend/app.js', fnDecl: 'function initializeArticlePreview' },
];

const GUARD_PATTERNS = [
  // Per-element sentinel: el.__<feature>Wired === true OR set
  // to a truthy value. Covers the canonical pattern from
  // initializeArticlePreview (modal.__articlePreviewWired === true
  // — issue #583) AND initializeInventoryMetricsChart (the
  // !.__Painted variant — issue #564).
  {
    label: 'per-element __<feature>Wired/Painted/Bound sentinel',
    re: /\.\s*__[a-zA-Z][a-zA-Z0-9_]*(Wired|Painted|Bound|Installed)\s*===?\s*(?:true|"1"|"true")/,
  },
  // Per-window sentinel: window.__<feature>Wired (the lock-in-once
  // install-once markers per docs/CODE_CHANGES.md note on the
  // pattern set by __articlePreviewWired).
  {
    label: 'per-window __<feature>Wired sentinel',
    re: /window\s*\.\s*__[a-zA-Z][a-zA-Z0-9_]*(Wired|Bound|Installed|Painted)\s*===?\s*true/,
  },
  // Per-button / per-element property-set guard used by the
  // cheatsheet copy/insert buttons: `if (button.__cheatsheetBound)`.
  {
    label: 'per-element property set guard',
    re: /if\s*\(\s*[A-Za-z_$][A-Za-z0-9_$.[\]]*\.__[a-zA-Z][a-zA-Z0-9_]*(Bound|Wired|Installed|Painted)\s*\)/,
  },
  // Truthy test of a __<feature>Wired property: `if (!wrapper.__...Painted)`.
  {
    label: 'truthy-test sentinel',
    re: /if\s*\(\s*!?[A-Za-z_$][A-Za-z0-9_$.[\]]*\.__[a-zA-Z][a-zA-Z0-9_]*(Wired|Painted|Bound|Installed)\s*\)/,
  },
  // Per-element dataset.<feature>Wired sentinel (used by
  // initializeArticlePreview's modal hook-up, the termDisclosure
  // dedupe, and the new initializeTabs/initializeBrowseView/
  // initializeEntryTypeForms guards). Matches both
  // `dataset.fooWired === "1"` and `dataset.fooWired = "1"`.
  {
    label: 'per-element dataset.<feature>Wired sentinel',
    re: /\.dataset\s*\.\s*[a-zA-Z][a-zA-Z0-9_]*(Wired|Bound|Painted|Installed)\s*(?:===?\s*["']1["']|=)/,
  },
];

let pass = 0;
let fail = 0;
const failures = [];

function report(name, ok, detail) {
  if (ok) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.log(`  ✗ ${name}`);
    console.log(`    ${detail}`);
    failures.push({ name, detail });
  }
}

const src = readFileSync(APP_JS, 'utf8');

for (const init of INITIALIZERS) {
  const fnIdx = src.indexOf(init.fnDecl + '(');
  if (fnIdx < 0) {
    report(init.name, false, `function header ${init.fnDecl} not found in ${init.file}`);
    continue;
  }
  // Capture the first 4000 chars of the function body
  // (matches the entry-form-test precedent; the body is
  // typically < 200 lines).
  const slice = src.slice(fnIdx, fnIdx + 4000);
  const matched = GUARD_PATTERNS.find((p) => p.re.test(slice));
  if (matched) {
    report(`${init.name} has ${matched.label}`, true);
  } else {
    report(
      init.name,
      false,
      `no __<feature>Wired/Bound/Installed/Painted sentinel in the first 4000 chars of ` +
        `${init.fnDecl}; future htmx:load swaps will double-install the click handler. ` +
        `Pattern set by __articlePreviewWired (commit 87645011). ` +
        `Add e.g. 'if (el.__${init.name.replace(/^initialize/, '').toLowerCase()}Wired) return; ` +
        `el.__${init.name.replace(/^initialize/, '').toLowerCase()}Wired = true;' near the top.`,
    );
  }
}

console.log(`\n${pass} passed, ${fail} failed`);

if (STRICT && fail > 0) {
  console.error(`\nlint-js-init-guards: ${fail} initializer(s) missing __<feature>Wired guard`);
  process.exit(1);
}
