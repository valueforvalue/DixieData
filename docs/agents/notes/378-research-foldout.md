# Slice Plan — Issue #378 (Research & Review foldout)

Continuation of slice 1 (commit `4b13118`) which shipped the picker shell
+ sticky cookie + 3 routes. Slices 2 and 3 remain.

This document defines the work **before** any code change. Each slice
ships as one reviewable commit; each slice has a RED-first regression net
that pins the slice's behavior before the feature ships. Slice 1's
7 RED-first tests in `internal/appshell/research_context_handlers_test.go`
already prove the picker shell's permission/auth/redirect gates.

## Locked decisions (from issue #378 Q&A section)

| # | Decision | Choice | Source |
|---|---|---|---|
| 1 | Recents storage | localStorage, key `dixiedata.research.recents`, cap 10 | Q1 resolved 2026-07-07 |
| 2 | Nav placement | Between Insights and Tags, immediately before Share | Q2 locked 2026-07-07 |
| 3 | Picker-routing scope | Only soldier-scoped entries route through picker; global Research Collections/Log/Review Queue keep their global routes | Q3 locked 2026-07-07 |
| 4 | No-go zones | No DnD, no modal, no scope creep on the Review Queue pill, no rewriting of sub-page UI | issue body §"Out of scope" |

## Foldout menu shape (from issue #378)

```
[Research & Review ▾]
├ Open Camaraderie         → /soldiers/{id}/camaraderie
├ Open Timeline            → /soldiers/{id}/timeline
├ Open Research Log        → /soldiers/{id}/research-log
├ Open Merge Review Ledger → /soldiers/{id}/conflict-ledger
├ Research Packs           → /soldiers/{id}/research-pack/{state|county}
├ Research Collections     → /research-collections
├ Change Person…           → /research
```

Each soldier-scoped link: if `dd_person_ctx` cookie is present, go direct;
otherwise route through `/research?next=<sub-route>` (picker redirects
through `/research/select` with the same `next`).

> Note: only one Research Packs entry (5 routes collapse to one menu
> item with two state/county variants — see Open Question A below).

---

## Slice 2 — Nav foldout button + htmx search + per-link picker-guard

### Files to touch

- `internal/templates/layout.templ`
  - Add `<a>`/button Research & Review inside the `<nav>` between
    "Insights" and the existing Share Foldout, modeled on the Share
    foldout's trigger attributes. Use `<details>` primitive OR the
    `components.Foldout` component (slice 1 ships picker, not the menu,
    so consult existing top-nav pattern).
  - First child: form to `/research/select` (Continue shortcut, uses
    cookie); second child: link to `/research` (Change Person…).
    Other children: 5 soldier-scoped links to the sub-pages; Research
    Collections link to `/research-collections`; Research Packs roll-up.

- `internal/appshell/soldiers_handlers.go` — `handleSoldierByID` is
  already the single chokepoint for all 6 sub-paths via the
  `switch parts[1]` chain. Original plan called for chi middleware,
  but `/soldiers/*` is registered as a chi catch-all (`r.Get("/soldiers/*", a.handleSoldierByID)`)
  and the 6 sub-paths are dispatched by content of `parts[1]`, so
  middleware can't filter by sub-path. Insert the picker guard at
  the top of the switch (one site, six branches covered):

  ```go
  // Issue #378 slice 2: redirect through the Person picker when
  // no dd_person_ctx cookie is set. Guarded sub-paths are the
  // 6 foldout entries from internal/templates/layout.templ;
  // /soldiers/{id} (detail) and /edit/resolve/flag/etc. are
  // intentionally ungated.
  pickerGated := map[string]string{
      "camaraderie":    "camaraderie",
      "timeline":       "timeline",
      "research-log":   "research-log",
      "conflict-ledger":"conflict-ledger",
      "research-pack":  "research-pack",  // picker sub-screen asks state vs county
  }
  if len(parts) > 1 {
      if next, ok := pickerGated[parts[1]]; ok && r.Method == http.MethodGet {
          if !a.pickerContextPresent(r) {
              http.Redirect(w, r, routebuilder.ResearchPicker() + "?next=" + url.QueryEscape(next), http.StatusSeeOther)
              return
          }
      }
  }
  ```

  ...with `pickerContextPresent` defined as:

  ```go
  func (a *App) pickerContextPresent(r *http.Request) bool {
      if a.personCtxKey == nil { return true } // no-key mode = pass-through
      _, ok := cookies.ReadPersonCtx(r, a.personCtxKey)
      return ok
  }
  ```

  **Deviation from initial D1 choice:** chi middleware doesn't apply
  because all 6 sub-paths share one chi route (`/soldiers/*`) and
  the sub-path intent lives inside `handleSoldierByID`'s `switch`.
  The new shape achieves the same coverage via **one insert site**
  (vs. D2's six), which keeps the contract greppable without
  middleware-level gymnastics on a shared handler.

- `internal/appshell/research_picker_handlers.go` — picker now reads
  `?next=` and forwards it as a hidden field on every result form
  instead of hardcoding `next="camaraderie"`. `handleResearchSelect`
  already validates `next` against the allowlist — no behavior change
  needed there except removing the hardcoded default.

- `internal/appshell/research_picker_handlers.go` — new htmx handler
  `handleResearchSearch` returns just the results fragment
  (`#panel.research.picker.results`) for `GET /research?q=foo&partial=1`.

- `internal/appshell/routes.go` — register `GET /research?q=&partial=1`
  → `handleResearchSearch` (or alternatively, route existing
  `handleResearchPicker` via an in-handler branch — see Open Question B).

- `internal/templates/research_picker.templ` — wire the search input
  with `hx-get` + `hx-trigger="keyup changed delay:300ms, search"` +
  `hx-target={ uiids.PanelResearchPickerResults }`. Forms now
  forward `?next=`.

- `internal/appshell/research_context_handlers_test.go` — add new tests:
  - `TestPickerSearchPartialFragment` — `GET /research?q=foo&partial=1`
    returns only the results panel (no full layout).
  - `TestPickerForwardsNextFromQuery` — `GET /research?next=timeline`
    renders the results with `next="timeline"` hidden fields.
  - `TestSubRouteRedirectsThroughPicker` — `GET /soldiers/411/timeline`
    with no cookie responds 303 → `/research?next=timeline`.
  - `TestSubRouteLoadsDirectWithCookie` — `GET /soldiers/411/timeline`
    with cookie returns 200 (existing behavior preserved).
  - `TestPickerRejectsUnknownNextInFlow` — picker forwards
    `?next=../../etc/passwd` as a hidden field but `handleResearchSelect`
    still 400s on submit (gate unchanged).
  - `TestResearchSelectHonorsForwardedNext` — picker submits
    `next=timeline`, handler redirects to `/soldiers/411/timeline`
    (not hardcoded `/camaraderie`).

### Success criteria

- Top-nav shows "Research & Review ▾" between Insights and Share. Click
  opens a foldout menu with 6 entries.
- All 5 soldier-scoped entries route through picker when no
  `dd_person_ctx` cookie; go direct when cookie present.
- Research Collections entry goes direct to `/research-collections`
  (no picker gating, per Q3 lock).
- Search input updates the results region live via htmx 300ms after
  the user stops typing; empty query collapses to "type to search"
  placeholder.
- Picker form preserves `?next=` across search (so picker re-submit
  after search lands on the right sub-page).
- Net new handler: 1 (`handleResearchSearch` with `?partial=1`).
- Net new helper: 1 (`pickerContextPresent`).
- Net new wiring: 1 insertion site in `handleSoldierByID`'s switch
  (covers 6 sub-paths via in-handler allowlist map, not 6 chi routes).

### Regression net

- Existing slice-1 tests (7) all green.
- All 6 soldier-scoped handler tests green (no behavior change when
  cookie present).
- `audit/discover_orphan_handlers.mjs` shows no new orphans.
- `make test` + `make tpl` clean.
- Optional: `audit/smoke.mjs` adds one assertion that
  `page.goto('/soldiers/411/timeline')` with no cookie lands on
  `/research?next=timeline`, with cookie lands on
  `/soldiers/411/timeline`. Defer to slice 3 if the slice-2 budget
  is tight.

### CHANGELOG

Single `### Added` bullet under `[Unreleased]`: "Research & Review
top-nav foldout routes soldier-scoped sub-pages through the Person
picker when no Person is in context." Mark slice 3's localStorage
recents as the second bullet in the same release commit, or land
slice 3 separately. (Per AGENTS.md, "every user-visible change gets
a bullet in the same commit that lands the change" — slice 2 +
slice 3 will each have their own `### Added` bullets, separate
commits, separate CHANGELOG touches.)

---

## Slice 3 — localStorage-backed Recent list + smoke probe + CHANGELOG tail

### Files to touch

- `frontend/app.js` — small module that:
  - On every form submit that POSTs to `/research/select` with a
    `person_id`, push `{id, name, display_id}` into
    `localStorage['dixiedata.research.recents']` (cap 10, dedupe
    by id, push to head).
  - On `/research` page load, read localStorage; if non-empty, swap
    `#panel.research.picker.recent` with the populated list (call
    the new fragment endpoint — see files below).
  - Falls back to current "empty" state silently if localStorage
    is unavailable (e.g. Wails CSP blocks storage in some cases —
    ship defensive, do not throw).

- `internal/templates/research_picker.templ` — already renders
  `#panel.research.picker.recent` (slice 1 placeholder); no
  template change needed if slice 3 hydrates from JS only. **Open
  Question C:** server-render vs JS-only? Recommend JS-only (avoids
  duplicate logic across SSR + browser paths; matches how slice 0
  Cookie helper works because cookie IS HTTP-only while localStorage
  is client-only — different transport).

- `internal/appshell/research_picker_handlers.go` — new
  `handleResearchRecent` returns the recent-list fragment for
  `GET /research/recent?ids=1,2,3`. Loops over a stored-id list and
  calls `a.soldiers.GetByID` for each. Cache-friendly; matches the
  UIIDs panel by ID.

- `internal/appshell/routes.go` — register `GET /research/recent`
  → `handleResearchRecent`. Also `GET /research?q=&partial=1` if not
  already in slice 2 (likely yes).

- `audit/smoke_research_picker.mjs` — new smoke file. Steps:
  1. Boot server, seed a Person Record (slice 1 helper exists).
  2. Goto `/research`; assert PageResearchPicker visible; assert
     PanelResearchPickerRecent shows "No recent picks yet."
  3. Type "Gil" in search input; wait 400ms; assert
     PanelResearchPickerResults swaps to one result.
  4. Click "Open"; assert redirects to `/soldiers/411/camaraderie`;
     assert dd_person_ctx cookie was set.
  5. Goto `/research` again (cookie still set); assert
     PanelResearchPickerContinue shows "Continue: …"; assert
     Recent list now shows that Person (because JS populated
     localStorage from the previous click).
  6. With cookie set, goto `/soldiers/411/timeline`; assert direct
     navigation (no picker gate).
  7. Click "Change Person" (formaction `/research/clear`); assert
     cookie cleared; goto `/soldiers/411/timeline`; assert now
     redirects through `/research?next=timeline`.

- `CHANGELOG.md` `[Unreleased]` → `### Added` bullet under slice 3's
  header: "Research picker Recent list persists across sessions in
  localStorage, capped at 10 entries."

### Success criteria

- Recent list shows picks from localStorage; picks persist across
  page reloads; cap 10 enforced; dedupe by person_id enforced.
- Smoke probe (7 steps above) green in CI.
- No new orphans.

### Regression net

- All slice-1 + slice-2 tests green.
- `make test` clean.
- `make audit` clean (smoke probe added).

---

## Out-of-bounds (do not ship here, defer to other issues)

- Issue #379 — quick-link tile on Person Record
- Auditor pass on other "Advanced X" patterns
- The "change via picker prompt if user already picked then
  re-departed on /soldiers/{id}" UX
- Modifier for non-soldier-scoped routes (Research Collections,
  Research Log) — they stay global per Q3 lock

## Open Questions for Maintainer (before slice 2 commit)

### A. Research Packs: one menu item or two?

The 5 foldout links include "Research Packs" which has two
sub-routes (`state` and `county`). Two options:

| Option | Shape |
|---|---|
| A1 (Recommended): single "Research Packs" link → picker first, then either state or county via a small picker sub-screen | One menu item; picker adds a State vs County radio on `/research?next=research-pack` |
| A2: two separate menu items "Research Packs (State)" + "Research Packs (County)" | Two entries; no picker sub-screen |

Recommend **A1**. Three reasons: (1) the picker is the right place
to ask "which geography?" because the answer is shared with the
picker UI; (2) less nav clutter; (3) the issue body lists "Research
Packs" as one entry, not two.

→ If you choose A2, slice 2 still ships but adds one extra menu
item and the picker sub-screen drops.

### B. htmx search: separate endpoint or in-handler branch?

| Option | Shape |
|---|---|
| B1: `handleResearchPicker` reads `?partial=1` and returns either full layout OR just the results panel from the same handler | One handler, one route |
| B2: separate `handleResearchSearch` registered at `GET /research?q=...&partial=1` only | Two handlers, two routes (but cleaner separation) |

Recommend **B2**. The picker handler is already complex (cookie +
search + results + recents placeholder + continue); splitting the
fragment-only path keeps both small and easy to test in isolation.

### C. Recent list: server-render or JS-hydrate only?

| Option | Shape |
|---|---|
| C1 (Recommended): browser reads localStorage on each `/research` load, calls `GET /research/recent?ids=…` fragment endpoint, JS swaps `#panel.research.picker.recent` | Server stays cookie-only (stateless); client owns the recents |
| C2: server reads localStorage via a cookie (not possible — localStorage != HTTP cookie) | N/A |
| C3: server reads the recents from a per-user DB table (overkill, deferred per Q1 answer) | N/A |

Recommend **C1**. Mirrors the existing pattern where server-side
data (cookie) and client-side data (localStorage) each own their
own transport and don't try to unify.

### D. Per-link guard pattern

Should the picker-redirect guard live as a chi middleware on the
6 sub-routes, or inline at the top of each handler?

| Option | Shape |
|---|---|
| D1 (Recommended): chi middleware applied via `r.With(guardMiddleware).Group(...)` or per-route `r.Get(pattern, a.guardMiddleware(a.handleX))` | Cleaner; one place to read; matches how `middleware.Recoverer` and `middleware.RequestID` are stacked today |
| D2: inline `if a.redirectThroughPickerIfNoContext(w, r) { return }` at the top of each handler | Explicit; easy to grep |

Recommend **D1**. Two reasons: (1) all 6 routes share the guard
contract — middleware captures that; (2) the inline variant
duplicates the early-return pattern across 6 sites, which is the
exact "shotgun surgery" footgun the slice plan is supposed to
prevent.

---

## Slice-by-slice commit shape (predictions, will be confirmed at ship)

| Slice | Commit subject | Files touched (est.) | Ins (est.) |
|---|---|---|---|
| 2 | `feat(nav): Research & Review top-nav foldout + per-link picker-guard (issue #378 slice 2, tracer bullet)` | layout.templ, research_picker_handlers.go, app.go, routes.go, routebuilder.go, research_context_handlers_test.go | ~280 |
| 3 | `feat(picker): localStorage-backed Recent list + smoke probe (issue #378 slice 3)` | frontend/app.js, research_picker_handlers.go, routes.go, audit/smoke_research_picker.mjs, CHANGELOG.md | ~260 |

## Plan deviations during slice 2 implementation (archaeology)

Two material deviations from the locked decisions, surfaced during the RED-first implementation pass on 2026-07-07. Both improve the plan; both are documented here so a future reviewer doesn't think the plan was wrong.

### Deviation 1 — D1 (chi middleware on 6 routes) collapsed to one in-handler insert site

The plan chose D1: chi middleware on the 6 soldier-scoped routes. The
actual code path is `r.Get("/soldiers/*", a.handleSoldierByID)` —
chi registers `/soldiers/*` as a single catch-all. The 6 sub-paths
(`/camaraderie`, `/timeline`, etc.) are dispatched by content of
`parts[1]` inside `handleSoldierByID`'s switch — chi cannot filter by
sub-path because there is no sub-path to filter on.

Adapted shape: the picker-guard is a single insertion at the top of
the switch, gated by an in-handler allowlist map:

```go
pickerGated := map[string]string{
    "camaraderie":     "camaraderie",
    "timeline":        "timeline",
    "research-log":    "research-log",
    "conflict-ledger": "conflict-ledger",
    "research-pack":   "research-pack",
}
if len(parts) > 1 && r.Method == http.MethodGet {
    if next, ok := pickerGated[parts[1]]; ok {
        if !a.pickerContextPresent(r) {
            redirect := routebuilder.ResearchPicker() + "?next=" + url.QueryEscape(next)
            http.Redirect(w, r, redirect, http.StatusSeeOther)
            return
        }
    }
}
```

Coverage is **equal** to the D1 plan (same 6 sub-paths gated) at **one**
site rather than six middleware registrations. The trade-off: the
guard sits in handler code instead of router config, so a
`grep middleware` won't surface it. Mitigated by the explicit
`// Issue #378 slice 2:` comment on the insert + the 2 RED tests
(`TestSubRouteRedirectsThroughPickerNoCookie` +
`TestSubRouteLoadsDirectWithCookie`) that pin both branches.

### Deviation 2 — B2 (separate `handleResearchSearch` route) collapsed to in-handler `?partial=1` branch

The plan chose B2: a separate `GET /research?q=...&partial=1` route
registered against a fresh `handleResearchSearch` handler. The
canonical Go-chi router matches by path, not by query string, so
`/research` and `/research?q=foo&partial=1` resolve to the same
chi route. A separate handler would require a non-canonical URL
(`/research/search`, `/research/lookup`) which carries UX cost and
splits the picker surface across two URLs.

Adapted shape: the picker handler branches on `r.URL.Query().Get("partial") == "1"`
and returns just the `#panel.research.picker.results` panel via the
existing `/research` URL when the flag is set. The `routebuilder.ResearchSearch()`
helper exists and returns the canonical URL string (`"/research?partial=1"`) so
the templ references stay typed — but the *handler* is the existing
`handleResearchPicker`. No second route registration.

Trade-off: `handleResearchPicker` is now slightly larger (cookie +
continue + search-shell + partial-branch). It's still under 100
lines and reads as four numbered blocks. The deviation saves a route
registration + a goquery probe-line ("orphan handlers") at the cost
of one extra `if partial { return }` branch.
