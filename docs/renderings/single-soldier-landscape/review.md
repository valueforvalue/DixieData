# single-soldier-landscape

One Soldier record card, landscape orientation. Template:
`soldier_landscape.typ`. The `render` subcommand of
`dixiedata-tune` produces this PDF via
`pkg/exportbridge.BulkRenderer.RenderSingle` →
`internal/archive.ExportService.ExportSoldierPDF` →
`Registry.Render` → `templates/soldier_landscape.typ` (which
delegates to `templates/common/record_card.typ`).

## Round 1 → Round 2 changes

User annotations in round 1:

1. "This is rendered without an image? is the image missing
   from the actual soldier data or missing from PDF"
2. "This text should match the location size etc of the
   anniversary pdf we just worked on. J. Morris's Civil War
   Research Archive and have the same horizontal line under
   it."
3. "This William Pickney Looney should be smaller and closer
   to DXD-00001 - Soldier and the DXD-00001 - Soldier should
   be closer to Identity & Vital Details"
4. "There needs to be a matching horizontal line above the
   dixiedata footer as well"

### What changed

- **Image renders** (BUGFIX): the bridge set
  `soldier.Images[i].ResolvedPath` to a *relative* path
  (`images\D\X\DXD-00001\...`) because the data dir passed in
  by `tools/tune` was `.` (`filepath.Dir(".dixiedata")` →
  `.` — `Dir` drops the trailing component when it's a
  directory, not a file). The image-staging step does an
  `os.Stat` on `ResolvedPath`; a relative path from the
  current working directory didn't find the file, so the
  source-image copy was silently skipped and the template
  rendered no image.
  Fix: bridge `NewBulkRenderer` now resolves `dataDir` to an
  absolute path via `filepath.Abs` before storing it. The
  tune flag handling also changed: `--db <dir>` is treated
  as the data directory directly (rather than
  `Dir(<dir>)`), so `--db .dixiedata` no longer collapses to
  `.`. The pre-iteration render of soldier 1 was 79 KB with
  no image; round 2 is 224 KB with the primary image
  embedded. PDF text extraction confirms the rest of the
  layout is unchanged.

- **Header + footer match anniversary**: the shared
  `page-params` helper in `templates/common/record_card.typ`
  is now the single source of truth for header/footer
  chrome, matching the anniversary layout. Header: top-left
  7pt secondary text. Footer: 6pt muted text with a 0.6pt
  accent-coloured horizontal rule above it. Anniversary
  already had this layout (round 4-5 of that surface); the
  per-record templates now inherit it instead of carrying
  their own.

- **Title hierarchy tightened**: `render-title-block` is
  20pt → 14pt, gap to display-id 0.2em → 0.1em, display-id
  10pt → 9pt, gap to first section 0.6em → 0.2em. The three
  title-block lines now read as a single header unit rather
  than three separate chunks.

- **All 11 record-card snapshots regenerated**:
  `TestArchiveContractSnapshots` and
  `TestCLIContractSnapshots` both updated. The changes
  apply uniformly to soldier/widow/wife/linked-person
  variants and the bulk path (which uses the same
  `page-params`).

## Verification

- Soldier 1 landscape: 79,397 bytes (no image) → 224,142
  bytes (with image). 4 image files staged into the typst
  workdir.
- All 22 snapshot tests pass against the regenerated
  fixtures (11 in-process, 11 CLI).
- Single round 2 PDF for comparison:
  `docs/renderings/single-soldier-landscape/round-2.pdf`.

## Open questions for the next round

- **Bulk path header**: the bulk export reuses
  `page-params` so the new chrome applies to bulk too.
  The bulk divider pages (templates/group_divider.typ,
  templates/bulk_soldier.typ) also use `page-params` —
  consistent. Want me to check the bulk layout specifically
  to see if anything looks off?
- **Portrait orientation**: the title block was
  left-aligned for landscape and centered for portrait.
  The smaller font + tighter gaps may need a portrait-only
  tweak if the title overlaps the image panel.
- **Image panel size**: the right-column image panel
  position is unchanged. With the title block now
  tighter, the image might benefit from a slight size bump
  to fill the visual gap. Want me to try?
- **Terminology conflicts** tracked in
  `docs/renderings/TERMINOLOGY.md` are still open (Records
  vs Source Records, Record Type vs Person Record Type).
  These are visible on every record card and can be tackled
  as a separate round.
