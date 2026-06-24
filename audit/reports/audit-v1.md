# DixieData UI/UX Audit — Round 1

**Date:** 2026-06-24
**Server:** `cmd/dixiedata-web` (HEAD = `0fe92ed`)
**Seed:** 50 soldiers, 100 records, 94 images into `.scratch/webmode`
**Tooling:** Playwright (chromium-headless-shell) + `@axe-core/playwright` + custom DOM heuristics
**Scope:** 13 unique routes × 3 viewports (desktop 1280×800, tablet 900×1200, mobile 390×844)
**Raw data:** `audit/reports/{findings,routes}.json`, `audit/screenshots/*`

---

## Executive summary

| Severity | Count | Notes |
|---|---:|---|
| Critical (axe) | 282 | Form labels, select names |
| Serious (axe) | 9 | Nested interactive controls |
| High (visual) | 28 | Mobile overflow, scrollbars |
| Medium (visual) | 22 | Unlabeled inputs, small tap targets |
| Low / info | 52 | Dead onclick, truncated titles, nav density |

**The single biggest takeaway:** the desktop layout is a long table with 8 narrow columns; the mobile layout is a single column with a giant header that nearly fills the viewport. Both extremes ignore middle ground, and the floating dock (Scratch Pad / Feedback / Menu) collides with main content on every page.

**Top 10 actionable redesign priorities:**

1. **Replace the dense 8-column browse table** with a card list on mobile and a virtualised table on desktop. Names should display "First Last (Middle)" not "First Last Middle X".
2. **Move/relocate the floating dock** so it never overlays content. Either pin to bottom-edge full-width, or hide non-essential actions behind a single "Menu" button.
3. **Add `<label>` association** to the 243+ inputs axe flagged. Most have id but no `for`. Mechanical fix, but pervasive across browse, search, soldier forms.
4. **Wrap non-`<button>` clickable elements** (the 9 nested-interactive issues on `/soldiers/new`) — likely `<details>` containing buttons.
5. **Fix the mobile calendar header overflow** — `top-shell` is 382px wide on a 390px viewport (6px overflow). Add proper breakpoint at `<420px`.
6. **Replace `Browse Local Archive` heading + filter chip layout** with a more intuitive query builder. The 8 filter inputs on one row is hard to scan.
7. **Add visible pagination / virtualisation** to the browse page (current: 50 records = 4369px scroll on desktop).
8. **Reduce header pill density on mobile** — 7 nav pills + 1 CTA = 8 elements wrapping to 2 rows. Use a hamburger menu below 768px.
9. **Fix the "I have fought a good fight" quote card overlap** with the floating dock on `/calendar` (mobile).
10. **Stats tiles inconsistent heights** — "0 SPOUSE RECORDS" taller than "50 SOLDIERS" because "RECORDS" wraps differently. Use grid with `align-items: start` or equal-height rows.

---

## A11y findings (axe-core)

### `label` — 243 occurrences, critical

**What:** Form elements must have labels.

**Pattern:** `<input id="...">` exists but `<label for="...">` is missing, and no `aria-label` / `aria-labelledby` / wrapping label / `placeholder` to compensate.

**Sample targets:**
- `input[data-browse-label="DXD-00001"]` (browse table)
- `select[name="scope"]` (browse filters)
- `input[name="q"]` (search)
- `input[name="display_id"]`, `select[name="entry_type"]`, `input[name="first_name"]` (soldier form)

**Fix:** Add `for=...` attributes to existing `<label>` elements, OR add `aria-label` to inputs that don't have visible labels (like row-select checkboxes).

### `select-name` — 39 occurrences, critical

**What:** Select element must have an accessible name.

**Pattern:** `<select name="scope">` etc. with no label/aria-label.

**Same fix as above.** Note this was partially addressed in commit `d1b31d6 feat(a11y): associate form labels with inputs via for/id pairs (#51)` but only on certain routes — browse and search remain.

### `nested-interactive` — 9 occurrences, serious

**What:** Interactive controls must not be nested.

**Sample target:** `details > .items-start` on `/soldiers/new` and `/soldiers/{id}/edit`.

**Fix:** A `<details>` element wraps `<a>` or `<button>` children. Either use `<summary>` only and put the button outside, or convert to non-semantic div with click handler.

---

## Visual findings (custom DOM heuristics)

### `overflow-x` / `h-scroll` — 15 + 13 occurrences, high

**Where:** All routes on mobile (390×844).

**Root cause:** The `top-shell` header containing 8 nav pills + brand title is 382px wide on a 390px screen → 6px overflow → horizontal scrollbar appears on every page.

**Fix:** Add a CSS breakpoint below ~420px that:
- Collapses the nav into a hamburger
- Or reduces pill padding
- Or hides the brand subtitle

### `unlabeled-input` — 21 occurrences, medium

Mirrors axe `label` findings (different detector: my script looks at form inputs without wrapping label or aria attributes, while axe checks for programmatic association via `for`). 21 of these are *not* caught by axe — likely inputs with `placeholder` but no other label. Adds 21 inputs to fix.

### `small-tap-target` — 1 occurrence, medium

**Where:** Browse row links on desktop. Soldier name links are 16px tall (well under 24px WCAG minimum).

**Sample:** `<a>Jasper Gray Turner A</a>` at 142×16px.

**Fix:** Increase row height to ≥44px on mobile, ≥32px on desktop.

### `nav-density` — 39 occurrences, info

**What:** Captures avg width per nav item.

**Stats:**
- Desktop: 8 items, 1140px nav width → ~142px per item. Reasonable.
- Tablet: 8 items, 768px nav width → ~96px per item. Pills wrap to 2 rows.
- Mobile: 8 items, 358px nav width → ~45px per item. Pills are cramped, wrapping causes 2 rows.

**Recommendation:** Below 768px, replace the nav with a single "Menu" button that opens a drawer/sheet containing the same links.

---

## Per-route notes

### `/calendar` — calendar grid view
- **Mobile only:** Quote card collides with floating dock (Scratch Pad / Feedback / Menu overlay the text "I have fought a good fight, I have finished my course...").
- **Stats tile row:** "0 SPOUSE RECORDS" stacks alone because "RECORDS" wraps differently — wasted vertical space. Use 2-col grid on mobile instead of 3-col-with-stacked-third.
- **Header overflow** (same root cause as other routes).

### `/soldiers` — search page
- **23 unlabeled inputs** (search box, filter dropdowns, recent search hydrate state).
- Loads slowly (5.7s vs <1s for other routes) — likely the recent-records hydration fetch. Investigate.

### `/browse` — record list
- **Worst offender** for a11y (243 label + 39 select-name instances across the table).
- **Table is 8 columns wide** with cramped headers — "DISPLAY ID", "ENTRY TYPE", "RANK OUT" all wrap to 2-3 lines.
- **50 rows on one page** = 4369px scroll. Add pagination or virtualisation.
- **Filter chip row** uses 8 inputs (Scope, Sort, Page Size, Entry Type, Pension State, Review Status, Unit, Buried In, Confederate Home Status = 9 actually). Hard to scan.

### `/soldiers/new` and `/soldiers/{id}/edit`
- **9 nested-interactive violations** — `<details>` containing buttons.
- Likely the "Add Another..." / section toggle patterns. Verify and refactor.

### `/insights`, `/settings`, `/share`, `/review-queue`
- Each shows 1 axe violation type — likely a single unlabeled select or input.
- Visual issues are mostly mobile overflow + nav density (consistent with all routes).

### `/soldiers/{id}` — detail view
- Clean (0 a11y, 1 visual). The detail page is the most usable screen.

---

## Recommended next steps

### Quick wins (1–2 days)
- Fix all `label` / `select-name` issues mechanically (add `for` to existing `<label>`s). ~280 fixes.
- Hide floating dock on tablet/mobile, or relocate to a bottom-bar style.
- Add `<meta viewport>` responsive breakpoints below 420px.

### Medium (1 week)
- Redesign `/browse` table — card list on mobile, virtualised table on desktop, or split into "compact / comfortable" view toggle (the app already has `layoutMode` for this!).
- Replace `<details>` + nested button patterns on soldier form.
- Fix quote card / floating dock collision on calendar.

### Large (2+ weeks)
- Full nav redesign: hamburger below 768px, persistent sidebar 768–1200px, top bar >1200px.
- Replace 8-filter browse chip row with a saved-views / query-string builder.
- Audit colors for AA contrast (especially `#eddca6` on `#22303d` and `#a88a46` on `#1f2b38`).
- Refactor `app.js` to ship ESM modules (currently a single 138KB IIFE — hurts caching and grep-ability).

---

## Files in this audit

```
audit/
├── package.json
├── run.mjs                            # main harness
├── reports/
│   ├── summary.md                     # machine-generated overview
│   ├── routes.json                    # per-route axe + visual report
│   ├── findings.json                  # flat finding list (input for triage)
│   └── audit-v1.md                    # this document
└── screenshots/                       # 39 PNGs, 3 viewports × 13 routes
```

## Reproducing

```bash
# 1. Build web-mode server
go build -o build/bin/dixiedata-web.exe ./cmd/dixiedata-web

# 2. Seed data
build/bin/seed-data.exe -data-dir .scratch/webmode -soldiers 50 -reset

# 3. Boot server (background)
build/bin/dixiedata-web.exe -scratch-dir .scratch/webmode -addr 127.0.0.1:8765 &

# 4. Audit
node audit/run.mjs
```