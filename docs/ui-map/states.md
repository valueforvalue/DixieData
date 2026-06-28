# Cross-Cutting States

States every screen should handle. Listed once here, referenced from
each screen wireframe that has non-trivial handling.

## Loading

- **Initial page load**: full HTML response from handler. No skeleton
  rendered. Server-side rendered Templ.
- **HTMX swap**: target element is replaced. Brief flash of stale
  content possible; mitigated by `hx-indicator` on long requests.
- **Background job polling**: `OverlayJobsProgress`
  (`data-jobs-progress-region`) polls `/jobs/active` every 3s. Renders
  progress card while active. Silent kinds (e.g. `static_archive`)
  filtered out — region stays empty.

## Empty

- **Zero-archive state**: `components.EmptyState(archiveKeyword, counts)`
  with `counts.TotalRecords() == 0`. Per-screen copy via archive
  keyword (calendar, soldiers, browse, etc.).
- **Zero-results state**: results panel renders inline copy or
  `EmptyState`. Distinct from zero-archive.

## Error

- **Form validation**: server-rendered error messages under field.
  Client-side validation is progressive enhancement only.
- **HTMX 4xx/5xx**: target element shows response. Toast region
  surfaces top-level errors via `data-toast-region` (success/error/info).
- **Dialog guard** (`docs/agents/dialog-guard.md`): native file dialogs
  guarded by `a.inFlight.LoadOrStore`. Violation crashes the Wails
  frontend on Windows. See [gaps.md](gaps.md) for handler inventory.

## Unauthorized

- **Local-only app**: no auth model. State not currently surfaced.

## Not implemented

- **Skeletons**: no skeleton loaders. Initial loads block.
- **Optimistic UI**: most HTMX mutations re-render the server response.
  Scratch pad and similar use optimistic updates; check
  `frontend/app.js` for inline patterns.
- **Offline**: no offline state. SQLite is local-only.