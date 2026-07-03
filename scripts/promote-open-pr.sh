#!/usr/bin/env bash
# promote-open-pr.sh — open a `dev → stable` PR with gate-chain output
# embedded in the body. Called from `make promote` after `make promote-dry-run`
# has passed. Per ADR 0009 §"The promote flow (PR via GitHub UI)".
#
# Usage: promote-open-pr.sh <stable-branch>
#
# Exits 0 on success (PR opened); non-zero on any failure (the PR
# is not opened in that case). The script does NOT merge the PR.
# The operator reviews + merges via the GitHub UI per ADR 0009.
#
# Requires:
#   - gh CLI authenticated with PR-write scope
#   - git remote `origin` reachable
#   - versioninfo.go parseable (AppVersion line)

set -euo pipefail

STABLE_BRANCH="${1:-stable}"

if [ -z "${STABLE_BRANCH}" ]; then
  echo "promote-open-pr: STABLE_BRANCH is required (usage: promote-open-pr.sh <stable-branch>)"
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "promote-open-pr: gh CLI not found in PATH"
  exit 1
fi

# Parse AppVersion from versioninfo.go. The line shape is:
#   AppVersion = "1.1.1"
# This is the line the bump-version.ps1 script maintains; we read
# it the same way for consistency.
version=$(grep -oP 'AppVersion\s*=\s*"\K[^"]+' internal/versioninfo/versioninfo.go | head -1)
if [ -z "${version}" ]; then
  echo "promote-open-pr: could not parse AppVersion from versioninfo.go"
  exit 1
fi

# Capture the commit log BEFORE writing the body, so the body
# shows what was on dev at PR-open time.
commit_log=$(git log origin/"${STABLE_BRANCH}"..origin/dev --oneline 2>/dev/null | head -50 || true)
diff_stat=$(git diff origin/"${STABLE_BRANCH}"..origin/dev --stat 2>/dev/null | tail -20 || true)

# Build the PR body in a temp file. Using a file (not a heredoc in
# the shell invocation) avoids escaping headaches with gh's --body
# argument + backticks + markdown.
body_file=$(mktemp)
trap 'rm -f "${body_file}"' EXIT

cat > "${body_file}" <<EOF
## Promotion: dev → ${STABLE_BRANCH} (v${version})

Auto-opened by \`make promote\` via \`scripts/promote-open-pr.sh\`. Per ADR 0009, the operator is the merge authority.

### Gate chain result

\`make promote-dry-run\` passed (gates: \`test tpl css bump-verify debug freshness archive\`).

### Commit log

\`\`\`
${commit_log}
\`\`\`

### Diff stat (top 20 lines)

\`\`\`
${diff_stat}
\`\`\`

### Review checklist

- [ ] Diff reviewed (\`git diff origin/${STABLE_BRANCH}..origin/dev --stat\`)
- [ ] Commit shape reviewed (one-commit-per-change per AGENTS.md)
- [ ] \`safe-for-in-place\` label applied (or \`unsafe-for-in-place\` with explanation)
- [ ] CI green (audit + build + test) — the branch protection rule requires this

### Operator action

Merge via the GitHub UI when ready. After merge:

\`\`\`bash
make promote-confirm           # sanity: stable HEAD == dev merge SHA
pwsh -File scripts/release-github.ps1   # tag + draft release
\`\`\`
EOF

echo "promote-open-pr: opening PR dev → ${STABLE_BRANCH} (v${version})"
gh pr create \
  --base "${STABLE_BRANCH}" \
  --head dev \
  --title "promote: dev → ${STABLE_BRANCH} (v${version})" \
  --body-file "${body_file}"

echo ""
echo "promote-open-pr: PR opened. Review + merge via the GitHub UI."