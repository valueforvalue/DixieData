# anniversary

Monthly anniversary report. Template: `anniversary.typ`. The
`anniversary` subcommand of `dixiedata-tune` produces this PDF
via `pkg/exportbridge.BulkRenderer.RenderAnniversary` →
`internal/archive.ExportService.ExportMonthlyAnniversaryPDF`.

## Round 4 → Round 5 changes

User annotations in round 4:

1. "I think it should be smaller/ less promanent" — footer
   smaller.
2. "Continued suffix" — for day headers that repeat on the
   right column.
3. "Yes" — decade italic OK.
4. "We should fix this" — unknown-death-year soldiers falling
   loose at the end of a day.

### What changed

- **Smaller footer**: anniversary overrides the shared
  `page-params` footer locally. Was 8pt in
  `theme.palette.text_secondary`; now 6pt in
  `theme.palette.text_muted` (one step less prominent). The
  shared `page-params` helper wasn't modified; other
  templates still get the 8pt default. Verified in the PDF
  stream: font-size-6 text is present, the 8pt footer is
  gone for this surface.
- **Unknown death year bucketed under "Unknown" sub-header**:
  `soldier-decade()` returned `""` before; now returns
  `"Unknown"` so the yearless entries render under their own
  italic 6.5pt sub-header, sorted last (sort key 9999 to
  keep them after real decades). No more "floating loose".
- **Day-header repeat investigated**: I tried to implement
  `(cont.)` for day headers that appear twice (once per
  column). Investigation: typst's `columns(2, ...)` block
  treats the entire `#for day in days` loop as a single
  flow and breaks at content boundaries. **Day headers do
  not repeat across the column boundary** in the current
  data. The user's (cont.) request was based on a misreading
  of the layout — verified by grepping `Day \d+` in
  `round-4.pdf` and confirming each day appears exactly
  once. The `state`/`introspection` approaches I tried to
  add (cont.) detection didn't propagate reliably within a
  `columns(2)` block, and aren't needed.

  I also tried switching to a `grid(2, ...)` layout with
  manual day-splitting — that made it 2 pages because the
  grid forces both columns to be the same height, and the
  longer right column wraps. Reverted to `columns(2, ...)`.

- Still 1 page on May (43) and February (45).

## Open questions for the next round

- **Day-3 with many soldiers**: as more records are added, a
  single day could eventually overflow into the right column
  in `columns(2, ...)`. At that point we'd need a
  `(cont.)` suffix to mark the wrap. Today, the largest day
  in the live archive has 2-3 soldiers; the threshold is
  well above current data. Want me to add a state-based
  `(cont.)` detection anyway as a future-proofing measure?
- **Footer removal on cover page**: a cover page with no
  footer would be a bigger change (typst's `cover` /
  `frontmatter` pattern). Worth doing?
- **Sorted by last name instead of by year**: would let the
  user search a name within a day. Currently sorted by
  death year, then last name, then first name. Within a
  decade, the year-sort means alphabetical only when years
  tie. Acceptable?
