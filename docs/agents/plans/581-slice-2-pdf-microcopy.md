# Plan — issue #581, Slice 2: PDF microcopy audit

This plan covers **Slice 2 of issue #581** — the audit + remediation
of the Typst PDF templates and their shared helper. Slice 1 (Static
Archive HTML) shipped in `ed37efd`. Slices 3 (iCalendar) and 4
(runtime / server / CLI) are out of scope here; they get their own
plans. Decomposition follows the `docs/agents/notes/slice3-decomposition.md`
pattern (one slice = one commit = one reviewable capability), the
`docs/agents/tdd.md` RED-first discipline, and the slice-1 commit
shape (probe + tests + driver wiring + fix + ux doc + CHANGELOG in
one reviewable commit).

## Why this is its own slice

**#581 locked decision 4**: "Format-specific rules or manual review
are acceptable where HTML-oriented `.templ` rules do not safely
translate." Typst is a different grammar from Go-templ + HTML. The
`.templ` probe (`audit/smoke_microcopy.mjs`) walks HTML markup and
removes redundant eyebrow text + body narration under headings; it
cannot meaningfully parse Typst markup. A format-aware probe is
required.

**Pragmatic guidance from `docs/agents/pragmatic-principles.md`**:
the audit is a one-shot correctness pass; broad strategic interfaces
and YAGNI both point at "ship the smallest probe that catches the
regression class", not at "build a general Typst linter".

## Surface inventory (verified by direct read)

17 Typst files (2,583 lines), grouped by role:

| File | Role | Orient. | Primary heading | Sub-headings |
|---|---|---|---|---|
| `templates/common/record_card.typ` | shared helper, all per-record PDFs | both | (helpers, no chrome) | — |
| `templates/common/theme.typ` | palette + page helpers | — | — | — |
| `templates/soldier_landscape.typ` | per-soldier card | landscape | (delegated) | — |
| `templates/soldier_portrait.typ` | per-soldier card | portrait | (delegated) | — |
| `templates/widow_landscape.typ` | per-widow card | landscape | (delegated) | — |
| `templates/widow_portrait.typ` | per-widow card | portrait | (delegated) | — |
| `templates/spouse_landscape.typ` | per-spouse card | landscape | (delegated) | — |
| `templates/spouse_portrait.typ` | per-spouse card | portrait | (delegated) | — |
| `templates/bulk_soldier.typ` | bulk archive + group dividers | any | "Grouped by …" eyebrow + value | — |
| `templates/event_landscape.typ` | per-event card | landscape | event title | INTERNAL NOTES, DISPLAY ID / NAME / DATES, LINKED PERSON RECORDS |
| `templates/event_portrait.typ` | per-event card | portrait | event title | (as above) |
| `templates/article_landscape.typ` | per-article card | landscape | article title | — |
| `templates/article_portrait.typ` | per-article card | portrait | article title | — |
| `templates/anniversary.typ` | monthly anniversary report | portrait | "{Month} Anniversary Report" | "Day N" headers |
| `templates/analytics_summary.typ` | archive analytics | portrait | "Archive Summary Report" | Record Types / Top Cemeteries / Confederate Home Participation {Status breakdown, Most frequent home names} / Pension Distribution / Unit Representation / Chronological Overview {Birth decades, Death decades} |
| `templates/biography_appendix.typ` | biography appendix | portrait | compose-name + "{DisplayID} | {EntryType} | Full Biography Appendix" + "Biography" | — |
| `templates/group_divider.typ` | inter-group divider | portrait | "Grouped by {label}" eyebrow + "{value}" | — |
| `templates/hello.typ` | smoke template | — | — | (smoke only, skip) |

The analytics_summary + group_divider + bulk_soldier + event cards
are the high-density offenders because they stack weight:bold
typography in patterns the #561 `.templ` R4 + R5 rules classify as
redundant. The per-record landscape/portrait cards delegate to
`common/record_card.typ`, which means a single helper-level fix
covers 6 thin wrappers. See "Confirmed findings".

## Confirmed findings (catalogued during recon)

Each row is independently fixable in one commit; the slice as a
whole lands together as a single reviewable unit (one
decomposition, one commit) following the slice-1 precedent — they
all target the same `templates/` surface and the same probe, so
isolation gain from splitting further is nil and the review cost
of splitting is high.

| # | File:line | Verbatim (or paraphrase) | Class | Smallest fix |
|---|---|---|---|---|
| 1 | `templates/analytics_summary.typ:60-65` | H1 "Archive Summary Report" + immediately-following 38-word subtitle paragraph "High-level archive analytics covering burial density, Confederate Home participation, record types, pension geography, unit representation, and decade trends." | R5 verbose body under heading | drop paragraph; H1 stands |
| 2 | `templates/analytics_summary.typ:75-79` | bold "Record Types" heading + bullet list of 3 entries that each repeat the heading content | R4 stacked heading + R5 verbose | collapse bullet values into one comma-separated line |
| 3 | `templates/analytics_summary.typ:82-85` | bold "Top Cemeteries" heading + immediately-following bullet-list | R4 redundant role | promote to a single one-line label + count |
| 4 | `templates/analytics_summary.typ:89-95` | bold "Status breakdown" sub-heading directly under bold "Confederate Home Participation" heading | R4 stacked headings w/o body between | drop "Status breakdown" (its bullets are self-explanatory after the parent heading) |
| 5 | `templates/analytics_summary.typ:97-100` | "Most frequent home names" sub-heading same shape | R4 | drop |
| 6 | `templates/analytics_summary.typ:107-110` | "Pension Distribution" same shape | R4 | trim |
| 7 | `templates/analytics_summary.typ:115-118` | "Unit Representation" same shape | R4 | trim |
| 8 | `templates/analytics_summary.typ:123-126` | "Chronological Overview" + sub-headings "Birth decades" / "Death decades" | R4 stacked | drop "Birth decades" / "Death decades" (the decade data is self-evident from the entry names) |
| 9 | `templates/group_divider.typ:38-50` | eyebrow "Grouped by {label}" followed by value title followed by trailing small-print "The following record pages belong to this section." | R5 verbose body trailing a title | drop trailing sentence (the title + value is self-explanatory; the divider page itself is the instruction) |
| 10 | `templates/bulk_soldier.typ:43-58` | inlined `render-divider-page` duplicates the group_divider template structure with the same trailing "The following record pages belong to this section." | R4 cross-template redundant copy | replace inlined body with `import "group_divider.typ": render-divider-page` (already a function-like block); drop trailing sentence once at the source |
| 11 | `templates/event_landscape.typ:265` + 318 | "INTERNAL NOTES" + "LINKED PERSON RECORDS" ALL-CAPS eyebrows on labels of single-field groups | R2 unnecessary title | keep label-as-bold, drop ALL-CAPS eyebrow (or merge into label) |
| 12 | `templates/event_portrait.typ` | mirror of 11 | R2 | mirror |
| 13 | `templates/biography_appendix.typ:34-43` | "Biography" bold heading + leading empty state "No biography recorded for this person." (mirrors #561 R5 status-form pattern) | R5 status-form empty state | rephrase to action-form ("Add a biography in the editor to include it in this appendix") OR drop the empty state (the heading alone suffices); accept either, prefer drop |
| 14 | `templates/anniversary.typ` "Day N" headers | "Day 4" + immediately-following italic decade label "1840s" + bullet list — same stacked-heading shape | R4 stacked | convert decade to inline parenthetical on the bullet ("(1840s)") or drop (the bullets carry the year) |
| 15 | `templates/common/record_card.typ` `render-link` | "Click to view" anchor already aligned across PDF + Static Archive per #541 — no change | (none, alignment preserved) | — |

**Out-of-scope skips (recorded, not fixed):**

- `hello.typ` — smoke harness target, not user-facing.
- `theme.typ` — palette + page-params helpers, no chrome.
- Landscape/portrait orient-only variants of the per-record cards
  (soldier/widow/spouse) — they delegate to `common/record_card.typ`
  with no chrome of their own; the helper-level fix in row 15 covers
  them. A future #583-class consolidation is the right home if
  drift ever appears.
- `event_landscape`/`event_portrait` per-row "DISPLAY ID" / "NAME"
  / "DATES" column labels (row 11 notes the eyebrows; the column
  labels are real table headers, not chrome — leave them).

## Slice decomposition (one commit = one reviewable unit)

### Slice 2A — Probe + driver wiring + RED fixtures

**Goal**: ship the probe with required/forbidden patterns + a per-
template fixture that pins each known finding verbatim, BEFORE
code edits land. The probe exits 0 on the current repo
(baseline-on-HEAD) per `#561` slice-5 precedent — the same
`--strict` gate then flips AFTER fixes land.

**Files added**:

- `audit/smoke_pdf_microcopy.mjs` — format-aware probe. Walks
  `templates/**/*.typ`, applies:
  - **Required copy** (lines flagged when missing): each per-
    template title + glossary-aligned "Person Record" /
    "Source Record" labels when present in the source.
  - **Forbidden copy** (lines flagged when present): the
    `analytics_summary.typ` verbose subtitle, the "Grouped by
    {label}" eyebrow + trailing "The following record pages
    belong to this section.", the "Status breakdown" / "Most
    frequent home names" / "Birth decades" / "Death decades"
    sub-headings, "INTERNAL NOTES" / "LINKED PERSON RECORDS"
    ALL-CAPS eyebrows, the "No biography recorded for this
    person." status-form sentence.
  - **Duplicate copy**: any phrase appearing twice inside the
    same template file (case-insensitive, whitespace-collapsed)
    is counted; if count > 1, flag.
  - Exclusion: `templates/hello.typ`, `templates/common/theme.typ`,
    templ metadata blocks (lines starting with `//`), Typst code
    expressions (`#let`, `#set`, `#import`, `#if`, `#for`).
  - Mirrors the slice-1 probe shape: `STATIC_ARCHIVE_SOURCE`
    env-override → use `PDF_SOURCE` env-override here.
- `audit/smoke_pdf_microcopy.test.mjs` — 16 assertions: 5 baseline-
  on-HEAD (probe exits 0, summary present, each canonical rule
  fires on a synthetic fixture), 8 negative-path assertions (each
  rule's required text is present in current repo, each rule's
  forbidden text is present in current repo BEFORE the fix), 3
  duplicate-detection assertions, plus the strict-mode flip
  (exits 1 when forbidden text remains). Mirrors
  `smoke_static_archive_microcopy.test.mjs`.
- `Makefile`: add `lint-pdf-microcopy` (calls probe) and
  `lint-pdf-microcopy-test` (calls test).
- `.github/workflows/test.yml`: add both calls to the audit step.
- `docs/agents/ux-microcopy.md`: new section "Static Archive
  coverage" gets a sibling "PDF coverage" paragraph mirroring
  slice-1's prose.

**Per `docs/agents/tdd.md`**: the RED tests in
`smoke_pdf_microcopy.test.mjs` ARE the acceptance criterion. The
slice is GREEN when `node audit/smoke_pdf_microcopy.test.mjs` is
green on the post-fix repo with `--strict` exiting 0.

**Cross-artifact contract**: this slice mirrors
`audit/smoke_static_archive_microcopy.mjs` line-for-line where
possible (env override name, strinct exit code, finding
shape). The review-cost reduction from reusing slice 1's
contract is the whole point.

### Slice 2B — Fixes (the actual microcopy edits)

**Goal**: shrink the Typst chrome to the agreed-on helper copy +
verified required labels.

**Files changed** (14 edits across 5 templates — see "Confirmed
findings" rows 1–13):

- `templates/analytics_summary.typ` — rows 1–8.
- `templates/group_divider.typ` — row 9.
- `templates/bulk_soldier.typ` — row 10 (replace inlined
  `render-divider-page` with an `#import` of the shared
  `group_divider.typ` module); pre-existing helpers remain the
  same name, so no Go-side template-resolution changes
  required.
- `templates/event_landscape.typ` — row 11.
- `templates/event_portrait.typ` — row 11 mirror.
- `templates/biography_appendix.typ` — row 13.
- `templates/anniversary.typ` — row 14 (decade label inline).

**Constraint**: no domain meaning changes; no machine-readable
export contract changes (PDF binary layout, record-card
provenance headers, page chrome dimensions); no glossary
violations (`Person Record`, `Source Record`, `Display ID`
remain canonical).

**Files updated to drop assertions on removed text** (mirror
slice-1, `669a761` precedent):

- `tools/tune/snapshot_test.go` — drop any substring assertion
  on `The following record pages belong to this section.` or
  the verbose analytics subtitle (audit confirms current
  snapshot does not assert these — verify in `grep` before
  editing).
- `internal/archive/export_service_test.go` and the relevant
  snapshot test files — same audit pass.

**Fixture-driven regression test** (committed in 2A, GREEN in 2B):

- The probe's required/forbidden patterns + the strict flip
  pin the post-fix state. Fixture text in
  `audit/smoke_pdf_microcopy.test.mjs` pins each removed line
  by string, so a partial fix breaks the test.

### Slice 2C — CHANGELOG + commit

**Goal**: close the slice with the same review-trail shape that
slice 1 used (`ed37efd`).

**Files changed**:

- `CHANGELOG.md`: `[Unreleased]` `### Fixed` bullet naming
  issue #581 + the slice shape + the regression net + the
  format-aware probe.

**Commit subject**: `pdf: tighten Typst microcopy (#581 slice 2)`.
**Commit body** mirrors slice 1:
- First bullet: fix class summary (verbose body trailing analytics
  title, stacked analytics sub-headings, group divider trailing
  narration, INTERNAL NOTES / LINKED PERSON RECORDS eyebrows,
  biography appendix status-form empty state, anniversary decade
  stacked headings).
- Second bullet: regression net — `audit/smoke_pdf_microcopy.mjs`
  + `audit/smoke_pdf_microcopy.test.mjs`, Make + CI wiring, doc
  amendment to `docs/agents/ux-microcopy.md`.
- Third bullet: out-of-scope notes (`hello.typ`, the per-record
  helper, the column-header labels in the event cards).

## Acceptance criteria for the whole slice

- [ ] `node audit/smoke_pdf_microcopy.mjs --strict` exits 0.
- [ ] `node audit/smoke_pdf_microcopy.test.mjs` reports all
      assertions green.
- [ ] `make lint-pdf-microcopy` + `make lint-pdf-microcopy-test`
      both exit 0.
- [ ] CI's audit step runs both new probes alongside
      `smoke_microcopy.mjs` + `smoke_static_archive_microcopy.mjs`.
- [ ] `docs/agents/ux-microcopy.md` carries the "PDF coverage"
      paragraph mirroring the "Static Archive coverage" prose.
- [ ] `CHANGELOG.md` `[Unreleased]` `### Fixed` bullet references
      #581 and the regression net.
- [ ] `go test -short ./...` stays green (verify by running the
      subset that covers archive, tune, records — the surfaces
      with snapshot tests).
- [ ] `tools/tune` renders all 17 templates against the gold
      archive with no missing-required warning output.

## Principle warnings (`docs/agents/pragmatic-principles.md`)

- **DRY (§2) — temporarily violated by row 10.** The bulk
  soldier template inlines `render-divider-page` rather than
  importing from `group_divider.typ`. Row 10 deduplicates by
  converting the inline block to an `#import`. The principle
  is upheld by the fix; the violation is the bug being fixed.
  No warn+cite needed in the commit body.
- **Orthogonality (§4) — format-aware probe is justified** by
  locked decision 4 of #581. Without format-aware rules the
  `.templ` probe would either false-positive (treating Typst
  identifiers as headings) or false-negative (missing Typst-
  specific stacked-heading patterns). The probe is small
  (~150 LoC) and one-shot; no risk of growing into a generic
  Typst linter.

## Verification commands

```text
node audit/smoke_pdf_microcopy.mjs --strict           # should be 0 findings
node audit/smoke_pdf_microcopy.test.mjs               # all assertions green
node audit/smoke_microcopy.mjs --strict               # slice-1 still green
node audit/smoke_static_archive_microcopy.mjs --strict # slice-1 still green
go test -short -count=1 ./...                         # no regressions
```

## Out of scope (deferred to future issues per #581 locked decisions)

- Slice 3 (iCalendar) — separate plan, separate PR.
- Slice 4 (runtime / server / startup / CLI) — separate plan.
- General Typst linter — `#581` is microcopy-only.
- Renaming the landscape/portrait helper files (e.g.
  `soldier_landscape.typ` → ...) — not a UX concern.

## Related

- `ed37efd` — slice 1 (Static Archive HTML) — same commit
  shape, same probe contract.
- `docs/agents/ux-microcopy.md` — the rule spec the probe
  enforces. This slice extends the spec's coverage map; it
  does not change the five rules.
- `docs/agents/tdd.md` — RED-first discipline.
- `docs/agents/notes/slice3-decomposition.md` — the
  decomposition pattern (slice = one commit = one reviewable
  unit, with a pre-slice RED test).
