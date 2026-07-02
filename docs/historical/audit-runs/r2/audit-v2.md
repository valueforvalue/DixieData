# DixieData UI/UX Audit — Round 2 (deep routes)

**Date:** 2026-06-24
**Seed:** 50 soldiers (soldier ids 47, 35 used for sub-route exploration)
**Routes added beyond round 1:**
- `/soldiers/{id}/camaraderie`
- `/soldiers/{id}/timeline`
- `/soldiers/{id}/research-log`
- `/soldiers/{id}/conflict-ledger`
- `/soldiers/{id}/research-pack/state`
- `/soldiers/{id}/research-pack/county`
- `/compare?id1={a}&id2={b}`
- `/research-collections`
- `/soldiers/search/advanced?q=...` (fragment endpoint)
- `/soldiers/{id}` (second sample)

**Total routes audited (rounds 1+2):** 23 unique × 3 viewports = 69 page views
**Raw data:** `audit/reports-r2/`, `audit/screenshots-r2/`

---

## Headline numbers

| Severity | Round 1 | Round 2 |
|---|---:|---:|
| Critical (axe) | 282 | 0 |
| Serious (axe) | 9 | 7 |
| High (visual) | 28 | 19 |
| Medium (visual) | 22 | 1 |
| Total findings | 393 | 62 |

**The deep routes are mostly read-only and clean.** No critical a11y issues. The serious ones are concentrated in `/soldiers/search/advanced` (which is an HTMX fragment endpoint, not a full page — see "Caveats" below) and one mobile `scrollable-region-focusable` on `/compare`.

---

## Concrete problems observed

### `/compare` on mobile — fundamentally broken

The page is designed as a **side-by-side diff table** (DXD-00047 column | DXD-00035 column). On desktop it works. On mobile (390px) the table is wider than the viewport, so:

1. Only the **left** record's column is visible without scrolling
2. **No keyboard affordance** to scroll the table (`scrollable-region-focusable` violation)
3. The "DIFFERENCES TO REVIEW FIRST" pill row sits above the table — clicking a pill should jump to / filter the row, but unclear if it does

**Fix options:**
- Add `tabindex="0"` to the `.overflow-x-auto` container (1-line fix, unblocks keyboard users)
- OR collapse to stacked cards on mobile: each row becomes a card showing "DXD-00047: Robert | DXD-00035: Elijah" instead of two columns
- The pill row should anchor-scroll to the matching row in the table

### `/soldiers/{id}/camaraderie` and `/timeline` — read-only, clean

Both rendered without critical a11y issues. Same mobile overflow + nav density as round 1. No unique problems.

### `/soldiers/{id}/research-pack/{state|county}` — fragment-loaded content

These load the research pack view via htmx. Same mobile overflow issues but no new problems.

### `/soldiers/search/advanced?q=...` — fragment endpoint, axe misfires

The handler returns only the inner HTML fragment (no `<html>`, `<head>`, `<title>`) because it's an htmx swap target. Axe runs against the fragment and reports `document-title` and `html-has-lang` violations — these are **not real bugs**, just artefacts of running axe against an HTMX fragment.

**Fix:** carve out fragment endpoints from axe scans (or accept the noise). Either way, this doesn't affect users — the fragment swaps into a page that already has a title and lang.

### Floating dock overlaps content on every deep route

The floating dock (Scratch Pad / Feedback / Menu) sits at `position: fixed; bottom: 6; right: 6` and overlays content on every page, including the new deep routes. On `/compare` mobile it sits on top of the second record's "1 image(s)" pill. On desktop compare it overlaps the right edge of the table. **This is the highest-priority visual fix.**

---

## Per-route notes (round 2 only)

| Route | A11y issues | Notes |
|---|---|---|
| `/soldiers/47/camaraderie` | clean | unit graph; mobile overflow |
| `/soldiers/47/timeline` | clean | anniversary timeline; mobile overflow |
| `/soldiers/47/research-log` | clean | log entries; mobile overflow |
| `/soldiers/47/conflict-ledger` | clean | ledger table; mobile overflow |
| `/soldiers/47/research-pack/state` | clean | research pack view; mobile overflow |
| `/soldiers/47/research-pack/county` | clean | research pack view; mobile overflow |
| `/compare?id1=47&id2=35` | 1 serious (mobile) | side-by-side diff; mobile scrollable-region-focusable |
| `/research-collections` | clean | empty hub state |
| `/soldiers/search/advanced?q=test` | 2 serious × 3 viewports | fragment endpoint; axe misfires — see above |
| `/soldiers/35` | clean | same as `/soldiers/47` (soldier detail) |

---

## Caveats and limitations

### Fragment endpoints vs full pages

Endpoints that return HTMX fragments don't render a complete HTML document. Running axe against them produces false positives. Affected endpoints identified:

- `/soldiers/search/advanced`
- `/soldiers/search/recent` (likely — not directly audited)
- `/soldiers/search`
- `/soldiers/search?browse=1`
- `/insights/drilldown`
- `/soldiers/{id}/images/*` (fragment responses)
- `/soldiers/{id}/pdf` (form, not fragment)

**Recommendation:** have the harness detect fragment responses (`Content-Type: text/html` but no `<html>`) and skip axe for them. Add this in round 3.

### Empty-state seed

`/review-queue`, `/merge-review/*`, `/research-collections/{id}` show empty states with the current 50-soldier seed (no duplicates, no conflicts, no collections). Audit of those states is partial — populated state would surface different issues. Round 3 should add a richer seed or fixture data.

### Static-only audit

The audit does not exercise interactive flows:
- Click "Compare Selected" in `/browse` to see the dynamic compare launch
- Open the image viewer, rotate, screenshot
- Open the scratch pad, save, close
- Open feedback modal, submit
- Trigger htmx swaps (live counts, etc.)

Round 3 should add flow-level testing (click + assert state).

---

## Combined round 1 + round 2 priorities

Picking up from round 1's top-10 list, with round 2's findings folded in:

1. **Replace dense 8-column browse table** with card list on mobile, virtualised table on desktop
2. **Move/relocate the floating dock** — now confirmed broken on **every** page, including `/compare`, `/calendar`, `/browse`, deep soldier routes
3. **Add `<label for/id>` associations** to the 243+ inputs from round 1 (mechanical fix)
4. **Wrap non-`<button>` clickable elements** (round 1's 9 nested-interactive)
5. **Fix mobile header overflow** — 6px overflow on 390px viewport, fires on every route
6. **Fix `/compare` on mobile** — side-by-side table needs `tabindex="0"` or stacked-card refactor
7. **Replace `/browse` filter chip row** (9 inputs) with collapsible drawer
8. **Add pagination/virtualisation** to `/browse` (50 records = 4369px scroll)
9. **Reduce header pill density** on mobile (8 nav items wrap to 2 rows)
10. **Stats tiles inconsistent heights** on `/calendar`
11. **Investigate 5.7s load on `/soldiers` and `/soldiers/search/advanced`** — both fire the same recent-records hydrate fetch
12. **Carve out fragment endpoints from axe scans** (audit harness improvement)

---

## Files

```
audit/
├── run.mjs                     # round 1 harness
├── run-round2.mjs              # round 2 harness
├── reports/                    # round 1
│   ├── audit-v1.md
│   ├── summary.md
│   ├── routes.json
│   └── findings.json
├── reports-r2/                 # round 2
│   ├── audit-v2.md (this file)
│   ├── summary.md
│   ├── routes.json
│   └── findings.json
├── screenshots/                # round 1 PNGs
└── screenshots-r2/             # round 2 PNGs
```

Round 3 plan (next):
- Add fragment-detection to skip axe on non-document responses
- Add flow tests (click compare-selected, open image viewer, submit feedback)
- Test populated review queue + research collections
- Run against a richer seed (e.g. 250 soldiers, seed duplicates + conflicts)