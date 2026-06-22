# anniversary

Monthly anniversary report. Template: `anniversary.typ`. The
`anniversary` subcommand of `dixiedata-tune` produces this PDF
via `pkg/exportbridge.BulkRenderer.RenderAnniversary` →
`internal/archive.ExportService.ExportMonthlyAnniversaryPDF`.

## Round 3 → Round 4 changes

User annotations in round 3:

1. "Yes densinty is good" — keep current density.
2. "Okay for now" — keep unknown-death-year behavior (no
   sub-header; falls through to end of day).
3. "No page n of me footer" — keep typst's natural flow.
4. "No it should be repeated for ease of reading." — keep day
   headers repeating across column boundaries.
5. New: horizontal rule under the title — same color as the
   day text, full margin width.
6. New: the "Made with DixieData" footer (version + build
   commit) from soldier_landscape should be on this too.

### What changed

- **Horizontal rule under title**: `#line(length: 100%, stroke:
  0.6pt + theme.palette.accent)`. Spans the full text width
  (page width minus 0.63in left/right margins), accent color
  (`#8d7440` — same as day text). Verified via PDF stream
  inspection: stroke color `rgb(141, 116, 64)`, line length
  `521.28 pt = 7.25in` (matches Letter minus margins).
- **"Made with DixieData" footer now on by default**: the
  `page-params` footer was already configured; it just was
  suppressed because the tune subcommands defaulted
  `--printer-friendly true`. Flipped the default for
  `anniversary` and `insights` to `false`. Existing
  `--printer-friendly` flag still works for users who want to
  suppress it (e.g. for a real paper print run).
- The shared `page-params` helper needed no change — it
  already reads `opts.printerFriendly` and emits the footer
  via the `Branding.footer_text` template (`<version> | Build:
  <commit>`).
- Still 1 page on May (43 entries) and February (45 entries).

## Open questions for the next round

- **Footer readability on dense months**: footer at 8pt sits
  ~0.4in from bottom. Readable at the current size? The
  accent color is the rule, not the footer; footer is
  secondary text color.
- **Day boundary rendering**: a single day with 5+ soldiers
  spans columns; the day header is repeated in the right
  column. Is that the desired behavior, or do you want a
  "(continued)" suffix on the second occurrence?
- **Decade headers**: italic 6.5pt secondary colour. Some
  PDF viewers may not render the italic well at 6.5pt; would
  you prefer bold (no italic) for the same weight class?
- **Single soldier with unknown year**: renders at the end of
  the day, no sub-header. Could be visually awkward if a day
  has both grouped and ungrouped soldiers. Acceptable?
