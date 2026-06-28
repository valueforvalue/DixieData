# CLI subcommand plan — `dixiedata <cmd>`

This is the canonical roadmap for DixieData's headless CLI surface.
Phases ship one at a time; this file updates after each ship.

**Status snapshot** (last update after commit `98ddb45`; Phase 7 shipped in the same series):

| Phase | Status | Commit |
|-------|--------|--------|
| 1 — `--smoke` | ✅ shipped | `df981a1` |
| 2 — `doctor` | ✅ shipped | `7d9fc69` |
| 3 — `list` / `show` / `search` | ✅ shipped | `e5f8e61` |
| 4 — `export ...` | ✅ shipped | `e4474b7` |
| 5 — `import ...` | ✅ shipped | `98ddb45` |
| 6 — `migrate` / `backup` / `restore point` / `logs` / `config` | ⏳ backlog | — |
| 7 — `debug ...` | ✅ shipped | `82bf194` |

## Design

### Entry point

```
func main() {
    if subcommand, ok := parseSubcommand(os.Args[1:]); ok {
        os.Exit(runSubcommand(subcommand))
    }
    // existing wails.Run path
}
```

Dispatch order in main.go (most specific first):

1. `appshell.HasDoctorFlag` — `doctor` subcommand
2. `appshell.HasQuerySubcommand` — `list` / `show` / `search`
3. `appshell.HasExportSubcommand` — `export ...`
4. `appshell.HasImportSubcommand` — `import ...` (Phase 5, not yet wired)
5. `appshell.HasSmokeFlag` / `EnvRequestsSmoke` — `--smoke` / `DIXIEDATA_SMOKE=1`
6. fall through to Wails GUI

Each subcommand handler follows the same pattern:

```go
func runXxxSubcommand() int {
    opts, err := appshell.ParseXxxArgs(os.Args[1:])
    if err != nil { fmt.Fprintln(os.Stderr, "error:", err); return 3 }
    a := appshell.NewApp()
    ctx := context.Background()
    a.Startup(ctx)
    defer a.Shutdown(ctx)
    opts.App = a
    code, err := appshell.RunXxx(ctx, opts)
    if err != nil { fmt.Fprintln(os.Stderr, "error:", err) }
    return code
}
```

### Dispatch style

Plain `os.Args[1:]` parsing — no cobra, no kingpin. Reasons confirmed by
shipping experience:

- DixieData stays dep-light (see `go.mod`).
- Per subcommand ≤ 10 flags. Hand-rolled < 200 lines each.
- Cobra adds ~300kb to binary + reflection cost on every parse.
- Easier to keep the smoke / doctor path audit-friendly.

If we ever hit >20 subcommands, revisit.

### Flag conventions

- All subcommands support `--json` for machine-readable output.
- All subcommands support `--help` (generated from a docstring table;
  doctor / smoke already auto-print their flags in failure messages).
- All subcommands accept `--data-dir <path>` to override the data dir
  (default: standard location via `appdata`). *(Not yet wired — see
  Phase 6 admin / Open questions.)*
- Bool flags: `--no-images`, `--printer-friendly`, `--dry-run`, `--fix`,
  `--yes`.
- Both `--flag value` and `--flag=value` forms are accepted everywhere.

### Exit codes

| Code | Meaning |
|------|---------|
| 0    | Success |
| 1    | Operation failed (e.g. soldier not found, export error) |
| 2    | Environment error (data dir unwritable, pdfium.dll missing) |
| 3    | Usage error (unknown flag, missing required arg) |
| 4    | Auth/permission error (read-only archive, etc.) |

### JSON output shape

Every subcommand emits a stable JSON envelope when `--json` is set.
Smoke / doctor / query / export each define their own struct but share
the convention of one JSON object per check/result plus an envelope at
the end with `command`, `started_at`, `duration_ms`, `exit`. CI parses
the JSON; humans use the default text output.

### App lifecycle in headless mode

Every CLI subcommand handler calls `a.Startup(ctx)` then
`defer a.Shutdown(ctx)`. Startup does:

- Resolves data dir via `appdata.DefaultDir()` (env override
  `DIXIEDATA_DATA_DIR`).
- Migrates logs from `.dixiedata/` to sibling `.dixiedata-logs/`
  (`b9a30ccb66`).
- Configures structured logging via `internal/debug`.
- Opens SQLite + runs idempotent migrations.
- Wires the soldiers / export / backup / anniversary / image /
  calendar / analytics / audit / integration / updater facades.
- Sets up the `jobs.Registry`.
- Loads `quotes.json` + restore-point state.
- Registers HTTP routes via `chi` so `a.mux` is non-nil
  (smoke `routes_registered` check needs this).

Shutdown drains jobs (5s timeout) then closes the DB.

**Lesson learned (shipping Phases 1–4):** `startup()` will print
`jobs: open .../jobs.jsonl for append failed: ...` when invoked from a
cwd with no data dir. This is **not fatal** — startup still succeeds.
Tests that look for "clean output" should account for this line.

## Subcommand taxonomy

### Phase 1 — `smoke` ✅ shipped

```
dixiedata --smoke
dixiedata --smoke --json
DIXIEDATA_SMOKE=1 dixiedata
```

8 checks. Files: `internal/appshell/smoke.go`, `smoke_test.go`.

### Phase 2 — `doctor` ✅ shipped

```
dixiedata doctor
dixiedata doctor --check=data_dir --check=sqlite --json
dixiedata doctor --fix
```

12 checks (8 smoke + 4 deeper). Files: `internal/appshell/doctor.go`,
`doctor_test.go`, `docs/agents/doctor-impl-notes.md`.

**Lessons learned:**

- `pdfium_loadable` must be `optional: true` for dev builds (no
  pdfium.dll shipped). Release artifact linter (future Pattern D test)
  is the right place to enforce "release zip must contain pdfium.dll".
- `templates_parseable` distinguishes parse from runtime errors by
  inspecting the first `error:` line — `error: expected`/`unexpected`
  is parse; `error: type ... has no method` is runtime. Empty `{}`
  stub data.json lets templates parse; runtime failures get tolerated
  so we don't false-positive on missing data shape.
- Real data fixtures for `templates_parseable` (per-template
  `templates/testdata/<name>.json`) are still future work. Today the
  check catches syntax regressions only.

### Phase 3 — read-only subcommands ✅ shipped

```
dixiedata list soldiers [--query Q] [--limit N] [--page P] [--json]
dixiedata show soldier <id|display-id> [--json]
dixiedata search <query> [--limit N] [--page P] [--json]
```

Files: `internal/appshell/cli_query.go`, `cli_query_test.go`.

**Lessons learned:**

- `show`'s target must be the **last** non-flag positional arg so flags
  can be interleaved anywhere. `--show soldier --json DXD-00052` works
  the same as `--show soldier DXD-00052 --json`.
- Numeric IDs and display IDs (`DXD-00052`) both work for `show`.
  Auto-detect via `strconv.ParseInt` first.
- `list sources [--person <id>]` deferred — sources are nested
  `[Record]models.Soldier` not a separate list endpoint. `show soldier`
  already prints them in the detail view.

### Phase 4 — export subcommands ✅ shipped

```
dixiedata export pdf --soldier <id> --out <path>
dixiedata export pdf --month YYYY-MM | M --out <path>
dixiedata export pdf --full --out <path> [--settings <json>]
dixiedata export jpg --soldier <id> --out <path>
dixiedata export json --out <path>
dixiedata export csv --out <path>
dixiedata export ical --out <path>
dixiedata export static-archive --out <zip>
dixiedata export backup --out <file>
```

Files: `internal/appshell/cli_export.go`, `cli_export_test.go`.

**Critical:** bypasses `SaveFileDialog` entirely. This solves the
c1d9dc1-class bug **permanently for CLI use** (CLI users never trigger
the WebView2 race because there's no WebView2 in CLI mode). GUI users
still benefit from the in-flight guard added in c1d9dc1.

**Lessons learned:**

- **JPG is two-stage** (PDF → rasterize → JPG) via
  `archive.ExportSoldierJPG`. Needs `pdfium.dll` to be present in
  `build/bin/` for the rasterize step. Without it, JPG exports fail
  with `pdfium.dll was not found; expected it beside the app
  executable, in the working directory, or at DIXIEDATA_PDFIUM_DLL`.
- **Static archive is a ZIP, not a directory.** `ExportStaticArchive`
  calls `zipDirectory(outputPath, exportRoot)`. Users must pass a
  `.zip` path; `--out` is the file path.
- **Backup is a `.ddbak` ZIP** containing `manifest.json` +
  `data/dixiedata.db` + `images/...`. Output path ends in `.ddbak` by
  convention but `.zip` would also work.
- **PDFium restore gotcha:** `wails build -clean` wipes `build/bin/`,
  so every debug build needs `Restore-DixieDataPdfiumBinary` after.
  The restore function now prefers a local cache (any
  `release/*/pdfium.dll` with matching version + SHA256) over the
  GitHub download — saves 30s+ per debug build.
- **ExportModeUnknown gotcha for JPG:** JPG path doesn't set
  `opts.Mode` (only PDF does). `runExportJPG` must check
  `opts.SoldierID == 0` directly, not `opts.Mode != ExportModeSingle`.

### Phase 5 — `import ...` ✅ shipped (commit `98ddb45`)

```
dixiedata import backup          --from <file.ddbak>          [--dry-run|--yes]
dixiedata import shared-archive --from <file.ddshare>        [--dry-run|--yes]
dixiedata import images         --soldier <id|dxd-id> --from <file>...
dixiedata import memorial-json  --from <file.json>           [--dry-run]
```

**Scope locked at design time:** four real imports only.
`import static-archive` and `import feedback-log` were dropped
during planning — neither has a GUI button in the live app
(`/import/backup`, `/import/shared-archive`,
`/import/memorial-json/preview|confirm`, and
`/soldiers/{id}/images/import` are the only import routes
registered in `internal/appshell/routes.go`). Static archives
are read-only browser-viewable output with no companion import
path. Feedback is hand-typed in the GUI; there's no consumer
for ingesting JSONL feedback logs.

**File:** `internal/appshell/cli_import.go` (+ `cli_import_zip.go`).

**Dispatch table:**

| Command | App method |
|---------|-----------|
| `import backup`          | `a.backup.ImportWithLocalIdentity(from, dataDir, localIdentity, preserveLocalIdentity)`. Closes + reopens DB around the staging swap, exactly mirroring `handleImportBackup`. `loadLocalImportIdentity` helper inlines `archive.currentImportIdentity` so the local identity can be resolved before the DB handle is closed. |
| `import shared-archive`  | `a.backup.ImportSharedBackup(from, dataDir)`. Single blocking call. `SharedImportSummary` (soldiers/records/images inserted/updated, pending conflicts, log path) is printed + JSON-serializable. |
| `import images`          | `a.ImportImagePaths(soldier, paths)` — new exported wrapper around the unexported `importImagePaths` used by `handleImportSoldierImages`. Supports `SoldierID` and `DisplayID` (auto-detect from `--soldier` value). |
| `import memorial-json`   | `a.soldiers.ImportMemorialArchive(from)` — single blocking call; writes its own issues log under the data dir. Skips the GUI's two-phase preview (CLI user has already chosen the file). Dry-run path uses `a.soldiers.PreviewMemorialArchive` for the same preview the GUI shows. |

**Decisions settled during implementation:**

1. **Where does `--dry-run` surface?** Per-kind lightweight pre-flight
   inside the CLI dispatch (not in the service layer). Each
   `runImport*` function branches on `opts.DryRun` BEFORE touching
   the App's facades. No invasive `DryRun bool` parameter on the
   service signatures — that would force every caller (GUI handlers
   included) to deal with the same plumbing.

2. **`--yes` enforcement.** Without `--yes`, refuse to run if the
   import is destructive (`import backup` and `import shared-archive`
   only — `ImportKind.IsDestructive()` decides). Refusal is a parser
   error → exit 3 = usage error per the standard exit codes. Matches
   the `git rebase` / `apt install` pattern. No TTY check — the user
   can pass `--yes` in scripts.

3. **Conflict resolution for shared-archive.** Deferred. Existing
   `ImportSharedBackup` auto-merges non-conflicting records and
   stages conflicts for review via the existing merge-review UI.
   CLI just prints the `PendingConflicts` count and the merge log
   path. A `--conflict=skip|merge|overwrite` flag would need an
   upstream signature change to `ImportSharedBackup`. Park until a
   real user story demands it.

4. **Atomicity.** **Pre-import restore-point safety net was REMOVED
   after live testing revealed a contract conflict. See "Restore-point
   finding" below for the full story.** Rollback story is surfaced
   in import output instead: backup import says "rollback: re-run
   with a different .ddbak"; shared-archive import says "rollback:
   review pending conflicts in the merge-review UI".

**Restore-point finding — READ BEFORE TOUCHING PHASE 6:**

The Phase 5 plan called for taking a restore point via
`a.restorePoints.Create` before any non-dry-run backup or
shared-archive import, then printing the ID so the user could
roll back. Implemented and tested at the parser level (14 unit
tests passing). Then live-tested: parser accepted, manager
returned success, `writeZipArchive.Rename` returned success,
manager defer correctly skipped cleanup — yet no files on
disk. After three rounds of debug prints at every step of the
manager + `writeZipArchive`, the root cause surfaced:

> The restore-point manager writes to
> `<dataDir>/updates/restore-points/<id>/`, which is INSIDE the
> data dir. `archive.replaceDataDir` (called by backup import
> via `ImportWithLocalIdentity`) renames the data dir to a
> `*-previous-*` sibling and then `RemoveAll`s it, destroying
> any restore point that landed inside. `ImportSharedBackup`
> mutates the live DB in place, so a restore point in
> `<dataDir>/updates/` would have been overwritten by the
> merge. Either way, the restore-point files do not survive
> the import that created them.

The manager isn't broken. The plan assumption was wrong:
**the in-place update restore-point manager is a
data-dir-resident store, but backup imports REPLACE the data
dir in place.** Those contracts can't coexist.

**Locked regression test:**
`internal/archive/backup_service_test.go::TestBackupService_ImportDestroysFilesInsideDataDir`
plants a sidecar file at `<dataDir>/updates/restore-points/<id>/local-archive.ddbak`,
imports a `.ddbak`, asserts the sidecar and the whole `updates/`
tree are gone. If `replaceDataDir` ever starts preserving
sidecars (or the restore-point root moves), this test goes red
and forces the right update.

**What Phase 6 needs to do to make pre-import restore points
viable:** add a sibling restore-point root
(`.dixiedata-restore-points/`) outside the data dir. The
manager's `dataDir` field would point at the sibling; existing
tests need their `dataDir` updated. `RestorePointManager` will
need a constructor parameter or a new constructor
(`NewSiblingRestorePointManager`) — the existing
`NewRestorePointManager(dataDir)` writes inside dataDir and
should stay unchanged for the in-place update flow.

**Lessons learned:**

- The native restore-point system is for **update-in-place**.
  It snapshots the live archive BEFORE the update swaps it.
  The user's mental model is "update the binary; the restore
  point rolls back the data dir if the new binary breaks". My
  CLI tried to use it as "import rolls back the data dir if
  the import breaks". Same machinery, different purpose, and
  the data-dir-resident root is wrong for the import case.
- Restore-point safety net for backup imports is **structurally
  useless** because the imported `.ddbak` IS the rollback
  artifact. Re-importing a different `.ddbak` is faster and
  cleaner than rolling back a snapshot to import a different
  snapshot.
- Restore-point safety net for shared-archive imports is
  **valuable** because the merge mutates existing data; the
  pre-merge state is not preserved anywhere else. Phase 6's
  sibling root unblocks this.

### Phase 6 — admin subcommands ⏳ backlog

```
dixiedata migrate status
dixiedata migrate up
dixiedata migrate down <version>
dixiedata backup list
dixiedata backup prune --keep-last N
dixiedata restore point list
dixiedata restore point create [--note <text>]
dixiedata restore point apply <id>
dixiedata logs tail [--follow]
dixiedata logs path
dixiedata config show
dixiedata config set <key> <value>
```

Each maps to an existing `*App` method.

- `migrate` wraps the SQLite migration runner. `migrate up` is just
  `db.Open(dataDir)` (idempotent — `applySchema` short-circuits if
  `user_version == CurrentSchemaVersion`). `migrate down` is more
  invasive; consider whether we want it.
- `backup` / `restore point` wrap the existing `.ddbak` /
  restore-point machinery. **Phase 6's `restore point` subcommand
  is the dependency for re-enabling the pre-import safety net
  that Phase 5 had to disable** (see "Restore-point finding"
  above). Specifically: Phase 6 must add a sibling
  `.dixiedata-restore-points/` root OUTSIDE the data dir before
  `import shared-archive --yes` can offer a rollback safety net.
  The current `RestorePointManager` writes inside the data dir
  and conflicts with `replaceDataDir`. New constructor
  `NewSiblingRestorePointManager(siblingDir)` (or equivalent)
  writes to the sibling and survives the data-dir swap. Once
  that lands, Phase 5 should call the sibling manager in
  `createImportRestorePoint` and resume printing the ID for
  rollback via `restore point apply`.
- `logs tail` reads the JSONL file directly (last N lines). `--follow`
  tails like `tail -f`. Needs careful handling of file rotation.
- `config show` / `config set` operate on the SQLite-backed
  `local_settings` table.
- Add `--data-dir` flag support to all subcommands here (and Phase 7).

### Phase 7 — debugging ✅ shipped (commit `82bf194`)

```
dixiedata debug dump > archive-summary.json
dixiedata debug hx-invariants
dixiedata debug browser-tree
dixiedata debug request <path>
```

**File:** `internal/appshell/cli_debug.go` (+ `cli_debug_test.go`).

**Hard constraint upheld:** debug subcommands never write to
the archive, never accept `--yes`, and never accept `--fix`.
Four kinds only. `--json` envelope + `--data-dir PATH` on
every subcommand.

**Dispatch table:**

| Command | Implementation |
|---------|---------------|
| `debug dump`             | `App.ArchiveInventory()` — new thin wrapper. Reads `user_version` (PRAGMA), `archive_counts` via `soldiers.ArchiveCounts()`, plus row counts for 15 tables (soldiers, records, images, calendar_items, duplicate_audit_findings, merge_review_sessions, merge_review_conflicts, shared_merge_aliases, research_tasks, research_collections, research_collection_items, import_batches, soldiers_needing_review) + the two pending-state sub-counts. Also surfaces `local_settings.json` snapshot + `user_identity` from `db.UserIdentity()`. Returns `ArchiveInventory` struct that JSON-encodes to a stable envelope. |
| `debug hx-invariants`    | Pure source walker. Walks every `.templ` file under `<repo>/internal/templates/` (via `collectTemplFiles`), extracts `hx-target`/`hx-post`/`hx-get`/`hx-put`/`hx-delete`/`hx-patch`/`hx-trigger` attributes via regex, builds a global DOM ID index from `id="..."` declarations, then AST-walks `internal/appshell/routes.go` for registered (pattern, method) pairs. Cross-references: (a) every `hx-target="#id"` must resolve to a known DOM ID; (b) every `hx-{post,get,put,delete,patch}="URL"` must resolve to a registered route (with chi-style `{param}` and `/*` wildcard matching). Exit 0 if clean, exit 1 if any violations. |
| `debug browser-tree`     | Same AST walker as `hx-invariants` but renders the registered route table grouped by HTTP method. Sorted output for stable diffs. |
| `debug request <path>`   | `App.DispatchHeadlessRequest(path)` — new thin wrapper. Builds a synthetic `httptest.NewRequest("GET", path, nil)`, runs the registered mux (via `a.mux`, which is wrapped in `debug.Middleware + recoverMiddleware + chi.NewRouter`), and captures the recorder's status + headers (subset: Content-Type, Location, X-Request-Id, HX-Trigger, HX-Redirect) + body (capped at 64 KB with `body_truncated: true` when oversized). Trims the body's leading `/` if missing. |

**Decisions settled during implementation:**

1. **Routes come from AST, not the live mux.** `a.mux` is
   wrapped in `debug.Middleware` + `recoverMiddleware`, so the
   chi router is not directly reachable. We AST-walk
   `routes.go` (same source-of-truth file `routes_method_guard_test.go`
   uses) instead. This also gives CLI + tests a consistent view.

2. **Wildcard route matching is literal-pattern style.** `/*`
   is treated as prefix-match; `{name}` is treated as any
   single segment. The matcher is in `matchChiPattern`. Method
   families (POST vs GET) aren't enforced here — that's the
   job of `TestRouteMethodMatchesHandler`. The walker only
   checks route existence.

3. **`debug dump` reads, never writes.** The wrapper never
   calls `SetSystemConfig`, never updates `local_settings.json`,
   never inserts into `import_batches`. It's the closest
   thing to a support snapshot we have — surface everything
   useful but mutate nothing. Row counts surface as a
   map so new tables can be added in one line (see
   `inventoryRowQueries`).

4. **`debug request` honours `--data-dir` via env var
   precedence.** `--data-dir PATH` sets `DIXIEDATA_DATA_DIR`
   before `NewApp`, so `appdata.DefaultDir()` inside
   `startup()` picks it up. Same mechanism the Phase 6 admin
   subcommands will use. Precedence: CLI flag > env var >
   default. Centralised in `ApplyDebugDataDirOverride` so
   Phase 6 can call it too.

5. **`hx-trigger` is intentionally NOT linted.** Triggers
   are event names (`click`, `keyup`, `intersect once`, etc.)
   and would be impossible to validate without re-implementing
   htmx's grammar. The walker skips them.

6. **Empty `hx-target` values are skipped.** A template
   that emits `hx-target=""` (invalid htmx) is a separate
   lint concern — the walker only checks non-empty values
   that look like ID selectors (`#id`). Selectors like
   `this`, `body`, `closest .x` are also skipped because
   they're valid htmx selectors that aren't DOM IDs.

7. **Test-only writer = `io.Discard`.** Integration tests
   that exercise the real App against a temp data dir use
   `io.Discard` to keep the test output clean. The CLI
   path defaults to `os.Stdout` exactly like every other
   subcommand.

8. **Windows-specific test workaround.** The jobs registry
   opens `<dataDir>/jobs.jsonl` in append mode and does not
   close the handle across `Shutdown`. On Windows that
   blocks `t.TempDir()`'s `RemoveAll` cleanup. Test helper
   `closeJobsLogWriter` reaches into the registry's
   unexported `logCloser` field via `reflect` + `unsafe.Pointer`
   and calls `Close()` before the temp dir is removed.
   Documented as test-only — production code calls
   `os.Exit` after Shutdown so this never matters.

**Lessons learned:**

- The chi router's pattern matcher uses `{param}` and
  `{name:regex}` shapes. For the existence check, we accept
  any `{...}` as a single segment. If a future route uses
  `{rest:.*}` (chi's catch-all syntax), the literal
  substitution breaks; Phase 8 can revisit if a template
  ever hits one.
- `debug dump` against an empty DB shows schema_version=55
  (CurrentSchemaVersion) and zero counts everywhere —
  useful for confirming the user's "I just created the data
  dir" claim. The wrapper intentionally fails loud (exit 1)
  if any table disappears rather than returning zero, so the
  support report stays trustworthy across schema drifts.
- `debug hx-invariants` AST-walking `routes.go` means the
  walker is compiler-independent — it survives templ
  regeneration, chi upgrades, and middleware reorderings.
  The same trick lets `debug browser-tree` work in any
  checkout where `internal/appshell/routes.go` exists.
- `debug request <path>` is the most useful subcommand for
  triage: it reproduces a browser request without the
  WebView2 race that bites the GUI, so support engineers
  can poke routes from a shell. Exit codes mirror the GUI:
  a missing route returns 404, a broken handler panics and
  `recoverMiddleware` returns 500.
- The `cli_query.go` pattern (parsed args + thin App
  dispatch) scales perfectly to four subcommands — total
  file size is ~800 lines including JSON renderers. No
  cobra/kingpin would have been cheaper here.

## Implementation order (final)

| # | Phase | Status | Commit |
|---|-------|--------|--------|
| 1 | smoke | ✅ | `df981a1` |
| 2 | doctor | ✅ | `7d9fc69` |
| 3 | list / show / search | ✅ | `e5f8e61` |
| 4 | export | ✅ | `e4474b7` |
| 5 | import (4 real kinds, --dry-run + --yes; pre-import restore-point REMOVED — see restore-point finding) | ✅ | `98ddb45` |
| 6 | migrate / backup / restore point / logs / config / --data-dir (restore-point sibling root FIRST so Phase 5 can re-enable safety net) | ⏳ | — |
| 7 | debug (incl. hx-invariants walker; read-only) | ✅ | `82bf194` |

## Cross-cutting concerns

### `--data-dir` flag

Not yet implemented anywhere. Trivial addition to each `RunXxx`
subcommand — `appdata.DefaultDir()` returns the default; we should
honour `--data-dir=<path>` and `DIXIEDATA_DATA_DIR` env var in every
subcommand. Phase 6 work.

### JSON envelope standardisation

Each subcommand defines its own `XxxResult` struct. They share
fields but aren't unified. Consider a top-level envelope:

```go
type CLIResult struct {
    Command    string        `json:"command"`
    Subcommand string        `json:"subcommand,omitempty"`
    StartedAt  string        `json:"started_at"`
    DurationMs int64         `json:"duration_ms"`
    Exit       int           `json:"exit"`
    Error      string        `json:"error,omitempty"`
    Data       json.RawMessage `json:"data,omitempty"` // subcommand-specific
}
```

Adding this in Phase 6 (when `config show` returns structured data)
unifies parsing for any downstream CI tool.

### Logging in headless mode

Headless mode currently routes logs through `internal/debug` to the
JSONL file at `<dataDir>/.dixiedata-logs/app.log.jsonl`. For
debugging a failed CLI invocation, the user has to know where the
log file is. Add a `--log-to-stderr` flag in Phase 6 that mirrors
the JSONL to stderr in real time. Cheap (one `io.MultiWriter`).

## Anti-goals (re-confirmed)

- **No TUI / REPL.** Stick to single-shot commands. REPL adds state
  management that doesn't earn its keep for our use cases.
- **No shell completion.** PowerShell users can wrap the JSON
  output. Saves implementing bash / zsh / fish completers.
- **No plugin system.** Every subcommand is compiled in.
- **No HTTP server in CLI mode.** CLI is single-shot. If you need a
  long-running process, that's the Wails app.

## Open questions

1. **`--data-dir` precedence:** env var, CLI flag, default. Doc the
   precedence order. (Easy; do in Phase 6.)
2. **Import atomicity for shared-archive:** is `--dry-run` enough, or
   do we need a "two-phase commit" with explicit `--apply`? The
   restore-point snapshot covers rollback but not parallel-import
   conflicts. Defer to Phase 5 design.
3. **Backup format versioning:** `.ddbak` zip embeds a
   `manifest.json` with `format: "1"` and `version: <n>`. When we
   bump the format, does `import backup` reject older versions or
   migrate? Currently rejects. Decision belongs with the import
   command.
4. **`--json` for shared-archive import:** when conflicts arise,
   the report is large. JSON is the right format. Phase 5.
5. **TUI for restore point resolution:** when an import has 50
   conflicts, plain text is painful. Anti-goal says no TUI, so we
   stick to JSON + `--filter-conflicts <field>=<value>` to narrow.
6. **`dixiedata package` for building `.ddbak` / `.ddsa` bundles
   from a directory tree** (not in any current user story, but
   would be useful for CLI-only archive assembly without the GUI).
   Defer indefinitely.

## What this enables (cumulative)

After Phase 7 ships:

- **CI** runs `dixiedata --smoke --json` after every release build
  (catches the `7dbff27` / `caf2c28626`-class bugs).
- **User support** runs `dixiedata doctor --fix` to repair a broken
  install without the GUI.
- **Scripting** exports every soldier PDF without clicks via
  `for id in $(dixiedata list soldiers --json | jq -r '.[].id');
  do dixiedata export pdf --soldier $id --out pdfs/$id.pdf; done`.
- **Scripting** imports the full archive via
  `dixiedata import backup --from $BACKUP --yes` and merges shared
  archives via `dixiedata import shared-archive --from $SHARE --yes`,
  all from a shell.
- **Disaster recovery** (backup import): re-import a different
  `.ddbak`. The original `.ddbak` IS the rollback artifact.
- **Disaster recovery** (shared-archive import): review pending
  conflicts in the merge-review UI. No automatic rollback yet;
  the restore-point sibling root is Phase 6 work that would
  unlock `restore point apply <id>` for shared-archive imports.
- **Integration tests** boot a real `*App`, drive imports + exports
  via the CLI, assert output files + DB state. No Wails, no
  Playwright, no GUI.
- **User support** runs `dixiedata debug dump --json` to
  capture a full archive snapshot for a bug report. Read-only,
  safe to ask users to run.
- **User support** runs `dixiedata debug hx-invariants` to
  surface broken `hx-target` / `hx-post` references without
  needing a browser. Exits non-zero on any violation so it's
  shell-friendly for CI / regression nets.
- **User support** runs `dixiedata debug request /some/path`
  to reproduce a browser request from a shell. Returns the
  same status + headers + body the GUI would see, without
  the WebView2 focus-race that bites the GUI.

## References

- `docs/agents/dialog-guard.md` — why `SaveFileDialog` is the
  reason CLI export is interesting.
- `docs/COMMON_BUGS.md` §4.5 — startup order. `smoke` checks
  this; `doctor` repairs it.
- `docs/COMMON_BUGS.md` §4.10 — release artifact packaging.
  `--smoke` is the verification step.
- `docs/CODE_CHANGES.md` — pre-mortem on the dialog-guard crash.
- `main.go` — the one place that gets the CLI/GUI dispatch.
- `internal/appshell/smoke.go` — Phase 1.
- `internal/appshell/doctor.go` — Phase 2.
- `internal/appshell/cli_query.go` — Phase 3.
- `internal/appshell/cli_export.go` — Phase 4.
- `internal/appshell/cli_import.go` — Phase 5.
- `internal/appshell/cli_debug.go` — Phase 7.
- `docs/agents/doctor-impl-notes.md` — design decisions for the
  doctor's `templates_parseable` check (Typst stub strategy).