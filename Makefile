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
        render-svg tpl css audit clean log-clean bump release-github

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
# gold-master binary). Without these dependencies the user
# runs `make debug`, opens the app, hits a button, and the
# harness smoke test fails because the web server binary isn't
# in build/bin/. The dependencies guarantee a one-shot `make
# debug` produces everything a debug session needs.
WEB_BIN := build/bin/dixiedata-web.exe
SEED_BIN := build/bin/seed-data.exe
GOLD_BIN := build/bin/gold-master.exe
TUNE_BIN := tools/tune/bin/dixiedata-tune.exe

build debug: SCRIPT := scripts/build-debug.ps1
build debug: TARGET := debug
build debug: ARGS :=
build debug: ## Debug build via scripts/build-debug.ps1
	$(LOG_RECIPE)
	@$(MAKE) --no-print-directory web seed gold tune-bin

# Web server (audit/smoke.mjs, ui-diff, render-round).
web: ## Build cmd/dixiedata-web (web-mode server, audit harness target)
	@mkdir -p build/bin
	go build -o $(WEB_BIN) ./cmd/dixiedata-web

# Seed tool (bootstraps .scratch/webmode for audit harness).
seed: ## Build cmd/seed-data (audit harness fixture seeder)
	@mkdir -p build/bin
	go build -o $(SEED_BIN) ./cmd/seed-data

# Gold-master regression runner (`make goldmaster`).
gold: ## Build cmd/gold-master
	@mkdir -p build/bin
	go build -o $(GOLD_BIN) ./cmd/gold-master

# Tune harness (`make render-round`, `make render-round-ONE`).
tune-bin: ## Build tools/tune (render-round PDF harness)
	@mkdir -p tools/tune/bin
	cd tools/tune && go build -o bin/dixiedata-tune.exe .

# `make tune` is the existing run target (renders a PDF against
# the live archive). Add `tune-bin` for the build-only step so
# the debug chain can depend on it without colliding with the run
# target.

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
test test-quiet: ## Go test ./... with -short -count=1
	go test ./... -short -count=1

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

# npm --silent suppresses npm's own chatter; tailwind output is short.
css: ## Rebuild Tailwind bundle
	npm run build:css --silent

# --- Maintenance ---

audit: ## Re-run token-saver audit (scripts/token-audit.ps1)

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

# --- Release pipeline (interactive; output NOT logged) ---

# Bump CurrentSchemaVersion in internal/versioninfo/versioninfo.go.
# Strict: refuses to advance > 1 without -Force, requires
# docs/migrations/v{N+1}.md to exist with at least one '- ' bullet.
# Protects DixieData's local update feature.
bump: ## Bump schema version (writes versioninfo.go; commit before tagging)
	$(PWSH) -File scripts/bump-version.ps1

# Tag, push main, push tag, create DRAFT GitHub release via gh CLI.
# Safety gates: clean tree, committed bump, archive present, tag absent
# (local + remote), gh authenticated. Draft = not auto-published.
release-github: ## Tag + push + draft gh release (run 'make archive' first)
	$(PWSH) -File scripts/release-github.ps1
