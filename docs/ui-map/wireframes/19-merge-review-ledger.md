# 19 — Merge Review Ledger

- **Route**: `/soldiers/{id}/conflict-ledger` (GET), via
  `routebuilder.SoldierConflictLedger(id)`
- **Template**: `internal/templates/conflict_ledger.templ`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Merge Review Ledger ───────────────────────────────────────────┐
│ [← Back btn] [Open Person Record pill]                           │
│                                                                    │
│ responsive-2-col:                                                 │
│  ┌──[aside]────────────────┐  ┌──[main]──────────────────────┐  │
│  │ h2 Name + DisplayID      │  │ if entries == 0:              │  │
│  │ [Open Conflicts counter] │  │   "No shared-import conflicts"│  │
│  │ [Resolved Entries counter]│  │ else:                         │  │
│  │ [Ledger Purpose copy]    │  │   per entry:                  │  │
│  └──────────────────────────┘  │     status pill, type pill    │  │
│                                 │     IncomingDisplayID         │  │
│                                 │     Reason copy               │  │
│                                 │     Created/Resolved ts       │  │
│                                 │     [Difference field chips]  │  │
│                                 │     [Local snapshot] card     │  │
│                                 │     [Incoming snapshot] card  │  │
│                                 └──────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

`page.merge-review-ledger` registered. No inner panels.

## Atomic components

- `Button` — Back.
- `Card` — aside + per-entry sections.

## HTMX wiring

None. Read-only ledger. Pagination, if any, is plain `<a href>`.

## Footguns

- **Read-only view** — confirm no accidental action buttons were
  rendered.
- **`conflictLedgerStatusLabel`** treats empty resolution as "Open".
- **Snapshot data may be stale** — verify how the viewmodel handles
  the local/incoming record state at conflict time vs now.

## See also

- [12-review-queue-compare.md](12-review-queue-compare.md)
- [08-export.md](08-export.md) (Merge Review during import)