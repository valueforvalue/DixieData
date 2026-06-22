# anniversary

Monthly anniversary report. Template: `anniversary.typ`. The
`anniversary` subcommand of `dixiedata-tune` produces this PDF
via `pkg/exportbridge.BulkRenderer.RenderAnniversary` →
`internal/archive.ExportService.ExportMonthlyAnniversaryPDF`.

## Round 1 → Round 2 changes

### 1. Days were out of sequential order

**Round 1:** `Day 1, Day 11, Day 12, Day 13, ..., Day 19, Day 2, ...`

**Round 2:** `Day 1, Day 2, Day 4, Day 6, Day 7, ...` (numerical)

The Go map serializes to JSON with string keys. Typst's
`.sorted()` was sorting lexicographically. Fix: `.sorted(key: d
=> int(d))`.

### 2. Subtitle text was present and shouldn't be

Removed: `Includes soldier names and database numbers for the
selected month.`

### 3. Soldier names were missing

**Round 1:** `DXD-00025 (DXD-00025)`

**Round 2:** `D. Henry Feely (DXD-00025)`

The template was reading `s.display_name` but the JSON payload
doesn't include that field (Go's `models.Soldier` doesn't have a
`display_name` json tag — `DisplayName` is a method, not a field).
Fix: assemble the name in the template from `first_name`,
`middle_name`, `last_name` (a `soldier-name()` helper).

### 4. Two-column layout

Wrapped the day loop in `columns(2, gutter: 1.2em)`. Page grew
from 1 page to 2 pages because the May calendar has 30 days × 1
soldier on average = a lot of content. See open question below
about tightening.

### 5. Review.md content (not a code issue)

The first round's review.md was copied from
`single-soldier-landscape/` and contained the wrong surface
description at the top. The file has been replaced with this
content. Anniversary uses its own template (`anniversary.typ`),
not `soldier_landscape.typ`.

## Open questions for the next round

- **Tighter layout.** The current 2-page result is 92KB; would
  the user prefer 1 page with smaller font, or is 2 pages OK?
- **Section per soldier.** The current layout is one line per
  soldier with no further detail. Would you like each soldier to
  also show their death year (the anniversary type) so the user
  can tell birth vs death at a glance?
- **Group by anniversary type.** Birth and death anniversaries
  are currently mixed within a day. The data model has
  `birth_date` and `death_date`; should the layout show them
  separately?
