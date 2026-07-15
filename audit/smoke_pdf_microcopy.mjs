// audit/smoke_pdf_microcopy.mjs
//
// Issue #581 — PDF (Typst) microcopy gate.
//
// The `.templ` probe in audit/smoke_microcopy.mjs walks Go-templ
// HTML. Typst is a different grammar; this probe walks
// templates/**/*.typ and asserts the format-aware rules agreed
// for slice 2 of #581:
//
//   - concise titles + single-line subtitles (no verbose body
//     under the analytics-summary title);
//   - no stacked weight:bold sub-headings immediately under a
//     parent section heading;
//   - no duplicated narration across the group_divider and the
//     inlined bulk_soldier variant;
//   - no ALL-CAPS eyebrows above self-explanatory single-field
//     groups;
//   - no status-form empty-state copy ("No X recorded yet.")
//     where the heading alone suffices;
//   - glossary-aligned canonical labels: Person Record, Source
//     Record, Display ID, Confederate Home Status (only in files
//     that actually render labels — delegated thin wrappers are
//     not flagged because their label lives in the helper).
//
// Skipped: templates/hello.typ (smoke target), templates/common/theme.typ
// (palette + page-params helpers, no chrome), and the templates/common/
// directory itself (helpers, no chrome of their own).
//
// Usage:
//   node audit/smoke_pdf_microcopy.mjs
//   node audit/smoke_pdf_microcopy.mjs --strict
//
// Exit codes:
//   0  clean OR non-strict with findings printed
//   1  strict and at least one finding
//   2  fatal (couldn't find templates/)
//
// PDF_SOURCE exists for synthetic probe fixtures (mirrors the
// STATIC_ARCHIVE_SOURCE convention from slice 1).

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { basename, join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const TEMPLATES = process.env.PDF_SOURCE || join(ROOT, 'templates');
const STRICT = process.argv.includes('--strict');

const SKIP_BASENAMES = new Set([
  'hello.typ',
  'theme.typ',
]);

const SKIP_DIRS = new Set([
  'common',
  'testdata',
  'node_modules',
]);

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue;
    const path = join(dir, entry);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      out.push(...walk(path));
    } else if (entry.endsWith('.typ')) {
      if (SKIP_BASENAMES.has(entry)) continue;
      out.push(path);
    }
  }
  return out.sort();
}

// Strip code expressions and inline comments so the duplication
// probe looks only at prose. Heading + body checks use the raw
// source. Lines that read as Typst code (assignments, attribute
// lookups, function calls, math operators) are excluded — they
// are markup, not prose, even when the bracket-tokens have been
// stripped.
function visibleText(source) {
  return source
    .split('\n')
    .map((line) => {
      const trimmed = line.trimStart();
      if (trimmed.startsWith('//')) return '';
      const commentIdx = line.indexOf('//');
      let core = commentIdx >= 0 ? line.slice(0, commentIdx) : line;
      core = core.replace(/#[A-Za-z{][^#\n]*/g, '');
      core = core.replace(/\[/g, '').replace(/\]/g, '');
      const out = core.replace(/\s+/g, ' ').trim();
      if (!out) return '';
      if (/[A-Za-z_][A-Za-z0-9_]*\s*=/.test(out)) return '';
      if (/\.at\(/.test(out)) return '';
      if (/^[a-z_][A-Za-z0-9_]*\s*\(/.test(out)) return '';
      if (/^let\s+/.test(out) || /^if\s+/.test(out) || /^for\s+/.test(out)) return '';
      if (/^import\s+/.test(out)) return '';
      if (/\bcalc\./.test(out)) return '';
      if (/^(line|grid|box|columns|align|v|pagebreak|h)\s*\(/.test(out)) return '';
      if (/(stroke|fill|size):/.test(out)) return '';
      if (/^[\(\),.;:\-]$/.test(out)) return '';
      if (/\.\.\.?$/.test(out)) return '';
      return out;
    })
    .filter((s) => s.length > 0);
}

// Per-template required copy. Each rule lists the file basenames it
// applies to. Forbidden rules apply globally. Thin wrapper files
// (per-record landscape/portrait delegators to common/record_card.typ)
// are NOT listed because their labels live in the helper; flagging
// the helpers is a helper-level fix, not a per-file requirement.
const REQUIRED_BY_BASENAME = [
  {
    basenames: ['analytics_summary.typ'],
    patterns: [
      ['title', /Archive Summary Report/],
    ],
  },
  {
    basenames: ['anniversary.typ'],
    patterns: [
      ['title pattern', /Anniversary Report/],
      ['Day header', /Day \d/],
    ],
  },
  {
    basenames: ['group_divider.typ'],
    patterns: [
      ['eyebrow pattern', /Grouped by /],
    ],
  },
  {
    basenames: ['biography_appendix.typ'],
    patterns: [
      ['section', /\[Biography\]/],
    ],
  },
  {
    basenames: ['event_landscape.typ', 'event_portrait.typ'],
    patterns: [
      ['table header DISPLAY ID', /\bDISPLAY ID\b/],
      ['table header NAME', /\bNAME\b/],
      ['table header DATES', /\bDATES\b/],
    ],
  },
];

const FORBIDDEN = [
  [
    'analytics_summary verbose subtitle',
    /High-level archive analytics[\s\S]*Confederate Home participation/,
    'drop the 38-word subtitle under the "Archive Summary Report" title',
  ],
  [
    'analytics_summary stacked sub-heading "Status breakdown"',
    /\[Status breakdown\]/,
    'drop the sub-heading; the parent "Confederate Home Participation" heading is enough',
  ],
  [
    'analytics_summary stacked sub-heading "Most frequent home names"',
    /\[Most frequent home names\]/,
    'drop the sub-heading; the parent heading is enough',
  ],
  [
    'analytics_summary stacked sub-heading "Birth decades"',
    /\[Birth decades\]/,
    'drop the sub-heading; the entries are self-evidently decades',
  ],
  [
    'analytics_summary stacked sub-heading "Death decades"',
    /\[Death decades\]/,
    'drop the sub-heading; the entries are self-evidently decades',
  ],
  [
    'group divider trailing verbose sentence (canonical)',
    /\[\s*The following record pages belong to this section\.\s*\]/,
    'drop the trailing sentence; the divider page itself is the instruction',
  ],
  [
    'event card ALL-CAPS eyebrow INTERNAL NOTES',
    /\[INTERNAL NOTES\]/,
    'drop the eyebrow; keep the label-as-bold',
  ],
  [
    'event card ALL-CAPS eyebrow LINKED PERSON RECORDS',
    /\[LINKED PERSON RECORDS\]/,
    'drop the eyebrow; keep the label-as-bold',
  ],
  [
    'biography_appendix status-form empty state',
    /No biography recorded for this person\./,
    'replace with action-form copy or drop; the "Biography" heading alone suffices',
  ],
];

// Visible-text duplication: any 16+ char prose line appearing twice
// in the SAME template file (after markup is stripped) is flagged
// when it is not a known canonical label.
function duplicateFindings(file, stripped) {
  const out = [];
  const counts = new Map();
  for (const line of stripped) {
    if (line.length < 16) continue;
    counts.set(line, (counts.get(line) || 0) + 1);
  }
  for (const [line, n] of counts) {
    if (n < 2) continue;
    if (/^(Person Record|Source Record|DISPLAY ID|NAME|DATES|Linked Person|Grouped by|Day \d|Birth Decades|Death Decades|Pension|Unit Representation|Confederate Home|Top Cemeteries|Record Types|Confederate Home Participation|Chronological Overview)/i.test(line)) continue;
    out.push({
      file,
      rule: 'duplicate',
      name: 'duplicate visible line',
      text: line,
      count: n,
      hint: 'drop the duplicated line',
    });
  }
  return out;
}

function fileShort(file) {
  return file.replace(ROOT, '').replace(/^[/\\]+/, '');
}

const findings = [];
let scanned = 0;

try {
  statSync(TEMPLATES);
} catch (err) {
  console.error(`fatal: cannot read templates directory at ${TEMPLATES}: ${err.message}`);
  process.exit(2);
}

for (const file of walk(TEMPLATES)) {
  const source = readFileSync(file, 'utf8');
  scanned += 1;
  const base = basename(file);
  const stripped = visibleText(source);

  for (const spec of REQUIRED_BY_BASENAME) {
    if (!spec.basenames.includes(base)) continue;
    for (const [name, pattern] of spec.patterns) {
      if (!pattern.test(source)) {
        findings.push({
          file,
          rule: 'required',
          name,
          text: String(pattern),
          hint: `required copy missing: ${name}`,
        });
      }
    }
  }

  for (const [name, pattern, hint] of FORBIDDEN) {
    if (pattern.test(source)) {
      findings.push({ file, rule: 'forbidden', name, text: String(pattern), hint });
    }
  }

  findings.push(...duplicateFindings(file, stripped));
}

const byRule = { required: 0, forbidden: 0, duplicate: 0 };
for (const finding of findings) byRule[finding.rule]++;

console.log(`Templates scanned: ${scanned}`);
console.log(`  Required copy findings: ${byRule.required}`);
console.log(`  Forbidden/verbose copy findings: ${byRule.forbidden}`);
console.log(`  Duplicate copy findings: ${byRule.duplicate}`);
console.log(`Total findings: ${findings.length}`);

if (findings.length) {
  console.log('');
  console.log('=== findings ===');
  for (const finding of findings) {
    console.log(`  ${finding.rule.padEnd(10)}  ${fileShort(finding.file)}  ${finding.name}`);
    console.log(`              ${finding.text}`);
    console.log(`              → ${finding.hint}`);
  }
}

if (STRICT && findings.length) {
  console.log('');
  console.log('--strict: treating as a CI failure.');
  process.exit(1);
}

console.log(findings.length ? '\nTypst PDF microcopy sweep: informational findings.' : '\nTypst PDF microcopy sweep: clean.');
process.exit(0);
