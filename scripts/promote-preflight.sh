#!/usr/bin/env bash
# promote-preflight.sh — print the dev vs $STABLE_BRANCH
# divergence summary that the promote-dry-run + promote
# recipes append after the gate chain. Common code path
# between the two recipes (don't push, don't tag, don't open
# a PR — read-only inspection).
#
# Reads STABLE_BRANCH from env (defaults to "stable"). The
# justfile recipe sets it via ${STABLE_BRANCH:-stable}.

set -euo pipefail

STABLE_BRANCH="${STABLE_BRANCH:-stable}"

echo ""
echo "=== pre-flight: dev vs $STABLE_BRANCH divergence ==="
git fetch origin "$STABLE_BRANCH" dev 2>/dev/null || true
echo "Commits on dev not on $STABLE_BRANCH:"
if git log "origin/$STABLE_BRANCH..origin/dev" --oneline 2>/dev/null | head -50; then
  :
else
  echo "  (none; dev and $STABLE_BRANCH are in sync)"
fi
