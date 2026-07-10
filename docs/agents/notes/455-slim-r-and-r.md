# Slice Plan — Issue #455 (slim Research & Review foldout)

Continuation of the audit posted on issue #455 (commit prior to this
document). The maintainer answered Q1-Q5 + the follow-ups on the
RQ-placement update: the shape is **3 R&R sub-pages + Review Queue
relocated into the foldout + `?person=ID` query param + trigger badge**.
This document pins those answers and lays out the slice decomposition.
Each slice ships as one reviewable commit; each slice has a RED-first
regression net.

## Locked decisions (from issue #455 comment trail)

| # | Decision | Choice | Source |
|---|---|---|---|
| 1 | Core scope | 3 (Timeline / Research Log / Research Collections) + Change Person | Q1, 2026-07-10 |
| 2 | Camaraderie fate | Drop; replace with Insights drilldown scoped to unit | Q2-Q3, 2026-07-10 |
| 3 | Research Pack fate | Drop entirely | Q2-Q3, 2026-07-10 |
| 4 | Merge Review Ledger fate | Fold into Review Queue Resolved tab | Q4, 2026-07-10 |
| 5 | Person context | `?person=ID` query param | Q5, 2026-07-10 |
| 6 | RQ nav placement | Top item in R&R foldout; drop top-nav pill | Q-A, 2026-07-10 |
| 7 | Trigger badge shape | Inline `<span>` between label and `▾`, polled via existing `/layout/review-count` | Q-B, 2026-07-10 |
| 8 | Slice scope for #6 + #7 | Re-shape Slice 1; new Slice 1.5 | Q-C, 2026-07-10 |

## Slice decomposition

| # | Slice | Commit shape |
|---|---|---|
| 0 | Prefactor audit | Doc-only |
| 1 | Foldout trim + RQ relocation + soldier_card tiles | Templ + layout |
| 1.5 | Foldout trigger badge (`triggerSlot` primitive + htmx) | Templ + primitive + frontend |
| 2 | `?person=ID` query + picker pivot + cookie machinery delete | Backend + frontend |
| 3 | Camaraderie + Research Pack removal | Delete files + dispatch branches |
| 4 | Review Queue Resolved tab + Ledger removal | Handler extension + delete files |
| 5 | Docs/glossary/smoke-probe rim | Docs only |

Slices 1 + 1.5 land as **one PR, two commits** (slice-split encodes
which files move together, not which PR ships them). Other slices each
land on their own commit per `feature-protocol.md` three-tier rule.

## Slice 0 — Prefactor audit findings (this commit)

Six blockers audited before any code lands. All clear:

| Blocker | Finding |
|---|---|
| `internal/cookies/context.go` consumer sweep | Only picker machinery + lifecycle (`app.go`, `lifecycle.go`) + `routebuilder.go`. No production code outside the picker touches it. Safe to delete whole file in Slice 2. |
| `pickerGated` map at `soldiers_handlers.go:495-501` | Confined to `handleSoldierByID`. No other dispatcher reads it. Slice 2 deletion is local. |
| `UnitMembership` table | Does not exist as a separate table. Camaraderie uses inline unit-string matching against `soldiers.unit`. No orphaned-table concern for Slice 3. |
| `research_pack_views` table | Does not exist. Slice 3 deletes presentation + service code only; no schema migration needed. |
| `merge_review_conflicts` table | Active in `backup_service.go` (replay paths) + `cli_debug.go:178-179` (diagnostic queries). Slice 4 keeps the table; only the per-person sub-page dies. |
| `merge_review_conflicts` FK column names (v60 rename) | `local_record_id` / `left_record_id` / `right_record_id` (v60+); block-60 migration renamed from `*_soldier_id`. No additional rename needed. |

## Foldout menu shape (post-slim, after Slice 1)

```
[Research & Review ▾]
├ Open Review Queue           → /review-queue                       (top item)
├ Open Timeline               → /soldiers/{id}/timeline?person={id}
├ Open Research Log           → /soldiers/{id}/research-log?person={id}
├ Research Collections        → /research-collections
├ Change Person…              → /research                            (search-only landing)
```

Each soldier-scoped link: `?person={id}` from the picker (Slice 2) or a
soldier_card tile. No cookie. Direct deep-links work without 303.
The trigger button shows a live count badge polled via the existing
`/layout/review-count` endpoint (Slice 1.5) — same endpoint serves
both old top-nav pill (gone) and new foldout trigger badge.

## Precedents

- **Issue #378** (the foldout's birth) — same picker cookie machinery
  Slice 2 now deletes. Worker-friendly: this is the inverse of the
  trade #378 accepted (anti-bookmarking person-ctx). Slice 2 trades it
  back when the maintenance cost outweighs the anti-bookmark benefit.
- **`docs/agents/feature-protocol.md`** three-tier commit rule —
  each slice below is a separate reviewable commit.
- **`AGENTS.md` apply-sites law** — every wire-deletes a Go handler
  + the matching UI apply-site in the same commit.
