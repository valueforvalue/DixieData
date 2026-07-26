#!/usr/bin/env bash
# promote-confirm.sh — post-merge sanity check per ADR 0009
# §"The promote flow" step 4. Asserts origin/$STABLE_BRANCH
# and origin/dev are in sync (no diff). Exits 0 on sync,
# non-zero otherwise with a commit-list dump for triage.
#
# Reads STABLE_BRANCH from env (defaults to "stable").

set -euo pipefail

STABLE_BRANCH="${STABLE_BRANCH:-stable}"

echo "=== promote-confirm: post-merge sanity ==="
git fetch origin "$STABLE_BRANCH" dev
echo ""

if git diff --quiet "origin/$STABLE_BRANCH..origin/dev" 2>/dev/null; then
  echo "promote-confirm: $STABLE_BRANCH and dev are in sync. Safe to run scripts/release-github.ps1."
else
  echo "promote-confirm: $STABLE_BRANCH and dev STILL differ. Investigate before tagging."
  git log "origin/$STABLE_BRANCH..origin/dev" --oneline 2>/dev/null | head -10 || true
  exit 1
fi
