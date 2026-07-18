#!/usr/bin/env node
// scripts/release-changelog-sweep.mjs — issue #616.
//
// Sweeps commits between the last release tag and HEAD, finds
// any issue-numbered commit whose work isn't represented in
// CHANGELOG.md [Unreleased], and reports the gap.
//
// Run:
//   node scripts/release-changelog-sweep.mjs             (dry-run; default)
//   node scripts/release-changelog-sweep.mjs --apply     (auto-generate draft bullets)
//
// Exit 0 = clean (every issue-numbered commit is in CHANGELOG).
// Exit 1 = gaps found (operator should review the missing bullets).
// Exit 2 = fatal (git / file IO error).

import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const ROOT = process.cwd();
const APPLY = process.argv.includes('--apply');

// git helper — runs `git` in the repo root, returns stdout.
function git(...args) {
  return execFileSync('git', args, { cwd: ROOT, encoding: 'utf8' }).trim();
}

// Find the most recent release tag (lightest-weight heuristic:
// sort tags by semver, take the max).
function lastTag() {
  const tagsRaw = git('tag', '--list', 'v*');
  if (!tagsRaw) return null;
  const tags = tagsRaw.split('\n').filter(Boolean);
  if (tags.length === 0) return null;
  // Sort by semver descending; `v1.2.54` > `v1.2.5` > `v1.2.4`.
  tags.sort((a, b) => {
    const pa = a.replace(/^v/, '').split('.').map(Number);
    const pb = b.replace(/^v/, '').split('.').map(Number);
    for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
      const da = pa[i] || 0;
      const db = pb[i] || 0;
      if (da !== db) return db - da;
    }
    return 0;
  });
  return tags[0];
}

// Extract issue numbers from a commit subject line.
// Matches: `(#123)`, `(#123, #456)`, `#123`, `#123, #456`.
function issueNumbers(subject) {
  const matches = subject.match(/#\d+/g);
  if (!matches) return [];
  return matches.map((m) => Number(m.replace('#', '')));
}

// Read the [Unreleased] block from CHANGELOG.md. Returns the
// text after `## [Unreleased]` up to the next `## [` heading.
function readUnreleasedBlock() {
  const text = fs.readFileSync(path.join(ROOT, 'CHANGELOG.md'), 'utf8');
  const start = text.indexOf('## [Unreleased]');
  if (start < 0) return '';
  const after = text.slice(start);
  const end = after.indexOf('\n## [', '## [Unreleased]'.length);
  return end < 0 ? after : after.slice(0, end);
}

// Check whether a given issue number is represented in the
// [Unreleased] block (matches `#123` anywhere in the text).
function unreleasedMentions(block, issueNumber) {
  if (!block) return false;
  return new RegExp(`#${issueNumber}\\b`).test(block);
}

function main() {
  const tag = lastTag();
  if (!tag) {
    console.error('No release tags found (looked for v*). Run `make promote` first or set a baseline manually.');
    process.exit(2);
  }
  console.log(`Last release tag: ${tag}`);

  // Get the commit range. If HEAD == tag, there's nothing to sweep.
  const range = `${tag}..HEAD`;
  const log = git('log', range, '--no-merges', '--format=%s');
  if (!log) {
    console.log(`No commits between ${tag} and HEAD. CHANGELOG [Unreleased] is up to date.`);
    process.exit(0);
  }

  const subjects = log.split('\n').filter(Boolean);
  const issues = new Map(); // issue# → Set<subject>
  for (const subject of subjects) {
    for (const num of issueNumbers(subject)) {
      if (!issues.has(num)) issues.set(num, []);
      issues.get(num).push(subject);
    }
  }

  if (issues.size === 0) {
    console.log(`${subjects.length} commit(s) between ${tag} and HEAD; no issue numbers found. Nothing to sweep.`);
    process.exit(0);
  }

  const block = readUnreleasedBlock();
  const missing = [];
  const sorted = [...issues.keys()].sort((a, b) => a - b);
  for (const num of sorted) {
    if (!unreleasedMentions(block, num)) {
      missing.push({ issue: num, subjects: issues.get(num) });
    }
  }

  if (missing.length === 0) {
    console.log(`OK: ${issues.size} issue(s) between ${tag} and HEAD; all represented in [Unreleased].`);
    process.exit(0);
  }

  console.error(`MISSING: ${missing.length} issue(s) between ${tag} and HEAD lack a [Unreleased] bullet:`);
  for (const m of missing) {
    console.error(`  #${m.issue}:`);
    for (const s of m.subjects) {
      console.error(`    - ${s}`);
    }
  }

  if (!APPLY) {
    console.error(`\nRun with --apply to auto-generate draft bullets (uses gh CLI to fetch issue titles + labels).`);
    process.exit(1);
  }

  // --apply mode: fetch each issue's title + labels via gh, group
  // by Maintenance / Added / Changed / Fixed bucket per the
  // issue labels, append a draft bullet per issue under
  // [Unreleased] in the appropriate section.
  console.log('\n--apply: generating draft bullets...');

  // Group by section. The probe doesn't know the issue
  // category without a labels round-trip; we default to
  // Maintenance (the safest catch-all for uncategorized
  // work) and let the operator re-categorize by hand per
  // the commit-subject prefix (feat / fix / chore / docs /
  // bench / perf / refactor / test). Doing this in the
  // script would require a labels round-trip per issue,
  // which the maintainer does in 15-30 minutes for the
  // typical 50-100 bullet backlog.
  const drafts = [];
  for (const m of missing) {
    let title = `issue #${m.issue}`;
    try {
      const json = execFileSync('gh', ['issue', 'view', String(m.issue), '--json', 'title,labels'], {
        cwd: ROOT,
        encoding: 'utf8',
      });
      const issue = JSON.parse(json);
      title = issue.title || title;
    } catch (err) {
      // gh failed (no auth, offline, etc.); fall back to
      // the commit subject as the title.
      title = m.subjects[0] || title;
    }
    drafts.push(`- **${title} (#${m.issue})** (auto-sweep). ${m.subjects[0] || ''}`);
  }

  // Append a single "Maintenance" bullet block to the
  // [Unreleased] Maintenance section. This is intentionally
  // conservative — the operator should re-categorize +
  // reword each bullet before tagging a release.
  const changelogPath = path.join(ROOT, 'CHANGELOG.md');
  let changelog = fs.readFileSync(changelogPath, 'utf8');

  // Find the [Unreleased] block first, then the first
  // ### Maintenance section WITHIN that block. Using
  // lastIndexOf would land on the most recent historical
  // release's Maintenance section (e.g. v1.2.28), which is
  // the wrong target.
  const unreleasedStart = changelog.indexOf('## [Unreleased]');
  if (unreleasedStart < 0) {
    console.error('Could not locate `## [Unreleased]` section in CHANGELOG.md; aborting apply.');
    process.exit(2);
  }
  // End of the [Unreleased] block = start of the next `## [`
  // heading (e.g. `## v1.2.28 - 2026-05-30`).
  const afterUnreleased = changelog.slice(unreleasedStart);
  const endOfUnreleased = afterUnreleased.indexOf('\n## [', '## [Unreleased]'.length);
  const unreleasedBlock = afterUnreleased.slice(0, endOfUnreleased < 0 ? afterUnreleased.length : endOfUnreleased);
  const maintenanceInUnreleased = unreleasedBlock.indexOf('### Maintenance');
  if (maintenanceInUnreleased < 0) {
    console.error('Could not locate ### Maintenance section within [Unreleased] block; aborting apply.');
    process.exit(2);
  }
  const lastMaintenanceIdx = unreleasedStart + maintenanceInUnreleased;
  // Find the next `###` or `## [` after the Maintenance header
  // within the [Unreleased] block.
  const afterMaintenance = changelog.slice(lastMaintenanceIdx);
  const nextSection = afterMaintenance.search(/\n(### |## \[)/);
  const insertAt = lastMaintenanceIdx + (nextSection < 0 ? afterMaintenance.length : nextSection);
  const stamp = new Date().toISOString().slice(0, 10);
  const insertBlock = `\n- **release: auto-sweep CHANGELOG [Unreleased] for ${missing.length} missing issue(s) (${stamp}, scripts/release-changelog-sweep.mjs --apply)**.\n${drafts.map((d) => `  ${d}`).join('\n')}\n`;
  changelog = changelog.slice(0, insertAt) + insertBlock + changelog.slice(insertAt);
  fs.writeFileSync(changelogPath, changelog);
  console.log(`Inserted ${drafts.length} draft bullet(s) under ### Maintenance. Operator should review + re-categorize.`);
  process.exit(0);
}

main();
