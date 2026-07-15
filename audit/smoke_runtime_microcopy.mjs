// audit/smoke_runtime_microcopy.mjs
//
// Issue #581 — runtime + server + startup + CLI microcopy gate
// (slice 4, thin-recon audit-only).
//
// Slices 1–3 cover Static Archive HTML, Typst PDF, and iCalendar.
// This probe covers the runtime copy those three did not:
//   - frontend/app.js top-traffic showToast() call sites
//     (action-form verb-led toasts, slice-1 invariants preserved);
//   - the loading-screen placeholder in internal/appshell/app.go
//     (one sentence repeated by the slice-1 R5 rule);
//   - the CLI help text in main.go::cliHelpText.
//
// The probe pins the canonical strings verbatim. It is
// audit-only: slice 4 ships GREEN-on-HEAD, no microcopy edits.
// A future contributor who wants to drift any pinned string
// must update the catalog + cite #581 in their commit message.
//
// Skipped (per slice-3 precedent): JSON keys, route names, CSS,
// generated Tailwind output, service-worker plumbing, error
// chains not shown to researchers, internal-handler error
// wrappers that don't surface to the researcher via toasts or
// pages.
//
// Usage:
//   node audit/smoke_runtime_microcopy.mjs
//   node audit/smoke_runtime_microcopy.mjs --strict
//
// Exit codes:
//   0  clean OR non-strict with findings printed
//   1  strict and at least one finding
//   2  fatal (couldn't read RUNTIME_SOURCE)
//
// RUNTIME_SOURCE exists for synthetic probe fixtures (mirrors the
// STATIC_ARCHIVE_SOURCE / PDF_SOURCE / ICS_SOURCE conventions
// from slices 1+2+3). When unset, the probe concatenates the
// four canonical files.

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const STRICT = process.argv.includes('--strict');

const FILES = {
  'app.js': join(ROOT, 'frontend', 'app.js'),
  'app.go (startup placeholder)': join(ROOT, 'internal', 'appshell', 'app.go'),
  'cliHelpText (main.go)': join(ROOT, 'main.go'),
};

// Required strings — each is pinned verbatim so any future
// drift breaks the gate.
const REQUIRED = [
  // top-traffic toasts in app.js (action-form verb-led, ≤80 chars each)
  ['toast: Saved local draft restored.', 'frontend/app.js', 'Saved local draft restored.'],
  ['toast: Path copied.', 'frontend/app.js', 'Path copied.'],
  ['toast: No path to copy.', 'frontend/app.js', 'No path to copy.'],
  ['toast: Could not load print options.', 'frontend/app.js', 'Could not load print options.'],
  ['toast: Browse refresh failed.', 'frontend/app.js', 'Browse refresh failed.'],
  ['toast: Choose exactly two records to compare.', 'frontend/app.js', 'Choose exactly two records to compare.'],
  ['toast: Nothing to copy.', 'frontend/app.js', 'Nothing to copy.'],
  ['toast: Clipboard helper unavailable.', 'frontend/app.js', 'Clipboard helper unavailable.'],
  ['toast: Preview content was not available.', 'frontend/app.js', 'Preview content was not available.'],

  // startup placeholder (app.go)
  ['startup: title', 'app.go', '<title>Loading DixieData...</title>'],
  ['startup: body heading', 'app.go', 'text-2xl font-semibold text-[var(--theme-text-primary)]">Loading DixieData...</p>'],
  ['startup: status body', 'app.go', 'The local archive is still starting up. This screen will refresh automatically.'],

  // CLI help (main.go)
  ['cli: DixieData CLI opener', 'cliHelpText (main.go)', 'DixieData CLI \u2014 headless archive operations'],
  ['cli: usage line', 'cliHelpText (main.go)', 'dixiedata <subcommand> [flags]'],
  ['cli: doc reference', 'cliHelpText (main.go)', 'See docs/agents/cli-plan.md for the full roadmap.'],
];

// Forbidden — drift signposts. Each tuple: [name, file, regex,
// hint].
const FORBIDDEN = [
  [
    'toast: status-form empty copy in app.js',
    'frontend/app.js',
    /showToast\(\s*"No records yet\."/,
    'toasts must be action-form ("Saved X", "Could not load Y"); never status-form ("No records yet.")',
  ],
  [
    'startup: verbose second paragraph',
    'app.go',
    /Loading DixieData\.\.\.[\s\S]{0,400}still starting up[\s\S]{0,400}refreshes? automatically/,
    'the startup second sentence must remain in its current concise form; the placeholder auto-refreshes regardless',
  ],
  [
    'cli: redundant subcommand list',
    'cliHelpText (main.go)',
    /Subcommands:\s*\n\s*\n/,
    'CLI help verb descriptions must remain on consecutive lines (no blank lines between verb entries)',
  ],
];

let combined = '';
const fileContents = {};
for (const [label, path] of Object.entries(FILES)) {
  let contents;
  try {
    contents = readFileSync(path, 'utf8');
  } catch (err) {
    console.error(`fatal: cannot read ${path}: ${err.message}`);
    process.exit(2);
  }
  fileContents[label] = contents;
  combined += `\n// ===== ${label} =====\n` + contents;
}

// Allow override: RUNTIME_SOURCE = a single file path. Useful
// for synthetic fixtures.
import { existsSync } from 'node:fs';
if (process.env.RUNTIME_SOURCE && existsSync(process.env.RUNTIME_SOURCE)) {
  combined = readFileSync(process.env.RUNTIME_SOURCE, 'utf8');
}

// When RUNTIME_SOURCE is set, the test fixture replaces the
// real repo file contents for both required and forbidden
// checks so a synthetic fixture can validate the rules
// end-to-end.
const syntheticContent = process.env.RUNTIME_SOURCE
  ? combined
  : null;

const findings = [];
for (const [name, _label, substring] of REQUIRED) {
  const haystack = syntheticContent ?? combined;
  if (!haystack.includes(substring)) {
    findings.push({
      rule: 'required',
      name,
      text: substring,
      hint: `required copy missing: ${name}`,
    });
  }
}
for (const [name, label, pattern, hint] of FORBIDDEN) {
  const source = syntheticContent
    ? combined
    : fileContents[label];
  if (source && pattern.test(source)) {
    findings.push({ rule: 'forbidden', name, text: String(pattern), hint });
  }
}

const byRule = { required: 0, forbidden: 0 };
for (const finding of findings) byRule[finding.rule]++;

console.log(`Sources scanned: ${Object.keys(FILES).length}`);
console.log(`  Required copy findings: ${byRule.required}`);
console.log(`  Forbidden/verbose copy findings: ${byRule.forbidden}`);
console.log(`Total findings: ${findings.length}`);

if (findings.length) {
  console.log('');
  console.log('=== findings ===');
  for (const finding of findings) {
    console.log(`  ${finding.rule.padEnd(10)}  ${finding.name}`);
    console.log(`              ${finding.text}`);
    console.log(`              → ${finding.hint}`);
  }
}

if (STRICT && findings.length) {
  console.log('');
  console.log('--strict: treating as a CI failure.');
  process.exit(1);
}

console.log(findings.length ? '\nRuntime microcopy sweep: informational findings.' : '\nRuntime microcopy sweep: clean.');
process.exit(0);
