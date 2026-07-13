# dixiedata-tune

A developer tool for iterating on the Typst-based PDF export templates. Opens a DixieData SQLite and renders templates through the same code path the appshell uses (via `pkg/exportbridge`) so a PDF produced by tune is byte-stable across runs (SOURCE_DATE_EPOCH pinned in `pkg/render/renderers.go`) and rendered through the same export pipeline as the appshell.

Issue: [#69](https://github.com/valueforvalue/DixieData/issues/69). Slice follow-ups: #358 (event), #430 (article), #447 (seed-data), #515 / #516 / #517 / #518 (audit fixes — June 2026).

## Build

```sh
make tune
# or, manually:
cd tools/tune && go build -o bin/dixiedata-tune .
```

The binary lives at `tools/tune/bin/dixiedata-tune` (or `dixiedata-tune.exe` on Windows).

## Subcommands

```
dixiedata-tune [global flags] <subcommand> [flags]
```

Global flags:

- `--db PATH` — path to the DixieData data directory (the one containing `dixiedata.db`). Defaults to `$DIXIEDATA_DB`. **Strict mode (issue #516):** if the resolved `dixiedata.db` does not exist, the binary fails fast with `no dixiedata.db found at <path>` rather than silently `MkdirAll`-creating a phantom empty db. Set `DIXIEDATA_TUNE_DB_CREATE=1` to opt in to the legacy auto-create behavior (e.g. seed-data bootstrap flows).
- `--typst PATH` — path to the typst binary. Defaults to `<repo>/bin/typst-windows.exe`.
- `--templates PATH` — path to the templates directory. Defaults to `<repo>/templates`.
- `--data-dir PATH` — directory for image resolution. Defaults to `--db`.

Subcommands:

- `render` — render a template against a record (Person / Event / Article) or the bulk archive
- `watch` — re-render on `templates/*.typ` change
- `diff` — diff two existing PDFs (text + page count). Requires the Poppler CLI (`pdftotext` + `pdfinfo` in `PATH`); on Windows install via MSYS2 or conda and document the prereq. Falls back to a raw-byte length comparison (essentially useless) when Poppler is missing — install it for usable diffs.
- `anniversary` — render the monthly anniversary report (issue #188; `--month N` 1–12)
- `insights` — render the archive summary / analytics report (issue #195)
- `list-templates` — list discovered typst templates
- `list-records` — list records in `--db` (`--kind soldier|article|event`; issue #518)
- `print-defaults` — print the appshell's default flag set (bulk or record)
- `doctor` — preflight gate (issue #515 slice D3): checks typst binary + version, templates dir resolves, `--db` opens, seed fixture present, snapshot suites green. Run before iterating to catch missing setup.
- `--version` — print tune version + typst version + bridge module version (issue #515 slice D2).

## Usage

```sh
# Discover what templates exist
dixiedata-tune list-templates

# List records in the local DixieData SQLite
dixiedata-tune --db ~/.dixiedata list-records

# Render a single template against one record
dixiedata-tune --db ~/.dixiedata render \
    --template soldier_landscape --mode record --record 54 \
    --out out.pdf

# Render a single Event Record (issue #358; uses the same
# path as the appshell's /events/{id}/pdf handler).
dixiedata-tune --db ~/.dixiedata render \
    --template event_landscape --mode event --record 12 \
    --orientation L \
    --out event.pdf

# Render a single Article Record (issue #430; uses the same
# path as the appshell's /articles/{id}/pdf handler).
dixiedata-tune --db ~/.dixiedata render \
    --template article_landscape --mode article --record 1 \
    --orientation L \
    --out article.pdf

# List Article ids (issue #430)
dixiedata-tune --db ~/.dixiedata list-records --kind article

# List Event ids (issue #518)
dixiedata-tune --db ~/.dixiedata list-records --kind event

# Render the full bulk archive
dixiedata-tune --db ~/.dixiedata render \
    --template bulk_soldier --mode bulk \
    --out bulk.pdf

# Render with grouping (divider pages between groups)
dixiedata-tune --db ~/.dixiedata render \
    --template bulk_soldier --mode bulk \
    --group-by-pension-state \
    --out grouped.pdf

# Render the monthly anniversary report for July
dixiedata-tune --db ~/.dixiedata anniversary --month 7 --orientation L \
    --out july.pdf

# Render the archive summary / analytics
dixiedata-tune --db ~/.dixiedata insights --orientation P \
    --out insights.pdf

# Re-render every time a .typ file in templates/ changes
dixiedata-tune --db ~/.dixiedata watch \
    --template bulk_soldier --mode bulk --record-ids 1,2,3,4,5 \
    --out preview.pdf

# Diff two existing PDFs (requires Poppler: pdftotext + pdfinfo)
dixiedata-tune diff --before out-before.pdf --after out-after.pdf

# Print the appshell's default flag set (copy-paste to reproduce)
dixiedata-tune print-defaults --mode bulk

# Print the binary version + its dependencies
dixiedata-tune --version

# Preflight: are all the moving parts in place before I start iterating?
dixiedata-tune doctor
```

## SVG / PNG output

All `render` / `anniversary` / `insights` invocations infer the output format from the `--out` extension. Default (no recognised extension or `.pdf`) writes PDF. `.svg` writes per-page SVG files (`out-1.svg`, `out-2.svg`, ...) via the renderer's `TYPST_KEEP_WORKDIR` hook. `.png` writes per-page PNG files the same way. Single-page output lands at `--out`; multi-page output drops pages 2..N as `{stem}-2.{ext}`, `{stem}-3.{ext}`, ... next to the primary output.

```sh
# Single-page SVG preview
dixiedata-tune --db ~/.dixiedata render --template soldier_landscape \
    --mode record --record 1 --out preview.svg

# Multi-page PNG (pages 2..N land as preview-2.png, preview-3.png, ...)
dixiedata-tune --db ~/.dixiedata render --template bulk_soldier \
    --mode bulk --record-ids 1,2,3,4,5 --out preview.png
```

## How it works

`tools/tune/` is a separate Go module (`github.com/valueforvalue/DixieData/tools/tune`). It uses Go's `replace` directive to import from the main DixieData module:

- `pkg/exportbridge` — the canonical facade. Both the appshell and tools/tune drive the same `BulkRenderer.RenderBulk` / `RenderSingle` / `RenderEventSingle` / `RenderArticleSingle` / `RenderAnniversary` / `RenderInsights`. Rendered through the same export pipeline as the appshell.
- `pkg/render` — the typst renderer (pinned `SOURCE_DATE_EPOCH=1577836800` for byte-stable PDF metadata).
- `internal/models` — the `models.Soldier` / `models.Article` types.

The tool shells out to the bundled Typst binary in `bin/` via `exec.Command` directly (no go-typst wrapper). This keeps the Windows build free of console-window flashes during render.

## Snapshot determinism

The renderer's `SOURCE_DATE_EPOCH` pin makes PDF metadata (CreationDate, ModDate) stable across runs. The snapshot suites assert two stronger invariants per case (issue #517 slice A4):

1. **Determinism self-check** — render the case twice and assert byte-equality. A non-determinism regression (time / UUID / map-iteration order leaking into the typst data payload) fails here with a distinct message: `do NOT regen the golden, fix the determinism bug first`.
2. **Golden match** — compare against the checked-in snapshot. A genuine surface change fails here with the standard `snapshot mismatch` message.

The two checks live in:

- `internal/exportcontract/snapshots_test.go::TestArchiveContractSnapshots` (in-process via `pkg/exportbridge`)
- `internal/exportcontract/cli_contract_test.go::TestCLIContractSnapshots` (shells out to the actual `dixiedata-tune` binary)
- `tools/tune/snapshot_test.go::TestTuneRecordLandscapeSnapshot` (single golden for soldier id=1)

Regenerate via `make tune-snapshots` (or `UPDATE_SNAPSHOTS=1 go test -count=1 ./internal/exportcontract/ ./tools/tune/...`) and then a no-update rerun to verify byte-stability.

```sh
go test -count=1 ./internal/exportcontract/ -v
go test -count=1 ./tools/tune/...
```

## Make targets

- `make tune` — build the binary
- `make tune-smoke` — render the live `.dixiedata/` archive (smoke test, no byte comparison)
- `make tune-snapshots` — regenerate the snapshots and verify byte-stability

## License

DixieData is licensed under the same terms as the main project.