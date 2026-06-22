# PDF Rendering Iteration

This directory is the surface area for the rendering-iteration loop. It
holds one folder per export type. Each folder contains a baseline PDF and
a review file the user annotates.

## Workflow

```
                ┌──────────────────────────────────────┐
                │  1. Edit templates/*.typ to address   │
                │     the user's annotations            │
                └────────────────┬─────────────────────┘
                                 │
                                 ▼
                ┌──────────────────────────────────────┐
   2. Rebuild tune (auto) and  │  make render-round    │
      re-render every surface  │  (or -Round 2 etc.)   │
                └────────────────┬─────────────────────┘
                                 │
                                 ▼
                ┌──────────────────────────────────────┐
                │  3. User opens round-N.pdf alongside │
                │     review.md, writes new notes,     │
                │     marks old notes resolved         │
                └────────────────┬─────────────────────┘
                                 │
                                 ▼
                          (back to step 1)
```

Round 1 is the baseline. Each subsequent round writes
`round-N.pdf` (the new render) **next to** `pre-iteration.pdf` (the
original baseline), so a side-by-side diff in the PDF viewer shows what
changed.

## Surfaces

| Surface | Template | Driver |
|---|---|---|
| `single-soldier-landscape` | `soldier_landscape.typ` | soldier id=1 |
| `single-soldier-portrait`  | `soldier_portrait.typ`  | soldier id=1 |
| `single-widow-landscape`   | `widow_landscape.typ`   | widow id=61 |
| `single-widow-portrait`    | `widow_portrait.typ`    | widow id=61 |
| `bulk-sorted`              | `bulk_soldier.typ`      | full archive, sort=last_name |
| `bulk-grouped-pension-state` | `bulk_soldier.typ`    | grouped by pension state |
| `bulk-grouped-burial-location` | `bulk_soldier.typ`  | grouped by buried-in |
| `anniversary`              | `anniversary.typ`       | month=5 |
| `insights`                 | `analytics_summary.typ` | live snapshot |

The record IDs are hard-coded in `scripts/render-round.ps1`. The bulk
exports render the entire archive.

## Terminology

`TERMINOLOGY.md` is the source of truth for terms that appear in the
PDFs. When you change a term, update the glossary first, then the
template. The agent's code changes cite glossary entries by anchor.

## How to annotate

Open the round-N PDF in a PDF viewer. For each issue, write a
section in the surface's `review.md`:

```
### <short title>

- Term: <term from TERMINOLOGY.md by anchor #term-…>
- Location: <template:line or general area>
- What I see: <visible string or layout>
- What I want: <proposed visible string or layout>
- Why: <one sentence>
```

The agent reads the file, makes the change, and re-renders. The next
round's PDF is timestamped so you can compare visually.
