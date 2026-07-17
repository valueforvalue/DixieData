# ADR template

Canonical shape for new Architecture Decision Records in this repo.
Mirrors the existing ADRs 0001-0010. New ADRs should be filed
under `docs/adr/NNNN-<slug>.md` where `NNNN` is the next free
four-digit number (currently 0011).

## Shape

```
# <Title in imperative mood>

## Status

<Proposed | Accepted | Deprecated | Superseded by ADR-NNNN> YYYY-MM-DD.

## Context

<The forces in play. The problem this ADR resolves. Bullet form
is fine; prose is fine; both are fine. Cite the issue, the
follow-up conversation, or the upstream constraint that
motivated the decision.>

## Decision

<The choice. State it cleanly. Include the rationale inline
(why this option over the alternatives).>

## Consequences

<What becomes easier. What becomes harder. What trade-offs
are now locked in.>

## References

<Cross-links to related ADRs (by relative path), related
docs (by relative path), and any external conversation /
issue / PR / commit that motivated the decision. Use the
existing ADR cross-link style: `[<title>](<slug>.md)` for
sibling ADRs.>

## Author

<Name> (<handle>) — <date>

Examples: `Jeremy Morris (@jeremymorris) — 2026-07-09` for
new work; `Jeremy Morris (@jeremymorris) — backfilled 2026-07-09`
when retrofitting an older ADR.
```

## Conventions

- **Numbering**: zero-padded 4 digits (`0001`, `0009`, `0102`).
  Pick the next free number; do not backfill skipped numbers.
- **Slug**: kebab-case short identifier; matches the file
  name minus the `NNNN-` prefix. Example: ADR `0011-repository-seam-person-record`
  would land as `docs/adr/0011-repository-seam-person-record.md`.
- **Reference material**: companion documents (catalogs,
  token tables, index files) live under `docs/adr/references/`
  and keep the ADR's number as a prefix. Example: the ADR
  `0003-design-system-tokens` ships its token catalog at
  `docs/adr/references/0003-design-system-tokens.md`. This
  keeps the slug search (e.g. `grep 0003 docs/adr/INDEX.md`)
  consistent across both halves.
- **Status dates**: ISO 8601 (`YYYY-MM-DD`). When amending an
  ADR, add a `## Amendment` section below `## Status` and
  document the change with a fresh date.
- **Author field**: required. See the shape above. Backfills
  are acceptable; missing Author is a `needs-info` flag for
  future audits.

## Author

Jeremy Morris (@jeremymorris) — 2026-07-09