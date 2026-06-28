# 12 — Review Queue Compare

- **Route**: `/review-queue/compare/{findingID}` (GET),
  `/review-queue/compare?id1=…&id2=…&from=…` (manual compare)
- **Builder**: none (bare URLs)
- **Template**: `internal/templates/review_queue.templ:ReviewQueueCompareView`
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── <comparisonTitle> ─────────────────────────────────────────────┐
│ h2 + Reason + differing/matching field count pills                │
│ [← Back btn] [Mark Match Resolved btn] (if finding open)          │
├───────────────────────────────────────────────────────────────────┤
│ [panel.review-queue.compare]                                     │
│  responsive-two-col:                                              │
│   [Left card]  DisplayID + name + subhead                         │
│                N source records / N images / Needs Review pill    │
│                "Open Left Person Record" pill                     │
│   [Right card] (mirrored)                                        │
│                                                                    │
│  if differences:                                                  │
│    "Differences to Review First" — chips of differing fields      │
│                                                                    │
│  [Field comparison table]                                         │
│   headers: Field | LeftID | RightID                              │
│   for each field:                                                 │
│     [row highlighted if differing — gold tint]                   │
│     Field label | Left value | Right value                       │
│   mobile: vertical cards (md:hidden)                              │
└───────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `panel.review-queue.compare` | Main section | Side-by-side comparison + field table |

## Atomic components

- `Button` — Back, Mark Match Resolved.
- `Card` — section + left/right record cards.
- `Pill` — Open Left/Right Person Record.

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Mark Match Resolved | POST | `/review-queue/compare/{findingID}/resolve` | (default) | No `hx-target` — verify server response |
| Per-row Open pill | GET | `/soldiers/{id}` | (full nav) | Bare URL |
| Back btn | JS | — | — | `data-history-back`, falls back to `comparisonBackHref(comparison)` |

## Footguns

- **No `hx-target` on Mark Match Resolved** — relies on default
  (current target). Verify the server returns a redirect or
  meaningful response.
- **Bare `/review-queue/compare/{findingID}/resolve`** —
  routebuilder gap.
- **`comparisonBackHref` / `comparisonBackLabel`** come from the
  viewmodel (`BackHref`, `BackLabel`) — verify they survive the
  multi-step flow (queue → finding → compare).
- **Field table has `tabindex="0"`, `role="region"`,
  `aria-label`** for horizontal scroll — verify keyboard scroll
  works.
- **Mobile cards duplicate the desktop table** — tests must cover
  both layouts.

## See also

- [11-review-queue.md](11-review-queue.md)
- [19-merge-review-ledger.md](19-merge-review-ledger.md)