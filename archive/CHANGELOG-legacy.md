# CHANGELOG (archive: legacy)

Archived from CHANGELOG.md by scripts/archive-changelog.ps1.
Historical entries only — the active file is CHANGELOG.md.

## v1.1.21 - Patch Release

- Replaced the UI's remote Tailwind CDN dependency with a checked-in local CSS bundle so desktop installs render correctly on offline machines.
- Automated CSS regeneration in the shared PowerShell build path so debug and release builds refresh the bundled stylesheet before Wails packaging.
- Carried the release line forward to `v1.1.21` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.20 - Patch Release

- Fixed the entry-form draft recovery banner so the **Discard local draft** button remains visible after the page initializes and after draft-status updates run.
- Carried the release line forward to `v1.1.20` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.19 - Patch Release

- Fixed the new-record localStorage draft flow so successful creates clear the cached entry instead of repopulating the next record form.
- Added an in-app **Discard local draft** recovery action on new/edit record forms so stuck entry drafts can be cleared without DevTools or a debug build.
- Enabled Confederate Home fields for wife and widow records in the entry form.
- Carried the release line forward to `v1.1.19` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.18 - Full Release

- Hardened `.ddbak`, `.ddshare`, diagnostics, and static archive ZIP creation to write through a temp file, verify ZIP finalization, and only then replace the destination file.
- This avoids success-shaped partial archives caused by unverified final ZIP close/flush behavior at the final save path.

## v1.1.17 - Patch Release

- Fixed the static web archive detail view so exported `index.html` and `viewer.html` can open a selected person without leaving the expanded data area blank.
- Carried the release line forward to `v1.1.17` so the runtime metadata, Wails title, exported artifacts, and docs stay aligned.
- Migrated `handleCalendarPDF`'s nine `slog.Debug` entry/exit/branch
  calls to `trace.Log` per ADR 0006 (issue #221). Each call loses
  zero value at INFO+ (structural only) and gains structured
  attributes that render in the Debug Console and JSONL log under
  `-tags debug` builds. Path PII drops from `dialog_cancelled`
  and `dialog_returned`; form-payload dump drops from `month`
  (operator-noise). `fmt` import retained for `dupKey` sprintf;
  `debug` import retained for other handlers in the file.
  Regression net: `go build ./...`, `go build -tags debug ./...`,
  `go test ./... -short -count=1` (22 packages green),
  `go test -tags debug ./internal/debug/...` (2 packages green).

- Fixed the white-screen bug on initial load under blocked states
  (pre-mux window, setup-required, recovery, startupErr). The
  `blockIfFragment` helper returns 204 + `X-DixieData-Redirect`
  for htmx fragment requests; htmx does not auto-follow a 204 the
  way it follows a 3xx, so the empty `<body>` shell stayed empty
  and the user saw a white page. Added an `htmx:afterRequest`
  listener in `frontend/app.js` that reads the redirect header
  from any 204 response and calls `window.location.assign`. New
  `audit/smoke_fragment_redirect.mjs` regression test asserts the
  full client path: empty shell loads, htmx fires, server returns
  204 + header, browser navigates to `/setup`, body renders.
  Test would fail without the listener (final URL stays on the
  shell path, body length 0) and pin the server response shape
  (204 + header) so the contract doesn't drift. The pattern
  addresses the systemic gap that let 4 prior fragment-204 fixes
  (#209 pre-mux, #212 setup, #214 recovery + startupErr) ship
  without a working client bridge.

- Moved `jobs.jsonl` out of the data dir into the sibling
  `.dixiedata-logs/` directory. The on-disk jobs registry held
  an open append handle on `<dataDir>/jobs.jsonl` for the
  lifetime of the process; on Windows, that descendant handle
  blocked the atomic `os.Rename` that `replaceDataDir`
  performs at the start of a `.ddbak` restore, returning
  "Access is denied" after 5 retries. The fix follows the
  same convention as commit `b9a30cc` for `app.log.jsonl`:
  the data dir contains only the SQLite database and the
  image store. `migrateLegacyJobsLog` moves any existing
  legacy file to the new location on first startup (copy +
  remove fallback when the rename itself is denied). New
  tests pin the convention:
  - `TestOpenJobsRegistryLogPathOutsideDataDir` asserts
    the canonical log path is outside the data dir
  - `TestMigrateLegacyJobsLogRenamesOldFile` covers the
    upgrade path for existing installs
  - `TestMigrateLegacyJobsLogIsIdempotent` covers restart
    safety
  - `TestReplaceDataDir_.../open_file_outside_target_dir...`
    confirms `replaceDataDir` no longer touches the logs
    directory
  - `audit/smoke_jobs_log_location.mjs` confirms the
    deployed web binary writes the log to the right place.
  `docs/COMMON_BUGS.md` §4.17 codifies the convention:
  every new file written to the data dir must live under
  `appdata.LogsDir(dataDir)` instead.
- Lazy-load the print-records fragment to make /browse and
  /share nav GETs fast at scale. The print-config modal's
  filter panel + record picker were server-rendered on
  every /browse and /share GET, paying a listAllSoldiers()
  cost that was 80-95% of the request time (bench-verified:
  261ms at 5k records, 717ms+ at 10k). The modal now opens
  immediately on first paint; a new
  `GET /share/print-records-fragment` endpoint serves the
  filter + picker as a htmx-swapped fragment on modal open.
  The fragment is rendered by a new `PrintRecordsFragment`
  templ and cached client-side after the first load;
  archive-mutating actions (export template save/update/
  delete) invalidate the cache so the next open re-fetches.
  Bench: 5k records /browse GET drops from 261ms to 36ms
  (7x faster); 5k fragment GET stays under 250ms. New
  files:
  - `internal/templates/partials/print_records_fragment.templ`
  - `internal/appshell/print_records_fragment_handlers.go`
  - `internal/appshell/print_records_fragment_test.go`
  - `audit/smoke_browse_nav_speed.mjs`
  Threshold test updated: `TestHandleBrowseResponseUnderThreshold`
  asserts 100ms at 5k records (was 500ms at 1k). 3 new
  unit tests for the fragment endpoint (renders, empty
  archive, 405 contract). `handleShare` deliberately keeps
  listAllSoldiers() for the `CalendarDriftStatus` consumer
  (one-shot destination; cost is acceptable for the page's
  single visit per session).

- **Removed three fluff test files** (issue #318 Slice 0).
  `internal/routebuilder/routebuilder_test.go` (60+
  one-liner string-concat asserts), `internal/persondisplay/
  display_test.go` (2 trivial format asserts), and
  `internal/debug/trace/trace_test.go` (3 stdlib-semantics
  tests in a `//go:build debug` no-op package) — none caught
  real bugs per the test-strategy audit. The two useful
  edge cases from `routebuilder_test.go` (URL-escape + trim)
  folded into a single table-driven
  `internal/routebuilder/routebuilder_edge_test.go`.
  Net: -356 LOC of test code, +19 LOC of real coverage.
  Test file count: 172 → 170.

- **Skipped `internal/buildinfo` doc-audit tests in `-short`**
  (issue #318 Slice 0). The 3 tests
  (`TestEveryInternalPackageHasSynopsis`,
  `TestNoWrongStarterDocComments`,
  `TestPerPackageDocCoverageFloor`) shell out to `go doc`
  per-package across 150+ internal + pkg packages; each
  subprocess cold-starts at ~100ms, totaling 70s of
  wall-clock for the package. The audit catches missing
  `// Package foo` doc comments per the Go Doc audit
  Phase 1+2 — real value, but too heavy for every
  `make test` invocation. Now skipped in `-short` with a
  message pointing at the explicit invocation. `make test`
  drops from ~125s to ~62s on this repo. Audit still runs
  under `go test -count=1 ./internal/buildinfo/...` or any
  CI lane that drops `-short`.

- **CI race detector gate enabled** (issue #318 Slice 1).
  `.github/workflows/test.yml::Go test (short, race detector)`
  now runs `go test -race ./... -short -count=1` with
  `CGO_ENABLED=1`. Catches the concurrency bug class per
  `docs/COMMON_BUGS.md` §4.1 (race condition). Local
  `make test` is unchanged — the `-race` flag requires cgo,
  which the Windows dev env typically lacks. CI has gcc on
  `ubuntu-latest`. A race bug will surface as a PR CI failure
  rather than a local `make test` failure. The previous
  `TestMuxSelectEmitted` registry drift was already resolved
  in commit `c136789`; the failing-test concern from the
  audit is moot on this branch.

- **Goroutine leak detection via `goleak`** (issue #318 Slice 2).
  Added `go.uber.org/goleak v1.3.0` as a direct dep and a shared
  `internal/leaktest/leaktest.go` helper. `TestMain` in the root
  `main_test.go` plus `internal/jobs/zzz_goleak_test.go`,
  `internal/appshell/zzz_goleak_test.go`, and
  `internal/archive/zzz_goleak_test.go` install the gate.
  Catches the `docs/COMMON_BUGS.md` §4.1 class automatically
  — a test that starts a goroutine without cancelling its
  context fails the package suite with the leak's stack
  trace. Go's `TestMain` is per-package so root-only coverage
  wouldn't gate the packages where leaks actually live;
  per-package opt-in via `zzz_goleak_test.go` extends the
  gate to the highest-leverage surfaces (jobs, appshell,
  archive). Default matchers ignore net/http keepalive /
  readLoop / writeLoop + runtime pollWait so the signal is
  DixieData leaks, not stdlib noise.

- **Property tests in `internal/dates`** (issue #318 Slice 3).
  Added `pgregory.net/rapid v1.2.0` and
  `internal/dates/dates_property_test.go` with 7 properties:
  full-date roundtrip (Format → Parse → Format), year-only
  roundtrip, never-panic on arbitrary input, normalization
  idempotence, all-zero formatting sentinel, Display never
  panics, and ParseBirthInfo year extraction under randomised
  word boundaries. Catches the `docs/COMMON_BUGS.md` §9.2
  (normalization) class automatically. Local `make test`
  runs 20 iterations per property (rapid default under
  `-short`); CI workflow bumped to 500 (`-short` / 5 = 100
  iterations) so ≥100 random inputs land on every PR.
  Existing hand-written `dates_test.go` stays — properties
  complement, not replace, the table-driven coverage.

## v1.1.16 - Gold Master
