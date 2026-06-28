# CLI subcommand plan — `dixiedata <cmd>`

This is the canonical roadmap for DixieData's headless CLI surface.
Phases ship one at a time; this file updates after each ship.

**Status snapshot** (last update after commit `a1e6222`):

| Phase | Status | Commit |
|-------|--------|--------|
| 1 — `--smoke` | ✅ shipped | `df981a1` |
| 2 — `doctor` | ✅ shipped | `7d9fc69` |
| 3 — `list` / `show` / `search` | ✅ shipped | `e5f8e61` |
| 4 — `export ...` | ✅ shipped | `e4474b7` |
| 5 — `import ...` | ✅ shipped | `98ddb45` |
| 5b — pre-import restore-point safety net (re-enabled via sibling root) | ✅ shipped | `35ab425` |
| 6 — admin subcommands + `--data-dir` + sibling restore-point root | ✅ shipped | `91dc8b8` |
| 7 — `debug ...` (dump / hx-invariants / browser-tree / request) | ✅ shipped | `5b32510` (merged `a1e6222`) |

**All seven phases shipped.** The CLI is feature-complete per this
plan. Open follow-up work is now tracked in this file's
"Follow-up" section below, not as separate phases.

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
4. `appshell.HasImportSubcommand` — `import ...`
5. `appshell.HasAdminSubcommand` — `migrate` / `backup` /
   `restore point` / `logs` / `config`
6. `appshell.HasDebugSubcommand` — `debug ...`
7. `appshell.HasSmokeFlag` / `EnvRequestsSmoke` — `--smoke` / `DIXIEDATA_SMOKE=1`
8. fall through to Wails GUI

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
  (default: standard location via `appdata`). Precedence:
  **CLI flag > env var > default**. Shipped in commit `91dc8b8`.
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

### Phase 6 — admin subcommands ✅ shipped (commit `91dc8b8`)

```
dixiedata migrate status
dixiedata migrate up
dixiedata backup list
dixiedata backup prune --keep-last N
dixiedata restore point list
dixiedata restore point create [--note <text>] [--root <path>]
dixiedata restore point apply <id>          # placeholder: prints record
dixiedata logs path
dixiedata logs tail [--follow] [--lines N]
dixiedata config show
dixiedata config set <key> <value>
```

All commands accept `--data-dir PATH` (sets
`DIXIEDATA_DATA_DIR` before `App.Startup` so
`appdata.DefaultDir()` picks it up via the existing env-var
fallback in `internal/appdata/appdata.go`) and `--json` for
a stable JSON envelope.

**Dispatch table — every subcommand hits an existing service:**

| Command | Implementation |
|---------|---------------|
| `migrate status`        | `db.Open(dataDir)` + `PRAGMA user_version` query. Prints applied vs current schema, app build, status (up-to-date / pending). |
| `migrate up`            | `db.Open(dataDir)` twice (first to read pre-state, second to trigger `applySchema`). applySchema short-circuits if already at `CurrentSchemaVersion`, so this is a no-op on up-to-date DBs. |
| `backup list`           | `update.NewRetainedBackupManager(dataDir).List()` — pre-schema-upgrade snapshots, newest-first. |
| `backup prune`          | Loop: keep first `--keep-last N` (default 5, matches `defaultMaxRetainedBackups`), `os.RemoveAll` the rest. Same shape `Housekeeping()` would use, but invoked on demand. |
| `restore point list`    | `a.restorePoints.List()` — the in-place update manager wired in `lifecycle.go`. |
| `restore point create`  | Default: `a.restorePoints.Create(...)` writing to `<dataDir>/updates/restore-points/<id>/`. With `--root PATH`: `update.NewSiblingRestorePointManager(dataDir, root).Create(...)` writing to `<root>/<id>/`. Both call `a.backup.Export` for the local-archive snapshot. |
| `restore point apply <id>` | `a.restorePoints.Get(id)` — prints the record. **Apply is NOT YET wired** (see "Open follow-up" below). |
| `logs path`             | `appdata.AppLogPath(dataDir)` — single line. |
| `logs tail`             | `os.ReadFile` the JSONL, `bufio.Scanner`, last N lines. `--follow` polls every 250ms, reopens on rotation (inode change detected via size rollback). |
| `config show`           | `records.LoadLocalSettings(dataDir)`. |
| `config set <key> <value>` | `LoadLocalSettings` + `SaveLocalSettings`. Only known keys accepted via `isKnownConfigKey` (currently `debug_mode`) — fails loudly on typos. |

**`migrate down` is NOT shipped** — the schema-version-down
path is best-effort and would need a fresh `--yes` guard plus
a pre-migration snapshot. Phase 6 ships `status` + `up` (which
is the same as opening the DB). The `down` command can land
when the schema actually has a real down-migration story.

**`restore point apply` is NOT YET wired** — the manager has
no public Apply because the in-place update flow does the
actual restore (downgrade via `internal/update` scripts). For
Phase 6 we print the record so the user can manually `cp` the
`local-archive.ddbak` aside and
`dixiedata import backup --from <copy> --yes` to apply. A real
`apply` is the "Open follow-up" item below.

**Sibling restore-point root** — the design that unblocks
Phase 5's pre-import safety net:

- `NewRestorePointManager(dataDir)` UNCHANGED. The in-place
  update flow keeps using it (binary swap doesn't move the
  data dir, so the data-dir-resident root is correct for it).
- `NewSiblingRestorePointManager(dataDir, root)` adds a `root`
  field. When set:
    - `restorePointsRoot()` returns `root` (not
      `appdata.UpdateRestorePointsDir(dataDir)`)
    - `pathBase()` returns empty (paths stored in the record
      are `<id>/local-archive.ddbak`, NOT
      `updates/restore-points/<id>/local-archive.ddbak`) so
      they resolve cleanly under `root` via `absolutePath()`
    - `absolutePath()` joins with `root`, not `dataDir`
  When `root` is empty (in-place manager), `pathBase()` returns
  `updates/restore-points` and `absolutePath()` joins with
  `dataDir` — identical to the previous behavior.

Locked regression test:
`TestRestorePointManagerSiblingRootSurvivesDataDirRename` in
`internal/update/restore_point_manager_test.go` creates a
sibling manager, creates a restore point, RENAMES the data
dir (simulating `archive.replaceDataDir`), asserts the
sibling restore point is still on disk. This is the test that
would have failed under the old in-place manager (and did
fail during Phase 5's live testing, which is what kicked off
Phase 6's sibling-root work).

### Phase 7 — debugging ✅ shipped (commit `5b32510`, merged in `a1e6222`)

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
   subcommands use (`main.go`'s `firstDataDir` helper is
   the canonical implementation; `ApplyDebugDataDirOverride`
   in `cli_debug.go` is the package-internal equivalent
   for debug subcommands). Precedence: CLI flag > env var >
   default.

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
| 4 | export (9 subcommands; pdf / jpg / json / csv / ical / static-archive / backup / shared-archive) | ✅ | `e4474b7` |
| 5 | import (4 real kinds, --dry-run + --yes; pre-import restore-point REMOVED) | ✅ | `98ddb45` |
| 5b | re-enable pre-import restore-point safety net (sibling root + shared-archive merge rollback) | ✅ | `35ab425` |
| 6 | admin subcommands + --data-dir + sibling restore-point root (migrate down + restore point apply not yet wired — see follow-up) | ✅ | `91dc8b8` |
| 7 | debug (dump / hx-invariants / browser-tree / request; read-only) | ✅ | `5b32510` (merged `a1e6222`) |

## Cross-cutting concerns

### `--data-dir` flag

✅ Shipped in commit `91dc8b8`. Implemented in `main.go`'s
`firstDataDir` helper, called by `runExportSubcommand` /
`runImportSubcommand` / `runAdminSubcommand` /
`runDebugSubcommand` (Phase 7 ships its own
`appshell.ApplyDebugDataDirOverride` with the same semantics).
Sets `DIXIEDATA_DATA_DIR` BEFORE `a.Startup(ctx)` so
`appdata.DefaultDir()` picks it up via the existing env-var
fallback in `internal/appdata/appdata.go`. Precedence:
**CLI flag > env var > default**.

### JSON envelope standardisation

Each subcommand still defines its own `XxxResult` struct
(idiomatic for the per-subcommand output format). Top-level
unification deferred indefinitely — the per-subcommand shapes
are stable enough that downstream CI can parse them
individually, and a top-level envelope would force every
subcommand to wrap its data, losing the `jq -r '.soldiers'`
ergonomics of the current shapes.

### Logging in headless mode

`--log-to-stderr` still NOT shipped. Headless mode routes logs
through `internal/debug` to the JSONL file at
`<dataDir>/.dixiedata-logs/app.log.jsonl`. For debugging a
failed CLI invocation, the user has to know where the log
file is (`dixiedata logs path` reveals it, but real-time
mirroring to stderr would be cheaper). Cheap to add: one
`io.MultiWriter` swap. Defer to follow-up.

## Anti-goals (re-confirmed)

- **No TUI / REPL.** Stick to single-shot commands. REPL adds state
  management that doesn't earn its keep for our use cases.
- **No shell completion.** PowerShell users can wrap the JSON
  output. Saves implementing bash / zsh / fish completers.
- **No plugin system.** Every subcommand is compiled in.
- **No HTTP server in CLI mode.** CLI is single-shot. If you need a
  long-running process, that's the Wails app.

## Open follow-up (all phases shipped, these are next)

Tracked separately from the phase plan now. Each item is small
enough for its own commit.

1. **`restore point apply` — real implementation.** Currently
   prints the record; the actual restore would either
   re-invoke `dixiedata import backup --from <copy>` (clunky)
   or refactor `archive.replaceDataDir` to accept an
   alternate target (cleaner). Block on whether we need
   in-process restore (CLI script-friendly) or only
   manual-copy-then-import (user-driven).

2. **`migrate down <version>`.** Best-effort undo path;
   needs a `--yes` guard plus a pre-migration snapshot.
   Schema actually has a real down-migration story would
   unblock this.

3. **`--log-to-stderr`.** Mirror JSONL to stderr in real
   time via `io.MultiWriter`. Cheap.

4. **JSON envelope unification.** See "Cross-cutting
   concerns" above. Parked indefinitely.

5. **`dixiedata package` for building `.ddbak` / `.ddsa`
   bundles from a directory tree.** Not in any current user
   story. Defer indefinitely.

6. **TUI for restore point resolution.** When an import has
   50 conflicts, plain text is painful. Anti-goal says no
   TUI, so we stick to JSON +
   `--filter-conflicts <field>=<value>` to narrow (not
   shipped; not blocking).

7. **Backup format versioning.** `.ddbak` zip embeds a
   `manifest.json` with `format: "1"` and `version: <n>`.
   When we bump the format, does `import backup` reject
   older versions or migrate? Currently rejects. Decision
   belongs with the import command when a real format
   bump happens.

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
- **Disaster recovery** (shared-archive import): a sibling
  restore-point snapshot at `<parent>/.dixiedata-restore-points/<id>/`
  is taken BEFORE the merge (commit `35ab425`); the ID is
  printed in import output. A real `restore point apply <id>`
  is parked in follow-up; for now, manual rollback is
  `dixiedata import backup --from <archived-.ddbak> --yes`
  (the .ddbak in the sibling dir IS the pre-merge snapshot).
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
  the WebView2 focus-race that bites the GUI. (Note: under
  Git Bash on Windows, prefix with `MSYS_NO_PATHCONV=1` so
  `/calendar` is not converted to a Windows path.)

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
- `internal/appshell/cli_admin.go` — Phase 6.
- `internal/appshell/cli_debug.go` — Phase 7.
- `internal/update/restore_point_manager.go` — sibling
  manager added in Phase 6, unlocks Phase 5's pre-import
  safety net (re-enabled in commit `35ab425`).
- `docs/agents/doctor-impl-notes.md` — design decisions for the
  doctor's `templates_parseable` check (Typst stub strategy).