# CLI subcommand plan — `dixiedata <cmd>`

This is the design for growing DixieData from "GUI-only" to
"GUI + headless CLI" without code duplication. The CLI is
implemented as subcommands on the same binary, dispatching to
existing `*App` methods rather than new HTTP/handler paths.

Status: **Phase 1 (`--smoke`) ships in the next round.** Phases
2-4 below are the follow-on backlog. Don't implement them yet;
file as GitHub issues and pick up after the next bug sweep.

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

`parseSubcommand` returns `nil, false` when args are empty (GUI
launch). It returns `&cliCommand{Name: "smoke", ...}, true` when
the first arg matches a known subcommand.

`runSubcommand` is a switch over the subcommand name. Each case
boots an `*appshell.App` (without Wails), calls the relevant
methods, prints results, returns exit code.

### Dispatch style

Plain `os.Args[1:]` parsing — no cobra, no kingpin. Reasons:

- DixieData already avoids large dep trees (see `go.mod`).
- We have ≤10 subcommands. Hand-rolled is < 150 lines.
- Cobra adds ~300kb to binary + reflection cost on every parse.
- Easier to keep the smoke path audit-friendly.

If we ever hit >20 subcommands, revisit.

### Flag conventions

- All subcommands support `--json` for machine-readable output
- All subcommands support `--help` (auto-generated from a docstring table)
- All subcommands accept `--data-dir <path>` to override the
  data dir (default: standard location via `appdata`)
- Bool flags: `--verbose`, `--dry-run`
- No `--yes` / `-y`. Destructive operations prompt.

### Exit codes

| Code | Meaning |
|------|---------|
| 0    | Success |
| 1    | Operation failed (e.g. export error) |
| 2    | Environment error (missing binary, data dir unwritable) |
| 3    | Usage error (unknown flag, missing arg) |
| 4    | Auth/permission error (read-only archive, etc.) |

### JSON output shape

```json
{
  "command": "smoke",
  "started_at": "2026-06-27T22:00:00Z",
  "duration_ms": 42,
  "exit": 0,
  "checks": [
    {"name": "data_dir_resolves", "passed": true,  "duration_ms": 2},
    {"name": "sqlite_open",       "passed": false, "duration_ms": 18,
     "error": "database is locked"}
  ]
}
```

Same shape for every subcommand. CI parses this; humans use the
default text output.

## Subcommand taxonomy

### Phase 1 — `smoke` (this round)

```
dixiedata --smoke
dixiedata --smoke --json
DIXIEDATA_SMOKE=1 dixiedata
```

8 checks:
1. `data_dir_resolves`
2. `logs_dir_separate`
3. `sqlite_open`
4. `migrations_applied`
5. `oauth_defaults_loaded`
6. `templates_dir`
7. `typst_binary`
8. `routes_registered`

File: `internal/appshell/smoke.go`. Caller: `main.go`.

### Phase 2 — `doctor` (next round)

```
dixiedata doctor
dixiedata doctor --check=data_dir --check=sqlite --json
```

Same check set as `--smoke`, but:
- Each `--check` is a separate fast invocation
- Adds 4 deeper checks:
  - `archive_writable` — can we create a Restore Point?
  - `feedback_log_open` — JSONL not corrupt
  - `pdfium_loadable` — `dlopen(pdfium.dll)` succeeds
  - `templates_parseable` — every `.typ` parses
- Adds `--fix` mode: runs repair operations (truncate feedback
  log, re-apply migrations, re-extract templates from zip).

File: `internal/appshell/doctor.go`. Shares `smokeCheck` shape
with `smoke.go`.

### Phase 3 — read-only subcommands

```
dixiedata list soldiers [--query Q] [--limit N] [--json]
dixiedata show soldier <display-id>
dixiedata list sources [--person <id>]
dixiedata search <query> [--limit N]
```

Dispatches to existing `*App` methods. No new logic — just HTTP
adapter. Useful for scripting research workflows.

### Phase 4 — export subcommands (no GUI dialog)

```
dixiedata export pdf --soldier <id> --out <path>
dixiedata export pdf --month 2026-06 --out <path>
dixiedata export pdf --full --out <path> [--settings <json>]
dixiedata export jpg --soldier <id> --out <path>
dixiedata export json --out <path>
dixiedata export csv --out <path>
dixiedata export ical --out <path>
dixiedata export static-archive --out <dir>
dixiedata export backup --out <file>
```

**Critical: bypasses `SaveFileDialog` entirely.** Uses an
`-out` flag for the destination. Reuses the existing
`ExportFullDatabasePDF` / `ExportSoldierPDF` etc. but routes
through a new `*App.ExportToPath(kind, opts, dest)` helper that
takes the path directly instead of opening a native dialog.

This solves the c1d9dc1-class bug **permanently for CLI use**
(CLI users never trigger the WebView2 race because there's no
WebView2). GUI users still benefit from the in-flight guard.

New helper: `internal/appshell/export_to_path.go`. Each handler
gets a CLI variant that:
1. Constructs the same `PrintSettings` payload
2. Calls the export service directly with the path
3. Returns a success/failure struct

### Phase 5 — import subcommands

```
dixiedata import backup --from <file> [--dry-run]
dixiedata import static-archive --from <dir>
dixiedata import shared-archive --from <file>
dixiedata import images --soldier <id> --from <file>...
dixiedata import feedback-log --from <file>
```

Dispatches to existing import handlers, with a `--dry-run`
default that shows what would change without modifying the
archive. **`--yes` is required for non-dry-run import of
backup/static/shared** because they overwrite data.

### Phase 6 — admin subcommands

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

Each maps to an existing `*App` method. `migrate` wraps the
SQLite migration runner. `backup` / `restore point` wrap the
existing `.ddbak` / restore-point machinery.

### Phase 7 — debugging

```
dixiedata debug dump > archive-summary.json
dixiedata debug hx-invariants    # runs the invariant walker
dixiedata debug browser-tree     # dumps DOM structure
dixiedata debug request <path>   # fires one HTTP request, dumps response
```

`debug` subcommands never write to the archive. Useful for
supporting users without the GUI.

## Implementation order

Recommended PR breakdown (each ships independently):

1. `smoke` (Phase 1) — this round
2. `doctor` (Phase 2) — next round, paired with the bootstrap test
3. `export pdf --soldier <id> --out <path>` (Phase 4, partial) — first non-trivial command; validates the `ExportToPath` helper pattern
4. `migrate status` (Phase 6) — small, isolated, useful
5. `export pdf --month / --full` (Phase 4, rest)
6. `list / show / search` (Phase 3)
7. `import backup / static-archive` (Phase 5)
8. `doctor --fix` (Phase 2, completion)
9. `backup` / `restore point` (Phase 6, rest)
10. `debug` (Phase 7)

## What this enables

- **CI** runs `dixiedata --smoke --json` after every release build
  (catches the `7dbff27` / `caf2c28626`-class bugs).
- **User support** runs `dixiedata doctor --fix` to repair a
  broken install without the GUI.
- **Scripting** exports every soldier PDF without clicks via
  `for id in $(dixiedata list soldiers --json | jq -r '.[].id');
  do dixiedata export pdf --soldier $id --out pdfs/$id.pdf; done`
- **Integration tests** boot a real `*App`, drive exports via
  the CLI, assert output PDFs exist + have correct content.
  No Wails, no WebView2, no Playwright.

## Anti-goals

- **No TUI / REPL.** Stick to single-shot commands. REPL adds
  state management that doesn't earn its keep for our use cases.
- **No shell completion.** PowerShell users can wrap the JSON
  output. Saves implementing bash / zsh / fish completers.
- **No plugin system.** Every subcommand is compiled in.
- **No HTTP server in CLI mode.** CLI is single-shot. If you
  need a long-running process, that's the Wails app.

## Open questions

- Should `--smoke` and `doctor` share code, or be separate?
  (Plan: share via `smokeCheck` + `runChecks([]Check)`. Doctor
  is a superset of smoke with `--check` filtering.)
- Should we ship a separate `dixiedata-cli` binary for
  environments without WebView2 (e.g. Linux servers)?
  (Plan: same binary, same flags. WebView2 only loads when GUI
  mode. CLI mode is pure Go + sqlite.)
- Should we expose `dixiedata package` for building `.ddbak`
  and `.ddsa` bundles from a directory tree?
  (Defer — not in any current user story.)

## References

- `docs/agents/dialog-guard.md` — why `SaveFileDialog` is the
  reason CLI export is interesting.
- `docs/COMMON_BUGS.md` §4.5 — startup order. `smoke` checks
  this; `doctor` repairs it.
- `docs/COMMON_BUGS.md` §8 — release artifact packaging.
  `--smoke` is the verification step.
- `main.go` — the one place that gets the CLI/GUI dispatch.