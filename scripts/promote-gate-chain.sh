#!/usr/bin/env bash
# promote-gate-chain.sh — run the 7-gate promotion chain
# (test, tpl, css, bump-verify, debug, freshness, archive)
# in order, halting on first failure. Mirrors the Makefile's
# PROMOTE_GATES list (the `make promote-dry-run` recipe in
# the Makefile inlines the same logic; this script is the
# extracted form so the justfile and the Makefile can both
# call the same source of truth).
#
# Usage: promote-gate-chain.sh
#
# Exit codes:
#   0  all 7 gates passed
#   non-zero  the gate that failed; the for-loop aborts
#
# Why a separate script:
# - The Makefile's `promote-dry-run` recipe body is the
#   canonical gate chain (per ADR 0008 + ADR 0009). The just
#   cutover (#642) left the justfile's `promote-dry-run` as
#   a stub that just runs `git status --short; git diff
#   --check`. This script restores parity so `just
#   promote-dry-run` and `make promote-dry-run` produce the
#   same gate output.
# - Future changes to the gate set happen here once, not in
#   two recipe bodies.
#
# See ADR 0008 §"The promotion command" and ADR 0009
# §"The promote flow" for the gate-by-gate rationale.

set -euo pipefail

GATES=(
  test
  tpl
  css
  bump-verify
  debug
  freshness
  archive
)

echo "=== promote-gate-chain: gates 1-${#GATES[@]} ==="
for gate in "${GATES[@]}"; do
  echo ""
  echo "--- gate: $gate ---"
  if [ "$gate" = "bump-verify" ]; then
    # bump-verify is a flag on bump-version.ps1, not a
    # top-level recipe. The Makefile inlines the pwsh call
    # for the same reason.
    if ! pwsh -NoLogo -NoProfile -File scripts/bump-version.ps1 -VerifyOnly; then
      echo "FAIL at gate: $gate"
      exit 1
    fi
    continue
  fi
  if ! just "$gate"; then
    echo "FAIL at gate: $gate"
    exit 1
  fi
done
echo ""
echo "promote-gate-chain: OK"
