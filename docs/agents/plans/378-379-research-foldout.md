# Plan: Top-nav Research & Review foldout + Person picker (#378, #379)

## Decisions to confirm

- **Q1 — Sticky person context:** session cookie (signed value) vs URL query param (`?person=ID`) on every sub-link? Recommendation: **session cookie** (clean URLs, survives page refresh, scoped to current session). URL param stays as override.
- **Q2 — Recent-persons list storage:** localStorage (per-machine) vs server-side (per-user, since DixieData has no user accounts, "per-machine" is the same as "per-app-instance"). Recommendation: **localStorage** — matches the existing back-stack pattern, no DB schema, ephemeral by design.
- **Q3 — `/research` picker shape:** (a) bare picker (search + recents) only; (b) picker + global quick links to Insights / Review Queue / Tags. Recommendation: **(a) bare picker** — non-soldier-scoped surfaces already have top-nav pills. Adding more to the foldout panel defeats the purpose of the foldout.
- **Q4 — Foldout trigger label:** "Research & Review" (full) vs "Research" (short) vs "Tools" (catchall). Recommendation: **"Research & Review"** — matches the existing block's name and signals the soldier-scoped + review surfaces both.
- **Q5 — Tile position on soldier_card:** at the position of the old `<details>` block (just under summary dl) or higher (right next to name header). Recommendation: **at the old `<details>` position** — preserves visual continuity, the user doesn't lose their place.

## Slice 1 (tracer bullet — fully detailed)

**Scope:** Foldout trigger renders in top-nav; clicking it opens picker landing `/research` with a search box that filters Person Records by name/display_id (server-side, htmx-powered); selecting a person routes to `/research/camaraderie` (one of the soldier-scoped surfaces). Sticky person context stored in a signed session cookie. Quick-link tile replaces `<details>` block on soldier_card. ONE apply site.

### Files

- `internal/templates/layout.templ` — add second `Foldout` call alongside the existing Share one. Trigger label "Research & Review", panel ID `uiids.LayoutResearchMenu` (new UIID).
- `internal/templates/components/foldout.templ` — no changes; primitive already supports multi-instance via `menuID`.
- `internal/uiids/uiids.go` — add `LayoutResearchMenu` constant + register.
- `internal/routebuilder/routebuilder.go` — add `ResearchLanding()` (`/research`), `ResearchPersonScoped(action, id)` (`/research/{action}?person={id}` or path-based).
- `internal/appshell/routes.go` — register `GET /research` + `GET /research/{action}` (chi sub-router).
- `internal/appshell/research_handlers.go` (new) — `handleResearchLanding` (search + recents list) + `handleResearchPersonScoped` (dispatches to existing sub-page handlers with the picked person).
- `internal/appshell/research_picker.go` (new) — search helper that filters soldiers by name OR display_id substring, returns top-10.
- `internal/templates/research_landing.templ` (new) — picker UI: search input (htmx `hx-get="/research/search"` + `hx-trigger="input changed delay:250ms"` + `hx-target="#research-picker-results"`), results list (`<a href="/research/camaraderie?person={id}">`), recents list (read from localStorage via JS-injected hidden input).
- `internal/templates/research_person_scoped.templ` (new) — minimal shell that wraps the existing soldier-scoped sub-page contents (camaraderie, timeline, research-log, conflict-ledger, research-pack/state|county) inside a common "scoped to {person display_id}" header with breadcrumb back to `/research`.
- `internal/templates/soldier_card.templ:388-486` — replace `<details>` block (100 lines) with single always-visible tile (5-10 lines) linking to `/research?person={id}`.
- `internal/templates/components/foldout.templ` aria-current handling: extend to accept `aria-current` via `triggerAttrs` (already does per code review of primitive).
- `internal/cookies/research_session.go` (new) — signed cookie helpers: `SetResearchPerson(ctx, w, id)`, `GetResearchPerson(ctx, r) (id, ok)`.
- `frontend/app.js` — wire `applySmartBackLabels` for `/research/*` back stack (already works via existing `[data-history-back]` listener).
- `frontend/app.js` — `recents` localStorage helper: store last 10 picked persons on `/research/{action}` navigation; read on landing render.
- `internal/appsshell/research_handlers_test.go` (new) — RED-first tests.
- `audit/smoke_research.mjs` (new) — browser probe.
- `docs/ui-map/INDEX.md` — new row for `/research` landing + `/research/{action}` person-scoped surfaces.
- `docs/ui-map/surfaces.md` — `LayoutResearchMenu` + `PageResearchLanding` + `PanelResearchPickerResults` UIIDs.
- `docs/ui-map/routes.md` — `/research` + `/research/{action}` route + handler entries.
- `CHANGELOG.md` — bullet under `### Added`.

### Success criteria

1. Top-nav shows `<button>Research & Review</button>` next to Share foldout, aria-expanded toggles on click.
2. Clicking the trigger opens `/research` landing (full page, not a modal).
3. `/research` shows search input + recents list (or empty-state if no recents).
4. Typing in search filters person list live (htmx swap, 250ms debounce).
5. Clicking a person navigates to `/research/camaraderie?person={id}` (or path-based equivalent).
6. Person-scoped sub-pages render the existing soldier-scoped surface (camaraderie graph etc.) with a header showing "Scoped to {display_id}".
7. `SetResearchPerson` cookie set on selection; `GetResearchPerson` reads it; sub-pages use it as default when no `?person=` override.
8. Soldier_card page no longer renders the `<details>` block; instead shows a single tile with copy from #379 ("Open research tools for this Person Record" — eyebrow "Research & Review") linking to `/research?person={id}` (or the picker if no cookie).
9. UIID `LayoutResearchMenu` renders; `PageResearchLanding` renders on `/research`; `PanelResearchPickerResults` renders on search-results swap.
10. `audit/discover_orphan_handlers.mjs` shows no new orphans introduced.

### Regression net

- `TestResearchLandingRendersPicker` (handler test) — GET `/research` returns picker UI.
- `TestResearchLandingNoCookieShowsNoRecents` — empty cookie → no recents row in HTML.
- `TestResearchLandingWithCookieShowsRecents` — cookie set → recents row renders.
- `TestResearchSearchFiltersByDisplayID` — GET `/research/search?q=CSA-00411` returns the row.
- `TestResearchSearchFiltersByName` — GET `/research/search?q=Gillespie` returns matching rows.
- `TestResearchPersonScopedRequiresCookieOrQuery` — GET `/research/camaraderie` with no cookie + no query → 303 redirect to `/research`.
- `TestResearchPersonScopedWithQueryRendersSurface` — GET `/research/camaraderie?person=411` returns the camaraderie page contents.
- `TestResearchPersonScopedSetsCookie` — handler writes the cookie on first valid selection.
- `TestHandleSoldierDetailNoLongerRendersDetailsBlock` — soldier_card no longer emits `<details>` with summary "Advanced Research & Review".
- `TestHandleSoldierDetailRendersResearchTile` — new tile renders with the #379-mandated copy.
- `TestLayoutRendersResearchFoldout` — layout.templ emits both Foldout panels (Share + Research).
- `TestRegistryIncludesLayoutResearchMenu` — UIID registry has `LayoutResearchMenu`.
- `audit/smoke_research.mjs` steps:
  - Step 1: top-nav shows Research & Review trigger.
  - Step 2: click trigger → `/research` landing renders.
  - Step 3: type "Gillespie" → results list swaps in with matching row.
  - Step 4: click result → navigates to `/research/camaraderie?person=411`, URL unchanged assumption validated (cookie set), header shows "Scoped to CSA-00411".
  - Step 5: soldier_card page (pick any soldier) → no `<details>` block visible; research tile renders with copy from #379.

### Commit

Single commit. Layer coupling (layout + uiids + soldier_card + new handler + new templ + cookies + frontend JS + tests + ui-map + CHANGELOG) only makes sense together — the tracer-bullet slice proves the end-to-end critical path before any other apply-sites (timeline / research-log / conflict-ledger / research-pack) are added in subsequent slices.

## Subsequent slices (stub only — detail when their turn arrives)

- **Slice 2** — Person-scoped dispatch covers ALL 6 sub-pages (camaraderie, timeline, research-log, conflict-ledger, research-pack/state, research-pack/county). Slice 1 only wires `/research/camaraderie`; Slice 2 generalizes the dispatch.
- **Slice 3** — Recent-persons persistence across page reloads (currently only in-session via cookie). Lift to localStorage and back-fill on landing.
- **Slice 4** — `aria-current="page"` highlighting on the trigger when on any `/research/*` page (matches Share foldout's locked decision per issue #264).
- **Slice 5** — Documentation sweep: docs/ui-map/gaps.md reflects the old `<details>` removal + new landing; audit/_lib/orphan-handler probe flags the now-unused `components.Button` import sites if any.

## Out of scope

- Generalizing the picker to Event Records / Articles. Issue #378 specifies Person picker only.
- Replacing Insights / Review Queue / Tags with foldouts (issue #380).
- Cross-feature fixes swallowed errors discovered while touching this area (those go to #384).