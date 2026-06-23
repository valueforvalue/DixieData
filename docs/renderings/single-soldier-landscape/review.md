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

Image needs to start a little bit lower

DXD-00001 - Soldier needs to move up closer to the soldiers name

Identity & Vital Details
First Name William
Middle Name Pickney
Last Name Looney
Birth Date Unknown
Death Date May 12, 1909
Buried In Battle Creek Cemetery, Eolian, Stephens County,
Texas, USA
Service & Archive Details
Record Type Soldier
Rank In Pvt.
Rank Out Pvt.
Unit Co. I, 4th TN Cav. Rgmnt., C.S.A.
Pension State Texas
Pension ID P5399
Application ID A6510
Confederate Home Status N/A
Confederate Home Name N/A

all of this data needs to move up closer to DXD-00001

## Round 5 (in progress)

Visual sign-off: render-ONE for SURFACE=single-soldier-landscape
at round-5 captured the round-4 template state. Two structural
problems were visible:

1. **Empty space between display-id and "Identity & Vital Details"**.
   Round 4 added a 2-column title row (title-block + image) but
   typst's grid cell alignment is `center + horizon` by default,
   so the title text was vertically centered in the 40mm-tall
   image cell. Half the image height (~50pt) became empty space
   below the display-id before the body grid started.

2. **Image too small to read**. The 40mm-tall panel clipped a
   805x2000 portrait-orientation source to a narrow strip. The
   form text was unreadable.

Round 5 fixes:

- **Reverted the title-row refactor** (`render-record-card` for
  landscape). The title block now spans the full page width and
  the body grid contains the image in its right column, top-
  aligned via `#align(top)[...]`. The image's top edge sits at
  the same Y as the "Identity & Vital Details" header on the
  left, which closes the ~50pt gap.
- **Bumped `image_panel_height` 40mm -> 60mm** in
  `templates/common/theme.typ`. The form photo is now readable
  at the cost of a taller right column. Records section
  appears below the image rather than mid-page.

User-asked-for changes still open:

- **Image top at "William Pickney Looney" Y** (round-3 ask).
  Currently the image top is at the "Identity & Vital Details"
  Y. Reaching the title's Y would require either:
  (a) a 3-column title row that survives typst's grid centering
      (round-4's failed attempt), or
  (b) floating the image up via `place(top + right, dy: -Npt)`,
      or
  (c) a more invasive restructure that floats the image over
      the right half of the title block.
  Defer until the user weighs in.
- **Empty space in right column under Records**. The records
  section is short (one findagrave link for this record); the
  right column ends well before the left column does. Acceptable
  for sparse records; revisit if user finds it distracting.