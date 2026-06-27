# DixieData Copilot Instructions

## Build and test commands

The Makefile is the preferred entry point. `make help` lists every target. Common:

- `make test` — `go test ./... -short -count=1`
- `make tpl` — regenerate `*_templ.go` after editing `.templ` files
- `make debug` — debug build (writes `build\bin\Run-DixieData-Debug.ps1`)
- `make run` — build + launch debug build with UI IDs enabled
- `make release` / `make archive` — release build, with or without versioned zip
- `make demo` — seeded demo release package
- `make stress` / `make goldmaster` — full test suites

Underlying PowerShell scripts (for advanced use):

- `go test ./...` runs the full Go test suite.
- `go test -run TestAppServeHTTPMethodOverride .` runs a single root-package test. Swap in another test name as needed, or use `go test -run TestName ./...` when you do not know the package yet.
- `go build ./...` is the baseline full build validation.
- `go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate` regenerates Templ outputs. Run this whenever a `.templ` file changes.
- `.\scripts\build-debug.ps1` builds the desktop app and writes `build\bin\Run-DixieData-Debug.ps1`.
- `.\scripts\run-debug.ps1` launches the current debug build with UI surface IDs enabled.
- `.\scripts\build-release.ps1` builds the release executable, and `.\scripts\build-release.ps1 -Archive` also creates the versioned zip in `release\`.
- `.\scripts\run-stress-tests.ps1` runs the stress/fuzz-oriented validation workflow in `tests\stress`.

## High-level architecture

- This is a Windows-first Wails desktop app with a Go backend and SQLite storage. `main.go` is the repo entrypoint, and `internal\appshell\app.go` serves the UI through `http.ServeMux` handlers rather than a separate API + SPA split.
- `internal\appshell\app.go` is the delivery surface. It wires facade interfaces from `internal\appshell\app_facades.go`, parses requests, and delegates rendering through `internal\presentation`; keep business logic out of it.
- The frontend is server-rendered. `internal\templates\*.templ` defines the HTML, generated `*_templ.go` files are checked in, and `frontend\app.js` is a custom HTMX-style request/swap layer that drives navigation, form submissions, Smart Back, toasts, tabs, and other rich interactions.
- `internal\presentation\views.go` is the grey-box adapter between domain objects and rendered templates. `internal\viewmodel` holds display-ready DTOs that templates consume.
- Runtime behavior is split into deep domain packages: `internal\records` owns record/search/review/analytics/research workflows, `internal\archive` owns exports/backups/diagnostics/images, and `internal\integrations` owns Google integration logic.
- `internal\services` is now a compatibility shim over those deeper packages, not the architectural center.
- Startup resolves the working data directory through `internal\appdata\appdata.go`, opens SQLite through `internal\db\db.go`, applies schema/migrations from `internal\db\schema.go`, then reloads facades before routes are usable.
- Persistent app data lives under `.dixiedata` by default (or `DIXIEDATA_DATA_DIR`). The database, scratchpads, image files, backups, merge-review artifacts, and logs all live there instead of under the repo tree.
- Search is not plain SQL `LIKE`. The app maintains an FTS5 index plus canonical scratch pad content in `scratchpad_cache`, so scratch pads participate in normal search results.

## Key conventions

- Treat the current architecture as a hard guardrail: deep modules plus a grey-box delivery layer are intentional and should not be bypassed in new work.
- `.templ` files should consume `internal\viewmodel` DTOs only. Do not pass `internal\records`, `internal\archive`, `internal\integrations`, or raw persistence structs straight into template signatures.
- Delivery shaping belongs in `internal\presentation`, not inline in `internal\appshell\app.go`. Raw domain models should be transformed there into `internal\viewmodel` DTOs before rendering.
- Keep `internal\appshell\app.go` thin and extend frontend-facing contracts in `internal\appshell\app_facades.go` when new delivery operations are needed. Do not add raw business logic, calculations, or direct workflow coordination there.
- The deep modules are the primary ownership boundaries: put record workflow changes in `internal\records`, archive/export/image mechanics in `internal\archive`, and Google work in `internal\integrations`. Solve problems inside those modules first, keep helpers/package internals unexported unless a narrow public contract requires them, and avoid leaking internal mechanics across package boundaries.
- The `soldiers` table is the main people table for both actual soldiers and spouse entries. Use `entry_type` and `spouse_soldier_id` rather than assuming every row is a soldier.
- Display IDs are normalized to a canonical `NAMESPACE-00000` shape. Use the helpers in `internal\db\displayid.go` and identity helpers in `internal\db\identity.go` instead of hand-rolling parsing or generation.
- The release/app version is schema-driven. `internal\db\schema.go` defines `CurrentSchemaVersion`, `db.GetAppVersion()` derives the app version from it, and the PowerShell build scripts package releases from that value.
- Record images are stored on disk in a sharded path under `.dixiedata\images\<A>\<B>\<sanitized-display-id>\...`. Scratch pad bridge/window-state files may appear under `.dixiedata\scratchpads\...`, but canonical scratch pad content lives in SQLite.
- UI surface IDs are a real project convention, not just test-only metadata. The canonical registry is `internal\uiids\uiids.go`, the human reference is `docs\ui-ids.md`. Use the constants (e.g. `uiids.PageSoldierDetail`) instead of string literals in templates and HTMX attributes.
- Template changes are two-part changes: edit the `.templ` file and regenerate the checked-in `*_templ.go` output.
- Public delivery boundaries and facade contracts should stay covered by fast automated tests. Before closing out code changes, run `go test ./...` and rely on module-seam tests to catch leaks across the facade boundary.
- Many higher-level tests create a full temp app via `newStressApp(t)` and then call `configureTestIdentity(t, app)` so features that depend on initialized identity and `.dixiedata` layout behave like the real app.
