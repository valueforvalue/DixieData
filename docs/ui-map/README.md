# UI Map

Visual + structural reference for every screen in DixieData. Built for
bug hunting: spatial wireframes, canonical DOM IDs, owning files, and
cross-screen lookup. Hand-curated from `internal/uiids/`,
`internal/routebuilder/`, `internal/templates/`, and the active
audit round.

## Layout mode

The app shell supports two modes via `data-layout-mode`:

- **relaxed** (≥1000px) — top nav inline, roomier spacing
- **split-screen** (<1000px) — nav collapses into floating dock

Wireframes show the relaxed mode unless flagged otherwise.

## Documents

| Doc | Purpose | When to open |
| --- | --- | --- |
| [INDEX.md](INDEX.md) | Screen × panel matrix, full app at a glance | First stop |
| [routes.md](routes.md) | Every route → owning screen + handler | Tracing a URL back to UI |
| [surfaces.md](surfaces.md) | Canonical DOM IDs (`page.*`, `panel.*`, `tab.*`, `overlay.*`) | Naming a region, finding a constant |
| [components.md](components.md) | Atomic components (button, card, empty_state, field, pill, toast) | Picking the right primitive |
| [glossary.md](glossary.md) | Region vocabulary (drawer/modal/panel/section) | Disambiguating UI terms |
| [states.md](states.md) | Cross-cutting states (loading / empty / error / unauthorized) | Tracing state behavior |
| [gaps.md](gaps.md) | Orphaned routes, unrouted UI, redundancy findings | Hunting architectural debt |
| [wireframes/](wireframes/) | One ASCII wireframe per screen | Spatial reference while bug hunting |

## Conventions used here

- **DOM IDs** are `internal/uiids` constants, never string literals.
- **Files** are paths under `internal/templates/`.
- **Routes** are paths under `internal/routebuilder/routebuilder.go`.
- **Wireframes** are ASCII; they show relaxed mode unless flagged.
- **States** suffix each screen file (`empty`, `loading`, `error`,
  `unauthorized`) when the screen has non-trivial handling.

## How to use this

- **"Where is X rendered?"** → [INDEX.md](INDEX.md) → screen → wireframe.
- **"What's this DOM ID for?"** → [surfaces.md](surfaces.md).
- **"What owns this route?"** → [routes.md](routes.md).
- **"How does state look here?"** → screen wireframe + [states.md](states.md).
- **"Is this screen audited?"** → check screen footer for audit links.

## Status

Complete. All 23 screens wired. Gaps, routes, surfaces, components,
glossary, states, and per-screen footguns captured.

**Open follow-ups** (see [gaps.md](gaps.md)):
- Routebuilder coverage for `/export/*`, `/import/*`, `/integrations/*`,
  `/merge-review/*`, top-nav links.
- Dialog-guard audit on Share/Settings/Detail handlers.
- `partials/empty_state.templ` vs `components/empty_state.templ`
  redundancy.
- Audit forward links from each wireframe (populated as `audit/`
  rounds land).