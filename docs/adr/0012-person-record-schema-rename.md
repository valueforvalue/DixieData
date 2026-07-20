# Complete v60 Person Record schema rename — sweep 'soldiers' table + columns to canonical terminology

## Status

Proposed 2026-07-21.

## Context

ADR 0002 (v60 partial rename) renamed the user-facing "Soldier"
terminology to the canonical "Person Record" glossary term at
the **type alias + URL** layer only. The schema layer is still
pre-v60:

- Table: `soldiers` (should be `person_records`)
- Columns: `soldier_id`, `spouse_soldier_id`, `candidate_soldier_id`,
  `linked_soldier_id`, etc.
- Go struct: `models.Soldier` (should be `models.PersonRecord`)
- Cross-table FK names: `person_record_id` (the v60 partial rename
  in FK names, but the source column is still `soldier_id` in the
  originating table)

The same business concept ("Person Record") is represented by 3+
different identifiers across 3+ layers. This is the DRY violation
the 2026-07 pragmatic-programmer diagnostic flagged (row 6/10).

The v60 rename was partial because URLs are user-visible state
(bookmarks, browser history), schema columns are persisted in
`.ddbak` backups, and JSON field names are embedded in exported
archives shared between users. The phased approach (type alias
→ facade rename → schema rename → shim removal) was deliberate.
This ADR documents the **schema layer** phase: the coordinated
sweep of the `soldiers` table name, column names, Go type name,
and file paths to the canonical `person_records` / `PersonRecord`
form.

## Decision

### Table rename: `soldiers` → `person_records`

The `soldiers` table is renamed to `person_records` via an additive
migration:

1. `CREATE TABLE person_records (...)` with the new schema
2. `INSERT INTO person_records SELECT * FROM soldiers`
3. Drop old table, rename new table (inside a single transaction)

This is the SQLite-safe pattern (no `ALTER TABLE RENAME` dependency
on SQLite ≥3.25). The migration is reversible: down-migration does
the inverse.

### Column renames

| Old name | New name | Rationale |
|---|---|---|
| `soldier_id` (FK source) | `record_id` | Generic enough for Person/Event Records; the FK target is `person_records.id` |
| `spouse_soldier_id` | `spouse_record_id` | FK to `person_records`; "spouse" is a Person Record role, not a separate table |
| `candidate_soldier_id` | `candidate_record_id` | Aggregated soldier FK; "candidate" is a Person Record role |
| `linked_soldier_id` | `linked_record_id` | Linked soldier FK |
| `person_record_id` (FK name, already v60) | (unchanged) | Already canonical; verify consistency across all tables |
| `person_id` (inconsistent in event_person_links, person_record_tags) | `person_record_id` | Normalize to the canonical FK name |

### Go type: `models.Soldier` → `models.PersonRecord`

The `models.Soldier` struct is renamed to `models.PersonRecord`.
A type alias `type Soldier = PersonRecord` is preserved for one
release cycle so external consumers (Wails bindings, exported
JSON) can migrate.

Go field names follow the column renames:
- `SoldierID` / `SpouseSoldierID` → `RecordID` / `SpouseRecordID`
- Field-name aliases preserved via `json:"old_name,omitempty"` for
  one release cycle.

### File paths

| Old path | New path |
|---|---|
| `internal/records/soldier_service.go` | `internal/records/person_record_service.go` |
| `internal/templates/components/soldier_card.templ` | `person_record_card.templ` |
| All `*_soldier_*` file/dir names | `*_person_record_*` |

### URL surface (Slice 6)

`/soldiers/{id}` → `/person-records/{id}` with 301 redirects from
the old URLs for one release cycle. `/soldiers/new`,
`/soldiers/{id}/edit` follow the same pattern.

### Migration policy

- Every bump of `CurrentSchemaVersion` triggers a paired migration
  note in `docs/migrations/` per the existing `bump-version.ps1` policy.
- Backup archives shipped before the migration must still load.
  A compatibility view (`CREATE VIEW soldiers AS SELECT * FROM
  person_records`) is kept for one release cycle.
- Down-migration: the reverse `CREATE TABLE soldiers ...` +
  `INSERT INTO soldiers SELECT * FROM person_records` + rename.

### Slice plan

| Slice | Scope | Blast radius |
|---|---|---|
| **1** | ADR (this document) | Docs only |
| **2** | Schema migration (`soldiers` → `person_records`; column renames) | High — every query breaks |
| **3** | Models layer (`models.Soldier` → `models.PersonRecord`; field renames; type alias) | High — every package that imports models |
| **4** | Repos layer (PersonRecordRepo column aliases, FK name consistency) | Medium — repo impl layer |
| **5** | Service layer (file paths, function names, aliases) | Medium — service + handler call sites |
| **6** | URL surface (/person-records/..., 301 redirects from /soldiers/...) | Medium — smoke probes, bookmarks |
| **7** | Cross-table FK consistency sweep | Low — verification only |

Each slice is independently reviewable and ships as a separate PR.

## Consequences

### Easier

- **One name for one concept.** Every developer (human or LLM) sees
  `person_records` / `PersonRecord` at every layer. No translation
  table needed.
- **Schema matches glossary.** CONTEXT.md glossary term "Person Record"
  now matches the database table name.
- **DRY violation resolved.** The 2026-07 pragmatic-programmer diagnostic
  row 6/10 ("same concept, 3+ identifiers") is cleared.

### Harder

- **Schema migration is high-risk.** Slice 2 touches every query in
  the codebase. The additive migration pattern (new table + SELECT +
  drop + rename) is proven but must be tested against the full backup
  archive restore path.
- **URL redirects are user-visible.** Every smoke probe + bookmark
  that references `/soldiers/` must be updated. The 301 redirects
  are a one-release-cycle safety net.
- **Type alias removal.** The `Soldier = PersonRecord` alias is a
  one-release-cycle compatibility shim. Removing it in slice N+1
  requires a coordinated sweep of all remaining `Soldier` references.

### Trade-offs

- **Additive vs in-place migration.** The additive pattern doubles
  disk I/O during migration but is safer (no `ALTER TABLE RENAME`
  dependency). The trade-off is acceptable because migrations run
  at app startup, not during normal use.
- **One release cycle of dual names.** The type alias + 301 redirects
  + compatibility view add complexity for one release. The payoff
  is zero-downtime for users who update mid-cycle.

## References

- [ADR 0002 (v60 partial rename)](0002-glossary-tier2-person-record-rename.md)
- [CONTEXT.md](../../CONTEXT.md) §"Person Record" glossary term
- [CONTEXT.md](../../CONTEXT.md) §"Laws (non-negotiable)" — the `Person Record` term
- Issue #623 (this ADR)
- 2026-07 pragmatic-programmer diagnostic, DRY row

## Author

Jeremy Morris (@jeremymorris) — 2026-07-21
