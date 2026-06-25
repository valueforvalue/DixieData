# DixieData UI/UX Audit — Round 3 (verification + flows)

**Date:** 2026-06-24
**Mode:** Verification of round-1/round-2 fixes + interactive-flow harness
**Routes audited:** 24 (12 unique paths × 2 viewports)
**Interactive flows:** 10 (10 PASS, 0 FAIL)
**Raw data:** `audit/reports-r3/`, `audit/screenshots-r3/`

---

## Headline numbers

| Metric | Round 2 | Round 3 |
|---|---:|---:|
| Critical (axe) | 0 | 0 |
| Serious (axe) | 7 | 0 |
| High (visual) | 19 | 0 |
| Medium (visual) | 1 | 5 (small-tap-target) |
| Info (visual) | — | 27 (nav-density) |
| Total findings | 62 | 32 |

**Round 3 confirmed every blocker flagged in rounds 1 and 2 is resolved.**
The remaining 32 findings are non-blocking polish:
- 27 `nav-density` info — top-nav bar runs at 886px on desktop and 111px/item; flagged as
  information-only so future shrinkage stays visible.
- 5 `small-tap-target` medium — `/browse` row labels render as `<a>` elements that can fall
  below the 24×24 WCAG 2.5.5 minimum at narrow viewports; keyboard users unaffected.
- 2 `table-rows` low — `/compare` cell padding tightening opportunity.
- 1 `overflow-x` low — single sub-route deep-link on mobile.

---

## Verification of round-2 fixes

| Round-2 finding | Status |
|---|---|
| `/compare` `scrollable-region-focusable` on mobile | ✅ resolved (`tabindex="0"` added, round 4 acceptance #90) |
| `/compare` "DIFFERENCES TO REVIEW FIRST" pill anchoring | ✅ resolved (anchor links scroll to row) |
| `/soldiers/search/advanced` axe misfire (HTMX fragment) | ✅ resolved (axe scoped to full pages) |
| Top-nav density on desktop | ⚠️ carried as info — see "Carried findings" below |

---

## Interactive flows

All 10 flows PASS. The full harness in `audit/reports-r3/flows.json` records:

| Flow | Outcome |
|---|---|
| hamburger-opens-drawer | PASS — drawer opens, `aria-expanded="true"`, links reachable |
| hamburger-esc-closes | PASS — ESC closes drawer |
| hamburger-focus-returns | PASS — focus returns to toggle button after close |
| browse-filter-applies | PASS — filter narrows visible rows |
| browse-filter-persists-from-url | PASS — deep link restores filter state |
| compare-region-keyboard-accessible | PASS — Tab enters scrollable table |
| compare-differences-pills-present | PASS — pill row above table |
| browse-compare-selection-enabled | PASS — checkboxes enable Compare button |
| feedback-modal-opens-and-closes | PASS — modal opens, ESC + overlay close |
| htmx-library-loaded | PASS — `htmx.min.js` served on every page |

---

## Carried findings (info / future work)

### nav-density

Top-nav shows 8 items at 886px / 111px-each on desktop. WCAG does not set a hard limit, but
the audit flags it as `info` so that any future nav addition triggers re-evaluation. The
mobile hamburger drawer (issue #82, round 4) already absorbs this on viewports <768px.

### small-tap-target on `/browse`

Browse-row text labels render at h=16 on mobile, under the WCAG 2.5.5 24×24 minimum. Touch
users get a larger hit area via the row `<tr>` itself (click anywhere on the row), so the
tap-target concern is theoretical rather than reported in usability testing. Carried as a
follow-up for the round-5 polish pass.

### `/compare` table rows

Two cells use 6px vertical padding where 8px would be more comfortable. Cosmetic.

---

## What this round unblocks

- Issue #82 (hamburger nav drawer) — closed in round 4 against this evidence.
- Issue #85 (compare keyboard) — closed in round 4 against this evidence.
- The audit harness (`audit/harness.mjs` + `audit/run.mjs`) is now reusable for any future
  verification round.

---

## Caveats

- Round 3 used the same 50-soldier seed as round 2; no fresh data was generated.
- `audit/screenshots-r3/` contains the visual evidence and is the source of truth for
  pixel-level claims; the JSON files are derived from the harness output.
- The audit harness does not run against a Wails binary — it targets the dev server
  (`make dev`). Production-path regressions must be re-tested via `make goldmaster`.
