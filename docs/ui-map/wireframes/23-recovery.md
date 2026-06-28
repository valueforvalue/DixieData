# 23 — Recovery

- **Route**: `/recovery` (GET, POST)
- **Builder**: none
- **Template**: `internal/templates/recovery.templ:UpdateRecoveryPage`
- **Owner**: package `templates`

## Regions

```
┌── Update Recovery ───────────────────────────────────────────────┐
│ "Update recovery"                                                 │
│ h1 "The last update did not finish a healthy first launch."       │
│                                                                    │
│ [Restore point card]                                              │
│  Restore point created: <ts>                                      │
│  Previous build: v{sourceVersion}                                 │
│  Failed update: v{targetVersion}                                  │
│                                                                    │
│ if failureMessage:                                                │
│   [failure callout]                                               │
│                                                                    │
│ if rollbackStarted:                                               │
│   [progress callout] "Restoring the previous build now…"         │
│ else:                                                              │
│   <form method=post action=/recovery>                             │
│     <btn: Restore previous build and Local Archive>              │
│                                                                    │
│ NOTE: Standalone HTML shell, NOT @Layout. Bare CSS link.          │
└───────────────────────────────────────────────────────────────────┘
```

This page is intentionally minimal — it's the failure-state landing
when an update doesn't complete a healthy first launch.

## Panels / tabs

None. Page is rendered without the global `Layout` shell.

## Atomic components

None — uses inline HTML/CSS, not the components library.

## HTMX wiring

None. Native `<form method="post" action="/recovery">` — full page
submit, redirects back to `/recovery` (with `rollbackStarted=true`).

## Footguns

- **Standalone shell** — does NOT include header, floating dock,
  feedback modal, jobs overlay, or footer. Verify nothing on this
  page relies on those.
- **`data-progress-label="Restoring previous version…"`** on the
  submit button — verify `frontend/app.js` wires this for
  in-flight label swap.
- **Restore Point data** is server-loaded — verify `createdAt`,
  `sourceVersion`, `targetVersion` always populate (no zero values).
- **Two states (`rollbackStarted` true/false)** — verify the form
  is hidden once started so user can't double-submit.

## See also

- [21-settings.md](21-settings.md) (Software Updates → Apply)
- `docs/audit/` (update recovery flow audit)