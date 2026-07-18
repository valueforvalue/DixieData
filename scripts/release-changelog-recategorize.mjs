#!/usr/bin/env node
// scripts/release-changelog-recategorize.mjs — issue #624
// follow-up.
//
// Reads the auto-sweep dump in CHANGELOG.md [Unreleased]
// (inserted by release-changelog-sweep.mjs --apply) and
// splits each bullet into the correct ### Added /
// ### Changed / ### Fixed / ### Maintenance bucket per
// the commit-subject prefix parsed from the appended
// subject line (the original commit subject is included
// as a leading comment per the auto-sweep format).
//
// Run: node scripts/release-changelog-recategorize.mjs
//
// Exit 0 = every auto-sweep bullet was recategorized.
// Exit 1 = some bullets couldn't be categorized.
// Exit 2 = fatal.
//
// The script is idempotent: re-running on an already-
// recategorized dump is a no-op (the script detects the
// recategorized format and skips the affected lines).
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'path';
import process from 'node:process';

const ROOT = process.cwd();
const CHANGELOG = path.join(ROOT, 'CHANGELOG.md');

function git(...args) {
  return execFileSync('git', args, { cwd: ROOT, encoding: 'utf8' }).trim();
}

// Find the [Unreleased] block.
function readUnreleased(text) {
  const start = text.indexOf('## [Unreleased]');
  if (start < 0) return { start: -1, end: -1, body: '' };
  const after = text.slice(start);
  const endIdx = after.indexOf('\n## [', '## [Unreleased]'.length);
  const end = endIdx < 0 ? text.length : start + endIdx;
  return { start, end, body: text.slice(start, end) };
}

// Categorize a single auto-sweep bullet by parsing the
// commit-subject prefix from the trailing ". feat(...): ..."
// or ". fix(...): ..." etc. appended by the sweep script.
function categorize(bulletText) {
  // Bullet format from sweep.mjs:
  //   - **Title (#N)** (auto-sweep). <subject>
  // Subject format (conventional commit):
  //   <type>(<scope>): <description>
  //   path/to/area: <description>          (DixieData uses path-style too)
  //   <type1> + <type2> + ...: <description> (compound subject)
  const m = bulletText.match(/\(auto-sweep\)\.\s+(.+)$/);
  if (!m) return null;
  const subject = m[1].trim();
  // Try conventional-commit prefix first.
  let typeMatch = subject.match(/^([a-z][a-z0-9_-]*)(?:\([^)]+\))?:/);
  // If that fails, try the first word before ' + ' (compound
  // subject like "archive + appshell + templates: ...").
  if (!typeMatch) {
    const firstWord = subject.match(/^([a-z][a-z0-9_-]*)\b/);
    if (firstWord) {
      typeMatch = [null, firstWord[1]];
    }
  }
  if (!typeMatch) return null;
  const type = typeMatch[1];
  switch (type) {
    case 'feat':
    case 'ux':       // ux(buttons|tag|forms|...):  enhancements are user-facing
      return 'Added';
    case 'fix':
      return 'Fixed';
    // Everything else is Maintenance (chore, docs, bench,
    // perf, refactor, test, build, ci, style, plus the
    // DixieData-specific internal prefixes: uiids,
    // buildinfo, db, cli, update, infra, templ, audit,
    // tools/...).
    case 'chore':
    case 'docs':
    case 'bench':
    case 'perf':
    case 'refactor':
    case 'test':
    case 'build':
    case 'ci':
    case 'style':
    case 'uiids':
    case 'buildinfo':
    case 'db':
    case 'cli':
    case 'update':
    case 'infra':
    case 'templ':
    case 'audit':
    case 'tools':
    case 'share':       // share(...) features: usually refactor/internal; treat as Maintenance
    case 'inventory':
    case 'compare':
    case 'tune':        // tune: green-baseline, workflow bugs, etc.
    case 'server-gate': // server-gate /soldiers/{id}* on Event rows
    case 'routebuilder': // routebuilder: add EventTagAttach + EventTagDetach
    case 'archive':     // archive + appshell + templates: ... (compound subject)
    case 'appshell':    // for single-prefix subjects
    case 'templates':   // for single-prefix subjects
    case 'tests':       // tests: update 3 pre-existing appshell test needles ...
      return 'Maintenance';
    default:
      return null;
  }
}

// Find ALL (auto-sweep) bullets in the [Unreleased] block
// (not just the ones under the auto-sweep header). This
// makes the recategorize idempotent — re-running on an
// already-recategorized dump is a no-op (it finds the same
// bullets, recategorizes them, and writes them back to
// the same positions; the surrounding ### sections get
// rebuilt with the new bucket order).
function findAutoSweepBullets(unreleasedBody) {
  // The bullet is prefixed with "  - **" and ends with
  // "(auto-sweep). <subject>". The subject can be any
  // text. The reliable anchor is the "(auto-sweep)." tail
  // (the original sweep.mjs appends this token to every
  // auto-sweep bullet).
  const re = /^\s\s- \*\*[^\n]*?\(#\d+\)\*\*\s\(auto-sweep\)\..*$/gm;
  const bullets = [];
  let m;
  while ((m = re.exec(unreleasedBody)) !== null) {
    bullets.push({ text: m[0], index: m.index });
  }
  return bullets;
}

function main() {
  const text = fs.readFileSync(CHANGELOG, 'utf8');
  const unreleased = readUnreleased(text);
  if (unreleased.start < 0) {
    console.error('Could not find [Unreleased] block.');
    process.exit(2);
  }
  const bullets = findAutoSweepBullets(unreleased.body);
  if (bullets.length === 0) {
    console.log('No auto-sweep bullets to recategorize.');
    process.exit(0);
  }

  // Group bullets by their category. Strip the
  // `(auto-sweep)` marker when writing to the categorized
  // section so the recategorize is idempotent — re-running
  // it on the already-categorized dump is a no-op (the
  // regex below matches `(auto-sweep).` literally; without
  // the marker, the categorized bullets are skipped).
  const buckets = { Added: [], Changed: [], Fixed: [], Maintenance: [], Uncategorized: [] };
  for (const b of bullets) {
    const cat = categorize(b.text) || 'Uncategorized';
    const cleanedText = b.text.replace(/\s\(auto-sweep\)\./, '.');
    buckets[cat].push(cleanedText);
  }

  // Locate each `### <Bucket>` section in the [Unreleased]
  // block. Use the LAST occurrence (the most recently added
  // section for that bucket — the convention from prior
  // CHANGELOG entries is that the operator appends new
  // sections to the END of [Unreleased], so the last
  // occurrence is the dump target). The script handles the
  // case where no ### <Bucket> section exists by creating
  // one at the position the auto-sweep block was.
  function lastSectionIdx(body, header) {
    const re = new RegExp(`^### ${header}\\s*$`, 'gm');
    let last = -1;
    let m;
    while ((m = re.exec(body)) !== null) {
      last = m.index;
    }
    return last;
  }
  const sectionIdxs = {
    Added: lastSectionIdx(unreleased.body, 'Added'),
    Fixed: lastSectionIdx(unreleased.body, 'Fixed'),
    Maintenance: lastSectionIdx(unreleased.body, 'Maintenance'),
    Changed: lastSectionIdx(unreleased.body, 'Changed'),
  };

  // Build the new content. We replace the auto-sweep
  // block with a series of (possibly new) ### sections
  // each followed by its assigned bullets.
  const out = [];
  // Emit a section per bucket that has any bullets, in
  // the conventional order: Added, Changed, Fixed, Maintenance.
  for (const bucket of ['Added', 'Changed', 'Fixed', 'Maintenance']) {
    if (buckets[bucket].length === 0) continue;
    const header = `### ${bucket}`;
    if (sectionIdxs[bucket] < 0) {
      // Create a new section at the position the auto-sweep
      // block was. This handles the case where a bucket
      // didn't exist before.
      out.push(header);
      out.push('');
      for (const b of buckets[bucket]) out.push(b);
      out.push('');
    } else {
      out.push(header);
      out.push('');
      for (const b of buckets[bucket]) out.push(b);
      out.push('');
    }
  }
  // If any uncategorized, dump them at the end with a TODO.
  if (buckets.Uncategorized.length > 0) {
    out.push('### Uncategorized (operator review required)');
    out.push('');
    for (const b of buckets.Uncategorized) out.push(b);
    out.push('');
  }

  // Build the new [Unreleased] body. The auto-sweep
  // dump occupies a single contiguous span in the
  // [Unreleased] block: the auto-sweep header + its
  // 94 (auto-sweep) bullets. We compute the span
  // boundaries (first bullet index through last bullet
  // end-of-line) and replace that span with the new
  // categorized sections. Everything before/after the
  // span is preserved verbatim.
  const firstBulletIdx = bullets[0].index;
  const lastBullet = bullets[bullets.length - 1];
  // The last bullet's text ends at the next \n; include
  // the trailing newline so the splice preserves the
  // line structure of whatever follows the auto-sweep
  // block.
  const afterLastBullet = lastBullet.index + lastBullet.text.length;
  const spanEnd = unreleased.body.indexOf('\n', afterLastBullet);
  const safeSpanEnd = spanEnd < 0 ? unreleased.body.length : spanEnd + 1;
  const beforeBlock = unreleased.body.slice(0, firstBulletIdx);
  const afterBlock = unreleased.body.slice(safeSpanEnd);
  const newBlock = out.join('\n');
  // Ensure a blank line between the prior content and our
  // new sections (markdown convention: a blank line precedes
  // every `###` heading). If the beforeBlock already ends
  // with a blank line, don't add another; if it ends with
  // a non-blank line, add one.
  const sep = beforeBlock.endsWith('\n\n') ? '' : '\n';
  const newUnreleased = beforeBlock + sep + newBlock + afterBlock;

  // Splice the new body back into the full changelog.
  const updated = text.slice(0, unreleased.start) + newUnreleased + text.slice(unreleased.end);

  fs.writeFileSync(CHANGELOG, updated);

  // Report.
  console.log('Re-categorized auto-sweep bullets:');
  for (const b of ['Added', 'Changed', 'Fixed', 'Maintenance', 'Uncategorized']) {
    if (buckets[b].length > 0) {
      console.log(`  ${b}: ${buckets[b].length}`);
    }
  }
  if (buckets.Uncategorized.length > 0) {
    console.error(`\nWARNING: ${buckets.Uncategorized.length} bullet(s) couldn't be categorized. They live in "### Uncategorized (operator review required)".`);
    process.exit(1);
  }
  process.exit(0);
}

main();
