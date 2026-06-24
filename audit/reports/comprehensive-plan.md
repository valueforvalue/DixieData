# DixieData UI/UX Redesign — Comprehensive Plan

**Parent issue:** #74 (UI/UX revamp)
**Source:** Round 1 audit (`audit/reports/audit-v1.md`) + Round 2 audit (`audit/reports-r2/audit-v2.md`)
**Audit data:** `audit/reports/{findings,routes}.json`, `audit/reports-r2/{findings,routes}.json`
**Screenshots:** `audit/screenshots/`, `audit/screenshots-r2/`

This plan is filed as 12 sub-issues under #74. Each is a self-contained, PR-sized slice. Recommended execution order at the bottom.

---

## Slice overview

| # | Title | Effort | Depends on | Files |
|---|---|---|---|---|
| 1 | Fix floating dock so it doesn't overlay content | S (1 day) | — | `internal/templates/layout.templ`, `frontend/app.css` |
| 2 | Mechanical `<label for/id>` pass on browse + search + soldier form | S (1 day) | — | `internal/templates/{browse,soldier_card,entry_form}.templ` |
| 3 | Add 420px mobile breakpoint to fix 6px header overflow | S (half day) | — | `internal/templates/layout.templ`, `frontend/app.css` |
| 4 | Replace dense browse table with card list (mobile) + sticky virtualised table (desktop) | L (1–2 wk) | 2, 7 | `internal/templates/browse.templ`, `internal/viewmodel/`, `frontend/app.js` |
| 5 | Replace browse filter chip row with collapsible filter drawer | M (3 days) | 2 | `internal/templates/browse.templ`, `frontend/app.js` |
| 6 | Add pagination/virtualisation to /browse | M (3 days) | 4 | `frontend/app.js` |
| 7 | Reduce header nav density — hamburger menu below 768px | M (3 days) | 3 | `internal/templates/layout.templ`, `frontend/app.js` |
| 8 | Fix /compare on mobile — `tabindex="0"` short-term, stacked cards long-term | M (3 days) | 4, 7 | `internal/templates/review_queue*.templ`, `frontend/app.js` |
| 9 | Wrap `<details>` interactive children (nested-interactive fix) | S (1 day) | — | `internal/templates/entry_form.templ`, `internal/templates/soldier_card.templ` |
| 10 | Investigate 5.7s /soldiers hydration latency | M (3 days) | — | `frontend/app.js`, `internal/appshell/*_handlers.go` |
| 11 | Audit harness: skip axe on HTMX fragments | S (half day) | — | `audit/run.mjs`, `audit/run-round2.mjs` |
| 12 | Round 3 audit: interactive flows + populated seed | M (1 wk) | 11 | `audit/run-round3.mjs`, `cmd/seed-data/` |

---

## Detailed slices

### Slice 1 — Fix floating dock so it doesn't overlay content

**Problem:** The floating dock (`Scratch Pad` / `Feedback` / `Menu`) is `position: fixed; bottom: 6; right: 6` with z-index 40. On every page it sits on top of main content:
- `/calendar` mobile: overlaps the quote card text
- `/compare` desktop: overlaps the right edge of the diff table
- `/soldiers/{id}` mobile: overlaps the second record card

**Audit evidence:** `audit/screenshots-r2/mobile_compare.png`, `audit/screenshots-r2/desktop_compare.png`, `audit/screenshots/mobile_home.png`

**Acceptance criteria:**
- Dock does not overlap any main content element on any route at any viewport
- All three actions remain reachable from every page
- `aria-live` status updates still work
- Existing tests pass (`make test`)

**Scope:**
- `internal/templates/layout.templ` (move dock position, possibly to bottom-bar layout)
- `frontend/app.css` (responsive rules)
- `frontend/app.js` (verify no click-target regressions)

**Effort:** S — should be a single PR.

---

### Slice 2 — Mechanical `<label for/id>` pass

**Problem:** 243 `<input>` elements are missing `for/id` associations. axe-core flags every one as critical. Most have an `id` already; just need a `<label for="...">` next to them. Some need `aria-label` instead (visual-only labels like row-select checkboxes).

**Audit evidence:** `audit/reports/findings.json` — 243 `label` violations; 39 `select-name` violations.

**Approach:**
- Script-driven pass: read templates, find every `<input id="X">` and adjacent text, add `<label for="X" class="sr-only">` if missing visible label, or `<label for="X">text</label>` if text exists nearby
- Manual review for ambiguous cases (placeholder-only labels, icon-only buttons)
- Add `aria-label` to icon-only buttons and decorative selects

**Acceptance criteria:**
- axe-core reports zero `label` and zero `select-name` violations on `/browse`, `/soldiers`, `/soldiers/{id}/edit`, `/soldiers/new`, `/settings`, `/share`
- Visual layout unchanged (no new visible labels)
- All existing tests pass

**Scope:**
- `internal/templates/browse.templ`
- `internal/templates/soldier_card.templ`
- `internal/templates/entry_form.templ`
- `internal/templates/settings*.templ`
- `internal/templates/share*.templ`

**Effort:** S — high-volume but mechanical.

---

### Slice 3 — Add 420px mobile breakpoint

**Problem:** `top-shell` header is 382px wide on a 390px viewport, causing 6px horizontal overflow. Every page is affected on mobile. Fires `h-scroll` and `overflow-x` findings 28 times in round 1.

**Audit evidence:** `audit/reports/findings.json` — `overflow-x` (15), `h-scroll` (13), `h-scroll-body` (13).

**Acceptance criteria:**
- No horizontal scrollbar at 390px, 375px, 360px viewports
- Header still fits 8 nav items at 768px+
- Existing `data-layout-mode` system still works (already covers the 1000px breakpoint)

**Scope:**
- `internal/templates/layout.templ` (add Tailwind responsive classes)
- `frontend/app.css` (breakpoint at `max-width: 420px`)

**Effort:** S — single PR, possibly a subset of slice 7.

---

### Slice 4 — Replace dense browse table with card list (mobile) + sticky virtualised table (desktop)

**Problem:** `/browse` shows 50 records in a single 8-column table. On desktop the table is 4369px tall. On mobile columns are clipped (column widths force horizontal scroll per row). The "ENTRY TYPE", "RANK OUT", "PENSION STATE" headers wrap to 2–3 lines.

**Audit evidence:** `audit/screenshots/desktop_browse.png` (4369px tall, cramped headers), mobile browse screenshots (clipped columns).

**Approach:**
- Two layouts switched by viewport / `data-density`:
  - **Mobile (< 768px):** card list, one record per card, key fields only (Display ID, Name, Unit, Pension State, Last Edited). Expandable for full record.
  - **Desktop (≥ 768px):** sticky-header virtualised table (only render visible rows). Use simple windowing or `IntersectionObserver`-based chunking.
- Align with existing `data-density="comfortable|compact"` switch from #74 Phase 2.

**Acceptance criteria:**
- Mobile: zero horizontal overflow, tap targets ≥ 44px tall
- Desktop: table header stays visible while scrolling, page does not scroll past the last row
- All 8 current columns available; users can switch to a "compact" density to see more at once
- Existing `/browse/results` HTMX flow still works

**Scope:**
- `internal/templates/browse.templ` (split into `browse_card.templ` + `browse_table.templ`)
- `internal/viewmodel/types.go` (add `BrowseRow` projection if not present)
- `frontend/app.js` (virtualised rendering for desktop)
- `frontend/app.css` (card styles)

**Effort:** L — 1–2 weeks. Largest UX change in the plan.

---

### Slice 5 — Replace browse filter chip row with collapsible filter drawer

**Problem:** `/browse` shows 9 filter inputs (Scope, Sort, Page Size, Entry Type, Pension State, Review Status, Unit, Buried In, Confederate Home Status) in a single grid before any data shows. On mobile the grid wraps awkwardly and consumes 300px of vertical space.

**Audit evidence:** `audit/screenshots/desktop_browse.png` (filter chip row before data).

**Approach:**
- Collapse filters behind a "Filters" button by default
- On click, slide-down / drawer panel reveals the 9 inputs grouped logically (Record type, Military, Status)
- Show active filter count as a badge on the button
- Persist drawer state in localStorage

**Acceptance criteria:**
- Default state shows filters collapsed (just the button)
- Active filter count badge visible
- All 9 current filters still work and appear in the URL query string
- Works with htmx-driven `browse/results` fragment swap

**Scope:**
- `internal/templates/browse.templ` (restructure filter section)
- `frontend/app.js` (drawer toggle + state persistence)

**Effort:** M — 3 days.

---

### Slice 6 — Add pagination/virtualisation to /browse

**Problem:** With 50 seeded records, the browse page is 4369px tall. In production with thousands of records this is unusable. No pagination control exists.

**Audit evidence:** `audit/screenshots/desktop_browse.png` — page height 4369px.

**Approach:**
- Pagination (simpler): add `?page=N&pageSize=M` query params, render page controls at top + bottom
- Virtualisation (better): only render rows visible in the viewport, fetch next chunk on scroll

**Acceptance criteria:**
- Page height stays bounded regardless of record count
- URL reflects current page for sharing/bookmarking
- Existing sort/filter still work across pages

**Scope:**
- `internal/appshell/browse_handlers.go` (pagination logic)
- `internal/templates/browse.templ` (page controls)
- `frontend/app.js` (if virtualisation chosen)

**Effort:** M — 3 days for pagination; virtualisation doubles that.

---

### Slice 7 — Reduce header nav density — hamburger menu below 768px

**Problem:** The top header has 7 nav pills + 1 "Add Person Record" CTA = 8 elements. At < 768px they wrap to 2 rows and crowd the screen. At < 420px they overflow horizontally.

**Audit evidence:** `audit/reports/findings.json` — `nav-density` fires 39 times.

**Approach:**
- Below 768px: replace inline nav with a single "Menu" hamburger button
- Tap/click opens a full-screen drawer with the same links
- "Add Person Record" CTA remains visible as a separate floating button
- Preserve existing `data-layout-mode` system

**Acceptance criteria:**
- Below 768px: header fits on one row, no wrapping
- All nav links remain reachable
- Drawer animation matches existing modal style (fade + slide)
- Keyboard accessible (Esc closes, focus trap)

**Scope:**
- `internal/templates/layout.templ` (conditional render + drawer markup)
- `frontend/app.js` (drawer open/close, focus trap)
- `frontend/app.css` (drawer styles)

**Effort:** M — 3 days.

---

### Slice 8 — Fix /compare on mobile — `tabindex="0"` short-term, stacked cards long-term

**Problem:** `/compare` shows a side-by-side diff table (DXD-00047 column | DXD-00035 column). On mobile (390px) the table is wider than the viewport, so the right column is hidden. The `.overflow-x-auto` container has no keyboard affordance — axe flags `scrollable-region-focusable`.

**Audit evidence:** `audit/screenshots-r2/mobile_compare.png` (right column invisible), `audit/reports-r2/findings.json` (`scrollable-region-focusable`).

**Approach (two-step):**
1. **Short-term:** add `tabindex="0"` to `.overflow-x-auto` + `role="region"` + `aria-label="Field comparison"` — keyboard users can scroll to see right column. Single-line fix.
2. **Long-term:** below 768px, render as stacked cards: each row becomes a card showing "Field: DXD-00047 value | DXD-00035 value" with visual diff highlight.

**Acceptance criteria:**
- Short-term: axe `scrollable-region-focusable` violation gone
- Long-term: mobile users see all values without horizontal scroll
- Long-term: differences still visually highlighted (color, weight)

**Scope:**
- `internal/templates/review_queue_compare.templ` (or wherever compare renders)
- `frontend/app.js` (if stacked-card mode needs JS)

**Effort:** M — 3 days for stacked-card variant.

---

### Slice 9 — Wrap `<details>` interactive children (nested-interactive fix)

**Problem:** 9 axe `nested-interactive` violations. Affected: `/soldiers/new` and `/soldiers/{id}/edit`. A `<details>` element wraps `<button>` children, which is invalid HTML and confuses screen readers.

**Audit evidence:** `audit/reports/findings.json` — `nested-interactive` (9), target `details > .items-start`.

**Approach:**
- Replace `<details>` with a plain `<div>` and a clickable `<button>` that toggles `hidden` class via JS
- Or use `<details>` correctly: only `<summary>` for toggle, content is plain markup
- Verify no visual regression

**Acceptance criteria:**
- axe reports zero `nested-interactive` violations on the affected routes
- Expand/collapse behaviour preserved
- Keyboard accessibility preserved (Enter/Space toggles)

**Scope:**
- `internal/templates/entry_form.templ`
- `internal/templates/soldier_card.templ`

**Effort:** S — single PR.

---

### Slice 10 — Investigate 5.7s /soldiers hydration latency

**Problem:** `/soldiers` and `/soldiers/search/advanced` both take ~5.7s to load. Other routes load in < 1s. Likely cause: the recent-records hydration fetch in `frontend/app.js::hydrateRecentSearchResults` is synchronous-blocking or N+1.

**Audit evidence:** `audit/reports/routes.json` — load_ms for `/soldiers` is 5761 (desktop), 5771 (tablet), 5745 (mobile).

**Approach:**
- Profile the page load (browser devtools waterfall)
- Identify the slow request (`/soldiers/search/recent`?)
- Optimise: debounce, fetch in parallel with render, or remove the recent-list fetch on first load

**Acceptance criteria:**
- `/soldiers` load time < 2s on desktop and tablet, < 3s on mobile
- Recent records still appear (within 500ms of paint)
- No layout shift regressions

**Scope:**
- `frontend/app.js` (`hydrateRecentSearchResults`, `quickSearchInput`)
- Possibly `internal/appshell/soldiers_handlers.go` if the backend is slow

**Effort:** M — 3 days, depending on root cause.

---

### Slice 11 — Audit harness: skip axe on HTMX fragments

**Problem:** Round 2 audit produced 9 false-positive a11y violations (`document-title`, `html-has-lang`) from running axe against HTMX fragment endpoints. These aren't real bugs but they pollute the findings list.

**Approach:**
- Detect fragment responses: HTML without `<html>` tag, OR with `Content-Type: text/html` but body-only
- Skip axe on those, keep visual checks
- Add an `isFragment` flag to the per-route report

**Acceptance criteria:**
- Round 2 re-run produces zero `document-title` / `html-has-lang` false positives
- Fragment endpoints still get visual checks
- Per-route report clearly indicates fragment vs full-page

**Scope:**
- `audit/run.mjs`
- `audit/run-round2.mjs` (or fold into a shared helper)

**Effort:** S — half day.

---

### Slice 12 — Round 3 audit: interactive flows + populated seed

**Problem:** Rounds 1 and 2 audited page loads only. No interactive flows were tested (click compare-selected, image viewer, scratch pad, feedback, htmx swaps). Some routes showed only empty states (review queue, research collections).

**Approach:**
- Add flow tests: click compare-selected in /browse, verify /compare loads; open image viewer, verify zoom controls; submit feedback modal; toggle htmx-driven live counts
- Use a richer seed (250 soldiers, force duplicates so review queue is populated)
- Add fragment-detection from slice 11
- Re-run with current slices 1, 2, 3, 9 applied to verify they fixed the reported findings

**Acceptance criteria:**
- Round 3 report covers 5+ interactive flows
- Round 3 verifies slices 1, 2, 3, 9 reduced axe violations
- New findings categorised: regression vs new

**Scope:**
- `audit/run-round3.mjs` (new harness)
- `cmd/seed-data/` (add `--with-conflicts` flag)

**Effort:** M — 1 week.

---

## Recommended execution order

### Sprint 1 (week 1) — quick wins

Run in parallel, no dependencies:
- Slice 2 (labels) — high-volume mechanical
- Slice 3 (420px breakpoint) — single CSS rule
- Slice 9 (nested-interactive) — small templ fix
- Slice 11 (harness fragment detection) — improves all future audits

### Sprint 2 (week 2) — floating dock + nav

Sequential, dock first because it's the highest-priority visual fix:
- Slice 1 (floating dock) — must be done before any redesign so screenshots aren't polluted
- Slice 7 (hamburger nav) — depends on slice 3's 420px breakpoint logic

### Sprint 3 (weeks 3–4) — browse redesign

The largest single feature; the four browse slices compose:
- Slice 5 (filter drawer) — unblocks slice 4's visual focus
- Slice 4 (card list / virtualised table) — the meat of the redesign
- Slice 6 (pagination/virtualisation) — pairs with slice 4
- Slice 8 (compare mobile) — pairs with slice 4's table work

### Sprint 4 (week 5) — performance + verification

- Slice 10 (hydration latency) — performance work
- Slice 12 (round 3 audit) — verifies slices 1–4 + 8 worked

Total: ~5 weeks for the 12 slices if executed sequentially with single-developer capacity.

---

## Out of scope (this plan)

- Color / contrast audit (separate pass, requires design tokens extraction)
- Refactoring `frontend/app.js` from single IIFE to ESM modules (architectural)
- Extracting inline CSS from `layout.templ` into `frontend/app.css` (design system foundation — separate #74 Phase 0)
- Settings page redesign (lower-priority, not in audit findings)
- Share page redesign (lower-priority, not in audit findings)
- Print / PDF export redesign (separate track)

These belong to future plans.