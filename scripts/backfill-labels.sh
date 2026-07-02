#!/usr/bin/env bash
# scripts/backfill-labels.sh
#
# Apply `area:*` and `priority:*` labels to existing open issues
# by parsing the issue title + body. Idempotent — skips issues
# that already carry any of the new labels.
#
# Derivation rules (best-effort; manual triage still wins):
#   area:*     title prefix (feat|fix|ux)(area) → area:<area>
#              Falls back to keyword scan in title + body for known
#              component nouns. Default: area:backend.
#   priority:* keyword scan in title + body:
#              high: 'crash', 'lost data', 'regression', 'white screen',
#                    'data loss', 'cannot', 'broken', 'fatal', 'panic',
#                    'deadlock', 'corrupt'
#              low: 'polish', 'nice to have', 'eventually', 'cosmetic',
#                   'optional', 'tweak'
#              medium: default
#
# Usage:
#   ./scripts/backfill-labels.sh --dry-run   # print assignments, don't apply
#   ./scripts/backfill-labels.sh             # apply via `gh issue edit`
#   ./scripts/backfill-labels.sh --limit 5   # only first 5 issues (for testing)
#
# Implementation note: delegates JSON parsing to `node` (preferred)
# or `python3` (fallback). `jq` isn't required.

set -euo pipefail

LIMIT=0
DRY_RUN=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --limit)   shift; LIMIT="${1:-0}" ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

PARSER=""
if command -v node >/dev/null 2>&1; then PARSER="node"
elif command -v python3 >/dev/null 2>&1 || command -v python >/dev/null 2>&1; then PARSER="python"
else echo "ERROR: need node or python for JSON parsing" >&2; exit 2
fi

if [[ $DRY_RUN -eq 0 ]]; then
  ./scripts/sync-labels.sh >/dev/null
fi

ISSUES_JSON=$(gh issue list --state open --limit 500 \
  --json number,title,body,labels 2>/dev/null)

if [[ -z "$ISSUES_JSON" || "$ISSUES_JSON" == "[]" ]]; then
  echo "No open issues found."
  exit 0
fi

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

if [[ "$PARSER" == "node" ]]; then
  OUTPUT=$(echo "$ISSUES_JSON" | node "$SCRIPT_DIR/backfill-labels.node.mjs")
else
  PY=$(command -v python3 || command -v python)
  OUTPUT=$(echo "$ISSUES_JSON" | "$PY" "$SCRIPT_DIR/backfill-labels.py")
fi

ASSIGNED=0
SKIPPED=0
LIMIT_HIT=0

while IFS='|' read -r action num area pri title; do
  [[ "$action" == "SKIP" ]] && {
    [[ $DRY_RUN -eq 1 ]] && echo "  #$num  SKIP (already labeled): $title"
    SKIPPED=$((SKIPPED + 1))
    continue
  }

  if [[ $DRY_RUN -eq 1 ]]; then
    echo "  #$num  $area + $pri  ::  $title"
    ASSIGNED=$((ASSIGNED + 1))
  else
    if gh issue edit "$num" --add-label "$area" --add-label "$pri" >/dev/null 2>&1; then
      echo "  #$num  $area + $pri  applied  ::  $title"
      ASSIGNED=$((ASSIGNED + 1))
    else
      echo "  #$num  FAILED to apply: $title" >&2
    fi
  fi

  if [[ $LIMIT -gt 0 ]]; then
    LIMIT=$((LIMIT - 1))
    if [[ $LIMIT -eq 0 ]]; then
      LIMIT_HIT=1
      break
    fi
  fi
done <<< "$OUTPUT"

if [[ $LIMIT_HIT -eq 1 ]]; then
  echo "(hit --limit, stopping)"
fi

echo
echo "Assigned: $ASSIGNED"
echo "Skipped:  $SKIPPED"

if [[ $DRY_RUN -eq 1 ]]; then
  echo
  echo "Re-run without --dry-run to apply."
fi