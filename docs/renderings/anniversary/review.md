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
| 23 → 27 | Page chrome | use shared page-params() (header + footer rules), title sits flush between top rule and title's own rule (no leading) |

## Round 23-27 (page chrome alignment, signed off)

The anniversary template was overriding the shared page-params
helper to drop the horizontal rules from the page header and
footer. The user wants anniversary to match the chrome of the
record-card landscape PDFs (header rule below the archive
title, footer rule above the build line).

Round 23 fix: drop the override in `anniversary.typ`; use
`#set page(..page-params(is-landscape, branding, opts))`
directly. The shared helper provides the same chrome as the
landscape record cards.

Round 24-27 follow-up: the body's "May Anniversary Report"
title sat with asymmetric padding (no leading above the title,
`v(0.4em)` below the title's rule). The user wanted the gap
above and below the title equal, then smaller, then smaller
still. Final state: title is flush against the page header
rule and flush against the title's own rule (no leading on
either side). A small `v(0.1em)` after the title's rule
reserves breathing room before the body content (Day 1, ...)
starts.
