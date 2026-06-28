# Handoff: DixieData record-card iteration (snapshot 2026-06-23)

> **Status: HISTORICAL SNAPSHOT.** Captures the iteration state on
> 2026-06-23. The work continued past this point; current state is
> `c1d9dc1 fix(appshell): guard every native dialog call + lock the
> pattern as law` (2026-06-27). Use this file only to read the
> rationale that informed rounds before round 32. For anything
> after round 32, consult `CHANGELOG.md` and `git log`.

Date: 2026-06-23 (snapshot date)
Branch: `dev`
Last commit at snapshot time: `7f6a416 chore(snapshots): regenerate after round-32 suffix comma drop`

This handoff is a fresh snapshot of where the work is right
now, written so a fresh session can pick up the iteration
loop. Prior rounds and the rationale for each change live
in the per-surface `review.md` files and the typst layout
tips doc; the handoff itself stays small and current.

## Where we are

The user has been iterating on the typst-backed record-card
PDF surfaces (single-soldier-landscape, single-soldier-portrait,
single-widow-landscape, single-widow-portrait, single-widow-portrait,
bulk-sorted, bulk-grouped-pension-state, anniversary, insights)
for the past 8 days. The loop is well-defined and the
tooling is in place. The current state:

- **Layout is mostly done**. Image at title's Y, 0.6cm column
  gutter, click-to-view links in records, page chrome with
  header + footer rules, anniversary with clickable FaG links,
  biography appendix pages, widow/wife variants have Rank
  In/Out/Unit dropped, linked spouse renders as display_id.
- **Visual iteration is the steady state.** The user reviews
  each round's PDF in a browser, gives feedback, signs off,
  and the change is committed (template + snapshots, two
  commits per round).
- **Snapshot regen policy**: deferred until the user signs
  off the visual. Each round goes through one or more
  visual-only renders before snapshots are touched.
- **No uncommitted work** at the moment. The repo is clean.

## The 4-command canonical loop

```sh
# 1. Build the tune binary (only when pkg/render, pkg/exportbridge,
#    or tools/tune source changes; typst template edits don't need this).
make tune

# 2. Render the surface for one round.
#    ROUND auto-increments past the highest existing round.
#    RECORD overrides the default record (1 for soldier, 61 for
#    widow) for single-* surfaces.
#    KEEP=N preserves N rounds before the new one (default 1).
make render-round-ONE SURFACE=single-soldier-landscape RECORD=21

# 3. Open the PDF in a browser/SumatraPDF and review the visual.
start "" docs/renderings/single-soldier-landscape/round-N.pdf

# 4. After the user signs off the visual, regen the byte-stable
#    snapshot fixtures and verify byte-match:
make update-snapshots-ONE SURFACE=single-soldier-landscape
```

The shell helper at `C:/Users/value/bin/render-svg.sh` does
the same loop with PDF + native SVG + 150 DPI PNG. Use it when
you want to inspect vector transforms to debug layout. Use
`make render-round-ONE` when you only need the PDF.

## Repo conventions

- **Two commits per round** (template + test changes, then
  snapshots). The split makes the diff between code and
  byte-shift easier to review and revert.
- **Per-surface review.md** captures the iteration history,
  not the handoff. The handoff is a fresh snapshot for
  fresh sessions; review.md is a log.
- **Typst layout tips** in `docs/agents/typst-layout-tips.md`
  capture the gotchas. Read it before reaching for `place()`
  or `align()`. It was extended at round-23 and again at
  round-32 with new lessons.
- **Issue tracker is GitHub Issues via `gh` CLI**. See
  `docs/agents/issue-tracker.md`. Out-of-scope issues
  (terminology conflicts, etc.) get filed as separate
  issues, not blocked here.

## Surfaces and current state

| Surface | Template | Round | Status |
|---|---|---|---|
| single-soldier-landscape | soldier_landscape.typ | 32 | iterating, last sign-off round-32 |
| single-soldier-portrait | soldier_portrait.typ | 32 | iterating, last sign-off round-32 |
| single-widow-landscape | widow_landscape.typ | 31 | iterating, last sign-off round-31 |
| single-widow-portrait | widow_portrait.typ | 31 | iterating, last sign-off round-31 |
| bulk-sorted | bulk_soldier.typ | 32 | iterating, last sign-off round-32 |
| bulk-grouped-pension-state | bulk_soldier.typ | 32 | iterating, last sign-off round-32 |
| bulk-grouped-burial-location | bulk_soldier.typ | (no per-snap) | iterate by visual only |
| anniversary | anniversary.typ | 29 | iterating, last sign-off round-29 |
| insights | analytics_summary.typ | 32 | iterating, last sign-off round-32 |

All non-bulk-burial-location / anniversary / insights
surfaces have per-snapshot coverage in
`internal/exportcontract/testdata/{snapshots,snapshots-cli}/`.
For those, full snapshot regen runs after every visual
sign-off. The three surfaces without per-snapshots rely on
`make tune-snapshots` (full regen of all 22 fixtures, ~40s)
for layout regression coverage.

## What to do next session

1. **Wait for the user's next ask.** The user drives the
   iteration. The next ask is likely another visual tweak
   on one of the surfaces, but could be:
   - A new field added to the soldier model that needs to
     surface in the PDF
   - A bug report (something renders wrong)
   - A bulk-data export for an external recipient (e.g.
     a research collaborator)
   - A new surface (e.g. a "linked persons only" filter)

2. **For visual iteration:** follow the 4-command loop.
   Always make the template change first, render, get
   visual sign-off, then regen snapshots. If the user
   pushes back ("I asked for X, not Y"), revert the
   template change BEFORE pushing snapshots; the snapshot
   regen will be a no-op.

3. **For bulk renders:** the bulk-sorted full DB is
   ~188MB / ~500 pages. Use the live-DB record IDs from
   `sqlite3 .dixiedata/dixiedata.db "SELECT id, ..."` to
   pick representative records for visual review; the
   full render takes minutes and isn't useful for visual
   review of small tweaks.

4. **For debugging layout:** read the SVG (round-N.svg
   generated by `render-svg.sh`). The `transform="matrix(...)"`
   on each `<g>` element gives you the absolute X/Y of every
   text block. If something's at the wrong Y, the SVG tells
   you which block's height is responsible.

5. **For the next code change:** the most likely
   candidates are: more field-row tweaks in the household
   or service sections, more clickable-link polish
   (e.g. making the `record_type` bold in the records
   section larger), or surface-specific styling (e.g.
   anniversary heading colors). Read the per-surface
   review.md for the most recent state and the open
   questions.

## Open threads

These are NOT urgent; they're future-work items that came
out of the iteration but were deferred:

- **Issue #72 (terminology conflicts in templates/)**:
  `Records → Source Records`, `Record Type → Person Record
  Type`, etc. Filed separately; not part of this surface's
  iteration.
- **Record type normalization**: the DB has freeform
  `record_type` strings like `"fold3; Find a Grave"`,
  `"Find a Grave; newspaper; census"`. The current
  template just prints the string as-is. A future
  improvement is to parse the semicolon-separated types
  and render them as a list of badges.
- **Multi-URL details**: some records (e.g. Newton
  Anderson) have multiple URLs in their `details` field
  separated by blank lines. `render-link` only handles
  one URL per record. The user hasn't asked for this;
  fix is to split on newlines and render each as a
  separate "Click to view" link.
- **Snapshot fixture with a no-image record**: the
  in-process test fixture's records all have images,
  so the no-image code path isn't covered by snapshots.
  Round-22's fix moved records into the right column
  for no-image variants; that's an untested visual.
  Add a fixture with a no-image record + a snapshot
  case for it.
- **Snapshot fixture with a details-URL record**: the
  fixture's `details` strings are empty or non-URL
  (e.g. "Filed in 1880. https://example.com/record.").
  The `render-link` helper's URL detection (`starts-with
  http:// or https://`) makes the fixtures byte-stable
  (no URL detected → plain text → same bytes as before).
  But a future bug in the URL detection won't be caught
  by these tests. Add a fixture with a clean `https://`
  URL in details.

## Files in scope (this iteration)

- `templates/common/record_card.typ` — shared chrome,
  field rows, biography page, render-link helper
- `templates/common/theme.typ` — palette (link color
  set to #4A90E2), type scale, geometry constants
- `templates/{soldier,widow,spouse}_*.typ` — variant
  templates that call into `record_card.typ`
- `templates/bulk_soldier.typ` — bulk card layout
- `templates/anniversary.typ` — anniversary report,
  clickable FaG entries
- `templates/biography_appendix.typ` — full biography
  page (referenced by record_card.typ)
- `templates/analytics_summary.typ` — insights PDF
- `internal/archive/export_service.go` —
  `ExportSoldierPDF` enriches the soldier payload with
  `LinkedSpouseDisplayID`; `ExportMonthlyAnniversaryPDF`
  builds the `soldier_links` map for the anniversary
- `internal/models/models.go` — `LinkedSpouseDisplayID`
  field on Soldier
- `internal/archive/export_service_test.go` — round-30
  test assertion update (substring "Full Biography"
  inverted to assert absence)
- `Makefile` — render-round-ONE accepts RECORD=N flag
- `scripts/render-round.ps1` — `-Record` PowerShell
  parameter
- `C:/Users/value/bin/render-svg.sh` (NOT in repo) —
  `-r`/`--record` flag
- `docs/agents/typst-layout-tips.md` — gotchas captured
  during this iteration
- `docs/agents/tune-iteration.md` — iteration playbook
- `docs/renderings/<surface>/review.md` — per-surface
  iteration history (one per surface)
- `internal/exportcontract/testdata/{snapshots,
  snapshots-cli}/*.pdf` — 22 byte-stable fixtures

## Out of scope (still)

- The shared chrome header/footer typography
  (currently 7pt secondary for header, 6pt muted for
  footer — the user hasn't asked for changes; the
  round-27 anniversary work only touched the gap
  between the page header rule and the body title)
- Click-to-expand images (path 1: file:// link to
  source) — user said "not worth the work" at the
  round-22 image discussion. Re-evaluate only if the
  user changes their mind.
- Bulk portrait, bulk-grouped-burial-location,
  anniversary portrait — the user has only iterated on
  landscape anniversary; portrait/burial-location
  layouts are at the same fidelity as landscape but
  haven't been visually reviewed.
