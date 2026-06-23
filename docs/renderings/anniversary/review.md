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
| 28 → 29 | Clickable entries | soldier entries with a Find a Grave record become clickable (soft blue + underline); add 0.15em breathing room between consecutive entries in a decade |

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

## Round 28-29 (clickable entries, signed off)

The anniversary calendar payload used to be
`map[int][]models.Soldier` with no record-level information,
so the template could not link a soldier entry to their Find
a Grave memorial. The user wants each entry's anchor text
(e.g. `D. Henry Feely (DXD-00025, 1911)`) to be a clickable
hyperlink when a FaG record exists; soldiers without a FaG
record render as plain text, matching the pre-link behavior.

`internal/archive/export_service.go` gained a
`firstFindAGraveLinks` helper that runs one bulk query against
the records table:

  SELECT soldier_id, details FROM records
  WHERE LOWER(record_type) LIKE '%find a grave%'
    AND (details LIKE 'http://%' OR details LIKE 'https://%')
    AND soldier_id IN (?, ?, ...)
  ORDER BY soldier_id, id

For each soldier, the first row (lowest id) is taken. The
helper returns a `map[string]string` keyed by soldier id as
string, which the data payload exposes to typst as
`soldier_links`.

`templates/anniversary.typ` gained a `render-anniversary-entry`
helper that wraps a soldier's entry text in a typst `#link()`
when a URL is present, styled in soft blue + underline (the
same `theme.palette.link` colour used by the records-section
"Click to view" links in the record card). The bullet `-`
stays inside the link so the clickable region includes it.
Entries without a URL render as plain text. The link guards
against absurdly long URLs (>4000 chars) by falling back to
plain text, matching the records-section's behaviour.

Round 29 follow-up: a small `v(0.15em)` (~1pt at 7pt body
size) between consecutive entries in a decade so two
soldiers in the same decade (e.g. Day 4's 1910s / 1920s
groups) read as separate visual entries rather than a single
block. The extra space accumulates at the end of the decade
list; the existing `v(0.05em)` between decades still applies.

Data caveat: some soldiers have a FaG record with a
placeholder details string (e.g. "Add link here." or
garbled text like "Adhttps://..."). The bulk query's
`details LIKE 'http://%'` filter skips these, so the entry
renders as plain text even though the record_type says
"Find a Grave". Cleaning up the data is a separate
task — the link-rendering code is correct.

Live confirmations:

- May (round 28): D. Henry Feely, Mary Jane Carter, Joseph
  Monroe Howell, William Alexander Walston, and many more
  linked; Seth Morris, William Esom Dooley, Ruth Ann VanZandt
  rendered as plain text (placeholder URLs).
- June (round 29): same behaviour on the June calendar
  (Rice Eason, William Thomas Jackson Abercrombie, John
  Napoleon Hallum, etc. linked; Martin Jackson Welch
  plain text).
