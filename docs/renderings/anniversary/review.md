# anniversary

Monthly anniversary report. Template: `anniversary.typ`. The
`anniversary` subcommand of `dixiedata-tune` produces this PDF
via `pkg/exportbridge.BulkRenderer.RenderAnniversary` →
`internal/archive.ExportService.ExportMonthlyAnniversaryPDF`.

## Round 5 → Round 6 changes

User annotations in round 5:

1. "No" — don't future-proof `(cont.)` detection.
2. "No" — don't remove footer on a cover page.
3. "Yes" — sort by last name within a decade.
4. "This one is done for now" — anniversary iteration
   wrapping up.

### What changed

- **Sort within a decade**: dropped the `death_year`
  tie-breaker in the day sort. Now sorts by
  `(last_name, first_name)` only. Within a decade, soldiers
  appear alphabetically. Verified on Day 12 (4 soldiers
  split across two columns): 1900s renders
  `Brown, Charles Lewis` before `Looney, William Pickney` —
  alphabetical.

- No other changes. Footer, header override, decade headers,
  unknown-year bucketing, day-header repeat handling all
  carried forward from previous rounds.

- Still 1 page on May (43) and February (45).

## Status: this surface is **done** for now

The user said "this one is done for now" — moving on to other
surfaces. Future work items that came out of the iteration
loop but were declined or deferred:

- Future-proof `(cont.)` detection (declined; no current day
  spans columns).
- Footer-less cover page (declined).
- `Person Record` vs `Record` terminology conflict still
  unaddressed (tracked in `docs/renderings/TERMINOLOGY.md`).

## Iteration history

| Round | Theme | Key change |
|---|---|---|
| 1 → 2 | Bug fixes | days sorted numerically, names shown, subtitle removed |
| 2 → 3 | Density | one page, two columns, smaller font, death year inline |
| 3 → 4 | Branding | title-to-body spacing, footer on by default, horizontal rule under title |
| 4 → 5 | Refinements | smaller footer, unknown year bucketed, day-header repeat investigated |
| 5 → 6 | Sort | alphabetical by last name within a decade |
