#!/usr/bin/env bash
# promote-prep.sh — show the dev vs $STABLE_BRANCH divergence
# in both directions, then print the recommended sync steps
# (per ADR 0009 §"Conflict policy"). Read-only: does not push,
# does not merge, does not modify any branch.

set -euo pipefail

STABLE_BRANCH="${STABLE_BRANCH:-stable}"

echo "=== promote-prep: sync $STABLE_BRANCH with dev ==="
git fetch origin "$STABLE_BRANCH" dev
echo ""
echo "Commits on dev not on $STABLE_BRANCH:"
if git log "origin/$STABLE_BRANCH..origin/dev" --oneline 2>/dev/null | head -50; then
  :
else
  echo "  (none; dev and $STABLE_BRANCH are in sync)"
fi
echo ""
echo "Commits on $STABLE_BRANCH not on dev:"
if git log "origin/dev..origin/$STABLE_BRANCH" --oneline 2>/dev/null | head -10; then
  :
else
  echo "  (none)"
fi
echo ""
echo "promote-prep: if the divergence is non-conflicting, run:"
echo "  git checkout $STABLE_BRANCH && git merge --no-ff origin/dev"
echo "If the divergence is conflicting, resolve conflicts locally per ADR 0009 §\"Conflict policy\","
echo "commit the resolution to $STABLE_BRANCH via a hot-fix PR, then re-run 'just promote'."
