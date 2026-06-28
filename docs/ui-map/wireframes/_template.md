# Wireframe format

Every screen wireframe follows this skeleton. ASCII only.

## Frontmatter

```markdown
# NN — Screen Name

- **Route**: `/path/{param}` (HTTP method)
- **Builder**: `routebuilder.Foo(...)` (if exists)
- **Template**: `internal/templates/foo.templ`
- **Layout**: relaxed / split-screen / both
- **Owner**: package `templates`
- **Audit**: link to round-N finding(s), if any
```

## Sections

1. **Regions** — top-level layout of the page.
2. **Panels / tabs** — `panel.*` / `tab.*` regions with DOM IDs.
3. **Atomic components** — buttons, fields, pills used.
4. **HTMX wiring** — every `hx-get` / `hx-post` / target / swap / trigger.
5. **Modals / overlays** — every `overlay.*` triggered from here.
6. **State variants** — empty / loading / error (omit if trivial).
7. **Footguns** — known bugs, recent fixes, dialog-guard notes.

## ASCII conventions

- Boxes `[ ]` = containers with an ID.
- `<btn>` = button.
- `<field>` = input.
- `[tab: id]` = tab trigger.
- `[panel: id]` = panel region.
- `↑ ↓` = vertical flow.
- `→ target` = HTMX swap target.
- `#id` = DOM ID.
- `«modal»` = modal trigger.
- `[hidden]` = hidden by default.
- `↻` = polled / auto-refresh.

## Footguns section template

```markdown
## Footguns

- **<short title>** — <one-line description>. Fix:
  <commit or PR ref>. Documented in `docs/COMMON_BUGS.md` § <section>.
```