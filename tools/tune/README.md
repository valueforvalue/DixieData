# dixiedata-tune

A developer tool for iterating on the Typst-based PDF export templates. Opens a DixieData SQLite and renders templates through the same code path the appshell uses (via `pkg/exportbridge`) so a PDF produced by tune is byte-identical to one produced by the appshell for the same inputs.

Issue: [#69](https://github.com/valueforvalue/DixieData/issues/69).

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

- `--db PATH` — path to the DixieData data directory (the one containing `dixiedata.db`). Defaults to `$DIXIEDATA_DB`.
- `--typst PATH` — path to the typst binary. Defaults to `<repo>/bin/typst-windows.exe`.
- `--templates PATH` — path to the templates directory. Defaults to `<repo>/templates`.
- `--data-dir PATH` — directory for image resolution. Defaults to `--db`.

Subcommands:

- `render` — render a template against a record or the bulk archive
- `watch` — re-render on `templates/*.typ` change
- `diff` — diff two existing PDFs (text + page count)
- `list-templates` — list discovered typst templates
- `list-records` — list records in `--db`
- `print-defaults` — print the appshell's default flag set (bulk or record)

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

# Render the full bulk archive
dixiedata-tune --db ~/.dixiedata render \
    --template bulk_soldier --mode bulk \
    --out bulk.pdf

# Render with grouping (divider pages between groups)
dixiedata-tune --db ~/.dixiedata render \
    --template bulk_soldier --mode bulk \
    --group-by-pension-state \
    --out grouped.pdf

# Re-render every time a .typ file in templates/ changes
dixiedata-tune --db ~/.dixiedata watch \
    --template bulk_soldier --mode bulk --record-ids 1,2,3,4,5 \
    --out preview.pdf

# Diff two existing PDFs
dixiedata-tune diff --before out-before.pdf --after out-after.pdf

# Print the appshell's default flag set (copy-paste to reproduce)
dixiedata-tune print-defaults --mode bulk
```

## How it works

`tools/tune/` is a separate Go module (`github.com/valueforvalue/DixieData/tools/tune`). It uses Go's `replace` directive to import from the main DixieData module:

- `pkg/exportbridge` — the canonical facade. Both the appshell and tools/tune drive the same `BulkRenderer.RenderBulk` / `RenderSingle`. A PDF produced by tune is byte-identical to one produced by the appshell for the same inputs.
- `pkg/render` — the typst renderer.
- `internal/models` — the `models.Soldier` type.

The tool shells out to the bundled Typst binary in `bin/` via `exec.Command` directly (no go-typst wrapper). This keeps the Windows build free of console-window flashes during render.

## Byte-identical contract

`pkg/exportbridge` (added in step 1 of issue #69) makes every export the tool can produce match what the appshell produces for the same input. The contract is pinned by `internal/exportcontract` snapshots:

- `TestArchiveContractSnapshots` runs in-process via `pkg/exportbridge.RenderBulk` / `RenderSingle`.
- `TestCLIContractSnapshots` shells out to the actual `dixiedata-tune` binary and verifies the CLI surface.

Both test files produce byte-identical PDFs against snapshots in `internal/exportcontract/testdata/snapshots/` and `testdata/snapshots-cli/` respectively. `UPDATE_SNAPSHOTS=1` regenerates.

Run the snapshots:

```sh
go test -count=1 ./internal/exportcontract/ -v
```

## Make targets

- `make tune` — build the binary
- `make tune-smoke` — render the live `.dixiedata/` archive (smoke test, no byte comparison)
- `make tune-snapshots` — regenerate the snapshots and verify byte-stability

## License

DixieData is licensed under the same terms as the main project.