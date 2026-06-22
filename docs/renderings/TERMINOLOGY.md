# PDF Rendering Terminology

The source of truth for terms that appear in rendered PDF exports. This file is the
contract between the human's annotations and the agent's code changes. Every term
visible in a PDF must resolve to one of the entries below.

**Editing rule.** When a term changes, update this file FIRST, then the template
that emits it. Both the sidecar `review.md` files and the agent's code
diffs cite entries from this list by their anchor (`#term-…`).

## Domain glossary

`CONTEXT.md` is the upstream glossary for domain concepts (Person Record,
Source Record, Spouse Record, Local Archive, etc.). This file defers to it for
domain meaning; it only specifies how each term is *rendered*.

## Rendered terms

### A. Section headers

<a id="term-section-identity-vital-details"></a>
**Identity & Vital Details** — `templates/common/record_card.typ:184` (record card).
Hard-coded. OK. Anchor: `#term-section-identity-vital-details`.

<a id="term-section-service-archive-details"></a>
**Service & Archive Details** — `templates/common/record_card.typ:206` (record card).
Hard-coded. OK. Anchor: `#term-section-service-archive-details`.

<a id="term-section-household-context"></a>
**Household & Context** — `templates/common/record_card.typ:269` (record card).
Hard-coded. OK. Anchor: `#term-section-household-context`.

<a id="term-section-source-records"></a>
**Source Records** — `templates/common/record_card.typ:290` (record card, loop header).
Drives the source-evidence list. **Inconsistent with glossary:** template prints
`Records`; glossary §"Source Record" requires `Source Records`. Open.
Anchor: `#term-section-source-records`.

<a id="term-section-biography"></a>
**Biography** — `templates/common/record_card.typ:339` and `:362` (record card).
Drives `soldier.biography`. OK. Anchors: `#term-section-biography`,
`#term-section-biography-page-title`.

<a id="term-section-biography-appendix"></a>
**Full Biography Appendix** — `templates/biography_appendix.typ:42`.
Hard-coded subtitle for the optional biography-only export. OK.
Anchor: `#term-section-biography-appendix`.

<a id="term-section-archive-summary-report"></a>
**Local Archive Summary Report** — `templates/analytics_summary.typ:73`.
**Inconsistent with glossary:** template prints `Archive Summary Report`;
glossary §"Local Archive" forbids bare "Archive". Open.
Anchor: `#term-section-archive-summary-report`.

<a id="term-section-person-record-types"></a>
**Person Record Types** — `templates/analytics_summary.typ:81`.
**Inconsistent with glossary:** template prints `Record Types`; glossary
§"Person Record" forbids bare "Record". Open.
Anchor: `#term-section-person-record-types`.

<a id="term-section-top-cemeteries"></a>
**Top Cemeteries** — `templates/analytics_summary.typ:92`. OK.
Anchor: `#term-section-top-cemeteries`.

<a id="term-section-confederate-home-participation"></a>
**Confederate Home Participation** — `templates/analytics_summary.typ:98`. OK.
Anchor: `#term-section-confederate-home-participation`.

<a id="term-section-pension-distribution"></a>
**Pension Distribution** — `templates/analytics_summary.typ:113`. OK.
Anchor: `#term-section-pension-distribution`.

<a id="term-section-unit-representation"></a>
**Unit Representation** — `templates/analytics_summary.typ:120`. OK.
Anchor: `#term-section-unit-representation`.

<a id="term-section-chronological-overview"></a>
**Chronological Overview** — `templates/analytics_summary.typ:127`. OK.
Anchor: `#term-section-chronological-overview`.

<a id="term-section-month-anniversary"></a>
**`<Month> Anniversary Report`** — `templates/anniversary.typ:54`. The month
name is interpolated from `data.month`. OK.
Anchor: `#term-section-month-anniversary`.

<a id="term-section-day-n"></a>
**Day `<n>`** — `templates/anniversary.typ:67`. Heading per day of the month.
OK. Anchor: `#term-section-day-n`.

### B. Field labels (record card)

All emitted via `field-row()` at `templates/common/record_card.typ:130-134`.
Sources for the literal string are the line numbers in `record_card.typ`.

| Label | Source line | Data field | Status |
|---|---|---|---|
| `Prefix` | 189 | `soldier.prefix` | OK |
| `First Name` | 190 | `soldier.first_name` | OK |
| `Middle Name` | 191 | `soldier.middle_name` | OK |
| `Last Name` | 192 | `soldier.last_name` | OK |
| `Suffix` | 193 | `soldier.suffix` | OK |
| `Birth Date` | 194 | `soldier.birth_date` | OK |
| `Death Date` | 195 | `soldier.death_date` | OK |
| `Birth Info` | 196 | `soldier.birth_info` | OK |
| `Buried In` | 197 | `soldier.buried_in` | **CONFLICT** — see §G |
| `Person Record Type` | 217 | `soldier.entry_type` | **Inconsistent with glossary:** template prints `Record Type`; glossary requires `Person Record Type`. Open. |
| `Rank In` | 218 | `soldier.rank_in` | OK |
| `Rank Out` | 219 | `soldier.rank_out` | OK |
| `Unit` | 220 | `soldier.unit` | OK |
| `Pension State` | 221 | `soldier.pension_state` | OK |
| `Pension ID` | 222 | `soldier.pension_id` | OK |
| `Application ID` | 223 | `soldier.application_id` | OK |
| `Confederate Home Status` | 224 | `soldier.confederate_home_status` | OK |
| `Confederate Home Name` | 225 | `soldier.confederate_home_name` | OK |
| `Spouse` | 276 | `soldier.spouse_name` | OK |
| `Linked Spouse Record` | 289 | `soldier.spouse_soldier_id` | OK |
| `Maiden Name` | 296 | `soldier.maiden_name` | OK |

Inline labels (not via `field-row`):
- `App:` — `record_card.typ:297` for `record.app_id`. OK.
- Bolded `record_type` — `record_card.typ:297` for `record.record_type`. OK.

### C. Group divider titles

Emitted as `Grouped by <label>` prefix in `templates/group_divider.typ:32` and
`templates/bulk_soldier.typ:39`. Labels come from `groupAxisLabel()` at
`pkg/render/render.go:392-405`.

| Axis | `groupAxisLabel()` returns | Displayed as | Status |
|---|---|---|---|
| `unit` | `Unit` | `Grouped by Unit` | OK |
| `pension_state` | `Pension State` | `Grouped by Pension State` | OK |
| `confederate_home_status` | `Confederate Home Status` | `Grouped by Confederate Home Status` | OK |
| `buried_in` | `Burial Location` | `Grouped by Burial Location` | **CONFLICT** — see §G |

Divider body line: `The following record pages belong to this section.`
(`group_divider.typ:46`, `bulk_soldier.typ:54`). OK.

### D. Bulk archive headers / footers

No bulk-specific page header. Bulk uses the same `page-params()` as every
other template (`record_card.typ:383-410`).

- Page header: `<archive_title>` from `branding.archive_title`
  (`pkg/encode/encode.go:108-113`). Default literal:
  `<BrandingName>'s Civil War Research Archive`. OK.
- Page footer: `<footer_text>` from `branding.footer_text`
  (`pkg/encode/encode.go:114`). Default literal:
  `Made with DixieData | Version: <app_version> | Build: <build_identity>`.
  Suppressed when `opts.printerFriendly == true`
  (`record_card.typ:394`). OK.

### E. Empty-state / placeholder text

| String | Location | Condition | Status |
|---|---|---|---|
| `Unknown` (date) | `record_card.typ:58, 62, 89, 95` | empty / `0000-00-00` | OK |
| `Soldier` (entry-type fallback) | `record_card.typ:102` | empty `entry_type` | OK |
| `Person Record` (entry-type label) | `record_card.typ:106` | `entry_type == "linked_person"` | OK |
| `N/A` (pension state / id / app id / CH status / CH name) | `record_card.typ:221-225` | blank field | OK |
| `DB ID <n>` / `<name> (DB ID <n>)` | `record_card.typ:283-284` | `spouse_soldier_id` non-zero | OK |
| `Month Anniversary Report` | `anniversary.typ:48` | `data.month` not in `month-names` map | OK |
| `No soldiers are recorded for this month.` | `anniversary.typ:62` | `calendar` empty | OK |
| `No Person Records to summarise.` | `analytics_summary.typ:85` | all `record_types` counts zero | **Inconsistent with glossary:** template prints `No records to summarise.` Open. |
| `No burial locations are recorded yet.` | `analytics_summary.typ:93` | empty `cemetery_density` | OK |
| `No Confederate Home statuses are recorded yet.` | `analytics_summary.typ:104` | empty | OK |
| `No Confederate Home names are recorded yet.` | `analytics_summary.typ:109` | empty | OK |
| `No pension states are recorded yet.` | `analytics_summary.typ:116` | empty | OK |
| `No units are recorded yet.` | `analytics_summary.typ:123` | empty | OK |
| `No birth decades are recorded yet.` | `analytics_summary.typ:134` | empty | OK |
| `No death decades are recorded yet.` | `analytics_summary.typ:139` | empty | OK |
| `No biography recorded for this person.` | `biography_appendix.typ:49` | blank `biography` | OK |

### F. Filter / sort / export UI labels echoed into PDF

**None.** The bulk header does not echo the user's filter or sort
selections. Orientation is consumed silently to set page size; nothing
prints the words "Landscape" or "Portrait".

### G. Open conflicts (must resolve before iteration)

<a id="term-burial"></a>
**Burial location / Buried In / BuriedIn.** Three surface strings for
the same concept:
- Field label: `Buried In` (record card, `record_card.typ:197`).
- Group axis label: `Burial Location` (`pkg/render/render.go`).
- Go field name: `BuriedIn` (struct tag, no PDF surface).
- The split archive across all 9 export surfaces uses both forms. **Pick
  one for the rendered surface.** Glossary does not pin a term. **Open.**
Anchor: `#term-burial`.

<a id="term-glossary-records-vs-source-records"></a>
**Records vs Source Records.** Section header at
`record_card.typ:290` reads `Records`; should be `Source Records` per
glossary §"Source Record". **Open.**
Anchor: `#term-glossary-records-vs-source-records`.

<a id="term-glossary-record-type-vs-person-record-type"></a>
**Record Type vs Person Record Type.** Field label at
`record_card.typ:217` reads `Record Type`; should be `Person Record Type`
per glossary §"Person Record". **Open.**
Anchor: `#term-glossary-record-type-vs-person-record-type`.

<a id="term-glossary-archive-summary"></a>
**Archive Summary Report vs Local Archive Summary Report.**
Title at `analytics_summary.typ:73` reads `Archive Summary Report`;
should be `Local Archive Summary Report` per glossary §"Local Archive".
**Open.** Anchor: `#term-glossary-archive-summary`.

<a id="term-glossary-record-types-vs-person-record-types"></a>
**Record Types vs Person Record Types.** Section header at
`analytics_summary.typ:81` reads `Record Types`; should be
`Person Record Types` per glossary §"Person Record". **Open.**
Anchor: `#term-glossary-record-types-vs-person-record-types`.

<a id="term-glossary-no-records"></a>
**`No records to summarise.` vs `No Person Records to summarise.`**
Empty-state at `analytics_summary.typ:85`. **Open.**
Anchor: `#term-glossary-no-records`.

## Open glossary entries (terms that exist in the data but have no PDF surface yet)

These are documented so a future iteration can add a section for them:

- **Service Timeline** — `CONTEXT.md` term; not rendered.
- **Source Records** — `CONTEXT.md` term; only loosely rendered as `Records` (see §G).
- **Claim** / **Finding** — `CONTEXT.md` terms; not rendered.
- **Scratch Pad** / **Research Log** — `CONTEXT.md` terms; not rendered.
- **Research Collection** / **Research Pack** — `CONTEXT.md` terms; not rendered.
- **Display ID** — rendered in the title block of every record card but not
  as a labelled row.
