# Research Picker

- **Route**: `/research` (GET, with `?q=...&next=...&partial=1` variants)
- **Builder**: `routebuilder.ResearchPicker()`, `routebuilder.ResearchSearch()`, `routebuilder.ResearchRecent()`, `routebuilder.ResearchSelect()`, `routebuilder.ResearchClear()`
- **Template**: `internal/templates/research_picker.templ`
- **Layout**: relaxed
- **Owner**: package `templates`
- **Audit**: issue #378 slices 1+2+3, issue #422 slice 2 (Continue buttons), issue #426 follow-up (dispatcher opt-in)

## Purpose

Person picker landing for the top-nav Research & Review foldout
(issue #378). Reached when the user clicks a soldier-scoped entry
in the foldout (Camaraderie / Timeline / Research Log / Conflict
Ledger / Research Pack) without a current Person Record picked.
Research Collections is global, so it bypasses the picker.

Renders three regions (Continue shortcut when a Person is set;
Search; Recent). The picker writes a signed `dd_person_ctx` cookie
on select and redirects via `X-DixieData-Redirect` to the chosen
sub-page.

## Regions

```
┌── page.research.picker ──────────────────────────────────────┐
│ [← back]                                                     │
│ header:                                                      │
│   eyebrow: "Research & Review"                                │
│   h1: "Choose a Person Record"                               │
│   intro copy (cookie storage note)                           │
│                                                                │
│ if CurrentPerson != nil:                                     │
│ ┌── panel.research.picker.continue ────────────────────────┐ │
│ │ eyebrow: "Continue"                                       │ │
│ │ "Continue: {FirstName} {LastName} ({DisplayID})"          │ │
│ │ flex row of one <btn: Continue to {Action}> per supported │ │
│ │   sub-page + <btn: Change Person…>                         │ │
│ │ (each Continue button = inline form POST /research/select) │ │
│ └────────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌── panel.research.picker.search ──────────────────────────┐ │
│ │ label: "Search by name or Display ID"                     │ │
│ │ form GET /research?q=...:                                  │ │
│ │   <field: research-picker-query> (type=search, hx live)   │ │
│ │   <hidden next> (if NextAction set)                       │ │
│ │   <btn: Search>                                            │ │
│ │ ── search results region (hx-swap target) ──────────────  │ │
│ │ ┌── panel.research.picker.results ─────────────────────┐ │ │
│ │ │  if no query: "Type a name or Display ID…"            │ │ │
│ │ │  if no matches: "No matches for that query."          │ │ │
│ │ │  else: per-result card:                               │ │ │
│ │ │    {FirstName} {LastName} ({DisplayID}) + <btn: Open>  │ │ │
│ │ │    each Open = inline form POST /research/select       │ │ │
│ │ └───────────────────────────────────────────────────────┘ │ │
│ └────────────────────────────────────────────────────────────┘ │
│                                                                │
│ if NextAction == "research-pack":                            │
│ ┌── panel.research.picker.pack-sub-screen ─────────────────┐ │
│ │ eyebrow: "Research Pack geography"                         │ │
│ │ <select: research-pack-geography> (state / county?)       │ │
│ │ <btn: Save choice>                                         │ │
│ │ form GET /research?next=research-pack                     │ │
│ └────────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌── panel.research.picker.recent ───────────────────────────┐ │
│ │ eyebrow: "Recent"                                          │ │
│ │ if empty: "No recent picks yet."                          │ │
│ │ else: <ul> of person rows (form POST /research/select     │ │
│ │   per <li>; hydrated from localStorage on DOMContentLoaded │ │
│ │   via /research/recent fragment swap)                     │ │
│ └────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| Panel ID | Surface kind | Notes |
| --- | --- | --- |
| `page.research.picker` | page | Wrapper for `/research` |
| `panel.research.picker.continue` | panel | Continue shortcut; renders only when `dd_person_ctx` cookie is set |
| `panel.research.picker.search` | panel | Search input + form |
| `panel.research.picker.results` | panel | htmx swap target for live-search results (also the `?partial=1` fragment) |
| `panel.research.picker.pack-sub-screen` | panel | Research-pack geography picker; renders only when `?next=research-pack` |
| `panel.research.picker.recent` | panel | Recent list region (server-rendered empty + JS-hydrated) |

All registered in `internal/uiids/uiids.go` (`PageResearchPicker`,
`PanelResearchPickerContinue`, `PanelResearchPickerSearch`,
`PanelResearchPickerResults`, `PanelResearchPickerPackSubScreen`,
`PanelResearchPickerRecent`).

## Atomic components

- `Button` (`pill-link` class) — Search, Continue to {Action}, Change Person, Open.
- `Field` — search input + select + hidden inputs.
- `Card` — region wrappers (`card rounded-3xl p-5`).

## JS-hook data-* markers (no uiid)

| Marker | Owner | Notes |
| --- | --- | --- |
| `data-research-query` | `frontend/app.js` (none currently) | Marker on the search input; reserved for future client-side handling |
| `data-research-results` | `frontend/app.js` (none) | htmx swap target on the results wrapper (also asserted by Templ tests) |
| `data-research-continue-action` | none | Per-button marker carrying the `next` action key |
| `data-research-clear` | none | Marker on the Change Person button |
| `data-research-pack-sub-screen` | none | Marker on the geography section wrapper |
| `data-research-pack-geography` / `data-research-pack-state` / `data-research-pack-county` | none | Markers on the geography select / option elements |
| `data-research-recent-list` / `data-research-recent-empty` | `frontend/app.js#hydrateResearchPickerRecents` | Recents hydration: reads `localStorage[dixiedata.research.recents]`, GETs `/research/recent?ids=...&next=...`, swaps into this target |

Note: the issue #381 body listed speculative literal markers
(`data-research-picker-search`, `data-research-picker-recent`,
`data-research-picker-continue`, `data-dd-person-ctx`). Those
were never wired; the implementation uses the panel-level uiids
above + the JS-hook `data-research-*` family. Documented as a
drift correction so future agents don't add the speculative IDs.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| Search input | GET | `routebuilder.ResearchSearch()` | `[data-research-results]` | `outerHTML` | `hx-trigger="keyup changed delay:300ms, search"`, includes `[name='next']` |
| Search form submit | GET | `routebuilder.ResearchPicker()` | (page) | (default) | Falls back to full-page render when JS is off |
| Geography form submit | GET | `routebuilder.ResearchPicker()` | (page) | (default) | Echoes `?next=research-pack` + `?geography=state|county` |
| Continue buttons | POST | `routebuilder.ResearchSelect()` | (custom) | `X-DixieData-Redirect` | All forms carry `data-dixie-submit="true"` for the JS dispatcher (issue #426 follow-up) |
| Result / Recent Open | POST | `routebuilder.ResearchSelect()` | (custom) | `X-DixieData-Redirect` | Same dispatcher pattern |
| Change Person | POST | `routebuilder.ResearchClear()` | (custom) | `X-DixieData-Redirect` | Sets `MaxAge=-1` on cookie |

The `/research/recent` GET endpoint returns just the
`panel.research.picker.recent` inner contents (the `<ul>` or
empty `<p>`), so the JS hydrator can swap it in without a
full-page re-render. Issue #378 slice 3.

## State variants

- **No current person + no query + no recents**: three empty
  regions render. Default landing for a first-time user.
- **Current person set**: Continue shortcut appears at the top;
  per-Continue buttons filtered by `view.SupportedActions`
  (a Soldier missing unit data doesn't see Camaraderie, etc.).
  Issue #422 slice 2.
- **Query with matches**: results panel renders one card per
  match.
- **Query with no matches**: results panel renders "No matches
  for that query."
- **`?next=research-pack`**: pack-sub-screen renders between
  Search and Recent with the geography `<select>`.
- **Recent list populated**: `<ul data-research-recent-list>`
  renders one `<li>` per recents-id from localStorage; each
  `<li>` is its own form.

## Footguns

- **`dd_person_ctx` cookie** — signed; cleared via
  `routebuilder.ResearchClear()` (MaxAge=-1). The Continue +
  Change Person + Open forms ALL must carry
  `data-dixie-submit="true"` so the JS dispatcher intercepts
  the native submit + reads `X-DixieData-Redirect` + navigates.
  Without the attribute, browsers POST natively, the server
  returns 200 + empty body + the custom header (which browsers
  ignore), and the user lands on a blank white page at
  `/research/select`. Issue #426 follow-up + the regression
  net in `TestResearchPickerFormsOptIntoDispatcher` +
  `TestResearchRecentFormsOptIntoDispatcher`.
- **htmx live-search target selector** — `hx-target="[data-research-results]"`
  uses a data-attribute selector, NOT a dot-separated uiid
  (per issue #453). Same anti-pattern was applied here from
  day one.
- **Pack-sub-screen** — only renders when `NextAction ==
  "research-pack"` server-side. A user who manually visits
  `/research?next=camaraderie` doesn't see the geography picker
  (correct). A user who visits `/research` (no `?next=`) sees
  no Continue shortcut (correct — there's no sub-page to
  continue to).
- **Recent list ordering** — recents are returned in
  localStorage-insertion order (most recent first), not
  alphabetical. The picker preserves the order the JS hydrator
  sends.
- **`person_id` form field** — every Continue / Open form
  carries a hidden `person_id`. The `person_id` from a stale
  tab + a freshly-rotated cookie can mismatch; the server
  rejects the select with a 400 in that case (covered by the
  picker handler's person-cookie validation).

## See also

- [13-research-collections-hub.md](13-research-collections-hub.md)
- [14-research-collection-detail.md](14-research-collection-detail.md)
- [15-research-log.md](15-research-log.md) (one sub-page the picker routes to)
- [05-soldier-detail.md](05-soldier-detail.md) (soldier record that the picker picks)
- `docs/agents/notes/378-379-research-foldout.md` (slice plan)
- `docs/ui-map/surfaces.md` (`page.research.picker`, panel family, `layout.research.menu` + `layout.research.menu.trigger`)
