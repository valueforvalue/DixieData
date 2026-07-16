# DixieData Makefile — token-saver quiet-output wrappers.
#
# Strategy: native quiet flags + log redirection. Verbose streams go to
# build/log/<target>.log so LLM agents see exit status, not noise.
#
# Targets mirror existing PowerShell scripts:
#   make <target>  ==  pwsh -File scripts/<script>.ps1 [args]

PWSH  := pwsh -NoLogo -NoProfile
LOGDIR := build/log



.DEFAULT_GOAL := help

.PHONY: help build debug release archive demo run dev test test-quiet \
        stress goldmaster tune tune-smoke tune-snapshots tune-bin \
        web seed gold render-round render-round-ONE update-snapshots-ONE \
        render-svg tpl css audit clean log-clean bump release-github \
        probe-clean freshness release-pipeline cli-coverage changelog-archive

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

# --- Build targets (mirror scripts/build-*.ps1) ---

# Internal recipe — pipe pwsh output to log, abort on non-zero exit.
# Use `tee` (not just `>`) so human-running-make sees status in terminal too.
# `set -o pipefail` ensures pwsh's non-zero exit propagates through the pipe to tee.
LOG_RECIPE = @mkdir -p $(LOGDIR) && \
	bash -c 'set -o pipefail; $(PWSH) -File $(SCRIPT) $(ARGS) 2>&1 | tee $(LOGDIR)/$(TARGET).log' && \
	rm -f $(LOGDIR)/$(TARGET).log.tmp

# Debug chain: `make debug` builds the Wails desktop binary
# (scripts/build-debug.ps1) PLUS every sibling binary that the
# debug workflow expects to be present (audit smoke harness
# needs dixiedata-web + seed-data; the render/tune workflow
# needs dixiedata-tune; the gold-master suite needs the
# gold-master binary). Without these the user runs `make
# debug`, opens the app, hits a button, and the harness smoke
# test fails because the web server binary isn't in build/bin/.
# The recipes below inline the chain (issue #366) rather than
# recursing through `$(MAKE)`, which GNUWin32 make misparses
# when `$(MAKE)` itself lives under "Program Files (x86)" —
# the parens break sh's tokenization and the inner make exits
# with `e=87`. Inlining the chain as direct `go build` calls
# sidesteps the bug entirely and removes a layer of indirection
# for everyone, regardless of make variant.
WEB_BIN := build/bin/dixiedata-web.exe
SEED_BIN := build/bin/seed-data.exe
GOLD_BIN := build/bin/gold-master.exe
TUNE_BIN := tools/tune/bin/dixiedata-tune.exe

build: SCRIPT := scripts/build-debug.ps1
build: TARGET := build
build: ARGS :=
build: ## Build via scripts/build-debug.ps1; chains web+seed+gold+tune-bin
	@$(PWSH) -NoLogo -NoProfile -File scripts/probe-clean.ps1
	$(LOG_RECIPE)
	@mkdir -p build/bin
	go build -tags debug -o $(WEB_BIN) ./cmd/dixiedata-web
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/bundle-web-assets.ps1 $(PWD)
	@mkdir -p build/bin
	go build -tags debug -o $(SEED_BIN) ./cmd/seed-data
	@mkdir -p build/bin
	go build -tags debug -o $(GOLD_BIN) ./cmd/gold-master
	@mkdir -p tools/tune/bin
	cd tools/tune && go build -tags debug -o bin/dixiedata-tune.exe .

debug: SCRIPT := scripts/build-debug.ps1
debug: TARGET := debug
debug: ARGS :=
debug: ## Debug build via scripts/build-debug.ps1; chains web+seed+gold+tune-bin
	@$(PWSH) -NoLogo -NoProfile -File scripts/probe-clean.ps1
	$(LOG_RECIPE)
	@mkdir -p build/bin
	go build -tags debug -o $(WEB_BIN) ./cmd/dixiedata-web
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/bundle-web-assets.ps1 $(PWD)
	@mkdir -p build/bin
	go build -tags debug -o $(SEED_BIN) ./cmd/seed-data
	@mkdir -p build/bin
	go build -tags debug -o $(GOLD_BIN) ./cmd/gold-master
	@mkdir -p tools/tune/bin
	cd tools/tune && go build -tags debug -o bin/dixiedata-tune.exe .

# Debug-only binaries (audit harness, render-round, smoke) carry
# the -tags debug build flag so internal/debug/trace.Log() calls
# emit to the JSONL log + ring buffer + Debug Console. The
# no-op stub in trace_nodebug.go keeps the cost at zero in
# release builds (Wails wails build in scripts/build-common.ps1
# gates the same flag on -DebugBuild). These targets are debug-
# only by definition — `make release` does NOT invoke them — so
# always-on -tags debug is appropriate.
#
# Web server (audit/smoke.mjs, ui-diff, render-round).
web: ## Build cmd/dixiedata-web (web-mode server, audit harness target)
	@mkdir -p build/bin
	go build -tags debug -o $(WEB_BIN) ./cmd/dixiedata-web
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/bundle-web-assets.ps1 $(PWD)

# Seed tool (bootstraps .scratch/webmode for audit harness).
seed: ## Build cmd/seed-data (audit harness fixture seeder)
	@mkdir -p build/bin
	go build -tags debug -o $(SEED_BIN) ./cmd/seed-data

# Gold-master regression runner (`make goldmaster`).
gold: ## Build cmd/gold-master
	@mkdir -p build/bin
	go build -tags debug -o $(GOLD_BIN) ./cmd/gold-master

# Tune harness (`make render-round`, `make render-round-ONE`).
tune-bin: ## Build tools/tune (render-round PDF harness)
	@mkdir -p tools/tune/bin
	cd tools/tune && go build -tags debug -o bin/dixiedata-tune.exe .

# `make tune` is the existing run target (renders a PDF against
# the live archive). Add `tune-bin` for the build-only step so
# the debug chain can depend on it without colliding with the run
# target.

# --- Freshness check (build-protocol.md §1, §2) ---
#
# Builds every debug subtool and runs a sanity probe on each.
# Catches the case where `make debug` succeeds but a subtool
# (dixiedata-web, seed-data, gold-master, dixiedata-tune) is
# stale or broken. Also runs `dixiedata debug cli-coverage`
# to assert every documented CLI subcommand still parses +
# dispatches.
#
# Each subtool probe is wrapped in a shell function so a
# failure in one doesn't mask the others; the final exit
# code is the OR of all probes.
FRESHNESS_BIN := build/bin/dixiedata.exe
freshness: web seed gold tune-bin cli-coverage ## Build + sanity-probe every debug subtool
	@echo ""
	@echo "=== freshness: probing subtools ==="
	@status=0; \
	for pair in \
	  "dixiedata-web|$(WEB_BIN)|--help" \
	  "seed-data|$(SEED_BIN)|-h" \
	  "gold-master|$(GOLD_BIN)|-h" \
	  "dixiedata-tune|$(TUNE_BIN)|-h"; do \
	    IFS='|' read -r name bin flag <<< "$$pair"; \
	    if [ ! -x "$$bin" ]; then \
	      echo "  [FAIL] $$name: $$bin not built"; status=1; \
	      continue; \
	    fi; \
	    out=$$("$$bin" $$flag 2>&1); rc=$$?; \
	    if [ $$rc -ne 0 ]; then \
	      echo "  [FAIL] $$name $$flag (exit $$rc)"; status=1; \
	    elif [ -z "$$out" ]; then \
	      echo "  [FAIL] $$name $$flag (empty output)"; status=1; \
	    else \
	      echo "  [ok]   $$name $$flag"; \
	    fi; \
	  done; \
	if [ $$status -ne 0 ]; then echo ""; echo "freshness: FAILED"; exit 1; fi
	@echo ""
	@echo "=== freshness: dixiedata --smoke ==="
	@mkdir -p $(LOGDIR)
	@$(FRESHNESS_BIN) --smoke --json > $(LOGDIR)/freshness-smoke.json 2>&1; rc=$$?; \
	if [ $$rc -ne 0 ]; then echo "  [FAIL] --smoke (exit $$rc); see $(LOGDIR)/freshness-smoke.json"; exit 1; fi; \
	echo "  [ok] --smoke"
	@echo ""
	@echo "freshness: OK"

# CLI subcommand coverage check. Runs the Go binary built by
# the debug target chain and asserts every documented
# subcommand (docs/agents/cli-plan.md) is implemented + every
# implemented subcommand is documented. Drift detector.
#
# Implementation lives in internal/appshell/cli_debug.go
# (`runDebugSubcommand` case "cli-coverage"). The Node script
# scripts/cli-coverage.mjs is a fallback for offline use.
cli-coverage: ## Walk dispatcher vs cli-plan.md; report documented/implemented drift
	@if [ ! -x $(FRESHNESS_BIN) ]; then \
	  echo "cli-coverage: $(FRESHNESS_BIN) not built; run 'make freshness' or 'make debug' first"; exit 1; \
	fi
	@$(FRESHNESS_BIN) debug cli-coverage

release: SCRIPT := scripts/build-release.ps1
release: TARGET := release
release: ARGS :=
release: ## Release build via scripts/build-release.ps1
	$(LOG_RECIPE)

archive: SCRIPT := scripts/build-release.ps1
archive: TARGET := archive
archive: ARGS := -Archive
archive: ## Release build + zip archive
	$(LOG_RECIPE)

demo: SCRIPT := scripts/build-demo-release.ps1
demo: TARGET := demo
demo: ARGS :=
demo: ## Demo release via scripts/build-demo-release.ps1
	$(LOG_RECIPE)

run: SCRIPT := scripts/run-debug.ps1
run: TARGET := run
run: ARGS :=
run: ## Build + launch debug (scripts/run-debug.ps1)
	$(LOG_RECIPE)

# wails dev is interactive — no redirect, full output to terminal.
dev: ## wails dev (interactive — no log capture)
	wails dev

# --- Test targets ---

# Go test default mode is non-verbose; -short skips integration tests that flood logs.
# tools/tune is a separate Go module (its own go.mod) so its
# tests run from its own dir; the snapshot test there auto-
# skips on dev machines without typst + seed-data in PATH.
test: ## Go test ./... with -short -count=1
	go test ./... -short -count=1
	cd tools/tune && go test -short -count=1

test-quiet: ## Alias of `make test`
	go test ./... -short -count=1
	cd tools/tune && go test -short -count=1

stress: SCRIPT := scripts/run-stress-tests.ps1
stress: TARGET := stress
stress: ARGS :=
stress: ## Stress test suite
	$(LOG_RECIPE)

goldmaster: SCRIPT := tests/goldmaster/run-suite.ps1
goldmaster: TARGET := goldmaster
goldmaster: ARGS :=
goldmaster: ## Gold-master suite
	$(LOG_RECIPE)

# --- dixiedata-tune (issue #69 step 5) ---

# Build the standalone tool. Output binary lives at
# tools/tune/bin/dixiedata-tune (Windows: .exe suffix). Cached
# across invocations unless source files change.
tune: ## Run the Tune iteration harness
	@mkdir -p tools/tune/bin
ifeq ($(OS),Windows_NT)
	cd tools/tune && go build -o bin/dixiedata-tune.exe .
else
	cd tools/tune && go build -o bin/dixiedata-tune .
endif

# Run dixiedata-tune against the live archive (.dixiedata/dixiedata.db).
# Smoke test only -- no byte comparison (the live DB changes over
# time). Verifies the tool opens the archive, renders, and exits 0.
# Useful for surfacing layout overflow / edge cases on real data.
tune-smoke: ## Run Tune smoke tests only
	@if [ ! -d .dixiedata ]; then echo "no .dixiedata/ directory; run the appshell once first"; exit 1; fi
	cd tools/tune && go build -o bin/dixiedata-tune .
	tools/tune/bin/dixiedata-tune --db .dixiedata render --template bulk_soldier --mode bulk --out "$(PWD)/build/log/tune-smoke.pdf"
	@ls -la "$(PWD)/build/log/tune-smoke.pdf"

# Regenerate the byte-identical PDF snapshots that pin tune's
# output against internal/archive's output. Requires typst in PATH.
tune-snapshots: ## Update Tune snapshot fixtures (export-contract)
	UPDATE_SNAPSHOTS=1 go test -count=1 ./internal/exportcontract/ -run 'TestArchiveContractSnapshots|TestCLIContractSnapshots' -timeout 600s
	@echo "snapshots regenerated; rerun without UPDATE_SNAPSHOTS=1 to verify byte-stability"
	go test -count=1 ./internal/exportcontract/ -run 'TestArchiveContractSnapshots|TestCLIContractSnapshots' -timeout 600s

# Render every PDF export surface against the live archive for the
# current iteration round. Writes to docs/renderings/<surface>/.
# Iteration loop (issue #69 follow-up): user annotates
# docs/renderings/<surface>/review.md; agent makes code changes;
# rerun with ROUND=2+ to capture successive states.
render-round: ## Render the audit round (default: round 4)
	@if [ ! -d .dixiedata ]; then echo "no .dixiedata/ directory; run the appshell once first"; exit 1; fi
	cd tools/tune && go build -o bin/dixiedata-tune.exe .
	pwsh -NoLogo -NoProfile -File scripts/render-round.ps1 -Round 1

# Render a single surface for one round. Use this when iterating
# on a layout so disk + wall-clock don't scale with the full
# surface set. ROUND defaults to one greater than the highest
# round-<N>.pdf already on disk for this surface. The script
# auto-prunes rounds older than KeepRounds (default 1) so only
# the previous round stays behind for diffing.
#
# Example:
#   make render-round-ONE SURFACE=single-soldier-landscape ROUND=5
#   make render-round-ONE SURFACE=bulk-sorted ROUND=6 KEEP=2
#
# Override the record ID for single-* surfaces (the default
# is record 1 for soldier, record 61 for widow). Useful for
# iterating on a record that has no image, long data, or any
# other layout edge case. The ID is the SQLite primary key
# in the `soldiers` table.
#
#   make render-round-ONE SURFACE=single-soldier-landscape RECORD=21
#   make render-round-ONE SURFACE=single-soldier-portrait  RECORD=21
#   make render-round-ONE SURFACE=single-widow-landscape   RECORD=72
#
# RECORD is ignored for bulk-* / anniversary / insights surfaces.
render-round-ONE: ## Render a single round for a single surface (SURFACE=... ROUND=N)
	@if [ ! -d .dixiedata ]; then echo "no .dixiedata/ directory; run the appshell once first"; exit 1; fi
	@if [ -z "$(SURFACE)" ]; then echo "SURFACE is required, e.g. SURFACE=single-soldier-landscape" >&2; exit 2; fi
	cd tools/tune && go build -o bin/dixiedata-tune.exe .
	@SURFACE=$(SURFACE); ROUND=$(ROUND); KEEP=$(KEEP); RECORD=$(RECORD); \
	  if [ -z "$$ROUND" ]; then \
	    ROUND=$$(ls -1 docs/renderings/$$SURFACE/round-*.pdf 2>/dev/null | sed 's/.*round-//;s/\.pdf//' | sort -V | tail -1); \
	    ROUND=$$(( $${ROUND:-0} + 1 )); \
	  fi; \
	  if [ -z "$$KEEP" ]; then KEEP=1; fi; \
	  echo "rendering $$SURFACE round $$ROUND (keep=$$KEEP)"; \
	  if [ -n "$$RECORD" ]; then \
	    pwsh -NoLogo -NoProfile -File scripts/render-round.ps1 -Round $$ROUND -Only $$SURFACE -KeepRounds $$KEEP -Record $$RECORD; \
	  else \
	    pwsh -NoLogo -NoProfile -File scripts/render-round.ps1 -Round $$ROUND -Only $$SURFACE -KeepRounds $$KEEP; \
	  fi

# Regenerate the byte-stable snapshot fixture(s) for a single
# surface, then verify the regen matches what the export
# pipeline produces today. Snapshots live in
# internal/exportcontract/testdata/{snapshots,snapshots-cli}/
# and are tracked in git, so this is the right place to commit
# layout-driven byte drift alongside the template change.
#
# SURFACE→SNAPSHOT map:
#   single-soldier-landscape         soldier-landscape
#   single-soldier-portrait          soldier-portrait
#   single-widow-landscape           widow-landscape
#   single-widow-portrait            widow-portrait
#   bulk-sorted                      bulk-landscape
#   bulk-grouped-pension-state       grouped-by-pension-state
#   bulk-grouped-burial-location     (no snapshot — single-template change;
#                                    run `make tune-snapshots` to regen all
#                                    22 fixtures at once)
#   anniversary, insights            (no snapshot — same as above)
#
# Example:
#   make update-snapshots-ONE SURFACE=single-soldier-landscape
update-snapshots-ONE: ## Update audit snapshots for a single round (SURFACE=... ROUND=N)
	@if [ -z "$(SURFACE)" ]; then echo "SURFACE is required, e.g. SURFACE=single-soldier-landscape" >&2; exit 2; fi
	@bash -c 'set -e; \
	  case "$(SURFACE)" in \
	    single-soldier-landscape) SNAP=soldier-landscape ;; \
	    single-soldier-portrait)  SNAP=soldier-portrait ;; \
	    single-widow-landscape)   SNAP=widow-landscape ;; \
	    single-widow-portrait)    SNAP=widow-portrait ;; \
	    bulk-sorted)              SNAP=bulk-landscape ;; \
	    bulk-grouped-pension-state) SNAP=grouped-by-pension-state ;; \
	    bulk-grouped-burial-location|anniversary|insights) \
	      echo "no per-surface snapshot for $(SURFACE); run \`make tune-snapshots\` to regen all 22"; exit 1 ;; \
	    *) echo "unknown surface: $(SURFACE)" >&2; exit 2 ;; \
	  esac; \
	  echo "updating snapshots for $$SNAP (in-process + CLI)"; \
	  echo "--- in-process ---"; \
	  UPDATE_SNAPSHOTS=1 go test -count=1 -run "TestArchiveContractSnapshots/$$SNAP" ./internal/exportcontract/ -timeout 120s; \
	  echo "--- CLI ---"; \
	  UPDATE_SNAPSHOTS=1 go test -count=1 -run "TestCLIContractSnapshots/$$SNAP" ./internal/exportcontract/ -timeout 120s; \
	  echo "--- verify (no UPDATE_SNAPSHOTS) ---"; \
	  go test -count=1 -run "TestArchiveContractSnapshots/$$SNAP" ./internal/exportcontract/ -timeout 120s; \
	  go test -count=1 -run "TestCLIContractSnapshots/$$SNAP" ./internal/exportcontract/ -timeout 120s'

# Native-SVG previews alongside the PDFs. ROUND picks the round
# number (default: latest). ONLY restricts to a single surface
# (saves disk + wall-clock when iterating on one layout). IDS is
# a comma-separated list of record IDs for bulk renders. See
# scripts/render-round.ps1 for the same -Only / -RecordIDs flags.
render-svg: ## Render SVG previews via render-svg.sh (issue #14)
ifneq ($(wildcard /c/Users/value/bin/render-svg.sh),)
	@if [ ! -d .dixiedata ]; then echo "no .dixiedata/ directory; run the appshell once first"; exit 1; fi
	cd tools/tune && go build -o bin/dixiedata-tune.exe .
	ROUND?=$$(ls -1 docs/renderings/single-soldier-landscape/round-*.pdf 2>/dev/null | sed 's/.*round-//;s/\.pdf//' | sort -V | tail -1); \
	  echo "rendering round $${ROUND:-4}"; \
	  /c/Users/value/bin/render-svg.sh all $${ROUND:-4}
else
	@echo "render-svg: /c/Users/value/bin/render-svg.sh not installed on this machine (local-only target, skipping)"
endif

# --- Asset generation ---

# Version pinned per scripts/build-common.ps1.
tpl: ## Regenerate templ files
	go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate
	# Sub-target via shell `make` (not $(MAKE)) to dodge the
	# GNUWin32 path-with-parens expansion bug per AGENTS.md.
	sh -c 'make release-notes-bake && make activity-history-bake'

# release-notes-bake: parse CHANGELOG.md into
# internal/releasehistory/baked.go (gitignored). Run via `make tpl`
# (which calls this) or directly. The dev binary ships with
# baked == nil until this runs. Issue #585 slice 2.
release-notes-bake: ## Parse CHANGELOG.md into releasehistory/baked.go
	go run ./scripts/bake-release-notes

# activity-history-bake: parse git log + GitHub Issues into
# internal/activityhistory/baked.go (gitignored). Issue #586
# slice 2/3.
activity-history-bake: ## Bake git log + closed issues into activityhistory/baked.go
	go run ./scripts/bake-activity

# verify-fresh-bake: pre-commit gate that proves the build
# chain works from a clean tree (issue #589). The Makefile's
# standard targets (make test, make tpl) all operate on the
# current working tree, which means a contributor can land a
# bake-style generator change with a stale gitignored
# `baked.go` from a prior session masking a real compile
# failure. This target deletes the gitignored generated
# files, regenerates them via make tpl, then runs make test.
# Any non-zero exit halts.
#
# Generated files deleted (must be re-baked):
#   internal/templates/*_templ.go   (templ generate)
#   internal/releasehistory/baked.go (release-notes-bake)
#   internal/activityhistory/baked.go (activity-history-bake)
#
# Update this list when adding a new bake-style generator so
# the gate stays accurate.
verify-fresh-bake: ## Clean-tree gate: rm gitignored generated files + make tpl + make test (issue #589)
	@echo "=== verify-fresh-bake: deleting gitignored generated files ==="
	@rm -f internal/templates/*_templ.go
	@rm -f internal/releasehistory/baked.go
	@rm -f internal/activityhistory/baked.go
	@echo "=== verify-fresh-bake: regenerating via make tpl ==="
	@$(MAKE) --no-print-directory tpl
	@echo "=== verify-fresh-bake: running make test ==="
	@$(MAKE) --no-print-directory test
	@echo "verify-fresh-bake: OK"

# Alias for the verify-fresh-bake target. Same shape as
# make probe-clean (issue #367) — short verb for the
# filesystem analog.
verify-clean: ## Alias of make verify-fresh-bake (issue #589)
	@$(MAKE) --no-print-directory verify-fresh-bake

# npm --silent suppresses npm's own chatter; tailwind output is short.
css: ## Rebuild Tailwind bundle
	npm run build:css --silent

# --- Maintenance ---

audit: ## Re-run token-saver audit (scripts/token-audit.ps1)

# Issue #316 — htmx-guard lint probes (toast-no-redirect + JS submit
# coexistence). Defaults to informational mode (exit 0, lists drift).
# Use `make lint-htmx-guard-strict` for CI failure mode.
#
# Run this on every PR that touches internal/appshell/*.go,
# internal/htmxattr/*.go, or frontend/app.js. Sibling convention
# for the discover_orphan_handlers probe.
lint-htmx-guard: ## htmx-guard lint (informational; exit 1 → make lint-htmx-guard-strict)
	node audit/discover_htmx_guard.mjs

lint-htmx-guard-strict: ## htmx-guard lint as a CI failure
	node audit/discover_htmx_guard.mjs --strict

lint-htmx-guard-test: ## Run the discover_htmx_guard probe test suite
	node audit/discover_htmx_guard.test.mjs

# lint-bake-bootstrap (issue #591): a script under
# scripts/bake-*/main.go must not import the package it is
# responsible for generating. Catches the bootstrap-ordering
# bug class documented in issue #588 (chicken-egg between
# bake script's import + the gitignored baked.go file the
# script is supposed to produce). Informational by default;
# --strict flips to CI failure. Sibling to lint-htmx-guard.
lint-bake-bootstrap: ## bake-script-imports-target lint (issue #591); docs/agents/issue-tracker.md
	node audit/lint_bake_bootstrap.mjs

lint-bake-bootstrap-strict: ## bake-script-imports-target lint as a CI failure
	node audit/lint_bake_bootstrap.mjs --strict

lint-bake-bootstrap-test: ## Run the lint_bake_bootstrap probe test suite
	node audit/lint_bake_bootstrap.test.mjs

lint-dispatcher-tdz-test: ## Run the dispatcher_tdz_fix regression test (slice-2 typecheck fix)
	node audit/dispatcher_tdz_fix.test.mjs

lint-typecheck-augmentations-test: ## Run the typecheck_augmentations regression test (slice-3 Window / element / htmx augmentations)
	node audit/typecheck_augmentations.test.mjs

# Swallowed-error lint (issue #438, ADR 0010). Three rules that
# prevent the #384 + #436 manual sweeps from regressing:
#   1. Go deferclose  -- no `defer X.Close()` discards
#   2. Go baretempl   -- no bare `templ.Component.Render(...)` discards (slice 2)
#   3. JS no-bare-catch -- no `.catch(() => {})` without a // intentional marker (slice 3)
#
# Build the Go analyzer binary once, then use `go vet -vettool`
# to run the registered analyzers against the DixieData module.
# The unitchecker driver requires go-vet semantics; calling the
# binary directly is not supported.
lint-defer-close: ## Go defer-.Close() lint (issue #438, ADR 0010)
	cd tools/lintrules && go build -o bin/lintrules.exe ./cmd/lintrules
	@go vet -vettool=tools/lintrules/bin/lintrules.exe ./... > $(LOGDIR)/lint-defer-close.txt 2>&1; rc=$$?; \
	  if [ $$rc -ne 0 ]; then cat $(LOGDIR)/lint-defer-close.txt; exit $$rc; fi; \
	  node -e "var s=require('fs').readFileSync('$(LOGDIR)/lint-defer-close.txt','utf8');if(s.includes('\"message\"')){console.log(s);process.exit(1)}"

lint-bare-templ-render: ## Go bare-templ-Render lint (issue #438, ADR 0010)
	cd tools/lintrules && go build -o bin/lintrules.exe ./cmd/lintrules
	@go vet -vettool=tools/lintrules/bin/lintrules.exe ./... > $(LOGDIR)/lint-bare-templ.txt 2>&1; rc=$$?; \
	  if [ $$rc -ne 0 ]; then cat $(LOGDIR)/lint-bare-templ.txt; exit $$rc; fi; \
	  node -e "var s=require('fs').readFileSync('$(LOGDIR)/lint-bare-templ.txt','utf8');if(s.includes('\"message\"')){console.log(s);process.exit(1)}"

lint-no-bare-catch: ## JS bare-.catch() lint (issue #438, ADR 0010) — slice 3
	@echo "lint-no-bare-catch: running ESLint..."
	npm run lint:js

lint-swallowed-errors: ## Run all swallowed-error lints (issue #438, ADR 0010)
	@echo "=== swallowed-error lint sweep ==="
	make lint-defer-close
	make lint-bare-templ-render
	make lint-no-bare-catch
	@echo "swallowed-error lint sweep: OK"

lint: ## Run all codebase lints (including swallowed-errors)
	make lint-swallowed-errors
	make lint-migration-columns
	make lint-htmx-guard
	make lint-bake-bootstrap
	make lint-dialog-guard
	make lint-microcopy
	make lint-static-archive-microcopy-strict
	make lint-pdf-microcopy-strict
	make lint-icalendar-microcopy-strict
	make lint-runtime-microcopy-strict
	make lint-typecheck

lint-typecheck: ## TypeScript type-check on frontend/**/*.js via tsc (--noEmit)
	@npm run typecheck --silent

lint-migration-columns: ## Grep production Go for SQL referencing renamed columns (issue #435)
	@node audit/smoke_migration_columns.mjs

# Issue #445 — extend the dialog-guard sweep. Walks every native
# (Open|Save)(File|Directory|MultipleFiles)?Dialog call in
# internal/appshell/ and asserts the enclosing function has a
# enterInFlight / LoadOrStore / guarded*Dialog helper / errExportInFlight
# sentinel per docs/agents/dialog-guard.md. Exits 0 with 0 unguarded
# sites; informational by default (--strict flips to CI failure).
lint-dialog-guard: ## Native-dialog-guard sweep (issue #445); docs/agents/dialog-guard.md
	@node audit/smoke_dialog_guard.mjs

lint-dialog-guard-strict: ## Native-dialog-guard sweep as CI failure (--strict)
	@node audit/smoke_dialog_guard.mjs --strict

# Issue #561 — UX microcopy regression net. Static source scan that
# walks every .templ file under internal/templates/** and asserts
# the three rules documented in docs/agents/ux-microcopy.md:
#   R1. Eyebrow (`uppercase tracking-` <p>/<div>) above a single
#       self-explanatory block (blockquote / table) with no form
#       controls — the canonical "Rotating Local Archive Quote"
#       violation.
#   R2. <h*> text equals adjacent <button> text within ~15 lines.
#   R3. Same visible user-facing paragraph appears twice within
#       ~15 lines (CSS class strings, templ component calls, JSON
#       attribute strings, and Go control-flow lines are stripped).
# Informational by default; --strict flips to CI failure.
lint-microcopy: ## UX microcopy sweep (issue #561); docs/agents/ux-microcopy.md
	@node audit/smoke_microcopy.mjs

lint-microcopy-strict: ## UX microcopy sweep as CI failure (--strict)
	@node audit/smoke_microcopy.mjs --strict

lint-microcopy-test: ## Run the smoke_microcopy probe test suite
	@node audit/smoke_microcopy.test.mjs

# Static Archive microcopy (issue #581, Slice 1). The archive viewer
# embeds HTML + JS in internal/archive/static_archive.go, so this
# format-aware probe checks user-facing shell and renderer strings
# without applying .templ parsing to CSS, JS plumbing, or data fields.
lint-static-archive-microcopy: ## Static Archive microcopy sweep (issue #581)
	@node audit/smoke_static_archive_microcopy.mjs

lint-static-archive-microcopy-strict: ## Static Archive microcopy sweep as CI failure
	@node audit/smoke_static_archive_microcopy.mjs --strict

lint-static-archive-microcopy-test: ## Run Static Archive microcopy probe tests
	@node audit/smoke_static_archive_microcopy.test.mjs

# Typst PDF microcopy (issue #581, Slice 2). The PDF surface uses
# Typst, not Go-templ + HTML; this format-aware probe walks
# templates/**/*.typ for the canonical forbidden-pattern rules
# (analytics_summary verbose subtitle + stacked sub-headings,
# group_divider trailing sentence, event card ALL-CAPS eyebrows,
# biography_appendix status-form empty state) without applying
# `.templ` parsing to common/ helpers or the theme/hello smoke
# templates. Informational by default; --strict is the CI gate.
lint-pdf-microcopy: ## Typst PDF microcopy sweep (issue #581, slice 2)
	@node audit/smoke_pdf_microcopy.mjs

lint-pdf-microcopy-strict: ## Typst PDF microcopy sweep as CI failure
	@node audit/smoke_pdf_microcopy.mjs --strict

lint-pdf-microcopy-test: ## Run Typst PDF microcopy probe tests
	@node audit/smoke_pdf_microcopy.test.mjs

# iCalendar microcopy (issue #581, Slice 3, audit-only). The
# .ics export is the only DixieData surface that emits RFC 5545
# text; the probe walks internal/archive/export_service.go for
# the canonical user-facing copy (calname, PRODID, SUMMARY
# presets, DESCRIPTION lines, VALARM bodies) without applying
# .templ or Typst rules to the wrong grammar. Slice 3 ships
# GREEN-on-HEAD (no edits planned); the probe pins the surface
# so a future contributor cannot drift the catalogued copy
# without breaking the CI gate.
lint-icalendar-microcopy: ## iCalendar microcopy sweep (issue #581, slice 3, audit-only)
	@node audit/smoke_icalendar_microcopy.mjs

lint-icalendar-microcopy-strict: ## iCalendar microcopy sweep as CI failure
	@node audit/smoke_icalendar_microcopy.mjs --strict

lint-icalendar-microcopy-test: ## Run iCalendar microcopy probe tests
	@node audit/smoke_icalendar_microcopy.test.mjs

# Runtime + server + startup + CLI microcopy (issue #581,
# Slice 4, audit-only). The probe walks frontend/app.js +
# internal/appshell/app.go (loading placeholder) + main.go
# (cliHelpText) for the canonical user-facing copy that the
# earlier three probes did not cover. Slice 4 ships
# GREEN-on-HEAD (no edits planned); the probe pins the
# surface so a future contributor cannot drift it without
# breaking the CI gate.
lint-runtime-microcopy: ## Runtime + server + startup + CLI microcopy (issue #581, slice 4)
	@node audit/smoke_runtime_microcopy.mjs

lint-runtime-microcopy-strict: ## Runtime microcopy sweep as CI failure
	@node audit/smoke_runtime_microcopy.mjs --strict

lint-runtime-microcopy-test: ## Run runtime microcopy probe tests
	@node audit/smoke_runtime_microcopy.test.mjs

# Issue #446 — extend the dispatcher-contract coverage. Live-server
# probe that sends POST + X-HTTP-Method-Override for every PATCH/
# PUT/DELETE route in routes.go and asserts the body preserves
# through the override chain. Complements the static-source
# dispatcher_patch_method.test.mjs (which pins the JS dispatcher
# code) with a runtime assertion that the server actually sees the
# body. Requires a running dixiedata-web server at $BASE_URL
# (default http://127.0.0.1:8765) with seed data.
probe-dispatcher-contract: ## Runtime dispatcher-contract probe (issue #446)
	@node audit/probe-dispatcher-contract.mjs

# Issue #444 — end-to-end "button click → user sees error" probe.
# Browser-driven probe that clicks five representative buttons that
# should produce a user-visible error surface (toast / empty-state-
# error / aria-alert / inline-validation / 404-chrome), then asserts
# the surface appeared within a timeout. Two cases (PDF export with
# missing typst; "Move Source Up past top") are documented skips —
# see audit/probe-error-surfaces.mjs for the rationale per case.
# Requires a running dixiedata-web at $BASE_URL with seed data
# (same as probe-dispatcher-contract).
probe-error-surfaces: ## Browser end-to-end button-click error probe (issue #444)
	@node audit/probe-error-surfaces.mjs

# Kill any leftover dixiedata-* processes from a previous probe run.
# Without this, the next `make debug` fails with `unlinkat ...
# dixiedata-web.exe: The process cannot access the file because it
# is being used by another process.`
#
# Lives in scripts/probe-clean.ps1 rather than inline here because
# the verify loop (kill → wait → re-query → retry) is too long for
# a one-liner, and a .ps1 surfaces clear errors when a process
# survives the kill (AV hold, protected process, re-spawn). The
# script exits 1 if anything survives; make propagates that, so the
# build halts with context instead of failing later at unlinkat.
#
# Safe to run anytime — the script is a no-op (exit 0) when no
# target processes are alive.
probe-clean: ## Kill + verify straggler dixiedata-*.exe processes (see scripts/probe-clean.ps1)
	$(PWSH) -NoLogo -NoProfile -File scripts/probe-clean.ps1

# UI v1 vs v2 side-by-side screenshot diff (issue #74 Phase 0 PR4).
# Requires the dixiedata-web server to be running; see audit/README.md
# for the boot + seed steps. Output: audit/reports/ui-diff/.
ui-diff: ## Capture v1 vs v2 side-by-side screenshots (issue #74)
	node scripts/ui-diff.mjs
	$(PWSH) -File scripts/token-audit.ps1

clean: ## Remove generated artifacts (scripts/token-clean.ps1)
	$(PWSH) -File scripts/token-clean.ps1

log-clean: ## Truncate build/log/*.log
	@mkdir -p $(LOGDIR)
	@rm -f $(LOGDIR)/*.log
	@echo "Cleared $(LOGDIR)/*.log"

# Archive CHANGELOG.md entries older than the current year into
# archive/CHANGELOG-{year}.md (issue #442). The active file keeps
# [Unreleased] + current-year entries; legacy/undated entries
# (pre-2026 header format) go to archive/CHANGELOG-legacy.md.
# Idempotent — re-running on an already-archived file is a no-op.
changelog-archive: ## Move entries older than current year to archive/CHANGELOG-{year}.md
	$(PWSH) -NoLogo -NoProfile -File scripts/archive-changelog.ps1

# --- Release pipeline (interactive; output NOT logged) ---

# Bump CurrentSchemaVersion in internal/versioninfo/versioninfo.go.
# Strict: refuses to advance > 1 without -Force, requires
# docs/migrations/v{N+1}.md to exist with at least one '- ' bullet.
# Protects DixieData's local update feature.
bump: ## Bump schema version (writes versioninfo.go; commit before tagging)
	$(PWSH) -File scripts/bump-version.ps1

# Detect schema-touching drift (build-protocol.md §4). Walks
# HEAD..base commit subjects for feat(db) / feat(schema) / fix(db)
# patterns; fails if CurrentSchemaVersion is unchanged. The
# 'chore: skip-schema-bump' hatch covers the rare case where a
# PR touches the db layer without changing the schema shape.
# Mirrors the bash step in .github/workflows/test.yml.
bump-detect-drift: ## Detect schema-touching drift in the current branch
	@if [ -z "$$GITHUB_BASE_REF" ]; then echo "bump-detect-drift: GITHUB_BASE_REF not set; skipping (run inside GitHub Actions or set it manually)"; exit 0; fi
	GITHUB_BASE_REF=$$GITHUB_BASE_REF $(PWSH) -File scripts/bump-version.ps1 -DetectDrift

# Tag, push main, push tag, create DRAFT GitHub release via gh CLI.
# Safety gates: clean tree, committed bump, archive present, tag absent
# (local + remote), gh authenticated. Draft = not auto-published.
release-github: ## Tag + push + draft gh release (run 'make archive' first)
	$(PWSH) -File scripts/release-github.ps1

# --- Release pipeline (build-protocol.md §3) ---
#
# Ordered gates that must pass before a release is cut. Halts
# on the first non-zero exit. Run interactively — each gate's
# output streams so the operator can see what failed.
#
# Gates:
#   1. test             — go test -short -count=1
#   2. tpl              — regenerate templ files (catches stale generated files)
#   3. css              — rebuild Tailwind bundle
#   4. bump-verify      — bump-version.ps1 -VerifyOnly (schema discipline intact)
#   5. debug            — build DixieData + 4 subtools (chains web/seed/gold/tune-bin)
#   6. freshness        — subtool sanity probes + CLI coverage
#   7. archive          — build + zip release/DixieData-release-v1.2.{N}.zip
#   8. release-github   — tag + push + draft gh release
#
# The 'audit' gate is MANUAL (operator runs `make audit` and
# signs off) — not chained here. Add it explicitly between
# freshness and archive if your release needs the visual sweep.
#
# Pass RELEASE_SKIP_FRESHNESS=1 to skip gate 6 in emergencies
# (CI outage, subtool build failure blocking a security fix).
# Document the skip in the PR description.
RELEASE_PIPELINE_GATES := test tpl css bump-verify debug freshness archive release-github
release-pipeline: ## Run the ordered release chain; halt on first failure
	@echo "=== release-pipeline ==="
	@failed=0; \
	for gate in $(RELEASE_PIPELINE_GATES); do \
	  if [ "$$gate" = "bump-verify" ]; then \
	    echo ""; echo "--- gate: bump-verify ---"; \
	    if ! $(PWSH) -NoLogo -NoProfile -File scripts/bump-version.ps1 -VerifyOnly; then \
	      echo "FAIL at gate: $$gate"; failed=1; break; \
	    fi; \
	    continue; \
	  fi; \
	  if [ "$$gate" = "freshness" ] && [ "$(RELEASE_SKIP_FRESHNESS)" = "1" ]; then \
	    echo ""; echo "--- gate: freshness (SKIPPED via RELEASE_SKIP_FRESHNESS=1) ---"; \
	    continue; \
	  fi; \
	  echo ""; echo "--- gate: $$gate ---"; \
	  if ! $(MAKE) --no-print-directory $$gate; then \
	    echo "FAIL at gate: $$gate"; failed=1; break; \
	  fi; \
	done; \
	if [ $$failed -ne 0 ]; then echo ""; echo "release-pipeline: ABORTED"; exit 1; fi
	@echo ""
	@echo "release-pipeline: OK"

# --- Promotion chain: dev → stable (ADR 0009 §"The promote flow") ---
#
# Per ADR 0009, the promote flow is PR via GitHub UI:
#   1. make promote-dry-run  (gate chain, no push, no PR)
#   2. make promote          (gate chain + open PR dev → stable)
#   3. operator reviews PR + merges via GitHub UI
#   4. make promote-confirm  (post-merge sanity: stable HEAD == merge SHA)
#   5. operator runs scripts/release-github.ps1
#
# The gate chain here is the same as release-pipeline EXCEPT
# release-github itself — that's the operator's last step,
# after the PR is merged and the operator reviews the diff.
# The promote-prep target handles the case where dev has
# commits stable doesn't have (see ADR 0009 §"Conflict policy").

STABLE_BRANCH ?= stable

PROMOTE_GATES := test tpl css bump-verify debug freshness archive
promote-dry-run: ## Run the promotion gate chain (no push, no PR); per ADR 0009
	@echo "=== promote-dry-run: gates 1-$$(echo "$(PROMOTE_GATES)" | wc -w) ==="
	@failed=0; \
	for gate in $(PROMOTE_GATES); do \
	  if [ "$$gate" = "bump-verify" ]; then \
	    echo ""; echo "--- gate: bump-verify ---"; \
	    if ! $(PWSH) -NoLogo -NoProfile -File scripts/bump-version.ps1 -VerifyOnly; then \
	      echo "FAIL at gate: $$gate"; failed=1; break; \
	    fi; \
	    continue; \
	  fi; \
	  echo ""; echo "--- gate: $$gate ---"; \
	  if ! $(MAKE) --no-print-directory $$gate; then \
	    echo "FAIL at gate: $$gate"; failed=1; break; \
	  fi; \
	done; \
	if [ $$failed -ne 0 ]; then echo ""; echo "promote-dry-run: ABORTED"; exit 1; fi
	@echo ""
	@echo "promote-dry-run: OK"
	@echo ""
	@echo "=== pre-flight: dev vs $(STABLE_BRANCH) divergence ==="
	@git fetch origin $(STABLE_BRANCH) dev 2>/dev/null || true
	@echo "Commits on dev not on $(STABLE_BRANCH):"
	@git log origin/$(STABLE_BRANCH)..origin/dev --oneline 2>/dev/null | head -50 || echo "  (none; dev and $(STABLE_BRANCH) are in sync)"
	@echo ""
	@echo "promote-dry-run: gates passed; safe to run 'make promote'"

promote: ## Run gate chain + open PR dev → stable via gh CLI; per ADR 0009
	@echo "=== promote: gates 1-$$(echo "$(PROMOTE_GATES)" | wc -w) ==="
	@$(MAKE) --no-print-directory promote-dry-run
	@echo ""
	@echo "=== pre-flight: divergence check ==="
	@git fetch origin $(STABLE_BRANCH) dev 2>/dev/null || true
	@if ! git diff --quiet origin/$(STABLE_BRANCH)..origin/dev 2>/dev/null; then \
	  echo ""; \
	  echo "promote: ABORTED — dev has commits $(STABLE_BRANCH) doesn't have."; \
	  echo "Run 'make promote-prep' to sync $(STABLE_BRANCH) with dev first."; \
	  exit 1; \
	fi
	@echo "promote: dev and $(STABLE_BRANCH) are in sync."
	@echo ""
	@echo "=== opening PR dev → $(STABLE_BRANCH) ==="
	@bash scripts/promote-open-pr.sh $(STABLE_BRANCH)

promote-prep: ## Sync $(STABLE_BRANCH) with dev for conflict-free promotion; per ADR 0009
	@echo "=== promote-prep: sync $(STABLE_BRANCH) with dev ==="
	@git fetch origin $(STABLE_BRANCH) dev
	@echo ""
	@echo "Commits on dev not on $(STABLE_BRANCH):"
	@git log origin/$(STABLE_BRANCH)..origin/dev --oneline 2>/dev/null | head -50 || echo "  (none; dev and $(STABLE_BRANCH) are in sync)"
	@echo ""
	@echo "Commits on $(STABLE_BRANCH) not on dev:"
	@git log origin/dev..origin/$(STABLE_BRANCH) --oneline 2>/dev/null | head -10 || echo "  (none)"
	@echo ""
	@echo "promote-prep: if the divergence is non-conflicting, run:"
	@echo "  git checkout $(STABLE_BRANCH) && git merge --no-ff origin/dev"
	@echo "If the divergence is conflicting, resolve conflicts locally per ADR 0009 §\"Conflict policy\","
	@echo "commit the resolution to $(STABLE_BRANCH) via a hot-fix PR, then re-run 'make promote'."

promote-confirm: ## Post-merge sanity: stable HEAD matches dev merge SHA
	@echo "=== promote-confirm: post-merge sanity ==="
	@git fetch origin $(STABLE_BRANCH) dev
	@echo ""
	@if git diff --quiet origin/$(STABLE_BRANCH)..origin/dev 2>/dev/null; then \
	  echo "promote-confirm: $(STABLE_BRANCH) and dev are in sync. Safe to run scripts/release-github.ps1."; \
	else \
	  echo "promote-confirm: $(STABLE_BRANCH) and dev STILL differ. Investigate before tagging."; \
	  git log origin/$(STABLE_BRANCH)..origin/dev --oneline 2>/dev/null | head -10; \
	  exit 1; \
	fi
