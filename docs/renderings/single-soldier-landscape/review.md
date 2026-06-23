# single-soldier-landscape

One Soldier record card, landscape orientation. Template:
`soldier_landscape.typ`. The `render` subcommand of
`dixiedata-tune` produces this PDF via
`pkg/exportbridge.BulkRenderer.RenderSingle` →
`internal/archive.ExportService.ExportSoldierPDF` →
`Registry.Render` → `templates/soldier_landscape.typ` (which
delegates to `templates/common/record_card.typ`).

## Round 2 → Round 3 changes

User annotations in round 2:

1. "No we will deal with that later. If we tune this right
   that should take care of itself" — bulk layout check,
   declined for now.
2. "Okay we will address that later as well once we get to
   portrait PDF" — portrait tweak, deferred.
3. "Not yet we need a couple of tweeks to figure out how
   the image should go" — image panel size, deferred.
4. "Okay we should probably file those as an issue to look
   at later" — file an issue for terminology conflicts.
5. "William Pickney Looney needs to be closer to
   DXD-00001 - Soldier almost resting on top of it" — title
   tightening.
6. "J. Morris's Civil War Research Archive should stay
   where it is but needs a horiz line that matches the
   color of the one at the bottom above the footer just
   underneath it" — header rule.

### What changed

- **Header rule** in `page-params`: `place(bottom, line(...))`
  anchored at the bottom of the header area draws a 0.6pt
  accent-coloured line directly under the top-left
  branding text. Same color as the footer rule, so the
  document is now framed top-and-bottom with matching
  accent rules. Verified in the PDF stream: 2 `0.6 w`
  matches (header + footer).

- **Title block gap closed**: gap between the 14pt name and
  the 9pt display-id line was `v(0.1em)` (a visible space);
  now `v(-0.3em)` so the display-id "rests on" the name. The
  two lines now visually merge into a single header unit.
  The 0.2em gap to the first section heading is unchanged.

- **Terminology conflicts filed as issue #72**:
  `Records` → `Source Records`, `Record Type` → `Person
  Record Type`, `Archive Summary Report` → `Local Archive
  Summary Report`, `Record Types` → `Person Record Types`,
  `No records to summarise.` → `No Person Records to
  summarise.`. Acceptance criteria include regenerating
  the 22 exportcontract snapshot fixtures.

- **All 22 exportcontract snapshots regenerated**:
  `TestArchiveContractSnapshots` x 11,
  `TestCLIContractSnapshots` x 11. The header rule and
  tightened title gap apply to every per-record and bulk
  surface that uses `page-params` /
  `render-title-block`.

## Open questions for the next round

- **Image panel size** (deferred from round 2): the user
  wants to "figure out how the image should go" before
  bumping the right-column image panel. Want a specific
  soldier with multiple images to render and see the
  current size before deciding?
- **Portrait orientation** (deferred): will be picked up
  when iterating on `single-soldier-portrait`.
- **Bulk layout** (deferred): `bulk-sorted` and
  `bulk-grouped-*` PDFs are now affected by the
  `page-params` change; not yet reviewed.
- **Issue #72** (terminology): 5 conflicts remain in
  `templates/`. Tracked separately; not part of this
  surface's iteration.



Line under J. Morris's Civil War Research Archive needs to be moved slighly below the text

William Pickney Looney needs to move down nearly on top of the DXD-00001 - Soldier 

DXD-00001 - Soldier needs to move a little closer to Identity & Vital Details

and section headers such as Identity & Vital Details need to move down a little closer to their date.


Once William Pickney Looney is positioned correctly the top of the image file needs to start at the same height as the text William Pickney Looney rests if an imaginary line was drawn across the page