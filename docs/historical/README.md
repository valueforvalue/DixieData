# Historical Artifacts

This directory holds docs and audit results retained for traceability
but **not loaded by default** by an AI agent. See
[`CONTEXT.md`](../../CONTEXT.md) §Historical Artifact for the
glossary entry and [`docs/agents/INDEX.md`](../../agents/INDEX.md)
for the Tier 2 (on-demand) progressive-disclosure rule.

## Why these exist

Some docs are no longer live but still useful for tracing a past
decision ("why did the code look like this before commit X?"). They
earn their keep by sitting quietly in the working tree, available
when an issue or PR explicitly names them. They cost their keep by
being in the way of every agent session.

The compromise: keep them, but route them through a known location
with a clear rule. Agents load them only when directed.

## Subdirectories

| Subdir | What lives here | Source after move |
|---|---|---|
| `handoffs/` | Superseded session handoffs and SNAPSHOTs | `docs/agents/handoff-*.SNAPSHOT.md` |
| `audit-resolved/` | Audits with `STATUS: RESOLVED` banners | `docs/audit/resolved/` + `docs/audit/static-web-archive-audit-2026-06.md` |
| `audit-runs/` | Audit round results past the latest 3 | `audit/reports-r2/`, `audit/reports-r3/`, `audit/notes-*.md`, `audit/audit-interactive-report.json`, `audit/manual-walk-report.json` |
| `renderings/` | Per-surface Typst iteration rounds past the latest 3 | `docs/renderings/<surface>/round-N.md` (older than the latest 3) |

## Retention rule

For each category, the **latest 3 rounds stay in the working tree
at their original location** (or as a date-stamped file for audit
runs). Older rounds move here.

When the latest-3 count drops below 3 (because nothing newer
exists), nothing moves. The rule is "keep up to 3 in tree; older
goes here", not "always have 3 in tree."

### Audit runs (`audit-runs/`)

`audit/reports/` (the current round) and the two previous rounds
(`audit/reports-r2/`, `audit/reports-r3/` analog) stay in
`audit/`. Everything older (round 4, 5, ...) moves here.

`audit/notes-<date>.md` and the dated JSON reports are dated; each
new round creates a new dated file. The latest 3 stay at the
`audit/` root. Older move to `audit-runs/<date>/`.

### Renderings (`renderings/`)

`docs/renderings/<surface>/` keeps:
- `pre-iteration.pdf` (gitignored after slice 10)
- `review.md` (the current round, latest version)

Older `review.md` rounds (when multiple exist for one surface) get
renamed to `round-N-<date>.md` and moved to `renderings/<surface>/`.
The current round stays in `docs/renderings/<surface>/review.md`.

When no current round exists for a surface (the surface was
abandoned), the entire surface folder moves here.

### Handoffs (`handoffs/`)

A handoff with `> **Status: HISTORICAL SNAPSHOT.**` in its banner
is superseded by the work that came after it. It moves here
immediately. Future handoffs that capture "this iteration is done,
the next session starts from commit X" follow the same pattern.

A live handoff (current iteration in progress) stays at
`docs/agents/handoff-<date>-<topic>.md` until the iteration
completes. Then it moves here.

### Audit-resolved (`audit-resolved/`)

A `STATUS: RESOLVED` audit moves here immediately when the work
that resolved it ships. The move is part of the resolution PR;
no separate retention window.

## How to use these

1. An issue or PR explicitly names a file under `docs/historical/`.
   Load it. Read the banner for the resolution commit / status.
2. An agent is investigating a past decision. Search this directory
   for the relevant topic. If found, load it.
3. Otherwise: don't load. Don't ls this directory at session
   start. Don't grep it for context.

## Adding a file here

Don't add files here unless they meet one of the categories above.
A live doc that hasn't been superseded belongs at its working-tree
location, not here.

If you're not sure whether a doc is "historical" or "live":

- Does it have a `STATUS: RESOLVED` banner, a `HISTORICAL
  SNAPSHOT` banner, or is it a round-N file past the latest 3?
  → Yes: it goes here.
- Does it describe a system that's still in use, with no banner?
  → No: keep it where it is.
- Is it a dated audit run from a previous sweep? → Yes: latest 3
  stay in `audit/`; older go to `audit-runs/`.

## Cold storage (out of repo)

Files older than 12 months that are still being referenced by
issues or PRs stay here. Files older than 12 months that aren't
referenced should be considered for cold storage (separate repo,
external archive). Cold storage is **not** implemented yet; if
you hit the 12-month boundary, open an issue.

## References

- [`CONTEXT.md`](../../CONTEXT.md) §Historical Artifact — glossary
- [`docs/agents/INDEX.md`](../../agents/INDEX.md) §Tier 2 — the
  progressive-disclosure rule
- [`docs/agents/feature-protocol.md`](../../agents/feature-protocol.md)
  §When to load what — the same idea, scoped to feature work