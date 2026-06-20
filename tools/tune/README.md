# dixiedata-tune

A developer tool for iterating on the Typst-based PDF export
templates. Lives in `tools/tune/` as a separate Go module. Reads the
DixieData SQLite in read-only mode and renders templates through
both the legacy fpdf path and the new Typst path so you can see
them side by side.

## Build

```sh
cd tools/tune
go build -o bin/dixiedata-tune .
```

## Usage

```sh
# Discover what templates exist
dixiedata-tune list-templates

# List records in the local DixieData SQLite
dixiedata-tune --db ~/.dixiedata/dixiedata.db list-records

# Render a single Typst template against a record
dixiedata-tune --db ~/.dixiedata/dixiedata.db render \
    --template soldier_landscape --record 42 --out out.pdf

# Capture the fpdf baseline for every record
dixiedata-tune --db ~/.dixiedata/dixiedata.db capture-baseline

# Render the same record through both fpdf and Typst; saves both PDFs
dixiedata-tune --db ~/.dixiedata/dixiedata.db compare \
    --template soldier_landscape --record 42
```

## How it works

`tools/tune/` is a separate Go module (`github.com/valueforvalue/DixieData/tools/tune`).
It uses Go's `replace` directive to import from the main DixieData
module:

- `pkg/render` — the `Renderer` interface, `FpdfRenderer`, and
  `TypstRenderer`.
- `pkg/encode` — the `TemplateData` shape that templates read via
  `sys.inputs`.
- `pkg/dixiedata` — a thin read-only adapter that opens a DixieData
  SQLite and walks the Person Records.
- `internal/models` — the `models.Soldier` type.

The tool uses `github.com/Dadido3/go-typst` to shell out to the
bundled Typst binary in `bin/`.

## Annotation feedback loop

When you render a template, write notes in a sidecar `.md` file
next to the output:

```sh
dixiedata-tune render --template soldier_landscape --record 42 --out out.pdf
# Then write:
echo "Section title too dark; make it lighter" > out.md
# Re-render to read the notes as context.
dixiedata-tune render --template soldier_landscape --record 42 --out out.pdf
```

The convention: `out.md` next to `out.pdf` for the same render. The
agent or the developer reads the most recent `.md` for a given
`(template, record)` pair before re-rendering.

## License

DixieData is licensed under the same terms as the main project.
