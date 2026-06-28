# CLI subcommand plan — `dixiedata <cmd>`

This is the canonical roadmap for DixieData's headless CLI surface.
Phases ship one at a time; this file updates after each ship.

**Status snapshot** (last update after commit `e4474b7`):

| Phase | Status | Commit |
|-------|--------|--------|
| 1 — `--smoke` | ✅ shipped | `df981a1` |
| 2 — `doctor` | ✅ shipped | `7d9fc69` |
| 3 — `list` / `show` / `search` | ✅ shipped | `e5f8e61` |
| 4 — `export ...` | ✅ shipped | `e4474b7` |
| 5 — `import ...` | ⏳ next | — |
| 6 — `migrate` / `backup` / `restore point` / `logs` / `config` | ⏳ backlog | — |
| 7 — `debug ...` | ⏳ backlog | — |

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

### Phase 5 — `import ...` ⏳ NEXT

```
dixiedata import backup --from <file> [--dry-run]
dixiedata import static-archive --from <dir> [--dry-run]
dixiedata import shared-archive --from <file> [--dry-run]
dixiedata import images --soldier <id> --from <file>...
dixiedata import feedback-log --from <file>
```

Dispatches to existing import handlers, with a `--dry-run`
default that shows what would change without modifying the
archive. **`--yes` is required for non-dry-run import of
backup/static/shared** because they overwrite data.

**File:** `internal/appshell/cli_import.go`.

**Dispatch target:**

| Command | App method |
|---------|-----------|
| `import backup` | `a.backup.ImportWithLocalIdentity(...)` or similar |
| `import static-archive` | `a.export.ExportStaticArchive`-shape import (new? check archive package) |
| `import shared-archive` | `a.backup.ImportSharedBackup(...)` |
| `import images` | `a.imports.handleImportSoldierImages` (HTTP handler) — adapt to service-level |
| `import feedback-log` | `appendFeedbackEntry` (existing in `app_feedback.go`) |

**Decisions to make during planning:**

1. **Where does `--dry-run` surface?** Each import handler has its
   own pre-flight checks. Probably easiest: add a `DryRun bool` to
   each import method signature and have it return a
   `DryRunReport{Changes []Change, Errors []error}` instead of
   mutating. That's invasive — alternative is a wrapper that calls
   the real import then rolls back. Rollback is risky for shared
   archives.

   Recommendation: add `DryRun` parameter to each method. The
   report is JSON-serializable so `--json` output is useful.

2. **`--yes` enforcement.** Without `--yes`, refuse to run if the
   import is destructive. Match the pattern from `git rebase` /
   `apt install` — default to dry-run + show what would happen, then
   prompt user. In CLI mode without a TTY, refuse unless `--yes`
   is set. Exit 4 (auth/permission) for refusal.

3. **Conflict resolution for shared-archive.** Existing
   `ImportSharedBackup` returns a `SharedImportSummary` that may
   include conflicts. CLI should print the conflict summary and ask
   user how to resolve (skip / merge / overwrite). For non-interactive
   use, a `--conflict=skip|merge|overwrite` flag picks the resolution
   up-front.

4. **Atomicity.** If an import fails halfway, does the data dir
   contain a partial state? Current code has no transaction wrapper
   for `backup.Import`. The restore-point machinery
   (`a.restorePoints.Create`) can be used to take a snapshot before
   import so the user can roll back. Phase 6 should add
   `dixiedata restore point apply <id>` before the import starts.

   **For Phase 5 minimum:** take an automatic restore point before any
   non-dry-run import; print the restore point ID so the user can
   roll back if something goes wrong.

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
  restore-point machinery. `restore point apply <id>` will be used
  by the Phase 5 import subcommand to roll back on failure.
- `logs tail` reads the JSONL file directly (last N lines). `--follow`
  tails like `tail -f`. Needs careful handling of file rotation.
- `config show` / `config set` operate on the SQLite-backed
  `local_settings` table.
- Add `--data-dir` flag support to all subcommands here (and Phase 7).

### Phase 7 — debugging ⏳ backlog

```
dixiedata debug dump > archive-summary.json
dixiedata debug hx-invariants
dixiedata debug browser-tree
dixiedata debug request <path>
```

`debug` subcommands never write to the archive. Useful for
supporting users without the GUI.

`debug hx-invariants` is the closest thing to a UI regression net
that doesn't need a browser. Walks every `.templ` file and checks
that `hx-target` IDs exist, `hx-post` routes are registered, etc.
(Connects to the Phase A test plan in the original bug-class
audit — `internal/templates/hx_invariant_test.go` was the planned
file.)

## Implementation order (final)

| # | Phase | Status | Commit |
|---|-------|--------|--------|
| 1 | smoke | ✅ | `df981a1` |
| 2 | doctor | ✅ | `7d9fc69` |
| 3 | list / show / search | ✅ | `e5f8e61` |
| 4 | export | ✅ | `e4474b7` |
| 5 | import (with --dry-run + --yes + auto restore point) | ⏳ next | — |
| 6 | migrate / backup / restore point / logs / config / --data-dir | ⏳ | — |
| 7 | debug (incl. hx-invariants walker) | ⏳ | — |

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

After Phase 5 ships:

- **CI** runs `dixiedata --smoke --json` after every release build
  (catches the `7dbff27` / `caf2c28626`-class bugs).
- **User support** runs `dixiedata doctor --fix` to repair a broken
  install without the GUI.
- **Scripting** exports every soldier PDF without clicks via
  `for id in $(dixiedata list soldiers --json | jq -r '.[].id');
  do dixiedata export pdf --soldier $id --out pdfs/$id.pdf; done`.
- **Disaster recovery** uses `dixiedata restore point apply <id>`
  to roll back after a bad import, all from a shell.
- **Integration tests** boot a real `*App`, drive imports + exports
  via the CLI, assert output files + DB state. No Wails, no
  Playwright, no GUI.

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
- `docs/agents/doctor-impl-notes.md` — design decisions for the
  doctor's `templates_parseable` check (Typst stub strategy).