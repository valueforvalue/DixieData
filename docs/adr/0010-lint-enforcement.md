# Lint enforcement for swallowed-error patterns

## Status

Accepted 2026-07-08. Closes the loop on issue #384 (20-slice manual
sweep, landed on `dev` as of `acc391f`) and issue #436 (JS catch
follow-up, landed on `dev` as of `abb7d97`). The sweep is complete;
this ADR adds the **prevention** layer that keeps the patterns from
returning. Closes the loop on issue #384 (20-slice manual
sweep, landed on `dev` as of `acc391f`) and issue #436 (JS catch
follow-up, landed on `dev` as of `abb7d97`). The sweep is complete;
this ADR adds the **prevention** layer that keeps the patterns from
returning.

## Context

DixieData's defense against swallowed errors is currently
`audit/smoke_swallowed_errors.mjs` — a Node source-scan probe that
walks `internal/` and `frontend/`, fails the run if any of the
banned patterns appears, and grows 117 assertions across both
languages. It works. It also has three structural limitations that
make it a regression net, not a prevention net:

1. **It runs after the commit, not before.** A contributor pushes,
   the workflow runs, the probe fires, the contributor context-
   switches, the fix round-trip costs 10-30 minutes per offender.
2. **It catches at the editor level but cannot enforce the
   positive form.** The probe can say "you have a bare
   `Render(r.Context(), w)` here" but cannot say "you forgot the
   `respondErrorFragment` wrapper" or "your catch has no `console.
   warn` and no `showToast`" without a flood of false positives.
3. **It is the only defense.** If the probe is bypassed (a future
   contributor edits the assertion list, or the probe is split out
   of `just test-lint && go test -short ./...`), the patterns return silently.

Issue #384 captured the lessons. This ADR encodes the prevention
layer as a normal part of the toolchain: two Go analyzer rules
(standalone `go/analysis` package at `tools/lintrules/`) and one
ESLint rule (workspace-local `eslint-plugin-dixie`).

## Decision

### Rule 1 — `deferclose`: no `defer X.Close()` ignoring the error

Matches any `DeferStmt` whose `Call.Fun` is a `SelectorExpr` with
`Sel.Name == "Close"` and a `Close() error` method per type info.
Reports the call site with a `SuggestedFixes` entry pointing at
`debug.DeferCloseLog` (`internal/debug/close.go`).

Bail-out: `//nolint:dixie/deferclose` comment on the same line.
The probe (`audit/smoke_swallowed_errors.mjs`) becomes the
backstop — it counts the `DeferCloseLog` sites per file and
fails if the count drops, so a `//nolint` that bypasses the
structural check still gets caught at the "did you replace it
with the right helper" level.

### Rule 2 — `baretempl`: no bare `templ.Component.Render(...)` discards

Matches `*ast.ExprStmt` whose `X` is a `CallExpr` ending in
`.Render(...)` with a `templ.Component` receiver (per type info),
where the call is **not** the argument of a parent
`respondErrorFragment(...)` or `respondError(...)` call. Walks
the parent chain via a stack-tracked inspector so nested
expressions do not false-positive on the helper wrappers.

Bail-out: `//nolint:dixie/baretempl` comment on the same line.
The positive form (per-handler `respondErrorFragment` wrap at
known sites, lines 211-637 of the probe) stays in the probe —
the lint rule catches the negative, the probe asserts the
positive.

### Rule 3 — `eslint-plugin-dixie/no-bare-catch`

Matches `CallExpression` where `callee.property.name === "catch"`
and `arguments[0]` is an `ArrowFunctionExpression` whose `body`
is a `BlockStatement` with `body.length === 0`. Reports the
expression with a link to `docs/agents/error-handling.md`
"JS catches (client-side)".

Bail-out: a `// intentional: <reason>` comment on the line
immediately before the catch (the existing convention from
`frontend/debug.js` 6 logger sites, locked in issue #436).
The lint rule's regex matches the same `intentional` keyword
the existing probe uses, so the marker is shared — the
probe can be retired (see "Probe retirement" below).

### Module layout

`tools/lintrules/` is a **separate Go module** with its own
`go.mod` (mirrors `tools/tune/`). Single `cmd/lintrules/main.go`
runs `golang.org/x/tools/go/analysis/unitchecker` with all
analyzer packages registered. CI runs it via
`go run ./tools/lintrules ./...` (root module) — the
`unitchecker` driver accepts a package list and loads each
analyzer in the same process.

`frontend/eslint-plugin-dixie/` is a workspace-local npm
package (own `package.json`, no `node_modules` checked in).
Single `index.js` exports the rule. Root `eslint.config.js`
imports it via the workspace alias.

### CI integration

- `.github/workflows/test.yml` gains two steps after the
  existing "htmx-guard lint (issue #316)" step (~line 131):
  - `just lint-no-bare-catch` — runs the Go `lintrules` binary
    and ESLint in sequence.
- `Makefile` gains the following targets near the
  `lint-htmx-guard` block (~line 248):

  ```
  lint-defer-close: ## Go defer-.Close() lint (issue #438)
      go run ./tools/lintrules/cmd/lintrules ./...

  lint-bare-templ-render: ## Go bare-templ-Render lint (issue #438)
      go run ./tools/lintrules/cmd/lintrules ./...

  lint-no-bare-catch: ## JS bare-.catch() lint (issue #438)
      npx eslint 'frontend/**/*.js'

  lint-swallowed-errors: ## Run all three linters (issue #438)
      make lint-defer-close
      make lint-bare-templ-render
      make lint-no-bare-catch
  ```

- `just lint-all-frontend` chains the new target as a backstop (lint
  enforces pre-merge; audit catches anything lint misses).
- ESLint runs only against `frontend/**/*.js` — the
  `audit/smoke_swallowed_errors.mjs` walk over `frontend/` is
  retired (see below).

### Probe retirement

`audit/smoke_swallowed_errors.mjs` retains the **positive**
assertions (the per-handler `respondErrorFragment` wrap checks
at lines 211-637, the `EmptyStateError` + `respondErrorFragment`
declarations at lines 1093-1131, the LS-wrapper check at lines
150-194) and removes the **negative** assertions that the lint
rules now own:

- Lines 644-670: `internal/appshell/ has no bare-Render sites
  left` — **retire** (lint rule supersedes with AST, not regex).
- Lines 691-1054: per-file `defer .Close()` scans across
  `backup_service.go`, `soldier_service.go`, `export_service.go`,
  `tag_service.go`, `updater.go`, `event_service.go`,
  `audit_service.go`, `appshell/app.go`, `quality_scan.go`,
  `article_service.go` + the `internal/` meta-walk — **retire**
  the regex scans, **keep** the per-file `DeferCloseLog` count
  assertions (count + tag discipline are unique value).
- Lines 1172-1195: `frontend/ has no bare .catch()` — **retire**
  (ESLint rule supersedes with AST).

Header comment of the probe updates to point at the lint rules
for archaeology. Total assertion count drops from 117 to ~95.

### Toolchain additions

- Root `package.json` adds `eslint@^9` + `globals` + `@eslint/js`
  as `devDependencies`. `frontend/eslint-plugin-dixie/package.json`
  declares the local plugin (no `node_modules` published).
- `frontend/eslint-plugin-dixie/index.js` is ~50 lines
  (single rule file, no test harness needed — covered by the
  fixture test under `tools/lintrules/` ESLint adapter if
  desired; current scope is the rule itself).
- `tools/lintrules/go.mod` requires `golang.org/x/tools v0.30+`
  (already a transitive dep of `templ` per `go.sum`; promote to
  direct).
- `.nvmrc` is created with `20` to lock the ESLint runtime
  (CI already pins Node 20; local is Node 24 — pin for parity).

### Cost estimate

- Go `deferclose` analyzer: ~80 LoC analyzer + 40 LoC test
  fixtures = 2 hours.
- Go `baretempl` analyzer: ~120 LoC (parent-chain walk is the
  fiddly bit) + 60 LoC test fixtures = 3 hours.
- ESLint rule: ~50 LoC + 30 LoC fixture = 1 hour.
- CI wiring + Makefile + `package.json` updates + probe
  retirement: 1 hour.
- Total: ~1 working day, all mechanical, all testable against
  the existing sweep's zero-offender baseline.

## Alternatives considered

1. **golangci-lint v2 plugin module** — would integrate with the
   de-facto Go linter. Rejected: requires installing
   `golangci-lint` in CI (new toolchain dep), the v2 plugin
   module layout is heavier than a standalone `go/analysis`
   package, and DixieData has no existing golangci-lint config
   to build on. A standalone `tools/lintrules/` is the same
   detection with one fewer external dep.
2. **Source-code-formatter (gofmt-style) for catches** —
   rejected: catches are semantically meaningful, not
   syntactically regularizable. The "fix" is a different
   pattern (named try/catch + console.warn + surface), not a
   rewrite.
3. **Custom `//nolint` for the JS rule** — rejected: the
   existing `// intentional` convention is the shared marker
   with the probe (now retired), and ESLint's `// eslint-
   disable-next-line dixie/no-bare-catch` adds a new keyword.
   Keep one marker.
4. **One big `errors` rule that combines all three** — rejected:
   splits by language tooling (Go binary vs ESLint binary),
   splits by ownership (Go handled by `tools/lintrules/`, JS
   handled by `eslint-plugin-dixie/`), and the rules trip at
   different times in different workflows.

## Consequences

Positive:
- Patterns caught at `git commit` (via `make lint-swallowed-
  errors` in pre-commit hook, future work — out of scope for
  this ADR) or at the latest at the first CI run after the
  push. No more "fix round-trip" cost.
- AST-level analysis catches false negatives the regex probe
  missed (e.g. `.Render(r.Context(), w)` split across multiple
  lines, the `frontend/` regex's 3-line window missing markers
  in `.then().catch()` chains).
- Defense in depth: lint rule + probe (positive form) =
  impossible to regress without rewriting the test harness.

Negative:
- New toolchain deps: ESLint 9 + `frontend/eslint-plugin-dixie/`
  + `tools/lintrules/`. Each is small but each is something
  the next contributor has to learn.
- `//nolint:dixie/<rule>` markers can hide a future regression
  if used carelessly. Mitigated by the probe's count assertion:
  every `//nolint` adds 1 to a file's "expected DeferCloseLog
  count" line in the probe, so the count must be updated
  deliberately.
- Two Go analyzer rules + one ESLint rule = three new things
  to maintain. The test fixtures are the maintenance anchor;
  if a fixture breaks, the rule regressed.

## References

- Issue #384 — parent (sweep)
- Issue #436 — JS catch follow-up (locked the `// intentional`
  convention this ADR inherits)
- Issue #438 — this ADR's issue
- `docs/agents/error-handling.md` — house style that the rules
  encode
- `internal/debug/close.go` — `DeferCloseLog` helper the
  `deferclose` rule suggests as a fix
- `internal/appshell/respond.go` — `respondErrorFragment` /
  `respondError` helpers the `baretempl` rule looks for in the
  parent chain
- ADR 0006 — `slog` vs `trace` decision tree (precedent for a
  toolchain-policy ADR)
- ADR 0004 — Option C dispatcher (precedent for an
  enforcement-via-test-harness ADR)


## Author

Jeremy Morris (@jeremymorris) — backfilled 2026-07-17
