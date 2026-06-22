# anniversary

Monthly anniversary report. Template: `anniversary.typ`. The
`anniversary` subcommand of `dixiedata-tune` produces this PDF
via `pkg/exportbridge.BulkRenderer.RenderAnniversary` →
`internal/archive.ExportService.ExportMonthlyAnniversaryPDF`.

## Round 2 → Round 3 changes

User annotations in round 2:

1. "I would like this to fit on one page so a smaller font
   would be great. We will however have to test for large
   ammounts of anniversaries and maybe have a hard limit that
   switches to multiple pages"
2. "Yes death and birth year would be useful dont use too much
   space with it though"
3. "yes group" — clarified out-of-band: **group by decade**
4. Branding text "J. Morris's Civil War Research Archive" was
   too prominent; should be top-left in a much smaller font.
5. Title "May Anniversary Report" should be centered, with the
   text under it allowed to be a tiny bit closer.

### What changed

- **Header override**: `set page(..page-dict, header: ...)`
  replaces the shared `page-params` header with a 7pt
  top-left-aligned branding line. The shared `page-params`
  helper isn't modified; anniversary overrides locally.
- **Title**: 20pt → 16pt, kept centered, gap from title to
  body cut from `0.6em` to `0.15em`.
- **Day lists**: 9pt → 7pt; spacing tightened (`0.45em`
  leading → `0.35em`, gutter 1.2em → 0.9em). Day headers 8pt
  bold; decade sub-headers 6.5pt italic; soldiers 7pt regular.
- **Death year inline**: `Name (display_id, 1911)` — comma
  between id and year, minimal space.
- **Decade grouping**: soldiers within a day are sorted by
  `(death_year, last_name, first_name)`, then grouped by
  decade. Decade sub-headers in 6.5pt italic secondary text
  colour.
- **One-page default**: 43 (May) and 45 (February — the
  largest month in the live archive) both fit on a single
  page now. Threshold `one-page-budget = 50` is in the
  template; typst's natural pagination will flow large months
  onto a second page at the same density.

## Open questions for the next round

- **Wide years (1850s)**: a single-day decade group can have
  5+ entries; is the current density still readable?
- **Unknown death year**: soldiers without a death year get no
  sub-header (`soldier-decade` returns ""). They render at the
  end of their day with no grouping. OK?
- **Multi-page behaviour**: nothing actively changes when
  `total-entries > 50`; typst flows the columns naturally. If
  you want a "Page N of M" footer, say so.
- **Day header alignment**: currently flush left of each
  column. When a day spans a column boundary, the header
  repeats in both columns (one per side). Should the second
  occurrence be suppressed?
