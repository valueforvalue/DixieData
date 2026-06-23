# Tune iteration: rendering PDF/SVG from typst templates

`tools/tune` is the CLI driver for rendering the typst-backed PDF
surfaces against the live `.dixiedata/` archive. The same render
path is used by the appshell (`internal/appshell/handleSoldierPDF`)
and by `internal/exportcontract` snapshot tests, so a layout change
committed here shows up identically in every render surface.

This doc is the iteration playbook for an agent picking up a
layout-iteration task on the dev branch. It assumes the agent can
read typst and run `make`.

## What the surfaces are

`docs/renderings/<surface>/` holds the iteration artifacts for one
of nine export surfaces:

| Surface | Template | Orientation | Notes |
|---|---|---|---|
| `single-soldier-landscape`  | `soldier_landscape.typ`  | L | the main record card |
| `single-soldier-portrait`   | `soldier_portrait.typ`   | P | same soldier, portrait |
| `single-widow-landscape`    | `widow_landscape.typ`    | L | widow record card |
| `single-widow-portrait`     | `widow_portrait.typ`     | P | widow, portrait |
| `bulk-sorted`               | `bulk_soldier.typ`       | L | full archive, sorted |
| `bulk-grouped-pension-state`| `bulk_soldier.typ`       | L | grouped by pension state |
| `bulk-grouped-burial-location` | `bulk_soldier.typ`    | L | grouped by burial location |
| `anniversary`               | `anniversary.typ`        | P | one-month anniversary list |
| `insights`                  | `analytics_summary.typ`  | P | archive summary |

The single-* surfaces are one PDF per record. The bulk-*,
anniversary, and insights surfaces are many-page PDFs.

## Build + render in 4 commands

```sh
# 1. Build the tune binary (only when pkg/render, pkg/exportbridge,
#    or tools/tune source changes; typst template edits don't need this).
make tune

# 2. Render the surface you are iterating on. ROUND defaults to one
#    greater than the highest existing round-N.pdf for that surface.
#    KEEP=N preserves N rounds before the new one (default 1).
#    RECORD=<id> overrides the default record (1 for soldier, 61 for
#    widow) for single-* surfaces, useful for iterating on a record
#    with no image, long data, or other layout edge cases.
make render-round-ONE SURFACE=single-soldier-landscape
# Or, for a specific record (e.g. Elbert Dixon Anderson, DXD-00019,
# a no-image record):
make render-round-ONE SURFACE=single-soldier-landscape RECORD=21

# 3. Open the PDF (and the SVG if you want a vector preview):
start "" docs/renderings/single-soldier-landscape/round-N.pdf
# the same call with --out X.svg also works, written next to the PDF.

# 4. After the user signs off on the layout, regen the byte-stable
#    snapshot fixture for this surface and verify byte-match:
make update-snapshots-ONE SURFACE=single-soldier-landscape
```

Repeat. The full loop is: edit typ -> step 2 -> review -> edit typ
-> ... -> user accepts -> step 4 -> commit.

## Where the typst lives

The shared chrome (page geometry, header, footer, field sections)
lives in `templates/common/record_card.typ`. Per-variant templates
in `templates/{soldier,widow,spouse}_*.typ` import it. Bulk uses
`templates/bulk_soldier.typ`. Anniversary uses `templates/anniversary.typ`.
Insights uses `templates/analytics_summary.typ`.

The single-record and bulk paths are independent enough that a
record-card chrome change usually affects only the per-record
surfaces. Watch for surprises in the bulk layout if you change
page geometry or the section vertical-gap constants.

## SVG and PNG previews

Typst 0.15 supports native SVG output (`typst compile --format svg`)
and per-page PNG (`--format png`). `tools/tune` wires these into
the CLI:

- `tune render --out X.svg` writes page 1 to `X.svg` and pages
  2..N to `X-2.svg`, `X-3.svg`, ... next to it. Multi-page SVG
  needs the `{p}` page-template; tune takes care of it.
- `tune render --out X.png` writes `out-{p}.png` and streams
  `out-1.png` to `X.png`. Subsequent pages land as `X-{p}.png`
  siblings.

A 150 DPI PNG preview of page 1 is also a useful scan artifact
when you want a quick visual sanity-check without opening a
vector viewer. The shell helper at `C:/Users/value/bin/render-svg.sh`
handles both, plus a PNG preview, in one call:

```sh
# Render PDF + native SVG + 150 DPI PNG preview, all into
# docs/renderings/<surface>/round-N.{pdf,svg,png}:
/c/Users/value/bin/render-svg.sh single-soldier-landscape N

# Render a specific record (overrides the default 1 for soldier,
# 61 for widow). Useful for no-image, long-data, or edge cases:
/c/Users/value/bin/render-svg.sh single-soldier-landscape N -r 21

# Limit bulk surfaces to specific record IDs (saves ~99% disk):
/c/Users/value/bin/render-svg.sh bulk-sorted N -i 1,2,3,4,5

# Render every surface (matches make render-round, but as SVG):
/c/Users/value/bin/render-svg.sh all
```

The PNG and SVG artifacts are gitignored (see `.gitignore`); they
are scratch for visual review, not source. Only the round-*.pdf
under `docs/renderings/<surface>/` is referenced from review.md
check-ins; even those are gitignored, just kept locally for the
user to open alongside the per-surface review.md.

## Snapshot fixtures

`internal/exportcontract/testdata/{snapshots,snapshots-cli}/`
hold 22 byte-stable PDF fixtures (11 in-process via the bridge,
11 CLI via `dixiedata-tune`). The `go test` step in
`make update-snapshots-ONE` runs with `UPDATE_SNAPSHOTS=1` to
regen, then without to verify byte-match.

Snapshots are git-tracked. Treat any byte drift as a layout
regression unless the source change is intentional and paired
with a snapshot regen in the same commit. Three surfaces
(`bulk-grouped-burial-location`, `anniversary`, `insights`) have
no per-surface snapshot; their layout drift is captured by
`bulk-landscape.pdf` (same `bulk_soldier.typ` template + group-by
behaviour) and by the live-archive renderings under
`docs/renderings/`. For those surfaces, run the full regen:

```sh
make tune-snapshots
```

(regen all 22 fixtures in one go; takes ~40s.)

## Disk policy

`render-round-ONE` with default `KEEP=1` keeps only the previous
round before writing the new one. That is:

- 9 record-surfaces x ~500KB per round = ~4.5MB
- 3 bulk surfaces x ~200MB per round = ~600MB (the bulk PDFs are
  huge because the fixture archive is large)
- 1 anniversary + 1 insights x ~100KB per round = ~200KB

Bulk PDF artifacts are the dominant cost. If disk matters during
iteration, render bulk with `-RecordIDs 1,2,3,4,5` (or similar)
to keep the test fast and the output small:

```sh
make render-round-ONE SURFACE=bulk-sorted ROUND=N
# Or, via PowerShell directly:
pwsh -File scripts/render-round.ps1 -Round N -Only bulk-sorted \
    -RecordIDs 1,2,3,4,5
```

To disable pruning and keep every round (e.g. when reviewing a
long sequence of small changes), pass `-KeepRounds 99` or set
`KEEP=99` on the make target.

## Verifying appshell parity

The appshell uses the same `internal/archive/ExportService` and
the same `pkg/render` registry as `tools/tune` and the snapshot
tests. A successful `go test -count=1 ./internal/exportcontract/...`
implies appshell parity for every snapshot-covered surface. The
non-snapshot-covered surfaces (bulk-grouped-burial-location,
anniversary, insights) should be visually compared against a PDF
exported from the running appshell when there is doubt.

## Common gotchas

- **`bin/typst-windows.exe` is gitignored**. If you delete it
  accidentally, restore it from the Wails build artifacts or
  rebuild the appshell; tune cannot run without it.

- **`tools/tune/bin/dixiedata-tune.exe` is gitignored**. `make tune`
  rebuilds it. On Windows, `make tune` now writes the `.exe`
  suffix explicitly (the old Makefile rule wrote `dixiedata-tune`
  without suffix, which broke shell scripts that hardcoded
  `dixiedata-tune.exe`).

- **`tune` runs out of cwd**. If you invoke `tools/tune/bin/dixiedata-tune.exe`
  from a directory other than the repo root, the relative paths
  in `--typst`, `--templates`, `--db` resolve against the cwd
  and fail. Either `cd` to the repo root first, or pass absolute
  paths.

- **Multi-page SVG sort order**: typst's `{p}` template does NOT
  honour `{0p}` for zero-padding (despite the docs). Files are
  named `out-1.svg, out-2.svg, ..., out-10.svg`, so directory
  listings sort lexicographically (`out-10` before `out-2`). This
  is intentional; `copyExtraPages` and the shell helper handle
  the unsorted names correctly.

- **`SOURCE_DATE_EPOCH` is pinned** by `runTypstCompile` to
  `1577836800` when not already set in the environment. This
  makes PDF bytes byte-stable across runs and is required by
  the snapshot contract. If you `unset SOURCE_DATE_EPOCH`
  globally, the test will fail with non-deterministic byte drift.

- **Cache cleanup**: `os.RemoveAll` on the typst workdir runs
  after every render. The appshell has historically leaked
  `dixiedata-typst-*` dirs under `%TEMP%`; ignore them unless
  disk fills up.
