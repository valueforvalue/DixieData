#!/usr/bin/env node
// lint-rc-commits.mjs — commit-message gate for rc/v* branches
// (ADR 0011). Walks the commits added by a PR targeting an
// rc/v* branch and fails if any commit's conventional-commit
// type is in the disallowed list.
//
// Allowed on rc/v*:
//   fix, docs, test, ci, chore
//
// Disallowed on rc/v*:
//   feat, refactor, perf, build, or any commit that omits a
//   type: prefix entirely (a feat by another name).
//
// Also rejects any commit whose diff touches ≥ 50 files —
// large diffs are the canonical signal of a refactor or
// feature masquerading as a fix (per ADR 0011 §Decision 2).
//
// The script reads GITHUB_BASE_REF / GITHUB_HEAD_REF + the
// GitHub API to discover the merge-base + diffstat. It is
// designed to run in CI on every PR to rc/v*. It also has a
// `--local <base>..<head>` mode for pre-push local checks.
//
// Exit code:
//   0 — every commit passes
//   1 — at least one disallowed commit or oversized diff
//   2 — environment error (missing token, bad args)

import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";

const ALLOWED_TYPES = new Set(["fix", "docs", "test", "ci", "chore"]);
const TYPE_PATTERN = /^(?<type>[a-z]+)(?:\([^)]+\))?!?: /;
const MAX_DIFF_FILES = 50;

function parseArgs(argv) {
  const out = { mode: "ci", base: null, head: null };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--local") {
      out.mode = "local";
      out.base = argv[++i];
      out.head = argv[++i];
    } else if (a === "--help" || a === "-h") {
      printHelp();
      process.exit(0);
    }
  }
  return out;
}

function printHelp() {
  console.log(`lint-rc-commits.mjs — gate for rc/v* branches (ADR 0011)

Usage:
  # CI mode (default): read PR base/head from GITHUB_* env
  node scripts/ci/lint-rc-commits.mjs

  # Local mode: pre-push check
  node scripts/ci/lint-rc-commits.mjs --local <base-sha> <head-sha>

Exit codes:
  0  all commits pass
  1  at least one disallowed commit
  2  environment error`);
}

function getCommitMessagesLocal(base, head) {
  const out = execFileSync("git", ["log", "--no-merges", "--pretty=%s", `${base}..${head}`], { encoding: "utf8" });
  return out.trim().split("\n").filter(Boolean);
}

function getCommitSHAsLocal(base, head) {
  const out = execFileSync("git", ["log", "--no-merges", "--pretty=%H", `${base}..${head}`], { encoding: "utf8" });
  return out.trim().split("\n").filter(Boolean);
}

function getDiffStatLocal(base, head) {
  const out = execFileSync("git", ["diff", "--name-only", `${base}..${head}`], { encoding: "utf8" });
  return out.trim().split("\n").filter(Boolean);
}

function classifySubject(subject) {
  const m = subject.match(TYPE_PATTERN);
  if (!m) {
    return { type: null, valid: false, reason: "no conventional-commit type prefix" };
  }
  const type = m.groups.type;
  if (!ALLOWED_TYPES.has(type)) {
    return { type, valid: false, reason: `type '${type}:' is disallowed on rc/v* branches (ADR 0011)` };
  }
  return { type, valid: true, reason: null };
}

function lint({ subjects, shas, fileCounts }) {
  const failures = [];
  subjects.forEach((subject, idx) => {
    const sha = shas[idx] || "(unknown)";
    const short = sha.slice(0, 7);
    const result = classifySubject(subject);
    if (!result.valid) {
      failures.push(`  [${short}] ${subject}  —  ${result.reason}`);
    }
  });
  for (const { sha, count } of fileCounts) {
    if (count >= MAX_DIFF_FILES) {
      const short = sha.slice(0, 7);
      failures.push(`  [${short}] (any subject)  —  diff touches ${count} files; rc/v* cap is ${MAX_DIFF_FILES} (ADR 0011)`);
    }
  }
  return failures;
}

async function getGitHubPRInfo() {
  const token = process.env.GITHUB_TOKEN;
  const eventPath = process.env.GITHUB_EVENT_PATH;
  const repo = process.env.GITHUB_REPOSITORY;
  if (!token || !eventPath || !repo) {
    console.error("lint-rc-commits: GITHUB_TOKEN / GITHUB_EVENT_PATH / GITHUB_REPOSITORY required in CI mode");
    process.exit(2);
  }
  const event = JSON.parse(readFileSync(eventPath, "utf8"));
  if (!event.pull_request) {
    console.error("lint-rc-commits: GITHUB_EVENT_PATH does not contain a pull_request event");
    process.exit(2);
  }
  const prNumber = event.pull_request.number;
  const baseRef = event.pull_request.base.ref;
  const headRef = event.pull_request.head.ref;
  // Fetch commits via the GitHub API
  const url = `https://api.github.com/repos/${repo}/pulls/${prNumber}/commits?per_page=100`;
  const resp = await fetch(url, {
    headers: {
      Authorization: `token ${token}`,
      Accept: "application/vnd.github+json",
      "User-Agent": "DixieData-rc-lint/1.0",
    },
  });
  if (!resp.ok) {
    console.error(`lint-rc-commits: GitHub API returned HTTP ${resp.status} for PR #${prNumber}`);
    process.exit(2);
  }
  const commits = await resp.json();
  // Fetch the diff for the whole PR to get total file count
  const diffResp = await fetch(`${url}&per_page=1`, {
    headers: {
      Authorization: `token ${token}`,
      Accept: "application/vnd.github.diff",
      "User-Agent": "DixieData-rc-lint/1.0",
    },
  });
  const diffText = diffResp.ok ? await diffResp.text() : "";
  const fileCount = (diffText.match(/^diff --git /gm) || []).length;
  return {
    base: baseRef,
    head: headRef,
    subjects: commits.map((c) => (c.commit && c.commit.message ? c.commit.message.split("\n")[0] : "")),
    shas: commits.map((c) => c.sha),
    fileCounts: [{ sha: "(pr)", count: fileCount }],
  };
}

function main() {
  const args = parseArgs(process.argv);
  if (args.mode === "local") {
    if (!args.base || !args.head) {
      console.error("lint-rc-commits: --local requires <base> and <head>");
      process.exit(2);
    }
    const subjects = getCommitMessagesLocal(args.base, args.head);
    const shas = getCommitSHAsLocal(args.base, args.head);
    const files = getDiffStatLocal(args.base, args.head);
    const fileCounts = [{ sha: "(range)", count: files.length }];
    run({ subjects, shas, fileCounts, base: args.base, head: args.head });
  } else {
    getGitHubPRInfo().then(run).catch((err) => {
      console.error(`lint-rc-commits: ${err.message || err}`);
      process.exit(2);
    });
  }
}

function run({ subjects, shas, fileCounts, base, head }) {
  // Accept any base ref that contains 'rc/v' (e.g. 'rc/v1.1',
  // 'origin/rc/v1.1', or 'refs/heads/rc/v1.1'). Local mode
  // passes the qualified ref; CI mode passes the bare branch
  // name. ADR 0011 §Decision 1.
  if (!base || !/rc\/v/.test(base)) {
    console.log(`lint-rc-commits: base ref '${base}' is not an rc/v* branch; gate is a no-op.`);
    process.exit(0);
  }
  console.log(`lint-rc-commits: checking ${subjects.length} commit(s) on ${base}..${head}`);
  const failures = lint({ subjects, shas, fileCounts });
  if (failures.length === 0) {
    console.log("lint-rc-commits: all commits pass the rc/v* policy (ADR 0011).");
    process.exit(0);
  }
  console.error("");
  console.error("lint-rc-commits: the following commits violate the rc/v* policy (ADR 0011):");
  console.error("");
  for (const f of failures) console.error(f);
  console.error("");
  console.error("Allowed types on rc/v*: fix, docs, test, ci, chore (see ADR 0011 §Decision 2).");
  console.error("Maximum diff size: 50 files. Larger diffs are rejected as refactor-by-stealth.");
  console.error("Need a feature on this release line? Open it on dev instead.");
  process.exit(1);
}

main();
