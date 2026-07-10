# Historical — Merge Review Ledger (deprecated 2026-07-10)

This wireframe documents the **pre-issue-#455** `/soldiers/{id}/conflict-ledger`
sub-page. The sub-page has been **deleted** as part of issue #455 slim R&R
foldout:

- The data lives on the **Review Queue Resolved tab** at
  `/review-queue?tab=resolved` (shipped in slice 4 as a placeholder +
  tab strip; the listing content is a slice-4 follow-up).
- The dispatch branch in `soldiers_handlers.go` is removed and
  `handleConflictLedger` is now a 303→/review-queue?tab=resolved stub.
- The foldout menu entry and soldier_card tile for the Merge Review
  Ledger were removed in slice 1.
- The `merge_review_conflicts` table is alive (audit-facade +
  shared-archive replay paths still use it).

This wireframe is retained for archaeology per `docs/historical/`
retention rule. For the current surface inventory see
[`docs/ui-map/INDEX.md`](../ui-map/INDEX.md).

The original wireframe text follows below.

---

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