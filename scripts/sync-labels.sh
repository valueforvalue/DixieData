#!/usr/bin/env bash
# scripts/sync-labels.sh
#
# Idempotently create or update the DixieData issue label set.
# Reads the label spec from this script and applies it via `gh label`.
# Safe to run repeatedly; existing labels are updated (name preserved,
# color + description overwritten with the spec).
#
# Usage:
#   ./scripts/sync-labels.sh              # sync the standard set
#   ./scripts/sync-labels.sh --dry-run    # print what would change
#
# Requires: gh CLI authenticated against the DixieData repo.

set -euo pipefail

DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
  echo "DRY RUN — no changes will be made"
  echo
fi

# Label spec: name|color|description
# Keep alphabetical within each axis for diff stability.
LABELS=(
  # --- Type (what's the work) ---
  "bug|d73a4a|Something isn't working"
  "documentation|0075ca|Improvements or additions to documentation"
  "enhancement|a2eeef|New feature or request"

  # --- Status (where in triage) ---
  "needs-triage|D4C5F9|Maintainer needs to evaluate this issue"
  "needs-info|FBCA04|Waiting on reporter for more information"
  "ready-for-agent|0E8A16|Fully specified, ready for an AFK agent"
  "ready-for-human|1D76DB|Requires human implementation"

  # --- Area (which part of the system) ---
  "area:backend|5319e7|Go backend (appshell handlers, services, archive)"
  "area:cli|5319e7|Headless CLI subcommand (dixiedata <cmd>)"
  "area:db|5319e7|SQLite schema, migrations, query layer"
  "area:debug|5319e7|Debug harness, trace instrumentation, slog"
  "area:docs|5319e7|Repository docs (CONTEXT.md, agents/, adr/, migrations/)"
  "area:export|5319e7|Export surface (PDF, JPG, JSON, CSV, iCal, archive)"
  "area:frontend|5319e7|Frontend JS, CSS, htmx, templates/"
  "area:import|5319e7|Import surface (.ddbak, .ddshare, images, memorial-json)"
  "area:share|5319e7|/share screen (exports, imports, sync subpages)"
  "area:tags|5319e7|Tag system (Person Record free-text labels)"
  "area:templates|5319e7|Templ HTML templates + Typst PDF templates"

  # --- Priority (how urgent) ---
  "priority:high|b60205|Known regression, lost-data bug, or active user blocker"
  "priority:low|cccccc|Polish or speculative work"
  "priority:medium|fbca04|Default priority"

  # --- Cohort (which batch) ---
  "audit-fallout|B60205|Issues discovered by a full audit sweep (see CHANGELOG)"

  # --- Meta (process state) ---
  "blocked|cccccc|Held by another issue; cannot proceed until unblocked"
  "duplicate|cfd3d7|This issue or pull request already exists"
  "good first issue|7057ff|Good for newcomers"
  "help wanted|008672|Extra attention is needed"
  "invalid|e4e669|This doesn't seem right"
  "question|d876e3|Further information is requested"
  "wontfix|ffffff|This will not be worked on"
)

CHANGED=0
UNCHANGED=0
CREATED=0

for entry in "${LABELS[@]}"; do
  IFS='|' read -r name color desc <<< "$entry"

  if [[ $DRY_RUN -eq 1 ]]; then
    if gh label list --json name --jq '.[].name' | grep -qxF "$name"; then
      echo "  exists: $name"
    else
      echo "  would create: $name (#$color)"
    fi
    continue
  fi

  if gh label list --json name --jq '.[].name' | grep -qxF "$name"; then
    # Update color + description; preserve existing name
    if gh label edit "$name" --color "$color" --description "$desc" >/dev/null 2>&1; then
      UNCHANGED=$((UNCHANGED + 1))
    else
      echo "  FAILED to update: $name" >&2
      exit 1
    fi
  else
    if gh label create "$name" --color "$color" --description "$desc" >/dev/null 2>&1; then
      CREATED=$((CREATED + 1))
      echo "  created: $name"
    else
      echo "  FAILED to create: $name" >&2
      exit 1
    fi
  fi
done

if [[ $DRY_RUN -eq 0 ]]; then
  echo
  echo "Labels created: $CREATED"
  echo "Labels updated: $UNCHANGED"
fi