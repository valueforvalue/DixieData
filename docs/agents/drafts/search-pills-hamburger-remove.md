# Refactor plan: clean up search-result pill layout + remove mobile hamburger drawer

## Problem Statement

The `/soldiers` Search/Quick View page renders each highlighted result
inside a `SoldierCard` whose highlighted branch appends a horizontal row
of three pill-shaped badges (entry-type / death-date / burial-place):

```templ
if highlighted {
    <div class="mt-3 flex flex-wrap gap-2 text-xs">
        <span class="rounded-full ..."> { entryBadgeLabel(s) } </span>
        if s.DeathDate != "" { <span ...>Death: ...</span> }
        if s.BuriedIn  != "" { <span ...>Buried In: ...</span> }
    </div>
}
```

The badges are visually noisy, redundant (entry-type and burial are
already on the detail page), and they crowd the quick-view row into a
"pill horizontal layout that is absolutely non-functional" (user
feedback, 2026-06-25). The non-highlighted `SoldierCard` and the
`Needs Review` pill row are out of scope and stay as-is.

Separately, this is a desktop Wails app, not a mobile site. The
sub-768px hamburger drawer (`data-top-nav-drawer`) was added in
commit `f75e73f` for "issue #82" but the app is windowed and
non-responsive at the OS level — the drawer is dead UI that never
fires. The CSS that only matters at sub-780px viewports (the
`@media (max-width: 780px)` block in `frontend/tailwind.css`) should
be removed along with the drawer.

The split-screen / windowed-resize breakpoints stay — those are by
design for a 16" monitor in split-screen mode, not mobile UI:
- `@media (max-width: 1040px)` block (tightens top-nav padding when
  the window is narrow).
- `@media (max-width: 1100px) .calendar-layout` rule (collapses
  calendar two-column to single-column).
- `@media (max-width: 900px) .responsive-two-col` rule (collapses
  detail/edit two-column to single-column).
- `md:hidden` / `md:flex` utility classes inside content templates
  (`browse.templ`, `review_queue.templ`) — those are card-vs-table
  toggles at split-screen widths, not mobile UI.

## Solution

Two coupled UI cleanups in one focused PR:

1. **Search results**: drop the highlighted-branch pill row in
   `SoldierCard`. Replace with a tiny `dl` line of plain text
   ("Soldier · d. 1864-04-12 · Buried in Memphis") that only renders
   when at least one of those fields is non-empty.
2. **Mobile hamburger drawer**: remove the `data-top-nav-toggle`
   button + `#top-nav-drawer` div from `layout.templ`, drop the
   `initializeTopNav` JS handler, and delete the
   `@media (max-width: 780px)` block in `frontend/tailwind.css` that
   shrunk the top-nav and floating-dock for sub-768px viewports.
   The bottom floating-dock "Menu" panel stays — it's the
   always-visible on-demand quick-nav.

The top-nav inline pill row (Calendar / Search/Quick View / Browse /
Review Queue / Insights / Share / Settings / Add Person Record) at
desktop / split-screen widths stays exactly as it is — user
explicitly confirmed it is "as it should be."

## Commits

Order matters. Each commit leaves the app in a working state and
passes `go test -short ./...`.

1. **Add a failing test that asserts the highlighted pill row is
   gone.** Edit `internal/templates/soldier_card_test.go` — add a
   new `TestSoldierCardHighlightedPlainMeta` that renders
   `SoldierCard(view, true)` with `EntryType="soldier"`, `DeathDate`,
   `BuriedIn` populated, then asserts the rendered output contains
   `Soldier`, `d.`, `Buried in` as plain text inside a `dl`, and
   asserts `rounded-full border-[rgba(141,116,64,0.55)]` is **not**
   present. Run `go test ./internal/templates/...` — confirm it
   fails on the highlighted-branch pill markup. Commit "test:
   assert SoldierCard highlighted branch renders plain meta, not
   pills."

2. **Swap the highlighted pill row for a plain `<dl>`.** Edit
   `internal/templates/soldier_card.templ`: replace the
   `if highlighted { <div class="mt-3 flex flex-wrap gap-2 text-xs">
   ...rounded-full spans... </div> }` block with a guarded `<dl
   class="mt-2 text-xs text-slate-500">` whose `<dt>/<dd>` entries
   are "Entry Type", "Death", "Burial" and render only when the
   matching field is non-empty. Use existing helpers
   (`entryBadgeLabel`, `deathDate`, `emptyDetail`). Run `make tpl`
   to regenerate `soldier_card_templ.go` and rerun tests — the new
   test passes, existing ones still pass. Commit "fix(ui): replace
   highlighted SoldierCard pills with plain definition list."

3. **Add a failing layout test that asserts the hamburger drawer is
   gone.** Edit `internal/templates/layout_test.go`: extend the
   needle list to assert `data-top-nav-toggle` and
   `data-top-nav-drawer` are **not** present in the rendered layout.
   Run `go test ./internal/templates/...` — confirm it fails.
   Commit "test: assert layout renders no mobile hamburger drawer."

4. **Strip the hamburger drawer markup from `layout.templ`.** Remove
   the `<button ... data-top-nav-toggle ... class="secondary-button
   md:hidden">Menu</button>` inside the header, and delete the
   entire `<div id="top-nav-drawer" data-top-nav-drawer ...>...</div>`
   block that contains the stacked pill-link drawer nav. Also drop
   the `md:flex` class on the inline `<nav>` so it always renders.
   Run `make tpl` to regenerate `layout_templ.go`. Commit
   "fix(ui): drop top-nav hamburger drawer markup."

5. **Drop `initializeTopNav` from `frontend/app.js`.** Delete the
   function (currently around lines 2282–2346) and its call inside
   the `DOMContentLoaded` handler (around line 3369). Confirm the
   file still parses (build the frontend assets via `npm run
   build:css` and `make css`). Commit "refactor(web): remove
   initializeTopNav handler now that the drawer is gone."

6. **Strip mobile-only CSS from `frontend/tailwind.css`.** Delete
   the `@media (max-width: 780px)` block (shrinks nav link padding
   to 0.36rem / 0.66rem and floating-dock buttons to 0.7rem — pure
   mobile compression that existed only to support the hamburger
   drawer at sub-768px viewports). Keep the
   `@media (max-width: 1040px)` block (split-screen nav tightening
   at 16" monitor widths, by design), the
   `@media (max-width: 1100px) .calendar-layout` rule, and the
   `@media (max-width: 900px) .responsive-two-col` rule — all
   three are intentional split-screen / windowed-resize breakpoints,
   not mobile UI. Do **not** touch `md:hidden` / `md:flex`
   utilities in `browse.templ`, `review_queue.templ`, or any other
   content template — those are split-screen card-vs-table toggles
   that stay. Run `npm run build:css` to regenerate
   `frontend/app.css`. Commit "fix(ui): remove mobile-only sub-780px
   breakpoint CSS for the dropped hamburger drawer."

7. **Re-run the audit harness to capture the search-with-results
   screenshot.** The audit fixtures don't currently exercise search
   with results (the search page screenshot shows the empty state),
   so a `desktop_search_results.png` capture is needed. Update
   `audit/run.mjs` to add a `seed` step that creates one Person
   Record and a `desktop_search_results` capture that types a query
   matching that record. Run `npm run audit`, confirm no new
   findings in the search results layout. Commit "audit: capture
   desktop_search_results fixture for the no-pill regression check."

8. **Update `CHANGELOG.md` [Unreleased] block** with a `### Fixed`
   entry: "Search results no longer render the highlighted
   `SoldierCard` pill row (entry-type/death-date/burial-place); the
   same data now appears as a plain definition list inside the card.
   Mobile hamburger drawer removed (Wails desktop app)." Commit
   "docs: changelog entry for the search-pill + hamburger removal."

## Decision Document

- **Modules modified**:
  - `internal/templates/soldier_card.templ` — highlighted branch
    markup change.
  - `internal/templates/layout.templ` — drop drawer markup +
    `md:flex` on the inline nav.
  - `frontend/app.js` — drop `initializeTopNav` and its caller.
  - `frontend/tailwind.css` + `frontend/app.css` — drop the
    `@media (max-width: 780px)` block.
  - `audit/run.mjs` — add `desktop_search_results` capture.
- **Public interfaces**: unchanged. No new routes, no new request
  params, no DB changes.
- **CSS class changes**: `pill-link` is reused for the always-on
  top-nav and floating-dock-menu nav — keep. `md:hidden`/`md:flex`
  on the top-nav drawer are removed; the same utilities in content
  templates stay.
- **JS contracts**: `initializeTopNav` removed. No event listeners
  on `data-top-nav-toggle` / `data-top-nav-drawer` remain in the
  codebase after step 5 (grep-verified).
- **Schema**: unchanged.
- **API**: unchanged.
- **Test contracts**: existing `TestLayoutUsesLocalBootstrapScript`
  needles will need to be edited — the test asserts on the literal
  string `class="pill-link top-nav-link"` (still present on the inline
  nav, so survives) and on `md:hidden` / `md:flex` substrings
  (currently asserted via the absence of "responsive foundation
  control" complaints). The new negative-assertions in step 3
  supersede those. After step 6 the test must pass with the new
  class strings.

## Testing Decisions

- **Good test = external behavior**: we assert on rendered HTML text
  and class strings, not on internal template-component structure.
- **Modules tested**:
  - `internal/templates` — `TestSoldierCardHighlightedPlainMeta`
    (new), updated `TestLayoutUsesLocalBootstrapScript` (negative
    assertions for hamburger markup).
  - `audit/run.mjs` — `desktop_search_results` fixture.
- **Prior art**: the existing tests use `strings.Contains` on
  rendered output to verify class strings and visible text. Follow
  the same pattern.
- **No new browser-side unit tests needed**: removing the drawer is
  tested by absence of the markup selectors. The audit harness
  remains the integration test.

## Out of Scope

- Changing the **top-nav inline** pills (Calendar / Search/Quick
  View / Browse / Review Queue / Insights / Share / Settings /
  Add Person Record) at any viewport — user confirmed they are
  correct.
- Changing the **floating-dock** bottom bar or its menu panel —
  it's the always-visible quick-nav, not a mobile-only thing.
- Changing the `Needs Review` pill row inside `SoldierCard` — user
  confirmed it is fine.
- Re-touching `SoldierCard` outside the highlighted branch.
- Re-styling the `SearchPreviewContent` quick-view panel — it has
  its own definition lists and works as-is.
- Split-screen / windowed-resize breakpoints (by design for 16"
  monitor):
  - `@media (max-width: 1040px)` block in `tailwind.css`.
  - `@media (max-width: 1100px) .calendar-layout` rule.
  - `@media (max-width: 900px) .responsive-two-col` rule.
  - `md:hidden` / `md:flex` utilities inside `browse.templ`,
    `review_queue.templ`, and other content templates — those are
    card-vs-table split-screen toggles that stay.
- The `attachArchiveCounts` helper in
  `internal/appshell/soldiers_handlers.go` — defined but never
  called (reverted in commit `7a7c17b`); separate concern.

## Further Notes

- `TestLayoutUsesLocalBootstrapScript` already loads
  `frontend/app.css` from disk and asserts on its compiled content;
  step 6 keeps that contract intact — none of the needle substrings
  match the deleted `@media (max-width: 780px)` block.
- After this PR, the `top-nav-link` class is still used by the
  inline nav (so the `top-nav-link` selector in `tailwind.css`
  stays).