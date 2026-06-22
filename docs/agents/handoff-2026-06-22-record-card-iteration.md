# Handoff: single-soldier-landscape round 3 (in progress)

Date: 2026-06-22
Branch: `dev`
Last commit: `76c3ec4 feat(record-card): round-3 header rule + tighter title gap`
This handoff picks up: round 4 of the single-soldier-landscape
PDF iteration loop, plus a tooling note for PDF rendering.

## Where we are

The user is iterating on
`docs/renderings/single-soldier-landscape/round-3.pdf` against
the fpdf-era reference PDF `TDM65-00202-landscape.pdf` (in
the repo root). Round 3's annotations asked for:

1. Header rule needs visible space below "J. Morris's Civil
   War Research Archive" rather than flush against the text.
2. Title block: "William Pickney Looney" and "DXD-00001 -
   Soldier" tighter, then closer to "Identity & Vital
   Details".
3. Section headers closer to their data.
4. Image top aligned with the title (an "imaginary line
   across the page" at the title's Y).

## What's in flight (uncommitted)

`templates/common/record_card.typ` has the round-4 changes
applied but **the exportcontract snapshots have NOT been
regenerated** against these template changes. The code is
committed-ready but the snapshot regen is the missing step
before push.

Code changes pending commit:

- `v(0em)` between name and display-id (was `v(-0.3em)`;
  the user wanted them on separate lines, close together).
- `v(0.1em)` between display-id and first section (was 0.2em).
- `v(0.2em)` between each section header and its data (was 0.4em).
- `v(0.4em)` between service section and its header (was 0.6em).
- New 2-col title row in `render-record-card` so the image
  top aligns with the title text in landscape.
- `v(0.3em)` between header text and header rule, so the rule
  sits visibly below the text rather than flush against it.

These changes are NOT yet validated visually because the
rendering toolchain produced PDFs that didn't render in
Edge headless (see "Tooling note" below).

## What to do next session

1. Run the test PDF and view it. Don't push until the user
   confirms the visual.

2. To regenerate the snapshot fixtures once the visual is
   accepted:
   ```sh
   UPDATE_SNAPSHOTS=1 go test -count=1 -run 'TestArchiveContractSnapshots|TestCLIContractSnapshots' -timeout 600s ./internal/exportcontract/
   go test -count=1 -run 'TestArchiveContractSnapshots|TestCLIContractSnapshots' -timeout 600s ./internal/exportcontract/
   ```
   22 snapshots need regenerating (11 in-process + 11 CLI).
   The page-params change is shared across
   soldier/widow/wife/linked-person/bulk/grouped-bulk, so
   all 22 fixtures shift.

3. Render the round-4 PDF for the user to compare:
   ```sh
   pwsh -NoLogo -NoProfile -File scripts/render-round.ps1 -Round 4
   ```

4. Update `docs/renderings/single-soldier-landscape/review.md`
   with the round-3→round-4 changelog and any new open
   questions.

5. Commit and push:
   ```sh
   git add templates/common/record_card.typ internal/exportcontract/testdata/snapshots internal/exportcontract/testdata/snapshots-cli docs/renderings/single-soldier-landscape/
   git commit -m "feat(record-card): round-4 ..."
   git push
   ```

## Tooling note: PDF rendering

**Problem**: Edge headless (`msedge.exe --headless --screenshot`)
used as a stopgap PDF viewer produces inconsistent output:
sometimes shows the rendered PDF, sometimes shows "File not
found" (when the window-size is bigger than the PDF page
size, the rendered image gets cropped to "not found"), and
sometimes the typst-rendered text shows but the lines don't.

**Recommended alternatives for the next session**:

1. **SumatraPDF** is now installed at
   `C:\Users\value\bin\SumatraPDF.exe` (3.5.2, 64-bit). It
   has a CLI render mode:
   ```sh
   /c/Users/value/bin/SumatraPDF.exe -print-to-default -print-settings "noxprintnoscale" /tmp/single-test.pdf /c/Users/...
   ```
   But this prints to the default printer, not great for
   iteration. The better use is the GUI: open the PDF
   directly and screenshot, or use its `-tts` mode.

2. **Mupdf / mutool**: install via choco once you have admin
   (choco install mupdf). `mutool draw -o out.png -r 150
   in.pdf 1` will rasterize page 1 at 150 DPI. This is the
   cleanest path for a pipeline view.

3. **PDF.js in a headless browser** (Firefox has a
   dedicated `pdfjs-disabled` flag): install Firefox via
   winget (`winget install Mozilla.Firefox`) and use
   `firefox --headless --screenshot=... file:///path/to.pdf`.
   More reliable than Edge for PDFs.

4. **Direct typst-to-image**: typst itself can export to
   PNG via `--format png` (typst 0.15+). The current
   pipeline always goes to PDF; if we just need to verify
   visual layout during iteration, `typst compile
   --format png` skips the PDF round-trip.

If iterating visually matters, **option 2 (mutool)** is the
recommended next step. The user is iterating on the look of
the PDF, so the iteration loop is bottlenecked on the
viewer.

## Files in scope

- `templates/common/record_card.typ` — pending changes
  (described above)
- `docs/renderings/single-soldier-landscape/review.md` —
  needs the round 3→4 changelog
- `internal/exportcontract/testdata/snapshots/*.pdf` —
  11 fixtures, need regen
- `internal/exportcontract/testdata/snapshots-cli/*.pdf` —
  11 fixtures, need regen

## Out of scope

- Issue #72 (terminology conflicts in templates/):
  `Records → Source Records`, `Record Type → Person Record
  Type`, `Archive Summary Report → Local Archive Summary
  Report`, `Record Types → Person Record Types`, `No records
  to summarise → No Person Records to summarise`. Filed
  separately; not part of this surface's iteration.
- Anniversary iteration: complete (round 6).
- Bulk / portrait / insights / widow / spouse variants: not
  yet iterated. Will be picked up after
  single-soldier-landscape is done.
