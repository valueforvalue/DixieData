# Templ conventions

This file documents the conventions DixieData templates follow. The
Stabilization Sprint codified these so future authors (human or AI)
produce markup that composes cleanly with the route builders, the
uiids registry, and the goquery test layer.

## Three-attribute namespace rule

DixieData uses three different purposes for HTML data attributes.
Mixing them produces bugs because an author sees an attribute and
infers the wrong purpose.

### `data-testid="..."` — goquery selectors only

Owned by the template that defines it. The `internal/templates/page_snapshot_test.go`
file uses these as stable anchors for goquery assertions. The
Stabilization Sprint reserved `data-testid` exclusively for tests so a
future cosmetic refactor never silently breaks a snapshot.

### `data-<feature>-...` — runtime JS hooks only

Owned by `frontend/app.js` (or whoever reads it). The feature prefix
matches the page that owns the JS hook: `data-browse-*` for browse,
`data-jobs-*` for jobs, `data-debug-*` for the debug console. NOT a
test selector.

### `data-ui-id` — REMOVED

Removed in PR #0 (Stabilization Sprint). Used to be a developer-overlay
attribute. The PR #0 cleanup removed it from 52 template sites, the
`SurfaceBadge` / `InlineSurfaceBadge` components, the `DebugEnabled`
toggle, and the `data-debug-ui-ids` env-var/flag machinery. Do NOT
reintroduce. The `uiids` package itself is still useful (see below).

## URL building

All URLs templates reference (in `hx-get`, `hx-post`, `href`, form
action attributes) must go through `internal/routebuilder`. Bare
string literals like `hx-get="/some/route"` are flagged by
`internal/templates/hx_guard_test.go::TestHXURLsUseBuilders`. The
builder list is auto-discovered from `routebuilder.go`, so adding a
new builder is the only way to satisfy the test for a new URL.

Wrap every builder call in `templ.SafeURL` so URLs stay escaped:

```templ
<a href={ templ.SafeURL(routebuilder.SoldierDetail(soldier.ID)) }>...</a>
```

Or use the typed wrapper for HTMX attributes:

```templ
<form { htmxattr.Mux{
    Get:    routebuilder.BrowseResults(),
    Target: "#browse-results",
}.Attrs()... }>
```

## Selector IDs

`hx-target` and `hx-select` attributes that start with `#` should
reference an ID from `internal/uiids.Registry`. The registry holds
78 surface identifiers (PageCalendar, PanelBrowseResults, etc.). The
`TestHXTargetsPreferRegistry` test reports ad-hoc selectors that
should be promoted to the registry.

Ad-hoc selectors are allowed (transient panels like `#feedback-form`
don't earn a registry entry) but consider promoting them when the
panel becomes durable.

## HTMX attribute typing

`hx-get`, `hx-post`, `hx-target`, `hx-select`, `hx-swap`, `hx-trigger`,
`hx-confirm` go through `internal/htmxattr.Mux`. URL values are wrapped
in `templ.SafeURL`. `Swap` is validated against an allowlist (invalid
values panic at render time). `Target` selectors that start with `#`
are checked against the uiids registry.

## Component primitives

Reuse the existing primitives in `internal/templates/components/`:

- `Button(label, kind, extraClass, attrs)` — primary button primitive
- `Card(title, content)` — structural bounding container
- `Pill(label, href, extraClass, attrs)` — pill-shaped link
- `EmptyState(...)` — empty-state placeholder
- `Field(...)` — labelled input field
- `Toast(...)` — toast region primitive

Don't write raw `<button class="primary-button ...">` markup; use
the `Button` primitive so the visual vocabulary stays centralised.

## File organisation

- Big templates (entry_form, soldier_card, share) are split across
  multiple `.templ` files. All `templ` symbols in the same Go package
  can reference each other freely — splitting is additive.
- Pure Go helpers (no `templ` syntax) live in `.go` files alongside
  the `.templ` files. The first PR #4 split moved entry_form helpers
  to `entry_form_helpers.go`.
- One big file is a smell. If a `.templ` file passes 600 lines, look
  for natural seams to split.

## Tests

- Existing component snapshots stay byte-equality (regression net for
  visual primitives).
- Page snapshots (`internal/templates/page_snapshot_test.go`) assert
  behavioral invariants via goquery, not byte-equality. New page
  tests should follow this pattern.
- Architectural boundaries are checked by
  `internal/architecture/architecture_test.go`. The forbidden-import
  table names every deep-module package; adding a new one means
  updating the table in the same commit.
- HTMX attribute invariants are checked by
  `internal/templates/hx_guard_test.go`. Currently advisory; flips
  to strict when the route builder inventory is complete.