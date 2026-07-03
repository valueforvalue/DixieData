# Ghost-link token swap — contrast baseline

**Date:** 2026-07-02
**Issue:** #291 Phase 2, ghost-link family
**Decision:** DEFER swap to Phase 3 (#292) due to WCAG regression.
**Decision driver:** `ink-slate #666870` on the sepia body gradient drops to AA-large-only (top stop) and **FAIL** (bottom stop).

## Family checklist item (from #291)

> Ghost link — uses `#324253` (`ink-mid`). Issue body explicit ask: replace with `ink-slate` (`#666870`) for warmer secondary text.

## Visual baseline

The ghost-link class renders on the calendar day page (Edit / Open Record / Cancel buttons at lines 118/158/176 of `internal/templates/calendar_day.templ`) and on the entry form (Cancel button at line 483 of `internal/templates/entry_form.templ`). All four surfaces sit on top of the body sepia gradient (`linear-gradient(180deg, var(--bg-sepia-top) 0%, var(--bg-sepia-mid) 42%, var(--bg-sepia-bottom) 100%)`).

Computed `color` of `.ghost-link` is the only thing changing. Compiled CSS at `6704c3d..9ff9e14`:

```css
/* Current (live, post-revert) — token-mapped @apply, original hex */
.ghost-link {
  --tw-text-opacity: 1;
  color: rgb(50 66 83 / var(--tw-text-opacity, 1)); /* #324253 = ink-mid */
}
.ghost-link:focus-visible { outline-color: #324253; }
```

```css
/* Proposed (reverted) */
.ghost-link {
  --tw-text-opacity: 1;
  color: rgb(102 104 112 / var(--tw-text-opacity, 1)); /* #666870 = ink-slate */
}
.ghost-link:focus-visible { outline-color: #666870; }
```

## WCAG contrast across the sepia gradient

Computed using the standard sRGB → relative-luminance formula at every gradient stop. Body bg rendered behind ghost-link is whichever gradient stop the link sits over; the link is 0.9rem (14.4px), below the AA-large threshold of 18pt = 24px regular or 14pt = 18.5px bold. So the gate is **AA-normal ≥ 4.5:1**.

| Stop | Color | ink-mid #324253 (current) | ink-slate #666870 (proposed) | Delta |
|---|---|---:|---:|---:|
| top (lightest) | `#d7d2c9` | 6.84:1 — **AA pass** | 3.69:1 — AA-large only | **-3.15:1** |
| mid | `#c9c2b5` | 5.82:1 — **AA pass** | 3.14:1 — AA-large only | **-2.68:1** |
| bottom (darkest) | `#b9b1a3` | 4.84:1 — **AA pass** | 2.61:1 — **FAIL** | **-2.23:1** |

## Decision rationale

The pre-change `ink-mid #324253` passes AA-normal at every gradient stop. The proposed `ink-slate #666870` only passes AA-large on the lightest stop and **fails on the darkest stop entirely**. That is not a "softer confirmation" — it is a contrast regression that lowers the calendar day's primary nav affordance to illegible on a chunk of the rendered viewport.

The aesthetic argument (warmer secondary text on the warm body) is real and survives — `ink-slate` does fight the sepia less than `ink-mid` does. But the contrast gate isn't negotiable for normal-weight 14px link text.

## Phase 3 options (when this lands)

When the visual-change commit lands (as part of #292 or its own follow-up), the desired outcome is "warmer" — not "softer". Three directions, all require WCAG re-verification:

1. **`ink-deep #1f2b38`** — current primary text token. Loses the ghost-link visual distinction; ghost becomes indistinguishable from primary. Not recommended.
2. **A new token** between `ink-mid` (#324253) and `ink-deep` (#1f2b38) at, e.g., `#293546` or `#2c3a4a`. Re-tuned to land at ~4.8:1 on the darkest gradient stop. This is the cleanest win — a new token, a one-family-only migration, no contrast gate to break.
3. **Bump `.ghost-link` to `1rem` (16px) or `600` weight** so it qualifies for AA-large. Less visual change than a swap, more typing, marginal design tradeoff.

The recommendation is **(2)** — add the new token in a follow-up commit (separate issue, not #291 Phase 2), gated on a fresh WCAG pass per the Phase 3 visual-review protocol.

## Verification commands

```bash
node audit/reports/phase2-ghost-link-baseline.mjs
# (the .mjs source lives in .scratch/wcag-ghost.mjs for now)
```
