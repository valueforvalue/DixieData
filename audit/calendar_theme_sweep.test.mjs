import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');

let pass = 0;
let fail = 0;
function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  ✓ ${name}`);
  } catch (err) {
    fail++;
    console.log(`  ✗ ${name}`);
    console.log(`    ${err.message}`);
  }
}

// Issue #482 (A4 follow-up to #477): the calendar + calendar_day +
// browse + recovery + share templates still carry hardcoded hex
// literals for body text + form labels + stat labels + tag pill
// text. The pre-fix behavior was that switching themes (Default →
// High Contrast → Soft) left those surfaces showing slate-blue text
// regardless of the theme — i.e. the body text didn't follow the
// theme palette.
//
// This regression net pins the fix: every muted body-text hex value
// in calendar + calendar_day + browse must be replaced with a
// var(--theme-*) token (specifically --theme-text-mid or
// --theme-text-muted). Hex values used for SEMANTIC state colors
// (success / error / info / warning + the "Today" pill green) are
// intentionally NOT swept — those need to stay constant across
// themes for the state to read correctly.

const TARGET_FILES = [
  'internal/templates/calendar.templ',
  'internal/templates/calendar_day.templ',
  'internal/templates/browse.templ',
];

// Hex values that MUST be gone from the target files (they're
// theme-leaking muted body text + form labels).
const FORBIDDEN_HEX = [
  '#445260', // body text — calendar descriptions
  '#5a6a78', // muted hint text — calendar_day hints + empty-state text
  '#51606e', // form labels — calendar/calendar_day section headers
  '#6a7a88', // stat count labels — calendar hero stats
  '#5a3b1f', // tag pill text — browse filter chips
];

test('theme-leaking muted text hex values are gone from calendar + calendar_day + browse', () => {
  for (const rel of TARGET_FILES) {
    const content = readFileSync(join(ROOT, rel), 'utf8');
    for (const hex of FORBIDDEN_HEX) {
      const needle = `text-[${hex}]`;
      if (content.includes(needle)) {
        throw new Error(
          `${rel} still contains hardcoded \`text-[${hex}]\`. ` +
          `Replace with \`text-[var(--theme-text-mid)]\` or \`text-[var(--theme-text-muted)]\` ` +
          `so the muted body text follows the active theme. ` +
          `See issue #482.`,
        );
      }
    }
  }
});

test('calendar + calendar_day + browse now use --theme-text-mid or --theme-text-muted for body text', () => {
  // At least one var(--theme-text-mid) and one var(--theme-text-muted)
  // reference must be present in each swept file (proves the swap
  // landed, not just deleted).
  for (const rel of TARGET_FILES) {
    const content = readFileSync(join(ROOT, rel), 'utf8');
    const hasMid = content.includes('text-[var(--theme-text-mid)]');
    const hasMuted = content.includes('text-[var(--theme-text-muted)]');
    if (!hasMid && !hasMuted) {
      throw new Error(
        `${rel} has no var(--theme-text-mid) or var(--theme-text-muted) reference. ` +
        `At least one themed body-text token must be present so the sweep landed. ` +
        `See issue #482.`,
      );
    }
  }
});

test('--theme-text-mid has per-theme overrides in tailwind.css (Default + High Contrast + Soft)', () => {
  const css = readFileSync(join(ROOT, 'frontend/tailwind.css'), 'utf8');
  // Pull every "--theme-text-mid: ..." assignment. Need one in
  // :root (Default) + one in html[data-theme="high-contrast"] + one
  // in html[data-theme="soft"].
  const matches = [...css.matchAll(/--theme-text-mid:\s*([^;]+);/g)].map((m) => m[1].trim());
  if (matches.length < 3) {
    throw new Error(
      `expected 3 --theme-text-mid declarations (Default + High Contrast + Soft); ` +
      `found ${matches.length}: ${JSON.stringify(matches)}`,
    );
  }
});

test('--theme-text-muted has per-theme overrides in tailwind.css', () => {
  const css = readFileSync(join(ROOT, 'frontend/tailwind.css'), 'utf8');
  const matches = [...css.matchAll(/--theme-text-muted:\s*([^;]+);/g)].map((m) => m[1].trim());
  if (matches.length < 3) {
    throw new Error(
      `expected 3 --theme-text-muted declarations; ` +
      `found ${matches.length}: ${JSON.stringify(matches)}`,
    );
  }
});

console.log(`\n  calendar_theme_sweep regression net: ${pass} passed, ${fail} failed`);
if (fail > 0) {
  process.exit(1);
}