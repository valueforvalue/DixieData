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
  echo "DRY RUN - no changes will be made"
  echo
fi

# Label spec: name|color|description
# Keep alphabetical within each axis for diff stability.
# Total target: ~35 labels (Type x4, Status x4, Area x13, Priority x3,
# Cohort x1, Meta x7, In-place safety x2, Release-process x1).
# If you find yourself adding more, prune first.
LABELS=(
  # --- Type (what's the work) ---
  "bug|d73a4a|Something isn't working"
  "documentation|0075ca|Improvements or additions to documentation"
  "enhancement|a2eeef|New feature or request"
  "reference-doc|0E8A16|Long-lived reference doc, not active work"

  # --- Status (where in triage) ---
  "needs-triage|D4C5F9|Maintainer needs to evaluate this issue (or is waiting on reporter)"
  "needs-info|FBCA04|Reporter must supply more information before triage can proceed"
  "ready-for-agent|0E8A16|Fully specified, ready for an AFK agent"
  "ready-for-human|1D76DB|Requires human implementation"

  # --- Area (which part of the system) ---
  "area:backend|5319e7|Go backend (appshell handlers, services, archive)"
  "area:build|5319e7|Build pipeline (Makefile, scripts/, release pipeline, versioninfo)"
  "area:ci|5319e7|GitHub Actions workflows, race gates, CI plumbing"
  "area:cli|5319e7|Headless CLI subcommand (dixiedata <cmd>)"
  "area:db|5319e7|SQLite schema, migrations, query layer"
  "area:debug|5319e7|Debug harness, trace instrumentation, slog"
  "area:docs|5319e7|Repository docs (CONTEXT.md, agents/, adr/)"
  "area:export|5319e7|Export surface (PDF, JPG, JSON, CSV, iCal, archive)"
  "area:frontend|5319e7|Frontend JS, CSS, htmx, templates/, UX/microcopy"
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
  "deferred|bfd4f2|Held until a future trigger; reopen conditions documented in the issue body"
  "duplicate|cfd3d7|This issue or pull request already exists"
  "good first issue|7057ff|Good for newcomers"
  "help wanted|008672|Extra attention is needed"
  "invalid|e4e669|This doesn't seem right"
  "question|d876e3|Further information is requested"
  "wontfix|ffffff|This will not be worked on"

  # --- In-place update safety (build-protocol.md §5) ---
  "safe-for-in-place|0E8A16|Reviewed against the 4 safety rules; safe for in-place update on main"
  "unsafe-for-in-place|B60205|Intentional destructive change; ships via full re-install + restore-point only"

  # --- Release-process exemptions ---
  "release-counter-exempt|fbca04|PR may skip the CurrentAppVersionInt bump gate (rare; docs-only releases, see RELEASING.md)"

  # --- Target branch routing (ADR 0011, added 2026-07-26) ---
  "target:dev|c5def5|New feature or bug found during normal development; lands on dev for the next release (the default)"
  "target:rc|5319e7|Stabilization fix blocking a release-in-progress; PR opens against the active rc/v* branch"
  "target:stable|b60205|Urgent hotfix on the released-code home after a promote (rare; slipped past make promote)"

  # --- RC branch policy (ADR 0011) ---
  "release-blocker|D93F0B|Bug fix or stabilization change eligible to land on the rc/v* branch (Zephyr-style feature freeze). Required on every PR to rc/v* together with target:rc."
)

CHANGED=0
UNCHANGED=0
CREATED=0
DELETED=0

for entry in "${LABELS[@]}"; do
  IFS='|' read -r name color desc <<< "$entry"

  # Use plain-text label listing -- `gh label list --json` silently drops
  # labels in some CLI versions; the text path is reliable.
  if [[ $DRY_RUN -eq 1 ]]; then
    if gh label list --limit 200 2>/dev/null | awk -F'\t' '{print $1}' | grep -qxF "$name"; then
      echo "  exists: $name"
    else
      echo "  would create: $name (#$color)"
    fi
    continue
  fi

  if gh label list --limit 200 2>/dev/null | awk -F'\t' '{print $1}' | grep -qxF "$name"; then
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

# Delete labels that are no longer in the spec (one-off cleanup).
# Idempotent: missing labels exit 0 silently.
if [[ $DRY_RUN -eq 0 ]]; then
  for stale_name in blocked; do
    if gh label list --limit 200 2>/dev/null | awk -F'\t' '{print $1}' | grep -qxF "$stale_name"; then
      if gh label delete "$stale_name" --yes >/dev/null 2>&1; then
        DELETED=$((DELETED + 1))
        echo "  deleted: $stale_name (no longer in spec)"
      else
        echo "  FAILED to delete: $stale_name" >&2
        exit 1
      fi
    fi
  done
fi

if [[ $DRY_RUN -eq 0 ]]; then
  echo
  echo "Labels created: $CREATED"
  echo "Labels updated: $UNCHANGED"
  echo "Labels deleted: $DELETED"
fi