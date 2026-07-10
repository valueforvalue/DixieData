# 19a — Review Queue Resolved tab

- **Route**: `/review-queue?tab=resolved` (GET), optional `?person=ID` filter
- **Builder**: none
- **Template**: `internal/templates/review_queue.templ` (re-uses the same
  `ReviewQueueView` templ as the Open tab; switches activeTab via the
  `?tab=resolved` query param)
- **Layout**: both
- **Owner**: package `templates`

## Regions (relaxed mode)

```
┌── Review Queue ───────────────────────────────────────────────────┐
│ (intro copy — same as Open tab)                                  │
│ ┌──[Open] [Resolved]──┐  (tab strip; Resolved is the active state)│
│                                                                    │
│ if tab=resolved:                                                   │
│   [Resolved history placeholder panel]                            │
│   "Resolved conflict rows from merge_review_conflicts will surface │
│    here, scoped by ?person=ID when the deep-link from a Person   │
│    Record arrives. The data is in place; the Resolved listing is  │
│    the next slice-4 follow-up (the per-soldier sub-page is gone  │
│    after issue #455)."                                           │
│                                                                    │
│ else (tab=open):                                                   │
│   (existing /review-queue list + bulk form + pagination)         │
└────────────────────────────────────────────────────────────────────┘
```

## Panels / tabs

| ID | Region | Contents |
| --- | --- | --- |
| `panel.review-queue.list` | (same as Open tab) | Review queue bulk form + entry list + pagination |
| `panel.review-queue.resolved` | Resolved-tab body | Placeholder copy + future resolved-row listing |

## Atomic components

- `Button` — (none added; the tab strip is two `<a role="tab">` anchors)
- `Card` — Resolved history card (future)
- `Pill` — (none added)

## HTMX wiring

| Trigger | Verb | URL | Target | Notes |
| --- | --- | --- | --- | --- |
| Open tab anchor | GET | `/review-queue` | (full nav) | Active when `activeTab == "open"`; aria-selected="true" |
| Resolved tab anchor | GET | `/review-queue?tab=resolved` | (full nav) | Active when `activeTab == "resolved"`; aria-selected="true" |
| Resolved + per-person filter | GET | `/review-queue?tab=resolved&person={id}` | (full nav) | Slice-4 follow-up; today's stub ignores `person` |

## Footguns

- **No listing data yet** — the Resolved tab is a placeholder panel
  today; the audit facade `ResolvedConflicts(page, pageSize, personID)`
  query + the result-table render are the slice-4 follow-up.
- **`a.audit.ResolvedConflicts` doesn't exist** — if a future commit
  lands the data path before the render path, the handler carries a
  nil pointer crash. The data path + render path MUST land in the
  same commit.
- **Bookmarking the tab requires the query param** — `/review-queue`
  alone always lands on the Open tab. Refresh always preserves the
  query through the tab anchor's href.
- **The tab strip is server-rendered, not HTMX-driven** — clicking
  Resolved is a full nav. Future slice could swap to HTMX + pushState
  if telemetry shows users frequently switch.
