#!/usr/bin/env node
// probe_issue_estimates.mjs — issue #619 regression net.
//
// Lists every open GitHub issue missing the `## Estimate`
// header (the canonical PERT format documented in
// docs/agents/issue-tracker.md §"Estimate"). Run via:
//
//   node audit/probe_issue_estimates.mjs
//
// Exit 0 = every open issue has the header.
// Exit 1 = one or more issues are missing the header
//          (these should be labeled `needs-info`).
//
// The probe is read-only; it never modifies the issue
// tracker. It shells out to `gh issue list` + `gh issue view`
// to fetch the open issues + their bodies. No GitHub token
// required beyond the `gh` CLI's existing auth state.
import { execFileSync } from "node:child_process";
import process from "node:process";

const ESTIMATE_HEADER = /^##\s+Estimate\s*$/m;
const MIN_BODY_LENGTH = 200; // ignore "stub" issues with a body < 200 chars

function gh(...args) {
  try {
    return execFileSync("gh", args, { encoding: "utf8" });
  } catch (err) {
    console.error(`gh ${args.join(" ")} failed: ${err.message}`);
    process.exit(2);
  }
}

function main() {
  // Fetch open issues (limit 100; the project rarely has more
  // open issues than that).
  const listJson = gh(
    "issue",
    "list",
    "--state",
    "open",
    "--limit",
    "100",
    "--json",
    "number,title,body",
  );
  const issues = JSON.parse(listJson);

  const missing = [];
  for (const issue of issues) {
    const body = (issue.body || "").trim();
    // Skip stub issues (very short bodies — usually "needs-triage"
    // placeholders).
    if (body.length < MIN_BODY_LENGTH) continue;
    if (!ESTIMATE_HEADER.test(body)) {
      missing.push({
        number: issue.number,
        title: issue.title,
        bodyLen: body.length,
      });
    }
  }

  if (missing.length === 0) {
    console.log(`OK: ${issues.length} open issue(s); all have the \`## Estimate\` header.`);
    process.exit(0);
  }

  console.error(`MISSING: ${missing.length} open issue(s) lack the \`## Estimate\` header:`);
  for (const m of missing) {
    console.error(`  #${m.number} (${m.bodyLen} chars body): ${m.title}`);
  }
  console.error(`\nLabel these \`needs-info\` and add the header.`);
  process.exit(1);
}

main();
