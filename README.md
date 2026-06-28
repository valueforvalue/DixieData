# DixieData

DixieData is a local-first archive for Civil War research records. It keeps Person Records, Source Records, images, notes, and merge workflows in a SQLite-backed application, with printable reports, shareable archive packages, and restore-point safety for in-place updates.

This README is the developer / maintainer entry point. The end-user operating guide lives in [`docs/user-manual.md`](docs/user-manual.md). The on-disk data model and ubiquitous-language glossary live in [`CONTEXT.md`](CONTEXT.md).

## Stack

- **Language:** Go 1.x
- **GUI:** [Wails v2](https://wails.io) (v2.12.0) — Go backend + HTML/HTMX/Tailwind frontend, WebView2 on Windows
- **Templates:** [Templ](https://templ.guide) (`.templ` → generated `_templ.go` via `make tpl`)
- **Database:** SQLite via `database/sql` + `mattn/go-sqlite3`
- **PDF generation:** [Typst v0.15.0](https://github.com/typst/typst) (bundled, see `bin/MANIFEST.md`)
- **PDF rasterization / images:** PDFium (`chromium/7857`, downloaded on first build, see `bin/MANIFEST.md`)
- **HTTP routing:** [go-chi/chi](https://github.com/go-chi/chi)
- **Frontend:** Vanilla HTML + HTMX + Tailwind CSS (offline-compiled via `make css`)
- **Headless modes:** Go-only binaries (CLI, `dixiedata-web`, `dixiedata-tune`)

No JavaScript framework. No npm dependency at runtime — Tailwind and Playwright are dev-time only.

## Built-in developer tools

DixieData ships four distinct executables. They share the same Go code paths via `internal/appshell`, `pkg/exportbridge`, and `pkg/render`, so output is byte-identical across tools for the same input.

| Binary | Source | Purpose |
|---|---|---|
| `DixieData.exe` (or `dixiedata`) | `main.go` | The desktop GUI. Wails-driven. |
| `dixiedata <subcommand>` | `main.go` (headless dispatch) | The same binary, with subcommand verbs. No GUI, no Wails. See **CLI** below. |
| `dixiedata-tune.exe` | `tools/tune/` | Dev tool for iterating on Typst PDF templates. See **Tune** below. |
| `dixiedata-web.exe` | `cmd/dixiedata-web/` | Boots the same app on a plain HTTP listener so the UI can be driven by a headless browser (Playwright, axe-core, manual smoke). See **Web mode** below. |

Build all four with `make help` (see the Makefile). Most-used targets: `make test`, `make debug`, `make release`, `make run`, `make tune`, `make audit`.

### CLI — `dixiedata <subcommand>`

Single-shot headless commands, no GUI. Every subcommand accepts `--json` (stable JSON envelope) and `--data-dir PATH` (override the data dir; precedence CLI flag > `DIXIEDATA_DATA_DIR` env var > default).

The full roadmap lives in [`docs/agents/cli-plan.md`](docs/agents/cli-plan.md). All seven phases are shipped.

| Subcommand | Phase | Purpose |
|---|---|---|
| `--smoke` / `DIXIEDATA_SMOKE=1` | 1 | Headless boot checks; JSON-friendly; runs in CI. |
| `doctor` (`--json`, `--fix`) | 2 | Diagnose / repair a broken install. `doctor --fix` runs without the GUI. |
| `list soldiers [--limit N] [--page N]` | 3 | Paginated soldier list. |
| `show soldier <id-or-display-id>` | 3 | Full record dump. Auto-detects numeric ID vs display ID. |
| `search soldiers <query>` | 3 | Substring search. |
| `export pdf --soldier <id> --out <path>` | 4 | Soldier PDF (landscape by default). |
| `export pdf --month YYYY-MM --out <path>` | 4 | Calendar-month PDF. |
| `export pdf --full [--settings <json>] --out <path>` | 4 | Full-archive PDF. |
| `export jpg --soldier <id> --out <path>` | 4 | Soldier image-card JPG. |
| `export json --out <path>` | 4 | Full archive as JSON. |
| `export csv --out <path>` | 4 | Full archive as CSV. |
| `export ical --out <path>` | 4 | Calendar items as iCalendar. |
| `export static-archive --out <dir>` | 4 | Read-only browser-viewable archive. |
| `export backup --out <file.ddbak>` | 4 | Full-replacement backup. |
| `export shared-archive --out <file.ddshare>` | 4 | Merge-oriented archive package. |
| `import backup --from <file.ddbak> [--dry-run\|--yes]` | 5 | Restore from `.ddbak`. Destructive; needs `--yes`. |
| `import shared-archive --from <file.ddshare> [--dry-run\|--yes]` | 5 | Merge from `.ddshare`. Destructive; takes a sibling-root restore-point snapshot before merging. |
| `import images --soldier <id-or-display-id> --from <file>...` | 5 | Attach images to a soldier. Additive; no `--yes` needed. |
| `import memorial-json --from <file.json> [--dry-run]` | 5 | Import Find A Grave memorial JSON. Additive. |
| `migrate status` | 6 | Print schema version + pending. |
| `migrate up` | 6 | Apply pending migrations (idempotent). |
| `backup list` | 6 | Pre-schema-upgrade backup snapshots. |
| `backup prune [--keep-last N]` | 6 | Trim the backup index. |
| `restore point list` | 6 | In-place update restore points. |
| `restore point create [--note T] [--root PATH]` | 6 | Snapshot live archive. `--root` writes to a sibling of the data dir (used by `import shared-archive`). |
| `restore point apply <id>` | 6 | Print the record. (Real apply is a follow-up item.) |
| `logs path` | 6 | Path to the JSONL log file. |
| `logs tail [--follow] [--lines N]` | 6 | Tail the log. `--follow` polls and survives rotation. |
| `config show` | 6 | `local_settings.json` snapshot. |
| `config set <key> <value>` | 6 | Mutate a known key. Only `debug_mode` is currently accepted. |
| `debug dump` | 7 | Full archive inventory (counts, schema, identity). Read-only. |
| `debug hx-invariants` | 7 | AST-walk every `.templ` file; check `hx-target` / `hx-post` etc. against registered routes. Exit 1 on any violation. |
| `debug browser-tree` | 7 | Print the registered route tree, grouped by method. |
| `debug request <path>` | 7 | Dispatch a headless HTTP request; print status + headers + body. (Windows + Git Bash: prefix with `MSYS_NO_PATHCONV=1`.) |

**Standard exit codes** (across all subcommands):

- `0` — success
- `1` — invalid args / invariant failure (e.g. `debug hx-invariants` violation)
- `2` — environment error (file not found, DB open failed)
- `3` — usage error (parser rejection; the user typed something wrong)
- `4` — permission (reserved, not yet used)
- `5` — internal error (reserved, not yet used)

### Tune — `dixiedata-tune`

A standalone Go module under `tools/tune/` for iterating on the Typst PDF templates without rebuilding the full app. Renders templates through the same code path the appshell uses (via `pkg/exportbridge`), so a PDF produced by `tune` is byte-identical to one produced by the GUI for the same inputs. Locked by `internal/exportcontract` snapshot tests.

Build: `make tune`. The full subcommand reference lives in [`tools/tune/README.md`](tools/tune/README.md). Headline subcommands:

- `render --template <name> --mode {record,bulk} [--record <id>] [--out <path>]` — render once
- `watch --template <name> --out <path>` — re-render on every `templates/*.typ` change (designer loop)
- `diff --before <a.pdf> --after <b.pdf>` — diff two existing PDFs (text + page count)
- `list-templates` / `list-records` / `print-defaults` — discovery helpers

### Web mode — `dixiedata-web`

Boots the same `*App` over plain HTTP so the UI can be driven by a headless browser without the Wails WebView2 wrapper. Routes, handlers, templ templates, `app.js`, and `app.css` are all real — this is the production app served over HTTP.

Build: `go build -o build/bin/dixiedata-web.exe ./cmd/dixiedata-web`. Run: `build/bin/dixiedata-web.exe -data-dir <path> -addr :8080` (then point a browser at `http://localhost:8080`).

**Not for production.** The web-mode app uses `context.Background()` for the appshell ctx, so any handler that calls Wails dialog APIs will panic. Read-only browsing routes do not call them. The audit harness uses web mode exclusively (see below).

### Audit — `audit/`

Playwright + axe-core + custom DOM heuristics that drive web mode in a headless browser. Used for the UI/UX audit rounds documented in `docs/agents/`. Run: `npm run audit` (see `audit/README.md`). The audit tool is a dev-time dep; the production app has zero JS bundle beyond the offline Tailwind CSS.

## Build and validation

The Makefile is the preferred entry point. PowerShell scripts under `scripts/` remain the underlying implementation that the Makefile wraps. `make help` lists every target. The most common:

- `make test` — `go test ./... -short -count=1`
- `make tpl` — regenerate `*_templ.go` after editing `.templ` files
- `make css` — regenerate `frontend/app.css` after Tailwind class changes
- `make debug` — debug build via `scripts/build-debug.ps1`
- `make release` — release build (executable only)
- `make archive` — release build + versioned zip in `release/`
- `make demo` — seeded demo release package
- `make run` — build + launch debug build with UI IDs enabled
- `make tune` — build the `dixiedata-tune` developer tool
- `make stress` / `make goldmaster` — full test suites
- `make audit` — Playwright + axe-core UI/UX audit
- `make clean` / `make log-clean` — maintenance

Verbose build output is captured to `build/log/<target>.log` for post-mortem; `make` itself only surfaces pass/fail.

The build scripts (`scripts/build-common.ps1`) restore `build/bin/google-oauth-defaults.json` from the repo root if present, fetch the PDFium runtime into `build/bin/`, and refuse to install on SHA256 mismatch. See [`bin/MANIFEST.md`](bin/MANIFEST.md) for the pinned binary versions.

## How to get oriented

- `CONTEXT.md` — domain glossary (Person Record, Local Archive, Shared Archive, Source Record, Claim, Finding, etc.)
- `AGENTS.md` — agent entry point: file map, glossary pointers, agent-skills overview
- `AGENT_ARCHITECTURE_MAP.md` — structural map of the current Deep Modules, Grey Box boundary, Facades, and automation entrypoints
- `docs/agents/cli-plan.md` — the CLI roadmap (Phases 1-7, all shipped) + open follow-up
- `docs/user-manual.md` — end-user operating guide
- `docs/RELEASING.md` — the version-bump + GitHub release workflow
- `bin/MANIFEST.md` — pinned versions + SHA256s for every shipped third-party binary

## Releases

The current production line is derived from `internal/db/schema.go` via `db.GetAppVersion()`. Each release commit bumps the schema version (and therefore the app version), regenerates the snapshot tests, and archives a zip in `release/`. See `docs/RELEASING.md` for the full procedure.

## License

DixieData source code is released under the **MIT License**. See [`LICENSE`](LICENSE) for the full text.

The release binaries bundle the following third-party components, each under its own license. The build pipeline pins every version + SHA256 in [`bin/MANIFEST.md`](bin/MANIFEST.md) so a fresh clone can reproduce the build deterministically.

| Component | License | Source | Bundled? |
|---|---|---|---|
| [Wails v2](https://wails.io) (v2.12.0) | MIT | `github.com/wailsapp/wails/v2` | Statically linked into `DixieData.exe`. WebView2Loader statically linked per [Microsoft's static-link guidance](https://learn.microsoft.com/en-us/microsoft-edge/webview2/how-to/static); the WebView2 runtime itself is supplied by the OS on Windows 10+ / Windows 11. |
| [Typst](https://github.com/typst/typst) (v0.15.0) | Apache-2.0 | `github.com/typst/typst` | `bin/typst-windows.exe` (vendored, committed to the repo). Pinned SHA256 in `bin/MANIFEST.md`. |
| PDFium (`chromium/7857`) | Apache-2.0 (Chromium project) | `github.com/bblanchon/pdfium-binaries` | `build/bin/pdfium.dll` (downloaded on first build by `Restore-DixieDataPdfiumBinary`; SHA256-verified). |
| [go-chi/chi](https://github.com/go-chi/chi) | MIT | `github.com/go-chi/chi` | Go module dependency. |
| [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) | MIT | `github.com/mattn/go-sqlite3` | Go module dependency; bundles SQLite (public domain). |
| [Templ](https://templ.guide) | MIT | `github.com/a-h/templ` | Code-gen only; no runtime artifact ships. |
| [HTMX](https://htmx.org) | BSD-2-Clause | frontend `app.js` | Vendored as a static asset. |
| [Tailwind CSS](https://tailwindcss.com) | MIT | `frontend/app.css` | Compiled offline; the generated CSS bundle is the only artifact. |

When redistributing `DixieData.exe` (release builds), the licenses above apply to the bundled components. The Typst and PDFium Apache-2.0 notices should be shipped alongside the binary per each license's terms — for now, they live at the top of `bin/MANIFEST.md` and can be copied verbatim into a release-notes file or `THIRD-PARTY-NOTICES.txt`.
