# 11 — Review Queue

- **Route**: `/review-queue` (GET), `/review-queue?page=…` (pagination)
- **Builder**: `routebuilder.ReviewQueueBulk()`
- **Template**: `internal/templates/review_queue.templ:ReviewQueueView`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Review Queue ───────────────────────────────────────────────────┐
│ if zero records: EmptyStateCard("review", counts)                 │
│ h2 + intro copy                                                   │
│ [panel.review-queue.list]                                        │
│  if entries == 0:                                                │
│    "The review queue is clear."                                   │
│  else:                                                            │
│    <form> hx-post ReviewQueueBulk() hx-target="body"             │
│      [Select-all + Ignore Selected + Delete Selected toolbar]    │
│      for each entry:                                              │
│        [Entry card #review-queue-item-{id}]                       │
│          Compare cb | DisplayID + Needs Review pill               │
│          Name | Review reason | subhead                           │
│          if duplicate findings: [Compare with N] pill(s)         │
│          View Person Record pill | Mark as Resolved btn (hx-post) │
│    pagination (Previous / Next)                                   │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `panel.review-queue.list` | Main section | List + bulk form + pagination |

## Atomic components

- `Button` — Ignore Selected, Delete Selected, Mark as Resolved.
- `Card` — section wrapper.
- `Pill` — Compare, View.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Bulk form submit | POST | `routebuilder.ReviewQueueBulk()` | `body` | `name="bulk_action"`, value=`ignore` / `delete` |
| Mark as Resolved btn | POST | `/soldiers/{id}/review/resolve?context=queue` | `#review-queue-item-{id}` | `hx-swap="outerHTML"` removes the row |
| Compare with N pill | GET | `/review-queue/compare/{findingID}` | (full nav) | → [12-review-queue-compare.md](12-review-queue-compare.md) |
| Pagination | GET | `/review-queue?page=…` | (full nav) | Bare URL — routebuilder gap |

## State variants

- **Zero archive**: EmptyStateCard.
- **Empty queue**: "The review queue is clear."
- **Single page**: pagination hidden.

## Footguns

- **Bulk action targets `body`** — relies on server returning
  redirect. Verify handler does so.
- **`bulkDeleteConfirmMessage` and `bulkIgnoreConfirmMessage`**
  templated prompts — page count comes from page size, not the
  selected count. Misleading. Document or fix.
- **`hx-swap="outerHTML"` on Mark as Resolved** — replaces the card
  div. If the server returns just success copy instead of empty,
  the card stays.
- **Bare `/review-queue/compare/{findingID}`** and
  `/soldiers/{id}/review/resolve?context=queue` — routebuilder gap.
- **`data-select-all="review-queue"`** — checkbox group `review-queue`
  must be unique across screens.

## See also

- [12-review-queue-compare.md](12-review-queue-compare.md)
- [19-merge-review-ledger.md](19-merge-review-ledger.md)