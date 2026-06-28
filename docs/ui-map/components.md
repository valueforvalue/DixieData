# Atomic Components

Visual primitives rendered via `internal/templates/components/`. Each
is a `templ` component callable from any screen.

| Component | File | Variants | Used for |
| --- | --- | --- | --- |
| `Button` | `components/button.templ` | `ButtonPrimary`, `ButtonSecondary` | All click actions |
| `Card` | `components/card.templ` | — | Surface a region |
| `EmptyState` | `components/empty_state.templ` | per-archive-keyword (calendar, soldiers, etc.) | Zero-data messaging |
| `Field` | `components/field.templ` | input/select/textarea wrappers | Form controls |
| `Pill` | `components/pill.templ` | default, link | Tags, nav links, status |
| `Toast` | `components/toast.templ` | success, error, info | Transient notifications |

## Conventions

- **Buttons**: prefer `Button` over raw `<button>`. Variants:
  - `ButtonPrimary` — main CTA (gold, dark).
  - `ButtonSecondary` — secondary action (outlined).
- **Fields**: wrap inputs in `Field` for consistent label/error/help
  text rendering. Raw `field-input` class is acceptable for inline
  controls in popouts.
- **Empty states**: use `components.EmptyState(archiveKeyword, ...)`
  keyed by archive (calendar, soldiers, etc.) so copy stays consistent.
- **Pills**: `pill-link` for nav links, `pill` for tags.

## Notes

- `internal/templates/partials/empty_state.templ` is the older
  `EmptyStateCard(...)` partial — redundant with `components.EmptyState`
  (the component accepts an archive keyword). See
  [gaps.md](gaps.md).
- Component tests live next to each component
  (`*_test.go` + `*_content_test.go` for goquery invariant checks).