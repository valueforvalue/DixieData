// audit/smoke_static_archive_microcopy.mjs
//
// Issue #581 — Static Archive microcopy gate.
//
// Static Archive UI is one Go raw-string template containing HTML and
// JavaScript-rendered HTML. This probe checks only human-facing output
// markers in that template. It does not apply .templ rules to CSS, JS
// plumbing, JSON keys, or researcher-authored archive content.
//
// Usage:
//   node audit/smoke_static_archive_microcopy.mjs
//   node audit/smoke_static_archive_microcopy.mjs --strict
//
// STATIC_ARCHIVE_SOURCE exists for synthetic probe fixtures.

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const SOURCE = process.env.STATIC_ARCHIVE_SOURCE || join(ROOT, 'internal', 'archive', 'static_archive.go');
const STRICT = process.argv.includes('--strict');

const source = readFileSync(SOURCE, 'utf8');
const marker = 'const staticArchiveIndexHTML = `';
const start = source.indexOf(marker);
if (start < 0) {
  console.error(`fatal: static archive template not found in ${SOURCE}`);
  process.exit(2);
}
const bodyStart = start + marker.length;
const bodyEnd = source.indexOf('`', bodyStart);
if (bodyEnd < 0) {
  console.error(`fatal: static archive template is not closed in ${SOURCE}`);
  process.exit(2);
}
const template = source.slice(bodyStart, bodyEnd);

const findings = [];
const required = [
  ['hero description', 'Read-only Static Archive export.'],
  ['Person Record detail marker', 'Person Record View'],
  ['Source Records heading', 'Source Records</h4>'],
  ['Person Record Type label', 'Person Record Type'],
  ['Linked Soldier label', 'Linked Soldier'],
  ['Person Record action', 'View Person Record'],
  ['Event Record action', 'View Event Record'],
  ['Article action', 'View Article'],
  ['Confederate Home Status label', 'Confederate Home Status'],
  ['Linked Persons label', 'Linked Persons'],
  ['print instruction', 'Open printable report in new tab, then use browser Print → PDF.'],
];

for (const [name, text] of required) {
  if (!template.includes(text)) {
    findings.push({ rule: 'required', name, text, hint: `required copy missing: ${name}` });
  }
}

const forbidden = [
  ['verbose hero description', 'Browse this standalone DixieData archive as a read-only mirror'],
  ['duplicated report instructions', 'Open the printable report in a new tab. Use your browser\'s Print'],
  ['duplicated row instructions', 'Click any row for the full detail view'],
  ['duplicated insight instructions', 'Click any entry to filter Person Records'],
  ['old detail marker', />\s*Record View\s*</],
  ['old action label', />\s*View More\s*</],
  ['old related-record action', />\s*Open Related Record\s*</],
  ['old source heading', />\s*Records\s*<\/h4>/],
  ['bare source-record fallback', "recordType || 'Record'"],
  ['bare source-record print fallback', "r.recordType || 'Record'"],
  ['old linked-soldier label', 'Linked Soldier Record'],
  ['old insight label', 'Linked people'],
  ['old home-status casing', 'Confederate Home status'],
];

for (const [name, pattern] of forbidden) {
  const matches = typeof pattern === 'string' ? template.includes(pattern) : pattern.test(template);
  if (matches) {
    findings.push({ rule: 'forbidden', name, text: String(pattern), hint: `remove or replace ${name}` });
  }
}

function count(text) {
  return template.split(text).length - 1;
}

// One visible report instruction is enough. The title is an accessible
// label for the same action, not a second explanation.
const printInstruction = 'Open printable report in new tab, then use browser Print → PDF.';
if (count(printInstruction) !== 1) {
  findings.push({
    rule: 'duplicate',
    name: 'print instruction count',
    text: printInstruction,
    hint: 'keep one concise report instruction',
  });
}

const byRule = { required: 0, forbidden: 0, duplicate: 0 };
for (const finding of findings) byRule[finding.rule]++;

console.log(`File scanned: ${SOURCE}`);
console.log(`  Required copy findings: ${byRule.required}`);
console.log(`  Forbidden/verbose copy findings: ${byRule.forbidden}`);
console.log(`  Duplicate copy findings: ${byRule.duplicate}`);
console.log(`Total findings: ${findings.length}`);

if (findings.length) {
  console.log('');
  console.log('=== findings ===');
  for (const finding of findings) {
    console.log(`  ${finding.rule}  ${finding.name}`);
    console.log(`    ${finding.text}`);
    console.log(`    → ${finding.hint}`);
  }
}

if (STRICT && findings.length) {
  console.log('');
  console.log('--strict: treating as a CI failure.');
  process.exit(1);
}

console.log(findings.length ? '\nStatic Archive microcopy sweep: informational findings.' : '\nStatic Archive microcopy sweep: clean.');
process.exit(0);
