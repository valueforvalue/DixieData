# 28 — Event PDF

- **Route**: `/events/{id}/pdf` (GET, POST)
- **Builder**: `routebuilder.EventPDF(id)`
- **Template**: — (PDF)
- **Layout**: n/a
- **Owner**: package `appshell` (event PDF handler)

Renders a per-Event PDF (issue #320 v1) via the same Typst pipeline
the `/soldiers/{id}/pdf` route uses, scoped to a single Event Record.
The HTML wrapper chrome is intentionally absent — the response body
is the rendered PDF binary with a `Content-Disposition: inline` /
`attachment` header.

## Regions

None. The request is a binary download.

## Panels / tabs

None.

## Atomic components

None.

## HTMX wiring

| Trigger | Verb | URL | Target | Swap | Notes |
| --- | --- | --- | --- | --- | --- |
| EventPDFExport btn | GET | `routebuilder.EventPDF(id)` | `this` | `none` | Triggers download. Same shape as `SoldierPDF`. |

The button is rendered by the
`components.EventPDFExport(eventID)` partial on the event detail
page; the detail page itself is [25-event-detail.md](25-event-detail.md).

## Modals / overlays

- `overlay.print-config.modal` — opened from the EventPDFExport btn
  when PDF preferences are set (`data-pdf-pref-scope="event"`).

## State variants

- **404**: `respondNotFound` when the event id is unknown.
- **Render failure**: handler returns 500 with `X-DixieData-Toast` so
  the user gets a toast and the modal stays open (the same failure
  surface as the per-Soldier PDF).

## Footguns

- **Native dialog guard**: the export path opens a `SaveFileDialog`
  on the desktop binary. MUST be guarded with `a.inFlight.LoadOrStore`
  per [dialog-guard.md](../../agents/dialog-guard.md). The handler
  uses `a.saveFileDialogOverride` in tests so CI bypasses the native
  dialog.
- **`PDFExcerptOverride`** — when set on the event record, the PDF
  uses the shorter override text in the excerpt block instead of
  `description`. The detail page surfaces a yellow callout when
  this is active.
- **No routebuilder test** for the GET vs POST dual-mount — verify
  both routes are registered in `internal/appshell/routes.go`.

## See also

- [25-event-detail.md](25-event-detail.md) (caller surface)
- [08a-exports.md](08a-exports.md) (share exports)
