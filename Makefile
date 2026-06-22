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
        stress goldmaster tune tune-smoke tune-snapshots render-round tpl css audit clean log-clean bump release-github

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

build debug: SCRIPT := scripts/build-debug.ps1
build debug: TARGET := debug
build debug: ARGS :=
build debug: ## Debug build via scripts/build-debug.ps1
	$(LOG_RECIPE)

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
tune:
	@mkdir -p tools/tune/bin
	cd tools/tune && go build -o bin/dixiedata-tune .

# Run dixiedata-tune against the live archive (.dixiedata/dixiedata.db).
# Smoke test only -- no byte comparison (the live DB changes over
# time). Verifies the tool opens the archive, renders, and exits 0.
# Useful for surfacing layout overflow / edge cases on real data.
tune-smoke:
	@if [ ! -d .dixiedata ]; then echo "no .dixiedata/ directory; run the appshell once first"; exit 1; fi
	cd tools/tune && go build -o bin/dixiedata-tune .
	tools/tune/bin/dixiedata-tune --db .dixiedata render --template bulk_soldier --mode bulk --out "$(PWD)/build/log/tune-smoke.pdf"
	@ls -la "$(PWD)/build/log/tune-smoke.pdf"

# Regenerate the byte-identical PDF snapshots that pin tune's
# output against internal/archive's output. Requires typst in PATH.
tune-snapshots:
	UPDATE_SNAPSHOTS=1 go test -count=1 ./internal/exportcontract/ -run 'TestArchiveContractSnapshots|TestCLIContractSnapshots' -timeout 600s
	@echo "snapshots regenerated; rerun without UPDATE_SNAPSHOTS=1 to verify byte-stability"
	go test -count=1 ./internal/exportcontract/ -run 'TestArchiveContractSnapshots|TestCLIContractSnapshots' -timeout 600s

# Render every PDF export surface against the live archive for the
# current iteration round. Writes to docs/renderings/<surface>/.
# Iteration loop (issue #69 follow-up): user annotates
# docs/renderings/<surface>/review.md; agent makes code changes;
# rerun with ROUND=2+ to capture successive states.
render-round:
	@if [ ! -d .dixiedata ]; then echo "no .dixiedata/ directory; run the appshell once first"; exit 1; fi
	cd tools/tune && go build -o bin/dixiedata-tune.exe .
	pwsh -NoLogo -NoProfile -File scripts/render-round.ps1 -Round 1

# --- Asset generation ---

# Version pinned per scripts/build-common.ps1.
tpl: ## Regenerate templ files
	go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate

# npm --silent suppresses npm's own chatter; tailwind output is short.
css: ## Rebuild Tailwind bundle
	npm run build:css --silent

# --- Maintenance ---

audit: ## Re-run token-saver audit (scripts/token-audit.ps1)
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
