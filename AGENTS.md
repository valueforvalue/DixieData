# AGENTS.md

Working notes for AI coding agents operating in this repository.

## Domain vocabulary (ubiquitous language)

DixieData has a strict, evolving glossary. Every feature, doc, and commit message should
match. **Before writing any user-facing copy or schema-touching code, read
[`CONTEXT.md`](CONTEXT.md) end-to-end.** The canonical terms:

- **Person Record** — primary archive entry for one person (soldier, wife, widow)
- **Display ID** — canonical user-facing identifier for a Person Record
- **Local Archive** — live working collection on one machine
- **Shared Archive** — merge-oriented archive package exchanged between users
- **Backup Archive** — full replacement snapshot for restore and safekeeping
- **Restore Point** — automatic pre-update recovery bundle
- **Static Archive** — read-only browser-viewable export
- **Source Record** — attached evidence item (pension, application, etc.)
- **Claim** — assertion extracted from a Source Record
- **Finding** — verified Claim that has cleared review

See `CONTEXT.md` for the full glossary and anti-patterns.

## File map (entry points)

| Path | Role |
|---|---|
| `main.go` | Wails app entry point (also handles `--smoke` headless boot) |
| `internal/appshell/` | App bootstrap + request handlers + stress tests |
| `internal/db/` | SQLite schema, migrations, `GetAppVersion()` |
| `internal/htmxattr/` | Typed `htmxattr.Mux` builder — use instead of raw `hx-*` strings |
| `internal/routebuilder/` | Typed URL builders — use instead of string route literals |
| `internal/records/` | Person Record, Source Record, Claim, Finding model logic |
| `internal/templates/` | Templ HTML templates (regenerate with `make tpl`) |
| `internal/uiids/` | Canonical DOM ID constants (used by goquery invariant tests) |
| `internal/exportcontract/` | Shared types between Go export pipeline and JS frontend |
| `frontend/` | Static assets, Tailwind input, `app.js`, `app.css` output |
| `audit/` | UI/UX audit harness (round 1 → 4), `npm run audit` |
| `tools/tune/` | Iteration harness for design polish |
| `templates/` | Typst source templates (PDF rendering, NOT Go `text/template`) |
| `docs/` | User manual, ADRs, audit narrative, release docs |
| `docs/ui-map/` | **UI reference** — screen × component matrix, per-screen ASCII wireframes, route/surface lookup, gaps. Read first for any UI bug hunt or redesign. |
| `scripts/` | PowerShell + bash build/test helpers (`build-common.ps1`, `run-crash-dump.ps1`, `debug-crash.dlv`) |
| `Makefile` | Top-level DX (run `make help` for all targets) |
| `CONTEXT.md` | **Glossary + Laws source of truth** — read first |

## Commits and branches

- **One commit = one logical change.** If the message splits cleanly in half and each half still stands alone, you have two commits. Recurring failure: a 200-line "fix export buttons" commit that bundles the templ + handler + audit test + regression net + CHANGELOG. That should be 4 commits.
- **Commit message shape:** `<area>: <imperative summary>` for the subject (≤72 chars), blank line, then 1–3 bullets explaining *why* and what the regression net is. Reference the issue number if one exists (`issue #130`). Look at recent commits with `git log --oneline -20` for the in-repo house style.
- **Branch names:** `feature/<short-kebab>` for new surfaces, `fix/<short-kebab>` for bug fixes, `chore/<short-kebab>` for refactors / docs. Never `agent-scratch`, never `temp`, never `wip`. Push the branch the moment it has one green commit.
- **Before pushing:** `make test` (runs `go test ./... -short`) and `make tpl` (regenerates templ — the diff should be empty if your templ edits match the generated output). If you touched htmx or templ markup, run `node audit/smoke.mjs` against a live `dixiedata-web` server. `make audit` runs the full visual sweep.
- **CHANGELOG:** every user-visible change gets a bullet in `CHANGELOG.md` `[Unreleased]` under `### Added`, `### Changed`, `### Fixed`, or `### Maintenance` in the same commit that lands the change. Internal refactors that don't change user-visible behavior live under `### Maintenance`.
- **Click-driven surfaces:** any new templ button that POSTs and expects navigation must follow the recipe in `internal/templates/components/conventions.md` ("Buttons that POST and expect navigation") AND grow a matching `audit/smoke.mjs` assertion that verifies both the response shape AND `page.url()` after the click. The response-only assertion is insufficient — that is how the htmx `hx-swap="none"` + 303 silent-swallow bug shipped (commit `70878ac` → caught in `3612dab`).

## Agent skills

### Working guides (read before touching the layer)

These two files are the high-leverage pre-commit reads. They document
recurring bug patterns extracted from 79 `fix:` commits across the
history. Read the section that matches the layer you're about to
touch:

- [`docs/COMMON_BUGS.md`](docs/COMMON_BUGS.md) — bug-pattern catalog
  by layer (HTMX wiring, templ markup, frontend JS, Go backend,
  Typst, accessibility, calendar/API, build/CI, database,
  debugging). Includes `Find it:` greps for each pattern.
- [`docs/CODE_CHANGES.md`](docs/CODE_CHANGES.md) — cross-layer
  working contract. Read this when your change touches templ +
  htmx + JS + Go handler together (the chi-router migration and
  the hx-attr strip both shipped as one-system drift).

### UI map (read before any UI bug hunt or redesign)

[`docs/ui-map/README.md`](docs/ui-map/README.md) is the single
entry point for the UI reference. It contains:

- [`INDEX.md`](docs/ui-map/INDEX.md) — screen × component matrix.
- [`routes.md`](docs/ui-map/routes.md) — every route → owning
  screen + handler.
- [`surfaces.md`](docs/ui-map/surfaces.md) — canonical DOM IDs
  (`page.*`, `panel.*`, `tab.*`, `overlay.*`).
- [`components.md`](docs/ui-map/components.md) — atomic components
  (button, card, empty_state, field, pill, toast).
- [`glossary.md`](docs/ui-map/glossary.md) — region vocabulary
  (drawer/modal/panel/section).
- [`states.md`](docs/ui-map/states.md) — cross-cutting states
  (loading / empty / error / unauthorized).
- [`gaps.md`](docs/ui-map/gaps.md) — orphaned routes, unrouted UI,
  redundancy findings.
- [`wireframes/`](docs/ui-map/wireframes/) — one ASCII wireframe
  per screen, with HTMX wiring tables and per-screen footguns.

When hunting a UI bug, start at the wireframe for the affected
screen, then follow the HTMX wiring table to the owning handler.
  htmx + JS + Go handler together (the chi-router migration and
  the hx-attr strip both shipped as one-system drift).

### Issue tracker

Issues are tracked in GitHub Issues for this repository using the `gh` CLI.
See `docs/agents/issue-tracker.md`.

### Triage labels

The triage label vocabulary uses the canonical labels: `needs-triage`, `needs-info`,
`ready-for-agent`, `ready-for-human`, and `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

This repo uses a single-context domain-doc layout centered on the root `CONTEXT.md`;
ADRs live under `docs/adr/` when present. See `docs/agents/domain.md`.

### Native dialog guard law (read before adding any export or import)

Wails v2.12.0 on Windows crashes the frontend process if two native `SaveFileDialog`
or `OpenFileDialog` calls land on the UI thread at the same time. Every export and
import handler MUST guard its native dialog call with `a.inFlight.LoadOrStore` +
`defer a.inFlight.Delete`. **Before adding or modifying a handler that opens a
native dialog, read [`docs/agents/dialog-guard.md`](docs/agents/dialog-guard.md)
end-to-end.** The pattern (helper, inline, or sentinel-error) and the regression
tests are documented there. The rule is also encoded at glossary level in
`CONTEXT.md` under "Laws (non-negotiable)" so domain work can't drift past it.

### CLI / headless mode (read before adding non-GUI entry points)

`dixiedata --smoke` boots without the GUI for CI and user support.
The full subcommand roadmap (`doctor`, `list`, `show`, `search`,
`export`, `import`, `migrate`, `backup`, `restore point`, `logs`,
`config`, `debug`) is staged across 7 phases in
[`docs/agents/cli-plan.md`](docs/agents/cli-plan.md). Phase 1
(`smoke`) is shipping now. **Before adding any new subcommand, read
the phase layout in that doc** — every CLI command dispatches to
existing `*App` methods, never duplicates handler logic.
