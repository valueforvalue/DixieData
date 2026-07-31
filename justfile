# Canonical command layer. PowerShell and Go remain implementation primitives.

# Shared generated inputs. Build profiles keep their own flags and packaging.
generate:
    go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate
    go run ./scripts/bake-release-notes
    go run ./scripts/bake-activity
    npm run build:css

tpl: generate

release-notes-bake:
    go run ./scripts/bake-release-notes

activity-history-bake:
    go run ./scripts/bake-activity

css:
    npm run build:css

# Windows/Wails debug build plus sibling binaries required by local smoke/audit flows.
debug: generate
    pwsh -NoLogo -NoProfile -File scripts/probe-clean.ps1
    pwsh -NoLogo -NoProfile -File scripts/build-debug.ps1
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/dixiedata-web.exe ./cmd/dixiedata-web
    pwsh -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "& './scripts/bundle-web-assets.ps1' -Root (Get-Location).Path"
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/seed-data.exe ./cmd/seed-data
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/gold-master.exe ./cmd/gold-master
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force tools/tune/bin | Out-Null"
    pwsh -NoLogo -NoProfile -Command "Set-Location tools/tune; go build -tags debug -o bin/dixiedata-tune.exe ."

build: debug

web:
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/dixiedata-web.exe ./cmd/dixiedata-web
    pwsh -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "& './scripts/bundle-web-assets.ps1' -Root (Get-Location).Path"

seed:
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/seed-data.exe ./cmd/seed-data

gold:
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/gold-master.exe ./cmd/gold-master

tune-bin:
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force tools/tune/bin | Out-Null"
    pwsh -NoLogo -NoProfile -Command "Set-Location tools/tune; go build -tags debug -o bin/dixiedata-tune.exe ."

# Interactive Wails development stays uncaptured by design.
dev:
    wails dev

run:
    pwsh -NoLogo -NoProfile -File scripts/run-debug.ps1

release:
    pwsh -NoLogo -NoProfile -Command "if (\$env:DIXIEDATA_RELEASE_TAG) { & pwsh -NoLogo -NoProfile -File scripts/build-release.ps1 -LDFlags \"-X github.com/valueforvalue/DixieData/internal/versioninfo.CurrentReleaseTag=\$env:DIXIEDATA_RELEASE_TAG\" } else { & pwsh -NoLogo -NoProfile -File scripts/build-release.ps1 }"

archive:
    pwsh -NoLogo -NoProfile -Command "if (\$env:DIXIEDATA_RELEASE_TAG) { & pwsh -NoLogo -NoProfile -File scripts/build-release.ps1 -Archive -LDFlags \"-X github.com/valueforvalue/DixieData/internal/versioninfo.CurrentReleaseTag=\$env:DIXIEDATA_RELEASE_TAG\" } else { & pwsh -NoLogo -NoProfile -File scripts/build-release.ps1 -Archive }"

demo:
    pwsh -NoLogo -NoProfile -File scripts/build-demo-release.ps1

bump:
    pwsh -NoLogo -NoProfile -File scripts/bump-version.ps1 -BumpSchema

release-github:
    pwsh -NoLogo -NoProfile -File scripts/release-github.ps1

stress:
    pwsh -NoLogo -NoProfile -File scripts/run-stress-tests.ps1

goldmaster:
    pwsh -NoLogo -NoProfile -File tests/goldmaster/run-suite.ps1

test:
    go test ./... -short -count=1
    pwsh -NoLogo -NoProfile -Command "Set-Location tools/tune; go test -short -count=1"

test-quiet: test

# Freshness validates all debug subtools and the desktop smoke path.
freshness: debug
    pwsh -NoLogo -NoProfile -Command "if (-not (Test-Path build/bin/dixiedata-web.exe)) { throw 'missing dixiedata-web.exe' }; if (-not (Test-Path build/bin/seed-data.exe)) { throw 'missing seed-data.exe' }; if (-not (Test-Path build/bin/gold-master.exe)) { throw 'missing gold-master.exe' }; if (-not (Test-Path tools/tune/bin/dixiedata-tune.exe)) { throw 'missing dixiedata-tune.exe' }"
    build/bin/dixiedata-web.exe --help
    build/bin/seed-data.exe -h
    build/bin/gold-master.exe -h
    tools/tune/bin/dixiedata-tune.exe -h
    build/bin/DixieData.exe --smoke --json

clean-generated:
    pwsh -NoLogo -NoProfile -Command "Remove-Item internal/templates/*_templ.go,internal/releasehistory/baked.go,internal/activityhistory/baked.go -Force -ErrorAction SilentlyContinue"

verify-fresh-bake: clean-generated generate test

verify-clean: verify-fresh-bake

# Lint probes retain existing stable names.
lint-htmx-guard:
    node audit/discover_htmx_guard.mjs
lint-htmx-guard-strict:
    node audit/discover_htmx_guard.mjs --strict
lint-htmx-guard-test:
    node audit/discover_htmx_guard.test.mjs
lint-bake-bootstrap:
    node audit/lint_bake_bootstrap.mjs
lint-bake-bootstrap-strict:
    node audit/lint_bake_bootstrap.mjs --strict
lint-bake-bootstrap-test:
    node --test audit/lint_bake_bootstrap.test.mjs

# verify-embed-tree (issue #686): assert every frontend/** file
# referenced by index.html is reachable via //go:embed frontend.
# Catches the file-skip-by-prefix class from Go's embed package
# (the 12f1834a article Preview button bug shape).
verify-embed-tree:
    node audit/verify_embed_tree.mjs
verify-embed-tree-strict:
    node audit/verify_embed_tree.mjs --strict
verify-embed-tree-test:
    node audit/verify_embed_tree.test.mjs

# lint-no-nested-forms (issue #682): walks every .templ file
# and asserts that no <form> tag opens while another <form> is
# still on the element stack. HTML5 forbids <form> inside <form>;
# the parser silently closes the outer form at the inner form's
# open tag, reparenting submit buttons out of the outer form's
# DOM tree. The canonical historical instance: the inner image-
# upload form at entry_form.templ:392 (and soldier_card.templ:574)
# silently closed the outer form, breaking the Save Changes /
# Download Selected Images buttons.
lint-no-nested-forms:
    node audit/lint_no_nested_forms.mjs
lint-no-nested-forms-strict:
    node audit/lint_no_nested_forms.mjs --strict
lint-no-nested-forms-test:
    node audit/lint_no_nested_forms.test.mjs

# lint-all-frontend: aggregate gate that runs all the frontend
# lint gates in one shot. Used by CI as the PR-time frontend
# invariant check.
lint-all-frontend:
    just lint-htmx-guard
    just lint-bake-bootstrap
    just verify-embed-tree
    just lint-no-nested-forms
    just lint-js-init-guards
    just lint-button-actions-resolve
    just lint-orphan-handlers

# Aggregate test target: runs every node-based probe test suite.
# Issue #702 closed: lint-orphan-handlers-test is wired in. The
# pre-#700 unit test was broken (expected stale probe output +
# a regex that dropped the drive-letter colon). The 4de0ee4
# rewrite now pins the current contract: 6 assertions covering
# the summary header lines, the invoker count, the --strict
# exit-code branch, and the positive-coverage tripwires for
# /soldiers/{id}/tags (issue #256 fix) + /export/json
# (canonical "shipped but invisible" regression net).
test-lint:
    just lint-htmx-guard-test
    just lint-bake-bootstrap-test
    just verify-embed-tree-test
    just lint-no-nested-forms-test
    just lint-js-init-guards-test
    just lint-button-actions-resolve-test
    just lint-so-reuseaddr-test
    just lint-orphan-handlers-test
    just lint-foldout-trigger-marker-test
lint-dispatcher-tdz-test:
    node --test audit/dispatcher_tdz_fix.test.mjs

# Issue #683: dispatchDixieDataForm body-stripping regression
# net (issue #428, AGENTS.md §Wails runtime hazards, quirks
# 1 + 2). Two workarounds live in the dispatcher:
#   1. PATCH/PUT/DELETE → POST + X-HTTP-Method-Override when
#      the request is going to wails.localhost. Plain-Chromium
#      requests (audit harness) keep the real PATCH so the
#      Playwright probes still see the genuine method.
#   2. FormData → URLSearchParams.toString() so the Wails asset
#      server delivers the body to the Go handler.
# Both fixes are pinned by this 7-assertion probe; future
# refactors that drop a workaround fail the probe at editor
# time. Wired into CI by test.yml as a PR-time gate.
lint-dispatcher-patch-method:
    node audit/dispatcher_patch_method.test.mjs

# lint-js-init-guards (issue #685): walks frontend/app.js for
# the 13 initializers dispatched by initializeDynamicContent
# and asserts each one has a __<feature>Wired/Bound/Installed
# sentinel inside its first ~40 lines. Prevents the
# htmx-swap-re-binding bug class (fixes 87645011 + c0d89681)
# from returning. Pairs with lint-no-nested-forms and
# verify-embed-tree as the third editor-level sweep of the
# audit-fallout cohort.
lint-js-init-guards:
    node scripts/lint-js-init-guards.mjs
lint-js-init-guards-strict:
    node scripts/lint-js-init-guards.mjs --strict
lint-js-init-guards-test:
    node --test scripts/lint-js-init-guards.test.mjs

# discover-foldout-trigger-marker (issue #704): walks
# frontend/, internal/, audit/, docs/ for any
# `data-article-md-cheatsheet-open` literal (the stale
# early-#565 wireframe marker that does not exist in the
# rendered HTML; the cheatsheet uses the Foldout primitive's
# `data-foldout-trigger="<menuID>"` contract). Path-based
# exclusions whitelist the probe's own files + the corrected
# wireframe row + the Foldout unit test (which intentionally
# references the marker as a negative-control assertion).
# Comment-only mentions are stripped before matching. In
# strict mode exits 1 on any offender.
lint-foldout-trigger-marker:
    node audit/discover_foldout_trigger_marker.mjs
lint-foldout-trigger-marker-strict:
    node audit/discover_foldout_trigger_marker.mjs --strict
lint-foldout-trigger-marker-test:
    node --test audit/discover_foldout_trigger_marker.test.mjs

# lint-button-actions-resolve (issue #687): walks every
# .templ file for invoker URLs (form action, data-action,
# hx-get/post/put/patch/delete, Sprintf templates) and
# asserts each one resolves to a route registered in
# internal/appshell/routes.go OR matches the allowlist
# (external / templ.SafeURL + htmx dev paths). Catches
# the template-split button URL drift class (fixes
# 69eb735f + 266db08c — fictional /share/feedback-log
# URLs).
lint-button-actions-resolve:
    node scripts/lint-button-actions-resolve.mjs
lint-button-actions-resolve-strict:
    node scripts/lint-button-actions-resolve.mjs --strict
lint-button-actions-resolve-test:
    node --test scripts/lint-button-actions-resolve.test.mjs

# discover_orphan_handlers (audit/discover_orphan_handlers.mjs):
# walks internal/appshell/routes.go + every .templ file +
# every generated *_templ.go to confirm every registered
# handler has at least one templ/data-action invoker. Catches
# the "handler returns 200 but renders nothing" bug class.
# Triplet added in #700 slice 5 to mirror the convention used
# by the other 3 scanners (lint-no-nested-forms,
# lint-button-actions-resolve, lint-js-init-guards). The unit
# test audit/discover_orphan_handlers_test.mjs already exists.
lint-orphan-handlers:
    node audit/discover_orphan_handlers.mjs
lint-orphan-handlers-strict:
    node audit/discover_orphan_handlers.mjs --strict
lint-orphan-handlers-test:
    node --test audit/discover_orphan_handlers.test.mjs

# Issue #668 / ADR 0011: commit-type classifier for PRs
# targeting `rc/v*` branches. Walk every commit between
# BASE_REF..HEAD_REF, reject disallowed types
# (feat/refactor/perf/build), missing-type commits, and
# oversized diffs (≥ 50 files per ADR §Diff-size gate).
lint-rc-commits:
    BASE_REF="${BASE_REF:-origin/dev}" HEAD_REF="${HEAD_REF:-HEAD}" node scripts/ci/lint-rc-commits.mjs
lint-rc-commits-test:
    node --test scripts/ci/lint-rc-commits.test.mjs
lint-typecheck-augmentations-test:
    node --test audit/typecheck_augmentations.test.mjs
lint-no-bare-catch:
    npm run lint:js
lint-typecheck:
    npm run typecheck

lint-dialog-guard:
    node audit/smoke_dialog_guard.mjs
lint-dialog-guard-strict:
    node audit/smoke_dialog_guard.mjs --strict
lint-dialog-guard-test:
    node --test audit/smoke_dialog_guard.test.mjs
# Issue #708: regression net for SO_REUSEADDR on the smoke
# server listener. Boots two dixiedata-web processes on the
# same port sequentially and asserts the second one comes up
# without a bind error. Requires build/bin/dixiedata-web.exe
# to exist (run `just web` first on local Windows).
lint-so-reuseaddr-test:
    node --test audit/probe_so_reuseaddr.test.mjs
lint-microcopy:
    node audit/smoke_microcopy.mjs
lint-microcopy-strict:
    node audit/smoke_microcopy.mjs --strict
lint-microcopy-test:
    node audit/smoke_microcopy.test.mjs
lint-static-archive-microcopy-strict:
    node audit/smoke_static_archive_microcopy.mjs --strict
lint-static-archive-microcopy-test:
    node audit/smoke_static_archive_microcopy.test.mjs
lint-pdf-microcopy-strict:
    node audit/smoke_pdf_microcopy.mjs --strict
lint-pdf-microcopy-test:
    node audit/smoke_pdf_microcopy.test.mjs
lint-icalendar-microcopy-strict:
    node audit/smoke_icalendar_microcopy.mjs --strict
lint-icalendar-microcopy-test:
    node audit/smoke_icalendar_microcopy.test.mjs
lint-runtime-microcopy-strict:
    node audit/smoke_runtime_microcopy.mjs --strict
lint-runtime-microcopy-test:
    node audit/smoke_runtime_microcopy.test.mjs

# Issue #700: shared Playwright smoke runner. The aggregator
# walks audit/_lib/smoke_index.mjs::SURFACES[] and dispatches
# each entry to either kind: 'playwright' (audit/_lib/
# smoke_runner.mjs::runProbe) or kind: 'scanner' (spawnSync
# the existing class-2/4/6/8/9 CLI probes). Slice 1 (this
# commit) ships the skeleton + aggregator stub; SURFACES[]
# is empty so the runner exits 0 with no assertions.
# Subsequent slices migrate smoke_soldier_images /
# smoke_submit_e2e / smoke_mega_menu_nav onto the runner.
# The aggregator auto-discovers a built build/bin/
# dixiedata-web{,.exe}; on Linux that path is the bare
# dixiedata-web (the .github/workflows/audit.yml step
# already builds it; local Windows devs run `just debug`).
test-smoke:
    node audit/smoke_aggregator.mjs
test-smoke-strict:
    node audit/smoke_aggregator.mjs --strict
test-smoke-test:
    node --test audit/_lib/smoke_runner.test.mjs

# Existing lint aggregate. Individual recipes remain independently runnable.
lint: lint-htmx-guard-strict lint-htmx-guard-test lint-bake-bootstrap-strict lint-bake-bootstrap-test lint-dialog-guard-strict lint-dialog-guard-test lint-microcopy-strict lint-microcopy-test lint-static-archive-microcopy-strict lint-static-archive-microcopy-test lint-pdf-microcopy-strict lint-pdf-microcopy-test lint-icalendar-microcopy-strict lint-icalendar-microcopy-test lint-runtime-microcopy-strict lint-runtime-microcopy-test lint-no-bare-catch lint-typecheck verify-embed-tree-strict verify-embed-tree-test lint-no-nested-forms-strict lint-no-nested-forms-test lint-dispatcher-patch-method lint-js-init-guards-strict lint-js-init-guards-test lint-button-actions-resolve-strict lint-button-actions-resolve-test lint-foldout-trigger-marker

tune:
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force tools/tune/bin | Out-Null"
    pwsh -NoLogo -NoProfile -Command "Set-Location tools/tune; go build -o bin/dixiedata-tune.exe ."

tune-smoke:
    pwsh -NoLogo -NoProfile -Command "if (-not (Test-Path .dixiedata)) { throw 'no .dixiedata/ directory; run the appshell once first' }; Set-Location tools/tune; go build -o bin/dixiedata-tune.exe .; Set-Location ../..; ./tools/tune/bin/dixiedata-tune.exe --db .dixiedata render --template bulk_soldier --mode bulk --out (Join-Path (Get-Location) 'build/log/tune-smoke.pdf')"

render-round:
    pwsh -NoLogo -NoProfile -File scripts/render-round.ps1 -Round 1

render-round-ONE ROUND='1' SURFACE='single-soldier-landscape' KEEP='1' RECORD='1':
    pwsh -NoLogo -NoProfile -File scripts/render-round.ps1 -Round {{ROUND}} -Only {{SURFACE}} -KeepRounds {{KEEP}} -Record {{RECORD}}

update-snapshots-ONE:
    pwsh -NoLogo -NoProfile -Command "$env:UPDATE_SNAPSHOTS='1'; go test -count=1 -run ('TestArchiveContractSnapshots/' + $env:SURFACE) ./internal/exportcontract/ -timeout 120s"

tune-snapshots:
    pwsh -NoLogo -NoProfile -Command "$env:UPDATE_SNAPSHOTS='1'; go test -count=1 ./internal/exportcontract/ -run 'TestArchiveContractSnapshots|TestCLIContractSnapshots' -timeout 600s"

render-svg:
    pwsh -NoLogo -NoProfile -Command "if (Test-Path /c/Users/value/bin/render-svg.sh) { /c/Users/value/bin/render-svg.sh all 4 } else { Write-Host 'render-svg helper unavailable; skipping' }"

lint-migration-columns:
    node audit/smoke_migration_columns.mjs
lint-defer-close:
    pwsh -NoLogo -NoProfile -Command "Set-Location tools/lintrules; go build -o bin/lintrules.exe ./cmd/lintrules; Set-Location ../..; New-Item -ItemType Directory -Force build/log | Out-Null; go vet -vettool=tools/lintrules/bin/lintrules.exe ./..."
lint-bare-templ-render:
    pwsh -NoLogo -NoProfile -Command "Set-Location tools/lintrules; go build -o bin/lintrules.exe ./cmd/lintrules; Set-Location ../..; go vet -vettool=tools/lintrules/bin/lintrules.exe ./..."
lint-swallowed-errors: lint-defer-close lint-bare-templ-render lint-no-bare-catch
probe-dispatcher-contract:
    node audit/probe-dispatcher-contract.mjs
probe-error-surfaces:
    node audit/probe-error-surfaces.mjs

promote-dry-run:
    bash scripts/promote-gate-chain.sh
    bash scripts/promote-preflight.sh
    @echo ""
    @echo "promote-dry-run: gates passed; safe to run 'just promote'"
promote: promote-dry-run
    bash scripts/promote-preflight.sh
    @if ! git diff --quiet origin/${STABLE_BRANCH:-stable}..origin/dev 2>/dev/null; then echo "promote: ABORTED — dev has commits ${STABLE_BRANCH:-stable} doesn't have. Run 'just promote-prep'."; exit 1; fi
    @echo "promote: dev and ${STABLE_BRANCH:-stable} are in sync."
    @echo ""
    @echo "=== opening PR dev -> ${STABLE_BRANCH:-stable} ==="
    @bash scripts/promote-open-pr.sh ${STABLE_BRANCH:-stable}
promote-prep:
    bash scripts/promote-prep.sh
promote-confirm:
    bash scripts/promote-confirm.sh

changelog-archive:
    pwsh -NoLogo -NoProfile -File scripts/archive-changelog.ps1

audit-build:
    go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate
    go run ./scripts/bake-release-notes
    go run ./scripts/bake-activity
    bash -euc 'mkdir -p build/bin && go build -o build/bin/dixiedata-web ./cmd/dixiedata-web && go build -o build/bin/seed-data ./cmd/seed-data'

audit:
    npm run audit

clean:
    pwsh -NoLogo -NoProfile -Command "Remove-Item build -Recurse -Force -ErrorAction SilentlyContinue"

log-clean:
    pwsh -NoLogo -NoProfile -Command "Remove-Item build/log -Recurse -Force -ErrorAction SilentlyContinue"

probe-clean:
    pwsh -NoLogo -NoProfile -File scripts/probe-clean.ps1

contract-test:
    node --test audit/just_contract.test.mjs

# RC cohort manifest publisher (issue #654). Generates the
# manifest entry for a newly-built RC zip. Operator pastes
# the output into the dixiedata-rc-manifest repo's
# manifest.json and commits. After this lands in the
# manifest repo, the cohort's updater (pointed at the

# Manifest repo update is in tmp/manifest-repo/manifest.json.
# Usage:
#   just rc-publish ZIP=release/DixieData-release-v1.1.4-rc1.zip
rc-publish ZIP='':
    @powershell -NoLogo -NoProfile -File scripts/rc-publish.ps1 -Zip "{{ZIP}}"
