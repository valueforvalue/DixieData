# Glossary

Visual vocabulary used across the UI. Each word maps to a structural
pattern; wireframes use these labels.

## Surfaces (DOM ID prefixes)

| Prefix | Meaning |
| --- | --- |
| `page.*` | Top-level routable screen. One per URL. |
| `panel.*` | Named region inside a page. Has a DOM ID. |
| `tab.*` | Switch trigger inside a page (paired with `panel.*`). |
| `overlay.*` | Floating UI (modal, popup, viewer). Z-indexed above page. |

## Layout regions (in a page)

- **header** — top nav bar, brand + pill links. Lives once in `Layout`.
- **main** — page body, swapped per route. Wireframe scope.
- **footer** — build identity + (debug mode) debug button. Lives in `Layout`.
- **floating dock** — fixed bottom bar with Scratch Pad, Feedback, Menu.
- **floating nav panel** — `hidden` by default, toggled from Menu.

## Spatial containers

- **drawer** — side-anchored region that pushes/overlays content.
- **popout panel** — small floating form/picker anchored to a trigger
  (e.g. Export Month dropdown).
- **modal** — full-overlay, blocks interaction until dismissed.
  `role="dialog"`, `aria-modal="true"`.

## Atomic components

See [components.md](components.md). One component per visual primitive:
`Button`, `Card`, `EmptyState`, `Field`, `Pill`, `Toast`.

## State regions

- **toast region** — `data-toast-region`, top-right (per CSS), transient
  success/error messages.
- **jobs progress overlay** — `data-jobs-progress-region`, polled every
  3s, fixed position. Lives once in `Layout`.
- **detail pane** — `#details-pane` (Calendar-specific), HTMX swap
  target for day clicks.

## Anti-patterns

- **string DOM IDs in templates** — use `uiids.*` constants.
- **string URLs in `hx-*` / `href`** — use `routebuilder.*()`.
- **raw `hx-*` attributes** — use `htmxattr.Mux{}.Attrs()`.