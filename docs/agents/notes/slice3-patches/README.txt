#!/usr/bin/env bash
# Generate slice-3 decomposition patches in the current directory.
# Eight logical groups; 7 patch files emitted (chunk 5 is documented
# in slice3-decomposition.md but is not a separate file because it
# shares events_handlers.go + routes.go with chunk 4).
#
# Run from the repo root: `bash docs/agents/notes/slice3-patches/README.txt`
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
PARENT=a363b6d
SOURCE=d09e852

emit_patch() {
  local name="$1"; shift
  local subject="$1"; shift
  local files=("$@")
  local out="$REPO_ROOT/docs/agents/notes/slice3-patches/$name.patch"
  {
    printf 'From: Jeremy Morris <valueforvalue76@gmail.com>\n'
    printf 'Date: Sat Jul 4 13:00:00 2026 -0500\n'
    printf 'Subject: %s\n' "$subject"
    printf 'X-Slice3-Chunk: %s\n' "$name"
    printf '\n'
    printf 'Slice 3 decomposition patch (issue #326). See\n'
    printf 'docs/agents/notes/slice3-decomposition.md for the 8-group\n'
    printf 'decomposition rationale. This patch is the source-level diff\n'
    printf 'for the files in group %s, extracted from commit %s.\n\n' "$name" "$SOURCE"
    git -C "$REPO_ROOT" -c color.ui=never diff "$PARENT".."$SOURCE" -- "${files[@]}"
  } > "$out"
  echo "Wrote $out ($(wc -l < "$out") lines)"
}

emit_patch 01-foundation \
  '[events] slice 3 / 1: foundation (viewmodel + facade + DeleteEvent)' \
  internal/appshell/app.go \
  internal/appshell/app_facades.go \
  internal/viewmodel/types.go \
  internal/viewmodel/mappers.go \
  internal/records/event_service.go

emit_patch 02-routebuilder \
  '[events] slice 3 / 2: routebuilder Event URL helpers' \
  internal/routebuilder/routebuilder.go

emit_patch 03-presentation \
  '[events] slice 3 / 3: presentation Event* adapters' \
  internal/presentation/views.go

emit_patch 04-handlers-list-new-detail-edit \
  '[events] slice 3 / 4: handlers + routes - events CRUD' \
  internal/appshell/events_handlers.go \
  internal/appshell/events_handlers_test.go \
  internal/appshell/routes.go



emit_patch 06-event-list-templ \
  '[events] slice 3 / 6: event_list.templ' \
  internal/templates/event_list.templ

emit_patch 07-event-form-templ \
  '[events] slice 3 / 7: event_form.templ' \
  internal/templates/event_form.templ

emit_patch 08-detail-nav-filter-jsgate \
  '[events] slice 3 / 8: event_detail + browse filter + layout + JS gate' \
  internal/templates/event_detail.templ \
  internal/templates/browse.templ \
  internal/templates/soldier_card.templ \
  internal/templates/layout.templ \
  internal/templates/entry_form_helpers.go \
  frontend/app.js

cat > "$REPO_ROOT/docs/agents/notes/slice3-patches/05-handlers-attach-detach-quickadd-tab.README.txt" <<'EOF2'
Chunk 5 ("handlers/routes /soldiers/{id}/events/* M-to-M") is documented
in docs/agents/notes/slice3-decomposition.md §"The 8 logical groups"
group 5. It shares two source files (internal/appshell/events_handlers.go
and internal/appshell/routes.go) with chunk 4; both files are emitted in
full by chunk 4's patch. To apply chunk 5's handlers in isolation, use
`git log -L <funcname>:internal/appshell/events_handlers.go d09e852~1..d09e852`
per handler function and emit individual handler-scoped patches.
EOF2

echo
echo "7 patches emitted (chunks 1, 2, 3, 4, 6, 7, 8 + chunk 5 README)"
